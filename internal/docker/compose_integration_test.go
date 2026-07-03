//go:build integration

// Integration test for compose.go: exercises EnsureNetwork/EnsureVolume/
// EnsurePostgresContainer (and the StartPostgresEnvironment convenience
// wrapper) against a live local Docker Engine — no mocks. Requires Docker
// Desktop/dockerd running; run with:
//
//	go test -tags=integration ./internal/docker/...
//
// Uses a distinctly-named profile/service and host port 15432, chosen to
// avoid colliding with this machine's unrelated zeew_* containers (verified:
// zeew_postgres_dev uses host port 4103). Everything created is torn down in
// t.Cleanup so the test is fully self-cleaning and safely re-runnable.
package docker

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	"stackyard/internal/storage"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
)

func TestIntegration_StartPostgresEnvironment(t *testing.T) {
	c, err := NewClient()
	if err != nil {
		t.Fatalf("NewClient() failed: %v", err)
	}
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	if err := c.Ping(ctx); err != nil {
		t.Fatalf("Ping() failed to reach the local Docker Engine: %v", err)
	}

	const testProfileID int64 = 999001
	const testServiceID int64 = 999001
	const testHostPort = 15432

	username := "stackyard_test"
	password := "stackyard_test_pw"
	dbName := "stackyard_test_db"

	svc := storage.Service{
		ID:                testServiceID,
		ProfileID:         testProfileID,
		Engine:            storage.EnginePostgres,
		ImageTag:          "postgres:16-alpine",
		HostPort:          testHostPort,
		Username:          &username,
		PasswordEncrypted: &password,
		DBName:            &dbName,
		VolumeName:        "stackyard-test-vol-1.3",
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

	if err := c.StartPostgresEnvironment(ctx, svc); err != nil {
		t.Fatalf("StartPostgresEnvironment() (first call, create path) failed: %v", err)
	}
	t.Logf("StartPostgresEnvironment: network %q, volume %q, container %q created/started", networkName, svc.VolumeName, containerName)

	assertNetworkExists(t, ctx, c, networkName)
	assertVolumeExists(t, ctx, c, svc.VolumeName)
	assertContainerRunning(t, ctx, c, containerName)
	assertPostgresReachable(t, testHostPort)

	if err := c.StartPostgresEnvironment(ctx, svc); err != nil {
		t.Fatalf("StartPostgresEnvironment() (second call, already-running reuse path) failed: %v", err)
	}
	assertContainerRunning(t, ctx, c, containerName)
	t.Logf("StartPostgresEnvironment: second call against already-running environment succeeded (reuse path)")

	existing, err := c.cli.ContainerInspect(ctx, containerName)
	if err != nil {
		t.Fatalf("ContainerInspect before stop-and-restart check failed: %v", err)
	}
	originalID := existing.ID

	timeout := 5
	if err := c.cli.ContainerStop(ctx, originalID, container.StopOptions{Timeout: &timeout}); err != nil {
		t.Fatalf("ContainerStop (simulating external stop) failed: %v", err)
	}

	if err := c.StartPostgresEnvironment(ctx, svc); err != nil {
		t.Fatalf("StartPostgresEnvironment() (third call, exists-but-stopped path) failed: %v", err)
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
	assertPostgresReachable(t, testHostPort)
}

func assertNetworkExists(t *testing.T, ctx context.Context, c *Client, name string) {
	t.Helper()
	if _, err := c.cli.NetworkInspect(ctx, name, network.InspectOptions{}); err != nil {
		t.Fatalf("expected network %q to exist: %v", name, err)
	}
}

func assertVolumeExists(t *testing.T, ctx context.Context, c *Client, name string) {
	t.Helper()
	if _, err := c.cli.VolumeInspect(ctx, name); err != nil {
		t.Fatalf("expected volume %q to exist: %v", name, err)
	}
}

func assertContainerRunning(t *testing.T, ctx context.Context, c *Client, name string) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	var lastState string
	for time.Now().Before(deadline) {
		inspect, err := c.cli.ContainerInspect(ctx, name)
		if err != nil {
			t.Fatalf("expected container %q to exist: %v", name, err)
		}
		if inspect.State != nil {
			lastState = inspect.State.Status
			if inspect.State.Running {
				return
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatalf("container %q did not reach running state within timeout, last state: %q", name, lastState)
}

func assertPostgresReachable(t *testing.T, hostPort int) {
	t.Helper()
	addr := fmt.Sprintf("127.0.0.1:%d", hostPort)
	deadline := time.Now().Add(30 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
		if err == nil {
			conn.Close()
			t.Logf("TCP dial to %s succeeded — Postgres port is reachable", addr)
			return
		}
		lastErr = err
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatalf("could not reach Postgres on %s within timeout: %v", addr, lastErr)
}
