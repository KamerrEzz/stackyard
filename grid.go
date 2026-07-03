package main

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"stackyard/internal/dbengine"
	"stackyard/internal/storage"
)

// gridOperationTimeout bounds every editable-data-grid bound method
// (BrowseTableRows/UpdateTableRow/InsertTableRow/DeleteTableRows, tasks.md
// 4.1-4.4) below. Matches schemaIntrospectionTimeout's own budget rather
// than the shorter connect/test timeouts: these methods read table
// metadata via ListTables before writing, the same information_schema-
// backed call schema introspection already uses.
const gridOperationTimeout = 10 * time.Second

// ErrTableHasNoPrimaryKey is wrapped into the error UpdateTableRow and
// DeleteTableRows return when the target table has no primary key column
// at all (spec.md §4.3: "rows without a usable primary key are read-only
// with a visible reason"). Its message always starts with the literal
// "read-only: table has no primary key" — the frontend should check an
// error's message for that substring (Go's %w wrapping in
// UpdateTableRow/DeleteTableRows prepends additional context, e.g. "update
// table row: read-only: table has no primary key") to distinguish this
// specific, expected condition from any other failure.
var ErrTableHasNoPrimaryKey = errors.New("read-only: table has no primary key")

// gridSession resolves sessionID to its live querySession and the
// dbengine.Dialect its engineType maps to, the shared first step every
// editable-data-grid bound method needs before it can generate dialect-
// correct SQL.
func (a *App) gridSession(sessionID string) (*querySession, dbengine.Dialect, error) {
	session, ok := a.getQuerySession(sessionID)
	if !ok {
		return nil, "", fmt.Errorf("no open connection session %q", sessionID)
	}
	dialect, err := dialectForEngine(session.engineType)
	if err != nil {
		return nil, "", err
	}
	return session, dialect, nil
}

// dialectForEngine maps a session's storage.Engine to the dbengine.Dialect
// its generated grid SQL should use. The editable data grid (spec.md §4.3)
// is explicitly PostgreSQL/MySQL only — MongoDB and Redis have their own,
// entirely different browse/edit UI paradigms (Phase 5/6), so a session
// opened against either is rejected here rather than producing nonsensical
// SQL.
func dialectForEngine(engine storage.Engine) (dbengine.Dialect, error) {
	switch engine {
	case storage.EnginePostgres:
		return dbengine.DialectPostgres, nil
	case storage.EngineMySQL:
		return dbengine.DialectMySQL, nil
	default:
		return "", fmt.Errorf("the editable data grid supports PostgreSQL and MySQL only, not %q", engine)
	}
}

// gridTableInfo fetches schema's tables from session's live Engine and
// returns the one named table, or an error if it isn't present in that
// schema.
func gridTableInfo(ctx context.Context, session *querySession, schema, table string) (*dbengine.TableInfo, error) {
	tables, err := session.engine.ListTables(ctx, schema)
	if err != nil {
		return nil, fmt.Errorf("list tables: %w", err)
	}
	for i := range tables {
		if tables[i].Name == table {
			return &tables[i], nil
		}
	}
	return nil, fmt.Errorf("table %q not found in schema %q", table, schema)
}

// primaryKeyColumns returns info's primary key column names, sorted
// alphabetically so every caller that builds a WHERE clause from them
// (BuildUpdateRow/BuildDeleteRow/BuildSelectRowByPK in internal/dbengine)
// produces the same, deterministic column order regardless of the order
// ListTables originally reported them in.
func primaryKeyColumns(info *dbengine.TableInfo) []string {
	var columns []string
	for _, c := range info.Columns {
		if c.IsPrimaryKey {
			columns = append(columns, c.Name)
		}
	}
	sort.Strings(columns)
	return columns
}

// sortedKeys returns values' keys sorted alphabetically — used to give
// InsertTableRow a deterministic column order for its generated INSERT
// statement, since Go map iteration order is randomized.
func sortedKeys(values map[string]any) []string {
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// rowsToSingleMap converts a *dbengine.QueryResult expected to carry
// exactly one row (a Postgres RETURNING clause, or a re-SELECT-by-primary-
// key on MySQL — see InsertTableRow) into a map[string]any keyed by column
// name, the shape InsertTableRow returns to the frontend.
func rowsToSingleMap(result *dbengine.QueryResult) (map[string]any, error) {
	if result == nil || len(result.Rows) == 0 {
		return nil, fmt.Errorf("insert succeeded but the inserted row could not be read back")
	}
	row := result.Rows[0]
	out := make(map[string]any, len(result.Columns))
	for i, col := range result.Columns {
		if i < len(row) {
			out[col.Name] = row[i]
		}
	}
	return out, nil
}

// BrowseTableRows returns one page of table's rows (tasks.md 4.1's "View:
// paginated row browsing" requirement), fetched directly by table/schema
// name rather than through an arbitrary, possibly-multi-table query — this
// is what lets the frontend know unambiguously which table/schema the grid
// is bound to (see this task's design note in docs/STATE.md), unlike
// RunQuery's results, which stay read-only regardless of the query's
// shape.
func (a *App) BrowseTableRows(sessionID, schema, table string, limit, offset int) (*dbengine.QueryResult, error) {
	session, dialect, err := a.gridSession(sessionID)
	if err != nil {
		return nil, fmt.Errorf("browse table rows: %w", err)
	}

	ctx, cancel := context.WithTimeout(a.ctx, gridOperationTimeout)
	defer cancel()

	query, args := dbengine.BuildSelectTableRows(dialect, schema, table, limit, offset)

	start := time.Now()
	result, execErr := session.engine.Exec(ctx, query, args...)
	a.recordQueryHistory(session.connectionID, query, time.Since(start), result, execErr)
	if execErr != nil {
		return nil, fmt.Errorf("browse table rows: %w", execErr)
	}
	return result, nil
}

// UpdateTableRow commits a single cell edit as an UPDATE bound by table's
// primary key (tasks.md 4.1's "Edit" requirement), supporting a composite
// (multi-column) primary key via pkValues. Returns an error wrapping
// ErrTableHasNoPrimaryKey, without attempting any write, when table has no
// primary key column at all. Every value is bound as a real query
// parameter (see internal/dbengine.BuildUpdateRow/Engine.Exec) — never
// string-interpolated into the generated SQL.
func (a *App) UpdateTableRow(sessionID, schema, table string, pkValues map[string]any, columnName string, newValue any) error {
	session, dialect, err := a.gridSession(sessionID)
	if err != nil {
		return fmt.Errorf("update table row: %w", err)
	}

	ctx, cancel := context.WithTimeout(a.ctx, gridOperationTimeout)
	defer cancel()

	info, err := gridTableInfo(ctx, session, schema, table)
	if err != nil {
		return fmt.Errorf("update table row: %w", err)
	}

	pkColumns := primaryKeyColumns(info)
	if len(pkColumns) == 0 {
		return fmt.Errorf("update table row: %w", ErrTableHasNoPrimaryKey)
	}

	query, args, err := dbengine.BuildUpdateRow(dialect, schema, table, columnName, newValue, pkColumns, pkValues)
	if err != nil {
		return fmt.Errorf("update table row: %w", err)
	}

	start := time.Now()
	result, execErr := session.engine.Exec(ctx, query, args...)
	a.recordQueryHistory(session.connectionID, query, time.Since(start), result, execErr)
	if execErr != nil {
		return fmt.Errorf("update table row: %w", execErr)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("update table row: no row matched the given primary key values")
	}
	return nil
}

// InsertTableRow inserts one new row (tasks.md 4.2) built from values
// (column name to value), bound as real query parameters via
// internal/dbengine.BuildInsertRow/Engine.Exec, and returns the inserted
// row's final values (including any column defaults/generated values the
// database itself filled in). On Postgres this is a single round trip via
// a RETURNING * clause; MySQL has no RETURNING, so a second round trip
// re-SELECTs the row by primary key afterward — see resolveInsertedRow for
// exactly how that re-SELECT's WHERE clause is determined, including its
// documented best-effort fallback when no primary key can be correlated to
// the just-inserted row. Unlike UpdateTableRow/DeleteTableRows, a missing
// primary key does not block the insert itself — only re-reading the
// final row afterward can be affected.
func (a *App) InsertTableRow(sessionID, schema, table string, values map[string]any) (map[string]any, error) {
	session, dialect, err := a.gridSession(sessionID)
	if err != nil {
		return nil, fmt.Errorf("insert table row: %w", err)
	}

	ctx, cancel := context.WithTimeout(a.ctx, gridOperationTimeout)
	defer cancel()

	info, err := gridTableInfo(ctx, session, schema, table)
	if err != nil {
		return nil, fmt.Errorf("insert table row: %w", err)
	}

	columns := sortedKeys(values)
	query, args := dbengine.BuildInsertRow(dialect, schema, table, columns, values)

	start := time.Now()
	result, execErr := session.engine.Exec(ctx, query, args...)
	a.recordQueryHistory(session.connectionID, query, time.Since(start), result, execErr)
	if execErr != nil {
		return nil, fmt.Errorf("insert table row: %w", execErr)
	}

	row, err := a.resolveInsertedRow(ctx, session, dialect, schema, table, info, values, result)
	if err != nil {
		return nil, fmt.Errorf("insert table row: %w", err)
	}
	return row, nil
}

// resolveInsertedRow returns InsertTableRow's just-inserted row values.
// Postgres already has the full row from its INSERT ... RETURNING *
// result. MySQL has no equivalent clause, so this re-SELECTs by primary
// key: if every primary key column's value was present in the caller's
// original values, those are used directly; otherwise, if the table has
// exactly one primary key column and the caller didn't supply it (the
// common auto-increment case), result.LastInsertID (populated by the
// MySQL driver's sql.Result.LastInsertId(), see internal/dbengine/mysql)
// is used instead. If neither case applies — a composite primary key with
// some but not all columns supplied, or no primary key at all — the
// inserted row cannot be reliably correlated, so this falls back to
// returning the caller's original values map as-is (a documented, explicit
// limitation, not a silent bug: the caller sees exactly what it submitted,
// not the database's own defaults/computed columns for that row).
func (a *App) resolveInsertedRow(ctx context.Context, session *querySession, dialect dbengine.Dialect, schema, table string, info *dbengine.TableInfo, values map[string]any, result *dbengine.QueryResult) (map[string]any, error) {
	if dialect == dbengine.DialectPostgres {
		return rowsToSingleMap(result)
	}

	pkColumns := primaryKeyColumns(info)
	if len(pkColumns) == 0 {
		return values, nil
	}

	lookup := make(map[string]any, len(pkColumns))
	for _, col := range pkColumns {
		if v, ok := values[col]; ok {
			lookup[col] = v
		}
	}
	if len(lookup) != len(pkColumns) {
		if len(pkColumns) == 1 && len(lookup) == 0 && result.LastInsertID != 0 {
			lookup[pkColumns[0]] = result.LastInsertID
		} else {
			return values, nil
		}
	}

	query, args, err := dbengine.BuildSelectRowByPK(dialect, schema, table, pkColumns, lookup)
	if err != nil {
		return values, nil
	}

	selected, err := session.engine.Exec(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("re-select inserted row: %w", err)
	}
	return rowsToSingleMap(selected)
}

// DeleteTableRows deletes every row named in pkValuesList (tasks.md 4.3),
// building one independent DELETE statement per row and running them
// through internal/dbengine.ExecuteBatch so one row's failure (e.g. an FK
// constraint) never blocks the others from being deleted. The returned
// error is non-nil only when the whole operation could not even start
// (unknown session, unsupported engine, an empty pkValuesList, or the
// table having no primary key at all — wrapping ErrTableHasNoPrimaryKey,
// same as UpdateTableRow); once execution begins, per-row outcomes are
// reported ONLY via the returned []dbengine.StatementResult (Success/
// ErrorMessage per entry, in the same order as pkValuesList), never via
// the returned error — callers must inspect every entry rather than
// relying on the error to learn about a partial failure.
func (a *App) DeleteTableRows(sessionID, schema, table string, pkValuesList []map[string]any) ([]dbengine.StatementResult, error) {
	session, dialect, err := a.gridSession(sessionID)
	if err != nil {
		return nil, fmt.Errorf("delete table rows: %w", err)
	}
	if len(pkValuesList) == 0 {
		return nil, fmt.Errorf("delete table rows: at least one row is required")
	}

	ctx, cancel := context.WithTimeout(a.ctx, gridOperationTimeout)
	defer cancel()

	info, err := gridTableInfo(ctx, session, schema, table)
	if err != nil {
		return nil, fmt.Errorf("delete table rows: %w", err)
	}

	pkColumns := primaryKeyColumns(info)
	if len(pkColumns) == 0 {
		return nil, fmt.Errorf("delete table rows: %w", ErrTableHasNoPrimaryKey)
	}

	results := make([]dbengine.StatementResult, len(pkValuesList))
	var statements []dbengine.PreparedStatement
	var statementIndexes []int
	for i, pkValues := range pkValuesList {
		query, args, buildErr := dbengine.BuildDeleteRow(dialect, schema, table, pkColumns, pkValues)
		if buildErr != nil {
			results[i] = dbengine.StatementResult{Success: false, ErrorMessage: buildErr.Error()}
			continue
		}
		statements = append(statements, dbengine.PreparedStatement{Text: query, Args: args})
		statementIndexes = append(statementIndexes, i)
	}

	execResults := dbengine.ExecuteBatch(ctx, session.engine, statements)
	for i, execResult := range execResults {
		results[statementIndexes[i]] = execResult
	}

	for _, r := range results {
		a.recordStatementResultHistory(session.connectionID, r)
	}

	return results, nil
}

// recordStatementResultHistory logs one independently-executed statement's
// outcome to query_history (see recordQueryHistory's own doc comment for the
// same saved-connection-only scope) — one entry per statement, matching how
// RunQuery would log each statement if it had been typed and run
// individually. Shared by DeleteTableRows (one entry per deleted row) and
// RunMultiStatementQuery (one entry per statement in a multi-statement
// script, see multiquery.go).
func (a *App) recordStatementResultHistory(connectionID *int64, result dbengine.StatementResult) {
	var duration time.Duration
	if result.Result != nil {
		duration = result.Result.Duration
	}
	var execErr error
	if !result.Success {
		execErr = errors.New(result.ErrorMessage)
	}
	a.recordQueryHistory(connectionID, result.Statement, duration, result.Result, execErr)
}
