package netcheck

import (
	"fmt"
	"net"
	"testing"
)

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
