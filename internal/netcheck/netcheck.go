// Package netcheck provides a real, OS-level TCP port-availability probe.
//
// This is deliberately separate from anything Docker- or storage-aware: it
// answers exactly one question — "can a listener be opened on this port
// right now?" — with no knowledge of Stackyard's own SQLite-recorded
// services and no knowledge of Docker containers. That keeps it trivially
// unit-testable (bind a port yourself in a test to simulate "taken") and
// reusable by both internal/docker (task 1.5's per-service conflict check,
// which layers container-state awareness on top of this) and app.go (task
// 1.5's SuggestFreePort scan, which layers Stackyard's own storage records
// on top of this).
//
// It is a genuinely stronger check than app.go's nextFreeHostPort helper,
// which only consults ports Stackyard itself has already handed out — this
// package instead asks the operating system directly, so it also catches a
// port held by something Stackyard has never heard of (another app, a
// manually-run `docker run`, a dev server left running, etc.).
package netcheck

import (
	"fmt"
	"net"
)

// IsPortFree reports whether a TCP listener can be opened on
// 127.0.0.1:<port> right now.
//
// A false result means something on this machine — not necessarily
// Stackyard, and not necessarily Docker — currently holds that port. A true
// result means the OS considers the port free at this instant; like any
// check-then-act port probe (including Docker's own bind-on-start), a
// remaining TOCTOU race is inherent and not something this function can
// close on its own.
func IsPortFree(port int) bool {
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return false
	}
	_ = ln.Close()
	return true
}
