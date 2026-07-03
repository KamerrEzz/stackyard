package storage

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"
)

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()

	path := filepath.Join(t.TempDir(), "stackyard-test.db")

	db, err := OpenAt(path)
	if err != nil {
		t.Fatalf("OpenAt(%q) failed: %v", path, err)
	}
	t.Cleanup(func() { db.Close() })

	return db
}

func TestOpenAt_CreatesSchema(t *testing.T) {
	db := openTestDB(t)

	tables := []string{"profiles", "services", "connections", "snippets", "query_history"}
	for _, table := range tables {
		var name string
		err := db.QueryRow(
			`SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?`, table,
		).Scan(&name)
		if err != nil {
			t.Errorf("expected table %q to exist after OpenAt, got error: %v", table, err)
		}
	}
}

func TestOpenAt_IsIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "stackyard-idempotent.db")

	db1, err := OpenAt(path)
	if err != nil {
		t.Fatalf("first OpenAt failed: %v", err)
	}

	if _, err := CreateProfile(db1, "idempotency-check"); err != nil {
		t.Fatalf("seed insert failed: %v", err)
	}
	db1.Close()

	db2, err := OpenAt(path)
	if err != nil {
		t.Fatalf("second OpenAt on the same file failed: %v", err)
	}
	defer db2.Close()

	var count int
	if err := db2.QueryRow(`SELECT COUNT(*) FROM profiles`).Scan(&count); err != nil {
		t.Fatalf("count profiles after re-open: %v", err)
	}
	if count != 1 {
		t.Errorf("expected the seeded row to survive a second Open, got count=%d", count)
	}

	var version int
	if err := db2.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		t.Fatalf("read user_version: %v", err)
	}
	if version != len(schemaMigrations) {
		t.Errorf("expected user_version=%d after migrate, got %d", len(schemaMigrations), version)
	}
}

func TestOpenAt_EnforcesForeignKeys(t *testing.T) {
	db := openTestDB(t)

	_, err := db.Exec(
		`INSERT INTO services (profile_id, engine, image_tag, host_port, volume_name)
		 VALUES (?, ?, ?, ?, ?)`,
		999999, EnginePostgres, "postgres:16", 5432, "vol-orphan",
	)
	if err == nil {
		t.Fatal("expected inserting a service with a non-existent profile_id to fail with foreign keys enforced, but it succeeded")
	}
}

func TestAppDataDir_ResolvesAndCreatesDirectory(t *testing.T) {
	tempConfigDir := t.TempDir()
	t.Setenv("APPDATA", tempConfigDir)

	dir, err := AppDataDir()
	if err != nil {
		t.Fatalf("AppDataDir() failed: %v", err)
	}

	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("expected AppDataDir() to create %q, stat failed: %v", dir, err)
	}
	if !info.IsDir() {
		t.Fatalf("expected %q to be a directory", dir)
	}
	if filepath.Base(dir) != appDataDirName {
		t.Errorf("expected app data dir to be named %q, got %q", appDataDirName, filepath.Base(dir))
	}
}

func TestDBPath_PointsInsideAppDataDir(t *testing.T) {
	tempConfigDir := t.TempDir()
	t.Setenv("APPDATA", tempConfigDir)

	path, err := DBPath()
	if err != nil {
		t.Fatalf("DBPath() failed: %v", err)
	}

	if filepath.Base(path) != dbFileName {
		t.Errorf("expected db file name %q, got %q", dbFileName, filepath.Base(path))
	}

	expectedDir, err := AppDataDir()
	if err != nil {
		t.Fatalf("AppDataDir() failed: %v", err)
	}
	if filepath.Dir(path) != expectedDir {
		t.Errorf("expected DBPath() to live in %q, got %q", expectedDir, filepath.Dir(path))
	}
}
