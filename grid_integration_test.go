//go:build integration

// Integration test for the editable data grid's bound methods
// (BrowseTableRows/UpdateTableRow/InsertTableRow/DeleteTableRows, tasks.md
// 4.1-4.4): exercises each against a real Postgres AND a real MySQL
// container started through internal/docker's own
// Start<Engine>Environment (no mocks), proving cell edits/inserts/deletes
// actually change the real database, that constraint violations surface
// the database's own error text, that a PK-less table is refused with the
// documented read-only reason, and that DeleteTableRows deletes each row
// independently (one FK-blocked row among otherwise-successful ones).
//
// Requires Docker Desktop/dockerd running; run with:
//
//	go test -tags=integration ./...
//
// Uses test/profile/service IDs 999019 (Postgres) and 999020 (MySQL) —
// 999001-999018 are already taken across this repo's other integration
// tests (grepped for every 9990\d\d literal in the repo before picking
// these, per docs/STATE.md's running convention) — and host ports 15538
// (Postgres) and 13311 (MySQL), distinct from every other integration
// test's port in this repo. Everything created is torn down in
// t.Cleanup so each test is fully self-cleaning and safely re-runnable.
package main

import (
	"context"
	"strconv"
	"strings"
	"testing"
	"time"

	"stackyard/internal/docker"
	"stackyard/internal/storage"
)

const (
	gridIntegrationPostgresProfileID int64 = 999019
	gridIntegrationPostgresServiceID int64 = 999019
	gridIntegrationPostgresHostPort        = 15538

	gridIntegrationMySQLProfileID int64 = 999020
	gridIntegrationMySQLServiceID int64 = 999020
	gridIntegrationMySQLHostPort        = 13311
)

func TestIntegration_App_EditableGrid_Postgres(t *testing.T) {
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
		ID:                gridIntegrationPostgresServiceID,
		ProfileID:         gridIntegrationPostgresProfileID,
		Engine:            storage.EnginePostgres,
		ImageTag:          "postgres:16-alpine",
		HostPort:          gridIntegrationPostgresHostPort,
		Username:          &username,
		PasswordEncrypted: &password,
		DBName:            &dbName,
		VolumeName:        "stackyard-test-vol-app-grid-postgres",
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
		Port:     gridIntegrationPostgresHostPort,
		Username: username,
		Password: password,
		Database: dbName,
	}
	sessionID, err := waitForOpenConnection(t, a, fields, 90*time.Second)
	if err != nil {
		t.Fatalf("OpenConnection() never succeeded against the live Postgres container: %v", err)
	}

	if _, err := a.RunQuery(sessionID, `CREATE TABLE widgets (id SERIAL PRIMARY KEY, name TEXT NOT NULL UNIQUE, weight INT)`); err != nil {
		t.Fatalf("CREATE TABLE widgets failed: %v", err)
	}
	if _, err := a.RunQuery(sessionID, `CREATE TABLE widget_notes (id SERIAL PRIMARY KEY, widget_id INT NOT NULL REFERENCES widgets(id), note TEXT)`); err != nil {
		t.Fatalf("CREATE TABLE widget_notes failed: %v", err)
	}
	if _, err := a.RunQuery(sessionID, `CREATE TABLE readonly_widgets (name TEXT NOT NULL)`); err != nil {
		t.Fatalf("CREATE TABLE readonly_widgets failed: %v", err)
	}
	if _, err := a.RunQuery(sessionID, `INSERT INTO widgets (name, weight) VALUES ('bolt', 5), ('nut', 2), ('washer', 1)`); err != nil {
		t.Fatalf("seed INSERT failed: %v", err)
	}

	t.Run("BrowseTableRows returns paginated rows", func(t *testing.T) {
		result, err := a.BrowseTableRows(sessionID, "public", "widgets", 2, 0)
		if err != nil {
			t.Fatalf("BrowseTableRows() failed: %v", err)
		}
		if len(result.Rows) != 2 {
			t.Errorf("BrowseTableRows(limit=2) returned %d rows, want 2", len(result.Rows))
		}
	})

	t.Run("UpdateTableRow commits a real UPDATE", func(t *testing.T) {
		var boltID int64
		row, err := a.BrowseTableRows(sessionID, "public", "widgets", 10, 0)
		if err != nil {
			t.Fatalf("BrowseTableRows() failed: %v", err)
		}
		for _, r := range row.Rows {
			if r[1] == "bolt" {
				boltID = toInt64(r[0])
			}
		}
		if boltID == 0 {
			t.Fatal("could not find seeded 'bolt' row")
		}

		if err := a.UpdateTableRow(sessionID, "public", "widgets", map[string]any{"id": boltID}, "weight", 99); err != nil {
			t.Fatalf("UpdateTableRow() failed: %v", err)
		}

		result, err := a.RunQuery(sessionID, `SELECT weight FROM widgets WHERE name = 'bolt'`)
		if err != nil {
			t.Fatalf("verify SELECT failed: %v", err)
		}
		if len(result.Rows) != 1 || toInt64(result.Rows[0][0]) != 99 {
			t.Errorf("after UpdateTableRow, bolt.weight = %v, want 99", result.Rows)
		}
	})

	t.Run("UpdateTableRow on a PK-less table returns the read-only reason", func(t *testing.T) {
		err := a.UpdateTableRow(sessionID, "public", "readonly_widgets", map[string]any{"name": "x"}, "name", "y")
		if err == nil {
			t.Fatal("expected an error updating a PK-less table, got nil")
		}
		if !strings.Contains(err.Error(), "read-only: table has no primary key") {
			t.Errorf("UpdateTableRow() error = %q, want it to contain the read-only reason", err.Error())
		}
	})

	t.Run("UpdateTableRow surfaces the real constraint-violation error", func(t *testing.T) {
		result, err := a.RunQuery(sessionID, `SELECT id FROM widgets WHERE name = 'nut'`)
		if err != nil || len(result.Rows) != 1 {
			t.Fatalf("could not find 'nut' row: %v %v", result, err)
		}
		nutID := toInt64(result.Rows[0][0])

		err = a.UpdateTableRow(sessionID, "public", "widgets", map[string]any{"id": nutID}, "name", "bolt")
		if err == nil {
			t.Fatal("expected a unique-constraint violation renaming 'nut' to 'bolt', got nil")
		}
		if !strings.Contains(err.Error(), "duplicate key") && !strings.Contains(err.Error(), "unique") {
			t.Errorf("UpdateTableRow() error = %q, want it to surface Postgres's real unique-constraint error text", err.Error())
		}
	})

	t.Run("InsertTableRow inserts and re-reads via RETURNING", func(t *testing.T) {
		row, err := a.InsertTableRow(sessionID, "public", "widgets", map[string]any{"name": "gizmo", "weight": 7})
		if err != nil {
			t.Fatalf("InsertTableRow() failed: %v", err)
		}
		if row["name"] != "gizmo" {
			t.Errorf("InsertTableRow() row = %v, want name=gizmo", row)
		}
		if row["id"] == nil {
			t.Errorf("InsertTableRow() row = %v, want a generated id", row)
		}

		result, err := a.RunQuery(sessionID, `SELECT weight FROM widgets WHERE name = 'gizmo'`)
		if err != nil || len(result.Rows) != 1 || toInt64(result.Rows[0][0]) != 7 {
			t.Errorf("gizmo not queryable after InsertTableRow: result=%v err=%v", result, err)
		}
	})

	t.Run("InsertTableRow surfaces a NOT NULL violation", func(t *testing.T) {
		_, err := a.InsertTableRow(sessionID, "public", "widgets", map[string]any{"weight": 1})
		if err == nil {
			t.Fatal("expected a NOT NULL violation inserting a widget with no name, got nil")
		}
		if !strings.Contains(err.Error(), "null value") && !strings.Contains(err.Error(), "not-null") && !strings.Contains(err.Error(), "violates") {
			t.Errorf("InsertTableRow() error = %q, want it to surface Postgres's real NOT NULL error text", err.Error())
		}
	})

	t.Run("DeleteTableRows deletes independently, one FK-blocked row among successes", func(t *testing.T) {
		result, err := a.RunQuery(sessionID, `SELECT id, name FROM widgets ORDER BY id`)
		if err != nil {
			t.Fatalf("SELECT before delete failed: %v", err)
		}
		var washerID, gizmoID int64
		for _, r := range result.Rows {
			switch r[1] {
			case "washer":
				washerID = toInt64(r[0])
			case "gizmo":
				gizmoID = toInt64(r[0])
			}
		}
		if washerID == 0 || gizmoID == 0 {
			t.Fatalf("could not find washer/gizmo ids in %v", result.Rows)
		}

		if _, err := a.RunQuery(sessionID, `INSERT INTO widget_notes (widget_id, note) VALUES `+"("+itoa(washerID)+", 'blocked')"); err != nil {
			t.Fatalf("seed widget_notes failed: %v", err)
		}

		results, err := a.DeleteTableRows(sessionID, "public", "widgets", []map[string]any{
			{"id": gizmoID},
			{"id": washerID},
		})
		if err != nil {
			t.Fatalf("DeleteTableRows() failed: %v", err)
		}
		if len(results) != 2 {
			t.Fatalf("len(results) = %d, want 2", len(results))
		}
		if !results[0].Success {
			t.Errorf("results[0] (gizmo, no FK reference) = %+v, want Success", results[0])
		}
		if results[1].Success {
			t.Errorf("results[1] (washer, FK-referenced) = %+v, want Success == false", results[1])
		} else if !strings.Contains(results[1].ErrorMessage, "foreign key") && !strings.Contains(results[1].ErrorMessage, "violates") {
			t.Errorf("results[1].ErrorMessage = %q, want Postgres's real FK-violation text", results[1].ErrorMessage)
		}

		afterDelete, err := a.RunQuery(sessionID, `SELECT name FROM widgets WHERE name = 'gizmo'`)
		if err != nil {
			t.Fatalf("verify SELECT failed: %v", err)
		}
		if len(afterDelete.Rows) != 0 {
			t.Errorf("gizmo still present after a successful DeleteTableRows: %v", afterDelete.Rows)
		}

		stillThere, err := a.RunQuery(sessionID, `SELECT name FROM widgets WHERE name = 'washer'`)
		if err != nil {
			t.Fatalf("verify SELECT failed: %v", err)
		}
		if len(stillThere.Rows) != 1 {
			t.Errorf("washer should still be present since its delete was FK-blocked, got %v", stillThere.Rows)
		}
	})

	t.Run("DeleteTableRows on a PK-less table returns the read-only reason", func(t *testing.T) {
		_, err := a.DeleteTableRows(sessionID, "public", "readonly_widgets", []map[string]any{{"name": "x"}})
		if err == nil || !strings.Contains(err.Error(), "read-only: table has no primary key") {
			t.Errorf("DeleteTableRows() on a PK-less table error = %v, want the read-only reason", err)
		}
	})
}

func TestIntegration_App_EditableGrid_MySQL(t *testing.T) {
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
		ID:                gridIntegrationMySQLServiceID,
		ProfileID:         gridIntegrationMySQLProfileID,
		Engine:            storage.EngineMySQL,
		ImageTag:          "mysql:8",
		HostPort:          gridIntegrationMySQLHostPort,
		Username:          &username,
		PasswordEncrypted: &password,
		DBName:            &dbName,
		VolumeName:        "stackyard-test-vol-app-grid-mysql",
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

	if err := dockerClient.StartMySQLEnvironment(setupCtx, svc); err != nil {
		t.Fatalf("StartMySQLEnvironment() failed: %v", err)
	}
	t.Logf("StartMySQLEnvironment: network %q, volume %q, container %q created/started", networkName, svc.VolumeName, containerName)

	a := &App{ctx: context.Background()}
	fields := ConnectionFormFields{
		Engine:   storage.EngineMySQL,
		Host:     "localhost",
		Port:     gridIntegrationMySQLHostPort,
		Username: username,
		Password: password,
		Database: dbName,
	}
	sessionID, err := waitForOpenConnection(t, a, fields, 90*time.Second)
	if err != nil {
		t.Fatalf("OpenConnection() never succeeded against the live MySQL container: %v", err)
	}

	if _, err := a.RunQuery(sessionID, `CREATE TABLE widgets (id INT AUTO_INCREMENT PRIMARY KEY, name VARCHAR(255) NOT NULL UNIQUE, weight INT)`); err != nil {
		t.Fatalf("CREATE TABLE widgets failed: %v", err)
	}
	if _, err := a.RunQuery(sessionID, `CREATE TABLE widget_notes (id INT AUTO_INCREMENT PRIMARY KEY, widget_id INT NOT NULL, note VARCHAR(255), FOREIGN KEY (widget_id) REFERENCES widgets(id))`); err != nil {
		t.Fatalf("CREATE TABLE widget_notes failed: %v", err)
	}
	if _, err := a.RunQuery(sessionID, `CREATE TABLE readonly_widgets (name VARCHAR(255) NOT NULL)`); err != nil {
		t.Fatalf("CREATE TABLE readonly_widgets failed: %v", err)
	}
	if _, err := a.RunQuery(sessionID, `INSERT INTO widgets (name, weight) VALUES ('bolt', 5), ('nut', 2), ('washer', 1)`); err != nil {
		t.Fatalf("seed INSERT failed: %v", err)
	}

	t.Run("BrowseTableRows returns paginated rows", func(t *testing.T) {
		result, err := a.BrowseTableRows(sessionID, dbName, "widgets", 2, 0)
		if err != nil {
			t.Fatalf("BrowseTableRows() failed: %v", err)
		}
		if len(result.Rows) != 2 {
			t.Errorf("BrowseTableRows(limit=2) returned %d rows, want 2", len(result.Rows))
		}
	})

	t.Run("UpdateTableRow commits a real UPDATE", func(t *testing.T) {
		result, err := a.RunQuery(sessionID, `SELECT id FROM widgets WHERE name = 'bolt'`)
		if err != nil || len(result.Rows) != 1 {
			t.Fatalf("could not find 'bolt' row: %v %v", result, err)
		}
		boltID := toInt64(result.Rows[0][0])

		if err := a.UpdateTableRow(sessionID, dbName, "widgets", map[string]any{"id": boltID}, "weight", 99); err != nil {
			t.Fatalf("UpdateTableRow() failed: %v", err)
		}

		verify, err := a.RunQuery(sessionID, `SELECT weight FROM widgets WHERE name = 'bolt'`)
		if err != nil {
			t.Fatalf("verify SELECT failed: %v", err)
		}
		if len(verify.Rows) != 1 || toInt64(verify.Rows[0][0]) != 99 {
			t.Errorf("after UpdateTableRow, bolt.weight = %v, want 99", verify.Rows)
		}
	})

	t.Run("UpdateTableRow on a PK-less table returns the read-only reason", func(t *testing.T) {
		err := a.UpdateTableRow(sessionID, dbName, "readonly_widgets", map[string]any{"name": "x"}, "name", "y")
		if err == nil || !strings.Contains(err.Error(), "read-only: table has no primary key") {
			t.Errorf("UpdateTableRow() error = %v, want the read-only reason", err)
		}
	})

	t.Run("UpdateTableRow surfaces the real constraint-violation error", func(t *testing.T) {
		result, err := a.RunQuery(sessionID, `SELECT id FROM widgets WHERE name = 'nut'`)
		if err != nil || len(result.Rows) != 1 {
			t.Fatalf("could not find 'nut' row: %v %v", result, err)
		}
		nutID := toInt64(result.Rows[0][0])

		err = a.UpdateTableRow(sessionID, dbName, "widgets", map[string]any{"id": nutID}, "name", "bolt")
		if err == nil {
			t.Fatal("expected a unique-constraint violation renaming 'nut' to 'bolt', got nil")
		}
		if !strings.Contains(err.Error(), "Duplicate entry") {
			t.Errorf("UpdateTableRow() error = %q, want it to surface MySQL's real duplicate-entry error text", err.Error())
		}
	})

	t.Run("InsertTableRow inserts and re-selects by LastInsertID", func(t *testing.T) {
		row, err := a.InsertTableRow(sessionID, dbName, "widgets", map[string]any{"name": "gizmo", "weight": 7})
		if err != nil {
			t.Fatalf("InsertTableRow() failed: %v", err)
		}
		if row["name"] != "gizmo" {
			t.Errorf("InsertTableRow() row = %v, want name=gizmo", row)
		}
		if row["id"] == nil {
			t.Errorf("InsertTableRow() row = %v, want a generated id", row)
		}

		result, err := a.RunQuery(sessionID, `SELECT weight FROM widgets WHERE name = 'gizmo'`)
		if err != nil || len(result.Rows) != 1 || toInt64(result.Rows[0][0]) != 7 {
			t.Errorf("gizmo not queryable after InsertTableRow: result=%v err=%v", result, err)
		}
	})

	t.Run("InsertTableRow surfaces a NOT NULL violation", func(t *testing.T) {
		_, err := a.InsertTableRow(sessionID, dbName, "widgets", map[string]any{"weight": 1})
		if err == nil {
			t.Fatal("expected a NOT NULL violation inserting a widget with no name, got nil")
		}
		if !strings.Contains(err.Error(), "cannot be null") && !strings.Contains(err.Error(), "doesn't have a default value") {
			t.Errorf("InsertTableRow() error = %q, want it to surface MySQL's real NOT NULL error text", err.Error())
		}
	})

	t.Run("DeleteTableRows deletes independently, one FK-blocked row among successes", func(t *testing.T) {
		result, err := a.RunQuery(sessionID, `SELECT id, name FROM widgets ORDER BY id`)
		if err != nil {
			t.Fatalf("SELECT before delete failed: %v", err)
		}
		var washerID, gizmoID int64
		for _, r := range result.Rows {
			switch r[1] {
			case "washer":
				washerID = toInt64(r[0])
			case "gizmo":
				gizmoID = toInt64(r[0])
			}
		}
		if washerID == 0 || gizmoID == 0 {
			t.Fatalf("could not find washer/gizmo ids in %v", result.Rows)
		}

		if _, err := a.RunQuery(sessionID, `INSERT INTO widget_notes (widget_id, note) VALUES (`+itoa(washerID)+", 'blocked')"); err != nil {
			t.Fatalf("seed widget_notes failed: %v", err)
		}

		results, err := a.DeleteTableRows(sessionID, dbName, "widgets", []map[string]any{
			{"id": gizmoID},
			{"id": washerID},
		})
		if err != nil {
			t.Fatalf("DeleteTableRows() failed: %v", err)
		}
		if len(results) != 2 {
			t.Fatalf("len(results) = %d, want 2", len(results))
		}
		if !results[0].Success {
			t.Errorf("results[0] (gizmo, no FK reference) = %+v, want Success", results[0])
		}
		if results[1].Success {
			t.Errorf("results[1] (washer, FK-referenced) = %+v, want Success == false", results[1])
		} else if !strings.Contains(results[1].ErrorMessage, "foreign key constraint") {
			t.Errorf("results[1].ErrorMessage = %q, want MySQL's real FK-violation text", results[1].ErrorMessage)
		}

		afterDelete, err := a.RunQuery(sessionID, `SELECT name FROM widgets WHERE name = 'gizmo'`)
		if err != nil {
			t.Fatalf("verify SELECT failed: %v", err)
		}
		if len(afterDelete.Rows) != 0 {
			t.Errorf("gizmo still present after a successful DeleteTableRows: %v", afterDelete.Rows)
		}

		stillThere, err := a.RunQuery(sessionID, `SELECT name FROM widgets WHERE name = 'washer'`)
		if err != nil {
			t.Fatalf("verify SELECT failed: %v", err)
		}
		if len(stillThere.Rows) != 1 {
			t.Errorf("washer should still be present since its delete was FK-blocked, got %v", stillThere.Rows)
		}
	})

	t.Run("DeleteTableRows on a PK-less table returns the read-only reason", func(t *testing.T) {
		_, err := a.DeleteTableRows(sessionID, dbName, "readonly_widgets", []map[string]any{{"name": "x"}})
		if err == nil || !strings.Contains(err.Error(), "read-only: table has no primary key") {
			t.Errorf("DeleteTableRows() on a PK-less table error = %v, want the read-only reason", err)
		}
	})
}

// toInt64 normalizes a QueryResult cell value coming back from either
// driver (Postgres's pgx returns int32/int64 depending on column type;
// MySQL's driver, after this codebase's []byte-to-string conversion in
// internal/dbengine/mysql, returns numeric columns as strings) into int64
// for this test file's own row-identification/assertion logic.
func toInt64(v any) int64 {
	switch n := v.(type) {
	case int64:
		return n
	case int32:
		return int64(n)
	case int:
		return int64(n)
	case string:
		parsed, err := strconv.ParseInt(n, 10, 64)
		if err != nil {
			return 0
		}
		return parsed
	default:
		return 0
	}
}

// itoa formats an int64 as a plain decimal string, used by this test file
// to splice a previously-queried ID into a hand-written seed INSERT
// statement (test setup only — never how the grid's own bound methods
// build SQL, which always bind values as real parameters).
func itoa(n int64) string {
	return strconv.FormatInt(n, 10)
}
