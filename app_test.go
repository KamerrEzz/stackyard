package main

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"stackyard/internal/dbengine"
	"stackyard/internal/docker"
	"stackyard/internal/storage"

	mysqldriver "github.com/go-sql-driver/mysql"
)

// fakeSchemaEngine is a minimal dbengine.Engine stub used to exercise
// ListSchemasForSession/ListTablesForSession's session-lookup and
// nil-slice-normalization behavior without a live database connection — the
// real per-engine ListSchemas/ListTables logic is already covered by
// internal/dbengine's own postgres/mysql integration tests (tasks.md 3.2).
type fakeSchemaEngine struct {
	schemas        []string
	schemasErr     error
	tables         []dbengine.TableInfo
	tablesErr      error
	foreignKeys    []dbengine.ForeignKey
	foreignKeysErr error
}

func (f *fakeSchemaEngine) Connect(context.Context) error { return nil }
func (f *fakeSchemaEngine) Ping(context.Context) error    { return nil }
func (f *fakeSchemaEngine) Query(context.Context, string) (*dbengine.QueryResult, error) {
	return nil, nil
}

func (f *fakeSchemaEngine) Exec(context.Context, string, ...any) (*dbengine.QueryResult, error) {
	return nil, nil
}

func (f *fakeSchemaEngine) ListSchemas(context.Context) ([]string, error) {
	return f.schemas, f.schemasErr
}

func (f *fakeSchemaEngine) ListTables(context.Context, string) ([]dbengine.TableInfo, error) {
	return f.tables, f.tablesErr
}

func (f *fakeSchemaEngine) ListForeignKeys(context.Context, string) ([]dbengine.ForeignKey, error) {
	return f.foreignKeys, f.foreignKeysErr
}

func (f *fakeSchemaEngine) Close() error { return nil }

func newTestApp(t *testing.T) *App {
	t.Helper()

	path := filepath.Join(t.TempDir(), "stackyard-test.db")
	db, err := storage.OpenAt(path)
	if err != nil {
		t.Fatalf("storage.OpenAt(%q) failed: %v", path, err)
	}
	t.Cleanup(func() { db.Close() })

	return &App{db: db}
}

func TestApp_CheckPortAvailable_DetectsTakenAndFreePorts(t *testing.T) {
	a := newTestApp(t)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to bind a test listener: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port

	available, err := a.CheckPortAvailable(port)
	if err != nil {
		t.Fatalf("CheckPortAvailable(%d) returned unexpected error: %v", port, err)
	}
	if available {
		t.Errorf("CheckPortAvailable(%d) = true, want false while a listener is bound to it", port)
	}

	if closeErr := ln.Close(); closeErr != nil {
		t.Fatalf("failed to release the listener: %v", closeErr)
	}

	available, err = a.CheckPortAvailable(port)
	if err != nil {
		t.Fatalf("CheckPortAvailable(%d) returned unexpected error: %v", port, err)
	}
	if !available {
		t.Errorf("CheckPortAvailable(%d) = false, want true immediately after releasing the only listener on it", port)
	}
}

func TestApp_SuggestFreePort_SkipsOSLevelTakenPort(t *testing.T) {
	a := newTestApp(t)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to bind a test listener: %v", err)
	}
	defer ln.Close()
	takenPort := ln.Addr().(*net.TCPAddr).Port

	got, err := a.SuggestFreePort(takenPort)
	if err != nil {
		t.Fatalf("SuggestFreePort(%d) returned unexpected error: %v", takenPort, err)
	}
	if got == takenPort {
		t.Errorf("SuggestFreePort(%d) = %d, want a port other than the one currently bound", takenPort, got)
	}

	if !a.mustCheckPortAvailable(t, got) {
		t.Errorf("SuggestFreePort(%d) suggested %d, which is not actually free", takenPort, got)
	}
}

func TestApp_SuggestFreePort_SkipsStorageRecordedPort(t *testing.T) {
	a := newTestApp(t)

	profile, err := storage.CreateProfile(a.db, "suggest-free-port-test")
	if err != nil {
		t.Fatalf("CreateProfile failed: %v", err)
	}

	recordedPort := findFreePort(t)

	username := "postgres"
	password := "postgres"
	dbName := "postgres"
	_, err = storage.CreateService(a.db, &storage.Service{
		ProfileID:         profile.ID,
		Engine:            storage.EnginePostgres,
		ImageTag:          "postgres:16-alpine",
		HostPort:          recordedPort,
		Username:          &username,
		PasswordEncrypted: &password,
		DBName:            &dbName,
		VolumeName:        "stackyard-vol-suggest-free-port-test",
	})
	if err != nil {
		t.Fatalf("CreateService failed: %v", err)
	}

	got, err := a.SuggestFreePort(recordedPort)
	if err != nil {
		t.Fatalf("SuggestFreePort(%d) returned unexpected error: %v", recordedPort, err)
	}
	if got == recordedPort {
		t.Errorf("SuggestFreePort(%d) = %d, want a port other than one already recorded by another Stackyard service", recordedPort, got)
	}
}

func TestApp_SuggestFreePort_RequiresDB(t *testing.T) {
	a := &App{}

	if _, err := a.SuggestFreePort(5432); err == nil {
		t.Error("SuggestFreePort() with no db configured: expected an error, got nil")
	}
}

func findFreePort(t *testing.T) int {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to get an OS-assigned port: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	if err := ln.Close(); err != nil {
		t.Fatalf("failed to release the listener: %v", err)
	}
	return port
}

func (a *App) mustCheckPortAvailable(t *testing.T, port int) bool {
	t.Helper()
	available, err := a.CheckPortAvailable(port)
	if err != nil {
		t.Fatalf("CheckPortAvailable(%d) returned unexpected error: %v", port, err)
	}
	return available
}

func TestApp_DuplicateProfile_ReturnsNewSummaryWithServices(t *testing.T) {
	a := newTestApp(t)

	created, err := a.CreateProfile("dup-source", []ServiceRequest{{Engine: storage.EnginePostgres}})
	if err != nil {
		t.Fatalf("CreateProfile failed: %v", err)
	}

	dup, err := a.DuplicateProfile(created.Profile.ID)
	if err != nil {
		t.Fatalf("DuplicateProfile failed: %v", err)
	}

	if dup.Profile.ID == created.Profile.ID {
		t.Fatal("expected DuplicateProfile to return a new profile ID, not the original")
	}
	if dup.Profile.Name != "dup-source (copy)" {
		t.Errorf("expected duplicate name %q, got %q", "dup-source (copy)", dup.Profile.Name)
	}
	if len(dup.Services) != len(created.Services) {
		t.Fatalf("expected %d duplicated services, got %d", len(created.Services), len(dup.Services))
	}
	if dup.Services[0].ID == created.Services[0].ID {
		t.Error("expected duplicated service to have a new ID, not alias the original")
	}
}

func TestApp_DuplicateProfile_RequiresDB(t *testing.T) {
	a := &App{}

	if _, err := a.DuplicateProfile(1); err == nil {
		t.Error("DuplicateProfile() with no db configured: expected an error, got nil")
	}
}

func TestApp_RenameProfile_PersistsNewName(t *testing.T) {
	a := newTestApp(t)

	created, err := a.CreateProfile("before-rename", []ServiceRequest{{Engine: storage.EnginePostgres}})
	if err != nil {
		t.Fatalf("CreateProfile failed: %v", err)
	}

	renamed, err := a.RenameProfile(created.Profile.ID, "after-rename")
	if err != nil {
		t.Fatalf("RenameProfile failed: %v", err)
	}
	if renamed.Profile.Name != "after-rename" {
		t.Errorf("expected renamed profile name %q, got %q", "after-rename", renamed.Profile.Name)
	}

	fetched, err := a.ListProfiles()
	if err != nil {
		t.Fatalf("ListProfiles failed: %v", err)
	}
	if len(fetched) != 1 || fetched[0].Profile.Name != "after-rename" {
		t.Errorf("expected the rename to persist across ListProfiles, got %+v", fetched)
	}
}

func TestApp_RenameProfile_RequiresDB(t *testing.T) {
	a := &App{}

	if _, err := a.RenameProfile(1, "new-name"); err == nil {
		t.Error("RenameProfile() with no db configured: expected an error, got nil")
	}
}

func TestApp_DeleteProfile_RequiresDB(t *testing.T) {
	a := &App{}

	if err := a.DeleteProfile(1); err == nil {
		t.Error("DeleteProfile() with no db configured: expected an error, got nil")
	}
}

func TestDeleteProfileGuardError_BlocksWhenNotStopped(t *testing.T) {
	for _, status := range []string{"running", "partial", "unknown"} {
		if err := deleteProfileGuardError(1, status, nil); err == nil {
			t.Errorf("deleteProfileGuardError(status=%q) = nil, want a blocking error", status)
		}
	}
}

func TestDeleteProfileGuardError_BlocksWhenStatusCheckFailed(t *testing.T) {
	statusErr := context.DeadlineExceeded
	if err := deleteProfileGuardError(1, "stopped", statusErr); err == nil {
		t.Error("deleteProfileGuardError() with a status-check error = nil, want a blocking error")
	}
}

func TestDeleteProfileGuardError_AllowsWhenStopped(t *testing.T) {
	if err := deleteProfileGuardError(1, "stopped", nil); err != nil {
		t.Errorf("deleteProfileGuardError(status=%q) = %v, want nil", "stopped", err)
	}
}

func TestAssignHostPorts_AllFourEnginesGetDistinctDefaultPorts(t *testing.T) {
	requests := []ServiceRequest{
		{Engine: storage.EnginePostgres},
		{Engine: storage.EngineMySQL},
		{Engine: storage.EngineMongoDB},
		{Engine: storage.EngineRedis},
	}

	ports, err := assignHostPorts(map[int]bool{}, requests)
	if err != nil {
		t.Fatalf("assignHostPorts failed: %v", err)
	}

	want := []int{defaultPostgresHostPort, defaultMySQLHostPort, defaultMongoHostPort, defaultRedisHostPort}
	if !reflect.DeepEqual(ports, want) {
		t.Fatalf("assignHostPorts() = %v, want %v", ports, want)
	}

	seen := make(map[int]bool, len(ports))
	for _, p := range ports {
		if seen[p] {
			t.Fatalf("assignHostPorts() produced a duplicate port %d across engines: %v", p, ports)
		}
		seen[p] = true
	}
}

func TestAssignHostPorts_HonorsExplicitHostPort(t *testing.T) {
	requests := []ServiceRequest{{Engine: storage.EnginePostgres, HostPort: 15555}}

	ports, err := assignHostPorts(map[int]bool{}, requests)
	if err != nil {
		t.Fatalf("assignHostPorts failed: %v", err)
	}
	if ports[0] != 15555 {
		t.Errorf("assignHostPorts() = %d, want the explicit HostPort 15555 honored as-is", ports[0])
	}
}

func TestAssignHostPorts_BumpsPastAlreadyUsedPorts(t *testing.T) {
	used := map[int]bool{defaultPostgresHostPort: true, defaultPostgresHostPort + 1: true}
	requests := []ServiceRequest{{Engine: storage.EnginePostgres}}

	ports, err := assignHostPorts(used, requests)
	if err != nil {
		t.Fatalf("assignHostPorts failed: %v", err)
	}
	want := defaultPostgresHostPort + 2
	if ports[0] != want {
		t.Errorf("assignHostPorts() = %d, want %d (first port free past both already-used ports)", ports[0], want)
	}
}

func TestAssignHostPorts_AvoidsCollidingWithinTheSameCall(t *testing.T) {
	requests := []ServiceRequest{
		{Engine: storage.EnginePostgres, HostPort: 20000},
		{Engine: storage.EngineMySQL, HostPort: 20000},
	}

	ports, err := assignHostPorts(map[int]bool{}, requests)
	if err != nil {
		t.Fatalf("assignHostPorts failed: %v", err)
	}
	if ports[0] == ports[1] {
		t.Fatalf("assignHostPorts() gave both requests port %d — a later request in the same call must not collide with an earlier one", ports[0])
	}
}

func TestAssignHostPorts_RejectsUnsupportedEngine(t *testing.T) {
	requests := []ServiceRequest{{Engine: storage.Engine("oracle")}}

	if _, err := assignHostPorts(map[int]bool{}, requests); err == nil {
		t.Error("assignHostPorts() with an unsupported engine: expected an error, got nil")
	}
}

func TestDuplicateEngineError_RejectsRepeatedEngine(t *testing.T) {
	requests := []ServiceRequest{{Engine: storage.EnginePostgres}, {Engine: storage.EnginePostgres}}

	if err := duplicateEngineError(requests); err == nil {
		t.Error("duplicateEngineError() with two Postgres requests: expected an error, got nil")
	}
}

func TestDuplicateEngineError_AllowsOneOfEach(t *testing.T) {
	requests := []ServiceRequest{
		{Engine: storage.EnginePostgres},
		{Engine: storage.EngineMySQL},
		{Engine: storage.EngineMongoDB},
		{Engine: storage.EngineRedis},
	}

	if err := duplicateEngineError(requests); err != nil {
		t.Errorf("duplicateEngineError() with one of each engine = %v, want nil", err)
	}
}

func TestApp_CreateProfile_MultiEngine_CreatesOneServicePerEngineWithDistinctPorts(t *testing.T) {
	a := newTestApp(t)

	summary, err := a.CreateProfile("multi-engine-profile", []ServiceRequest{
		{Engine: storage.EnginePostgres},
		{Engine: storage.EngineMySQL},
		{Engine: storage.EngineMongoDB},
		{Engine: storage.EngineRedis},
	})
	if err != nil {
		t.Fatalf("CreateProfile failed: %v", err)
	}

	if len(summary.Services) != 4 {
		t.Fatalf("expected 4 services, got %d: %+v", len(summary.Services), summary.Services)
	}

	seenEngines := make(map[storage.Engine]bool, 4)
	seenPorts := make(map[int]bool, 4)
	for _, svc := range summary.Services {
		if svc.ProfileID != summary.Profile.ID {
			t.Errorf("service %d has ProfileID %d, want %d", svc.ID, svc.ProfileID, summary.Profile.ID)
		}
		if seenEngines[svc.Engine] {
			t.Errorf("engine %q appeared more than once in the created services", svc.Engine)
		}
		seenEngines[svc.Engine] = true
		if seenPorts[svc.HostPort] {
			t.Errorf("host port %d was assigned to more than one service", svc.HostPort)
		}
		seenPorts[svc.HostPort] = true
	}

	for _, engine := range []storage.Engine{storage.EnginePostgres, storage.EngineMySQL, storage.EngineMongoDB, storage.EngineRedis} {
		if !seenEngines[engine] {
			t.Errorf("expected a service for engine %q, none was created", engine)
		}
	}
}

func TestApp_CreateProfile_RejectsEmptyServiceList(t *testing.T) {
	a := newTestApp(t)

	if _, err := a.CreateProfile("no-services", nil); err == nil {
		t.Error("CreateProfile() with no services: expected an error, got nil")
	}
}

func TestApp_CreateProfile_RejectsDuplicateEngine(t *testing.T) {
	a := newTestApp(t)

	if _, err := a.CreateProfile("dup-engine", []ServiceRequest{
		{Engine: storage.EnginePostgres},
		{Engine: storage.EnginePostgres},
	}); err == nil {
		t.Error("CreateProfile() with two Postgres services: expected an error, got nil")
	}
}

func TestApp_CreateProfile_SecondPostgresProfileAvoidsTheFirstsPort(t *testing.T) {
	a := newTestApp(t)

	first, err := a.CreateProfile("pg-one", []ServiceRequest{{Engine: storage.EnginePostgres}})
	if err != nil {
		t.Fatalf("CreateProfile failed: %v", err)
	}
	second, err := a.CreateProfile("pg-two", []ServiceRequest{{Engine: storage.EnginePostgres}})
	if err != nil {
		t.Fatalf("CreateProfile failed: %v", err)
	}

	if first.Services[0].HostPort == second.Services[0].HostPort {
		t.Errorf("two Postgres profiles both got host port %d, want distinct ports", first.Services[0].HostPort)
	}
}

func TestApp_CreateProfile_RequiresDB(t *testing.T) {
	a := &App{}

	if _, err := a.CreateProfile("no-db", []ServiceRequest{{Engine: storage.EnginePostgres}}); err == nil {
		t.Error("CreateProfile() with no db configured: expected an error, got nil")
	}
}

func TestEngineStarters_MapsEachEngineToItsOwnStartFunction(t *testing.T) {
	want := map[storage.Engine]any{
		storage.EnginePostgres: (*docker.Client).StartPostgresEnvironment,
		storage.EngineMySQL:    (*docker.Client).StartMySQLEnvironment,
		storage.EngineMongoDB:  (*docker.Client).StartMongoEnvironment,
		storage.EngineRedis:    (*docker.Client).StartRedisEnvironment,
	}

	if len(engineStarters) != len(want) {
		t.Fatalf("engineStarters has %d entries, want %d", len(engineStarters), len(want))
	}

	for engine, wantFn := range want {
		gotFn, ok := engineStarters[engine]
		if !ok {
			t.Fatalf("engineStarters is missing an entry for engine %q", engine)
		}
		if reflect.ValueOf(gotFn).Pointer() != reflect.ValueOf(wantFn).Pointer() {
			t.Errorf("engineStarters[%q] does not point at the expected Start<Engine>Environment function", engine)
		}
	}

	distinctPointers := make(map[uintptr]storage.Engine, len(engineStarters))
	for engine, fn := range engineStarters {
		ptr := reflect.ValueOf(fn).Pointer()
		if other, exists := distinctPointers[ptr]; exists {
			t.Errorf("engine %q and %q resolve to the same function pointer — they should dispatch to different Start<Engine>Environment functions", engine, other)
		}
		distinctPointers[ptr] = engine
	}
}

func TestStartServiceEnvironment_UnsupportedEngineReturnsErrorWithoutTouchingDocker(t *testing.T) {
	svc := storage.Service{ID: 1, Engine: storage.Engine("oracle")}

	err := startServiceEnvironment(context.Background(), nil, svc)
	if err == nil {
		t.Fatal("startServiceEnvironment() with an unsupported engine: expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "oracle") {
		t.Errorf("startServiceEnvironment() error = %q, want it to name the unsupported engine", err.Error())
	}
}

func TestConnectionStringForService_DispatchesByEngine(t *testing.T) {
	port := 5000
	cases := []struct {
		engine     storage.Engine
		wantPrefix string
	}{
		{storage.EnginePostgres, "postgres://"},
		{storage.EngineMySQL, "mysql://"},
		{storage.EngineMongoDB, "mongodb://"},
		{storage.EngineRedis, "redis://"},
	}

	for _, tc := range cases {
		svc := storage.Service{Engine: tc.engine, HostPort: port}
		got, err := connectionStringForService(svc)
		if err != nil {
			t.Fatalf("connectionStringForService(engine=%q) failed: %v", tc.engine, err)
		}
		if !strings.HasPrefix(got, tc.wantPrefix) {
			t.Errorf("connectionStringForService(engine=%q) = %q, want prefix %q", tc.engine, got, tc.wantPrefix)
		}
	}
}

func TestConnectionStringForService_RejectsUnsupportedEngine(t *testing.T) {
	svc := storage.Service{Engine: storage.Engine("oracle"), HostPort: 5000}

	if _, err := connectionStringForService(svc); err == nil {
		t.Error("connectionStringForService() with an unsupported engine: expected an error, got nil")
	}
}

func TestApp_ParseConnectionURL_FlattensParamsAndPopulatesFields(t *testing.T) {
	a := &App{}

	got, err := a.ParseConnectionURL("postgres://alice:s3cret@localhost:5432/mydb?sslmode=require")
	if err != nil {
		t.Fatalf("ParseConnectionURL failed: %v", err)
	}

	want := ConnectionFormFields{
		Engine:   storage.EnginePostgres,
		Host:     "localhost",
		Port:     5432,
		Username: "alice",
		Password: "s3cret",
		Database: "mydb",
		Params:   map[string]string{"sslmode": "require"},
	}
	if !reflect.DeepEqual(*got, want) {
		t.Errorf("ParseConnectionURL() = %+v, want %+v", *got, want)
	}
}

func TestApp_ParseConnectionURL_SurfacesTheUnderlyingParseError(t *testing.T) {
	a := &App{}

	_, err := a.ParseConnectionURL("not-a-connection-string")
	if err == nil {
		t.Fatal("ParseConnectionURL() with a malformed string: expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "://") {
		t.Errorf("ParseConnectionURL() error = %q, want it to name the missing scheme separator", err.Error())
	}
}

func TestToConnectionFormFields_KeepsFirstValueOfRepeatedParam(t *testing.T) {
	fields := &dbengine.ConnectionFields{
		Engine: storage.EngineMySQL,
		Host:   "localhost",
		Port:   3306,
		Params: url.Values{"parseTime": []string{"false", "true"}},
	}

	got := toConnectionFormFields(fields)
	if got.Params["parseTime"] != "false" {
		t.Errorf("toConnectionFormFields() Params[\"parseTime\"] = %q, want the first value %q", got.Params["parseTime"], "false")
	}
}

func TestValidateConnectionFormFields_RequiresHost(t *testing.T) {
	fields := ConnectionFormFields{Host: "  ", Port: 5432}
	if err := validateConnectionFormFields(fields); err == nil {
		t.Error("validateConnectionFormFields() with a blank host: expected an error, got nil")
	}
}

func TestValidateConnectionFormFields_RejectsOutOfRangePort(t *testing.T) {
	for _, port := range []int{0, -1, 65536, 100000} {
		fields := ConnectionFormFields{Host: "localhost", Port: port}
		if err := validateConnectionFormFields(fields); err == nil {
			t.Errorf("validateConnectionFormFields(port=%d): expected an error, got nil", port)
		}
	}
}

func TestValidateConnectionFormFields_AcceptsValidHostAndPort(t *testing.T) {
	fields := ConnectionFormFields{Host: "localhost", Port: 5432}
	if err := validateConnectionFormFields(fields); err != nil {
		t.Errorf("validateConnectionFormFields() = %v, want nil", err)
	}
}

func TestBuildPostgresTestConnString_BuildsExpectedURL(t *testing.T) {
	fields := ConnectionFormFields{
		Engine:   storage.EnginePostgres,
		Host:     "localhost",
		Port:     5432,
		Username: "alice",
		Password: "s3cret",
		Database: "mydb",
		Params:   map[string]string{"sslmode": "require"},
	}

	got := buildPostgresTestConnString(fields)
	want := "postgres://alice:s3cret@localhost:5432/mydb?sslmode=require"
	if got != want {
		t.Errorf("buildPostgresTestConnString() = %q, want %q", got, want)
	}
}

func TestBuildPostgresTestConnString_PercentEncodesSpecialCharacters(t *testing.T) {
	fields := ConnectionFormFields{
		Engine:   storage.EnginePostgres,
		Host:     "localhost",
		Port:     5432,
		Username: "al ice",
		Password: "p@ss/word:with?chars",
		Database: "mydb",
	}

	got := buildPostgresTestConnString(fields)
	parsed, err := url.Parse(got)
	if err != nil {
		t.Fatalf("buildPostgresTestConnString() produced an unparseable URL %q: %v", got, err)
	}
	if parsed.User.Username() != "al ice" {
		t.Errorf("round-tripped username = %q, want %q", parsed.User.Username(), "al ice")
	}
	password, _ := parsed.User.Password()
	if password != "p@ss/word:with?chars" {
		t.Errorf("round-tripped password = %q, want %q", password, "p@ss/word:with?chars")
	}
}

func TestBuildPostgresTestConnString_OmitsPasswordSegmentWhenEmpty(t *testing.T) {
	fields := ConnectionFormFields{Engine: storage.EnginePostgres, Host: "localhost", Port: 5432, Username: "alice", Database: "mydb"}

	got := buildPostgresTestConnString(fields)
	want := "postgres://alice@localhost:5432/mydb"
	if got != want {
		t.Errorf("buildPostgresTestConnString() = %q, want %q (no password separator when password is empty)", got, want)
	}
}

func TestBuildMySQLTestDSN_IncludesParseTimeTrue(t *testing.T) {
	fields := ConnectionFormFields{
		Engine:   storage.EngineMySQL,
		Host:     "localhost",
		Port:     3306,
		Username: "root",
		Password: "mysql",
		Database: "mydb",
	}

	dsn := buildMySQLTestDSN(fields)
	if !strings.Contains(dsn, "parseTime=true") {
		t.Errorf("buildMySQLTestDSN() = %q, want it to contain parseTime=true", dsn)
	}

	cfg, err := mysqldriver.ParseDSN(dsn)
	if err != nil {
		t.Fatalf("buildMySQLTestDSN() produced an unparseable DSN %q: %v", dsn, err)
	}
	if !cfg.ParseTime {
		t.Error("re-parsed DSN has ParseTime = false, want true")
	}
	if cfg.User != "root" || cfg.Passwd != "mysql" || cfg.Addr != "localhost:3306" || cfg.DBName != "mydb" || cfg.Net != "tcp" {
		t.Errorf("re-parsed DSN = %+v, fields did not round-trip as expected", cfg)
	}
}

func TestBuildMySQLTestDSN_ForcesParseTimeTrueEvenIfParamsSaysFalse(t *testing.T) {
	fields := ConnectionFormFields{
		Engine:   storage.EngineMySQL,
		Host:     "localhost",
		Port:     3306,
		Username: "root",
		Database: "mydb",
		Params:   map[string]string{"parseTime": "false"},
	}

	dsn := buildMySQLTestDSN(fields)
	if strings.Count(dsn, "parseTime=") != 1 {
		t.Fatalf("buildMySQLTestDSN() = %q, want exactly one parseTime param, not a duplicate", dsn)
	}

	cfg, err := mysqldriver.ParseDSN(dsn)
	if err != nil {
		t.Fatalf("buildMySQLTestDSN() produced an unparseable DSN %q: %v", dsn, err)
	}
	if !cfg.ParseTime {
		t.Error("re-parsed DSN has ParseTime = false even though a conflicting Params[\"parseTime\"]=\"false\" was supplied — the forced true must win, not be silently overridden")
	}
}

func TestBuildMySQLTestDSN_PreservesOtherParams(t *testing.T) {
	fields := ConnectionFormFields{
		Engine:   storage.EngineMySQL,
		Host:     "localhost",
		Port:     3306,
		Username: "root",
		Database: "mydb",
		Params:   map[string]string{"tls": "skip-verify"},
	}

	dsn := buildMySQLTestDSN(fields)
	cfg, err := mysqldriver.ParseDSN(dsn)
	if err != nil {
		t.Fatalf("buildMySQLTestDSN() produced an unparseable DSN %q: %v", dsn, err)
	}
	if cfg.TLSConfig != "skip-verify" {
		t.Errorf("re-parsed DSN TLSConfig = %q, want %q", cfg.TLSConfig, "skip-verify")
	}
}

func TestBuildMySQLTestDSN_HandlesSpecialCharactersInPassword(t *testing.T) {
	fields := ConnectionFormFields{
		Engine:   storage.EngineMySQL,
		Host:     "localhost",
		Port:     3306,
		Username: "root",
		Password: "p@ss:w/ord",
		Database: "mydb",
	}

	dsn := buildMySQLTestDSN(fields)
	cfg, err := mysqldriver.ParseDSN(dsn)
	if err != nil {
		t.Fatalf("buildMySQLTestDSN() produced an unparseable DSN %q: %v", dsn, err)
	}
	if cfg.Passwd != "p@ss:w/ord" {
		t.Errorf("re-parsed DSN password = %q, want %q", cfg.Passwd, "p@ss:w/ord")
	}
}

func TestNewTestEngine_RejectsMongoAndRedisWithClearError(t *testing.T) {
	for _, engine := range []storage.Engine{storage.EngineMongoDB, storage.EngineRedis} {
		_, err := newTestEngine(ConnectionFormFields{Engine: engine, Host: "localhost", Port: 1})
		if err == nil {
			t.Errorf("newTestEngine(engine=%q): expected a not-yet-supported error, got nil", engine)
			continue
		}
		if !strings.Contains(err.Error(), "not yet supported") {
			t.Errorf("newTestEngine(engine=%q) error = %q, want it to say \"not yet supported\"", engine, err.Error())
		}
	}
}

func TestNewTestEngine_RejectsUnsupportedEngine(t *testing.T) {
	if _, err := newTestEngine(ConnectionFormFields{Engine: storage.Engine("oracle")}); err == nil {
		t.Error("newTestEngine() with an unsupported engine: expected an error, got nil")
	}
}

func TestNewTestEngine_ConstructsPostgresAndMySQLEngines(t *testing.T) {
	fields := ConnectionFormFields{Host: "localhost", Port: 5432, Database: "mydb"}

	fields.Engine = storage.EnginePostgres
	if _, err := newTestEngine(fields); err != nil {
		t.Errorf("newTestEngine(postgres) = %v, want nil", err)
	}

	fields.Engine = storage.EngineMySQL
	if _, err := newTestEngine(fields); err != nil {
		t.Errorf("newTestEngine(mysql) = %v, want nil", err)
	}
}

func TestApp_TestConnection_RejectsMissingHost(t *testing.T) {
	a := &App{ctx: context.Background()}

	err := a.TestConnection(ConnectionFormFields{Engine: storage.EnginePostgres, Port: 5432})
	if err == nil {
		t.Error("TestConnection() with a blank host: expected an error, got nil")
	}
}

func TestApp_TestConnection_UnreachableHostFailsWithinTimeout(t *testing.T) {
	a := &App{ctx: context.Background()}

	start := time.Now()
	err := a.TestConnection(ConnectionFormFields{
		Engine: storage.EnginePostgres,
		Host:   "127.0.0.1",
		Port:   1,
	})
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("TestConnection() against an unreachable port: expected an error, got nil")
	}
	if elapsed > testConnectionTimeout+5*time.Second {
		t.Errorf("TestConnection() took %v to fail, want it bounded near testConnectionTimeout (%v)", elapsed, testConnectionTimeout)
	}
}

func TestApp_TestConnection_MongoAndRedisReturnNotYetSupported(t *testing.T) {
	a := &App{ctx: context.Background()}

	for _, engine := range []storage.Engine{storage.EngineMongoDB, storage.EngineRedis} {
		err := a.TestConnection(ConnectionFormFields{Engine: engine, Host: "localhost", Port: 1})
		if err == nil {
			t.Errorf("TestConnection(engine=%q): expected a not-yet-supported error, got nil", engine)
			continue
		}
		if !strings.Contains(err.Error(), "not yet supported") {
			t.Errorf("TestConnection(engine=%q) error = %q, want it to say \"not yet supported\"", engine, err.Error())
		}
	}
}

func TestStringPtrOrNil_EmptyStringBecomesNil(t *testing.T) {
	if got := stringPtrOrNil(""); got != nil {
		t.Errorf("stringPtrOrNil(\"\") = %v, want nil", got)
	}
}

func TestStringPtrOrNil_NonEmptyStringBecomesPointer(t *testing.T) {
	got := stringPtrOrNil("alice")
	if got == nil || *got != "alice" {
		t.Errorf("stringPtrOrNil(\"alice\") = %v, want a pointer to \"alice\"", got)
	}
}

func TestStringOrEmpty_NilBecomesEmptyString(t *testing.T) {
	if got := stringOrEmpty(nil); got != "" {
		t.Errorf("stringOrEmpty(nil) = %q, want \"\"", got)
	}
}

func TestStringOrEmpty_DereferencesNonNilPointer(t *testing.T) {
	value := "bob"
	if got := stringOrEmpty(&value); got != "bob" {
		t.Errorf("stringOrEmpty(&\"bob\") = %q, want \"bob\"", got)
	}
}

func TestParamsToJSON_EmptyMapDefaultsToEmptyObject(t *testing.T) {
	got, err := paramsToJSON(nil)
	if err != nil {
		t.Fatalf("paramsToJSON(nil) failed: %v", err)
	}
	if got != "{}" {
		t.Errorf("paramsToJSON(nil) = %q, want %q", got, "{}")
	}
}

func TestParamsToJSON_EncodesNonEmptyMap(t *testing.T) {
	got, err := paramsToJSON(map[string]string{"sslmode": "require"})
	if err != nil {
		t.Fatalf("paramsToJSON failed: %v", err)
	}
	if got != `{"sslmode":"require"}` {
		t.Errorf("paramsToJSON() = %q, want %q", got, `{"sslmode":"require"}`)
	}
}

func TestParamsFromJSON_EmptyStringDecodesToEmptyMap(t *testing.T) {
	got, err := paramsFromJSON("")
	if err != nil {
		t.Fatalf("paramsFromJSON(\"\") failed: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("paramsFromJSON(\"\") = %v, want an empty map", got)
	}
}

func TestParamsFromJSON_RoundTripsThroughParamsToJSON(t *testing.T) {
	original := map[string]string{"sslmode": "require", "authSource": "admin"}

	encoded, err := paramsToJSON(original)
	if err != nil {
		t.Fatalf("paramsToJSON failed: %v", err)
	}
	decoded, err := paramsFromJSON(encoded)
	if err != nil {
		t.Fatalf("paramsFromJSON failed: %v", err)
	}
	if !reflect.DeepEqual(decoded, original) {
		t.Errorf("round trip = %v, want %v", decoded, original)
	}
}

func TestParamsFromJSON_RejectsMalformedJSON(t *testing.T) {
	if _, err := paramsFromJSON("not-json"); err == nil {
		t.Error("paramsFromJSON(\"not-json\"): expected an error, got nil")
	}
}

func TestConnectionFormFieldsFromStored_RehydratesAllFields(t *testing.T) {
	username := "alice"
	password := "s3cret"
	database := "mydb"
	conn := storage.Connection{
		ID:                7,
		Engine:            storage.EnginePostgres,
		Host:              "db.example.com",
		Port:              5432,
		Username:          &username,
		PasswordEncrypted: &password,
		Database:          &database,
		ParamsJSON:        `{"sslmode":"require"}`,
	}

	got, err := connectionFormFieldsFromStored(conn)
	if err != nil {
		t.Fatalf("connectionFormFieldsFromStored failed: %v", err)
	}

	want := ConnectionFormFields{
		Engine:            storage.EnginePostgres,
		Host:              "db.example.com",
		Port:              5432,
		Username:          "alice",
		Password:          "s3cret",
		Database:          "mydb",
		Params:            map[string]string{"sslmode": "require"},
		SavedConnectionID: 7,
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("connectionFormFieldsFromStored() = %+v, want %+v", got, want)
	}
}

func TestConnectionFormFieldsFromStored_NilFieldsBecomeEmptyStrings(t *testing.T) {
	conn := storage.Connection{ID: 1, Engine: storage.EngineRedis, Host: "localhost", Port: 6379}

	got, err := connectionFormFieldsFromStored(conn)
	if err != nil {
		t.Fatalf("connectionFormFieldsFromStored failed: %v", err)
	}
	if got.Username != "" || got.Password != "" || got.Database != "" {
		t.Errorf("expected nil Connection fields to become empty strings, got %+v", got)
	}
	if len(got.Params) != 0 {
		t.Errorf("expected empty ParamsJSON to become an empty map, got %v", got.Params)
	}
}

func TestConnectionFormFieldsFromStored_SurfacesMalformedParamsJSON(t *testing.T) {
	conn := storage.Connection{ID: 3, Engine: storage.EnginePostgres, Host: "localhost", Port: 5432, ParamsJSON: "not-json"}

	if _, err := connectionFormFieldsFromStored(conn); err == nil {
		t.Error("connectionFormFieldsFromStored() with malformed ParamsJSON: expected an error, got nil")
	}
}

func TestApp_ListConnections_RequiresDB(t *testing.T) {
	a := &App{}

	if _, err := a.ListConnections(); err == nil {
		t.Error("ListConnections() with no db configured: expected an error, got nil")
	}
}

func TestApp_ListConnections_ReturnsEmptySliceNotNilWhenNoneExist(t *testing.T) {
	a := newTestApp(t)

	connections, err := a.ListConnections()
	if err != nil {
		t.Fatalf("ListConnections failed: %v", err)
	}
	if connections == nil {
		t.Fatal("ListConnections() = nil, want a non-nil empty slice (a nil slice JSON-encodes to null, which crashes frontend code doing savedConnections.length)")
	}
	if len(connections) != 0 {
		t.Errorf("expected no connections, got %d", len(connections))
	}
}

func TestApp_ListConnections_ReturnsSavedConnectionsOrderedByName(t *testing.T) {
	a := newTestApp(t)

	for _, name := range []string{"zebra-conn", "apple-conn"} {
		if _, err := a.SaveConnection(ConnectionFormFields{Engine: storage.EnginePostgres, Host: "localhost", Port: 5432}, name); err != nil {
			t.Fatalf("SaveConnection(%q) failed: %v", name, err)
		}
	}

	connections, err := a.ListConnections()
	if err != nil {
		t.Fatalf("ListConnections failed: %v", err)
	}
	if len(connections) != 2 {
		t.Fatalf("expected 2 saved connections, got %d", len(connections))
	}
	if connections[0].Name != "apple-conn" || connections[1].Name != "zebra-conn" {
		t.Errorf("expected connections ordered by name, got %q then %q", connections[0].Name, connections[1].Name)
	}
}

func TestApp_SaveConnection_RequiresDB(t *testing.T) {
	a := &App{}

	if _, err := a.SaveConnection(ConnectionFormFields{Engine: storage.EnginePostgres, Host: "localhost", Port: 5432}, "name"); err == nil {
		t.Error("SaveConnection() with no db configured: expected an error, got nil")
	}
}

func TestApp_SaveConnection_RequiresName(t *testing.T) {
	a := newTestApp(t)

	if _, err := a.SaveConnection(ConnectionFormFields{Engine: storage.EnginePostgres, Host: "localhost", Port: 5432}, "  "); err == nil {
		t.Error("SaveConnection() with a blank name: expected an error, got nil")
	}
}

func TestApp_SaveConnection_RejectsInvalidFields(t *testing.T) {
	a := newTestApp(t)

	if _, err := a.SaveConnection(ConnectionFormFields{Engine: storage.EnginePostgres, Host: "  "}, "bad-fields"); err == nil {
		t.Error("SaveConnection() with a blank host: expected an error, got nil")
	}
}

func TestApp_SaveConnection_PersistsAllFieldsAndDefaultsEmptyParams(t *testing.T) {
	a := newTestApp(t)

	saved, err := a.SaveConnection(ConnectionFormFields{
		Engine:   storage.EnginePostgres,
		Host:     "db.example.com",
		Port:     5432,
		Username: "alice",
		Password: "s3cret",
		Database: "mydb",
		Params:   map[string]string{"sslmode": "require"},
	}, "my-connection")
	if err != nil {
		t.Fatalf("SaveConnection failed: %v", err)
	}

	if saved.Name != "my-connection" {
		t.Errorf("expected Name %q, got %q", "my-connection", saved.Name)
	}
	if saved.Username == nil || *saved.Username != "alice" {
		t.Errorf("expected Username %q, got %v", "alice", saved.Username)
	}
	if saved.ParamsJSON != `{"sslmode":"require"}` {
		t.Errorf("expected ParamsJSON %q, got %q", `{"sslmode":"require"}`, saved.ParamsJSON)
	}

	savedNoParams, err := a.SaveConnection(ConnectionFormFields{Engine: storage.EngineRedis, Host: "localhost", Port: 6379}, "no-params-connection")
	if err != nil {
		t.Fatalf("SaveConnection failed: %v", err)
	}
	if savedNoParams.ParamsJSON != "{}" {
		t.Errorf("expected empty Params to default to %q, got %q", "{}", savedNoParams.ParamsJSON)
	}
	if savedNoParams.Username != nil || savedNoParams.PasswordEncrypted != nil || savedNoParams.Database != nil {
		t.Errorf("expected empty Username/Password/Database to be stored as nil, got Username=%v PasswordEncrypted=%v Database=%v",
			savedNoParams.Username, savedNoParams.PasswordEncrypted, savedNoParams.Database)
	}
}

func TestApp_DeleteConnection_RequiresDB(t *testing.T) {
	a := &App{}

	if err := a.DeleteConnection(1); err == nil {
		t.Error("DeleteConnection() with no db configured: expected an error, got nil")
	}
}

func TestApp_DeleteConnection_RemovesSavedConnection(t *testing.T) {
	a := newTestApp(t)

	saved, err := a.SaveConnection(ConnectionFormFields{Engine: storage.EnginePostgres, Host: "localhost", Port: 5432}, "to-delete")
	if err != nil {
		t.Fatalf("SaveConnection failed: %v", err)
	}

	if err := a.DeleteConnection(saved.ID); err != nil {
		t.Fatalf("DeleteConnection failed: %v", err)
	}

	connections, err := a.ListConnections()
	if err != nil {
		t.Fatalf("ListConnections failed: %v", err)
	}
	if len(connections) != 0 {
		t.Errorf("expected the deleted connection to be gone, got %d remaining", len(connections))
	}
}

func TestApp_ConnectUsingSavedConnection_RequiresDB(t *testing.T) {
	a := &App{}

	if _, err := a.ConnectUsingSavedConnection(1); err == nil {
		t.Error("ConnectUsingSavedConnection() with no db configured: expected an error, got nil")
	}
}

func TestApp_ConnectUsingSavedConnection_ReturnsFormFieldsAndBumpsLastUsedAt(t *testing.T) {
	a := newTestApp(t)

	saved, err := a.SaveConnection(ConnectionFormFields{
		Engine:   storage.EngineMySQL,
		Host:     "db.example.com",
		Port:     3306,
		Username: "root",
		Password: "mysql",
		Database: "appdb",
		Params:   map[string]string{"parseTime": "true"},
	}, "reload-me")
	if err != nil {
		t.Fatalf("SaveConnection failed: %v", err)
	}
	if saved.LastUsedAt != nil {
		t.Fatal("expected a freshly saved connection to have a nil LastUsedAt")
	}

	fields, err := a.ConnectUsingSavedConnection(saved.ID)
	if err != nil {
		t.Fatalf("ConnectUsingSavedConnection failed: %v", err)
	}

	want := ConnectionFormFields{
		Engine:            storage.EngineMySQL,
		Host:              "db.example.com",
		Port:              3306,
		Username:          "root",
		Password:          "mysql",
		Database:          "appdb",
		Params:            map[string]string{"parseTime": "true"},
		SavedConnectionID: saved.ID,
	}
	if !reflect.DeepEqual(*fields, want) {
		t.Errorf("ConnectUsingSavedConnection() = %+v, want %+v", *fields, want)
	}

	connections, err := a.ListConnections()
	if err != nil {
		t.Fatalf("ListConnections failed: %v", err)
	}
	if len(connections) != 1 || connections[0].LastUsedAt == nil {
		t.Fatalf("expected ConnectUsingSavedConnection to have set LastUsedAt, got %+v", connections)
	}
}

func TestApp_ConnectUsingSavedConnection_NotFound(t *testing.T) {
	a := newTestApp(t)

	if _, err := a.ConnectUsingSavedConnection(999999); err == nil {
		t.Error("ConnectUsingSavedConnection() on a non-existent ID: expected an error, got nil")
	}
}

func TestApp_ListSchemasForSession_UnknownSessionReturnsError(t *testing.T) {
	a := &App{ctx: context.Background()}

	if _, err := a.ListSchemasForSession("does-not-exist"); err == nil {
		t.Error("ListSchemasForSession() with an unknown session ID: expected an error, got nil")
	}
}

func TestApp_ListSchemasForSession_ReturnsSchemasFromTheLiveEngine(t *testing.T) {
	a := &App{ctx: context.Background()}
	fake := &fakeSchemaEngine{schemas: []string{"public", "reporting"}}
	a.putQuerySession("sess-1", &querySession{engine: fake})

	got, err := a.ListSchemasForSession("sess-1")
	if err != nil {
		t.Fatalf("ListSchemasForSession failed: %v", err)
	}
	if !reflect.DeepEqual(got, []string{"public", "reporting"}) {
		t.Errorf("ListSchemasForSession() = %v, want %v", got, []string{"public", "reporting"})
	}
}

func TestApp_ListSchemasForSession_NormalizesNilResultToEmptySlice(t *testing.T) {
	a := &App{ctx: context.Background()}
	fake := &fakeSchemaEngine{schemas: nil}
	a.putQuerySession("sess-1", &querySession{engine: fake})

	got, err := a.ListSchemasForSession("sess-1")
	if err != nil {
		t.Fatalf("ListSchemasForSession failed: %v", err)
	}
	if got == nil {
		t.Fatal("ListSchemasForSession() = nil, want a non-nil empty slice (a nil slice JSON-encodes to null)")
	}
	if len(got) != 0 {
		t.Errorf("ListSchemasForSession() = %v, want empty", got)
	}
}

func TestApp_ListSchemasForSession_SurfacesEngineError(t *testing.T) {
	a := &App{ctx: context.Background()}
	fake := &fakeSchemaEngine{schemasErr: fmt.Errorf("boom")}
	a.putQuerySession("sess-1", &querySession{engine: fake})

	if _, err := a.ListSchemasForSession("sess-1"); err == nil {
		t.Error("ListSchemasForSession() with an engine error: expected an error, got nil")
	}
}

func TestApp_ListTablesForSession_UnknownSessionReturnsError(t *testing.T) {
	a := &App{ctx: context.Background()}

	if _, err := a.ListTablesForSession("does-not-exist", "public"); err == nil {
		t.Error("ListTablesForSession() with an unknown session ID: expected an error, got nil")
	}
}

func TestApp_ListTablesForSession_ReturnsTablesFromTheLiveEngine(t *testing.T) {
	a := &App{ctx: context.Background()}
	want := []dbengine.TableInfo{
		{Name: "users", Columns: []dbengine.ColumnInfo{{Name: "id", DataType: "integer", IsPrimaryKey: true}}},
	}
	fake := &fakeSchemaEngine{tables: want}
	a.putQuerySession("sess-1", &querySession{engine: fake})

	got, err := a.ListTablesForSession("sess-1", "public")
	if err != nil {
		t.Fatalf("ListTablesForSession failed: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ListTablesForSession() = %+v, want %+v", got, want)
	}
}

func TestApp_ListTablesForSession_NormalizesNilResultToEmptySlice(t *testing.T) {
	a := &App{ctx: context.Background()}
	fake := &fakeSchemaEngine{tables: nil}
	a.putQuerySession("sess-1", &querySession{engine: fake})

	got, err := a.ListTablesForSession("sess-1", "public")
	if err != nil {
		t.Fatalf("ListTablesForSession failed: %v", err)
	}
	if got == nil {
		t.Fatal("ListTablesForSession() = nil, want a non-nil empty slice (a nil slice JSON-encodes to null)")
	}
	if len(got) != 0 {
		t.Errorf("ListTablesForSession() = %+v, want empty", got)
	}
}

func TestApp_ListTablesForSession_SurfacesEngineError(t *testing.T) {
	a := &App{ctx: context.Background()}
	fake := &fakeSchemaEngine{tablesErr: fmt.Errorf("boom")}
	a.putQuerySession("sess-1", &querySession{engine: fake})

	if _, err := a.ListTablesForSession("sess-1", "public"); err == nil {
		t.Error("ListTablesForSession() with an engine error: expected an error, got nil")
	}
}

func TestApp_ListForeignKeysForSession_UnknownSessionReturnsError(t *testing.T) {
	a := &App{ctx: context.Background()}

	if _, err := a.ListForeignKeysForSession("does-not-exist", "public"); err == nil {
		t.Error("ListForeignKeysForSession() with an unknown session ID: expected an error, got nil")
	}
}

func TestApp_ListForeignKeysForSession_ReturnsForeignKeysFromTheLiveEngine(t *testing.T) {
	a := &App{ctx: context.Background()}
	want := []dbengine.ForeignKey{
		{TableName: "books", ColumnName: "author_id", ReferencedTable: "authors", ReferencedColumn: "id"},
	}
	fake := &fakeSchemaEngine{foreignKeys: want}
	a.putQuerySession("sess-1", &querySession{engine: fake})

	got, err := a.ListForeignKeysForSession("sess-1", "public")
	if err != nil {
		t.Fatalf("ListForeignKeysForSession failed: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ListForeignKeysForSession() = %+v, want %+v", got, want)
	}
}

func TestApp_ListForeignKeysForSession_NormalizesNilResultToEmptySlice(t *testing.T) {
	a := &App{ctx: context.Background()}
	fake := &fakeSchemaEngine{foreignKeys: nil}
	a.putQuerySession("sess-1", &querySession{engine: fake})

	got, err := a.ListForeignKeysForSession("sess-1", "public")
	if err != nil {
		t.Fatalf("ListForeignKeysForSession failed: %v", err)
	}
	if got == nil {
		t.Fatal("ListForeignKeysForSession() = nil, want a non-nil empty slice (a nil slice JSON-encodes to null)")
	}
	if len(got) != 0 {
		t.Errorf("ListForeignKeysForSession() = %+v, want empty", got)
	}
}

func TestApp_ListForeignKeysForSession_SurfacesEngineError(t *testing.T) {
	a := &App{ctx: context.Background()}
	fake := &fakeSchemaEngine{foreignKeysErr: fmt.Errorf("boom")}
	a.putQuerySession("sess-1", &querySession{engine: fake})

	if _, err := a.ListForeignKeysForSession("sess-1", "public"); err == nil {
		t.Error("ListForeignKeysForSession() with an engine error: expected an error, got nil")
	}
}

func TestApp_BuildSchemaDiagram_UnknownSessionReturnsError(t *testing.T) {
	a := &App{ctx: context.Background()}

	if _, err := a.BuildSchemaDiagram("does-not-exist", "public"); err == nil {
		t.Error("BuildSchemaDiagram() with an unknown session ID: expected an error, got nil")
	}
}

func TestApp_BuildSchemaDiagram_RendersMermaidFromTablesAndForeignKeys(t *testing.T) {
	a := &App{ctx: context.Background()}
	fake := &fakeSchemaEngine{
		tables: []dbengine.TableInfo{
			{Name: "authors", Columns: []dbengine.ColumnInfo{{Name: "id", DataType: "int4", IsPrimaryKey: true}}},
			{Name: "books", Columns: []dbengine.ColumnInfo{
				{Name: "id", DataType: "int4", IsPrimaryKey: true},
				{Name: "author_id", DataType: "int4"},
			}},
		},
		foreignKeys: []dbengine.ForeignKey{
			{TableName: "books", ColumnName: "author_id", ReferencedTable: "authors", ReferencedColumn: "id"},
		},
	}
	a.putQuerySession("sess-1", &querySession{engine: fake})

	got, err := a.BuildSchemaDiagram("sess-1", "public")
	if err != nil {
		t.Fatalf("BuildSchemaDiagram failed: %v", err)
	}

	want := "erDiagram\n" +
		"    authors {\n" +
		"        int4 id PK\n" +
		"    }\n" +
		"    books {\n" +
		"        int4 id PK\n" +
		"        int4 author_id FK\n" +
		"    }\n" +
		"    authors ||--o{ books : \"via author_id\"\n"
	if got != want {
		t.Errorf("BuildSchemaDiagram() =\n%s\nwant:\n%s", got, want)
	}
}

func TestApp_BuildSchemaDiagram_SurfacesTablesEngineError(t *testing.T) {
	a := &App{ctx: context.Background()}
	fake := &fakeSchemaEngine{tablesErr: fmt.Errorf("boom")}
	a.putQuerySession("sess-1", &querySession{engine: fake})

	if _, err := a.BuildSchemaDiagram("sess-1", "public"); err == nil {
		t.Error("BuildSchemaDiagram() with a ListTables engine error: expected an error, got nil")
	}
}

func TestApp_BuildSchemaDiagram_SurfacesForeignKeysEngineError(t *testing.T) {
	a := &App{ctx: context.Background()}
	fake := &fakeSchemaEngine{foreignKeysErr: fmt.Errorf("boom")}
	a.putQuerySession("sess-1", &querySession{engine: fake})

	if _, err := a.BuildSchemaDiagram("sess-1", "public"); err == nil {
		t.Error("BuildSchemaDiagram() with a ListForeignKeys engine error: expected an error, got nil")
	}
}
