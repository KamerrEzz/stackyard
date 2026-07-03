package netcheck

import (
	"fmt"
	"net"
	"testing"
)

// TestIsPortFree_DetectsTakenPort binds a real listener ourselves (the test
// simulating "something on this machine already has this port") and
// confirms IsPortFree reports it as unavailable while that listener is
// still open.
func TestIsPortFree_DetectsTakenPort(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to bind a test listener: %v", err)
	}
	defer ln.Close()

	port := ln.Addr().(*net.TCPAddr).Port

	if IsPortFree(port) {
		t.Errorf("IsPortFree(%d) = true, want false while a listener is bound to it", port)
	}
}

// TestIsPortFree_ReportsGenuinelyFreePort picks a port by letting the OS
// assign one (bind to :0), immediately releases it, and confirms IsPortFree
// reports it free again — the "confirm a genuinely free port is reported
// available" half of task 1.5's required coverage.
func TestIsPortFree_ReportsGenuinelyFreePort(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to get an OS-assigned port: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	if err := ln.Close(); err != nil {
		t.Fatalf("failed to release the listener: %v", err)
	}

	if !IsPortFree(port) {
		t.Errorf("IsPortFree(%d) = false, want true immediately after releasing the only listener on it", port)
	}
}

// TestIsPortFree_SecondListenerOnSamePortFails is a sanity check that our
// "taken" simulation technique above is actually valid on this platform:
// attempting a second bind to the same address must fail while the first
// listener is open, otherwise TestIsPortFree_DetectsTakenPort wouldn't be
// testing what it claims to.
func TestIsPortFree_SecondListenerOnSamePortFails(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to bind first listener: %v", err)
	}
	defer ln.Close()

	port := ln.Addr().(*net.TCPAddr).Port
	_, err = net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err == nil {
		t.Fatalf("expected binding %d twice to fail", port)
	}
}
