package storage

import (
	"database/sql"
	"path/filepath"
	"testing"
)

func TestMigrate_FreshDatabaseHasMigrationsFolderColumn(t *testing.T) {
	db := openTestDB(t)

	if _, err := CreateConnection(db, &Connection{Name: "fresh", Engine: EnginePostgres, Host: "localhost", Port: 5432}); err != nil {
		t.Fatalf("CreateConnection failed: %v", err)
	}

	if !hasColumn(t, db, "connections", "migrations_folder") {
		t.Error("expected a fresh database's connections table to already have a migrations_folder column")
	}
}

func TestMigrate_UpgradesExistingV1DatabaseCleanly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "stackyard-v1.db")

	db, err := sql.Open(driverName, buildDSN(path))
	if err != nil {
		t.Fatalf("open raw sqlite db: %v", err)
	}

	// Apply only the very first schema migration by hand, simulating a
	// database created by an older Stackyard build that predates
	// migrations_folder (schemaMigrations version 2).
	if err := applyMigration(db, schemaMigrations[0]); err != nil {
		t.Fatalf("apply v1 migration: %v", err)
	}

	if _, err := db.Exec(
		`INSERT INTO connections (name, engine, host, port, params_json) VALUES (?, ?, ?, ?, ?)`,
		"pre-existing", EnginePostgres, "localhost", 5432, "{}",
	); err != nil {
		t.Fatalf("seed pre-upgrade connection row: %v", err)
	}
	db.Close()

	upgraded, err := OpenAt(path)
	if err != nil {
		t.Fatalf("OpenAt on a v1 database failed to upgrade: %v", err)
	}
	defer upgraded.Close()

	var version int
	if err := upgraded.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		t.Fatalf("read user_version: %v", err)
	}
	if version != len(schemaMigrations) {
		t.Errorf("expected user_version=%d after upgrade, got %d", len(schemaMigrations), version)
	}

	if !hasColumn(t, upgraded, "connections", "migrations_folder") {
		t.Fatal("expected the upgraded database's connections table to have a migrations_folder column")
	}

	connections, err := ListConnections(upgraded)
	if err != nil {
		t.Fatalf("ListConnections after upgrade failed: %v", err)
	}
	if len(connections) != 1 || connections[0].Name != "pre-existing" {
		t.Fatalf("expected the pre-existing row to survive the upgrade, got %+v", connections)
	}
	if connections[0].MigrationsFolder != nil {
		t.Errorf("expected a pre-existing row's new migrations_folder column to default to NULL, got %v", *connections[0].MigrationsFolder)
	}
}

func TestMigrate_UpgradeIsIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "stackyard-idempotent-upgrade.db")

	db1, err := OpenAt(path)
	if err != nil {
		t.Fatalf("first OpenAt failed: %v", err)
	}
	db1.Close()

	db2, err := OpenAt(path)
	if err != nil {
		t.Fatalf("second OpenAt on an already-current database failed: %v", err)
	}
	defer db2.Close()

	var version int
	if err := db2.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		t.Fatalf("read user_version: %v", err)
	}
	if version != len(schemaMigrations) {
		t.Errorf("expected user_version=%d, got %d", len(schemaMigrations), version)
	}
}

func hasColumn(t *testing.T, db *sql.DB, table, column string) bool {
	t.Helper()

	rows, err := db.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		t.Fatalf("PRAGMA table_info(%s): %v", table, err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			cid       int
			name      string
			ctype     string
			notnull   int
			dfltValue sql.NullString
			pk        int
		)
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dfltValue, &pk); err != nil {
			t.Fatalf("scan table_info row: %v", err)
		}
		if name == column {
			return true
		}
	}
	return false
}
