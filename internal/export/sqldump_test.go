package export

import (
	"database/sql/driver"
	"strings"
	"testing"
	"time"

	"stackyard/internal/dbengine"
)

// fakeNumeric is a minimal driver.Valuer stand-in for pgx's pgtype.Numeric,
// used so this package's unit tests don't need to import pgx just to prove
// formatSQLLiteral/formatCSVValue resolve a driver.Valuer's Value() rather
// than stringifying the struct itself.
type fakeNumeric struct {
	text string
}

func (n fakeNumeric) Value() (driver.Value, error) {
	return n.text, nil
}

func TestBuildCreateTable_PostgresWithPrimaryKey(t *testing.T) {
	got := BuildCreateTable(dbengine.DialectPostgres, "public", "widgets", []ColumnDumpInfo{
		{Name: "id", SQLType: "integer", Nullable: false, IsPrimaryKey: true},
		{Name: "name", SQLType: "text", Nullable: false},
		{Name: "weight", SQLType: "numeric", Nullable: true},
	})
	want := `CREATE TABLE "public"."widgets" (
  "id" integer NOT NULL,
  "name" text NOT NULL,
  "weight" numeric,
  PRIMARY KEY ("id")
)`
	if got != want {
		t.Errorf("BuildCreateTable() =\n%s\nwant:\n%s", got, want)
	}
}

func TestBuildCreateTable_MySQLNoPrimaryKey(t *testing.T) {
	got := BuildCreateTable(dbengine.DialectMySQL, "", "logs", []ColumnDumpInfo{
		{Name: "message", SQLType: "varchar(255)", Nullable: true},
	})
	want := "CREATE TABLE `logs` (\n  `message` varchar(255)\n)"
	if got != want {
		t.Errorf("BuildCreateTable() =\n%s\nwant:\n%s", got, want)
	}
}

func TestBuildInsertStatements_BatchesRows(t *testing.T) {
	rows := make([][]any, 501)
	for i := range rows {
		rows[i] = []any{i}
	}
	statements := BuildInsertStatements(dbengine.DialectPostgres, "public", "widgets", []string{"id"}, rows)
	if len(statements) != 2 {
		t.Fatalf("BuildInsertStatements() returned %d statements, want 2 (501 rows at 500/batch)", len(statements))
	}
	if !strings.Contains(statements[0], "(0)") || !strings.Contains(statements[0], "(499)") {
		t.Errorf("first batch missing expected rows: %s", statements[0])
	}
	if !strings.Contains(statements[1], "(500)") {
		t.Errorf("second batch missing row 500: %s", statements[1])
	}
}

func TestBuildInsertStatements_EmptyRowsReturnsNil(t *testing.T) {
	if got := BuildInsertStatements(dbengine.DialectPostgres, "public", "widgets", []string{"id"}, nil); got != nil {
		t.Errorf("BuildInsertStatements() = %v, want nil for zero rows", got)
	}
}

func TestFormatSQLLiteral_NullVsEmptyString(t *testing.T) {
	if got := formatSQLLiteral(dbengine.DialectPostgres, nil); got != "NULL" {
		t.Errorf("formatSQLLiteral(nil) = %q, want NULL", got)
	}
	if got := formatSQLLiteral(dbengine.DialectPostgres, ""); got != "''" {
		t.Errorf("formatSQLLiteral(\"\") = %q, want ''", got)
	}
}

func TestFormatSQLLiteral_DateValue(t *testing.T) {
	when := time.Date(2024, 3, 15, 10, 30, 45, 0, time.UTC)
	got := formatSQLLiteral(dbengine.DialectPostgres, when)
	want := "'2024-03-15 10:30:45'"
	if got != want {
		t.Errorf("formatSQLLiteral(time) = %q, want %q", got, want)
	}
}

func TestFormatSQLLiteral_NumericValues(t *testing.T) {
	if got := formatSQLLiteral(dbengine.DialectPostgres, 42); got != "42" {
		t.Errorf("formatSQLLiteral(42) = %q, want 42", got)
	}
	if got := formatSQLLiteral(dbengine.DialectPostgres, 3.5); got != "3.5" {
		t.Errorf("formatSQLLiteral(3.5) = %q, want 3.5", got)
	}
}

func TestFormatSQLLiteral_DriverValuerNumeric(t *testing.T) {
	got := formatSQLLiteral(dbengine.DialectPostgres, fakeNumeric{text: "1234.5600"})
	if got != "1234.5600" {
		t.Errorf("formatSQLLiteral(fakeNumeric) = %q, want the bare numeric text 1234.5600, not quoted/stringified", got)
	}
}

func TestFormatSQLLiteral_BoolValues(t *testing.T) {
	if got := formatSQLLiteral(dbengine.DialectPostgres, true); got != "TRUE" {
		t.Errorf("formatSQLLiteral(true) = %q, want TRUE", got)
	}
	if got := formatSQLLiteral(dbengine.DialectMySQL, false); got != "FALSE" {
		t.Errorf("formatSQLLiteral(false) = %q, want FALSE", got)
	}
}

func TestFormatSQLLiteral_EscapesSingleQuote(t *testing.T) {
	got := formatSQLLiteral(dbengine.DialectPostgres, "O'Brien")
	want := "'O''Brien'"
	if got != want {
		t.Errorf("formatSQLLiteral(\"O'Brien\") = %q, want %q", got, want)
	}
}

func TestFormatSQLLiteral_MySQLEscapesBackslashBeforeQuote(t *testing.T) {
	got := formatSQLLiteral(dbengine.DialectMySQL, `trailing\`)
	want := `'trailing\\'`
	if got != want {
		t.Errorf("formatSQLLiteral(mysql, `trailing\\`) = %q, want %q (backslash must be escaped so it can't swallow the closing quote)", got, want)
	}
}

func TestFormatSQLLiteral_PostgresDoesNotEscapeBackslash(t *testing.T) {
	got := formatSQLLiteral(dbengine.DialectPostgres, `trailing\`)
	want := `'trailing\'`
	if got != want {
		t.Errorf("formatSQLLiteral(postgres, `trailing\\`) = %q, want %q (standard_conforming_strings treats backslash literally)", got, want)
	}
}

func TestToSQLDump_EndToEnd(t *testing.T) {
	columns := []ColumnDumpInfo{
		{Name: "id", SQLType: "integer", Nullable: false, IsPrimaryKey: true},
		{Name: "note", SQLType: "text", Nullable: true},
	}
	rows := [][]any{
		{1, "hello"},
		{2, nil},
	}
	got := ToSQLDump(dbengine.DialectPostgres, "public", "widgets", columns, rows)

	if !strings.HasPrefix(got, `CREATE TABLE "public"."widgets" (`) {
		t.Errorf("ToSQLDump() doesn't start with the expected CREATE TABLE: %s", got)
	}
	if !strings.Contains(got, "(1, 'hello')") {
		t.Errorf("ToSQLDump() missing expected row 1: %s", got)
	}
	if !strings.Contains(got, "(2, NULL)") {
		t.Errorf("ToSQLDump() missing expected NULL row: %s", got)
	}
	if strings.Count(got, ";\n") < 2 {
		t.Errorf("ToSQLDump() should terminate both CREATE TABLE and INSERT with ';': %s", got)
	}
}

func TestToSQLDump_EmptyTable(t *testing.T) {
	columns := []ColumnDumpInfo{{Name: "id", SQLType: "integer", Nullable: false, IsPrimaryKey: true}}
	got := ToSQLDump(dbengine.DialectMySQL, "", "widgets", columns, nil)
	want := "CREATE TABLE `widgets` (\n  `id` integer NOT NULL,\n  PRIMARY KEY (`id`)\n);\n"
	if got != want {
		t.Errorf("ToSQLDump() =\n%s\nwant:\n%s", got, want)
	}
}
