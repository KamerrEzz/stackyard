package main

import (
	"context"
	"fmt"
	"time"

	"stackyard/internal/migrations"
	"stackyard/internal/storage"
)

// migrationsOperationTimeout bounds EnsureMigrationsTable's Exec round trip
// against the target database (tasks.md 8.2) — a single "CREATE TABLE IF
// NOT EXISTS" is cheap, so this shares gridOperationTimeout's budget rather
// than needing its own longer one.
const migrationsOperationTimeout = 10 * time.Second

// SetConnectionMigrationsFolder points connectionID's saved Connection at
// folder as its migrations folder (tasks.md 8.1, plan.md §4's "per-
// connection folder the user points the app at") and returns the updated
// Connection. This is the only way MigrationsFolder is ever written — see
// storage.SetConnectionMigrationsFolder's own doc comment.
func (a *App) SetConnectionMigrationsFolder(connectionID int64, folder string) (*storage.Connection, error) {
	db, err := a.requireDB()
	if err != nil {
		return nil, err
	}

	conn, err := storage.SetConnectionMigrationsFolder(db, connectionID, folder)
	if err != nil {
		return nil, fmt.Errorf("set connection migrations folder: %w", err)
	}
	return conn, nil
}

// CreateMigrationFile scaffolds a new timestamped up/down migration file
// pair (tasks.md 8.1) inside connectionID's configured migrations folder,
// naming the pair from name (see internal/migrations' package doc for the
// exact naming convention). Returns an error if connectionID has no
// migrations folder configured yet — see SetConnectionMigrationsFolder.
func (a *App) CreateMigrationFile(connectionID int64, name string) (*migrations.Migration, error) {
	folder, err := a.connectionMigrationsFolder(connectionID)
	if err != nil {
		return nil, fmt.Errorf("create migration file: %w", err)
	}

	m, err := migrations.CreateMigration(folder, name)
	if err != nil {
		return nil, fmt.Errorf("create migration file: %w", err)
	}
	return &m, nil
}

// ListMigrations discovers every migration file pair in connectionID's
// configured migrations folder, sorted by version (tasks.md 8.1). It
// reports no applied/pending state of its own — that distinction is
// tasks.md 8.3's ("Apply") job, built on top of this list.
func (a *App) ListMigrations(connectionID int64) ([]migrations.Migration, error) {
	folder, err := a.connectionMigrationsFolder(connectionID)
	if err != nil {
		return nil, fmt.Errorf("list migrations: %w", err)
	}

	found, err := migrations.DiscoverMigrations(folder)
	if err != nil {
		return nil, fmt.Errorf("list migrations: %w", err)
	}
	return found, nil
}

// EnsureMigrationsTable bootstraps the schema_migrations tracking table
// (tasks.md 8.2) inside the target database behind the open connection
// session sessionID, idempotently — safe to call every time the migrations
// panel is opened for that session, not just once.
func (a *App) EnsureMigrationsTable(sessionID string) error {
	session, ok := a.getQuerySession(sessionID)
	if !ok {
		return fmt.Errorf("ensure migrations table: no open connection session %q", sessionID)
	}

	ctx, cancel := context.WithTimeout(a.ctx, migrationsOperationTimeout)
	defer cancel()

	if err := migrations.BootstrapTrackingTable(ctx, session.engine); err != nil {
		return fmt.Errorf("ensure migrations table: %w", err)
	}
	return nil
}

// connectionMigrationsFolder resolves connectionID's saved Connection and
// returns its configured MigrationsFolder, or an error if either the
// connection doesn't exist or has no migrations folder set yet.
func (a *App) connectionMigrationsFolder(connectionID int64) (string, error) {
	db, err := a.requireDB()
	if err != nil {
		return "", err
	}

	conn, err := storage.GetConnection(db, connectionID)
	if err != nil {
		return "", fmt.Errorf("connection %d: %w", connectionID, err)
	}
	if conn.MigrationsFolder == nil || *conn.MigrationsFolder == "" {
		return "", fmt.Errorf("connection %d has no migrations folder configured", connectionID)
	}
	return *conn.MigrationsFolder, nil
}
