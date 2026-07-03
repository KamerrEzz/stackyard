//go:build integration

// Integration test for postgres.go: exercises Engine against a real
// Postgres container started through internal/docker's own
// StartPostgresEnvironment (no bespoke container-launch code, no mocks).
// Requires Docker Desktop/dockerd running; run with:
//
//	go test -tags=integration ./internal/dbengine/...
//
// Uses test/profile/service ID 999010 (999001-999009 are already taken
// across internal/docker's and the repo-root's integration tests — grepped
// for every 9990\d\d literal in the repo before picking this one, per
// docs/STATE.md's running convention) and host port 15534, distinct from
// every other integration test's port in this repo. Everything created is
// torn down in t.Cleanup so the test is fully self-cleaning and safely
// re-runnable.
package postgres

import (
	"context"
	"testing"
	"time"

	"stackyard/internal/dbengine"
	"stackyard/internal/docker"
	"stackyard/internal/storage"
)

const (
	pgIntegrationTestProfileID int64 = 999010
	pgIntegrationTestServiceID int64 = 999010
	pgIntegrationTestHostPort        = 15534
)

func TestIntegration_PostgresEngine(t *testing.T) {
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
		ID:                pgIntegrationTestServiceID,
		ProfileID:         pgIntegrationTestProfileID,
		Engine:            storage.EnginePostgres,
		ImageTag:          "postgres:16-alpine",
		HostPort:          pgIntegrationTestHostPort,
		Username:          &username,
		PasswordEncrypted: &password,
		DBName:            &dbName,
		VolumeName:        "stackyard-test-vol-dbengine-postgres",
	}

	networkName := docker.ProfileNetworkName(svc.ProfileID)
	containerName := docker.ServiceContainerName(svc.ID)

	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()

		if err := dockerClient.RemoveContainer(cleanupCtx, containerName); err != nil {
			t.Logf("cleanup: failed to remove container %s: %v", containerName, err)
		} else {
			t.Logf("cleanup: removed container %s", containerName)
		}
		if err := dockerClient.RemoveVolume(cleanupCtx, svc.VolumeName); err != nil {
			t.Logf("cleanup: failed to remove volume %s: %v", svc.VolumeName, err)
		} else {
			t.Logf("cleanup: removed volume %s", svc.VolumeName)
		}
		if err := dockerClient.RemoveNetwork(cleanupCtx, networkName); err != nil {
			t.Logf("cleanup: failed to remove network %s: %v", networkName, err)
		} else {
			t.Logf("cleanup: removed network %s", networkName)
		}
	})

	if err := dockerClient.StartPostgresEnvironment(setupCtx, svc); err != nil {
		t.Fatalf("StartPostgresEnvironment() failed: %v", err)
	}
	t.Logf("StartPostgresEnvironment: network %q, volume %q, container %q created/started", networkName, svc.VolumeName, containerName)

	connString := docker.PostgresConnectionString(svc)
	engine := New(connString)

	if err := waitForConnect(t, engine, 90*time.Second); err != nil {
		t.Fatalf("Engine failed to become reachable within timeout: %v", err)
	}
	defer engine.Close()

	ctx := context.Background()

	if err := engine.Ping(ctx); err != nil {
		t.Fatalf("Ping() failed: %v", err)
	}
	t.Log("Ping() succeeded against the live container")

	if _, err := engine.Query(ctx, `CREATE TABLE widgets (id SERIAL PRIMARY KEY, name TEXT NOT NULL, weight INT)`); err != nil {
		t.Fatalf("CREATE TABLE failed: %v", err)
	}
	t.Log("CREATE TABLE widgets succeeded")

	insertResult, err := engine.Query(ctx, `INSERT INTO widgets (name, weight) VALUES ('bolt', 5), ('nut', 2)`)
	if err != nil {
		t.Fatalf("INSERT failed: %v", err)
	}
	if insertResult.RowsAffected != 2 {
		t.Errorf("INSERT RowsAffected = %d, want 2", insertResult.RowsAffected)
	}
	if len(insertResult.Columns) != 0 || len(insertResult.Rows) != 0 {
		t.Errorf("INSERT result should have no Columns/Rows, got Columns=%v Rows=%v", insertResult.Columns, insertResult.Rows)
	}
	t.Logf("INSERT succeeded: RowsAffected=%d", insertResult.RowsAffected)

	selectResult, err := engine.Query(ctx, `SELECT id, name, weight FROM widgets ORDER BY id`)
	if err != nil {
		t.Fatalf("SELECT failed: %v", err)
	}
	wantColumns := []string{"id", "name", "weight"}
	if len(selectResult.Columns) != len(wantColumns) {
		t.Fatalf("SELECT Columns = %+v, want names %v", selectResult.Columns, wantColumns)
	}
	for i, want := range wantColumns {
		if selectResult.Columns[i].Name != want {
			t.Errorf("SELECT Columns[%d].Name = %q, want %q", i, selectResult.Columns[i].Name, want)
		}
	}
	assertPostgresSelectResultColumns(t, selectResult.Columns)
	if len(selectResult.Rows) != 2 {
		t.Fatalf("SELECT returned %d rows, want 2", len(selectResult.Rows))
	}
	if selectResult.Rows[0][1] != "bolt" || selectResult.Rows[1][1] != "nut" {
		t.Errorf("SELECT Rows = %v, want name column to read bolt then nut", selectResult.Rows)
	}
	t.Logf("SELECT round trip succeeded: %v", selectResult.Rows)

	schemas, err := engine.ListSchemas(ctx)
	if err != nil {
		t.Fatalf("ListSchemas() failed: %v", err)
	}
	if !containsString(schemas, "public") {
		t.Errorf("ListSchemas() = %v, want it to include \"public\"", schemas)
	}
	for _, systemSchema := range []string{"pg_catalog", "information_schema"} {
		if containsString(schemas, systemSchema) {
			t.Errorf("ListSchemas() = %v, expected system schema %q to be excluded", schemas, systemSchema)
		}
	}
	t.Logf("ListSchemas() succeeded: %v", schemas)

	tables, err := engine.ListTables(ctx, "public")
	if err != nil {
		t.Fatalf("ListTables() failed: %v", err)
	}
	widgetsTable := findTable(tables, "widgets")
	if widgetsTable == nil {
		t.Fatalf("ListTables(\"public\") = %v, want it to include \"widgets\"", tables)
	}
	assertPostgresWidgetsColumns(t, widgetsTable.Columns)
	t.Logf("ListTables() succeeded: %+v", widgetsTable)

	cancelCtx, cancel := context.WithTimeout(ctx, 1*time.Second)
	defer cancel()
	cancelStart := time.Now()
	_, err = engine.Query(cancelCtx, `SELECT pg_sleep(30)`)
	cancelDuration := time.Since(cancelStart)
	if err == nil {
		t.Error("expected pg_sleep(30) under a 1s context timeout to fail, got nil error")
	}
	if cancelDuration > 10*time.Second {
		t.Errorf("cancelled query took %v to return, want it to abort promptly near the 1s timeout", cancelDuration)
	}
	t.Logf("context-cancelled query returned after %v with error: %v", cancelDuration, err)

	if err := engine.Close(); err != nil {
		t.Errorf("Close() failed: %v", err)
	}
	if err := engine.Ping(context.Background()); err == nil {
		t.Error("Ping() after Close() should fail")
	}
	t.Log("Close() succeeded; Ping() after Close() correctly fails")
}

func waitForConnect(t *testing.T, engine *Engine, timeout time.Duration) error {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		connectCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		err := engine.Connect(connectCtx)
		cancel()
		if err == nil {
			return nil
		}
		lastErr = err
		time.Sleep(1 * time.Second)
	}
	return lastErr
}

func containsString(values []string, target string) bool {
	for _, v := range values {
		if v == target {
			return true
		}
	}
	return false
}

func findTable(tables []dbengine.TableInfo, name string) *dbengine.TableInfo {
	for i := range tables {
		if tables[i].Name == name {
			return &tables[i]
		}
	}
	return nil
}

// assertPostgresSelectResultColumns asserts the QueryResult-level metadata
// (tasks.md 3.7) for the id/name/weight SELECT above: DatabaseType is a
// non-empty, sane type name for the integer id column and the text name
// column, and Nullable is nil for every column — documenting Postgres's
// known limitation that pgx's FieldDescription never reports nullability
// (see dbengine.ResultColumn's doc comment).
func assertPostgresSelectResultColumns(t *testing.T, columns []dbengine.ResultColumn) {
	t.Helper()
	byName := make(map[string]dbengine.ResultColumn, len(columns))
	for _, col := range columns {
		byName[col.Name] = col
		if col.Nullable != nil {
			t.Errorf("SELECT ResultColumn %q.Nullable = %v, want nil (Postgres never reports nullability)", col.Name, *col.Nullable)
		}
	}

	id, ok := byName["id"]
	if !ok || id.DatabaseType == "" {
		t.Errorf("SELECT ResultColumn %q.DatabaseType = %q, want a non-empty integer type name", "id", id.DatabaseType)
	} else if id.DatabaseType != "int4" {
		t.Errorf("SELECT ResultColumn %q.DatabaseType = %q, want %q", "id", id.DatabaseType, "int4")
	}

	name, ok := byName["name"]
	if !ok || name.DatabaseType == "" {
		t.Errorf("SELECT ResultColumn %q.DatabaseType = %q, want a non-empty text type name", "name", name.DatabaseType)
	} else if name.DatabaseType != "text" {
		t.Errorf("SELECT ResultColumn %q.DatabaseType = %q, want %q", "name", name.DatabaseType, "text")
	}
}

func assertPostgresWidgetsColumns(t *testing.T, columns []dbengine.ColumnInfo) {
	t.Helper()
	byName := make(map[string]dbengine.ColumnInfo, len(columns))
	for _, col := range columns {
		byName[col.Name] = col
	}

	id, ok := byName["id"]
	if !ok {
		t.Fatal("widgets.id column missing from ListTables result")
	}
	if !id.IsPrimaryKey {
		t.Error("widgets.id should be reported as the primary key")
	}

	name, ok := byName["name"]
	if !ok {
		t.Fatal("widgets.name column missing from ListTables result")
	}
	if name.IsPrimaryKey {
		t.Error("widgets.name should not be reported as a primary key")
	}
	if name.Nullable {
		t.Error("widgets.name is NOT NULL and should be reported as non-nullable")
	}

	weight, ok := byName["weight"]
	if !ok {
		t.Fatal("widgets.weight column missing from ListTables result")
	}
	if weight.IsPrimaryKey {
		t.Error("widgets.weight should not be reported as a primary key")
	}
	if !weight.Nullable {
		t.Error("widgets.weight has no NOT NULL constraint and should be reported as nullable")
	}
}
