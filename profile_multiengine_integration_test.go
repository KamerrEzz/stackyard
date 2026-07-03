//go:build integration

// Integration test for tasks.md 2.4: confirms StartProfile/StopProfile/
// GetProfileStatus start and stop a HETEROGENEOUS profile (Postgres + Redis)
// AS A SINGLE UNIT, dispatching each service to its own engine via
// engineStarters (app.go) rather than assuming every service is Postgres.
// The existing internal/docker per-engine integration tests already prove
// each Start<Engine>Environment works in isolation against a live engine;
// this test is the one that actually exercises the App-level multi-engine
// dispatch tasks.md 2.4 asks for. Requires Docker Desktop/dockerd running;
// run with:
//
//	go test -tags=integration ./...
//
// Uses test ID 999006 (internal/docker's per-engine integration tests
// already occupy 999001-999005 — see docs/STATE.md's Phase 2 wave 1 section
// for the registry; this is the next free ID). Host ports 25432 (Postgres)
// and 26379 (Redis) are distinct from every existing integration test's
// chosen ports. The profile and its two services are inserted directly via
// raw SQL with explicit IDs — rather than through storage.CreateProfile/
// CreateService's auto-increment — so the resulting Docker resource names
// are deterministic, matching the convention the internal/docker
// integration tests already establish, and so they can't collide with
// whatever ID a real dev database's own profile 1 happens to occupy.
// Everything created (SQLite rows, network, volumes, containers) is torn
// down in t.Cleanup via internal/docker's cleanup.go helpers, so the test is
// fully self-cleaning and safely re-runnable.
package main

import (
	"context"
	"net"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"stackyard/internal/docker"
	"stackyard/internal/storage"
)

const (
	multiEngineTestProfileID     int64 = 999006
	multiEngineTestPostgresSvcID int64 = 999006
	multiEngineTestRedisSvcID    int64 = 999007
	multiEngineTestPostgresPort        = 25432
	multiEngineTestRedisPort           = 26379
)

func TestIntegration_StartStopProfile_MixedEngines_AsAUnit(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "stackyard-multiengine-test.db")
	db, err := storage.OpenAt(dbPath)
	if err != nil {
		t.Fatalf("storage.OpenAt(%q) failed: %v", dbPath, err)
	}
	defer db.Close()

	dockerClient, err := docker.NewClient()
	if err != nil {
		t.Fatalf("docker.NewClient() failed: %v", err)
	}
	defer dockerClient.Close()

	pingCtx, pingCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer pingCancel()
	if err := dockerClient.Ping(pingCtx); err != nil {
		t.Fatalf("Ping() failed to reach the local Docker Engine: %v", err)
	}

	opCtx, opCancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer opCancel()
	a := &App{ctx: opCtx, db: db, docker: dockerClient}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := db.Exec(
		`INSERT INTO profiles (id, name, created_at) VALUES (?, ?, ?)`,
		multiEngineTestProfileID, "multi-engine-integration-test", now,
	); err != nil {
		t.Fatalf("insert test profile failed: %v", err)
	}

	pgUsername, pgPassword, pgDBName := "stackyard_test", "stackyard_test_pw", "stackyard_test_db"
	if _, err := db.Exec(
		`INSERT INTO services (id, profile_id, engine, image_tag, host_port, username, password_encrypted, db_name, volume_name)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		multiEngineTestPostgresSvcID, multiEngineTestProfileID, storage.EnginePostgres, "postgres:16-alpine", multiEngineTestPostgresPort,
		pgUsername, pgPassword, pgDBName, "stackyard-test-vol-2.4-postgres",
	); err != nil {
		t.Fatalf("insert test postgres service failed: %v", err)
	}

	redisPassword := "stackyard_test_pw"
	if _, err := db.Exec(
		`INSERT INTO services (id, profile_id, engine, image_tag, host_port, username, password_encrypted, db_name, volume_name)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		multiEngineTestRedisSvcID, multiEngineTestProfileID, storage.EngineRedis, "redis:7-alpine", multiEngineTestRedisPort,
		nil, redisPassword, nil, "stackyard-test-vol-2.4-redis",
	); err != nil {
		t.Fatalf("insert test redis service failed: %v", err)
	}

	postgresContainerName := docker.ServiceContainerName(multiEngineTestPostgresSvcID)
	redisContainerName := docker.ServiceContainerName(multiEngineTestRedisSvcID)
	networkName := docker.ProfileNetworkName(multiEngineTestProfileID)

	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()

		if err := dockerClient.RemoveContainer(cleanupCtx, postgresContainerName); err != nil {
			t.Logf("cleanup: failed to remove container %s: %v", postgresContainerName, err)
		} else {
			t.Logf("cleanup: removed container %s", postgresContainerName)
		}
		if err := dockerClient.RemoveContainer(cleanupCtx, redisContainerName); err != nil {
			t.Logf("cleanup: failed to remove container %s: %v", redisContainerName, err)
		} else {
			t.Logf("cleanup: removed container %s", redisContainerName)
		}
		if err := dockerClient.RemoveVolume(cleanupCtx, "stackyard-test-vol-2.4-postgres"); err != nil {
			t.Logf("cleanup: failed to remove volume stackyard-test-vol-2.4-postgres: %v", err)
		}
		if err := dockerClient.RemoveVolume(cleanupCtx, "stackyard-test-vol-2.4-redis"); err != nil {
			t.Logf("cleanup: failed to remove volume stackyard-test-vol-2.4-redis: %v", err)
		}
		if err := dockerClient.RemoveNetwork(cleanupCtx, networkName); err != nil {
			t.Logf("cleanup: failed to remove network %s: %v", networkName, err)
		} else {
			t.Logf("cleanup: removed network %s", networkName)
		}
	})

	if err := a.StartProfile(multiEngineTestProfileID); err != nil {
		t.Fatalf("StartProfile() (mixed Postgres+Redis profile) failed: %v", err)
	}

	assertContainerReachesState(t, a, postgresContainerName, "running")
	assertContainerReachesState(t, a, redisContainerName, "running")
	assertTCPReachable(t, multiEngineTestPostgresPort)
	assertTCPReachable(t, multiEngineTestRedisPort)

	status, err := a.GetProfileStatus(multiEngineTestProfileID)
	if err != nil {
		t.Fatalf("GetProfileStatus() after StartProfile failed: %v", err)
	}
	if status != "running" {
		t.Fatalf("GetProfileStatus() = %q, want %q after starting both services", status, "running")
	}

	if err := a.StopProfile(multiEngineTestProfileID); err != nil {
		t.Fatalf("StopProfile() (mixed Postgres+Redis profile) failed: %v", err)
	}

	assertContainerReachesState(t, a, postgresContainerName, "exited")
	assertContainerReachesState(t, a, redisContainerName, "exited")

	status, err = a.GetProfileStatus(multiEngineTestProfileID)
	if err != nil {
		t.Fatalf("GetProfileStatus() after StopProfile failed: %v", err)
	}
	if status != "stopped" {
		t.Fatalf("GetProfileStatus() = %q, want %q after stopping both services", status, "stopped")
	}
}

func assertContainerReachesState(t *testing.T, a *App, containerName, wantState string) {
	t.Helper()

	deadline := time.Now().Add(30 * time.Second)
	var lastState string
	var lastErr error
	for time.Now().Before(deadline) {
		state, err := a.docker.ContainerState(a.ctx, containerName)
		if err != nil {
			lastErr = err
		} else {
			lastState = state
			if state == wantState {
				return
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatalf("container %q did not reach state %q within timeout (last state: %q, last error: %v)", containerName, wantState, lastState, lastErr)
}

func assertTCPReachable(t *testing.T, hostPort int) {
	t.Helper()

	addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(hostPort))
	deadline := time.Now().Add(30 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
		if err == nil {
			conn.Close()
			t.Logf("TCP dial to %s succeeded", addr)
			return
		}
		lastErr = err
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatalf("could not reach %s within timeout: %v", addr, lastErr)
}
