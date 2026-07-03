//go:build integration

// Integration test for mysql.go: exercises EnsureMySQLContainer (and the
// StartMySQLEnvironment convenience wrapper) against a live local Docker
// Engine — no mocks. Requires Docker Desktop/dockerd running; run with:
//
//	go test -tags=integration ./internal/docker/...
//
// Uses a distinctly-named profile/service and host port 13306, chosen to
// avoid colliding with this machine's unrelated zeew_* containers and with
// compose_integration_test.go's own 15432. Everything created is torn down
// in t.Cleanup so the test is fully self-cleaning and safely re-runnable.
package docker

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	"stackyard/internal/storage"

	"github.com/docker/docker/api/types/container"
)

func TestIntegration_StartMySQLEnvironment(t *testing.T) {
	c, err := NewClient()
	if err != nil {
		t.Fatalf("NewClient() failed: %v", err)
	}
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	if err := c.Ping(ctx); err != nil {
		t.Fatalf("Ping() failed to reach the local Docker Engine: %v", err)
	}

	const testProfileID int64 = 999004
	const testServiceID int64 = 999004
	const testHostPort = 13306

	username := "stackyard_test"
	password := "stackyard_test_pw"
	dbName := "stackyard_test_db"

	svc := storage.Service{
		ID:                testServiceID,
		ProfileID:         testProfileID,
		Engine:            storage.EngineMySQL,
		ImageTag:          "mysql:8",
		HostPort:          testHostPort,
		Username:          &username,
		PasswordEncrypted: &password,
		DBName:            &dbName,
		VolumeName:        "stackyard-test-vol-2.1",
	}

	networkName := ProfileNetworkName(svc.ProfileID)
	containerName := ServiceContainerName(svc.ID)

	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()

		if inspect, err := c.cli.ContainerInspect(cleanupCtx, containerName); err == nil {
			timeout := 5
			_ = c.cli.ContainerStop(cleanupCtx, inspect.ID, container.StopOptions{Timeout: &timeout})
			if err := c.cli.ContainerRemove(cleanupCtx, inspect.ID, container.RemoveOptions{Force: true}); err != nil {
				t.Logf("cleanup: failed to remove container %s: %v", containerName, err)
			} else {
				t.Logf("cleanup: removed container %s", containerName)
			}
		}

		if err := c.cli.VolumeRemove(cleanupCtx, svc.VolumeName, true); err != nil {
			t.Logf("cleanup: failed to remove volume %s: %v", svc.VolumeName, err)
		} else {
			t.Logf("cleanup: removed volume %s", svc.VolumeName)
		}

		if err := c.cli.NetworkRemove(cleanupCtx, networkName); err != nil {
			t.Logf("cleanup: failed to remove network %s: %v", networkName, err)
		} else {
			t.Logf("cleanup: removed network %s", networkName)
		}
	})

	if err := c.StartMySQLEnvironment(ctx, svc); err != nil {
		t.Fatalf("StartMySQLEnvironment() (first call, create path) failed: %v", err)
	}
	t.Logf("StartMySQLEnvironment: network %q, volume %q, container %q created/started", networkName, svc.VolumeName, containerName)

	assertNetworkExists(t, ctx, c, networkName)
	assertVolumeExists(t, ctx, c, svc.VolumeName)
	assertContainerRunning(t, ctx, c, containerName)
	assertMySQLReachable(t, testHostPort)

	if err := c.StartMySQLEnvironment(ctx, svc); err != nil {
		t.Fatalf("StartMySQLEnvironment() (second call, already-running reuse path) failed: %v", err)
	}
	assertContainerRunning(t, ctx, c, containerName)
	t.Logf("StartMySQLEnvironment: second call against already-running environment succeeded (reuse path)")

	existing, err := c.cli.ContainerInspect(ctx, containerName)
	if err != nil {
		t.Fatalf("ContainerInspect before stop-and-restart check failed: %v", err)
	}
	originalID := existing.ID

	timeout := 5
	if err := c.cli.ContainerStop(ctx, originalID, container.StopOptions{Timeout: &timeout}); err != nil {
		t.Fatalf("ContainerStop (simulating external stop) failed: %v", err)
	}

	if err := c.StartMySQLEnvironment(ctx, svc); err != nil {
		t.Fatalf("StartMySQLEnvironment() (third call, exists-but-stopped path) failed: %v", err)
	}
	assertContainerRunning(t, ctx, c, containerName)

	afterRestart, err := c.cli.ContainerInspect(ctx, containerName)
	if err != nil {
		t.Fatalf("ContainerInspect after restart failed: %v", err)
	}
	if afterRestart.ID != originalID {
		t.Errorf("container was recreated (ID changed from %s to %s) instead of being restarted in place", originalID, afterRestart.ID)
	} else {
		t.Logf("container %s (id=%s) was started in place after external stop, not recreated", containerName, originalID)
	}
	assertMySQLReachable(t, testHostPort)
}

func assertMySQLReachable(t *testing.T, hostPort int) {
	t.Helper()
	addr := fmt.Sprintf("127.0.0.1:%d", hostPort)
	deadline := time.Now().Add(90 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
		if err == nil {
			conn.Close()
			t.Logf("TCP dial to %s succeeded — MySQL port is reachable", addr)
			return
		}
		lastErr = err
		time.Sleep(1 * time.Second)
	}
	t.Fatalf("could not reach MySQL on %s within timeout: %v", addr, lastErr)
}
