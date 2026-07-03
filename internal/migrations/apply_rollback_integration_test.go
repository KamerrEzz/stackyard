//go:build integration

// Integration test for Apply and Rollback (tasks.md 8.3-8.4): exercises
// them against real Postgres and MySQL containers started through
// internal/docker's own StartPostgresEnvironment/StartMySQLEnvironment (no
// bespoke container-launch code, no mocks), reusing
// bootstrap_integration_test.go's waitForConnect helper since this file is
// the same package. Requires Docker Desktop/dockerd running; run with:
//
//	go test -tags=integration ./internal/migrations/...
//
// Uses test/profile/service IDs 999029 (Postgres) and 999030 (MySQL) —
// 999001-999028 are already taken across this repo's other integration
// tests (grepped for every 9990\d\d literal in the repo before picking
// these, per docs/STATE.md's running convention) — and host ports 15543
// (Postgres) and 13313 (MySQL). These are distinct from every other
// integration test's HARDCODED HOST PORT in this repo — a separate number
// space from the 9990\d\d test-ID convention above, since ports and IDs
// are independently assigned; grep every `HostPort\s*=\s*\d+` literal
// (not just `9990\d\d`) before picking a port for a new integration test,
// since two different test IDs can still collide on the same port when
// `go test ./...` runs packages concurrently. Everything created is torn
// down in t.Cleanup so the test is fully self-cleaning and safely
// re-runnable.
package migrations

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"stackyard/internal/dbengine"
	"stackyard/internal/dbengine/mysql"
	"stackyard/internal/dbengine/postgres"
	"stackyard/internal/docker"
	"stackyard/internal/storage"
)

const (
	applyRollbackIntegrationPostgresProfileID int64 = 999029
	applyRollbackIntegrationPostgresServiceID int64 = 999029
	applyRollbackIntegrationPostgresHostPort        = 15543

	applyRollbackIntegrationMySQLProfileID int64 = 999030
	applyRollbackIntegrationMySQLServiceID int64 = 999030
	applyRollbackIntegrationMySQLHostPort        = 13313
)

func TestIntegration_ApplyRollback_Postgres(t *testing.T) {
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
		ID:                applyRollbackIntegrationPostgresServiceID,
		ProfileID:         applyRollbackIntegrationPostgresProfileID,
		Engine:            storage.EnginePostgres,
		ImageTag:          "postgres:16-alpine",
		HostPort:          applyRollbackIntegrationPostgresHostPort,
		Username:          &username,
		PasswordEncrypted: &password,
		DBName:            &dbName,
		VolumeName:        "stackyard-test-vol-apply-rollback-postgres",
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

	connString := docker.PostgresConnectionString(svc)
	engine := postgres.New(connString)

	if err := waitForConnect(t, engine, 90*time.Second); err != nil {
		t.Fatalf("Engine failed to become reachable within timeout: %v", err)
	}
	defer engine.Close()

	runApplyRollbackLifecycle(t, engine, dbengine.DialectPostgres, "public")
}

func TestIntegration_ApplyRollback_MySQL(t *testing.T) {
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
		ID:                applyRollbackIntegrationMySQLServiceID,
		ProfileID:         applyRollbackIntegrationMySQLProfileID,
		Engine:            storage.EngineMySQL,
		ImageTag:          "mysql:8",
		HostPort:          applyRollbackIntegrationMySQLHostPort,
		Username:          &username,
		PasswordEncrypted: &password,
		DBName:            &dbName,
		VolumeName:        "stackyard-test-vol-apply-rollback-mysql",
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

	dsn := fmt.Sprintf("%s:%s@tcp(127.0.0.1:%d)/%s?parseTime=true", username, password, svc.HostPort, dbName)
	engine := mysql.New(dsn)

	if err := waitForConnect(t, engine, 120*time.Second); err != nil {
		t.Fatalf("Engine failed to become reachable within timeout: %v", err)
	}
	defer engine.Close()

	runApplyRollbackLifecycle(t, engine, dbengine.DialectMySQL, dbName)
}

// runApplyRollbackLifecycle drives the full Apply/Rollback lifecycle
// against a single real, already-connected engine — shared between the
// Postgres and MySQL integration tests above so both exercise identical
// scenarios:
//
//  1. Rollback with nothing applied yet returns (nil, nil).
//  2. Apply against 3 pending migrations where the 2nd's up.sql is
//     deliberately invalid: migration 1 applies and is tracked; migration 2
//     neither creates its table nor gets a tracking row; migration 3 is
//     never attempted at all — every claim verified via direct queries
//     against the target database, not just Apply's returned Go value.
//  3. Fixing migration 2's up.sql and calling Apply again picks up exactly
//     the remaining pending migrations (2 and 3), in order.
//  4. Rollback, called three times in a row against a stack of 3 applied
//     migrations, reverts exactly one migration per call, most-recent-first,
//     each time confirmed via direct queries — never touching earlier ones.
//  5. Rollback with nothing applied afterward again returns (nil, nil).
func runApplyRollbackLifecycle(t *testing.T, engine dbengine.Engine, dialect dbengine.Dialect, schema string) {
	t.Helper()
	ctx := context.Background()

	if err := BootstrapTrackingTable(ctx, engine); err != nil {
		t.Fatalf("BootstrapTrackingTable() failed: %v", err)
	}

	folder := t.TempDir()

	reverted, err := Rollback(ctx, engine, dialect, folder)
	if err != nil {
		t.Fatalf("Rollback() with nothing applied failed: %v", err)
	}
	if reverted != nil {
		t.Fatalf("Rollback() with nothing applied = %v, want nil", reverted)
	}
	t.Log("Rollback() with nothing applied correctly returned (nil, nil)")

	m1 := writeMigrationPair(t, folder, 20260101000001, "m1_create_first",
		"CREATE TABLE integration_first (id INT PRIMARY KEY);",
		"DROP TABLE integration_first;")
	m2 := writeMigrationPair(t, folder, 20260101000002, "m2_create_second",
		"THIS IS NOT VALID SQL AT ALL AND WILL FAIL;",
		"DROP TABLE integration_second;")
	m3 := writeMigrationPair(t, folder, 20260101000003, "m3_create_third",
		"CREATE TABLE integration_third (id INT PRIMARY KEY);",
		"DROP TABLE integration_third;")

	result, err := Apply(ctx, engine, dialect, folder)
	if err != nil {
		t.Fatalf("Apply() with a deliberately broken 2nd migration returned a top-level error: %v", err)
	}
	if len(result.Applied) != 1 || result.Applied[0].Version != m1.Version {
		t.Fatalf("Apply().Applied = %v, want exactly migration 1", result.Applied)
	}
	if result.Failed == nil || result.Failed.Version != m2.Version {
		t.Fatalf("Apply().Failed = %v, want migration 2", result.Failed)
	}
	if result.FailedError == "" {
		t.Fatal("Apply().FailedError is empty, want the real DB error surfaced")
	}
	t.Logf("Apply() correctly reported migration 1 applied, migration 2 failed with: %s", result.FailedError)

	assertTableExists(t, engine, schema, "integration_first", true)
	assertTableExists(t, engine, schema, "integration_second", false)
	assertTableExists(t, engine, schema, "integration_third", false)
	assertAppliedVersions(t, engine, []int64{m1.Version})
	t.Log("confirmed via direct DB queries: migration 1's schema change and tracking row both landed, migration 2's schema change did not land and has no tracking row, migration 3 was never attempted")

	fixedM2 := writeMigrationPair(t, folder, 20260101000002, "m2_create_second",
		"CREATE TABLE integration_second (id INT PRIMARY KEY);",
		"DROP TABLE integration_second;")
	if fixedM2.Version != m2.Version {
		t.Fatalf("rewritten migration 2 version = %d, want %d", fixedM2.Version, m2.Version)
	}

	result, err = Apply(ctx, engine, dialect, folder)
	if err != nil {
		t.Fatalf("second Apply() call (after fixing migration 2) failed: %v", err)
	}
	if len(result.Applied) != 2 || result.Applied[0].Version != m2.Version || result.Applied[1].Version != m3.Version {
		t.Fatalf("second Apply().Applied = %v, want migrations 2 and 3 in order", result.Applied)
	}
	if result.Failed != nil {
		t.Fatalf("second Apply().Failed = %v, want nil", result.Failed)
	}

	assertTableExists(t, engine, schema, "integration_second", true)
	assertTableExists(t, engine, schema, "integration_third", true)
	assertAppliedVersions(t, engine, []int64{m1.Version, m2.Version, m3.Version})
	t.Log("confirmed via direct DB queries: after fixing migration 2's SQL, Apply picked up exactly migrations 2 and 3, in order")

	reverted, err = Rollback(ctx, engine, dialect, folder)
	if err != nil {
		t.Fatalf("first Rollback() call failed: %v", err)
	}
	if reverted == nil || reverted.Version != m3.Version {
		t.Fatalf("first Rollback() = %v, want migration 3 (the most recently applied)", reverted)
	}
	assertTableExists(t, engine, schema, "integration_third", false)
	assertTableExists(t, engine, schema, "integration_first", true)
	assertTableExists(t, engine, schema, "integration_second", true)
	assertAppliedVersions(t, engine, []int64{m1.Version, m2.Version})
	t.Log("confirmed via direct DB queries: first Rollback() reverted only migration 3, leaving 1 and 2 untouched")

	reverted, err = Rollback(ctx, engine, dialect, folder)
	if err != nil {
		t.Fatalf("second Rollback() call failed: %v", err)
	}
	if reverted == nil || reverted.Version != m2.Version {
		t.Fatalf("second Rollback() = %v, want migration 2", reverted)
	}
	assertTableExists(t, engine, schema, "integration_second", false)
	assertTableExists(t, engine, schema, "integration_first", true)
	assertAppliedVersions(t, engine, []int64{m1.Version})
	t.Log("confirmed via direct DB queries: second Rollback() reverted only migration 2, leaving migration 1 untouched")

	reverted, err = Rollback(ctx, engine, dialect, folder)
	if err != nil {
		t.Fatalf("third Rollback() call failed: %v", err)
	}
	if reverted == nil || reverted.Version != m1.Version {
		t.Fatalf("third Rollback() = %v, want migration 1", reverted)
	}
	assertTableExists(t, engine, schema, "integration_first", false)
	assertAppliedVersions(t, engine, nil)
	t.Log("confirmed via direct DB queries: third Rollback() reverted migration 1, leaving schema_migrations empty")

	reverted, err = Rollback(ctx, engine, dialect, folder)
	if err != nil {
		t.Fatalf("Rollback() after every migration was reverted failed: %v", err)
	}
	if reverted != nil {
		t.Fatalf("Rollback() after every migration was reverted = %v, want nil", reverted)
	}
	t.Log("Rollback() after every migration was reverted correctly returned (nil, nil) again")
}

// assertTableExists confirms, via engine.ListTables against the live
// database (not Apply/Rollback's own return value), whether tableName
// exists in schema.
func assertTableExists(t *testing.T, engine dbengine.Engine, schema, tableName string, want bool) {
	t.Helper()
	tables, err := engine.ListTables(context.Background(), schema)
	if err != nil {
		t.Fatalf("ListTables(%q) failed: %v", schema, err)
	}
	got := false
	for _, tbl := range tables {
		if strings.EqualFold(tbl.Name, tableName) {
			got = true
			break
		}
	}
	if got != want {
		t.Fatalf("table %q exists = %v, want %v (tables in schema: %v)", tableName, got, want, tables)
	}
}

// assertAppliedVersions confirms, via a direct SELECT against
// schema_migrations (not Apply/Rollback's own return value), exactly which
// versions are currently recorded as applied.
func assertAppliedVersions(t *testing.T, engine dbengine.Engine, want []int64) {
	t.Helper()
	result, err := engine.Query(context.Background(), `SELECT version FROM schema_migrations ORDER BY version`)
	if err != nil {
		t.Fatalf("query schema_migrations failed: %v", err)
	}
	got := make([]int64, 0, len(result.Rows))
	for _, row := range result.Rows {
		v, err := toInt64(row[0])
		if err != nil {
			t.Fatalf("parse schema_migrations.version: %v", err)
		}
		got = append(got, v)
	}
	if len(got) != len(want) {
		t.Fatalf("schema_migrations applied versions = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("schema_migrations applied versions = %v, want %v", got, want)
		}
	}
}
