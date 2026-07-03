package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"stackyard/internal/dbengine"
	"stackyard/internal/export"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// exportOperationTimeout bounds every Import/Export bound method below
// (tasks.md 7.1-7.3, spec.md §4.9). It is longer than gridOperationTimeout
// (grid.go) since a "full table" export pages through a table's entire row
// set (see fetchAllTableRows) rather than one bounded page, and a real
// production table can be large.
const exportOperationTimeout = 5 * time.Minute

// exportFetchPageSize is how many rows fetchAllTableRows requests per page.
const exportFetchPageSize = 1000

// mysqlColumnTypesQuery reads MySQL's information_schema.columns.COLUMN_TYPE
// directly (e.g. "varchar(255)", "decimal(10,2)", "enum('a','b')") — the
// fully-specified type string CREATE TABLE needs, unlike
// information_schema.columns.DATA_TYPE (what dbengine.ColumnInfo.DataType
// already carries via ListTables), which for MySQL is only the bare type
// keyword (e.g. "varchar") and is not valid standalone DDL for a type that
// requires a length. This is a separate, targeted raw query run through the
// same Engine.Exec path every other grid/query bound method already uses —
// it does not touch dbengine.Engine's interface or ListTables.
const mysqlColumnTypesQuery = `
SELECT column_name, column_type
FROM information_schema.columns
WHERE table_schema = ? AND table_name = ?
ORDER BY ordinal_position`

// fetchMySQLColumnTypes returns table's column name -> full COLUMN_TYPE
// string within schema (a MySQL database name, per the schema/database
// synonym dialectForEngine's callers already rely on).
func fetchMySQLColumnTypes(ctx context.Context, session *querySession, schema, table string) (map[string]string, error) {
	result, err := session.engine.Exec(ctx, mysqlColumnTypesQuery, schema, table)
	if err != nil {
		return nil, fmt.Errorf("fetch mysql column types: %w", err)
	}
	types := make(map[string]string, len(result.Rows))
	for _, row := range result.Rows {
		if len(row) < 2 {
			continue
		}
		name, _ := row[0].(string)
		columnType, _ := row[1].(string)
		types[name] = columnType
	}
	return types, nil
}

// buildColumnDumpInfo resolves info's columns into export.ColumnDumpInfo,
// each with a SQLType already valid to use directly in a CREATE TABLE
// statement for dialect. Postgres needs no extra query: ColumnInfo.DataType
// (from information_schema.columns.data_type) is already a complete,
// standalone-valid Postgres type name on its own (see export.ColumnDumpInfo's
// own doc comment for why that's specifically a Postgres property, not a
// general one). MySQL additionally looks up each column's full COLUMN_TYPE
// via fetchMySQLColumnTypes and prefers it; DataType is kept as a fallback
// only if a column is unexpectedly missing from that lookup.
func buildColumnDumpInfo(ctx context.Context, session *querySession, dialect dbengine.Dialect, schema string, info *dbengine.TableInfo) ([]export.ColumnDumpInfo, error) {
	var mysqlTypes map[string]string
	if dialect == dbengine.DialectMySQL {
		types, err := fetchMySQLColumnTypes(ctx, session, schema, info.Name)
		if err != nil {
			return nil, err
		}
		mysqlTypes = types
	}

	columns := make([]export.ColumnDumpInfo, len(info.Columns))
	for i, col := range info.Columns {
		sqlType := col.DataType
		if dialect == dbengine.DialectMySQL {
			if richType, ok := mysqlTypes[col.Name]; ok && richType != "" {
				sqlType = richType
			}
		}
		columns[i] = export.ColumnDumpInfo{
			Name:         col.Name,
			SQLType:      sqlType,
			Nullable:     col.Nullable,
			IsPrimaryKey: col.IsPrimaryKey,
		}
	}
	return columns, nil
}

// fetchAllTableRows returns every row of schema.table (tasks.md 7.1-7.3's
// "full table" export scope), paging through BuildSelectTableRows/Engine.Exec
// directly at exportFetchPageSize rows per page rather than issuing one
// single, potentially unbounded SELECT — a real production table can be
// large enough that fetching it in one round trip would hold an
// unnecessarily large result set in memory at once and block the connection
// for the entire duration. This deliberately bypasses BrowseTableRows (even
// though it wraps the identical query-building/Exec path) so a table export
// doesn't log one query_history entry per internal page (see
// recordQueryHistory's doc comment on grid.go) — an export isn't a query the
// user typed, and one export shouldn't spam the history log with a
// history-entry-per-page implementation detail.
func fetchAllTableRows(ctx context.Context, session *querySession, dialect dbengine.Dialect, schema, table string) ([]string, [][]any, error) {
	var columnNames []string
	var allRows [][]any

	offset := 0
	for {
		query, args := dbengine.BuildSelectTableRows(dialect, schema, table, exportFetchPageSize, offset)
		page, err := session.engine.Exec(ctx, query, args...)
		if err != nil {
			return nil, nil, fmt.Errorf("fetch table rows: %w", err)
		}
		if columnNames == nil {
			columnNames = make([]string, len(page.Columns))
			for i, c := range page.Columns {
				columnNames[i] = c.Name
			}
		}
		allRows = append(allRows, page.Rows...)
		if len(page.Rows) < exportFetchPageSize {
			break
		}
		offset += exportFetchPageSize
	}

	return columnNames, allRows, nil
}

// saveExportFile prompts the user for a save location via Wails' native
// save dialog (the first use of runtime.SaveFileDialog in this codebase),
// writes content to the chosen path, and returns that path. Kept as a
// single Go-side bound-method-internal helper — never exposed as its own
// bound method — specifically to stay within Wails v2.12.0's hard 1-or-2-
// return-value constraint on a bound method (see docs/STATE.md's Session
// 10/11 note): every ExportTableAs*/ExportQueryResultAs* method below
// returns exactly (string, error), never a separate path plus a separate
// blob. Returning ("", nil) — not an error — means the user cancelled the
// dialog; SaveFileDialog itself reports cancellation this way, not as an
// error, and callers (the frontend) should treat an empty path as "nothing
// was saved," not as a failure to report.
func (a *App) saveExportFile(defaultFilename, content string) (string, error) {
	path, err := wailsruntime.SaveFileDialog(a.ctx, wailsruntime.SaveDialogOptions{
		DefaultFilename: defaultFilename,
	})
	if err != nil {
		return "", fmt.Errorf("export: save dialog: %w", err)
	}
	if path == "" {
		return "", nil
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return "", fmt.Errorf("export: write file: %w", err)
	}
	return path, nil
}

// ExportTableAsCSV exports every row of schema.table (tasks.md 7.1's "full
// table" scope) as CSV, prompting for a save location and writing the file
// itself (see saveExportFile). See internal/export.ToCSV for the exact
// NULL-vs-empty-string convention.
func (a *App) ExportTableAsCSV(sessionID, schema, table string) (string, error) {
	session, dialect, err := a.gridSession(sessionID)
	if err != nil {
		return "", fmt.Errorf("export table as csv: %w", err)
	}

	ctx, cancel := context.WithTimeout(a.ctx, exportOperationTimeout)
	defer cancel()

	columnNames, rows, err := fetchAllTableRows(ctx, session, dialect, schema, table)
	if err != nil {
		return "", fmt.Errorf("export table as csv: %w", err)
	}

	content, err := export.ToCSV(columnNames, rows)
	if err != nil {
		return "", fmt.Errorf("export table as csv: %w", err)
	}
	return a.saveExportFile(table+".csv", content)
}

// ExportTableAsJSON exports every row of schema.table (tasks.md 7.1's "full
// table" scope) as a JSON array of objects, prompting for a save location
// and writing the file itself (see saveExportFile).
func (a *App) ExportTableAsJSON(sessionID, schema, table string) (string, error) {
	session, dialect, err := a.gridSession(sessionID)
	if err != nil {
		return "", fmt.Errorf("export table as json: %w", err)
	}

	ctx, cancel := context.WithTimeout(a.ctx, exportOperationTimeout)
	defer cancel()

	columnNames, rows, err := fetchAllTableRows(ctx, session, dialect, schema, table)
	if err != nil {
		return "", fmt.Errorf("export table as json: %w", err)
	}

	content, err := export.ToJSON(columnNames, rows)
	if err != nil {
		return "", fmt.Errorf("export table as json: %w", err)
	}
	return a.saveExportFile(table+".json", content)
}

// ExportTableAsSQLDump exports schema.table as a CREATE TABLE + INSERT SQL
// dump (tasks.md 7.3), prompting for a save location and writing the file
// itself (see saveExportFile). This is deliberately "full table" scope
// only — unlike CSV/JSON, a SQL dump has no meaningful "current query
// result" scope: an arbitrary query result can join multiple tables (so
// there is no single table to CREATE), and its columns only carry
// dbengine.ResultColumn.DatabaseType (a bare driver type name with no
// length/precision, e.g. Postgres's "varchar" or MySQL's "VARCHAR"), not the
// richer per-dialect DDL type buildColumnDumpInfo resolves here — attempting
// it would risk producing a dump that fails spec.md §4.9's actual
// requirement ("importable into a fresh instance of the same engine"), so
// this scope is narrowed on purpose rather than offered in a shape that
// might silently not round-trip.
func (a *App) ExportTableAsSQLDump(sessionID, schema, table string) (string, error) {
	session, dialect, err := a.gridSession(sessionID)
	if err != nil {
		return "", fmt.Errorf("export table as sql dump: %w", err)
	}

	ctx, cancel := context.WithTimeout(a.ctx, exportOperationTimeout)
	defer cancel()

	info, err := gridTableInfo(ctx, session, schema, table)
	if err != nil {
		return "", fmt.Errorf("export table as sql dump: %w", err)
	}

	columns, err := buildColumnDumpInfo(ctx, session, dialect, schema, info)
	if err != nil {
		return "", fmt.Errorf("export table as sql dump: %w", err)
	}

	_, rows, err := fetchAllTableRows(ctx, session, dialect, schema, table)
	if err != nil {
		return "", fmt.Errorf("export table as sql dump: %w", err)
	}

	content := export.ToSQLDump(dialect, schema, table, columns, rows)
	return a.saveExportFile(table+".sql", content)
}

// ExportQueryResultAsCSV exports an already-fetched query result (tasks.md
// 7.1's "current query result" scope) as CSV. columnNames/rows are the
// exact result the frontend already holds from RunQuery/RunMultiStatementQuery
// (dbengine.QueryResult.Columns[].Name / .Rows) — this method neither
// re-runs the query nor keeps any last-result cache of its own on the Go
// side; the frontend is the single source of truth for "what the last
// executed query returned" (see docs/STATE.md's Session 12 export design
// note for why: RunQuery/RunMultiStatementQuery already return the full
// result straight to the frontend, so caching it a second time in Go would
// be redundant state that could drift from what's actually rendered).
func (a *App) ExportQueryResultAsCSV(columnNames []string, rows [][]any) (string, error) {
	content, err := export.ToCSV(columnNames, rows)
	if err != nil {
		return "", fmt.Errorf("export query result as csv: %w", err)
	}
	return a.saveExportFile("query_result.csv", content)
}

// ExportQueryResultAsJSON exports an already-fetched query result (tasks.md
// 7.2's "current query result" scope) as a JSON array of objects — the
// query-result-scope counterpart of ExportTableAsJSON, see
// ExportQueryResultAsCSV's doc comment for why this takes columnNames/rows
// directly instead of a session ID.
func (a *App) ExportQueryResultAsJSON(columnNames []string, rows [][]any) (string, error) {
	content, err := export.ToJSON(columnNames, rows)
	if err != nil {
		return "", fmt.Errorf("export query result as json: %w", err)
	}
	return a.saveExportFile("query_result.json", content)
}
