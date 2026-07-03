// Package docker (this file, lifecycle.go) adds the stop/inspect-state half
// of container lifecycle management that compose.go's Ensure*/Start*
// functions don't cover.
//
// Both functions here are engine-agnostic: they only need a container name
// (see ServiceContainerName in compose.go), not a storage.Service, since
// stopping/inspecting a container never requires re-deriving its env vars,
// port bindings, or volume mounts.
package docker

import (
	"context"
	"fmt"
	"sync"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/errdefs"
)

// containerStopTimeoutSeconds is how long Docker waits for the container's
// own graceful-shutdown handling before forcibly killing it. 10s matches the
// Docker CLI's own default for `docker stop`.
const containerStopTimeoutSeconds = 10

// ContainerState reports a simplified lifecycle state for the named
// container:
//
//   - "running"   — the container exists and is currently running.
//   - "not_found" — no container with this name exists yet.
//   - anything else is the engine's own container.State.Status value
//     verbatim (e.g. "exited", "created", "paused", "dead", "restarting").
//
// Only a genuine Docker API failure (not "container doesn't exist") is
// returned as an error.
func (c *Client) ContainerState(ctx context.Context, name string) (string, error) {
	inspect, err := c.cli.ContainerInspect(ctx, name)
	if err != nil {
		if errdefs.IsNotFound(err) {
			return "not_found", nil
		}
		return "", wrapContainerInspectErr(err)
	}

	return containerStateFromInspect(inspect), nil
}

func containerStateFromInspect(inspect container.InspectResponse) string {
	if inspect.State == nil {
		return "unknown"
	}
	if inspect.State.Running {
		return "running"
	}
	return inspect.State.Status
}

// ContainerStatesForNames reports ContainerState for every name
// concurrently, mirroring StatsForContainers' one-call-per-container,
// no-partial-failure batching (stats.go): a container whose inspect fails is
// reported as "unknown" in its own entry rather than dropping it from the
// result or failing the whole batch, so the real-time status dashboard
// (tasks.md 2.8) can still show every other container's live state.
func (c *Client) ContainerStatesForNames(ctx context.Context, names []string) map[string]string {
	results := make(map[string]string, len(names))

	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, name := range names {
		wg.Add(1)
		go func(name string) {
			defer wg.Done()
			state, err := c.ContainerState(ctx, name)
			if err != nil {
				state = "unknown"
			}

			mu.Lock()
			defer mu.Unlock()
			results[name] = state
		}(name)
	}
	wg.Wait()

	return results
}

// StopContainer stops the named container if it exists and is running. It
// is a deliberate no-op — not an error — both when the container doesn't
// exist at all and when it exists but is already stopped.
func (c *Client) StopContainer(ctx context.Context, name string) error {
	inspect, err := c.cli.ContainerInspect(ctx, name)
	if err != nil {
		if errdefs.IsNotFound(err) {
			return nil
		}
		return wrapContainerInspectErr(err)
	}

	if inspect.State == nil || !inspect.State.Running {
		return nil
	}

	timeout := containerStopTimeoutSeconds
	if err := c.cli.ContainerStop(ctx, inspect.ID, container.StopOptions{Timeout: &timeout}); err != nil {
		return wrapContainerStopErr(err)
	}
	return nil
}

func wrapContainerStopErr(err error) error {
	return fmt.Errorf("docker: stop container: %w", err)
}
