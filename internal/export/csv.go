package export

import "strings"

// csvField is one already-formatted CSV cell: text is empty and isNull is
// true for a genuine SQL NULL; text is the empty string ("") and isNull is
// false for a real empty-string value. encodeCSVField is the only place that
// distinguishes the two on output.
type csvField struct {
	text   string
	isNull bool
}

// ToCSV renders columnNames as a header row followed by one row per entry
// in rows (tasks.md 7.1), using \r\n line endings per RFC 4180. Fields are
// quoted only when they contain a comma, quote, or newline — except an
// empty-string value, which is always rendered as a quoted empty pair `""`
// specifically so it stays distinguishable from a completely empty,
// unquoted NULL cell on re-import (task 7.4). This is the same convention
// PostgreSQL's own COPY ... CSV uses for NULL vs. an empty string by default, not a
// bespoke one invented for this package. A row shorter than columnNames
// (should not happen for a well-formed QueryResult, but defended against
// anyway) pads missing trailing cells as NULL.
func ToCSV(columnNames []string, rows [][]any) (string, error) {
	var buf strings.Builder

	header := make([]csvField, len(columnNames))
	for i, name := range columnNames {
		header[i] = csvField{text: name}
	}
	writeCSVRecord(&buf, header)

	for _, row := range rows {
		fields := make([]csvField, len(columnNames))
		for i := range columnNames {
			var v any
			if i < len(row) {
				v = row[i]
			}
			text, isNull := formatCSVValue(v)
			fields[i] = csvField{text: text, isNull: isNull}
		}
		writeCSVRecord(&buf, fields)
	}

	return buf.String(), nil
}

func writeCSVRecord(buf *strings.Builder, fields []csvField) {
	for i, f := range fields {
		if i > 0 {
			buf.WriteByte(',')
		}
		buf.WriteString(encodeCSVField(f))
	}
	buf.WriteString("\r\n")
}

// encodeCSVField implements ToCSV's NULL-vs-empty-string convention: a NULL
// field renders as nothing at all (not even quotes), while an empty-string
// field always renders as `""` even though an unquoted empty field would be
// visually identical to nothing — the whole point of forcing quotes here.
func encodeCSVField(f csvField) string {
	if f.isNull {
		return ""
	}
	if f.text == "" {
		return `""`
	}
	if strings.ContainsAny(f.text, ",\"\r\n") {
		return `"` + strings.ReplaceAll(f.text, `"`, `""`) + `"`
	}
	return f.text
}
