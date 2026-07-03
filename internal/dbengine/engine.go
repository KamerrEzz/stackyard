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
	Columns      []ResultColumn
	Rows         [][]any
	RowsAffected int64
	Duration     time.Duration
}

// ResultColumn describes one column of a QueryResult (tasks.md 3.7),
// distinct from ColumnInfo below: ColumnInfo describes a table's column as
// reported by ListTables (schema metadata, including primary-key
// membership), while ResultColumn describes a column as reported by the
// driver for one executed statement's result set.
//
// Nullable is a tri-state: nil means the engine/driver does not report
// nullability for this column at all (this is always the case for
// Postgres — pgx's pgconn.FieldDescription carries no nullability bit, and
// resolving it would require a separate catalog query per column that
// conflates this method's job with ListTables', so it is deliberately left
// unknown rather than guessed); a non-nil pointer means the driver did
// report it (MySQL's database/sql ColumnType.Nullable, when its second
// return value is true).
type ResultColumn struct {
	Name         string
	DatabaseType string
	Nullable     *bool
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
