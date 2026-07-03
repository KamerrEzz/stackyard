// Package docker (this file, portcheck.go) implements the port-conflict
// pre-check: port conflicts must be detected before start and surfaced with
// a suggested free port, not a raw Docker error.
//
// This layers on top of internal/netcheck's plain OS-level probe rather than
// duplicating it: netcheck needs no Docker knowledge at all, and the only
// Docker-specific piece here is the exemption below — distinguishing "the
// port is bound by something else" (a real conflict) from "the port is
// bound because this exact service's own container is already running" (not
// a conflict). Without that exemption, every restart of an already-running
// profile would falsely report its own port as taken.
package docker

import (
	"context"
	"fmt"

	"stackyard/internal/netcheck"
)

// PortConflict is the result of checking whether a service's configured
// host port can be used to (re)start it right now.
type PortConflict struct {
	// Port is the host port that was checked.
	Port int
	// Conflict is true when the port is held by something OTHER than this
	// service's own container.
	Conflict bool
}

// CheckServicePortConflict reports whether port is free to bind for the
// service identified by containerName (see ServiceContainerName).
//
// It first asks the OS directly (internal/netcheck.IsPortFree). Only if the
// OS reports the port taken does it fall back to checking this service's own
// container state: if that container is already running, the port is
// legitimately its own and this is not a conflict.
func (c *Client) CheckServicePortConflict(ctx context.Context, port int, containerName string) (PortConflict, error) {
	if netcheck.IsPortFree(port) {
		return PortConflict{Port: port, Conflict: false}, nil
	}

	state, err := c.ContainerState(ctx, containerName)
	if err != nil {
		return PortConflict{}, fmt.Errorf("docker: check port conflict for %q: %w", containerName, err)
	}

	return PortConflict{Port: port, Conflict: evaluateConflict(state)}, nil
}

func evaluateConflict(containerState string) bool {
	return containerState != "running"
}
