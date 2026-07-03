//go:build integration

// Integration test for BootstrapTrackingTable (tasks.md 8.2): exercises it
// against real Postgres and MySQL containers started through
// internal/docker's own StartPostgresEnvironment/StartMySQLEnvironment (no
// bespoke container-launch code, no mocks). Requires Docker Desktop/dockerd
// running; run with:
//
//	go test -tags=integration ./internal/migrations/...
//
// Uses test/profile/service IDs 999027 (Postgres) and 999028 (MySQL) —
// 999001-999026 are already taken across this repo's other integration
// tests (grepped for every 9990\d\d literal in the repo before picking
// these, per docs/STATE.md's running convention) — and host ports 15538
// (Postgres) and 13309 (MySQL), distinct from every other integration
// test's port in this repo. Everything created is torn down in t.Cleanup so
// the test is fully self-cleaning and safely re-runnable.
package migrations

import (
	"context"
	"fmt"
	"testing"
	"time"

	"stackyard/internal/dbengine"
	"stackyard/internal/dbengine/mysql"
	"stackyard/internal/dbengine/postgres"
	"stackyard/internal/docker"
	"stackyard/internal/storage"
)

const (
	bootstrapIntegrationPostgresProfileID int64 = 999027
	bootstrapIntegrationPostgresServiceID int64 = 999027
	bootstrapIntegrationPostgresHostPort        = 15538

	bootstrapIntegrationMySQLProfileID int64 = 999028
	bootstrapIntegrationMySQLServiceID int64 = 999028
	bootstrapIntegrationMySQLHostPort        = 13309
)

func TestIntegration_BootstrapTrackingTable_Postgres(t *testing.T) {
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
		ID:                bootstrapIntegrationPostgresServiceID,
		ProfileID:         bootstrapIntegrationPostgresProfileID,
		Engine:            storage.EnginePostgres,
		ImageTag:          "postgres:16-alpine",
		HostPort:          bootstrapIntegrationPostgresHostPort,
		Username:          &username,
		PasswordEncrypted: &password,
		DBName:            &dbName,
		VolumeName:        "stackyard-test-vol-migrations-postgres",
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

	assertBootstrapIsIdempotentAndCreatesTable(t, engine, "public")
}

func TestIntegration_BootstrapTrackingTable_MySQL(t *testing.T) {
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
		ID:                bootstrapIntegrationMySQLServiceID,
		ProfileID:         bootstrapIntegrationMySQLProfileID,
		Engine:            storage.EngineMySQL,
		ImageTag:          "mysql:8",
		HostPort:          bootstrapIntegrationMySQLHostPort,
		Username:          &username,
		PasswordEncrypted: &password,
		DBName:            &dbName,
		VolumeName:        "stackyard-test-vol-migrations-mysql",
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

	assertBootstrapIsIdempotentAndCreatesTable(t, engine, dbName)
}

// assertBootstrapIsIdempotentAndCreatesTable calls BootstrapTrackingTable
// twice against engine (proving the second call neither errors nor
// duplicates anything) and confirms schema_migrations exists in schema
// afterward via ListTables.
func assertBootstrapIsIdempotentAndCreatesTable(t *testing.T, engine dbengine.Engine, schema string) {
	t.Helper()
	ctx := context.Background()

	if err := BootstrapTrackingTable(ctx, engine); err != nil {
		t.Fatalf("first BootstrapTrackingTable call failed: %v", err)
	}
	t.Log("first BootstrapTrackingTable call succeeded")

	if err := BootstrapTrackingTable(ctx, engine); err != nil {
		t.Fatalf("second BootstrapTrackingTable call failed (expected idempotent no-op): %v", err)
	}
	t.Log("second BootstrapTrackingTable call succeeded (idempotent)")

	tables, err := engine.ListTables(ctx, schema)
	if err != nil {
		t.Fatalf("ListTables failed: %v", err)
	}

	found := false
	for _, tbl := range tables {
		if tbl.Name == "schema_migrations" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected schema_migrations to exist in schema %q after bootstrap, tables=%v", schema, tables)
	}
}

// waitForConnect retries engine.Connect until it succeeds or timeout
// elapses — freshly started database containers can take a few seconds
// before they accept connections, matching the retry-loop pattern
// internal/dbengine's own integration tests already use.
func waitForConnect(t *testing.T, engine dbengine.Engine, timeout time.Duration) error {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		if err := engine.Connect(context.Background()); err == nil {
			return nil
		} else {
			lastErr = err
		}
		time.Sleep(1 * time.Second)
	}
	return lastErr
}
