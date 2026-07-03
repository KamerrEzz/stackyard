package export

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// ToJSON renders columnNames/rows as a JSON array of one object per row
// (tasks.md 7.2), each object's keys in columnNames' original order — plain
// map[string]any would let encoding/json re-sort keys alphabetically, losing
// the table's/result set's own column order, so each row is instead wrapped
// in orderedJSONRow, whose MarshalJSON writes keys in the given order
// directly. NULL vs. empty string needs no special handling the way ToCSV's
// does: a SQL NULL arrives here as a Go nil and marshals to JSON's own
// `null`, while an empty string marshals to `""` — already distinguishable
// via JSON's own grammar. Output is indented (two spaces) for a human-
// readable export file, not compacted.
func ToJSON(columnNames []string, rows [][]any) (string, error) {
	var buf bytes.Buffer
	buf.WriteByte('[')
	for i, row := range rows {
		if i > 0 {
			buf.WriteByte(',')
		}
		encoded, err := json.Marshal(orderedJSONRow{columnNames: columnNames, row: row})
		if err != nil {
			return "", fmt.Errorf("export: encode row %d as json: %w", i, err)
		}
		buf.Write(encoded)
	}
	buf.WriteByte(']')

	var pretty bytes.Buffer
	if err := json.Indent(&pretty, buf.Bytes(), "", "  "); err != nil {
		return "", fmt.Errorf("export: indent json: %w", err)
	}
	return pretty.String(), nil
}

// orderedJSONRow marshals one row as a JSON object with keys in columnNames'
// exact order, the same order dbengine.QueryResult.Columns/ColumnInfo report
// them in — see ToJSON's doc comment for why plain map[string]any can't do
// this on its own.
type orderedJSONRow struct {
	columnNames []string
	row         []any
}

func (o orderedJSONRow) MarshalJSON() ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteByte('{')
	for i, name := range o.columnNames {
		if i > 0 {
			buf.WriteByte(',')
		}
		keyJSON, err := json.Marshal(name)
		if err != nil {
			return nil, err
		}
		buf.Write(keyJSON)
		buf.WriteByte(':')

		var v any
		if i < len(o.row) {
			v = o.row[i]
		}
		valueJSON, err := json.Marshal(normalizeJSONValue(v))
		if err != nil {
			return nil, err
		}
		buf.Write(valueJSON)
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}
