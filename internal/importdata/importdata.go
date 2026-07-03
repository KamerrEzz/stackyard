// Package importdata parses CSV/JSON import files into a shared
// intermediate shape and validates them against a target table's real
// columns before any row is written (tasks.md 7.4, spec.md §4.9's "Import
// validates the file against the target table's columns before committing;
// mismatches are reported before any row is written"). This package never
// touches a database connection itself — ImportFile (app.go) is the caller
// that runs Validate to completion and only then builds/executes the actual
// INSERT via dbengine.BuildBulkInsertRows.
//
// Null-vs-empty-string convention: a CSV cell that is entirely empty AND
// unquoted decodes to Go's nil (SQL NULL); a cell written as "" (two
// double-quote characters, i.e. an explicitly quoted empty string) decodes
// to Go's "" (an empty string value, not NULL). This mirrors PostgreSQL's
// own COPY ... CSV convention for the same reason: it is the only way to
// keep NULL and "" distinguishable in a CSV round trip (spec.md §4.9). Go's
// standard encoding/csv.Reader cannot express this distinction on its own —
// it unquotes every field before returning it, so a quoted "" and a bare
// empty field are indistinguishable by the time a caller sees them — so
// ParseCSV uses a small hand-written RFC4180-equivalent tokenizer (see
// csvparse.go) that tracks each field's original quoting instead of calling
// encoding/csv directly. This is a deliberate, documented deviation from
// literally using encoding/csv, made because the standard library's own
// abstraction throws away exactly the bit this feature's round-trip-fidelity
// requirement needs.
//
// No internal/export package existed in this repository at the time this
// was written, so no existing convention could be confirmed or reused;
// whichever export implementation ships must produce CSV output compatible
// with this convention (quote every empty string explicitly, leave a NULL
// cell entirely blank and unquoted) or a round trip through both will not
// preserve NULL vs "" correctly.
//
// JSON has no equivalent ambiguity: a present key with a JSON null value
// decodes to Go's nil directly via encoding/json, and a genuinely missing
// key is simply absent from the resulting map — both already distinguishable
// without any special-casing.
package importdata

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
)

// ParsedFile is the shared intermediate shape both ParseCSV and ParseJSON
// produce (tasks.md 7.4's "common intermediate shape" requirement). Columns
// is the CSV header row, in file order, for a CSV source; for a JSON source
// it is the sorted union of every key observed across every object in the
// array — informational only (row order/composition can genuinely differ
// per object in a JSON array, unlike CSV's fixed header), never assumed
// uniform. Validate (validate.go) checks each row against its own actual
// keys, not against Columns.
type ParsedFile struct {
	Columns []string
	Rows    []map[string]any
}

// ParseCSV parses r as a CSV file whose first row is a header naming each
// column (tasks.md 7.4). See the package doc for the exact null-vs-empty-
// string convention every non-header cell is decoded under. An empty file
// (zero bytes, or only blank lines) returns a ParsedFile with no columns and
// no rows, not an error.
func ParseCSV(r io.Reader) (*ParsedFile, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("read CSV: %w", err)
	}

	records, err := parseCSVRecords(string(data))
	if err != nil {
		return nil, fmt.Errorf("parse CSV: %w", err)
	}
	if len(records) == 0 {
		return &ParsedFile{}, nil
	}

	header := records[0]
	columns := make([]string, len(header))
	for i, field := range header {
		columns[i] = field.Value
	}

	rows := make([]map[string]any, 0, len(records)-1)
	for _, record := range records[1:] {
		row := make(map[string]any, len(columns))
		for i, column := range columns {
			if i >= len(record) {
				row[column] = nil
				continue
			}
			field := record[i]
			if field.Value == "" && !field.Quoted {
				row[column] = nil
			} else {
				row[column] = field.Value
			}
		}
		rows = append(rows, row)
	}

	return &ParsedFile{Columns: columns, Rows: rows}, nil
}

// ParseJSON parses r as a JSON array of flat objects (tasks.md 7.4). Each
// object's keys become that row's columns; ParsedFile.Columns is the sorted
// union of every key seen across every object (see ParsedFile's doc
// comment), for the caller's own display/summary purposes only.
func ParseJSON(r io.Reader) (*ParsedFile, error) {
	var rows []map[string]any
	if err := json.NewDecoder(r).Decode(&rows); err != nil {
		return nil, fmt.Errorf("parse JSON: %w", err)
	}

	columnSet := make(map[string]struct{})
	for _, row := range rows {
		for key := range row {
			columnSet[key] = struct{}{}
		}
	}
	columns := make([]string, 0, len(columnSet))
	for key := range columnSet {
		columns = append(columns, key)
	}
	sort.Strings(columns)

	if rows == nil {
		rows = []map[string]any{}
	}
	return &ParsedFile{Columns: columns, Rows: rows}, nil
}
