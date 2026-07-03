package docker

import (
	"errors"
	"strings"
	"testing"
)

var sentinel = errors.New("boom")

func TestWrapConnectErr(t *testing.T) {
	err := wrapConnectErr(sentinel)
	if err == nil {
		t.Fatal("expected non-nil error")
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("wrapped error does not unwrap to sentinel: %v", err)
	}
	if !strings.Contains(err.Error(), "connect to engine") {
		t.Errorf("wrapped error message missing context: %q", err.Error())
	}
}

func TestWrapPingErr(t *testing.T) {
	err := wrapPingErr(sentinel)
	if err == nil {
		t.Fatal("expected non-nil error")
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("wrapped error does not unwrap to sentinel: %v", err)
	}
	if !strings.Contains(err.Error(), "ping engine") {
		t.Errorf("wrapped error message missing context: %q", err.Error())
	}
	if !strings.Contains(err.Error(), "Docker Desktop") {
		t.Errorf("wrapped error message missing actionable hint: %q", err.Error())
	}
}

func TestWrapListErr(t *testing.T) {
	err := wrapListErr(sentinel)
	if err == nil {
		t.Fatal("expected non-nil error")
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("wrapped error does not unwrap to sentinel: %v", err)
	}
	if !strings.Contains(err.Error(), "list containers") {
		t.Errorf("wrapped error message missing context: %q", err.Error())
	}
}

func TestNewClient_ConstructionOnly(t *testing.T) {
	c, err := NewClient()
	if err != nil {
		t.Fatalf("NewClient() construction failed: %v", err)
	}
	defer c.Close()
	if c.cli == nil {
		t.Fatal("expected non-nil underlying client")
	}
}

func TestClient_Close_NilSafe(t *testing.T) {
	var c *Client
	if err := c.Close(); err != nil {
		t.Errorf("Close on nil *Client should be a no-op, got: %v", err)
	}

	c2 := &Client{}
	if err := c2.Close(); err != nil {
		t.Errorf("Close on Client with nil cli should be a no-op, got: %v", err)
	}
}
