//go:build integration

// Integration test for task 1.1: confirms real connectivity to the local
// Docker Engine, specifically validating Windows named-pipe transport
// access (plan.md §7's risk note). Requires a live local Docker Engine —
// run with: go test -tags=integration ./internal/docker/...
package docker

import (
	"context"
	"testing"
	"time"
)

func TestIntegration_ConnectAndListContainers(t *testing.T) {
	c, err := NewClient()
	if err != nil {
		t.Fatalf("NewClient() failed: %v", err)
	}
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := c.Ping(ctx); err != nil {
		t.Fatalf("Ping() failed to reach the local Docker Engine — is Docker Desktop/dockerd running? err: %v", err)
	}

	containers, err := c.ListContainers(ctx, true)
	if err != nil {
		t.Fatalf("ListContainers() failed: %v", err)
	}

	t.Logf("connected to local Docker Engine; found %d container(s):", len(containers))
	for _, ct := range containers {
		name := ct.ID
		if len(ct.Names) > 0 {
			name = ct.Names[0]
		}
		t.Logf("  - %s image=%s state=%s status=%s", name, ct.Image, ct.State, ct.Status)
	}
}
