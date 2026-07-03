package importdata

import (
	"strings"
	"testing"
)

func TestParseCSV_NullVsEmptyStringConvention(t *testing.T) {
	csv := "id,name,notes\n1,Ada,\n2,\"\",\"has value\"\n"
	file, err := ParseCSV(strings.NewReader(csv))
	if err != nil {
		t.Fatalf("ParseCSV() error = %v", err)
	}

	if got, want := file.Columns, []string{"id", "name", "notes"}; !equalStrings(got, want) {
		t.Fatalf("Columns = %v, want %v", got, want)
	}
	if len(file.Rows) != 2 {
		t.Fatalf("len(Rows) = %d, want 2", len(file.Rows))
	}

	row1 := file.Rows[0]
	if row1["id"] != "1" || row1["name"] != "Ada" {
		t.Errorf("row1 = %v, want id=1 name=Ada", row1)
	}
	if row1["notes"] != nil {
		t.Errorf("row1[notes] = %#v, want nil (unquoted empty cell = NULL)", row1["notes"])
	}

	row2 := file.Rows[1]
	if row2["id"] != "2" {
		t.Errorf("row2[id] = %v, want 2", row2["id"])
	}
	if row2["name"] != "" {
		t.Errorf("row2[name] = %#v, want empty string (quoted \"\" = empty string, not NULL)", row2["name"])
	}
	if row2["notes"] != "has value" {
		t.Errorf("row2[notes] = %#v, want %q", row2["notes"], "has value")
	}
}

func TestParseCSV_QuotedFieldWithEmbeddedCommaAndNewline(t *testing.T) {
	csv := "id,description\n1,\"multi, part\nvalue\"\n"
	file, err := ParseCSV(strings.NewReader(csv))
	if err != nil {
		t.Fatalf("ParseCSV() error = %v", err)
	}
	if len(file.Rows) != 1 {
		t.Fatalf("len(Rows) = %d, want 1", len(file.Rows))
	}
	want := "multi, part\nvalue"
	if got := file.Rows[0]["description"]; got != want {
		t.Errorf("description = %q, want %q", got, want)
	}
}

func TestParseCSV_DoubledQuoteEscape(t *testing.T) {
	csv := "id,quote\n1,\"she said \"\"hi\"\"\"\n"
	file, err := ParseCSV(strings.NewReader(csv))
	if err != nil {
		t.Fatalf("ParseCSV() error = %v", err)
	}
	want := `she said "hi"`
	if got := file.Rows[0]["quote"]; got != want {
		t.Errorf("quote = %q, want %q", got, want)
	}
}

func TestParseCSV_EmptyFile(t *testing.T) {
	file, err := ParseCSV(strings.NewReader(""))
	if err != nil {
		t.Fatalf("ParseCSV() error = %v", err)
	}
	if len(file.Columns) != 0 || len(file.Rows) != 0 {
		t.Errorf("ParseCSV(\"\") = %+v, want an empty ParsedFile", file)
	}
}

func TestParseCSV_HeaderOnly(t *testing.T) {
	file, err := ParseCSV(strings.NewReader("id,name\n"))
	if err != nil {
		t.Fatalf("ParseCSV() error = %v", err)
	}
	if len(file.Rows) != 0 {
		t.Errorf("len(Rows) = %d, want 0 for a header-only file", len(file.Rows))
	}
}

func TestParseJSON_RaggedObjectsAndNullVsMissing(t *testing.T) {
	body := `[
		{"id": 1, "name": "Ada", "notes": null},
		{"id": 2, "name": "Grace"}
	]`
	file, err := ParseJSON(strings.NewReader(body))
	if err != nil {
		t.Fatalf("ParseJSON() error = %v", err)
	}

	if got, want := file.Columns, []string{"id", "name", "notes"}; !equalStrings(got, want) {
		t.Fatalf("Columns = %v, want %v (sorted union of all observed keys)", got, want)
	}
	if len(file.Rows) != 2 {
		t.Fatalf("len(Rows) = %d, want 2", len(file.Rows))
	}

	row1 := file.Rows[0]
	if val, ok := row1["notes"]; !ok || val != nil {
		t.Errorf("row1[notes] = (%v, present=%v), want (nil, true) for an explicit JSON null", val, ok)
	}

	row2 := file.Rows[1]
	if _, ok := row2["notes"]; ok {
		t.Errorf("row2 has a \"notes\" key %v, want it entirely absent (never provided)", row2["notes"])
	}
}

func TestParseJSON_EmptyArray(t *testing.T) {
	file, err := ParseJSON(strings.NewReader("[]"))
	if err != nil {
		t.Fatalf("ParseJSON() error = %v", err)
	}
	if len(file.Columns) != 0 || len(file.Rows) != 0 {
		t.Errorf("ParseJSON(\"[]\") = %+v, want an empty ParsedFile", file)
	}
}

func TestParseJSON_MalformedInput(t *testing.T) {
	if _, err := ParseJSON(strings.NewReader(`{"not": "an array"}`)); err == nil {
		t.Error("ParseJSON() on a bare object, want an error")
	}
	if _, err := ParseJSON(strings.NewReader(`[{"a": 1,}]`)); err == nil {
		t.Error("ParseJSON() on malformed JSON, want an error")
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
