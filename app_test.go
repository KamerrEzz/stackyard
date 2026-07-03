package main

import (
	"context"
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

func TestApp_DuplicateProfile_ReturnsNewSummaryWithServices(t *testing.T) {
	a := newTestApp(t)

	created, err := a.CreateProfile("dup-source")
	if err != nil {
		t.Fatalf("CreateProfile failed: %v", err)
	}

	dup, err := a.DuplicateProfile(created.Profile.ID)
	if err != nil {
		t.Fatalf("DuplicateProfile failed: %v", err)
	}

	if dup.Profile.ID == created.Profile.ID {
		t.Fatal("expected DuplicateProfile to return a new profile ID, not the original")
	}
	if dup.Profile.Name != "dup-source (copy)" {
		t.Errorf("expected duplicate name %q, got %q", "dup-source (copy)", dup.Profile.Name)
	}
	if len(dup.Services) != len(created.Services) {
		t.Fatalf("expected %d duplicated services, got %d", len(created.Services), len(dup.Services))
	}
	if dup.Services[0].ID == created.Services[0].ID {
		t.Error("expected duplicated service to have a new ID, not alias the original")
	}
}

func TestApp_DuplicateProfile_RequiresDB(t *testing.T) {
	a := &App{}

	if _, err := a.DuplicateProfile(1); err == nil {
		t.Error("DuplicateProfile() with no db configured: expected an error, got nil")
	}
}

func TestApp_RenameProfile_PersistsNewName(t *testing.T) {
	a := newTestApp(t)

	created, err := a.CreateProfile("before-rename")
	if err != nil {
		t.Fatalf("CreateProfile failed: %v", err)
	}

	renamed, err := a.RenameProfile(created.Profile.ID, "after-rename")
	if err != nil {
		t.Fatalf("RenameProfile failed: %v", err)
	}
	if renamed.Profile.Name != "after-rename" {
		t.Errorf("expected renamed profile name %q, got %q", "after-rename", renamed.Profile.Name)
	}

	fetched, err := a.ListProfiles()
	if err != nil {
		t.Fatalf("ListProfiles failed: %v", err)
	}
	if len(fetched) != 1 || fetched[0].Profile.Name != "after-rename" {
		t.Errorf("expected the rename to persist across ListProfiles, got %+v", fetched)
	}
}

func TestApp_RenameProfile_RequiresDB(t *testing.T) {
	a := &App{}

	if _, err := a.RenameProfile(1, "new-name"); err == nil {
		t.Error("RenameProfile() with no db configured: expected an error, got nil")
	}
}

func TestApp_DeleteProfile_RequiresDB(t *testing.T) {
	a := &App{}

	if err := a.DeleteProfile(1); err == nil {
		t.Error("DeleteProfile() with no db configured: expected an error, got nil")
	}
}

func TestDeleteProfileGuardError_BlocksWhenNotStopped(t *testing.T) {
	for _, status := range []string{"running", "partial", "unknown"} {
		if err := deleteProfileGuardError(1, status, nil); err == nil {
			t.Errorf("deleteProfileGuardError(status=%q) = nil, want a blocking error", status)
		}
	}
}

func TestDeleteProfileGuardError_BlocksWhenStatusCheckFailed(t *testing.T) {
	statusErr := context.DeadlineExceeded
	if err := deleteProfileGuardError(1, "stopped", statusErr); err == nil {
		t.Error("deleteProfileGuardError() with a status-check error = nil, want a blocking error")
	}
}

func TestDeleteProfileGuardError_AllowsWhenStopped(t *testing.T) {
	if err := deleteProfileGuardError(1, "stopped", nil); err != nil {
		t.Errorf("deleteProfileGuardError(status=%q) = %v, want nil", "stopped", err)
	}
}
