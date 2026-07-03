//go:build integration

// Integration test for redis.go: exercises EnsureRedisContainer/
// StartRedisEnvironment against a live local Docker Engine — no mocks.
// Requires Docker Desktop/dockerd running; run with:
//
//	go test -tags=integration ./internal/docker/...
//
// Uses host port 16379, distinct from compose_integration_test.go's 15432
// and lifecycle_integration_test.go's 15435, and distinct from this
// machine's unrelated zeew_* containers (verified via `docker ps`/`docker
// port` before choosing it). Everything created is torn down in
// t.Cleanup so the test is fully self-cleaning and safely re-runnable.
package docker

import (
	"context"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"stackyard/internal/storage"

	"github.com/docker/docker/api/types/container"
)

func TestIntegration_StartRedisEnvironment(t *testing.T) {
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

	const testProfileID int64 = 999003
	const testServiceID int64 = 999003
	const testHostPort = 16379

	password := "stackyard_test_pw"

	svc := storage.Service{
		ID:                testServiceID,
		ProfileID:         testProfileID,
		Engine:            storage.EngineRedis,
		ImageTag:          "redis:7-alpine",
		HostPort:          testHostPort,
		PasswordEncrypted: &password,
		VolumeName:        "stackyard-test-vol-2.3",
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

	if err := c.StartRedisEnvironment(ctx, svc); err != nil {
		t.Fatalf("StartRedisEnvironment() (first call, create path) failed: %v", err)
	}
	t.Logf("StartRedisEnvironment: network %q, volume %q, container %q created/started", networkName, svc.VolumeName, containerName)

	assertNetworkExists(t, ctx, c, networkName)
	assertVolumeExists(t, ctx, c, svc.VolumeName)
	assertContainerRunning(t, ctx, c, containerName)
	assertRedisReachable(t, testHostPort)

	if err := c.StartRedisEnvironment(ctx, svc); err != nil {
		t.Fatalf("StartRedisEnvironment() (second call, already-running reuse path) failed: %v", err)
	}
	assertContainerRunning(t, ctx, c, containerName)
	t.Logf("StartRedisEnvironment: second call against already-running environment succeeded (reuse path)")

	existing, err := c.cli.ContainerInspect(ctx, containerName)
	if err != nil {
		t.Fatalf("ContainerInspect before stop-and-restart check failed: %v", err)
	}
	originalID := existing.ID

	timeout := 5
	if err := c.cli.ContainerStop(ctx, originalID, container.StopOptions{Timeout: &timeout}); err != nil {
		t.Fatalf("ContainerStop (simulating external stop) failed: %v", err)
	}

	if err := c.StartRedisEnvironment(ctx, svc); err != nil {
		t.Fatalf("StartRedisEnvironment() (third call, exists-but-stopped path) failed: %v", err)
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
	assertRedisReachable(t, testHostPort)
	assertRedisPing(t, testHostPort, password)
}

func assertRedisReachable(t *testing.T, hostPort int) {
	t.Helper()
	addr := formatHostPort(hostPort)
	deadline := time.Now().Add(30 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
		if err == nil {
			conn.Close()
			t.Logf("TCP dial to %s succeeded — Redis port is reachable", addr)
			return
		}
		lastErr = err
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatalf("could not reach Redis on %s within timeout: %v", addr, lastErr)
}

func assertRedisPing(t *testing.T, hostPort int, password string) {
	t.Helper()
	addr := formatHostPort(hostPort)

	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		t.Fatalf("dial %s for RESP AUTH/PING check failed: %v", addr, err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))

	authCmd := fmt.Sprintf("*2\r\n$4\r\nAUTH\r\n$%d\r\n%s\r\n", len(password), password)
	if _, err := conn.Write([]byte(authCmd)); err != nil {
		t.Fatalf("write AUTH command failed: %v", err)
	}
	authResp := make([]byte, 64)
	n, err := conn.Read(authResp)
	if err != nil {
		t.Fatalf("read AUTH response failed: %v", err)
	}
	if !strings.HasPrefix(string(authResp[:n]), "+OK") {
		t.Fatalf("AUTH response = %q, want prefix %q", string(authResp[:n]), "+OK")
	}

	if _, err := conn.Write([]byte("*1\r\n$4\r\nPING\r\n")); err != nil {
		t.Fatalf("write PING command failed: %v", err)
	}
	pingResp := make([]byte, 64)
	n, err = conn.Read(pingResp)
	if err != nil {
		t.Fatalf("read PING response failed: %v", err)
	}
	if !strings.HasPrefix(string(pingResp[:n]), "+PONG") {
		t.Fatalf("PING response = %q, want prefix %q", string(pingResp[:n]), "+PONG")
	}
	t.Logf("AUTH+PING against %s succeeded — container is a genuine password-protected Redis instance", addr)
}

func formatHostPort(hostPort int) string {
	return fmt.Sprintf("127.0.0.1:%d", hostPort)
}
