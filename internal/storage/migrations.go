package storage

import (
	"database/sql"
	"fmt"
)

type schemaMigration struct {
	version    int
	statements []string
}

var schemaMigrations = []schemaMigration{
	{
		version: 1,
		statements: []string{
			`CREATE TABLE IF NOT EXISTS profiles (
				id         INTEGER PRIMARY KEY AUTOINCREMENT,
				name       TEXT NOT NULL UNIQUE,
				created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
			)`,
			`CREATE TABLE IF NOT EXISTS services (
				id                 INTEGER PRIMARY KEY AUTOINCREMENT,
				profile_id         INTEGER NOT NULL REFERENCES profiles(id) ON DELETE CASCADE,
				engine             TEXT NOT NULL CHECK (engine IN ('postgres', 'mysql', 'mongodb', 'redis')),
				image_tag          TEXT NOT NULL,
				host_port          INTEGER NOT NULL,
				username           TEXT,
				password_encrypted TEXT,
				db_name            TEXT,
				volume_name        TEXT NOT NULL
			)`,
			`CREATE INDEX IF NOT EXISTS idx_services_profile_id ON services(profile_id)`,
			`CREATE TABLE IF NOT EXISTS connections (
				id                 INTEGER PRIMARY KEY AUTOINCREMENT,
				name               TEXT NOT NULL,
				engine             TEXT NOT NULL CHECK (engine IN ('postgres', 'mysql', 'mongodb', 'redis')),
				host               TEXT NOT NULL,
				port               INTEGER NOT NULL,
				username           TEXT,
				password_encrypted TEXT,
				database           TEXT,
				params_json        TEXT NOT NULL DEFAULT '{}',
				last_used_at       TEXT
			)`,
			`CREATE TABLE IF NOT EXISTS snippets (
				id            INTEGER PRIMARY KEY AUTOINCREMENT,
				connection_id INTEGER REFERENCES connections(id) ON DELETE SET NULL,
				engine        TEXT NOT NULL CHECK (engine IN ('postgres', 'mysql', 'mongodb', 'redis')),
				name          TEXT NOT NULL,
				body          TEXT NOT NULL,
				tags_json     TEXT NOT NULL DEFAULT '[]',
				created_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
				updated_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
			)`,
			`CREATE INDEX IF NOT EXISTS idx_snippets_connection_id ON snippets(connection_id)`,
			`CREATE TABLE IF NOT EXISTS query_history (
				id            INTEGER PRIMARY KEY AUTOINCREMENT,
				connection_id INTEGER NOT NULL REFERENCES connections(id) ON DELETE CASCADE,
				query_text    TEXT NOT NULL,
				executed_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
				duration_ms   INTEGER NOT NULL,
				success       INTEGER NOT NULL CHECK (success IN (0, 1)),
				rows_affected INTEGER NOT NULL DEFAULT 0,
				error_message TEXT
			)`,
			`CREATE INDEX IF NOT EXISTS idx_query_history_connection_id ON query_history(connection_id)`,
		},
	},
}

func migrate(db *sql.DB) error {
	var current int
	if err := db.QueryRow("PRAGMA user_version").Scan(&current); err != nil {
		return fmt.Errorf("read schema version: %w", err)
	}

	for _, m := range schemaMigrations {
		if m.version <= current {
			continue
		}

		if err := applyMigration(db, m); err != nil {
			return fmt.Errorf("apply schema version %d: %w", m.version, err)
		}
	}

	return nil
}

func applyMigration(db *sql.DB, m schemaMigration) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	for _, stmt := range m.statements {
		if _, err := tx.Exec(stmt); err != nil {
			return fmt.Errorf("exec statement: %w", err)
		}
	}

	if _, err := tx.Exec(fmt.Sprintf("PRAGMA user_version = %d", m.version)); err != nil {
		return fmt.Errorf("set schema version: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}

	return nil
}
