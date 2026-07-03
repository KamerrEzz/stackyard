package main

import (
	"context"
	"fmt"
	"time"

	"stackyard/internal/dbengine"
)

// createTableOperationTimeout bounds CreateTable's single Exec round trip
// against the target database (tasks.md 10.2). A CREATE TABLE statement is
// a single, cheap DDL operation the same way EnsureMigrationsTable's is, so
// this shares migrationsOperationTimeout's budget rather than needing its
// own.
const createTableOperationTimeout = 10 * time.Second

// CreateTable builds a CREATE TABLE statement from table and columns via
// dbengine.BuildCreateTableDDL, using sessionID's dialect (PostgreSQL/MySQL
// only — the same relational-only scope BrowseTableRows/ApplyMigrations
// already enforce via gridSession/dialectForEngine), and executes it through
// the session's live Engine (tasks.md 10.2). This is the one bound method
// the "Create table" form (QueryEditor.tsx) calls; the frontend is
// responsible for refreshing its Tables list afterward (ListTablesForSession,
// the same call "Refresh schema" already makes) since this method only
// reports success or failure, not the new table's own metadata.
func (a *App) CreateTable(sessionID, schema, table string, columns []dbengine.ColumnDefinition) error {
	session, dialect, err := a.gridSession(sessionID)
	if err != nil {
		return fmt.Errorf("create table: %w", err)
	}

	ddl, err := dbengine.BuildCreateTableDDL(dialect, schema, table, columns)
	if err != nil {
		return fmt.Errorf("create table: %w", err)
	}

	ctx, cancel := context.WithTimeout(a.ctx, createTableOperationTimeout)
	defer cancel()

	start := time.Now()
	result, execErr := session.engine.Exec(ctx, ddl)
	a.recordQueryHistory(session.connectionID, ddl, time.Since(start), result, execErr)
	if execErr != nil {
		return fmt.Errorf("create table: %w", execErr)
	}
	return nil
}
