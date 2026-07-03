package importdata

import (
	"testing"

	"stackyard/internal/dbengine"
)

func widgetsTable() dbengine.TableInfo {
	return dbengine.TableInfo{
		Name: "widgets",
		Columns: []dbengine.ColumnInfo{
			{Name: "id", DataType: "integer", Nullable: false, IsPrimaryKey: true},
			{Name: "name", DataType: "character varying", Nullable: false},
			{Name: "weight", DataType: "numeric", Nullable: true},
			{Name: "in_stock", DataType: "boolean", Nullable: true},
			{Name: "restocked_at", DataType: "timestamp without time zone", Nullable: true},
		},
	}
}

func TestValidate_UnknownColumn(t *testing.T) {
	file := &ParsedFile{
		Columns: []string{"id", "name", "not_a_real_column"},
		Rows: []map[string]any{
			{"id": "1", "name": "bolt", "not_a_real_column": "x"},
		},
	}

	report := Validate(file, widgetsTable())

	if len(report.Mismatches) == 0 {
		t.Fatal("Validate() reported zero mismatches, want a mismatch for the unknown column")
	}
	found := false
	for _, m := range report.Mismatches {
		if m.Column == "not_a_real_column" && m.RowIndex == -1 {
			found = true
		}
	}
	if !found {
		t.Errorf("Mismatches = %+v, want a RowIndex=-1 entry for %q", report.Mismatches, "not_a_real_column")
	}
}

func TestValidate_NumericTypeMismatch(t *testing.T) {
	file := &ParsedFile{
		Columns: []string{"id", "name", "weight"},
		Rows: []map[string]any{
			{"id": "1", "name": "bolt", "weight": "not-a-number"},
		},
	}

	report := Validate(file, widgetsTable())

	if len(report.Mismatches) == 0 {
		t.Fatal("Validate() reported zero mismatches, want a mismatch for a non-numeric weight")
	}
	if len(report.Mismatches) != 1 || report.Mismatches[0].Column != "weight" || report.Mismatches[0].RowIndex != 0 {
		t.Errorf("Mismatches = %+v, want exactly one weight mismatch at RowIndex 0", report.Mismatches)
	}
}

func TestValidate_DateTypeMismatch(t *testing.T) {
	file := &ParsedFile{
		Columns: []string{"id", "name", "restocked_at"},
		Rows: []map[string]any{
			{"id": "1", "name": "bolt", "restocked_at": "not-a-date"},
		},
	}

	report := Validate(file, widgetsTable())

	if len(report.Mismatches) == 0 {
		t.Fatal("Validate() reported zero mismatches, want a mismatch for an unparseable restocked_at")
	}
	if len(report.Mismatches) != 1 || report.Mismatches[0].Column != "restocked_at" {
		t.Errorf("Mismatches = %+v, want exactly one restocked_at mismatch", report.Mismatches)
	}
}

func TestValidate_BooleanTypeMismatch(t *testing.T) {
	file := &ParsedFile{
		Columns: []string{"id", "name", "in_stock"},
		Rows: []map[string]any{
			{"id": "1", "name": "bolt", "in_stock": "maybe"},
		},
	}

	report := Validate(file, widgetsTable())

	if len(report.Mismatches) == 0 {
		t.Fatal("Validate() reported zero mismatches, want a mismatch for an unrecognized boolean literal")
	}
	if len(report.Mismatches) != 1 || report.Mismatches[0].Column != "in_stock" {
		t.Errorf("Mismatches = %+v, want exactly one in_stock mismatch", report.Mismatches)
	}
}

func TestValidate_NullIntoNonNullableColumn(t *testing.T) {
	file := &ParsedFile{
		Columns: []string{"id", "name"},
		Rows: []map[string]any{
			{"id": "1", "name": nil},
		},
	}

	report := Validate(file, widgetsTable())

	if len(report.Mismatches) == 0 {
		t.Fatal("Validate() reported zero mismatches, want a mismatch for NULL into non-nullable name")
	}
	if len(report.Mismatches) != 1 || report.Mismatches[0].Column != "name" {
		t.Errorf("Mismatches = %+v, want exactly one name mismatch", report.Mismatches)
	}
}

func TestValidate_MissingKeyOnNonNullableColumnIsTreatedAsNull(t *testing.T) {
	file := &ParsedFile{
		Columns: []string{"id", "name"},
		Rows: []map[string]any{
			{"id": "1"},
		},
	}

	report := Validate(file, widgetsTable())

	if len(report.Mismatches) == 0 {
		t.Fatal("Validate() reported zero mismatches, want a mismatch for name entirely missing from the row")
	}
	if len(report.Mismatches) != 1 || report.Mismatches[0].Column != "name" {
		t.Errorf("Mismatches = %+v, want exactly one name mismatch", report.Mismatches)
	}
}

func TestValidate_GenuinelyValidFileHasNoMismatches(t *testing.T) {
	file := &ParsedFile{
		Columns: []string{"id", "name", "weight", "in_stock", "restocked_at"},
		Rows: []map[string]any{
			{"id": "1", "name": "bolt", "weight": "5.5", "in_stock": "true", "restocked_at": "2026-01-15"},
			{"id": "2", "name": "nut", "weight": nil, "in_stock": "0", "restocked_at": nil},
			{"id": "3", "name": "washer", "weight": "2", "in_stock": nil, "restocked_at": "2026-01-15 10:30:00"},
		},
	}

	report := Validate(file, widgetsTable())

	if len(report.Mismatches) != 0 {
		t.Fatalf("Validate() = %+v, want zero mismatches for a genuinely valid file", report.Mismatches)
	}
	if report.RowCount != 3 {
		t.Errorf("RowCount = %d, want 3", report.RowCount)
	}
}

func TestValidate_CollectsAllMismatchesAcrossWholeFile(t *testing.T) {
	file := &ParsedFile{
		Columns: []string{"id", "name", "weight"},
		Rows: []map[string]any{
			{"id": "1", "name": nil, "weight": "5"},
			{"id": "2", "name": "nut", "weight": "not-a-number"},
			{"id": "3", "name": "washer", "weight": "1"},
		},
	}

	report := Validate(file, widgetsTable())

	if len(report.Mismatches) != 2 {
		t.Fatalf("len(Mismatches) = %d, want 2 (one per bad row), got %+v", len(report.Mismatches), report.Mismatches)
	}
}

func TestValidate_JSONNativeTypesAreAcceptedDirectly(t *testing.T) {
	file := &ParsedFile{
		Columns: []string{"id", "name", "weight", "in_stock"},
		Rows: []map[string]any{
			{"id": float64(1), "name": "bolt", "weight": float64(5.5), "in_stock": true},
		},
	}

	report := Validate(file, widgetsTable())

	if len(report.Mismatches) != 0 {
		t.Fatalf("Validate() = %+v, want zero mismatches for native JSON number/bool values", report.Mismatches)
	}
}
