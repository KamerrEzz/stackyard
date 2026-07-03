package dbengine

import (
	"reflect"
	"testing"
)

func TestBuildSelectTableRows(t *testing.T) {
	cases := []struct {
		name       string
		dialect    Dialect
		schema     string
		table      string
		limit      int
		offset     int
		wantSQL    string
		wantParams []any
	}{
		{
			name:       "postgres with schema",
			dialect:    DialectPostgres,
			schema:     "public",
			table:      "widgets",
			limit:      50,
			offset:     100,
			wantSQL:    `SELECT * FROM "public"."widgets" LIMIT $1 OFFSET $2`,
			wantParams: []any{50, 100},
		},
		{
			name:       "mysql with schema",
			dialect:    DialectMySQL,
			schema:     "shop",
			table:      "widgets",
			limit:      50,
			offset:     0,
			wantSQL:    "SELECT * FROM `shop`.`widgets` LIMIT ? OFFSET ?",
			wantParams: []any{50, 0},
		},
		{
			name:       "postgres without schema",
			dialect:    DialectPostgres,
			schema:     "",
			table:      "widgets",
			limit:      10,
			offset:     0,
			wantSQL:    `SELECT * FROM "widgets" LIMIT $1 OFFSET $2`,
			wantParams: []any{10, 0},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotSQL, gotParams := BuildSelectTableRows(tc.dialect, tc.schema, tc.table, tc.limit, tc.offset)
			if gotSQL != tc.wantSQL {
				t.Errorf("BuildSelectTableRows() sql = %q, want %q", gotSQL, tc.wantSQL)
			}
			if !reflect.DeepEqual(gotParams, tc.wantParams) {
				t.Errorf("BuildSelectTableRows() params = %v, want %v", gotParams, tc.wantParams)
			}
		})
	}
}

func TestBuildUpdateRow(t *testing.T) {
	t.Run("postgres single-column primary key", func(t *testing.T) {
		sql, args, err := BuildUpdateRow(DialectPostgres, "public", "widgets", "name",
			"new-name", []string{"id"}, map[string]any{"id": int64(42)})
		if err != nil {
			t.Fatalf("BuildUpdateRow() error = %v", err)
		}
		wantSQL := `UPDATE "public"."widgets" SET "name" = $1 WHERE "id" = $2`
		if sql != wantSQL {
			t.Errorf("sql = %q, want %q", sql, wantSQL)
		}
		wantArgs := []any{"new-name", int64(42)}
		if !reflect.DeepEqual(args, wantArgs) {
			t.Errorf("args = %v, want %v", args, wantArgs)
		}
	})

	t.Run("mysql composite primary key, columns sorted for determinism", func(t *testing.T) {
		sql, args, err := BuildUpdateRow(DialectMySQL, "shop", "order_items", "quantity",
			3, []string{"order_id", "product_id"},
			map[string]any{"product_id": int64(7), "order_id": int64(5)})
		if err != nil {
			t.Fatalf("BuildUpdateRow() error = %v", err)
		}
		wantSQL := "UPDATE `shop`.`order_items` SET `quantity` = ? WHERE `order_id` = ? AND `product_id` = ?"
		if sql != wantSQL {
			t.Errorf("sql = %q, want %q", sql, wantSQL)
		}
		wantArgs := []any{3, int64(5), int64(7)}
		if !reflect.DeepEqual(args, wantArgs) {
			t.Errorf("args = %v, want %v", args, wantArgs)
		}
	})

	t.Run("missing pk value errors", func(t *testing.T) {
		_, _, err := BuildUpdateRow(DialectPostgres, "public", "widgets", "name",
			"x", []string{"id"}, map[string]any{})
		if err == nil {
			t.Fatal("expected an error when pkValues is missing a required primary key column")
		}
	})
}

func TestBuildInsertRow(t *testing.T) {
	t.Run("postgres uses RETURNING star", func(t *testing.T) {
		sql, args := BuildInsertRow(DialectPostgres, "public", "widgets",
			[]string{"name", "price"}, map[string]any{"name": "widget", "price": 9.99})
		wantSQL := `INSERT INTO "public"."widgets" ("name", "price") VALUES ($1, $2) RETURNING *`
		if sql != wantSQL {
			t.Errorf("sql = %q, want %q", sql, wantSQL)
		}
		wantArgs := []any{"widget", 9.99}
		if !reflect.DeepEqual(args, wantArgs) {
			t.Errorf("args = %v, want %v", args, wantArgs)
		}
	})

	t.Run("mysql has no RETURNING clause", func(t *testing.T) {
		sql, args := BuildInsertRow(DialectMySQL, "shop", "widgets",
			[]string{"name"}, map[string]any{"name": "widget"})
		wantSQL := "INSERT INTO `shop`.`widgets` (`name`) VALUES (?)"
		if sql != wantSQL {
			t.Errorf("sql = %q, want %q", sql, wantSQL)
		}
		wantArgs := []any{"widget"}
		if !reflect.DeepEqual(args, wantArgs) {
			t.Errorf("args = %v, want %v", args, wantArgs)
		}
	})
}

func TestBuildDeleteRow(t *testing.T) {
	t.Run("postgres single-column primary key", func(t *testing.T) {
		sql, args, err := BuildDeleteRow(DialectPostgres, "public", "widgets",
			[]string{"id"}, map[string]any{"id": int64(9)})
		if err != nil {
			t.Fatalf("BuildDeleteRow() error = %v", err)
		}
		wantSQL := `DELETE FROM "public"."widgets" WHERE "id" = $1`
		if sql != wantSQL {
			t.Errorf("sql = %q, want %q", sql, wantSQL)
		}
		wantArgs := []any{int64(9)}
		if !reflect.DeepEqual(args, wantArgs) {
			t.Errorf("args = %v, want %v", args, wantArgs)
		}
	})

	t.Run("mysql composite primary key", func(t *testing.T) {
		sql, args, err := BuildDeleteRow(DialectMySQL, "", "order_items",
			[]string{"order_id", "product_id"},
			map[string]any{"order_id": int64(5), "product_id": int64(7)})
		if err != nil {
			t.Fatalf("BuildDeleteRow() error = %v", err)
		}
		wantSQL := "DELETE FROM `order_items` WHERE `order_id` = ? AND `product_id` = ?"
		if sql != wantSQL {
			t.Errorf("sql = %q, want %q", sql, wantSQL)
		}
		wantArgs := []any{int64(5), int64(7)}
		if !reflect.DeepEqual(args, wantArgs) {
			t.Errorf("args = %v, want %v", args, wantArgs)
		}
	})

	t.Run("missing pk value errors", func(t *testing.T) {
		_, _, err := BuildDeleteRow(DialectPostgres, "public", "widgets",
			[]string{"id"}, map[string]any{"wrong_key": int64(1)})
		if err == nil {
			t.Fatal("expected an error when pkValues is missing a required primary key column")
		}
	})
}

func TestBuildSelectRowByPK(t *testing.T) {
	t.Run("postgres single-column primary key", func(t *testing.T) {
		sql, args, err := BuildSelectRowByPK(DialectPostgres, "public", "widgets",
			[]string{"id"}, map[string]any{"id": int64(9)})
		if err != nil {
			t.Fatalf("BuildSelectRowByPK() error = %v", err)
		}
		wantSQL := `SELECT * FROM "public"."widgets" WHERE "id" = $1`
		if sql != wantSQL {
			t.Errorf("sql = %q, want %q", sql, wantSQL)
		}
		wantArgs := []any{int64(9)}
		if !reflect.DeepEqual(args, wantArgs) {
			t.Errorf("args = %v, want %v", args, wantArgs)
		}
	})

	t.Run("missing pk value errors", func(t *testing.T) {
		_, _, err := BuildSelectRowByPK(DialectMySQL, "", "widgets",
			[]string{"id"}, map[string]any{})
		if err == nil {
			t.Fatal("expected an error when pkValues is missing a required primary key column")
		}
	})
}

func TestQuoteIdentifier(t *testing.T) {
	cases := []struct {
		dialect Dialect
		ident   string
		want    string
	}{
		{DialectPostgres, "widgets", `"widgets"`},
		{DialectPostgres, `weird"name`, `"weird""name"`},
		{DialectMySQL, "widgets", "`widgets`"},
		{DialectMySQL, "weird`name", "`weird``name`"},
	}
	for _, tc := range cases {
		got := QuoteIdentifier(tc.dialect, tc.ident)
		if got != tc.want {
			t.Errorf("QuoteIdentifier(%q, %q) = %q, want %q", tc.dialect, tc.ident, got, tc.want)
		}
	}
}
