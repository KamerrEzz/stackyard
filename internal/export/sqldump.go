package export

import (
	"fmt"
	"strings"

	"stackyard/internal/dbengine"
)

// ColumnDumpInfo is one column's DDL-ready shape for BuildCreateTable
// (tasks.md 7.3) — richer than dbengine.ColumnInfo, whose DataType alone is
// not always enough to reconstruct a valid CREATE TABLE: Postgres's
// information_schema.columns.data_type (e.g. "character varying", "numeric")
// is always already valid to use standalone as a Postgres type name (an
// unbounded varchar/arbitrary-precision numeric is legal Postgres DDL), but
// MySQL's equivalent data_type (e.g. "varchar") is not — MySQL requires a
// length on VARCHAR. SQLType must therefore already be a complete, dialect-
// valid type expression by the time it reaches this package; see export.go
// at the repo root for how each dialect resolves it (Postgres: ColumnInfo's
// own DataType, reused as-is; MySQL: a dedicated
// information_schema.columns.COLUMN_TYPE lookup, which already carries
// length/precision, e.g. "varchar(255)").
type ColumnDumpInfo struct {
	Name         string
	SQLType      string
	Nullable     bool
	IsPrimaryKey bool
}

// sqlDumpInsertBatchSize bounds how many rows one generated INSERT statement
// covers (tasks.md 7.3's "documented judgment call" on multi-row vs.
// one-per-row INSERTs): a single INSERT per batch of rows is both fewer
// round trips on re-import than one INSERT per row, and bounded so a very
// large table doesn't produce one arbitrarily huge statement that risks
// hitting a driver/server statement-size limit.
const sqlDumpInsertBatchSize = 500

// BuildCreateTable renders a single CREATE TABLE statement (no trailing
// semicolon — see ToSQLDump for how the dump file's statement separators are
// added) for table, quoting every identifier per dialect's own convention
// (dbengine.QuoteIdentifier) and appending a trailing PRIMARY KEY clause
// only when at least one column reports IsPrimaryKey. A table with no
// primary key at all (spec.md §4.3's "read-only" case elsewhere in this
// codebase) is valid DDL without that clause, so it is simply omitted
// rather than treated as an error here.
func BuildCreateTable(dialect dbengine.Dialect, schema, table string, columns []ColumnDumpInfo) string {
	lines := make([]string, 0, len(columns)+1)
	var pkColumns []string
	for _, col := range columns {
		line := fmt.Sprintf("  %s %s", dbengine.QuoteIdentifier(dialect, col.Name), col.SQLType)
		if !col.Nullable {
			line += " NOT NULL"
		}
		lines = append(lines, line)
		if col.IsPrimaryKey {
			pkColumns = append(pkColumns, col.Name)
		}
	}
	if len(pkColumns) > 0 {
		quoted := make([]string, len(pkColumns))
		for i, name := range pkColumns {
			quoted[i] = dbengine.QuoteIdentifier(dialect, name)
		}
		lines = append(lines, fmt.Sprintf("  PRIMARY KEY (%s)", strings.Join(quoted, ", ")))
	}

	return fmt.Sprintf("CREATE TABLE %s (\n%s\n)", qualifiedDumpTableName(dialect, schema, table), strings.Join(lines, ",\n"))
}

// BuildInsertStatements renders rows as one or more multi-row INSERT
// statements (no trailing semicolons), batched at sqlDumpInsertBatchSize
// rows per statement, in columnNames' given order. Every value is rendered
// via formatSQLLiteral — this is the one path in the codebase that embeds
// values directly into executable SQL text rather than binding them as
// query parameters (see formatSQLLiteral's own doc comment for why a SQL
// dump file has no bound-parameter equivalent to fall back on). Returns nil
// for zero rows, matching BuildCreateTable's already-standalone-valid
// output: an empty table's dump is just its CREATE TABLE statement.
func BuildInsertStatements(dialect dbengine.Dialect, schema, table string, columnNames []string, rows [][]any) []string {
	if len(rows) == 0 {
		return nil
	}

	quotedColumns := make([]string, len(columnNames))
	for i, name := range columnNames {
		quotedColumns[i] = dbengine.QuoteIdentifier(dialect, name)
	}
	tableName := qualifiedDumpTableName(dialect, schema, table)

	var statements []string
	for start := 0; start < len(rows); start += sqlDumpInsertBatchSize {
		end := start + sqlDumpInsertBatchSize
		if end > len(rows) {
			end = len(rows)
		}
		statements = append(statements, buildInsertBatch(dialect, tableName, quotedColumns, rows[start:end]))
	}
	return statements
}

func buildInsertBatch(dialect dbengine.Dialect, tableName string, quotedColumns []string, rows [][]any) string {
	valueTuples := make([]string, len(rows))
	for i, row := range rows {
		literals := make([]string, len(quotedColumns))
		for c := range quotedColumns {
			var v any
			if c < len(row) {
				v = row[c]
			}
			literals[c] = formatSQLLiteral(dialect, v)
		}
		valueTuples[i] = "(" + strings.Join(literals, ", ") + ")"
	}
	return fmt.Sprintf("INSERT INTO %s (%s) VALUES\n  %s", tableName, strings.Join(quotedColumns, ", "), strings.Join(valueTuples, ",\n  "))
}

// ToSQLDump renders table's full dump (tasks.md 7.3): BuildCreateTable
// followed by every BuildInsertStatements batch, each terminated with ";\n"
// so the result is a complete, directly-executable-statement-by-statement
// .sql file. columnNames for the INSERT statements are derived from
// columns' own order, so CREATE TABLE and INSERT always agree on column
// order without the caller having to pass it twice.
func ToSQLDump(dialect dbengine.Dialect, schema, table string, columns []ColumnDumpInfo, rows [][]any) string {
	columnNames := make([]string, len(columns))
	for i, col := range columns {
		columnNames[i] = col.Name
	}

	var b strings.Builder
	b.WriteString(BuildCreateTable(dialect, schema, table, columns))
	b.WriteString(";\n")
	for _, stmt := range BuildInsertStatements(dialect, schema, table, columnNames, rows) {
		b.WriteString("\n")
		b.WriteString(stmt)
		b.WriteString(";\n")
	}
	return b.String()
}

// qualifiedDumpTableName mirrors internal/dbengine/gridsql.go's own
// unexported qualifiedTableName exactly (schema-qualify table when schema is
// non-empty, per-dialect identifier quoting otherwise) — duplicated here
// rather than exported from dbengine, since gridsql.go's version is
// deliberately unexported and this package intentionally has no other
// dependency on gridsql.go's grid-specific SQL builders.
func qualifiedDumpTableName(dialect dbengine.Dialect, schema, table string) string {
	if schema == "" {
		return dbengine.QuoteIdentifier(dialect, table)
	}
	return dbengine.QuoteIdentifier(dialect, schema) + "." + dbengine.QuoteIdentifier(dialect, table)
}
