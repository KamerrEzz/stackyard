//go:build integration

// Integration test for tasks.md 2.8: confirms the real-time status
// dashboard's data source (App.buildStatusSnapshot) detects a service
// stopped OUTSIDE the app — via a direct `docker stop` CLI invocation,
// bypassing App.StopProfile/StopContainer entirely — within one poll
// (spec.md §3.5's second acceptance criterion: "reflects containers stopped/
// started outside the app... within one refresh cycle"). StartProfile (the
// app's own normal Postgres lifecycle) is used to bring the service up
// first, matching the raw-SQL-insert + real-Docker-Engine pattern already
// established in profile_multiengine_integration_test.go. Requires Docker
// Desktop/dockerd running AND the `docker` CLI on PATH; run with:
//
//	go test -tags=integration ./...
//
// Uses test ID 999009 — the next free ID per docs/STATE.md's registry
// (999001-999007 already taken as of Phase 2 wave 1/2.4; see
// profile_multiengine_integration_test.go's note). Host port 25999 is
// distinct from every other integration test's chosen port.
//
// This test deliberately never calls StartStatusWatcher/emitStatusSnapshot:
// those go through wailsruntime.EventsEmit, which calls log.Fatalf (an
// unrecoverable os.Exit, not a panic) if ctx doesn't carry the Wails
// runtime's internal "events" value — which no hand-built test App context
// ever does. buildStatusSnapshot is called directly instead, exercising the
// exact same live-Docker-read logic the watcher's goroutine uses every tick,
// without ever touching EventsEmit.
package main

import (
	"context"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"stackyard/internal/docker"
	"stackyard/internal/storage"
)

const (
	statusWatchTestProfileID int64 = 999009
	statusWatchTestServiceID int64 = 999009
	statusWatchTestHostPort        = 25999
)

func TestIntegration_BuildStatusSnapshot_DetectsExternalStop(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "stackyard-statuswatch-test.db")
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
		statusWatchTestProfileID, "status-watch-integration-test", now,
	); err != nil {
		t.Fatalf("insert test profile failed: %v", err)
	}

	username, password, dbName := "stackyard_test", "stackyard_test_pw", "stackyard_test_db"
	if _, err := db.Exec(
		`INSERT INTO services (id, profile_id, engine, image_tag, host_port, username, password_encrypted, db_name, volume_name)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		statusWatchTestServiceID, statusWatchTestProfileID, storage.EnginePostgres, "postgres:16-alpine", statusWatchTestHostPort,
		username, password, dbName, "stackyard-test-vol-2.8-postgres",
	); err != nil {
		t.Fatalf("insert test postgres service failed: %v", err)
	}

	containerName := docker.ServiceContainerName(statusWatchTestServiceID)
	networkName := docker.ProfileNetworkName(statusWatchTestProfileID)

	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()

		if err := dockerClient.RemoveContainer(cleanupCtx, containerName); err != nil {
			t.Logf("cleanup: failed to remove container %s: %v", containerName, err)
		} else {
			t.Logf("cleanup: removed container %s", containerName)
		}
		if err := dockerClient.RemoveVolume(cleanupCtx, "stackyard-test-vol-2.8-postgres"); err != nil {
			t.Logf("cleanup: failed to remove volume stackyard-test-vol-2.8-postgres: %v", err)
		}
		if err := dockerClient.RemoveNetwork(cleanupCtx, networkName); err != nil {
			t.Logf("cleanup: failed to remove network %s: %v", networkName, err)
		} else {
			t.Logf("cleanup: removed network %s", networkName)
		}
	})

	if err := a.StartProfile(statusWatchTestProfileID); err != nil {
		t.Fatalf("StartProfile() failed: %v", err)
	}
	assertContainerReachesState(t, a, containerName, "running")

	snapshot, err := a.buildStatusSnapshot(opCtx)
	if err != nil {
		t.Fatalf("buildStatusSnapshot() while running failed: %v", err)
	}
	svc := findServiceStatus(t, snapshot, statusWatchTestServiceID)
	if svc.State != "running" {
		t.Fatalf("service state = %q, want %q while the container is up", svc.State, "running")
	}
	if !svc.StatsAvailable {
		t.Fatalf("expected StatsAvailable = true for a running container")
	}
	if svc.MemoryUsageBytes == 0 {
		t.Fatalf("expected a plausible non-zero MemoryUsageBytes for a running Postgres container, got 0")
	}
	if svc.CPUPercent < 0 {
		t.Fatalf("expected a non-negative CPUPercent, got %f", svc.CPUPercent)
	}
	if svc.HostPort != statusWatchTestHostPort {
		t.Fatalf("HostPort = %d, want %d", svc.HostPort, statusWatchTestHostPort)
	}
	if svc.Engine != string(storage.EnginePostgres) || svc.EngineVersion != "postgres:16-alpine" {
		t.Fatalf("unexpected engine identity fields: %+v", svc)
	}

	stopCtx, stopCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer stopCancel()
	cmd := exec.CommandContext(stopCtx, "docker", "stop", containerName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("external `docker stop %s` failed: %v (output: %s)", containerName, err, output)
	}

	assertContainerReachesState(t, a, containerName, "exited")

	snapshot, err = a.buildStatusSnapshot(opCtx)
	if err != nil {
		t.Fatalf("buildStatusSnapshot() after external stop failed: %v", err)
	}
	svc = findServiceStatus(t, snapshot, statusWatchTestServiceID)
	if svc.State == "running" {
		t.Fatalf("service state = %q after an external `docker stop`, want a non-running state", svc.State)
	}
	if svc.StatsAvailable {
		t.Fatalf("expected StatsAvailable = false once the container is stopped")
	}
}

func findServiceStatus(t *testing.T, snapshot docker.EnvironmentStatusSnapshot, serviceID int64) docker.ServiceStatus {
	t.Helper()

	for _, profile := range snapshot.Profiles {
		for _, svc := range profile.Services {
			if svc.ServiceID == serviceID {
				return svc
			}
		}
	}
	t.Fatalf("service %d not found in snapshot: %+v", serviceID, snapshot)
	return docker.ServiceStatus{}
}
