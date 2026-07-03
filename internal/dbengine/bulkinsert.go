package dbengine

import (
	"fmt"
	"strings"
)

// BuildBulkInsertRows builds the SQL and bound args for inserting every row
// in rows via a single INSERT statement carrying one VALUES tuple per row
// (tasks.md 7.4's CSV/JSON import, spec.md §4.9), rather than one statement
// per row. columns is iterated in the same order for every row's tuple; a
// row missing an entry for one of columns binds Go's nil (SQL NULL) for
// that position. A single multi-row INSERT is atomic on both Postgres and
// MySQL/InnoDB without an explicit transaction — either every row commits or
// the whole statement is rolled back by the engine itself — which is what
// lets the importer (ImportFile, app.go) satisfy "abort before any row is
// written" even for a constraint violation its own pre-commit validation
// did not already catch.
func BuildBulkInsertRows(dialect Dialect, schema, table string, columns []string, rows []map[string]any) (string, []any) {
	quotedColumns := make([]string, len(columns))
	for i, col := range columns {
		quotedColumns[i] = QuoteIdentifier(dialect, col)
	}

	valueGroups := make([]string, len(rows))
	args := make([]any, 0, len(rows)*len(columns))
	position := 1
	for r, row := range rows {
		placeholders := make([]string, len(columns))
		for i, col := range columns {
			placeholders[i] = placeholder(dialect, position)
			args = append(args, row[col])
			position++
		}
		valueGroups[r] = "(" + strings.Join(placeholders, ", ") + ")"
	}

	sql := fmt.Sprintf("INSERT INTO %s (%s) VALUES %s",
		qualifiedTableName(dialect, schema, table),
		strings.Join(quotedColumns, ", "),
		strings.Join(valueGroups, ", "))
	return sql, args
}
