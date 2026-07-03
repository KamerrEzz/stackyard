package migrations

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"stackyard/internal/dbengine"
)

// fakeMigrationEngine is a dbengine.Engine + dbengine.Transactor test double
// for exercising Apply/Rollback's control flow and atomicity guarantees
// without a real database connection. queryFunc backs plain engine.Query
// calls (reading schema_migrations); every dbengine.Tx BeginTx returns
// shares execFunc, so a test can make an individual migration step (its
// up/down SQL, or the schema_migrations INSERT/DELETE) fail on demand.
type fakeMigrationEngine struct {
	queryFunc func(ctx context.Context, query string) (*dbengine.QueryResult, error)
	execFunc  func(ctx context.Context, query string, args ...any) (*dbengine.QueryResult, error)
	beginErr  error
	txs       []*fakeMigrationTx
}

func (f *fakeMigrationEngine) Connect(context.Context) error { return nil }
func (f *fakeMigrationEngine) Ping(context.Context) error    { return nil }

func (f *fakeMigrationEngine) Query(ctx context.Context, query string) (*dbengine.QueryResult, error) {
	if f.queryFunc != nil {
		return f.queryFunc(ctx, query)
	}
	return &dbengine.QueryResult{}, nil
}

func (f *fakeMigrationEngine) Exec(ctx context.Context, query string, args ...any) (*dbengine.QueryResult, error) {
	return f.Query(ctx, query)
}

func (f *fakeMigrationEngine) ListSchemas(context.Context) ([]string, error) { return nil, nil }

func (f *fakeMigrationEngine) ListTables(context.Context, string) ([]dbengine.TableInfo, error) {
	return nil, nil
}

func (f *fakeMigrationEngine) ListForeignKeys(context.Context, string) ([]dbengine.ForeignKey, error) {
	return nil, nil
}

func (f *fakeMigrationEngine) Close() error { return nil }

func (f *fakeMigrationEngine) BeginTx(ctx context.Context) (dbengine.Tx, error) {
	if f.beginErr != nil {
		return nil, f.beginErr
	}
	tx := &fakeMigrationTx{execFunc: f.execFunc}
	f.txs = append(f.txs, tx)
	return tx, nil
}

var (
	_ dbengine.Engine     = (*fakeMigrationEngine)(nil)
	_ dbengine.Transactor = (*fakeMigrationEngine)(nil)
)

// fakeMigrationTx is the dbengine.Tx fakeMigrationEngine.BeginTx returns.
type fakeMigrationTx struct {
	execFunc   func(ctx context.Context, query string, args ...any) (*dbengine.QueryResult, error)
	execCalls  []string
	committed  bool
	rolledBack bool
}

func (t *fakeMigrationTx) Exec(ctx context.Context, query string, args ...any) (*dbengine.QueryResult, error) {
	t.execCalls = append(t.execCalls, query)
	if t.execFunc != nil {
		return t.execFunc(ctx, query, args...)
	}
	return &dbengine.QueryResult{}, nil
}

func (t *fakeMigrationTx) Commit(ctx context.Context) error {
	t.committed = true
	return nil
}

func (t *fakeMigrationTx) Rollback(ctx context.Context) error {
	t.rolledBack = true
	return nil
}

var _ dbengine.Tx = (*fakeMigrationTx)(nil)

// writeMigrationPair writes one <version>_<slug>.up.sql/.down.sql pair
// directly (bypassing CreateMigration's own current-time versioning) so
// tests can control exact, distinct, ordered versions without racing
// CreateMigration's 1-second collision window.
func writeMigrationPair(t *testing.T, folder string, version int64, slug, upSQL, downSQL string) Migration {
	t.Helper()
	stem := fmt.Sprintf("%d_%s", version, slug)
	upPath := filepath.Join(folder, stem+upFileSuffix)
	downPath := filepath.Join(folder, stem+downFileSuffix)

	if err := os.WriteFile(upPath, []byte(upSQL), 0o644); err != nil {
		t.Fatalf("write up file: %v", err)
	}
	if err := os.WriteFile(downPath, []byte(downSQL), 0o644); err != nil {
		t.Fatalf("write down file: %v", err)
	}
	return Migration{Version: version, Slug: slug, UpPath: upPath, DownPath: downPath}
}

func TestPendingMigrations(t *testing.T) {
	all := []Migration{
		{Version: 1, Slug: "first"},
		{Version: 2, Slug: "second"},
		{Version: 3, Slug: "third"},
	}

	cases := []struct {
		name    string
		applied map[int64]bool
		want    []int64
	}{
		{"none applied", map[int64]bool{}, []int64{1, 2, 3}},
		{"nil applied set", nil, []int64{1, 2, 3}},
		{"first applied", map[int64]bool{1: true}, []int64{2, 3}},
		{"middle applied", map[int64]bool{2: true}, []int64{1, 3}},
		{"all applied", map[int64]bool{1: true, 2: true, 3: true}, []int64{}},
		{"applied set has unknown version", map[int64]bool{1: true, 999: true}, []int64{2, 3}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := PendingMigrations(all, c.applied)
			gotVersions := make([]int64, len(got))
			for i, m := range got {
				gotVersions[i] = m.Version
			}
			if len(gotVersions) != len(c.want) {
				t.Fatalf("PendingMigrations() = %v, want %v", gotVersions, c.want)
			}
			for i := range c.want {
				if gotVersions[i] != c.want[i] {
					t.Fatalf("PendingMigrations() = %v, want %v", gotVersions, c.want)
				}
			}
		})
	}
}

func TestToInt64(t *testing.T) {
	cases := []struct {
		in      any
		want    int64
		wantErr bool
	}{
		{int64(42), 42, false},
		{int32(42), 42, false},
		{int(42), 42, false},
		{"42", 42, false},
		{[]byte("42"), 42, false},
		{"not-a-number", 0, true},
		{3.14, 0, true},
	}

	for _, c := range cases {
		got, err := toInt64(c.in)
		if c.wantErr {
			if err == nil {
				t.Errorf("toInt64(%#v) = (%d, nil), want an error", c.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("toInt64(%#v) unexpected error: %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("toInt64(%#v) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestApply_RunsAllPendingInAscendingOrder(t *testing.T) {
	folder := t.TempDir()
	writeMigrationPair(t, folder, 20260101000001, "first", "CREATE TABLE t1 (id int);", "DROP TABLE t1;")
	writeMigrationPair(t, folder, 20260101000002, "second", "CREATE TABLE t2 (id int);", "DROP TABLE t2;")

	engine := &fakeMigrationEngine{
		queryFunc: func(ctx context.Context, query string) (*dbengine.QueryResult, error) {
			return &dbengine.QueryResult{}, nil
		},
	}

	result, err := Apply(context.Background(), engine, dbengine.DialectPostgres, folder)
	if err != nil {
		t.Fatalf("Apply() returned error: %v", err)
	}
	if result.Failed != nil {
		t.Fatalf("Apply().Failed = %v, want nil", result.Failed)
	}
	if len(result.Applied) != 2 || result.Applied[0].Slug != "first" || result.Applied[1].Slug != "second" {
		t.Fatalf("Apply().Applied = %v, want [first, second] in order", result.Applied)
	}
	if len(engine.txs) != 2 {
		t.Fatalf("expected 2 transactions to begin, got %d", len(engine.txs))
	}
	for i, tx := range engine.txs {
		if !tx.committed {
			t.Errorf("transaction %d was never committed", i)
		}
		if tx.rolledBack {
			t.Errorf("transaction %d was rolled back, want committed", i)
		}
		if len(tx.execCalls) != 2 {
			t.Errorf("transaction %d ran %d Exec calls, want 2 (up-SQL + tracking insert)", i, len(tx.execCalls))
		}
	}
}

func TestApply_SkipsAlreadyAppliedVersions(t *testing.T) {
	folder := t.TempDir()
	writeMigrationPair(t, folder, 20260101000001, "first", "CREATE TABLE t1 (id int);", "DROP TABLE t1;")
	writeMigrationPair(t, folder, 20260101000002, "second", "CREATE TABLE t2 (id int);", "DROP TABLE t2;")

	engine := &fakeMigrationEngine{
		queryFunc: func(ctx context.Context, query string) (*dbengine.QueryResult, error) {
			return &dbengine.QueryResult{Rows: [][]any{{int64(20260101000001)}}}, nil
		},
	}

	result, err := Apply(context.Background(), engine, dbengine.DialectPostgres, folder)
	if err != nil {
		t.Fatalf("Apply() returned error: %v", err)
	}
	if len(result.Applied) != 1 || result.Applied[0].Slug != "second" {
		t.Fatalf("Apply().Applied = %v, want only [second]", result.Applied)
	}
}

func TestApply_StopsAtFirstFailure_LeavesLaterMigrationsUntouched(t *testing.T) {
	folder := t.TempDir()
	writeMigrationPair(t, folder, 20260101000001, "first", "CREATE TABLE t1 (id int);", "DROP TABLE t1;")
	writeMigrationPair(t, folder, 20260101000002, "second", "THIS IS BROKEN SQL", "DROP TABLE t2;")
	writeMigrationPair(t, folder, 20260101000003, "third", "CREATE TABLE t3 (id int);", "DROP TABLE t3;")

	dbErr := errors.New(`syntax error at or near "THIS"`)
	engine := &fakeMigrationEngine{
		queryFunc: func(ctx context.Context, query string) (*dbengine.QueryResult, error) {
			return &dbengine.QueryResult{}, nil
		},
		execFunc: func(ctx context.Context, query string, args ...any) (*dbengine.QueryResult, error) {
			if strings.Contains(query, "THIS IS BROKEN SQL") {
				return nil, dbErr
			}
			return &dbengine.QueryResult{}, nil
		},
	}

	result, err := Apply(context.Background(), engine, dbengine.DialectPostgres, folder)
	if err != nil {
		t.Fatalf("Apply() returned a top-level error %v, want nil error with Failed populated instead", err)
	}

	if len(result.Applied) != 1 || result.Applied[0].Slug != "first" {
		t.Fatalf("Apply().Applied = %v, want exactly [first]", result.Applied)
	}
	if result.Failed == nil || result.Failed.Slug != "second" {
		t.Fatalf("Apply().Failed = %v, want the \"second\" migration", result.Failed)
	}
	if !strings.Contains(result.FailedError, dbErr.Error()) {
		t.Fatalf("Apply().FailedError = %q, want it to contain %q", result.FailedError, dbErr.Error())
	}

	if len(engine.txs) != 2 {
		t.Fatalf("expected exactly 2 transactions to begin (first + second; third never attempted), got %d", len(engine.txs))
	}
	if !engine.txs[0].committed {
		t.Error("first migration's transaction was not committed")
	}
	if !engine.txs[1].rolledBack {
		t.Error("second migration's transaction was not rolled back")
	}
	if engine.txs[1].committed {
		t.Error("second migration's transaction was committed, want rolled back only")
	}
}

func TestApply_ReportsAnErrorWhenEngineDoesNotSupportTransactions(t *testing.T) {
	folder := t.TempDir()
	writeMigrationPair(t, folder, 20260101000001, "first", "CREATE TABLE t1 (id int);", "DROP TABLE t1;")

	engine := &nonTransactionalEngine{}

	_, err := Apply(context.Background(), engine, dbengine.DialectPostgres, folder)
	if err == nil {
		t.Fatal("Apply() with a non-Transactor engine returned nil error, want an error")
	}
}

// nonTransactionalEngine is a dbengine.Engine that deliberately does not
// implement dbengine.Transactor, used to confirm Apply/Rollback reject an
// engine that cannot guarantee atomicity rather than silently running
// unsafe, non-atomic statements.
type nonTransactionalEngine struct{}

func (nonTransactionalEngine) Connect(context.Context) error { return nil }
func (nonTransactionalEngine) Ping(context.Context) error    { return nil }
func (nonTransactionalEngine) Query(context.Context, string) (*dbengine.QueryResult, error) {
	return &dbengine.QueryResult{}, nil
}
func (nonTransactionalEngine) Exec(context.Context, string, ...any) (*dbengine.QueryResult, error) {
	return &dbengine.QueryResult{}, nil
}
func (nonTransactionalEngine) ListSchemas(context.Context) ([]string, error) { return nil, nil }
func (nonTransactionalEngine) ListTables(context.Context, string) ([]dbengine.TableInfo, error) {
	return nil, nil
}
func (nonTransactionalEngine) ListForeignKeys(context.Context, string) ([]dbengine.ForeignKey, error) {
	return nil, nil
}
func (nonTransactionalEngine) Close() error { return nil }

var _ dbengine.Engine = nonTransactionalEngine{}
