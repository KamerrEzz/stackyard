package mysql

import (
	"context"
	"errors"
	"strings"
	"testing"

	mysqldriver "github.com/go-sql-driver/mysql"
)

func TestNew_DoesNotDial(t *testing.T) {
	e := New("user:pass@tcp(nonexistent-host-for-test:3306)/db")
	if e.db != nil {
		t.Error("New must not open a DB handle before Connect is called")
	}
}

func TestPing_BeforeConnect_ReturnsErrNotConnected(t *testing.T) {
	e := New("user:pass@tcp(localhost:3306)/db")
	if err := e.Ping(context.Background()); !errors.Is(err, ErrNotConnected) {
		t.Errorf("Ping() before Connect = %v, want ErrNotConnected", err)
	}
}

func TestQuery_BeforeConnect_ReturnsErrNotConnected(t *testing.T) {
	e := New("user:pass@tcp(localhost:3306)/db")
	if _, err := e.Query(context.Background(), "SELECT 1"); !errors.Is(err, ErrNotConnected) {
		t.Errorf("Query() before Connect = %v, want ErrNotConnected", err)
	}
}

func TestListSchemas_BeforeConnect_ReturnsErrNotConnected(t *testing.T) {
	e := New("user:pass@tcp(localhost:3306)/db")
	if _, err := e.ListSchemas(context.Background()); !errors.Is(err, ErrNotConnected) {
		t.Errorf("ListSchemas() before Connect = %v, want ErrNotConnected", err)
	}
}

func TestListTables_BeforeConnect_ReturnsErrNotConnected(t *testing.T) {
	e := New("user:pass@tcp(localhost:3306)/db")
	if _, err := e.ListTables(context.Background(), "test"); !errors.Is(err, ErrNotConnected) {
		t.Errorf("ListTables() before Connect = %v, want ErrNotConnected", err)
	}
}

func TestListForeignKeys_BeforeConnect_ReturnsErrNotConnected(t *testing.T) {
	e := New("user:pass@tcp(localhost:3306)/db")
	if _, err := e.ListForeignKeys(context.Background(), "test"); !errors.Is(err, ErrNotConnected) {
		t.Errorf("ListForeignKeys() before Connect = %v, want ErrNotConnected", err)
	}
}

func TestClose_BeforeConnect_IsSafe(t *testing.T) {
	e := New("user:pass@tcp(localhost:3306)/db")
	if err := e.Close(); err != nil {
		t.Errorf("Close() before Connect = %v, want nil", err)
	}
	if err := e.Close(); err != nil {
		t.Errorf("second Close() = %v, want nil", err)
	}
}

func TestIsReadStatement(t *testing.T) {
	cases := []struct {
		name  string
		query string
		want  bool
	}{
		{"select", "SELECT * FROM users", true},
		{"lowercase select", "select 1", true},
		{"with cte", "WITH cte AS (SELECT 1) SELECT * FROM cte", true},
		{"show", "SHOW TABLES", true},
		{"describe", "DESCRIBE users", true},
		{"desc", "DESC users", true},
		{"explain", "EXPLAIN SELECT 1", true},
		{"values", "VALUES ROW(1,2)", true},
		{"leading whitespace", "   \n\tSELECT 1", true},
		{"leading line comment", "-- a comment\nSELECT 1", true},
		{"leading block comment", "/* a comment */ SELECT 1", true},
		{"multiple leading comments", "-- one\n/* two */\nSELECT 1", true},
		{"insert", "INSERT INTO users (name) VALUES ('a')", false},
		{"update", "UPDATE users SET name = 'a'", false},
		{"delete", "DELETE FROM users", false},
		{"create table", "CREATE TABLE t (id INT)", false},
		{"drop table", "DROP TABLE t", false},
		{"alter table", "ALTER TABLE t ADD COLUMN c INT", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isReadStatement(tc.query); got != tc.want {
				t.Errorf("isReadStatement(%q) = %v, want %v", tc.query, got, tc.want)
			}
		})
	}
}

func TestStripLeadingNoise(t *testing.T) {
	cases := []struct {
		name  string
		query string
		want  string
	}{
		{"no noise", "SELECT 1", "SELECT 1"},
		{"leading whitespace", "  \n\tSELECT 1", "SELECT 1"},
		{"line comment", "-- note\nSELECT 1", "SELECT 1"},
		{"block comment", "/* note */ SELECT 1", "SELECT 1"},
		{"unterminated line comment", "-- note with no newline", ""},
		{"unterminated block comment", "/* never closed", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := stripLeadingNoise(tc.query); got != tc.want {
				t.Errorf("stripLeadingNoise(%q) = %q, want %q", tc.query, got, tc.want)
			}
		})
	}
}

func TestTranslateMySQLError_WrapsMySQLError(t *testing.T) {
	myErr := &mysqldriver.MySQLError{Number: 1062, Message: "Duplicate entry 'a' for key 'PRIMARY'"}
	err := translateMySQLError("exec", myErr)
	if !strings.Contains(err.Error(), "1062") {
		t.Errorf("expected error to mention MySQL error number, got: %v", err)
	}
	if !strings.Contains(err.Error(), "Duplicate entry") {
		t.Errorf("expected error to include MySQL's message, got: %v", err)
	}
	if !errors.Is(err, myErr) {
		t.Error("expected translated error to wrap the original *mysqldriver.MySQLError")
	}
}

func TestTranslateMySQLError_PassesThroughNonMySQLError(t *testing.T) {
	generic := errors.New("connection refused")
	err := translateMySQLError("query", generic)
	if !strings.Contains(err.Error(), "connection refused") {
		t.Errorf("expected wrapped error to preserve original message, got: %v", err)
	}
	if !errors.Is(err, generic) {
		t.Error("expected translated error to wrap the original via %w")
	}
}
