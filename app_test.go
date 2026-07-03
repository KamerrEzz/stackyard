package main

import (
	"context"
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
