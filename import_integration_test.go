//go:build integration

// Integration test for the CSV/JSON importer (ValidateImportFile/ImportFile,
// tasks.md 7.4): exercises it against a real Postgres container started
// through internal/docker's own StartPostgresEnvironment (no mocks),
// proving the two acceptance criteria spec.md §4.9 actually requires — a
// file with one bad row among otherwise-good rows aborts BEFORE any row is
// written (zero rows land, not "all but the bad one"), and a genuinely valid
// file's rows land exactly as given, including the null-vs-empty-string
// convention documented in internal/importdata's package doc.
//
// Requires Docker Desktop/dockerd running; run with:
//
//	go test -tags=integration ./...
//
// Uses test/profile/service ID 999024 — 999001-999023 are already taken
// across this repo's other integration tests (grepped for every 9990\d\d
// literal in the repo before picking this one, per docs/STATE.md's running
// convention) — and host port 15539, distinct from every other integration
// test's port in this repo. Everything created is torn down in t.Cleanup so
// this test is fully self-cleaning and safely re-runnable.
package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"stackyard/internal/docker"
	"stackyard/internal/storage"
)

const (
	importIntegrationProfileID int64 = 999024
	importIntegrationServiceID int64 = 999024
	importIntegrationHostPort        = 15539
)

func TestIntegration_App_ImportData_Postgres(t *testing.T) {
	dockerClient, err := docker.NewClient()
	if err != nil {
		t.Fatalf("docker.NewClient() failed: %v", err)
	}
	defer dockerClient.Close()

	setupCtx, setupCancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer setupCancel()
	if err := dockerClient.Ping(setupCtx); err != nil {
		t.Fatalf("Ping() failed to reach the local Docker Engine: %v", err)
	}

	username := "stackyard_test"
	password := "stackyard_test_pw"
	dbName := "stackyard_test_db"

	svc := storage.Service{
		ID:                importIntegrationServiceID,
		ProfileID:         importIntegrationProfileID,
		Engine:            storage.EnginePostgres,
		ImageTag:          "postgres:16-alpine",
		HostPort:          importIntegrationHostPort,
		Username:          &username,
		PasswordEncrypted: &password,
		DBName:            &dbName,
		VolumeName:        "stackyard-test-vol-app-import-postgres",
	}

	networkName := docker.ProfileNetworkName(svc.ProfileID)
	containerName := docker.ServiceContainerName(svc.ID)
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()
		if err := dockerClient.RemoveContainer(cleanupCtx, containerName); err != nil {
			t.Logf("cleanup: failed to remove container %s: %v", containerName, err)
		}
		if err := dockerClient.RemoveVolume(cleanupCtx, svc.VolumeName); err != nil {
			t.Logf("cleanup: failed to remove volume %s: %v", svc.VolumeName, err)
		}
		if err := dockerClient.RemoveNetwork(cleanupCtx, networkName); err != nil {
			t.Logf("cleanup: failed to remove network %s: %v", networkName, err)
		}
	})

	if err := dockerClient.StartPostgresEnvironment(setupCtx, svc); err != nil {
		t.Fatalf("StartPostgresEnvironment() failed: %v", err)
	}
	t.Logf("StartPostgresEnvironment: network %q, volume %q, container %q created/started", networkName, svc.VolumeName, containerName)

	a := &App{ctx: context.Background()}
	fields := ConnectionFormFields{
		Engine:   storage.EnginePostgres,
		Host:     "localhost",
		Port:     importIntegrationHostPort,
		Username: username,
		Password: password,
		Database: dbName,
	}
	sessionID, err := waitForOpenConnection(t, a, fields, 90*time.Second)
	if err != nil {
		t.Fatalf("OpenConnection() never succeeded against the live Postgres container: %v", err)
	}

	if _, err := a.RunQuery(sessionID, `CREATE TABLE import_widgets (
		id INT PRIMARY KEY,
		name TEXT NOT NULL,
		weight NUMERIC,
		notes TEXT
	)`); err != nil {
		t.Fatalf("CREATE TABLE import_widgets failed: %v", err)
	}

	tmpDir := t.TempDir()

	t.Run("a bad row among good rows aborts before any row is written", func(t *testing.T) {
		badCSV := "id,name,weight,notes\n" +
			"1,bolt,5.5,\n" +
			"2,nut,not-a-number,\n" +
			"3,washer,1,\n"
		path := filepath.Join(tmpDir, "bad.csv")
		if err := os.WriteFile(path, []byte(badCSV), 0o644); err != nil {
			t.Fatalf("write bad.csv: %v", err)
		}

		validation, err := a.ValidateImportFile(sessionID, "public", "import_widgets", path)
		if err != nil {
			t.Fatalf("ValidateImportFile() failed: %v", err)
		}
		if len(validation.Mismatches) == 0 {
			t.Fatal("ValidateImportFile() reported zero mismatches, want a weight mismatch on row 2")
		}
		if validation.RowCount != 3 {
			t.Errorf("RowCount = %d, want 3", validation.RowCount)
		}

		commit, err := a.ImportFile(sessionID, "public", "import_widgets", path)
		if err != nil {
			t.Fatalf("ImportFile() failed: %v", err)
		}
		if len(commit.Mismatches) == 0 {
			t.Fatal("ImportFile() reported zero mismatches, want it to refuse to commit")
		}
		if commit.RowsInserted != 0 {
			t.Errorf("RowsInserted = %d, want 0 (abort-before-write)", commit.RowsInserted)
		}

		result, err := a.RunQuery(sessionID, `SELECT COUNT(*) FROM import_widgets`)
		if err != nil {
			t.Fatalf("verify SELECT COUNT(*) failed: %v", err)
		}
		count := toInt64(result.Rows[0][0])
		if count != 0 {
			t.Fatalf("row count after a rejected import = %d, want 0 (not \"all but the bad one\")", count)
		}
	})

	t.Run("a genuinely valid file commits every row exactly as given", func(t *testing.T) {
		goodCSV := "id,name,weight,notes\n" +
			"10,bolt,5.5,\n" +
			"11,nut,2,\"has a note\"\n" +
			"12,washer,,\"\"\n"
		path := filepath.Join(tmpDir, "good.csv")
		if err := os.WriteFile(path, []byte(goodCSV), 0o644); err != nil {
			t.Fatalf("write good.csv: %v", err)
		}

		validation, err := a.ValidateImportFile(sessionID, "public", "import_widgets", path)
		if err != nil {
			t.Fatalf("ValidateImportFile() failed: %v", err)
		}
		if len(validation.Mismatches) != 0 {
			t.Fatalf("ValidateImportFile() mismatches = %+v, want none for a genuinely valid file", validation.Mismatches)
		}

		commit, err := a.ImportFile(sessionID, "public", "import_widgets", path)
		if err != nil {
			t.Fatalf("ImportFile() failed: %v", err)
		}
		if len(commit.Mismatches) != 0 {
			t.Fatalf("ImportFile() mismatches = %+v, want none", commit.Mismatches)
		}
		if commit.RowsInserted != 3 {
			t.Fatalf("RowsInserted = %d, want 3", commit.RowsInserted)
		}

		result, err := a.RunQuery(sessionID, `SELECT id, name, weight, notes FROM import_widgets WHERE id >= 10 ORDER BY id`)
		if err != nil {
			t.Fatalf("verify SELECT failed: %v", err)
		}
		if len(result.Rows) != 3 {
			t.Fatalf("len(Rows) = %d, want 3", len(result.Rows))
		}

		boltNotes := result.Rows[0][3]
		if boltNotes != nil {
			t.Errorf("bolt.notes = %#v, want nil (unquoted empty CSV cell = NULL)", boltNotes)
		}

		washerWeight := result.Rows[2][2]
		if washerWeight != nil {
			t.Errorf("washer.weight = %#v, want nil (unquoted empty CSV cell = NULL)", washerWeight)
		}
		washerNotes := result.Rows[2][3]
		if washerNotes == nil || strings.TrimSpace(washerNotes.(string)) != "" {
			t.Errorf("washer.notes = %#v, want an empty string, not NULL (quoted \"\" cell)", washerNotes)
		}
	})
}
