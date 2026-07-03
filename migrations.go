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

// migrationsApplyTimeout bounds ApplyMigrations and RollbackMigration's
// entire run (tasks.md 8.3-8.4), not just one statement — Apply may execute
// several pending migrations' SQL files in sequence, each an arbitrary DDL
// statement of unknown cost, so it shares exportOperationTimeout's longer
// "bulk operation" budget rather than migrationsOperationTimeout's 10s
// budget (sized for EnsureMigrationsTable's single cheap statement).
const migrationsApplyTimeout = 5 * time.Minute

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

// sessionMigrationsFolder resolves session's configured migrations folder
// via its connectionID, mirroring connectionMigrationsFolder but starting
// from an already-open querySession — ApplyMigrations/RollbackMigration take
// a sessionID rather than a bare connectionID (matching EnsureMigrationsTable's
// existing precedent) since both also need the session's live engine to run
// SQL against, not just the folder path. Returns an error if session was
// opened ad-hoc (session.connectionID == nil, e.g. straight from "Test
// connection" fields that were never saved), since there is no persisted
// Connection row to read a folder from in that case.
func (a *App) sessionMigrationsFolder(session *querySession) (string, error) {
	if session.connectionID == nil {
		return "", fmt.Errorf("this session has no saved connection, so no migrations folder is configured")
	}
	return a.connectionMigrationsFolder(*session.connectionID)
}

// ApplyMigrations discovers every migration configured for sessionID's
// connection, runs every pending one's up.sql content in ascending version
// order, and records each as applied in schema_migrations (tasks.md 8.3,
// spec.md §4.8) — see migrations.Apply's own doc comment for the exact
// atomicity and stop-at-first-failure guarantees. sessionID must already be
// open via OpenConnection against a saved PostgreSQL or MySQL connection
// (dialectForEngine rejects Mongo/Redis sessions, since migrations are
// relational-only). The returned *migrations.ApplyResult reports which
// migrations applied and, if one failed partway, which one and its error
// text — ApplyMigrations itself only returns a non-nil error for a failure
// outside any single migration's own SQL (no open session, no configured
// folder, or the connection not being PostgreSQL/MySQL).
func (a *App) ApplyMigrations(sessionID string) (*migrations.ApplyResult, error) {
	session, ok := a.getQuerySession(sessionID)
	if !ok {
		return nil, fmt.Errorf("apply migrations: no open connection session %q", sessionID)
	}

	dialect, err := dialectForEngine(session.engineType)
	if err != nil {
		return nil, fmt.Errorf("apply migrations: %w", err)
	}

	folder, err := a.sessionMigrationsFolder(session)
	if err != nil {
		return nil, fmt.Errorf("apply migrations: %w", err)
	}

	ctx, cancel := context.WithTimeout(a.ctx, migrationsApplyTimeout)
	defer cancel()

	result, err := migrations.Apply(ctx, session.engine, dialect, folder)
	if err != nil {
		return nil, fmt.Errorf("apply migrations: %w", err)
	}
	return result, nil
}

// RollbackMigration reverts exactly the most-recently-applied migration for
// sessionID's connection — one step per call, never more (tasks.md 8.4,
// spec.md §4.8's "no bulk rollback in v1") — running its down.sql content
// and removing its schema_migrations tracking row atomically, with the same
// guarantees migrations.Rollback documents. Returns (nil, nil), not an
// error, when nothing has been applied yet: the frontend should render that
// case as "nothing to roll back" rather than a red error banner, the same
// distinction migrations.Rollback's own doc comment explains.
func (a *App) RollbackMigration(sessionID string) (*migrations.Migration, error) {
	session, ok := a.getQuerySession(sessionID)
	if !ok {
		return nil, fmt.Errorf("rollback migration: no open connection session %q", sessionID)
	}

	dialect, err := dialectForEngine(session.engineType)
	if err != nil {
		return nil, fmt.Errorf("rollback migration: %w", err)
	}

	folder, err := a.sessionMigrationsFolder(session)
	if err != nil {
		return nil, fmt.Errorf("rollback migration: %w", err)
	}

	ctx, cancel := context.WithTimeout(a.ctx, migrationsApplyTimeout)
	defer cancel()

	reverted, err := migrations.Rollback(ctx, session.engine, dialect, folder)
	if err != nil {
		return nil, fmt.Errorf("rollback migration: %w", err)
	}
	return reverted, nil
}
