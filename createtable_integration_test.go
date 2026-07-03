//go:build integration

// Integration test for CreateTable (tasks.md 10.2): exercises the bound
// method against a real Postgres AND a real MySQL container started
// through internal/docker's own Start<Engine>Environment (no mocks),
// proving a table created through this path genuinely exists afterward —
// confirmed via ListTables and a direct SELECT — with the right columns,
// types, nullability, and primary key, and that an auto-increment primary
// key column actually auto-increments on insert.
//
// Requires Docker Desktop/dockerd running; run with:
//
//	go test -tags=integration ./...
//
// Uses test/profile/service IDs 999031 (Postgres) and 999032 (MySQL) —
// 999001-999030 are already taken across this repo's other integration
// tests (grepped for every 9990\d\d literal in the repo before picking
// these, per docs/STATE.md's running convention) — and host ports 15544
// (Postgres) and 13322 (MySQL), independently grepped for every existing
// `HostPort\s*=\s*[0-9]{4,5}` literal in the repo to confirm both are free,
// per the same doc's separate port-collision lesson (Session 14: test IDs
// and host ports are independent numbering conventions that must both be
// checked). Everything created is torn down in t.Cleanup so each test is
// fully self-cleaning and safely re-runnable.
package main

import (
	"context"
	"strings"
	"testing"
	"time"

	"stackyard/internal/dbengine"
	"stackyard/internal/docker"
	"stackyard/internal/storage"
)

const (
	createTableIntegrationPostgresProfileID int64 = 999031
	createTableIntegrationPostgresServiceID int64 = 999031
	createTableIntegrationPostgresHostPort        = 15544

	createTableIntegrationMySQLProfileID int64 = 999032
	createTableIntegrationMySQLServiceID int64 = 999032
	createTableIntegrationMySQLHostPort        = 13322
)

func TestIntegration_App_CreateTable_Postgres(t *testing.T) {
	dockerClient, err := docker.NewClient()
	if err != nil {
		t.Fatalf("docker.NewClient() failed: %v", err)
	}
	defer dockerClient.Close()

	setupCtx, setupCancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer setupCancel()
	if err := dockerClient.Ping(setupCtx); err != nil {
		t.Fatalf("Ping() failed to reach the local Docker Engine: %v", err)
	}

	username := "stackyard_test"
	password := "stackyard_test_pw"
	dbName := "stackyard_test_db"

	svc := storage.Service{
		ID:                createTableIntegrationPostgresServiceID,
		ProfileID:         createTableIntegrationPostgresProfileID,
		Engine:            storage.EnginePostgres,
		ImageTag:          "postgres:16-alpine",
		HostPort:          createTableIntegrationPostgresHostPort,
		Username:          &username,
		PasswordEncrypted: &password,
		DBName:            &dbName,
		VolumeName:        "stackyard-test-vol-app-createtable-postgres",
	}

	networkName := docker.ProfileNetworkName(svc.ProfileID)
	containerName := docker.ServiceContainerName(svc.ID)
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()
		if err := dockerClient.RemoveContainer(cleanupCtx, containerName); err != nil {
			t.Logf("cleanup: failed to remove container %s: %v", containerName, err)
		}
		if err := dockerClient.RemoveVolume(cleanupCtx, svc.VolumeName); err != nil {
			t.Logf("cleanup: failed to remove volume %s: %v", svc.VolumeName, err)
		}
		if err := dockerClient.RemoveNetwork(cleanupCtx, networkName); err != nil {
			t.Logf("cleanup: failed to remove network %s: %v", networkName, err)
		}
	})

	if err := dockerClient.StartPostgresEnvironment(setupCtx, svc); err != nil {
		t.Fatalf("StartPostgresEnvironment() failed: %v", err)
	}
	t.Logf("StartPostgresEnvironment: network %q, volume %q, container %q created/started", networkName, svc.VolumeName, containerName)

	a := &App{ctx: context.Background()}
	fields := ConnectionFormFields{
		Engine:   storage.EnginePostgres,
		Host:     "localhost",
		Port:     createTableIntegrationPostgresHostPort,
		Username: username,
		Password: password,
		Database: dbName,
	}
	sessionID, err := waitForOpenConnection(t, a, fields, 90*time.Second)
	if err != nil {
		t.Fatalf("OpenConnection() never succeeded against the live Postgres container: %v", err)
	}

	defaultStatus := "'active'"
	columns := []dbengine.ColumnDefinition{
		{Name: "id", Type: dbengine.ColumnTypeSerial, IsPrimaryKey: true},
		{Name: "name", Type: dbengine.ColumnTypeText, Nullable: false},
		{Name: "notes", Type: dbengine.ColumnTypeText, Nullable: true},
		{Name: "status", Type: dbengine.ColumnTypeVarchar, Nullable: false, Default: &defaultStatus},
	}

	if err := a.CreateTable(sessionID, "public", "widgets", columns); err != nil {
		t.Fatalf("CreateTable() failed: %v", err)
	}

	tables, err := a.ListTablesForSession(sessionID, "public")
	if err != nil {
		t.Fatalf("ListTablesForSession() failed: %v", err)
	}
	table := findTable(tables, "widgets")
	if table == nil {
		t.Fatalf("ListTablesForSession() did not report the new 'widgets' table: %v", tables)
	}

	assertColumn(t, table.Columns, "id", true, false)
	assertColumn(t, table.Columns, "name", false, false)
	assertColumn(t, table.Columns, "notes", false, true)
	assertColumn(t, table.Columns, "status", false, false)

	inserted, err := a.InsertTableRow(sessionID, "public", "widgets", map[string]any{"name": "bolt"})
	if err != nil {
		t.Fatalf("InsertTableRow() into the newly created table failed: %v", err)
	}
	if inserted["id"] == nil {
		t.Errorf("InsertTableRow() row = %v, want an auto-generated id from the SERIAL column", inserted)
	}
	if inserted["status"] != "active" {
		t.Errorf("InsertTableRow() row = %v, want status defaulted to 'active'", inserted)
	}

	secondInserted, err := a.InsertTableRow(sessionID, "public", "widgets", map[string]any{"name": "nut"})
	if err != nil {
		t.Fatalf("InsertTableRow() second row failed: %v", err)
	}
	if toInt64(secondInserted["id"]) <= toInt64(inserted["id"]) {
		t.Errorf("second inserted id = %v, want it greater than the first inserted id %v (auto-increment)", secondInserted["id"], inserted["id"])
	}
}

func TestIntegration_App_CreateTable_MySQL(t *testing.T) {
	dockerClient, err := docker.NewClient()
	if err != nil {
		t.Fatalf("docker.NewClient() failed: %v", err)
	}
	defer dockerClient.Close()

	setupCtx, setupCancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer setupCancel()
	if err := dockerClient.Ping(setupCtx); err != nil {
		t.Fatalf("Ping() failed to reach the local Docker Engine: %v", err)
	}

	username := "stackyard_test"
	password := "stackyard_test_pw"
	dbName := "stackyard_test_db"

	svc := storage.Service{
		ID:                createTableIntegrationMySQLServiceID,
		ProfileID:         createTableIntegrationMySQLProfileID,
		Engine:            storage.EngineMySQL,
		ImageTag:          "mysql:8",
		HostPort:          createTableIntegrationMySQLHostPort,
		Username:          &username,
		PasswordEncrypted: &password,
		DBName:            &dbName,
		VolumeName:        "stackyard-test-vol-app-createtable-mysql",
	}

	networkName := docker.ProfileNetworkName(svc.ProfileID)
	containerName := docker.ServiceContainerName(svc.ID)
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()
		if err := dockerClient.RemoveContainer(cleanupCtx, containerName); err != nil {
			t.Logf("cleanup: failed to remove container %s: %v", containerName, err)
		}
		if err := dockerClient.RemoveVolume(cleanupCtx, svc.VolumeName); err != nil {
			t.Logf("cleanup: failed to remove volume %s: %v", svc.VolumeName, err)
		}
		if err := dockerClient.RemoveNetwork(cleanupCtx, networkName); err != nil {
			t.Logf("cleanup: failed to remove network %s: %v", networkName, err)
		}
	})

	if err := dockerClient.StartMySQLEnvironment(setupCtx, svc); err != nil {
		t.Fatalf("StartMySQLEnvironment() failed: %v", err)
	}
	t.Logf("StartMySQLEnvironment: network %q, volume %q, container %q created/started", networkName, svc.VolumeName, containerName)

	a := &App{ctx: context.Background()}
	fields := ConnectionFormFields{
		Engine:   storage.EngineMySQL,
		Host:     "localhost",
		Port:     createTableIntegrationMySQLHostPort,
		Username: username,
		Password: password,
		Database: dbName,
	}
	sessionID, err := waitForOpenConnection(t, a, fields, 90*time.Second)
	if err != nil {
		t.Fatalf("OpenConnection() never succeeded against the live MySQL container: %v", err)
	}

	defaultStatus := "'active'"
	columns := []dbengine.ColumnDefinition{
		{Name: "id", Type: dbengine.ColumnTypeBigSerial, IsPrimaryKey: true},
		{Name: "name", Type: dbengine.ColumnTypeVarchar, Nullable: false},
		{Name: "notes", Type: dbengine.ColumnTypeText, Nullable: true},
		{Name: "status", Type: dbengine.ColumnTypeVarchar, Nullable: false, Default: &defaultStatus},
	}

	if err := a.CreateTable(sessionID, dbName, "widgets", columns); err != nil {
		t.Fatalf("CreateTable() failed: %v", err)
	}

	tables, err := a.ListTablesForSession(sessionID, dbName)
	if err != nil {
		t.Fatalf("ListTablesForSession() failed: %v", err)
	}
	table := findTable(tables, "widgets")
	if table == nil {
		t.Fatalf("ListTablesForSession() did not report the new 'widgets' table: %v", tables)
	}

	assertColumn(t, table.Columns, "id", true, false)
	assertColumn(t, table.Columns, "name", false, false)
	assertColumn(t, table.Columns, "notes", false, true)
	assertColumn(t, table.Columns, "status", false, false)

	inserted, err := a.InsertTableRow(sessionID, dbName, "widgets", map[string]any{"name": "bolt"})
	if err != nil {
		t.Fatalf("InsertTableRow() into the newly created table failed: %v", err)
	}
	if inserted["id"] == nil {
		t.Errorf("InsertTableRow() row = %v, want an auto-generated id from the BIGSERIAL/AUTO_INCREMENT column", inserted)
	}
	if inserted["status"] != "active" {
		t.Errorf("InsertTableRow() row = %v, want status defaulted to 'active'", inserted)
	}

	secondInserted, err := a.InsertTableRow(sessionID, dbName, "widgets", map[string]any{"name": "nut"})
	if err != nil {
		t.Fatalf("InsertTableRow() second row failed: %v", err)
	}
	if toInt64(secondInserted["id"]) <= toInt64(inserted["id"]) {
		t.Errorf("second inserted id = %v, want it greater than the first inserted id %v (auto-increment)", secondInserted["id"], inserted["id"])
	}
}

func TestIntegration_App_CreateTable_RejectsUnknownSession(t *testing.T) {
	a := &App{ctx: context.Background()}
	err := a.CreateTable("no-such-session", "public", "widgets", []dbengine.ColumnDefinition{
		{Name: "id", Type: dbengine.ColumnTypeInteger},
	})
	if err == nil {
		t.Fatal("CreateTable() with no open session expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "no open connection session") {
		t.Errorf("CreateTable() error = %q, want it to name the missing session", err.Error())
	}
}

func findTable(tables []dbengine.TableInfo, name string) *dbengine.TableInfo {
	for i := range tables {
		if tables[i].Name == name {
			return &tables[i]
		}
	}
	return nil
}

func assertColumn(t *testing.T, columns []dbengine.ColumnInfo, name string, wantPK, wantNullable bool) {
	t.Helper()
	for _, col := range columns {
		if col.Name != name {
			continue
		}
		if col.IsPrimaryKey != wantPK {
			t.Errorf("column %q IsPrimaryKey = %v, want %v", name, col.IsPrimaryKey, wantPK)
		}
		if col.Nullable != wantNullable {
			t.Errorf("column %q Nullable = %v, want %v", name, col.Nullable, wantNullable)
		}
		return
	}
	t.Errorf("column %q not found among %v", name, columns)
}

// toInt64 is grid_integration_test.go's own helper, in the same package
// under the same integration build tag — reused here as-is rather than
// redefined, since Go disallows two same-named functions in one package.
