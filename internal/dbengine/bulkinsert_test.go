package dbengine

import "testing"

func TestBuildBulkInsertRows_Postgres(t *testing.T) {
	rows := []map[string]any{
		{"id": 1, "name": "bolt"},
		{"id": 2, "name": "nut"},
	}
	sql, args := BuildBulkInsertRows(DialectPostgres, "public", "widgets", []string{"id", "name"}, rows)

	wantSQL := `INSERT INTO "public"."widgets" ("id", "name") VALUES ($1, $2), ($3, $4)`
	if sql != wantSQL {
		t.Errorf("sql = %q, want %q", sql, wantSQL)
	}
	wantArgs := []any{1, "bolt", 2, "nut"}
	if len(args) != len(wantArgs) {
		t.Fatalf("len(args) = %d, want %d", len(args), len(wantArgs))
	}
	for i := range wantArgs {
		if args[i] != wantArgs[i] {
			t.Errorf("args[%d] = %v, want %v", i, args[i], wantArgs[i])
		}
	}
}

func TestBuildBulkInsertRows_MySQL(t *testing.T) {
	rows := []map[string]any{
		{"id": 1, "name": "bolt"},
	}
	sql, args := BuildBulkInsertRows(DialectMySQL, "", "widgets", []string{"id", "name"}, rows)

	wantSQL := "INSERT INTO `widgets` (`id`, `name`) VALUES (?, ?)"
	if sql != wantSQL {
		t.Errorf("sql = %q, want %q", sql, wantSQL)
	}
	if len(args) != 2 || args[0] != 1 || args[1] != "bolt" {
		t.Errorf("args = %v, want [1 bolt]", args)
	}
}

func TestBuildBulkInsertRows_MissingKeyBindsNil(t *testing.T) {
	rows := []map[string]any{
		{"id": 1},
	}
	_, args := BuildBulkInsertRows(DialectPostgres, "", "widgets", []string{"id", "name"}, rows)

	if len(args) != 2 {
		t.Fatalf("len(args) = %d, want 2", len(args))
	}
	if args[1] != nil {
		t.Errorf("args[1] = %v, want nil for a row missing the \"name\" key", args[1])
	}
}

func TestBuildBulkInsertRows_EmptyRows(t *testing.T) {
	sql, args := BuildBulkInsertRows(DialectPostgres, "", "widgets", []string{"id"}, nil)
	wantSQL := `INSERT INTO "widgets" ("id") VALUES `
	if sql != wantSQL {
		t.Errorf("sql = %q, want %q", sql, wantSQL)
	}
	if len(args) != 0 {
		t.Errorf("len(args) = %d, want 0", len(args))
	}
}
