// Package docker (this file, portcheck.go) implements task 1.5's real
// port-conflict pre-check: spec.md §3.2 requires "port conflicts are
// detected before start and surfaced with a suggested free port, not a raw
// Docker error."
//
// This deliberately layers on top of internal/netcheck's plain OS-level
// probe rather than duplicating it: netcheck needs no Docker knowledge at
// all, and the only Docker-specific piece here is the exemption below —
// distinguishing "the port is bound by something else" (a real conflict)
// from "the port is bound because this exact service's own container is
// already running" (the normal already-running case, NOT a conflict).
// Without that exemption, every restart/re-start-click of an
// already-running profile would falsely report its own port as taken.
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
	// service's own container — i.e. starting would fail with a raw Docker
	// bind error if attempted.
	Conflict bool
}

// CheckServicePortConflict reports whether port is free to bind for the
// service identified by containerName (see ServiceContainerName).
//
// It first asks the OS directly (internal/netcheck.IsPortFree) — the same
// real bind-based check Docker itself relies on when it starts a
// container's port mapping, so this catches a port held by anything on the
// machine, not just other Stackyard-recorded services (unlike app.go's
// storage-only nextFreeHostPort helper). Only if the OS reports the port
// taken does it fall back to checking THIS service's own container state:
// if that container is already running, the port is legitimately its own
// and this is not a conflict.
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

// evaluateConflict is the pure decision extracted from
// CheckServicePortConflict specifically so the "own already-running
// container is not a conflict" branch is unit-testable without a live
// Docker daemon (mirrors containerStateFromInspect's extraction in
// lifecycle.go for the same reason: keep the actual decision logic testable
// as a plain function, separate from the I/O that feeds it).
//
// It is only meaningful for a containerState obtained AFTER netcheck has
// already reported the port taken by something — a "running" state at that
// point means this exact service's own container is the one holding it
// (Docker itself bound the host port to that container), so it is exempted
// from being reported as a conflict. Any other state — "not_found",
// "exited", "created", "paused", "dead", "restarting", "unknown" — means
// this service's own container is NOT the one holding the port, so
// something else on the machine must be, and that is a genuine conflict.
func evaluateConflict(containerState string) bool {
	return containerState != "running"
}
