package main

import (
	"net"
	"path/filepath"
	"testing"

	"stackyard/internal/storage"
)

func newTestApp(t *testing.T) *App {
	t.Helper()

	path := filepath.Join(t.TempDir(), "stackyard-test.db")
	db, err := storage.OpenAt(path)
	if err != nil {
		t.Fatalf("storage.OpenAt(%q) failed: %v", path, err)
	}
	t.Cleanup(func() { db.Close() })

	return &App{db: db}
}

func TestApp_CheckPortAvailable_DetectsTakenAndFreePorts(t *testing.T) {
	a := newTestApp(t)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to bind a test listener: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port

	available, err := a.CheckPortAvailable(port)
	if err != nil {
		t.Fatalf("CheckPortAvailable(%d) returned unexpected error: %v", port, err)
	}
	if available {
		t.Errorf("CheckPortAvailable(%d) = true, want false while a listener is bound to it", port)
	}

	if closeErr := ln.Close(); closeErr != nil {
		t.Fatalf("failed to release the listener: %v", closeErr)
	}

	available, err = a.CheckPortAvailable(port)
	if err != nil {
		t.Fatalf("CheckPortAvailable(%d) returned unexpected error: %v", port, err)
	}
	if !available {
		t.Errorf("CheckPortAvailable(%d) = false, want true immediately after releasing the only listener on it", port)
	}
}

func TestApp_SuggestFreePort_SkipsOSLevelTakenPort(t *testing.T) {
	a := newTestApp(t)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to bind a test listener: %v", err)
	}
	defer ln.Close()
	takenPort := ln.Addr().(*net.TCPAddr).Port

	got, err := a.SuggestFreePort(takenPort)
	if err != nil {
		t.Fatalf("SuggestFreePort(%d) returned unexpected error: %v", takenPort, err)
	}
	if got == takenPort {
		t.Errorf("SuggestFreePort(%d) = %d, want a port other than the one currently bound", takenPort, got)
	}

	if !a.mustCheckPortAvailable(t, got) {
		t.Errorf("SuggestFreePort(%d) suggested %d, which is not actually free", takenPort, got)
	}
}

func TestApp_SuggestFreePort_SkipsStorageRecordedPort(t *testing.T) {
	a := newTestApp(t)

	profile, err := storage.CreateProfile(a.db, "suggest-free-port-test")
	if err != nil {
		t.Fatalf("CreateProfile failed: %v", err)
	}

	recordedPort := findFreePort(t)

	username := "postgres"
	password := "postgres"
	dbName := "postgres"
	_, err = storage.CreateService(a.db, &storage.Service{
		ProfileID:         profile.ID,
		Engine:            storage.EnginePostgres,
		ImageTag:          "postgres:16-alpine",
		HostPort:          recordedPort,
		Username:          &username,
		PasswordEncrypted: &password,
		DBName:            &dbName,
		VolumeName:        "stackyard-vol-suggest-free-port-test",
	})
	if err != nil {
		t.Fatalf("CreateService failed: %v", err)
	}

	got, err := a.SuggestFreePort(recordedPort)
	if err != nil {
		t.Fatalf("SuggestFreePort(%d) returned unexpected error: %v", recordedPort, err)
	}
	if got == recordedPort {
		t.Errorf("SuggestFreePort(%d) = %d, want a port other than one already recorded by another Stackyard service", recordedPort, got)
	}
}

func TestApp_SuggestFreePort_RequiresDB(t *testing.T) {
	a := &App{}

	if _, err := a.SuggestFreePort(5432); err == nil {
		t.Error("SuggestFreePort() with no db configured: expected an error, got nil")
	}
}

func findFreePort(t *testing.T) int {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to get an OS-assigned port: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	if err := ln.Close(); err != nil {
		t.Fatalf("failed to release the listener: %v", err)
	}
	return port
}

func (a *App) mustCheckPortAvailable(t *testing.T, port int) bool {
	t.Helper()
	available, err := a.CheckPortAvailable(port)
	if err != nil {
		t.Fatalf("CheckPortAvailable(%d) returned unexpected error: %v", port, err)
	}
	return available
}
