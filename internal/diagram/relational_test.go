package diagram

import (
	"testing"

	"stackyard/internal/dbengine"
)

func TestBuildRelationalERDiagram_SelfReferencingForeignKey(t *testing.T) {
	tables := []dbengine.TableInfo{
		{
			Name: "employees",
			Columns: []dbengine.ColumnInfo{
				{Name: "id", DataType: "int4", IsPrimaryKey: true},
				{Name: "name", DataType: "text"},
				{Name: "manager_id", DataType: "int4"},
			},
		},
	}
	foreignKeys := []dbengine.ForeignKey{
		{TableName: "employees", ColumnName: "manager_id", ReferencedTable: "employees", ReferencedColumn: "id"},
	}

	want := "erDiagram\n" +
		"    employees {\n" +
		"        int4 id PK\n" +
		"        text name\n" +
		"        int4 manager_id FK\n" +
		"    }\n" +
		"    employees ||--o{ employees : \"via manager_id\"\n"

	if got := BuildRelationalERDiagram(tables, foreignKeys); got != want {
		t.Errorf("BuildRelationalERDiagram() =\n%s\nwant:\n%s", got, want)
	}
}

func TestBuildRelationalERDiagram_OneToManyAcrossTwoTables(t *testing.T) {
	tables := []dbengine.TableInfo{
		{
			Name: "authors",
			Columns: []dbengine.ColumnInfo{
				{Name: "id", DataType: "int4", IsPrimaryKey: true},
				{Name: "name", DataType: "text"},
			},
		},
		{
			Name: "books",
			Columns: []dbengine.ColumnInfo{
				{Name: "id", DataType: "int4", IsPrimaryKey: true},
				{Name: "title", DataType: "text"},
				{Name: "author_id", DataType: "int4"},
			},
		},
	}
	foreignKeys := []dbengine.ForeignKey{
		{TableName: "books", ColumnName: "author_id", ReferencedTable: "authors", ReferencedColumn: "id"},
	}

	want := "erDiagram\n" +
		"    authors {\n" +
		"        int4 id PK\n" +
		"        text name\n" +
		"    }\n" +
		"    books {\n" +
		"        int4 id PK\n" +
		"        text title\n" +
		"        int4 author_id FK\n" +
		"    }\n" +
		"    authors ||--o{ books : \"via author_id\"\n"

	if got := BuildRelationalERDiagram(tables, foreignKeys); got != want {
		t.Errorf("BuildRelationalERDiagram() =\n%s\nwant:\n%s", got, want)
	}
}

func TestBuildRelationalERDiagram_TableWithNoForeignKeys(t *testing.T) {
	tables := []dbengine.TableInfo{
		{
			Name: "settings",
			Columns: []dbengine.ColumnInfo{
				{Name: "id", DataType: "int4", IsPrimaryKey: true},
				{Name: "key", DataType: "character varying"},
				{Name: "value", DataType: "text"},
			},
		},
	}

	want := "erDiagram\n" +
		"    settings {\n" +
		"        int4 id PK\n" +
		"        character_varying key\n" +
		"        text value\n" +
		"    }\n"

	if got := BuildRelationalERDiagram(tables, nil); got != want {
		t.Errorf("BuildRelationalERDiagram() =\n%s\nwant:\n%s", got, want)
	}
}

func TestBuildRelationalERDiagram_ColumnThatIsBothPrimaryAndForeignKey(t *testing.T) {
	tables := []dbengine.TableInfo{
		{
			Name: "profiles",
			Columns: []dbengine.ColumnInfo{
				{Name: "user_id", DataType: "int4", IsPrimaryKey: true},
			},
		},
		{
			Name: "users",
			Columns: []dbengine.ColumnInfo{
				{Name: "id", DataType: "int4", IsPrimaryKey: true},
			},
		},
	}
	foreignKeys := []dbengine.ForeignKey{
		{TableName: "profiles", ColumnName: "user_id", ReferencedTable: "users", ReferencedColumn: "id"},
	}

	want := "erDiagram\n" +
		"    profiles {\n" +
		"        int4 user_id PK, FK\n" +
		"    }\n" +
		"    users {\n" +
		"        int4 id PK\n" +
		"    }\n" +
		"    users ||--o{ profiles : \"via user_id\"\n"

	if got := BuildRelationalERDiagram(tables, foreignKeys); got != want {
		t.Errorf("BuildRelationalERDiagram() =\n%s\nwant:\n%s", got, want)
	}
}

func TestMermaidToken(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"already safe", "widgets", "widgets"},
		{"space separated type", "character varying", "character_varying"},
		{"multi-word type", "timestamp without time zone", "timestamp_without_time_zone"},
		{"parens and digits", "varchar(255)", "varchar_255"},
		{"leading/trailing punctuation trimmed", "-id-", "id"},
		{"all punctuation falls back", "***", "col"},
		{"empty string falls back", "", "col"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := mermaidToken(tc.in); got != tc.want {
				t.Errorf("mermaidToken(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestBuildRelationalERDiagram_NoTablesNoForeignKeys(t *testing.T) {
	want := "erDiagram\n"
	if got := BuildRelationalERDiagram(nil, nil); got != want {
		t.Errorf("BuildRelationalERDiagram(nil, nil) = %q, want %q", got, want)
	}
}
