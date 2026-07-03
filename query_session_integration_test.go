//go:build integration

// Integration test for OpenConnection/RunQuery/CancelQuery/
// CloseConnectionSession (tasks.md 3.6): exercises the full session
// lifecycle against a real Postgres container started through
// internal/docker's own StartPostgresEnvironment (no mocks), proving
// RunQuery returns real query results and that CancelQuery genuinely
// aborts a slow, in-flight query server-side rather than merely flipping a
// UI flag — the query's wall-clock duration is measured and asserted to be
// far shorter than the full pg_sleep duration.
//
// Requires Docker Desktop/dockerd running; run with:
//
//	go test -tags=integration ./...
//
// Uses test/profile/service ID 999014 (999001-999013 are already taken —
// grepped for every 9990\d\d literal in the repo before picking this, per
// docs/STATE.md's running convention) and host port 15536, distinct from
// every other integration test's port in this repo. Everything created is
// torn down in t.Cleanup so the test is fully self-cleaning and safely
// re-runnable.
package main

import (
	"context"
	"strings"
	"testing"
	"time"

	"stackyard/internal/docker"
	"stackyard/internal/storage"
)

const (
	querySessionTestProfileID int64 = 999014
	querySessionTestServiceID int64 = 999014
	querySessionTestHostPort        = 15536
)

func TestIntegration_App_QuerySessionLifecycle_Postgres(t *testing.T) {
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
		ID:                querySessionTestServiceID,
		ProfileID:         querySessionTestProfileID,
		Engine:            storage.EnginePostgres,
		ImageTag:          "postgres:16-alpine",
		HostPort:          querySessionTestHostPort,
		Username:          &username,
		PasswordEncrypted: &password,
		DBName:            &dbName,
		VolumeName:        "stackyard-test-vol-app-querysession-postgres",
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
		Port:     querySessionTestHostPort,
		Username: username,
		Password: password,
		Database: dbName,
	}

	sessionID, err := waitForOpenConnection(t, a, fields, 90*time.Second)
	if err != nil {
		t.Fatalf("OpenConnection() never succeeded against the live Postgres container: %v", err)
	}
	t.Logf("OpenConnection() succeeded, session %q", sessionID)

	result, err := a.RunQuery(sessionID, "SELECT 1 AS one")
	if err != nil {
		t.Fatalf("RunQuery() failed: %v", err)
	}
	if len(result.Rows) != 1 || len(result.Columns) != 1 {
		t.Fatalf("RunQuery(\"SELECT 1 AS one\") = %+v, want exactly one row/column", result)
	}
	t.Logf("RunQuery(\"SELECT 1 AS one\") returned %+v", result)

	done := make(chan struct{})
	var slowErr error
	start := time.Now()
	go func() {
		defer close(done)
		_, slowErr = a.RunQuery(sessionID, "SELECT pg_sleep(30)")
	}()

	time.Sleep(500 * time.Millisecond)
	if err := a.CancelQuery(sessionID); err != nil {
		t.Fatalf("CancelQuery() failed: %v", err)
	}

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("RunQuery(pg_sleep(30)) did not return within 10s of CancelQuery being called — cancellation did not actually abort the query")
	}
	elapsed := time.Since(start)

	if slowErr == nil {
		t.Fatal("RunQuery(pg_sleep(30)) after CancelQuery: expected an error, got nil — the query should have been aborted, not completed successfully")
	}
	if elapsed >= 25*time.Second {
		t.Errorf("RunQuery(pg_sleep(30)) took %v to return after CancelQuery, want well under the full 30s sleep — cancellation did not genuinely abort the query server-side", elapsed)
	}
	t.Logf("RunQuery(pg_sleep(30)) aborted after %v (not the full 30s sleep), error: %v", elapsed, slowErr)

	if err := a.CloseConnectionSession(sessionID); err != nil {
		t.Fatalf("CloseConnectionSession() failed: %v", err)
	}

	if _, err := a.RunQuery(sessionID, "SELECT 1"); err == nil {
		t.Error("RunQuery() after CloseConnectionSession: expected an error, got nil")
	} else if !strings.Contains(err.Error(), sessionID) {
		t.Errorf("RunQuery() after close error = %q, want it to name the session ID", err.Error())
	}
}

// waitForOpenConnection retries OpenConnection until it succeeds or timeout
// elapses — a freshly started database container can take a few seconds
// before it accepts connections, matching the retry-loop pattern this
// repo's other integration tests already use for Connect/TestConnection.
func waitForOpenConnection(t *testing.T, a *App, fields ConnectionFormFields, timeout time.Duration) (string, error) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		id, err := a.OpenConnection(fields)
		if err == nil {
			return id, nil
		}
		lastErr = err
		time.Sleep(1 * time.Second)
	}
	return "", lastErr
}
