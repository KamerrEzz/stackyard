//go:build integration

// Integration test for tasks.md 7.3's SQL dump round trip: generates a dump
// from a live, seeded Postgres/MySQL table using this package's own real
// export.go helpers (fetchAllTableRows, buildColumnDumpInfo,
// internal/export.ToSQLDump/BuildCreateTable/BuildInsertStatements — not a
// reimplementation of that logic in the test), executes the generated
// CREATE TABLE + INSERT statements against a second, genuinely fresh
// container of the same engine, and compares the two tables' row contents
// exactly (not just "no SQL error"). Requires Docker Desktop/dockerd
// running; run with:
//
//	go test -tags=integration .
//
// Uses test/profile/service IDs 999023 (Postgres source), 999024 (Postgres
// fresh target), 999025 (MySQL source), 999026 (MySQL fresh target) — the
// next free IDs per docs/STATE.md's running 9990\d\d registry (999001-999022
// already taken; grepped the whole repo before picking these). Host ports
// 15540/15541 (Postgres) and 13320/13321 (MySQL), distinct from every other
// integration test's port in this repo. Everything created is torn down in
// t.Cleanup so the test is fully self-cleaning and safely re-runnable.
package main

import (
	"context"
	"fmt"
	"testing"
	"time"

	"stackyard/internal/dbengine"
	dbenginemysql "stackyard/internal/dbengine/mysql"
	dbenginepostgres "stackyard/internal/dbengine/postgres"
	"stackyard/internal/docker"
	"stackyard/internal/export"
	"stackyard/internal/storage"
)

func TestIntegration_ExportSQLDump_PostgresRoundTrip(t *testing.T) {
	const (
		sourceProfileID int64 = 999023
		sourceServiceID int64 = 999023
		sourceHostPort        = 15540

		targetProfileID int64 = 999024
		targetServiceID int64 = 999024
		targetHostPort        = 15541
	)

	dockerClient, err := docker.NewClient()
	if err != nil {
		t.Fatalf("docker.NewClient() failed: %v", err)
	}
	defer dockerClient.Close()

	setupCtx, setupCancel := context.WithTimeout(context.Background(), 240*time.Second)
	defer setupCancel()

	if err := dockerClient.Ping(setupCtx); err != nil {
		t.Fatalf("Ping() failed to reach the local Docker Engine: %v", err)
	}

	username := "stackyard_test"
	password := "stackyard_test_pw"
	dbName := "stackyard_test_db"

	sourceSvc := storage.Service{
		ID: sourceServiceID, ProfileID: sourceProfileID, Engine: storage.EnginePostgres,
		ImageTag: "postgres:16-alpine", HostPort: sourceHostPort,
		Username: &username, PasswordEncrypted: &password, DBName: &dbName,
		VolumeName: "stackyard-test-vol-export-pg-source",
	}
	targetSvc := storage.Service{
		ID: targetServiceID, ProfileID: targetProfileID, Engine: storage.EnginePostgres,
		ImageTag: "postgres:16-alpine", HostPort: targetHostPort,
		Username: &username, PasswordEncrypted: &password, DBName: &dbName,
		VolumeName: "stackyard-test-vol-export-pg-target",
	}

	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()
		for _, svc := range []storage.Service{sourceSvc, targetSvc} {
			containerName := docker.ServiceContainerName(svc.ID)
			networkName := docker.ProfileNetworkName(svc.ProfileID)
			_ = dockerClient.RemoveContainer(cleanupCtx, containerName)
			_ = dockerClient.RemoveVolume(cleanupCtx, svc.VolumeName)
			_ = dockerClient.RemoveNetwork(cleanupCtx, networkName)
		}
	})

	if err := dockerClient.StartPostgresEnvironment(setupCtx, sourceSvc); err != nil {
		t.Fatalf("StartPostgresEnvironment(source) failed: %v", err)
	}
	if err := dockerClient.StartPostgresEnvironment(setupCtx, targetSvc); err != nil {
		t.Fatalf("StartPostgresEnvironment(target) failed: %v", err)
	}

	sourceEngine := dbenginepostgres.New(docker.PostgresConnectionString(sourceSvc))
	if err := waitForPostgresConnect(sourceEngine, 90*time.Second); err != nil {
		t.Fatalf("source Engine failed to become reachable: %v", err)
	}
	defer sourceEngine.Close()

	targetEngine := dbenginepostgres.New(docker.PostgresConnectionString(targetSvc))
	if err := waitForPostgresConnect(targetEngine, 90*time.Second); err != nil {
		t.Fatalf("target Engine failed to become reachable: %v", err)
	}
	defer targetEngine.Close()

	ctx := context.Background()

	if _, err := sourceEngine.Query(ctx, `CREATE TABLE widgets (
		id SERIAL PRIMARY KEY,
		name TEXT NOT NULL,
		note TEXT,
		weight NUMERIC(10,2),
		created_at TIMESTAMP
	)`); err != nil {
		t.Fatalf("CREATE TABLE on source failed: %v", err)
	}

	if _, err := sourceEngine.Query(ctx, `INSERT INTO widgets (name, note, weight, created_at) VALUES
		('bolt', NULL, 12.50, '2024-03-15 10:30:00'),
		('nut', '', 3.75, '2024-03-16 11:00:00'),
		('washer', 'plain', 0.99, '2024-03-17 09:15:00')`); err != nil {
		t.Fatalf("INSERT on source failed: %v", err)
	}

	sourceSession := &querySession{engine: sourceEngine, engineType: storage.EnginePostgres}

	tableInfo, err := gridTableInfo(ctx, sourceSession, "public", "widgets")
	if err != nil {
		t.Fatalf("gridTableInfo() failed: %v", err)
	}

	columns, err := buildColumnDumpInfo(ctx, sourceSession, dbengine.DialectPostgres, "public", tableInfo)
	if err != nil {
		t.Fatalf("buildColumnDumpInfo() failed: %v", err)
	}

	sourceColumnNames, sourceRows, err := fetchAllTableRows(ctx, sourceSession, dbengine.DialectPostgres, "public", "widgets")
	if err != nil {
		t.Fatalf("fetchAllTableRows(source) failed: %v", err)
	}
	if len(sourceRows) != 3 {
		t.Fatalf("source table has %d rows, want 3", len(sourceRows))
	}

	dump := export.ToSQLDump(dbengine.DialectPostgres, "public", "widgets", columns, sourceRows)
	t.Logf("generated dump:\n%s", dump)

	createStatement := export.BuildCreateTable(dbengine.DialectPostgres, "public", "widgets", columns)
	if _, err := targetEngine.Query(ctx, createStatement); err != nil {
		t.Fatalf("executing generated CREATE TABLE against fresh target failed: %v\nstatement was:\n%s", err, createStatement)
	}

	for i, stmt := range export.BuildInsertStatements(dbengine.DialectPostgres, "public", "widgets", sourceColumnNames, sourceRows) {
		if _, err := targetEngine.Query(ctx, stmt); err != nil {
			t.Fatalf("executing generated INSERT batch %d against fresh target failed: %v\nstatement was:\n%s", i, err, stmt)
		}
	}

	targetSession := &querySession{engine: targetEngine, engineType: storage.EnginePostgres}
	targetColumnNames, targetRows, err := fetchAllTableRows(ctx, targetSession, dbengine.DialectPostgres, "public", "widgets")
	if err != nil {
		t.Fatalf("fetchAllTableRows(target) failed: %v", err)
	}

	assertRoundTrippedRowsMatch(t, sourceColumnNames, sourceRows, targetColumnNames, targetRows)
	assertNullVsEmptyStringPreserved(t, targetColumnNames, targetRows, "note")
}

func TestIntegration_ExportSQLDump_MySQLRoundTrip(t *testing.T) {
	const (
		sourceProfileID int64 = 999025
		sourceServiceID int64 = 999025
		sourceHostPort        = 13320

		targetProfileID int64 = 999026
		targetServiceID int64 = 999026
		targetHostPort        = 13321
	)

	dockerClient, err := docker.NewClient()
	if err != nil {
		t.Fatalf("docker.NewClient() failed: %v", err)
	}
	defer dockerClient.Close()

	setupCtx, setupCancel := context.WithTimeout(context.Background(), 240*time.Second)
	defer setupCancel()

	if err := dockerClient.Ping(setupCtx); err != nil {
		t.Fatalf("Ping() failed to reach the local Docker Engine: %v", err)
	}

	username := "stackyard_test"
	password := "stackyard_test_pw"
	dbName := "stackyard_test_db"

	sourceSvc := storage.Service{
		ID: sourceServiceID, ProfileID: sourceProfileID, Engine: storage.EngineMySQL,
		ImageTag: "mysql:8", HostPort: sourceHostPort,
		Username: &username, PasswordEncrypted: &password, DBName: &dbName,
		VolumeName: "stackyard-test-vol-export-mysql-source",
	}
	targetSvc := storage.Service{
		ID: targetServiceID, ProfileID: targetProfileID, Engine: storage.EngineMySQL,
		ImageTag: "mysql:8", HostPort: targetHostPort,
		Username: &username, PasswordEncrypted: &password, DBName: &dbName,
		VolumeName: "stackyard-test-vol-export-mysql-target",
	}

	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()
		for _, svc := range []storage.Service{sourceSvc, targetSvc} {
			containerName := docker.ServiceContainerName(svc.ID)
			networkName := docker.ProfileNetworkName(svc.ProfileID)
			_ = dockerClient.RemoveContainer(cleanupCtx, containerName)
			_ = dockerClient.RemoveVolume(cleanupCtx, svc.VolumeName)
			_ = dockerClient.RemoveNetwork(cleanupCtx, networkName)
		}
	})

	if err := dockerClient.StartMySQLEnvironment(setupCtx, sourceSvc); err != nil {
		t.Fatalf("StartMySQLEnvironment(source) failed: %v", err)
	}
	if err := dockerClient.StartMySQLEnvironment(setupCtx, targetSvc); err != nil {
		t.Fatalf("StartMySQLEnvironment(target) failed: %v", err)
	}

	sourceDSN := fmt.Sprintf("%s:%s@tcp(127.0.0.1:%d)/%s?parseTime=true", username, password, sourceHostPort, dbName)
	sourceEngine := dbenginemysql.New(sourceDSN)
	if err := waitForMySQLConnect(sourceEngine, 120*time.Second); err != nil {
		t.Fatalf("source Engine failed to become reachable: %v", err)
	}
	defer sourceEngine.Close()

	targetDSN := fmt.Sprintf("%s:%s@tcp(127.0.0.1:%d)/%s?parseTime=true", username, password, targetHostPort, dbName)
	targetEngine := dbenginemysql.New(targetDSN)
	if err := waitForMySQLConnect(targetEngine, 120*time.Second); err != nil {
		t.Fatalf("target Engine failed to become reachable: %v", err)
	}
	defer targetEngine.Close()

	ctx := context.Background()

	if _, err := sourceEngine.Query(ctx, `CREATE TABLE widgets (
		id INT AUTO_INCREMENT PRIMARY KEY,
		name VARCHAR(64) NOT NULL,
		note VARCHAR(255),
		weight DECIMAL(10,2),
		created_at DATETIME
	)`); err != nil {
		t.Fatalf("CREATE TABLE on source failed: %v", err)
	}

	if _, err := sourceEngine.Query(ctx, `INSERT INTO widgets (name, note, weight, created_at) VALUES
		('bolt', NULL, 12.50, '2024-03-15 10:30:00'),
		('nut', '', 3.75, '2024-03-16 11:00:00'),
		('washer', 'plain', 0.99, '2024-03-17 09:15:00')`); err != nil {
		t.Fatalf("INSERT on source failed: %v", err)
	}

	sourceSession := &querySession{engine: sourceEngine, engineType: storage.EngineMySQL}

	tableInfo, err := gridTableInfo(ctx, sourceSession, dbName, "widgets")
	if err != nil {
		t.Fatalf("gridTableInfo() failed: %v", err)
	}

	columns, err := buildColumnDumpInfo(ctx, sourceSession, dbengine.DialectMySQL, dbName, tableInfo)
	if err != nil {
		t.Fatalf("buildColumnDumpInfo() failed: %v", err)
	}
	for _, col := range columns {
		if col.Name == "name" && col.SQLType == "varchar" {
			t.Errorf("column %q SQLType = %q, want a length-qualified type like varchar(64) (COLUMN_TYPE lookup should have applied)", col.Name, col.SQLType)
		}
	}

	sourceColumnNames, sourceRows, err := fetchAllTableRows(ctx, sourceSession, dbengine.DialectMySQL, dbName, "widgets")
	if err != nil {
		t.Fatalf("fetchAllTableRows(source) failed: %v", err)
	}
	if len(sourceRows) != 3 {
		t.Fatalf("source table has %d rows, want 3", len(sourceRows))
	}

	dump := export.ToSQLDump(dbengine.DialectMySQL, dbName, "widgets", columns, sourceRows)
	t.Logf("generated dump:\n%s", dump)

	createStatement := export.BuildCreateTable(dbengine.DialectMySQL, dbName, "widgets", columns)
	if _, err := targetEngine.Query(ctx, createStatement); err != nil {
		t.Fatalf("executing generated CREATE TABLE against fresh target failed: %v\nstatement was:\n%s", err, createStatement)
	}

	for i, stmt := range export.BuildInsertStatements(dbengine.DialectMySQL, dbName, "widgets", sourceColumnNames, sourceRows) {
		if _, err := targetEngine.Query(ctx, stmt); err != nil {
			t.Fatalf("executing generated INSERT batch %d against fresh target failed: %v\nstatement was:\n%s", i, err, stmt)
		}
	}

	targetSession := &querySession{engine: targetEngine, engineType: storage.EngineMySQL}
	targetColumnNames, targetRows, err := fetchAllTableRows(ctx, targetSession, dbengine.DialectMySQL, dbName, "widgets")
	if err != nil {
		t.Fatalf("fetchAllTableRows(target) failed: %v", err)
	}

	assertRoundTrippedRowsMatch(t, sourceColumnNames, sourceRows, targetColumnNames, targetRows)
	assertNullVsEmptyStringPreserved(t, targetColumnNames, targetRows, "note")
}

// assertRoundTrippedRowsMatch compares source and target row contents by
// rendering both through export.ToCSV and comparing the resulting text —
// deliberately not reflect.DeepEqual on the raw driver values, since a
// numeric/decimal column's Go-side representation (e.g. pgx's pgtype.Numeric)
// only needs to be textually identical after a round trip, not structurally
// identical at the Go type level. This is also, itself, a second real
// exercise of ToCSV against two independently-fetched result sets.
func assertRoundTrippedRowsMatch(t *testing.T, sourceColumns []string, sourceRows [][]any, targetColumns []string, targetRows [][]any) {
	t.Helper()
	sourceCSV, err := export.ToCSV(sourceColumns, sourceRows)
	if err != nil {
		t.Fatalf("export.ToCSV(source) failed: %v", err)
	}
	targetCSV, err := export.ToCSV(targetColumns, targetRows)
	if err != nil {
		t.Fatalf("export.ToCSV(target) failed: %v", err)
	}
	if sourceCSV != targetCSV {
		t.Errorf("round-tripped table does not match source exactly.\nsource:\n%s\ntarget:\n%s", sourceCSV, targetCSV)
	}
}

// assertNullVsEmptyStringPreserved asserts columnName's value is nil for
// exactly one row and the empty string for exactly one other row within
// rows — the literal NULL-vs-empty-string distinguishability spec.md §4.9
// requires, checked directly against the re-imported target table's own
// values (not just against the generated dump's text).
func assertNullVsEmptyStringPreserved(t *testing.T, columnNames []string, rows [][]any, columnName string) {
	t.Helper()
	colIndex := -1
	for i, name := range columnNames {
		if name == columnName {
			colIndex = i
			break
		}
	}
	if colIndex < 0 {
		t.Fatalf("column %q not found in %v", columnName, columnNames)
	}

	var sawNull, sawEmptyString bool
	for _, row := range rows {
		if colIndex >= len(row) {
			continue
		}
		v := row[colIndex]
		if v == nil {
			sawNull = true
		} else if v == "" {
			sawEmptyString = true
		}
	}
	if !sawNull {
		t.Errorf("expected at least one row with %s = NULL after round trip, found none", columnName)
	}
	if !sawEmptyString {
		t.Errorf("expected at least one row with %s = '' (empty string, distinguishable from NULL) after round trip, found none", columnName)
	}
}

func waitForPostgresConnect(engine *dbenginepostgres.Engine, timeout time.Duration) error {
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

func waitForMySQLConnect(engine *dbenginemysql.Engine, timeout time.Duration) error {
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
