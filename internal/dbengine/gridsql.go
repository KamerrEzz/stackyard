package dbengine

import (
	"fmt"
	"strconv"
	"strings"
)

// Dialect identifies which SQL placeholder/quoting convention a generated
// statement should use — currently PostgreSQL and MySQL, the only two
// engines the editable data grid (spec.md §4.3, tasks.md 4.1-4.4) supports.
type Dialect string

const (
	DialectPostgres Dialect = "postgres"
	DialectMySQL    Dialect = "mysql"
)

// QuoteIdentifier quotes a schema/table/column name per dialect's own
// identifier-quoting convention (double quotes for Postgres, backticks for
// MySQL), doubling any embedded quote character rather than rejecting it —
// the same escaping both databases themselves require for a quoted
// identifier containing that character.
func QuoteIdentifier(dialect Dialect, name string) string {
	if dialect == DialectMySQL {
		return "`" + strings.ReplaceAll(name, "`", "``") + "`"
	}
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

// qualifiedTableName returns table quoted per dialect, prefixed with
// schema. + its own quoting when schema is non-empty. An empty schema means
// "use the connection's current default schema/database" — the resulting
// SQL names only the table.
func qualifiedTableName(dialect Dialect, schema, table string) string {
	if schema == "" {
		return QuoteIdentifier(dialect, table)
	}
	return QuoteIdentifier(dialect, schema) + "." + QuoteIdentifier(dialect, table)
}

// placeholder returns dialect's own bound-parameter placeholder token for
// the position'th parameter in a statement (1-based): "$1", "$2", ... for
// Postgres, a bare "?" for every position for MySQL.
func placeholder(dialect Dialect, position int) string {
	if dialect == DialectPostgres {
		return "$" + strconv.Itoa(position)
	}
	return "?"
}

// orderedPKArgs looks up every column in pkColumns (in the given order)
// within pkValues, returning an error naming the first column missing from
// pkValues — the shared validation BuildUpdateRow/BuildDeleteRow both need
// before they can safely build a WHERE clause that uniquely identifies one
// row.
func orderedPKArgs(pkColumns []string, pkValues map[string]any) ([]any, error) {
	args := make([]any, len(pkColumns))
	for i, col := range pkColumns {
		value, ok := pkValues[col]
		if !ok {
			return nil, fmt.Errorf("missing value for primary key column %q", col)
		}
		args[i] = value
	}
	return args, nil
}

// wherePKClause builds "col1 = $N AND col2 = $N+1 ..." (or MySQL's "col1 =
// ? AND col2 = ? ...") for pkColumns, continuing the placeholder numbering
// from startPosition (1-based) so callers that already bound earlier
// placeholders (e.g. BuildUpdateRow's SET value) get correctly numbered
// Postgres placeholders in the WHERE clause that follows.
func wherePKClause(dialect Dialect, pkColumns []string, startPosition int) string {
	parts := make([]string, len(pkColumns))
	for i, col := range pkColumns {
		parts[i] = fmt.Sprintf("%s = %s", QuoteIdentifier(dialect, col), placeholder(dialect, startPosition+i))
	}
	return strings.Join(parts, " AND ")
}

// BuildSelectTableRows builds the SQL and bound args for BrowseTableRows
// (tasks.md 4.1): a "SELECT * FROM <schema>.<table> LIMIT <n> OFFSET <n>"
// statement using dialect's own placeholder/quoting convention. Both
// Postgres and MySQL accept the identical "LIMIT n OFFSET n" clause shape,
// so only the placeholder tokens differ between dialects.
func BuildSelectTableRows(dialect Dialect, schema, table string, limit, offset int) (string, []any) {
	sql := fmt.Sprintf("SELECT * FROM %s LIMIT %s OFFSET %s",
		qualifiedTableName(dialect, schema, table), placeholder(dialect, 1), placeholder(dialect, 2))
	return sql, []any{limit, offset}
}

// BuildUpdateRow builds the SQL and bound args for a single-cell UPDATE by
// primary key (tasks.md 4.1), supporting a composite (multi-column)
// primary key: pkColumns is iterated in the order given, so callers should
// pass a deterministic order (e.g. sorted by column name) if they want
// stable, reproducible generated SQL across calls. Returns an error, before
// building any SQL, if pkValues is missing a value for any column named in
// pkColumns.
func BuildUpdateRow(dialect Dialect, schema, table, columnName string, newValue any, pkColumns []string, pkValues map[string]any) (string, []any, error) {
	pkArgs, err := orderedPKArgs(pkColumns, pkValues)
	if err != nil {
		return "", nil, fmt.Errorf("build update statement: %w", err)
	}

	sql := fmt.Sprintf("UPDATE %s SET %s = %s WHERE %s",
		qualifiedTableName(dialect, schema, table),
		QuoteIdentifier(dialect, columnName),
		placeholder(dialect, 1),
		wherePKClause(dialect, pkColumns, 2))

	args := append([]any{newValue}, pkArgs...)
	return sql, args, nil
}

// BuildInsertRow builds the SQL and bound args for an INSERT (tasks.md
// 4.2). columns is iterated in the order given, and values must contain an
// entry for every name in columns. On Postgres, the statement carries a
// RETURNING * clause so InsertTableRow (app.go) can read the inserted row's
// final values back in the same round trip; MySQL has no equivalent
// clause, so InsertTableRow must re-SELECT separately for that dialect —
// see InsertTableRow's own doc comment for how it does that.
func BuildInsertRow(dialect Dialect, schema, table string, columns []string, values map[string]any) (string, []any) {
	quotedColumns := make([]string, len(columns))
	placeholders := make([]string, len(columns))
	args := make([]any, len(columns))
	for i, col := range columns {
		quotedColumns[i] = QuoteIdentifier(dialect, col)
		placeholders[i] = placeholder(dialect, i+1)
		args[i] = values[col]
	}

	sql := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)",
		qualifiedTableName(dialect, schema, table),
		strings.Join(quotedColumns, ", "),
		strings.Join(placeholders, ", "))

	if dialect == DialectPostgres {
		sql += " RETURNING *"
	}
	return sql, args
}

// BuildSelectRowByPK builds the SQL and bound args to re-read a single row
// by primary key (used by InsertTableRow in app.go to read an inserted
// row's final values back on MySQL, which has no RETURNING clause — see
// BuildInsertRow's doc comment). Returns an error, before building any SQL,
// if pkValues is missing a value for any column named in pkColumns.
func BuildSelectRowByPK(dialect Dialect, schema, table string, pkColumns []string, pkValues map[string]any) (string, []any, error) {
	pkArgs, err := orderedPKArgs(pkColumns, pkValues)
	if err != nil {
		return "", nil, fmt.Errorf("build select statement: %w", err)
	}

	sql := fmt.Sprintf("SELECT * FROM %s WHERE %s",
		qualifiedTableName(dialect, schema, table),
		wherePKClause(dialect, pkColumns, 1))

	return sql, pkArgs, nil
}

// BuildDeleteRow builds the SQL and bound args for a single-row DELETE by
// primary key (tasks.md 4.3), supporting a composite primary key the same
// way BuildUpdateRow does. Returns an error, before building any SQL, if
// pkValues is missing a value for any column named in pkColumns.
func BuildDeleteRow(dialect Dialect, schema, table string, pkColumns []string, pkValues map[string]any) (string, []any, error) {
	pkArgs, err := orderedPKArgs(pkColumns, pkValues)
	if err != nil {
		return "", nil, fmt.Errorf("build delete statement: %w", err)
	}

	sql := fmt.Sprintf("DELETE FROM %s WHERE %s",
		qualifiedTableName(dialect, schema, table),
		wherePKClause(dialect, pkColumns, 1))

	return sql, pkArgs, nil
}
