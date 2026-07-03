// Package mysql implements dbengine.Engine for MySQL using the standard
// database/sql interface with github.com/go-sql-driver/mysql registered as
// the driver — the idiomatic way to use that driver, since it never exposes
// an API of its own outside the database/sql contract. A *Engine value is
// constructed already bound to one DSN via New; Connect performs the actual
// dial and an initial ping — construction itself never dials, matching
// dbengine.Engine's documented contract.
package mysql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"stackyard/internal/dbengine"

	mysqldriver "github.com/go-sql-driver/mysql"
)

var _ dbengine.Engine = (*Engine)(nil)

// ErrNotConnected is returned by Ping, Query, ListSchemas, and ListTables
// when called before a successful Connect.
var ErrNotConnected = errors.New("mysql: not connected")

// Engine implements dbengine.Engine for MySQL via database/sql.
type Engine struct {
	dsn string
	db  *sql.DB
}

// New returns an Engine bound to dsn, in go-sql-driver/mysql's own DSN
// format (e.g. "user:pass@tcp(host:port)/dbname?parseTime=true"), not a
// "mysql://" URL — parsing a "mysql://" URL into this format is
// urlparse.go's job (tasks.md 3.3), a separate, later concern. It does not
// dial; call Connect to open the pool.
//
// dsn is used exactly as given; Connect does not inject parseTime or any
// other parameter. Without "parseTime=true", MySQL DATETIME/TIMESTAMP
// columns scan as their raw textual bytes instead of time.Time — callers
// that want time.Time values must include it in dsn themselves.
func New(dsn string) *Engine {
	return &Engine{dsn: dsn}
}

// Connect opens the connection pool and confirms it is reachable with an
// initial ping. Calling Connect again after a prior successful call closes
// the existing pool before replacing it.
func (e *Engine) Connect(ctx context.Context) error {
	db, err := sql.Open("mysql", e.dsn)
	if err != nil {
		return fmt.Errorf("mysql: connect: %w", err)
	}
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return fmt.Errorf("mysql: connect: initial ping: %w", err)
	}
	if e.db != nil {
		e.db.Close()
	}
	e.db = db
	return nil
}

// Ping confirms the connection is still reachable.
func (e *Engine) Ping(ctx context.Context) error {
	if e.db == nil {
		return ErrNotConnected
	}
	if err := e.db.PingContext(ctx); err != nil {
		return fmt.Errorf("mysql: ping: %w", err)
	}
	return nil
}

// Query executes a single statement. For a read statement (SELECT and
// similar — see isReadStatement), Columns and Rows are populated; for a
// write/DDL statement, RowsAffected is populated from the driver's Exec
// result instead. database/sql has no single call that returns both rows
// and an affected-row count the way pgx's Query-plus-CommandTag does (see
// the postgres package), so Query classifies the statement itself to pick
// QueryContext or ExecContext. ctx governs the query's lifetime end to end:
// go-sql-driver/mysql watches ctx in a background goroutine and closes the
// underlying connection the moment it's cancelled or times out, dropping
// the in-flight query server-side rather than merely giving up on the
// client side.
func (e *Engine) Query(ctx context.Context, query string) (*dbengine.QueryResult, error) {
	if e.db == nil {
		return nil, ErrNotConnected
	}

	start := time.Now()

	if !isReadStatement(query) {
		result, err := e.db.ExecContext(ctx, query)
		if err != nil {
			return nil, translateMySQLError("exec", err)
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return nil, translateMySQLError("read rows affected", err)
		}
		return &dbengine.QueryResult{RowsAffected: affected, Duration: time.Since(start)}, nil
	}

	rows, err := e.db.QueryContext(ctx, query)
	if err != nil {
		return nil, translateMySQLError("query", err)
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		return nil, translateMySQLError("read columns", err)
	}

	var resultRows [][]any
	for rows.Next() {
		row, err := scanRow(rows, len(columns))
		if err != nil {
			return nil, translateMySQLError("scan row", err)
		}
		resultRows = append(resultRows, row)
	}
	if err := rows.Err(); err != nil {
		return nil, translateMySQLError("row iteration", err)
	}

	return &dbengine.QueryResult{Columns: columns, Rows: resultRows, Duration: time.Since(start)}, nil
}

// scanRow scans one row into a []any, converting driver []byte values
// (go-sql-driver/mysql's representation for most textual/decimal column
// types) into string so a QueryResult's rows are display-ready rather than
// raw byte slices.
func scanRow(rows *sql.Rows, numColumns int) ([]any, error) {
	values := make([]any, numColumns)
	scanArgs := make([]any, numColumns)
	for i := range values {
		scanArgs[i] = &values[i]
	}
	if err := rows.Scan(scanArgs...); err != nil {
		return nil, err
	}
	row := make([]any, numColumns)
	for i, v := range values {
		if b, ok := v.([]byte); ok {
			row[i] = string(b)
		} else {
			row[i] = v
		}
	}
	return row, nil
}

var readStatementPrefixes = []string{
	"SELECT", "SHOW", "DESCRIBE", "DESC", "EXPLAIN", "WITH", "TABLE", "VALUES",
}

// isReadStatement reports whether query's first keyword is one that
// produces a result set (a SELECT-shaped statement), as opposed to a
// write/DDL statement whose outcome is a row count. Leading whitespace and
// comments (-- line and /* block */) are skipped before the check.
func isReadStatement(query string) bool {
	trimmed := stripLeadingNoise(query)
	upper := strings.ToUpper(trimmed)
	for _, prefix := range readStatementPrefixes {
		if strings.HasPrefix(upper, prefix) {
			return true
		}
	}
	return false
}

// stripLeadingNoise removes leading whitespace and SQL comments from query,
// repeatedly, until it finds a line that starts with neither.
func stripLeadingNoise(query string) string {
	s := query
	for {
		trimmed := strings.TrimLeft(s, " \t\r\n")
		switch {
		case strings.HasPrefix(trimmed, "--"):
			if i := strings.IndexByte(trimmed, '\n'); i >= 0 {
				s = trimmed[i+1:]
				continue
			}
			return ""
		case strings.HasPrefix(trimmed, "/*"):
			if i := strings.Index(trimmed, "*/"); i >= 0 {
				s = trimmed[i+2:]
				continue
			}
			return ""
		default:
			return trimmed
		}
	}
}

const listSchemasQuery = `
SELECT schema_name
FROM information_schema.schemata
WHERE schema_name NOT IN ('mysql', 'information_schema', 'performance_schema', 'sys')
ORDER BY schema_name`

// ListSchemas returns databases, excluding MySQL's own system databases
// (mysql, information_schema, performance_schema, sys) since they hold
// MySQL's internal catalog/grant/instrumentation data, not user data — the
// same "hide system namespaces" judgment postgres.Engine.ListSchemas makes,
// applied to MySQL's schema/database synonym: MySQL has no separate
// "schema" concept below "database" (CREATE SCHEMA is literally an alias
// for CREATE DATABASE), so ListSchemas here returns the list of databases.
func (e *Engine) ListSchemas(ctx context.Context) ([]string, error) {
	if e.db == nil {
		return nil, ErrNotConnected
	}
	rows, err := e.db.QueryContext(ctx, listSchemasQuery)
	if err != nil {
		return nil, translateMySQLError("list schemas", err)
	}
	defer rows.Close()

	var schemas []string
	for rows.Next() {
		var schema string
		if err := rows.Scan(&schema); err != nil {
			return nil, translateMySQLError("scan schema", err)
		}
		schemas = append(schemas, schema)
	}
	if err := rows.Err(); err != nil {
		return nil, translateMySQLError("list schemas", err)
	}
	return schemas, nil
}

const listTablesQuery = `
SELECT table_name, column_name, data_type, is_nullable, column_key
FROM information_schema.columns
WHERE table_schema = ?
ORDER BY table_name, ordinal_position`

// ListTables returns every table in schema (a database name, per the
// schema/database synonym above) with its column metadata. Unlike Postgres,
// MySQL's information_schema.columns carries primary-key membership
// directly on COLUMN_KEY ('PRI' marks a primary key column), so no join
// against a separate constraints table is needed here.
func (e *Engine) ListTables(ctx context.Context, schema string) ([]dbengine.TableInfo, error) {
	if e.db == nil {
		return nil, ErrNotConnected
	}
	rows, err := e.db.QueryContext(ctx, listTablesQuery, schema)
	if err != nil {
		return nil, translateMySQLError("list tables", err)
	}
	defer rows.Close()

	var order []string
	tables := make(map[string]*dbengine.TableInfo)
	for rows.Next() {
		var tableName, columnName, dataType, isNullable, columnKey string
		if err := rows.Scan(&tableName, &columnName, &dataType, &isNullable, &columnKey); err != nil {
			return nil, translateMySQLError("scan table column", err)
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
			Nullable:     isNullable == "YES",
			IsPrimaryKey: columnKey == "PRI",
		})
	}
	if err := rows.Err(); err != nil {
		return nil, translateMySQLError("list tables", err)
	}

	result := make([]dbengine.TableInfo, 0, len(order))
	for _, name := range order {
		result = append(result, *tables[name])
	}
	return result, nil
}

// Close releases the connection pool. It is safe to call more than once.
func (e *Engine) Close() error {
	if e.db != nil {
		err := e.db.Close()
		e.db = nil
		return err
	}
	return nil
}

// translateMySQLError wraps err with op for context, extracting MySQL's own
// error number and message when err is a *mysqldriver.MySQLError so callers
// (and eventually the query editor's inline error display, spec.md §4.6)
// see the database's actual error rather than a generic driver message.
func translateMySQLError(op string, err error) error {
	var myErr *mysqldriver.MySQLError
	if errors.As(err, &myErr) {
		return fmt.Errorf("mysql: %s: %s (error %d): %w", op, myErr.Message, myErr.Number, err)
	}
	return fmt.Errorf("mysql: %s: %w", op, err)
}
