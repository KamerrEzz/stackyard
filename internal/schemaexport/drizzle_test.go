package schemaexport

import (
	"strings"
	"testing"

	"stackyard/internal/dbengine"
)

func TestBuildDrizzleSchema_SimpleTableWithPrimaryKey(t *testing.T) {
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

	want := "import { integer, numeric, pgTable, varchar } from \"drizzle-orm/pg-core\";\n" +
		"\n" +
		"export const widgets = pgTable(\"widgets\", {\n" +
		"  id: integer(\"id\").notNull().primaryKey(),\n" +
		"  name: varchar(\"name\").notNull(),\n" +
		"  weight: numeric(\"weight\"),\n" +
		"});\n"

	if got := BuildDrizzleSchema(dbengine.DialectPostgres, tables, nil); got != want {
		t.Errorf("BuildDrizzleSchema() =\n%s\nwant:\n%s", got, want)
	}
}

func TestBuildDrizzleSchema_PostgresTypeMapping(t *testing.T) {
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

	want := "export const kitchenSink = pgTable(\"kitchen_sink\", {\n" +
		"  colVarchar: varchar(\"col_varchar\"),\n" +
		"  colUuid: uuid(\"col_uuid\"),\n" +
		"  colInt: integer(\"col_int\"),\n" +
		"  colBigint: bigint(\"col_bigint\", { mode: \"number\" }),\n" +
		"  colBool: boolean(\"col_bool\"),\n" +
		"  colNumeric: numeric(\"col_numeric\"),\n" +
		"  colDouble: doublePrecision(\"col_double\"),\n" +
		"  colTimestamp: timestamp(\"col_timestamp\"),\n" +
		"  colJson: jsonb(\"col_json\"),\n" +
		"  colBytes: text(\"col_bytes\"),\n" +
		"  colUnknown: text(\"col_unknown\"),\n" +
		"});\n"

	got := BuildDrizzleSchema(dbengine.DialectPostgres, tables, nil)
	if !strings.Contains(got, want) {
		t.Errorf("BuildDrizzleSchema() table block not found or mismatched.\ngot:\n%s\nwant table block:\n%s", got, want)
	}
}

func TestBuildDrizzleSchema_MySQLTypeMapping(t *testing.T) {
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

	want := "export const kitchenSinkMysql = mysqlTable(\"kitchen_sink_mysql\", {\n" +
		"  colVarchar: varchar(\"col_varchar\", { length: 255 }),\n" +
		"  colInt: int(\"col_int\"),\n" +
		"  colTinyint: tinyint(\"col_tinyint\"),\n" +
		"  colBigint: bigint(\"col_bigint\", { mode: \"number\" }),\n" +
		"  colDecimal: decimal(\"col_decimal\"),\n" +
		"  colFloat: float(\"col_float\"),\n" +
		"  colDatetime: datetime(\"col_datetime\"),\n" +
		"  colJson: json(\"col_json\"),\n" +
		"  colBlob: text(\"col_blob\"),\n" +
		"  colEnum: text(\"col_enum\"),\n" +
		"  colUnknown: text(\"col_unknown\"),\n" +
		"});\n"

	got := BuildDrizzleSchema(dbengine.DialectMySQL, tables, nil)
	if !strings.Contains(got, want) {
		t.Errorf("BuildDrizzleSchema() table block not found or mismatched.\ngot:\n%s\nwant table block:\n%s", got, want)
	}
}

func TestBuildDrizzleSchema_CompositePrimaryKey(t *testing.T) {
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

	want := "export const memberships = pgTable(\"memberships\", {\n" +
		"  id: integer(\"id\").notNull(),\n" +
		"  tenantId: integer(\"tenant_id\").notNull(),\n" +
		"  role: text(\"role\").notNull(),\n" +
		"}, (table) => ({\n" +
		"  pk: primaryKey({ columns: [table.id, table.tenantId] }),\n" +
		"}));\n"

	got := BuildDrizzleSchema(dbengine.DialectPostgres, tables, nil)
	if !strings.Contains(got, want) {
		t.Errorf("BuildDrizzleSchema() table block not found or mismatched.\ngot:\n%s\nwant table block:\n%s", got, want)
	}
	if !strings.Contains(got, "primaryKey") || !strings.HasPrefix(got, "import {") {
		t.Errorf("BuildDrizzleSchema() import header missing primaryKey:\n%s", got)
	}
}

func TestBuildDrizzleSchema_ForeignKeyBetweenTwoTables(t *testing.T) {
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

	want := "export const books = pgTable(\"books\", {\n" +
		"  id: integer(\"id\").notNull().primaryKey(),\n" +
		"  title: text(\"title\").notNull(),\n" +
		"  authorId: integer(\"author_id\").notNull().references(() => authors.id),\n" +
		"});\n"

	got := BuildDrizzleSchema(dbengine.DialectPostgres, tables, foreignKeys)
	if !strings.Contains(got, want) {
		t.Errorf("BuildDrizzleSchema() table block not found or mismatched.\ngot:\n%s\nwant table block:\n%s", got, want)
	}
}
