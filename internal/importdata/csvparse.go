package importdata

import (
	"fmt"
	"strings"
)

// csvField is one raw field read from a CSV document, retaining whether the
// original text wrapped it in double quotes — see importdata.go's package
// doc for why this bit matters and why encoding/csv cannot provide it.
type csvField struct {
	Value  string
	Quoted bool
}

// parseCSVRecords tokenizes data as CSV: comma-delimited fields, double
// quotes for a field that itself contains a comma/newline/quote, and a
// doubled double-quote ("") as the escape for a literal quote character
// inside a quoted field — the same RFC4180 rules encoding/csv implements,
// reimplemented here only so each field's original quoting survives (see
// importdata.go's package doc). A completely blank line (no characters at
// all before its line terminator) is skipped rather than producing a
// one-field empty record, matching encoding/csv's own "blank lines are
// ignored" behavior. A quote character appearing anywhere other than as the
// very first character of a field is treated as a literal character rather
// than a parse error (matching encoding/csv's LazyQuotes mode) — this is a
// deliberately lenient subset of RFC4180, not a fully strict parser, which
// is an acceptable, documented scope for an import feature reading files the
// user themselves is expected to have produced from a spreadsheet or a
// database export.
func parseCSVRecords(data string) ([][]csvField, error) {
	var records [][]csvField
	var fields []csvField
	var buf strings.Builder

	quoted := false
	inQuotes := false
	fieldStarted := false

	endField := func() {
		fields = append(fields, csvField{Value: buf.String(), Quoted: quoted})
		buf.Reset()
		quoted = false
		fieldStarted = false
	}
	endRecord := func() {
		endField()
		if len(fields) == 1 && fields[0].Value == "" && !fields[0].Quoted {
			fields = nil
			return
		}
		records = append(records, fields)
		fields = nil
	}

	i, n := 0, len(data)
	for i < n {
		c := data[i]
		switch {
		case inQuotes:
			if c == '"' {
				if i+1 < n && data[i+1] == '"' {
					buf.WriteByte('"')
					i += 2
					continue
				}
				inQuotes = false
				i++
				continue
			}
			buf.WriteByte(c)
			i++
		case c == '"' && !fieldStarted:
			inQuotes = true
			quoted = true
			fieldStarted = true
			i++
		case c == ',':
			endField()
			i++
		case c == '\r':
			i++
		case c == '\n':
			endRecord()
			i++
		default:
			buf.WriteByte(c)
			fieldStarted = true
			i++
		}
	}
	if inQuotes {
		return nil, fmt.Errorf("unterminated quoted field")
	}
	if buf.Len() > 0 || fieldStarted || len(fields) > 0 {
		endRecord()
	}
	return records, nil
}
