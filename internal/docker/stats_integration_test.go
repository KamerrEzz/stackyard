//go:build integration

// Integration test for stats.go: exercises ContainerStats and
// StatsForContainers against a live local Docker Engine — no mocks. Requires
// Docker Desktop/dockerd running; run with:
//
//	go test -tags=integration ./internal/docker/...
//
// Uses redis:7-alpine directly (bypassing compose.go's Postgres-specific
// helpers) since this test only needs a lightweight, fast-starting container
// to poll stats against, not a full profile/service environment. Named
// distinctly from the other integration tests' containers so it never
// collides with them.
package docker

import (
	"context"
	"testing"
	"time"

	"github.com/docker/docker/api/types/container"
)

func TestIntegration_ContainerStats(t *testing.T) {
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

	const containerName = "stackyard-test-stats-2.7"
	const imageTag = "redis:7-alpine"

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
	})

	if err := c.ensureImage(ctx, imageTag); err != nil {
		t.Fatalf("ensureImage(%q) failed: %v", imageTag, err)
	}

	created, err := c.cli.ContainerCreate(ctx, &container.Config{Image: imageTag}, nil, nil, nil, containerName)
	if err != nil {
		t.Fatalf("ContainerCreate() failed: %v", err)
	}
	if err := c.cli.ContainerStart(ctx, created.ID, container.StartOptions{}); err != nil {
		t.Fatalf("ContainerStart() failed: %v", err)
	}
	assertContainerRunning(t, ctx, c, containerName)

	usage, err := c.ContainerStats(ctx, containerName)
	if err != nil {
		t.Fatalf("ContainerStats() failed: %v", err)
	}
	t.Logf("ContainerStats(%q) = %+v", containerName, usage)

	if usage.CPUPercent < 0 {
		t.Errorf("CPUPercent = %v, want >= 0", usage.CPUPercent)
	}
	if usage.MemoryUsageBytes == 0 {
		t.Errorf("MemoryUsageBytes = 0, want a nonzero reading for a running redis container")
	}
	if usage.MemoryPercent < 0 {
		t.Errorf("MemoryPercent = %v, want >= 0", usage.MemoryPercent)
	}

	batch := c.StatsForContainers(ctx, []string{containerName, "stackyard-test-stats-does-not-exist"})

	present, ok := batch[containerName]
	if !ok {
		t.Fatalf("StatsForContainers() result missing entry for %q", containerName)
	}
	if present.Err != nil {
		t.Errorf("StatsForContainers() entry for %q returned error: %v", containerName, present.Err)
	}
	if present.Usage.MemoryUsageBytes == 0 {
		t.Errorf("StatsForContainers() entry for %q has zero MemoryUsageBytes", containerName)
	}

	missing, ok := batch["stackyard-test-stats-does-not-exist"]
	if !ok {
		t.Fatalf("StatsForContainers() result missing entry for the nonexistent container name")
	}
	if missing.Err == nil {
		t.Errorf("StatsForContainers() entry for a nonexistent container should carry an error, got nil")
	} else {
		t.Logf("StatsForContainers() correctly reported a per-container error for the missing container: %v", missing.Err)
	}
}
