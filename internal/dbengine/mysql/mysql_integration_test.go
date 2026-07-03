//go:build integration

// Integration test for mysql.go: exercises Engine against a real MySQL
// container started through internal/docker's own StartMySQLEnvironment (no
// bespoke container-launch code, no mocks). Requires Docker Desktop/dockerd
// running; run with:
//
//	go test -tags=integration ./internal/dbengine/...
//
// Uses test/profile/service ID 999011 (999001-999010 are already taken
// across internal/docker's, the repo-root's, and dbengine/postgres's
// integration tests — grepped for every 9990\d\d literal in the repo before
// picking this one, per docs/STATE.md's running convention) and host port
// 13307, distinct from every other integration test's port in this repo.
// Everything created is torn down in t.Cleanup so the test is fully
// self-cleaning and safely re-runnable.
//
// TestIntegration_MySQLEngine_ForeignKeys (tasks.md 4.5.1) reuses this same
// pattern under a second test/profile/service ID (999018, host port 13310)
// rather than sharing 999011's container/tables, keeping the two tests
// independently runnable and cleanable.
package mysql

import (
	"context"
	"fmt"
	"testing"
	"time"

	"stackyard/internal/dbengine"
	"stackyard/internal/docker"
	"stackyard/internal/storage"
)

const (
	mysqlIntegrationTestProfileID int64 = 999011
	mysqlIntegrationTestServiceID int64 = 999011
	mysqlIntegrationTestHostPort        = 13307
)

func TestIntegration_MySQLEngine(t *testing.T) {
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
		ID:                mysqlIntegrationTestServiceID,
		ProfileID:         mysqlIntegrationTestProfileID,
		Engine:            storage.EngineMySQL,
		ImageTag:          "mysql:8",
		HostPort:          mysqlIntegrationTestHostPort,
		Username:          &username,
		PasswordEncrypted: &password,
		DBName:            &dbName,
		VolumeName:        "stackyard-test-vol-dbengine-mysql",
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

	if err := dockerClient.StartMySQLEnvironment(setupCtx, svc); err != nil {
		t.Fatalf("StartMySQLEnvironment() failed: %v", err)
	}
	t.Logf("StartMySQLEnvironment: network %q, volume %q, container %q created/started", networkName, svc.VolumeName, containerName)

	dsn := fmt.Sprintf("%s:%s@tcp(127.0.0.1:%d)/%s?parseTime=true", username, password, svc.HostPort, dbName)
	engine := New(dsn)

	if err := waitForConnect(t, engine, 120*time.Second); err != nil {
		t.Fatalf("Engine failed to become reachable within timeout: %v", err)
	}
	defer engine.Close()

	ctx := context.Background()

	if err := engine.Ping(ctx); err != nil {
		t.Fatalf("Ping() failed: %v", err)
	}
	t.Log("Ping() succeeded against the live container")

	if _, err := engine.Query(ctx, `CREATE TABLE widgets (id INT AUTO_INCREMENT PRIMARY KEY, name VARCHAR(64) NOT NULL, weight INT, status VARCHAR(20) NOT NULL DEFAULT 'pending')`); err != nil {
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
	assertMySQLSelectResultColumns(t, selectResult.Columns)
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
	if !containsString(schemas, dbName) {
		t.Errorf("ListSchemas() = %v, want it to include %q", schemas, dbName)
	}
	for _, systemSchema := range []string{"mysql", "information_schema", "performance_schema", "sys"} {
		if containsString(schemas, systemSchema) {
			t.Errorf("ListSchemas() = %v, expected system database %q to be excluded", schemas, systemSchema)
		}
	}
	t.Logf("ListSchemas() succeeded: %v", schemas)

	tables, err := engine.ListTables(ctx, dbName)
	if err != nil {
		t.Fatalf("ListTables() failed: %v", err)
	}
	widgetsTable := findTable(tables, "widgets")
	if widgetsTable == nil {
		t.Fatalf("ListTables(%q) = %v, want it to include \"widgets\"", dbName, tables)
	}
	assertMySQLWidgetsColumns(t, widgetsTable.Columns)
	t.Logf("ListTables() succeeded: %+v", widgetsTable)

	cancelCtx, cancel := context.WithTimeout(ctx, 1*time.Second)
	defer cancel()
	cancelStart := time.Now()
	_, err = engine.Query(cancelCtx, `SELECT SLEEP(30)`)
	cancelDuration := time.Since(cancelStart)
	if err == nil {
		t.Error("expected SLEEP(30) under a 1s context timeout to fail, got nil error")
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

const (
	mysqlFKIntegrationTestProfileID int64 = 999018
	mysqlFKIntegrationTestServiceID int64 = 999018
	mysqlFKIntegrationTestHostPort        = 13310
)

// TestIntegration_MySQLEngine_ForeignKeys (tasks.md 4.5.1) proves
// ListForeignKeys against a real MySQL container with two tables and a
// genuine FOREIGN KEY constraint between them (authors.id <- books.author_id),
// not just a unit test against the query string.
func TestIntegration_MySQLEngine_ForeignKeys(t *testing.T) {
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
		ID:                mysqlFKIntegrationTestServiceID,
		ProfileID:         mysqlFKIntegrationTestProfileID,
		Engine:            storage.EngineMySQL,
		ImageTag:          "mysql:8",
		HostPort:          mysqlFKIntegrationTestHostPort,
		Username:          &username,
		PasswordEncrypted: &password,
		DBName:            &dbName,
		VolumeName:        "stackyard-test-vol-dbengine-mysql-fk",
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

	if err := dockerClient.StartMySQLEnvironment(setupCtx, svc); err != nil {
		t.Fatalf("StartMySQLEnvironment() failed: %v", err)
	}
	t.Logf("StartMySQLEnvironment: network %q, volume %q, container %q created/started", networkName, svc.VolumeName, containerName)

	dsn := fmt.Sprintf("%s:%s@tcp(127.0.0.1:%d)/%s?parseTime=true", username, password, svc.HostPort, dbName)
	engine := New(dsn)

	if err := waitForConnect(t, engine, 120*time.Second); err != nil {
		t.Fatalf("Engine failed to become reachable within timeout: %v", err)
	}
	defer engine.Close()

	ctx := context.Background()

	if _, err := engine.Query(ctx, `CREATE TABLE authors (id INT AUTO_INCREMENT PRIMARY KEY, name VARCHAR(64) NOT NULL)`); err != nil {
		t.Fatalf("CREATE TABLE authors failed: %v", err)
	}
	if _, err := engine.Query(ctx, `CREATE TABLE books (id INT AUTO_INCREMENT PRIMARY KEY, title VARCHAR(64) NOT NULL, author_id INT, FOREIGN KEY (author_id) REFERENCES authors(id))`); err != nil {
		t.Fatalf("CREATE TABLE books failed: %v", err)
	}
	t.Log("CREATE TABLE authors/books (with FK) succeeded")

	foreignKeys, err := engine.ListForeignKeys(ctx, dbName)
	if err != nil {
		t.Fatalf("ListForeignKeys() failed: %v", err)
	}
	if len(foreignKeys) != 1 {
		t.Fatalf("ListForeignKeys() = %+v, want exactly 1 foreign key", foreignKeys)
	}

	fk := foreignKeys[0]
	if fk.TableName != "books" || fk.ColumnName != "author_id" || fk.ReferencedTable != "authors" || fk.ReferencedColumn != "id" {
		t.Errorf("ListForeignKeys()[0] = %+v, want {books author_id authors id}", fk)
	}
	t.Logf("ListForeignKeys() succeeded: %+v", fk)
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

// assertMySQLSelectResultColumns asserts the QueryResult-level metadata
// (tasks.md 3.7) for the id/name/weight SELECT above: DatabaseType is a
// non-empty, sane type name for the integer id column and the varchar name
// column, and Nullable is a real bool matching each column's actual
// nullability per the widgets DDL — id and name are NOT NULL (id via its
// PRIMARY KEY, name via an explicit constraint), weight has no constraint
// and is nullable.
func assertMySQLSelectResultColumns(t *testing.T, columns []dbengine.ResultColumn) {
	t.Helper()
	byName := make(map[string]dbengine.ResultColumn, len(columns))
	for _, col := range columns {
		byName[col.Name] = col
	}

	id, ok := byName["id"]
	if !ok || id.DatabaseType != "INT" {
		t.Errorf("SELECT ResultColumn %q.DatabaseType = %q, want %q", "id", id.DatabaseType, "INT")
	}
	if id.Nullable == nil || *id.Nullable {
		t.Errorf("SELECT ResultColumn %q.Nullable = %v, want a non-nil false (id is the PRIMARY KEY)", "id", id.Nullable)
	}

	name, ok := byName["name"]
	if !ok || name.DatabaseType != "VARCHAR" {
		t.Errorf("SELECT ResultColumn %q.DatabaseType = %q, want %q", "name", name.DatabaseType, "VARCHAR")
	}
	if name.Nullable == nil || *name.Nullable {
		t.Errorf("SELECT ResultColumn %q.Nullable = %v, want a non-nil false (name is NOT NULL)", "name", name.Nullable)
	}

	weight, ok := byName["weight"]
	if !ok || weight.DatabaseType != "INT" {
		t.Errorf("SELECT ResultColumn %q.DatabaseType = %q, want %q", "weight", weight.DatabaseType, "INT")
	}
	if weight.Nullable == nil || !*weight.Nullable {
		t.Errorf("SELECT ResultColumn %q.Nullable = %v, want a non-nil true (weight has no NOT NULL constraint)", "weight", weight.Nullable)
	}
}

func assertMySQLWidgetsColumns(t *testing.T, columns []dbengine.ColumnInfo) {
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
	if weight.HasDefault {
		t.Error("widgets.weight has no DEFAULT clause and should be reported as HasDefault: false")
	}

	status, ok := byName["status"]
	if !ok {
		t.Fatal("widgets.status column missing from ListTables result")
	}
	if !status.HasDefault {
		t.Error("widgets.status has a DEFAULT 'pending' clause and should be reported as HasDefault: true")
	}

	if name.HasDefault {
		t.Error("widgets.name has no DEFAULT clause and should be reported as HasDefault: false")
	}
}
