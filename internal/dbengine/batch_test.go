package dbengine

import (
	"context"
	"errors"
	"testing"
)

type fakeBatchEngine struct {
	execFunc func(ctx context.Context, query string, args ...any) (*QueryResult, error)
}

func (f *fakeBatchEngine) Connect(context.Context) error { return nil }
func (f *fakeBatchEngine) Ping(context.Context) error    { return nil }
func (f *fakeBatchEngine) Query(context.Context, string) (*QueryResult, error) {
	return nil, nil
}
func (f *fakeBatchEngine) Exec(ctx context.Context, query string, args ...any) (*QueryResult, error) {
	return f.execFunc(ctx, query, args...)
}
func (f *fakeBatchEngine) ListSchemas(context.Context) ([]string, error) { return nil, nil }
func (f *fakeBatchEngine) ListTables(context.Context, string) ([]TableInfo, error) {
	return nil, nil
}
func (f *fakeBatchEngine) ListForeignKeys(context.Context, string) ([]ForeignKey, error) {
	return nil, nil
}
func (f *fakeBatchEngine) Close() error { return nil }

func TestExecuteBatch_RunsEveryStatementIndependently(t *testing.T) {
	boom := errors.New("constraint violation")
	engine := &fakeBatchEngine{
		execFunc: func(_ context.Context, query string, args ...any) (*QueryResult, error) {
			if query == "fails" {
				return nil, boom
			}
			return &QueryResult{RowsAffected: 1}, nil
		},
	}

	statements := []PreparedStatement{
		{Text: "ok-1", Args: []any{1}},
		{Text: "fails", Args: []any{2}},
		{Text: "ok-2", Args: []any{3}},
	}

	results := ExecuteBatch(context.Background(), engine, statements)

	if len(results) != 3 {
		t.Fatalf("len(results) = %d, want 3", len(results))
	}
	if !results[0].Success || results[0].Result == nil {
		t.Errorf("results[0] = %+v, want a successful result", results[0])
	}
	if results[1].Success {
		t.Errorf("results[1] = %+v, want Success == false", results[1])
	}
	if results[1].ErrorMessage != boom.Error() {
		t.Errorf("results[1].ErrorMessage = %q, want %q", results[1].ErrorMessage, boom.Error())
	}
	if !results[2].Success || results[2].Result == nil {
		t.Errorf("results[2] = %+v, want a successful result (independent of results[1] failing)", results[2])
	}
}

func TestExecuteBatch_EmptyInputReturnsEmptyOutput(t *testing.T) {
	engine := &fakeBatchEngine{execFunc: func(context.Context, string, ...any) (*QueryResult, error) {
		t.Fatal("Exec should not be called for an empty batch")
		return nil, nil
	}}

	results := ExecuteBatch(context.Background(), engine, nil)
	if len(results) != 0 {
		t.Errorf("len(results) = %d, want 0", len(results))
	}
}

func TestSplitStatements(t *testing.T) {
	cases := []struct {
		name string
		sql  string
		want []string
	}{
		{"single statement, no trailing semicolon", "SELECT 1", []string{"SELECT 1"}},
		{"single statement, trailing semicolon", "SELECT 1;", []string{"SELECT 1"}},
		{"two statements", "SELECT 1; SELECT 2", []string{"SELECT 1", "SELECT 2"}},
		{"blank segments collapsed", "SELECT 1;;\n\nSELECT 2;", []string{"SELECT 1", "SELECT 2"}},
		{"whitespace only", "   \n\t  ", nil},
		{"empty", "", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := SplitStatements(tc.sql)
			if len(got) != len(tc.want) {
				t.Fatalf("SplitStatements(%q) = %v, want %v", tc.sql, got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("SplitStatements(%q)[%d] = %q, want %q", tc.sql, i, got[i], tc.want[i])
				}
			}
		})
	}
}

func TestSplitStatements_QuotedSemicolonsAreNotStatementBoundaries(t *testing.T) {
	cases := []struct {
		name string
		sql  string
		want []string
	}{
		{
			"semicolon inside single-quoted string literal",
			"INSERT INTO widgets (name) VALUES ('hello; world')",
			[]string{"INSERT INTO widgets (name) VALUES ('hello; world')"},
		},
		{
			"escaped single quote then semicolon still inside string literal",
			"INSERT INTO t (v) VALUES ('it''s a test; still inside')",
			[]string{"INSERT INTO t (v) VALUES ('it''s a test; still inside')"},
		},
		{
			"unquoted statements still split normally",
			"SELECT 1; SELECT 2;",
			[]string{"SELECT 1", "SELECT 2"},
		},
		{
			"semicolon inside double-quoted identifier",
			`SELECT "my;column" FROM t; SELECT 2;`,
			[]string{`SELECT "my;column" FROM t`, "SELECT 2"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := SplitStatements(tc.sql)
			if len(got) != len(tc.want) {
				t.Fatalf("SplitStatements(%q) = %v, want %v", tc.sql, got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("SplitStatements(%q)[%d] = %q, want %q", tc.sql, i, got[i], tc.want[i])
				}
			}
		})
	}
}

func TestExecuteMultiStatementText_RunsEachSplitStatementIndependently(t *testing.T) {
	boom := errors.New("syntax error")
	var seenQueries []string
	engine := &fakeBatchEngine{
		execFunc: func(_ context.Context, query string, _ ...any) (*QueryResult, error) {
			seenQueries = append(seenQueries, query)
			if query == "BAD SQL" {
				return nil, boom
			}
			return &QueryResult{RowsAffected: 1}, nil
		},
	}

	results := ExecuteMultiStatementText(context.Background(), engine, "INSERT INTO t VALUES (1); BAD SQL; INSERT INTO t VALUES (2)")

	if len(results) != 3 {
		t.Fatalf("len(results) = %d, want 3", len(results))
	}
	if !results[0].Success || !results[2].Success {
		t.Errorf("results = %+v, want statements 0 and 2 to succeed independently of statement 1 failing", results)
	}
	if results[1].Success {
		t.Errorf("results[1] = %+v, want Success == false", results[1])
	}
	wantQueries := []string{"INSERT INTO t VALUES (1)", "BAD SQL", "INSERT INTO t VALUES (2)"}
	for i, want := range wantQueries {
		if seenQueries[i] != want {
			t.Errorf("seenQueries[%d] = %q, want %q", i, seenQueries[i], want)
		}
	}
}
