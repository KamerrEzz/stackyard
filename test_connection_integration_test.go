//go:build integration

// Integration test for TestConnection (tasks.md 3.4): exercises the bound
// method end to end against real Postgres and MySQL containers started
// through internal/docker's own StartPostgresEnvironment/
// StartMySQLEnvironment (no bespoke container-launch code, no mocks).
// Requires Docker Desktop/dockerd running; run with:
//
//	go test -tags=integration ./...
//
// Uses test/profile/service IDs 999012 (Postgres) and 999013 (MySQL) —
// 999001-999011 are already taken across internal/docker's, the repo-root's,
// and internal/dbengine's integration tests (grepped for every 9990\d\d
// literal in the repo before picking these, per docs/STATE.md's running
// convention) — and host ports 15535 (Postgres) and 13308 (MySQL), distinct
// from every other integration test's port in this repo. Everything created
// is torn down in t.Cleanup so the test is fully self-cleaning and safely
// re-runnable.
package main

import (
	"context"
	"testing"
	"time"

	"stackyard/internal/docker"
	"stackyard/internal/storage"
)

const (
	testConnIntegrationPostgresProfileID int64 = 999012
	testConnIntegrationPostgresServiceID int64 = 999012
	testConnIntegrationPostgresHostPort        = 15535

	testConnIntegrationMySQLProfileID int64 = 999013
	testConnIntegrationMySQLServiceID int64 = 999013
	testConnIntegrationMySQLHostPort        = 13308
)

func TestIntegration_App_TestConnection_Postgres(t *testing.T) {
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
		ID:                testConnIntegrationPostgresServiceID,
		ProfileID:         testConnIntegrationPostgresProfileID,
		Engine:            storage.EnginePostgres,
		ImageTag:          "postgres:16-alpine",
		HostPort:          testConnIntegrationPostgresHostPort,
		Username:          &username,
		PasswordEncrypted: &password,
		DBName:            &dbName,
		VolumeName:        "stackyard-test-vol-app-testconnection-postgres",
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
		Port:     testConnIntegrationPostgresHostPort,
		Username: username,
		Password: password,
		Database: dbName,
	}

	if err := waitForTestConnection(t, a, fields, 90*time.Second); err != nil {
		t.Fatalf("TestConnection() never succeeded against the live Postgres container: %v", err)
	}
	t.Log("TestConnection() succeeded against the live Postgres container")

	wrongFields := fields
	wrongFields.Password = "wrong-password"
	if err := a.TestConnection(wrongFields); err == nil {
		t.Error("TestConnection() with the wrong password: expected an error, got nil")
	}
}

func TestIntegration_App_TestConnection_MySQL(t *testing.T) {
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
		ID:                testConnIntegrationMySQLServiceID,
		ProfileID:         testConnIntegrationMySQLProfileID,
		Engine:            storage.EngineMySQL,
		ImageTag:          "mysql:8",
		HostPort:          testConnIntegrationMySQLHostPort,
		Username:          &username,
		PasswordEncrypted: &password,
		DBName:            &dbName,
		VolumeName:        "stackyard-test-vol-app-testconnection-mysql",
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
		Host:     "127.0.0.1",
		Port:     testConnIntegrationMySQLHostPort,
		Username: username,
		Password: password,
		Database: dbName,
	}

	if err := waitForTestConnection(t, a, fields, 120*time.Second); err != nil {
		t.Fatalf("TestConnection() never succeeded against the live MySQL container: %v", err)
	}
	t.Log("TestConnection() succeeded against the live MySQL container")

	wrongFields := fields
	wrongFields.Password = "wrong-password"
	if err := a.TestConnection(wrongFields); err == nil {
		t.Error("TestConnection() with the wrong password: expected an error, got nil")
	}
}

func TestIntegration_App_TestConnection_UnreachableHostReturnsPromptly(t *testing.T) {
	a := &App{ctx: context.Background()}

	start := time.Now()
	err := a.TestConnection(ConnectionFormFields{
		Engine: storage.EnginePostgres,
		Host:   "127.0.0.1",
		Port:   1,
	})
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("TestConnection() against an unreachable port: expected an error, got nil")
	}
	if elapsed > testConnectionTimeout+10*time.Second {
		t.Errorf("TestConnection() took %v to fail, want it bounded near testConnectionTimeout (%v), not hanging", elapsed, testConnectionTimeout)
	}
}

// waitForTestConnection retries TestConnection until it succeeds or timeout
// elapses — freshly started database containers can take a few seconds
// before they accept connections, matching the retry-loop pattern
// internal/dbengine's own integration tests already use for Connect.
func waitForTestConnection(t *testing.T, a *App, fields ConnectionFormFields, timeout time.Duration) error {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		if err := a.TestConnection(fields); err == nil {
			return nil
		} else {
			lastErr = err
		}
		time.Sleep(1 * time.Second)
	}
	return lastErr
}
