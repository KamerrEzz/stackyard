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

	// Exec executes a single statement exactly like Query — Columns/Rows are
	// populated for a statement that returns rows (a SELECT, or an INSERT
	// carrying Postgres's RETURNING clause), RowsAffected/LastInsertID
	// otherwise — except args are bound via the driver's own placeholder
	// syntax (pgx's numbered $1,$2,... for Postgres, go-sql-driver's ? for
	// MySQL) rather than being embedded in query itself. This is the
	// parameterized execution path the editable data grid (spec.md §4.3,
	// tasks.md 4.1-4.4) and multi-statement batch execution (ExecuteBatch)
	// require, so grid-supplied values are always bound as real query
	// parameters, never string-interpolated into query text.
	Exec(ctx context.Context, query string, args ...any) (*QueryResult, error)

	// ListSchemas returns the schemas (Postgres) or databases (MySQL,
	// which has no separate schema concept) visible on this connection.
	ListSchemas(ctx context.Context) ([]string, error)

	// ListTables returns every table in the given schema, including
	// column metadata — rich enough for autocomplete (tasks.md 4.8)
	// without a later interface change.
	ListTables(ctx context.Context, schema string) ([]TableInfo, error)

	// ListForeignKeys returns every foreign key constraint across every
	// table in the given schema (Postgres) or database (MySQL) — one call
	// per schema, mirroring ListTables' scope, rather than one call per
	// table. This is schema/relationship metadata for the schema-diagram
	// feature (spec.md §4.11, tasks.md 4.5.1); it is not used by the query
	// editor or grid.
	ListForeignKeys(ctx context.Context, schema string) ([]ForeignKey, error)

	// Close releases the underlying connection/pool.
	Close() error
}

// QueryResult is the engine-agnostic shape of a single executed
// statement's outcome.
type QueryResult struct {
	Columns      []ResultColumn
	Rows         [][]any
	RowsAffected int64

	// LastInsertID is populated by MySQL's Exec for an INSERT into a table
	// with a single auto-increment column, via the driver's own
	// sql.Result.LastInsertId(). It is always 0 for Postgres (RETURNING
	// covers that case directly, see InsertTableRow in app.go) and for any
	// MySQL statement that isn't an auto-increment INSERT.
	LastInsertID int64

	Duration time.Duration
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
//
// HasDefault reports whether the column carries a database-level DEFAULT
// (e.g. DEFAULT NOW(), DEFAULT 'pending', or an identity/serial sequence
// default) — it is a boolean signal only, not the default expression text
// itself, since the only consumer (buildInsertPayload in the frontend's
// db-client module) needs to decide whether to omit an untouched column
// from an INSERT so the database applies its own default, not what that
// default evaluates to.
type ColumnInfo struct {
	Name         string
	DataType     string
	Nullable     bool
	IsPrimaryKey bool
	HasDefault   bool
}

// TableInfo describes one table and its columns.
type TableInfo struct {
	Name    string
	Columns []ColumnInfo
}

// ForeignKey describes one foreign key constraint within a schema: the
// column ColumnName on TableName references ReferencedColumn on
// ReferencedTable. A composite foreign key (spanning more than one column)
// is reported as multiple ForeignKey values sharing the same TableName/
// ReferencedTable pair, one per column — schema-diagram rendering (tasks.md
// 4.5.2) treats each as its own relationship line rather than needing a
// dedicated composite-key representation.
type ForeignKey struct {
	TableName        string
	ColumnName       string
	ReferencedTable  string
	ReferencedColumn string
}
