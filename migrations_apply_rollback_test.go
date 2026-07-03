package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"stackyard/internal/dbengine"
	"stackyard/internal/storage"
)

// fakeMigrationsBoundEngine is a dbengine.Engine + dbengine.Transactor test
// double for exercising ApplyMigrations/RollbackMigration's bound-method
// wiring (session/dialect/folder resolution) end to end, including a real
// call into migrations.Apply/migrations.Rollback, without a live database
// connection — the per-engine BeginTx/transaction behavior itself is
// already covered by internal/dbengine/postgres and mysql's own tests, and
// the Apply/Rollback control-flow logic by internal/migrations' own tests.
type fakeMigrationsBoundEngine struct {
	appliedVersions [][]any
}

func (f *fakeMigrationsBoundEngine) Connect(context.Context) error { return nil }
func (f *fakeMigrationsBoundEngine) Ping(context.Context) error    { return nil }

func (f *fakeMigrationsBoundEngine) Query(context.Context, string) (*dbengine.QueryResult, error) {
	return &dbengine.QueryResult{Rows: f.appliedVersions}, nil
}

func (f *fakeMigrationsBoundEngine) Exec(context.Context, string, ...any) (*dbengine.QueryResult, error) {
	return &dbengine.QueryResult{}, nil
}

func (f *fakeMigrationsBoundEngine) ListSchemas(context.Context) ([]string, error) { return nil, nil }

func (f *fakeMigrationsBoundEngine) ListTables(context.Context, string) ([]dbengine.TableInfo, error) {
	return nil, nil
}

func (f *fakeMigrationsBoundEngine) ListForeignKeys(context.Context, string) ([]dbengine.ForeignKey, error) {
	return nil, nil
}

func (f *fakeMigrationsBoundEngine) Close() error { return nil }

func (f *fakeMigrationsBoundEngine) BeginTx(context.Context) (dbengine.Tx, error) {
	return &fakeMigrationsBoundTx{}, nil
}

var (
	_ dbengine.Engine     = (*fakeMigrationsBoundEngine)(nil)
	_ dbengine.Transactor = (*fakeMigrationsBoundEngine)(nil)
)

type fakeMigrationsBoundTx struct{}

func (t *fakeMigrationsBoundTx) Exec(context.Context, string, ...any) (*dbengine.QueryResult, error) {
	return &dbengine.QueryResult{}, nil
}
func (t *fakeMigrationsBoundTx) Commit(context.Context) error   { return nil }
func (t *fakeMigrationsBoundTx) Rollback(context.Context) error { return nil }

var _ dbengine.Tx = (*fakeMigrationsBoundTx)(nil)

// newMigrationsTestSession creates a saved Connection (with folder pointed
// at a fresh temp dir containing one migration) on a's test DB and opens a
// querySession bound to engine, returning the sessionID ApplyMigrations/
// RollbackMigration expect.
func newMigrationsTestSession(t *testing.T, a *App, engine dbengine.Engine) string {
	t.Helper()

	folder := t.TempDir()
	if err := os.WriteFile(filepath.Join(folder, "20260101000001_create_widgets.up.sql"), []byte("CREATE TABLE widgets (id int);"), 0o644); err != nil {
		t.Fatalf("write up file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(folder, "20260101000001_create_widgets.down.sql"), []byte("DROP TABLE widgets;"), 0o644); err != nil {
		t.Fatalf("write down file: %v", err)
	}

	conn, err := storage.CreateConnection(a.db, &storage.Connection{
		Name:   "migrations-test",
		Engine: storage.EnginePostgres,
		Host:   "localhost",
		Port:   5432,
	})
	if err != nil {
		t.Fatalf("storage.CreateConnection() failed: %v", err)
	}
	if _, err := storage.SetConnectionMigrationsFolder(a.db, conn.ID, folder); err != nil {
		t.Fatalf("storage.SetConnectionMigrationsFolder() failed: %v", err)
	}

	sessionID := "migrations-test-session"
	a.putQuerySession(sessionID, &querySession{
		engine:       engine,
		connectionID: &conn.ID,
		engineType:   storage.EnginePostgres,
	})
	return sessionID
}

func TestApp_ApplyMigrations_NoOpenSession(t *testing.T) {
	a := &App{ctx: context.Background()}

	if _, err := a.ApplyMigrations("does-not-exist"); err == nil {
		t.Fatal("ApplyMigrations() with no open session returned nil error, want an error")
	}
}

func TestApp_ApplyMigrations_RejectsNonRelationalEngine(t *testing.T) {
	a := &App{ctx: context.Background()}
	a.putQuerySession("mongo-session", &querySession{
		engine:     &fakeMigrationsBoundEngine{},
		engineType: storage.EngineMongoDB,
	})

	if _, err := a.ApplyMigrations("mongo-session"); err == nil {
		t.Fatal("ApplyMigrations() against a Mongo session returned nil error, want an error")
	}
}

func TestApp_ApplyMigrations_RequiresSavedConnection(t *testing.T) {
	a := &App{ctx: context.Background()}
	a.putQuerySession("ad-hoc-session", &querySession{
		engine:     &fakeMigrationsBoundEngine{},
		engineType: storage.EnginePostgres,
	})

	if _, err := a.ApplyMigrations("ad-hoc-session"); err == nil {
		t.Fatal("ApplyMigrations() against a session with no saved connection returned nil error, want an error")
	}
}

func TestApp_ApplyMigrations_RunsThePendingMigration(t *testing.T) {
	a := newTestApp(t)
	a.ctx = context.Background()
	engine := &fakeMigrationsBoundEngine{}
	sessionID := newMigrationsTestSession(t, a, engine)

	result, err := a.ApplyMigrations(sessionID)
	if err != nil {
		t.Fatalf("ApplyMigrations() failed: %v", err)
	}
	if len(result.Applied) != 1 || result.Applied[0].Slug != "create_widgets" {
		t.Fatalf("ApplyMigrations().Applied = %v, want exactly the create_widgets migration", result.Applied)
	}
	if result.Failed != nil {
		t.Fatalf("ApplyMigrations().Failed = %v, want nil", result.Failed)
	}
}

func TestApp_RollbackMigration_NothingAppliedReturnsNilNil(t *testing.T) {
	a := newTestApp(t)
	a.ctx = context.Background()
	engine := &fakeMigrationsBoundEngine{}
	sessionID := newMigrationsTestSession(t, a, engine)

	reverted, err := a.RollbackMigration(sessionID)
	if err != nil {
		t.Fatalf("RollbackMigration() failed: %v", err)
	}
	if reverted != nil {
		t.Fatalf("RollbackMigration() with nothing applied = %v, want nil", reverted)
	}
}

func TestApp_RollbackMigration_RevertsTheAppliedMigration(t *testing.T) {
	a := newTestApp(t)
	a.ctx = context.Background()
	engine := &fakeMigrationsBoundEngine{
		appliedVersions: [][]any{{int64(20260101000001), "20260101000001_create_widgets"}},
	}
	sessionID := newMigrationsTestSession(t, a, engine)

	reverted, err := a.RollbackMigration(sessionID)
	if err != nil {
		t.Fatalf("RollbackMigration() failed: %v", err)
	}
	if reverted == nil || reverted.Slug != "create_widgets" {
		t.Fatalf("RollbackMigration() = %v, want the create_widgets migration", reverted)
	}
}
