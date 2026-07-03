package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"stackyard/internal/dbengine"
	"stackyard/internal/importdata"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// importOperationTimeout bounds ValidateImportFile and ImportFile's
// schema-introspection-plus-write round trip (tasks.md 7.4). More generous
// than gridOperationTimeout's 10s budget since a bulk INSERT carrying many
// rows in one statement can legitimately take longer than a single-row grid
// write.
const importOperationTimeout = 60 * time.Second

// ImportValidationResult is ValidateImportFile's Wails-bound return shape.
// Wrapped in a single struct per the hard IPC constraint documented in
// docs/STATE.md (Wails v2.12.0 silently drops any bound method's return
// value beyond the first two — see boundMethod.go's OutputCount switch):
// this method has two logical outputs, the mismatch list and the parsed row
// count, plus its error, so both data values are carried on this struct
// instead of being declared as separate return values.
type ImportValidationResult struct {
	Mismatches []importdata.Mismatch
	RowCount   int
}

// ImportCommitResult is ImportFile's Wails-bound return shape, wrapped for
// the same reason as ImportValidationResult above. Mismatches is always
// non-empty exactly when RowsInserted is 0, and vice versa: ImportFile
// re-validates internally immediately before committing (tasks.md 7.4's
// abort-before-write requirement) and never writes a single row if any
// mismatch survives that re-validation, even if an earlier
// ValidateImportFile call against the same path reported none — the file on
// disk could have changed in between the two calls.
type ImportCommitResult struct {
	Mismatches   []importdata.Mismatch
	RowsInserted int
}

// PickImportFile opens the native OS file-picker dialog (tasks.md 7.4),
// restricted to CSV/JSON files, and returns the chosen absolute path. An
// empty string with a nil error means the user cancelled the dialog — this
// is not an error condition, matching wailsruntime.OpenFileDialog's own
// contract. This is the first use of wailsruntime.OpenFileDialog anywhere in
// this codebase; every other file-touching feature (export, planned in
// tasks.md 7.1-7.3) either already has, or will need, its own SaveFileDialog
// call following the same pattern.
func (a *App) PickImportFile() (string, error) {
	path, err := wailsruntime.OpenFileDialog(a.ctx, wailsruntime.OpenDialogOptions{
		Title: "Select a CSV or JSON file to import",
		Filters: []wailsruntime.FileFilter{
			{DisplayName: "CSV/JSON files (*.csv, *.json)", Pattern: "*.csv;*.json"},
		},
	})
	if err != nil {
		return "", fmt.Errorf("pick import file: %w", err)
	}
	return path, nil
}

// ValidateImportFile parses filePath (tasks.md 7.4) and validates it against
// table's real columns via importdata.Validate, without writing anything —
// this is the "sees a validation report" half of the import flow (spec.md
// §4.9), called before the user is offered a confirm/commit action.
func (a *App) ValidateImportFile(sessionID, schema, table, filePath string) (*ImportValidationResult, error) {
	_, parsed, mismatches, err := a.prepareImport(sessionID, schema, table, filePath)
	if err != nil {
		return nil, fmt.Errorf("validate import file: %w", err)
	}
	return &ImportValidationResult{Mismatches: mismatches, RowCount: len(parsed.Rows)}, nil
}

// ImportFile re-validates filePath against table's real columns and, only if
// that re-validation finds zero mismatches, commits every row via a single
// bulk INSERT (tasks.md 7.4, spec.md §4.9's hard "abort-before-write on
// mismatch" requirement). Re-validating here rather than trusting a prior
// ValidateImportFile call is deliberate: the file on disk, or the target
// table's own columns, could have changed between the user's "see the
// report" step and their "confirm import" click, and this method must never
// depend on frontend-cached state for something this destructive.
//
// The whole file is validated to completion before a single row is written
// — importdata.Validate always scans every row (see its own doc comment),
// and this method only reaches the INSERT call at all when Mismatches is
// empty, so an interleaved validate-one-row/insert-one-row loop (which could
// let earlier rows commit before a later bad row aborts the batch) never
// happens. Once validation is clean, every row commits via one
// dbengine.BuildBulkInsertRows call, a single INSERT statement carrying one
// VALUES tuple per row rather than N separate InsertTableRow round trips —
// chosen for two reasons: it is one round trip instead of N regardless of
// file size, and a single SQL statement is atomic on both Postgres and
// MySQL/InnoDB without this code needing to manage an explicit transaction
// itself, so a constraint violation the pre-commit validation did not catch
// (e.g. a UNIQUE or CHECK constraint, which ColumnInfo does not expose)
// still rolls back the entire statement rather than leaving a partial
// import behind.
func (a *App) ImportFile(sessionID, schema, table, filePath string) (*ImportCommitResult, error) {
	session, dialect, err := a.gridSession(sessionID)
	if err != nil {
		return nil, fmt.Errorf("import file: %w", err)
	}

	_, parsed, mismatches, err := a.prepareImport(sessionID, schema, table, filePath)
	if err != nil {
		return nil, fmt.Errorf("import file: %w", err)
	}
	if len(mismatches) > 0 {
		return &ImportCommitResult{Mismatches: mismatches}, nil
	}
	if len(parsed.Rows) == 0 {
		return &ImportCommitResult{}, nil
	}

	ctx, cancel := context.WithTimeout(a.ctx, importOperationTimeout)
	defer cancel()

	query, args := dbengine.BuildBulkInsertRows(dialect, schema, table, parsed.Columns, parsed.Rows)

	start := time.Now()
	result, execErr := session.engine.Exec(ctx, query, args...)
	a.recordQueryHistory(session.connectionID, query, time.Since(start), result, execErr)
	if execErr != nil {
		return nil, fmt.Errorf("import file: %w", execErr)
	}

	return &ImportCommitResult{RowsInserted: len(parsed.Rows)}, nil
}

// prepareImport is ValidateImportFile and ImportFile's shared first half:
// resolve sessionID's live session, fetch table's real column metadata,
// parse filePath, and run importdata.Validate against it — every step a
// caller needs before it can decide whether to write anything.
func (a *App) prepareImport(sessionID, schema, table, filePath string) (*dbengine.TableInfo, *importdata.ParsedFile, []importdata.Mismatch, error) {
	session, _, err := a.gridSession(sessionID)
	if err != nil {
		return nil, nil, nil, err
	}

	ctx, cancel := context.WithTimeout(a.ctx, schemaIntrospectionTimeout)
	defer cancel()
	info, err := gridTableInfo(ctx, session, schema, table)
	if err != nil {
		return nil, nil, nil, err
	}

	parsed, err := parseImportFile(filePath)
	if err != nil {
		return nil, nil, nil, err
	}

	report := importdata.Validate(parsed, *info)
	return info, parsed, report.Mismatches, nil
}

// parseImportFile opens path and parses it as CSV or JSON based on its file
// extension (tasks.md 7.4) — the only two import formats spec.md §4.9
// requires.
func parseImportFile(path string) (*importdata.ParsedFile, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open import file: %w", err)
	}
	defer f.Close()

	switch strings.ToLower(filepath.Ext(path)) {
	case ".csv":
		return importdata.ParseCSV(f)
	case ".json":
		return importdata.ParseJSON(f)
	default:
		return nil, fmt.Errorf("unsupported import file extension %q (expected .csv or .json)", filepath.Ext(path))
	}
}
