// Package postgres implements dbengine.Engine for PostgreSQL using pgx/v5's
// connection pool (pgxpool). A *Engine value is constructed already bound to
// one connection string via New; Connect performs the actual pool creation
// and an initial ping — construction itself never dials, matching
// dbengine.Engine's documented contract.
package postgres

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"stackyard/internal/dbengine"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

var _ dbengine.Engine = (*Engine)(nil)

// ErrNotConnected is returned by Ping, Query, ListSchemas, and ListTables
// when called before a successful Connect.
var ErrNotConnected = errors.New("postgres: not connected")

// Engine implements dbengine.Engine for PostgreSQL via pgxpool.
type Engine struct {
	connString string
	pool       *pgxpool.Pool
}

// New returns an Engine bound to connString — a standard "postgres://" URL
// or a libpq key=value string, anything pgxpool.ParseConfig accepts. It does
// not dial; call Connect to establish the pool.
func New(connString string) *Engine {
	return &Engine{connString: connString}
}

// Connect creates the connection pool and confirms it is reachable with an
// initial ping. Calling Connect again after a prior successful call closes
// the existing pool before replacing it.
func (e *Engine) Connect(ctx context.Context) error {
	pool, err := pgxpool.New(ctx, e.connString)
	if err != nil {
		return fmt.Errorf("postgres: connect: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return fmt.Errorf("postgres: connect: initial ping: %w", err)
	}
	if e.pool != nil {
		e.pool.Close()
	}
	e.pool = pool
	return nil
}

// Ping confirms the connection is still reachable.
func (e *Engine) Ping(ctx context.Context) error {
	if e.pool == nil {
		return ErrNotConnected
	}
	if err := e.pool.Ping(ctx); err != nil {
		return fmt.Errorf("postgres: ping: %w", err)
	}
	return nil
}

// Query executes a single statement. For a statement that returns rows
// (e.g. SELECT), Columns and Rows are populated; for one that doesn't
// (INSERT/UPDATE/DELETE/DDL), RowsAffected is populated from Postgres's
// command tag instead — pgx's Query plus CommandTag cover both shapes
// through one call path, unlike database/sql's split Query/Exec model (see
// the mysql package). ctx governs the query's lifetime end to end:
// cancelling it or letting it time out makes pgx send Postgres a real
// cancel request that aborts the statement server-side, not just a
// client-side give-up. Every returned ResultColumn.Nullable is always nil:
// pgx's pgconn.FieldDescription carries no nullability bit, and resolving
// it would require a separate per-column catalog query that conflates this
// method's job with ListTables' — a deliberate, documented known
// limitation, not an oversight.
func (e *Engine) Query(ctx context.Context, query string) (*dbengine.QueryResult, error) {
	if e.pool == nil {
		return nil, ErrNotConnected
	}

	start := time.Now()
	rows, err := e.pool.Query(ctx, query)
	if err != nil {
		return nil, translatePgError("query", err)
	}
	defer rows.Close()

	fieldDescriptions := rows.FieldDescriptions()
	columns := make([]dbengine.ResultColumn, len(fieldDescriptions))
	for i, fd := range fieldDescriptions {
		columns[i] = dbengine.ResultColumn{
			Name:         fd.Name,
			DatabaseType: resolveTypeName(fd.DataTypeOID),
		}
	}

	var resultRows [][]any
	for rows.Next() {
		values, err := rows.Values()
		if err != nil {
			return nil, translatePgError("read row", err)
		}
		resultRows = append(resultRows, values)
	}
	if err := rows.Err(); err != nil {
		return nil, translatePgError("row iteration", err)
	}

	tag := rows.CommandTag()
	duration := time.Since(start)

	if len(columns) == 0 {
		return &dbengine.QueryResult{RowsAffected: tag.RowsAffected(), Duration: duration}, nil
	}
	return &dbengine.QueryResult{Columns: columns, Rows: resultRows, Duration: duration}, nil
}

const listSchemasQuery = `
SELECT schema_name
FROM information_schema.schemata
WHERE schema_name NOT IN ('pg_catalog', 'information_schema')
  AND schema_name NOT LIKE 'pg\_%'
ORDER BY schema_name`

// ListSchemas returns user schemas, excluding Postgres's own system schemas
// (pg_catalog, information_schema, and the pg_toast*/pg_temp* family, which
// all match the 'pg\_%' exclusion) — they hold Postgres's internal catalog
// and temp-table machinery, not user data, so surfacing them in a schema
// picker would be noise rather than something a Stackyard user ever needs
// to browse into.
func (e *Engine) ListSchemas(ctx context.Context) ([]string, error) {
	if e.pool == nil {
		return nil, ErrNotConnected
	}
	rows, err := e.pool.Query(ctx, listSchemasQuery)
	if err != nil {
		return nil, translatePgError("list schemas", err)
	}
	defer rows.Close()

	var schemas []string
	for rows.Next() {
		var schema string
		if err := rows.Scan(&schema); err != nil {
			return nil, translatePgError("scan schema", err)
		}
		schemas = append(schemas, schema)
	}
	if err := rows.Err(); err != nil {
		return nil, translatePgError("list schemas", err)
	}
	return schemas, nil
}

const listTablesQuery = `
SELECT
	c.table_name,
	c.column_name,
	c.data_type,
	c.is_nullable = 'YES' AS nullable,
	COALESCE(pk.is_primary_key, false) AS is_primary_key
FROM information_schema.columns c
LEFT JOIN (
	SELECT ku.table_schema, ku.table_name, ku.column_name, true AS is_primary_key
	FROM information_schema.table_constraints tc
	JOIN information_schema.key_column_usage ku
		ON tc.constraint_name = ku.constraint_name
		AND tc.table_schema = ku.table_schema
	WHERE tc.constraint_type = 'PRIMARY KEY'
		AND tc.table_schema = $1
) pk
	ON pk.table_schema = c.table_schema
	AND pk.table_name = c.table_name
	AND pk.column_name = c.column_name
WHERE c.table_schema = $1
ORDER BY c.table_name, c.ordinal_position`

// ListTables returns every table in schema with its column metadata,
// joining information_schema.columns against
// table_constraints/key_column_usage to determine which columns are primary
// keys — Postgres has no single-column "is this a primary key" flag the way
// MySQL's information_schema.columns.COLUMN_KEY does, so the constraint
// tables have to be joined explicitly.
func (e *Engine) ListTables(ctx context.Context, schema string) ([]dbengine.TableInfo, error) {
	if e.pool == nil {
		return nil, ErrNotConnected
	}
	rows, err := e.pool.Query(ctx, listTablesQuery, schema)
	if err != nil {
		return nil, translatePgError("list tables", err)
	}
	defer rows.Close()

	var order []string
	tables := make(map[string]*dbengine.TableInfo)
	for rows.Next() {
		var tableName, columnName, dataType string
		var nullable, isPrimaryKey bool
		if err := rows.Scan(&tableName, &columnName, &dataType, &nullable, &isPrimaryKey); err != nil {
			return nil, translatePgError("scan table column", err)
		}
		table, ok := tables[tableName]
		if !ok {
			table = &dbengine.TableInfo{Name: tableName}
			tables[tableName] = table
			order = append(order, tableName)
		}
		table.Columns = append(table.Columns, dbengine.ColumnInfo{
			Name:         columnName,
			DataType:     dataType,
			Nullable:     nullable,
			IsPrimaryKey: isPrimaryKey,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, translatePgError("list tables", err)
	}

	result := make([]dbengine.TableInfo, 0, len(order))
	for _, name := range order {
		result = append(result, *tables[name])
	}
	return result, nil
}

const listForeignKeysQuery = `
SELECT
	tc.table_name,
	kcu.column_name,
	ccu.table_name AS referenced_table,
	ccu.column_name AS referenced_column
FROM information_schema.table_constraints tc
JOIN information_schema.key_column_usage kcu
	ON tc.constraint_name = kcu.constraint_name
	AND tc.table_schema = kcu.table_schema
JOIN information_schema.constraint_column_usage ccu
	ON tc.constraint_name = ccu.constraint_name
	AND tc.table_schema = ccu.table_schema
WHERE tc.constraint_type = 'FOREIGN KEY'
	AND tc.table_schema = $1
ORDER BY tc.table_name, kcu.ordinal_position`

// ListForeignKeys returns every foreign key constraint in schema, joining
// information_schema.table_constraints/key_column_usage/
// constraint_column_usage the way Postgres's own documentation recommends
// for resolving a FOREIGN KEY constraint's referenced table/column — there
// is no single information_schema view that already carries this
// relationship. For a composite foreign key (more than one column),
// constraint_column_usage does not guarantee row-for-row column
// correspondence with key_column_usage; this is a known, documented
// limitation of this query rather than an oversight, and does not affect
// the common single-column case.
func (e *Engine) ListForeignKeys(ctx context.Context, schema string) ([]dbengine.ForeignKey, error) {
	if e.pool == nil {
		return nil, ErrNotConnected
	}
	rows, err := e.pool.Query(ctx, listForeignKeysQuery, schema)
	if err != nil {
		return nil, translatePgError("list foreign keys", err)
	}
	defer rows.Close()

	var foreignKeys []dbengine.ForeignKey
	for rows.Next() {
		var fk dbengine.ForeignKey
		if err := rows.Scan(&fk.TableName, &fk.ColumnName, &fk.ReferencedTable, &fk.ReferencedColumn); err != nil {
			return nil, translatePgError("scan foreign key", err)
		}
		foreignKeys = append(foreignKeys, fk)
	}
	if err := rows.Err(); err != nil {
		return nil, translatePgError("list foreign keys", err)
	}
	return foreignKeys, nil
}

// Close releases the connection pool. It is safe to call more than once.
func (e *Engine) Close() error {
	if e.pool != nil {
		e.pool.Close()
		e.pool = nil
	}
	return nil
}

// resolveTypeName resolves oid to Postgres's human-readable type name (e.g.
// "int4", "text", "varchar") using pgx's built-in OID registry. It is pure
// and dependency-free — no live connection is needed — so it can be unit
// tested directly against well-known OID constants. When oid is not a type
// pgtype's Map recognizes (a real, expected case for custom types, enums,
// and domains, not a bug path), it falls back to the OID's decimal string
// form so callers always get a non-empty DatabaseType.
func resolveTypeName(oid uint32) string {
	if t, ok := pgtype.NewMap().TypeForOID(oid); ok {
		return t.Name
	}
	return strconv.FormatUint(uint64(oid), 10)
}

// translatePgError wraps err with op for context, extracting Postgres's own
// SQLSTATE code and message when err is a *pgconn.PgError so callers (and
// eventually the query editor's inline error display, spec.md §4.6) see the
// database's actual error rather than a generic driver message.
func translatePgError(op string, err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return fmt.Errorf("postgres: %s: %s (SQLSTATE %s): %w", op, pgErr.Message, pgErr.Code, err)
	}
	return fmt.Errorf("postgres: %s: %w", op, err)
}
