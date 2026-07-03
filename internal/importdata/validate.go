package importdata

import (
	"fmt"

	"stackyard/internal/dbengine"
)

// Mismatch is one problem Validate found between a parsed import file and
// the target table's real columns (spec.md §4.9's "mismatches are reported
// before any row is written"). RowIndex is the mismatch's 0-based index into
// ParsedFile.Rows, or -1 for a file-level mismatch not tied to one specific
// row (currently only the unknown-column check below, evaluated once
// against the whole file's declared column set). A caller displaying
// RowIndex to a human should add 1 for a 1-based row number, plus 1 more for
// a CSV source's header line, to show the actual line number in the file.
type Mismatch struct {
	RowIndex int
	Column   string
	Reason   string
}

// ValidationReport is Validate's result: every mismatch found across the
// WHOLE file (spec.md §4.9 requires collecting all of them, not stopping at
// the first), plus RowCount for the caller's own "N rows ready to import"
// display.
type ValidationReport struct {
	Mismatches []Mismatch
	RowCount   int
}

// Valid reports whether report found zero mismatches — the hard-block gate
// tasks.md 7.4 requires: ImportFile (app.go) refuses to write a single row
// unless this is true.
func (r ValidationReport) Valid() bool {
	return len(r.Mismatches) == 0
}

// Validate checks file against table's real columns (tasks.md 7.4, spec.md
// §4.9's pre-commit validation requirement), collecting every mismatch
// across the whole file before returning rather than stopping at the first
// one found:
//
//  1. Unknown columns: any name in file.Columns (the CSV header, or the
//     sorted union of JSON object keys observed across all rows — see
//     ParseJSON) that is not one of table's real ColumnInfo.Name values.
//  2. NULL-ability: for a column that IS one of table's real columns, a row
//     supplying an explicit null/empty value for it, OR — for a JSON source
//     only, since a CSV row always has every header column by construction
//     — omitting it from that row's keys entirely, is a mismatch when that
//     column's ColumnInfo.Nullable is false. Omission is deliberately
//     treated the same as an explicit null here: ColumnInfo carries no
//     "has a default value" flag, so there is no reliable way to tell an
//     omitted NOT NULL column that has a database-side default (safe to
//     omit) from one that does not (would fail); treating omission as null
//     is the stricter, safer choice, at the cost of occasionally rejecting
//     a row that a column default would have made valid.
//  3. Type plausibility: for a column that IS one of table's real columns
//     AND the row supplies a non-null value for it, the value is checked
//     against categorizeDataType(column.DataType)'s plausibility rule (see
//     isValuePlausible) — a best-effort format check, not a guarantee the
//     database will accept the value.
//
// Only the columns file.Columns actually declares are checked per row (not
// every column of table) — a column the file never mentions at all is left
// entirely alone, exactly as an INSERT statement that never names it would
// leave it to the database's own default. This is also why ImportFile
// (app.go) builds its INSERT's column list from file.Columns, not from
// every one of table's columns.
func Validate(file *ParsedFile, table dbengine.TableInfo) ValidationReport {
	columnsByName := make(map[string]dbengine.ColumnInfo, len(table.Columns))
	for _, c := range table.Columns {
		columnsByName[c.Name] = c
	}

	var mismatches []Mismatch
	for _, name := range file.Columns {
		if _, ok := columnsByName[name]; !ok {
			mismatches = append(mismatches, Mismatch{
				RowIndex: -1,
				Column:   name,
				Reason:   fmt.Sprintf("column %q does not exist on the target table", name),
			})
		}
	}

	for rowIndex, row := range file.Rows {
		for _, name := range file.Columns {
			column, ok := columnsByName[name]
			if !ok {
				continue
			}

			value, present := row[name]
			if !present || value == nil {
				if !column.Nullable {
					mismatches = append(mismatches, Mismatch{
						RowIndex: rowIndex,
						Column:   name,
						Reason:   fmt.Sprintf("column %q is not nullable but this row has no value for it", name),
					})
				}
				continue
			}

			category := categorizeDataType(column.DataType)
			if !isValuePlausible(value, category) {
				mismatches = append(mismatches, Mismatch{
					RowIndex: rowIndex,
					Column:   name,
					Reason:   fmt.Sprintf("value %v for column %q does not look like a valid %s", value, name, column.DataType),
				})
			}
		}
	}

	return ValidationReport{Mismatches: mismatches, RowCount: len(file.Rows)}
}
