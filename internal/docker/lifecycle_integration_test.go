//go:build integration

// Integration test for lifecycle.go: exercises ContainerState and
// StopContainer against a live local Docker Engine — no mocks. Requires
// Docker Desktop/dockerd running; run with:
//
//	go test -tags=integration ./internal/docker/...
//
// Uses host port 15435, distinct from compose_integration_test.go's 15432 so
// the two integration tests never collide, and distinct from this machine's
// unrelated zeew_* containers.
package docker

import (
	"context"
	"testing"
	"time"

	"stackyard/internal/storage"

	"github.com/docker/docker/api/types/container"
)

func TestIntegration_ContainerState_And_StopContainer(t *testing.T) {
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

	const testProfileID int64 = 999002
	const testServiceID int64 = 999002
	const testHostPort = 15435

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
		VolumeName:        "stackyard-test-vol-1.4",
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

	state, err := c.ContainerState(ctx, containerName)
	if err != nil {
		t.Fatalf("ContainerState() before creation failed: %v", err)
	}
	if state != "not_found" {
		t.Fatalf("ContainerState() before creation = %q, want %q", state, "not_found")
	}

	if err := c.StopContainer(ctx, containerName); err != nil {
		t.Fatalf("StopContainer() on a nonexistent container should be a no-op, got: %v", err)
	}

	if err := c.StartPostgresEnvironment(ctx, svc); err != nil {
		t.Fatalf("StartPostgresEnvironment() failed: %v", err)
	}
	assertContainerRunning(t, ctx, c, containerName)

	state, err = c.ContainerState(ctx, containerName)
	if err != nil {
		t.Fatalf("ContainerState() after start failed: %v", err)
	}
	if state != "running" {
		t.Fatalf("ContainerState() after start = %q, want %q", state, "running")
	}

	if err := c.StopContainer(ctx, containerName); err != nil {
		t.Fatalf("StopContainer() failed: %v", err)
	}

	state, err = c.ContainerState(ctx, containerName)
	if err != nil {
		t.Fatalf("ContainerState() after stop failed: %v", err)
	}
	if state == "running" {
		t.Fatalf("ContainerState() after stop = %q, want a non-running state", state)
	}
	t.Logf("container state after stop: %q", state)

	if err := c.StopContainer(ctx, containerName); err != nil {
		t.Fatalf("StopContainer() on an already-stopped container should be a no-op, got: %v", err)
	}
}
