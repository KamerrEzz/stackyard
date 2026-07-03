package main

import (
	"context"
	"net"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"stackyard/internal/docker"
	"stackyard/internal/storage"
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
