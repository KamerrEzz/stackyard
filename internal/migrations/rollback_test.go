package migrations

import (
	"context"
	"errors"
	"strings"
	"testing"

	"stackyard/internal/dbengine"
)

func TestRollback_NothingApplied_ReturnsNilNil(t *testing.T) {
	folder := t.TempDir()

	engine := &fakeMigrationEngine{
		queryFunc: func(ctx context.Context, query string) (*dbengine.QueryResult, error) {
			return &dbengine.QueryResult{}, nil
		},
	}

	got, err := Rollback(context.Background(), engine, dbengine.DialectPostgres, folder)
	if err != nil {
		t.Fatalf("Rollback() with nothing applied returned error: %v, want nil", err)
	}
	if got != nil {
		t.Fatalf("Rollback() with nothing applied = %v, want nil", got)
	}
	if len(engine.txs) != 0 {
		t.Errorf("Rollback() with nothing applied began %d transactions, want 0", len(engine.txs))
	}
}

func TestRollback_RevertsOnlyTheMostRecentMigration(t *testing.T) {
	folder := t.TempDir()
	writeMigrationPair(t, folder, 20260101000001, "first", "CREATE TABLE t1 (id int);", "DROP TABLE t1;")
	writeMigrationPair(t, folder, 20260101000002, "second", "CREATE TABLE t2 (id int);", "DROP TABLE t2;")
	writeMigrationPair(t, folder, 20260101000003, "third", "CREATE TABLE t3 (id int);", "DROP TABLE t3;")

	engine := &fakeMigrationEngine{
		queryFunc: func(ctx context.Context, query string) (*dbengine.QueryResult, error) {
			return &dbengine.QueryResult{
				Rows: [][]any{{int64(20260101000003), "20260101000003_third"}},
			}, nil
		},
	}

	got, err := Rollback(context.Background(), engine, dbengine.DialectPostgres, folder)
	if err != nil {
		t.Fatalf("Rollback() returned error: %v", err)
	}
	if got == nil || got.Slug != "third" {
		t.Fatalf("Rollback() = %v, want the \"third\" migration", got)
	}

	if len(engine.txs) != 1 {
		t.Fatalf("expected exactly 1 transaction, got %d", len(engine.txs))
	}
	tx := engine.txs[0]
	if !tx.committed {
		t.Error("rollback transaction was not committed")
	}
	if len(tx.execCalls) != 2 {
		t.Fatalf("rollback transaction ran %d Exec calls, want 2 (down-SQL + tracking delete)", len(tx.execCalls))
	}
	if !strings.Contains(tx.execCalls[0], "DROP TABLE t3") {
		t.Errorf("first Exec call = %q, want the \"third\" migration's down.sql content", tx.execCalls[0])
	}
	if !strings.Contains(tx.execCalls[1], "DELETE FROM schema_migrations") {
		t.Errorf("second Exec call = %q, want the tracking-row delete", tx.execCalls[1])
	}
}

func TestRollback_MissingFileForRecordedVersion_ReturnsError(t *testing.T) {
	folder := t.TempDir()
	writeMigrationPair(t, folder, 20260101000001, "first", "CREATE TABLE t1 (id int);", "DROP TABLE t1;")

	engine := &fakeMigrationEngine{
		queryFunc: func(ctx context.Context, query string) (*dbengine.QueryResult, error) {
			return &dbengine.QueryResult{
				Rows: [][]any{{int64(999999999999), "999999999999_ghost"}},
			}, nil
		},
	}

	_, err := Rollback(context.Background(), engine, dbengine.DialectPostgres, folder)
	if err == nil {
		t.Fatal("Rollback() with a recorded version missing its file pair returned nil error, want an error")
	}
	if len(engine.txs) != 0 {
		t.Errorf("Rollback() began %d transactions before failing file lookup, want 0", len(engine.txs))
	}
}

func TestRollback_DownSQLFailure_RollsBackTransaction(t *testing.T) {
	folder := t.TempDir()
	writeMigrationPair(t, folder, 20260101000001, "first", "CREATE TABLE t1 (id int);", "DROP TABLE t1 CASCADE BROKEN;")

	dbErr := errors.New("syntax error near BROKEN")
	engine := &fakeMigrationEngine{
		queryFunc: func(ctx context.Context, query string) (*dbengine.QueryResult, error) {
			return &dbengine.QueryResult{
				Rows: [][]any{{int64(20260101000001), "20260101000001_first"}},
			}, nil
		},
		execFunc: func(ctx context.Context, query string, args ...any) (*dbengine.QueryResult, error) {
			if strings.Contains(query, "BROKEN") {
				return nil, dbErr
			}
			return &dbengine.QueryResult{}, nil
		},
	}

	got, err := Rollback(context.Background(), engine, dbengine.DialectPostgres, folder)
	if err == nil {
		t.Fatal("Rollback() with failing down-SQL returned nil error, want an error")
	}
	if got != nil {
		t.Fatalf("Rollback() with failing down-SQL = %v, want nil", got)
	}
	if !strings.Contains(err.Error(), dbErr.Error()) {
		t.Fatalf("Rollback() error = %q, want it to contain %q", err.Error(), dbErr.Error())
	}
	if len(engine.txs) != 1 || !engine.txs[0].rolledBack || engine.txs[0].committed {
		t.Fatalf("expected exactly 1 rolled-back (not committed) transaction, got %+v", engine.txs)
	}
}
