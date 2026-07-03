package schemaexport

import (
	"testing"

	"stackyard/internal/dbengine"
)

func TestBuildPrismaSchema_SimpleTableWithPrimaryKey(t *testing.T) {
	tables := []dbengine.TableInfo{
		{
			Name: "widgets",
			Columns: []dbengine.ColumnInfo{
				{Name: "id", DataType: "integer", Nullable: false, IsPrimaryKey: true},
				{Name: "name", DataType: "character varying", Nullable: false},
				{Name: "weight", DataType: "numeric", Nullable: true},
			},
		},
	}

	want := "datasource db {\n" +
		"  provider = \"postgresql\"\n" +
		"  url      = env(\"DATABASE_URL\")\n" +
		"}\n" +
		"\n" +
		"generator client {\n" +
		"  provider = \"prisma-client-js\"\n" +
		"}\n" +
		"\n" +
		"model widgets {\n" +
		"  id Int @id\n" +
		"  name String\n" +
		"  weight Decimal?\n" +
		"}\n"

	if got := BuildPrismaSchema(dbengine.DialectPostgres, tables, nil); got != want {
		t.Errorf("BuildPrismaSchema() =\n%s\nwant:\n%s", got, want)
	}
}

func TestBuildPrismaSchema_PostgresTypeMapping(t *testing.T) {
	tables := []dbengine.TableInfo{
		{
			Name: "kitchen_sink",
			Columns: []dbengine.ColumnInfo{
				{Name: "col_varchar", DataType: "character varying", Nullable: true},
				{Name: "col_uuid", DataType: "uuid", Nullable: true},
				{Name: "col_int", DataType: "integer", Nullable: true},
				{Name: "col_bigint", DataType: "bigint", Nullable: true},
				{Name: "col_bool", DataType: "boolean", Nullable: true},
				{Name: "col_numeric", DataType: "numeric", Nullable: true},
				{Name: "col_double", DataType: "double precision", Nullable: true},
				{Name: "col_timestamp", DataType: "timestamp without time zone", Nullable: true},
				{Name: "col_json", DataType: "jsonb", Nullable: true},
				{Name: "col_bytes", DataType: "bytea", Nullable: true},
				{Name: "col_unknown", DataType: "tsvector", Nullable: true},
			},
		},
	}

	want := "model kitchen_sink {\n" +
		"  col_varchar String?\n" +
		"  col_uuid String?\n" +
		"  col_int Int?\n" +
		"  col_bigint BigInt?\n" +
		"  col_bool Boolean?\n" +
		"  col_numeric Decimal?\n" +
		"  col_double Float?\n" +
		"  col_timestamp DateTime?\n" +
		"  col_json Json?\n" +
		"  col_bytes Bytes?\n" +
		"  col_unknown String?\n" +
		"}\n"

	got := BuildPrismaSchema(dbengine.DialectPostgres, tables, nil)
	if !containsAfterHeader(got, want) {
		t.Errorf("BuildPrismaSchema() model block not found or mismatched.\ngot:\n%s\nwant model block:\n%s", got, want)
	}
}

func TestBuildPrismaSchema_MySQLTypeMapping(t *testing.T) {
	tables := []dbengine.TableInfo{
		{
			Name: "kitchen_sink_mysql",
			Columns: []dbengine.ColumnInfo{
				{Name: "col_varchar", DataType: "varchar", Nullable: true},
				{Name: "col_int", DataType: "int", Nullable: true},
				{Name: "col_tinyint", DataType: "tinyint", Nullable: true},
				{Name: "col_bigint", DataType: "bigint", Nullable: true},
				{Name: "col_decimal", DataType: "decimal", Nullable: true},
				{Name: "col_float", DataType: "float", Nullable: true},
				{Name: "col_datetime", DataType: "datetime", Nullable: true},
				{Name: "col_json", DataType: "json", Nullable: true},
				{Name: "col_blob", DataType: "blob", Nullable: true},
				{Name: "col_enum", DataType: "enum", Nullable: true},
				{Name: "col_unknown", DataType: "geometry", Nullable: true},
			},
		},
	}

	want := "model kitchen_sink_mysql {\n" +
		"  col_varchar String?\n" +
		"  col_int Int?\n" +
		"  col_tinyint Int?\n" +
		"  col_bigint BigInt?\n" +
		"  col_decimal Decimal?\n" +
		"  col_float Float?\n" +
		"  col_datetime DateTime?\n" +
		"  col_json Json?\n" +
		"  col_blob Bytes?\n" +
		"  col_enum String?\n" +
		"  col_unknown String?\n" +
		"}\n"

	got := BuildPrismaSchema(dbengine.DialectMySQL, tables, nil)
	if !containsAfterHeader(got, want) {
		t.Errorf("BuildPrismaSchema() model block not found or mismatched.\ngot:\n%s\nwant model block:\n%s", got, want)
	}
}

func TestBuildPrismaSchema_CompositePrimaryKey(t *testing.T) {
	tables := []dbengine.TableInfo{
		{
			Name: "memberships",
			Columns: []dbengine.ColumnInfo{
				{Name: "id", DataType: "integer", Nullable: false, IsPrimaryKey: true},
				{Name: "tenant_id", DataType: "integer", Nullable: false, IsPrimaryKey: true},
				{Name: "role", DataType: "text", Nullable: false},
			},
		},
	}

	want := "model memberships {\n" +
		"  id Int\n" +
		"  tenant_id Int\n" +
		"  role String\n" +
		"\n" +
		"  @@id([id, tenant_id])\n" +
		"}\n"

	got := BuildPrismaSchema(dbengine.DialectPostgres, tables, nil)
	if !containsAfterHeader(got, want) {
		t.Errorf("BuildPrismaSchema() model block not found or mismatched.\ngot:\n%s\nwant model block:\n%s", got, want)
	}
}

func TestBuildPrismaSchema_ForeignKeyBetweenTwoTables(t *testing.T) {
	tables := []dbengine.TableInfo{
		{
			Name: "authors",
			Columns: []dbengine.ColumnInfo{
				{Name: "id", DataType: "integer", Nullable: false, IsPrimaryKey: true},
				{Name: "name", DataType: "text", Nullable: false},
			},
		},
		{
			Name: "books",
			Columns: []dbengine.ColumnInfo{
				{Name: "id", DataType: "integer", Nullable: false, IsPrimaryKey: true},
				{Name: "title", DataType: "text", Nullable: false},
				{Name: "author_id", DataType: "integer", Nullable: false},
			},
		},
	}
	foreignKeys := []dbengine.ForeignKey{
		{TableName: "books", ColumnName: "author_id", ReferencedTable: "authors", ReferencedColumn: "id"},
	}

	want := "datasource db {\n" +
		"  provider = \"postgresql\"\n" +
		"  url      = env(\"DATABASE_URL\")\n" +
		"}\n" +
		"\n" +
		"generator client {\n" +
		"  provider = \"prisma-client-js\"\n" +
		"}\n" +
		"\n" +
		"model authors {\n" +
		"  id Int @id\n" +
		"  name String\n" +
		"  author_books books[]\n" +
		"}\n" +
		"\n" +
		"model books {\n" +
		"  id Int @id\n" +
		"  title String\n" +
		"  author_id Int\n" +
		"  author authors @relation(fields: [author_id], references: [id])\n" +
		"}\n"

	got := BuildPrismaSchema(dbengine.DialectPostgres, tables, foreignKeys)
	if got != want {
		t.Errorf("BuildPrismaSchema() =\n%s\nwant:\n%s", got, want)
	}
}

// containsAfterHeader reports whether got, once the shared datasource/
// generator header is stripped off, contains wantModelBlock. Used by
// type-mapping tests that only care about one table's model block, not the
// repeated boilerplate header every BuildPrismaSchema output carries.
func containsAfterHeader(got, wantModelBlock string) bool {
	const marker = "generator client {\n  provider = \"prisma-client-js\"\n}\n\n"
	idx := indexOf(got, marker)
	if idx == -1 {
		return false
	}
	body := got[idx+len(marker):]
	return body == wantModelBlock
}

func indexOf(s, substr string) int {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
