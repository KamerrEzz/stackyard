// Package storage owns Stackyard's own local application state — profiles,
// services, saved connections, snippets, and query history — persisted in a
// single SQLite file under the OS-standard app-data directory. It is
// intentionally distinct from internal/migrations (Phase 8): that package
// manages schema migrations for the user's *target* databases (Postgres/
// MySQL) they connect to through the DB Client, tracked inside those
// databases via a schema_migrations table. This package never touches a
// target database — it only manages Stackyard's own on-disk state.
package storage

import (
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite" // registers the "sqlite" driver
)

const (
	appDataDirName = "Stackyard"
	dbFileName     = "stackyard.db"
	driverName     = "sqlite"
)

// AppDataDir returns the OS-standard per-user application-data directory for
// Stackyard, creating it if it doesn't already exist.
//
// On Windows this resolves to "%APPDATA%\Stackyard" via os.UserConfigDir(),
// which reads the APPDATA environment variable — matching plan.md §4's
// documented path. On macOS/Linux, os.UserConfigDir() resolves to their
// respective platform conventions; this package doesn't special-case
// Windows beyond relying on the stdlib for cross-platform correctness.
func AppDataDir() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("storage: resolve user config dir: %w", err)
	}

	dir := filepath.Join(configDir, appDataDirName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("storage: create app data dir %q: %w", dir, err)
	}

	return dir, nil
}

// DBPath returns the full path to Stackyard's local SQLite database file,
// creating the containing directory if needed. The file itself is created
// lazily by the SQLite driver on first connection.
func DBPath() (string, error) {
	dir, err := AppDataDir()
	if err != nil {
		return "", err
	}

	return filepath.Join(dir, dbFileName), nil
}

// Open resolves the OS-standard app-data path, opens (creating if absent)
// the Stackyard SQLite database there, and applies the schema. It is safe
// to call on every app launch: schema creation is idempotent.
func Open() (*sql.DB, error) {
	path, err := DBPath()
	if err != nil {
		return nil, err
	}

	return OpenAt(path)
}

// OpenAt opens (creating if absent) a Stackyard SQLite database at the given
// file path and applies the schema. Exposed separately from Open so tests
// can point at a temporary file instead of the real app-data path.
func OpenAt(path string) (*sql.DB, error) {
	dsn := buildDSN(path)

	db, err := sql.Open(driverName, dsn)
	if err != nil {
		return nil, fmt.Errorf("storage: open sqlite database %q: %w", path, err)
	}

	// modernc.org/sqlite (like most CGO-free SQLite drivers) does not
	// gracefully multiplex concurrent writers across pooled connections;
	// a single connection avoids SQLITE_BUSY errors under this app's
	// single-process, single-writer usage pattern.
	db.SetMaxOpenConns(1)

	if err := migrate(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("storage: migrate database %q: %w", path, err)
	}

	return db, nil
}

// buildDSN encodes the connection-level PRAGMAs Stackyard depends on
// (foreign key enforcement, a busy timeout to tolerate brief lock
// contention) directly into the DSN, since SQLite PRAGMAs are per-connection
// and don't persist in the database file itself.
func buildDSN(path string) string {
	q := url.Values{}
	q.Add("_pragma", "busy_timeout(5000)")
	q.Add("_pragma", "foreign_keys(1)")

	return "file:" + filepath.ToSlash(path) + "?" + q.Encode()
}
