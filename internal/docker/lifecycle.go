// Package docker (this file, lifecycle.go) adds the stop/inspect-state half
// of container lifecycle management that compose.go's Ensure*/Start*
// functions don't cover — task 1.4's "Stop" and "current status" bindings
// need a way to halt an existing container and to ask "is this container
// currently running?" without assuming compose.go's Postgres-specific
// create path.
//
// Both functions here are engine-agnostic: they only need a container name
// (see ServiceContainerName in compose.go), not a storage.Service, since
// stopping/inspecting a container never requires re-deriving its env vars,
// port bindings, or volume mounts the way creating one does. This means
// StopContainer/ContainerState work unchanged for any future engine
// (MySQL/MongoDB/Redis, Phase 2) that adopts the same
// "stackyard-service-<serviceID>" naming convention, even though only
// Postgres has a matching Ensure*Container/Start*Environment creation path
// today.
package docker

import (
	"context"
	"fmt"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/errdefs"
)

// containerStopTimeoutSeconds is how long Docker waits for the container's
// own graceful-shutdown handling (e.g. Postgres's SIGTERM handling) before
// forcibly killing it. 10s matches the Docker CLI's own default for
// `docker stop`.
const containerStopTimeoutSeconds = 10

// ContainerState reports a simplified lifecycle state for the named
// container:
//
//   - "running"   — the container exists and is currently running.
//   - "not_found" — no container with this name exists yet (e.g. the
//     profile has never been started).
//   - anything else is the engine's own container.State.Status value
//     verbatim (e.g. "exited", "created", "paused", "dead", "restarting"),
//     covering every non-running state without this package needing its own
//     enum that could drift from Docker's.
//
// Only a genuine Docker API failure (not "container doesn't exist") is
// returned as an error, so callers (e.g. App.GetProfileStatus) can treat
// ContainerState as a single source of truth without a separate existence
// check first.
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

// containerStateFromInspect maps an inspect result to the simplified state
// string ContainerState returns. Pulled out as a pure function (no Docker
// calls) so this branching logic — nil State, running vs. a specific
// Status value — is unit-testable without a live daemon, mirroring
// compose.go's buildPostgresContainerSpec extraction for the same reason.
func containerStateFromInspect(inspect container.InspectResponse) string {
	if inspect.State == nil {
		return "unknown"
	}
	if inspect.State.Running {
		return "running"
	}
	return inspect.State.Status
}

// StopContainer stops the named container if it exists and is running. It
// is a deliberate no-op — not an error — both when the container doesn't
// exist at all and when it exists but is already stopped, so callers (e.g.
// App.StopProfile) can call it unconditionally for every service in a
// profile without first checking state themselves.
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
