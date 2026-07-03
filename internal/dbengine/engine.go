// Package dbengine defines the engine-agnostic contract the DB Client
// module (spec.md Module 2) drives every relational/document connection
// through, plus the per-engine implementations (postgres/, mysql/, and
// later mongo/, redis/) that satisfy it. Callers depend on Engine, never
// on a specific driver package, so the tab/editor/grid UI in Phase 3-4
// works identically across engines.
package dbengine

import (
	"context"
	"time"
)

// Engine is satisfied by every supported database driver. A value is
// constructed already bound to one connection's parameters (host, port,
// credentials, database) by its engine-specific constructor; Connect
// performs the actual dial and Close releases it.
type Engine interface {
	// Connect establishes the connection. It must be called before Ping,
	// Query, ListSchemas, or ListTables.
	Connect(ctx context.Context) error

	// Ping confirms the connection is still reachable.
	Ping(ctx context.Context) error

	// Query executes a single statement and returns its result. Callers
	// cancel a running query by cancelling ctx (spec.md §4.6).
	Query(ctx context.Context, query string) (*QueryResult, error)

	// ListSchemas returns the schemas (Postgres) or databases (MySQL,
	// which has no separate schema concept) visible on this connection.
	ListSchemas(ctx context.Context) ([]string, error)

	// ListTables returns every table in the given schema, including
	// column metadata — rich enough for autocomplete (tasks.md 4.8)
	// without a later interface change.
	ListTables(ctx context.Context, schema string) ([]TableInfo, error)

	// Close releases the underlying connection/pool.
	Close() error
}

// QueryResult is the engine-agnostic shape of a single executed
// statement's outcome.
type QueryResult struct {
	Columns      []string
	Rows         [][]any
	RowsAffected int64
	Duration     time.Duration
}

// ColumnInfo describes one column of a table.
type ColumnInfo struct {
	Name         string
	DataType     string
	Nullable     bool
	IsPrimaryKey bool
}

// TableInfo describes one table and its columns.
type TableInfo struct {
	Name    string
	Columns []ColumnInfo
}
