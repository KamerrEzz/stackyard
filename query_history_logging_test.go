package main

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	"stackyard/internal/dbengine"
	"stackyard/internal/storage"
)

// openHistoryTestDB opens a throwaway SQLite database for exercising
// RunQuery's query_history logging (tasks.md 4.5) against real storage
// code, following the same t.TempDir()-backed OpenAt pattern
// internal/storage's own tests use (see connections_test.go's
// openTestDB).
func openHistoryTestDB(t *testing.T) *sql.DB {
	t.Helper()

	path := filepath.Join(t.TempDir(), "stackyard-history-test.db")
	db, err := storage.OpenAt(path)
	if err != nil {
		t.Fatalf("OpenAt(%q) failed: %v", path, err)
	}
	t.Cleanup(func() { db.Close() })

	return db
}

func createHistoryTestConnection(t *testing.T, db *sql.DB) *storage.Connection {
	t.Helper()

	conn, err := storage.CreateConnection(db, &storage.Connection{
		Name:   "history-test-conn",
		Engine: storage.EnginePostgres,
		Host:   "localhost",
		Port:   5432,
	})
	if err != nil {
		t.Fatalf("CreateConnection failed: %v", err)
	}
	return conn
}

func TestApp_RunQuery_LogsSuccessfulExecutionToHistory(t *testing.T) {
	db := openHistoryTestDB(t)
	conn := createHistoryTestConnection(t, db)

	a := &App{ctx: context.Background(), db: db}
	connectionID := conn.ID
	engine := &fakeQueryEngine{
		queryFunc: func(ctx context.Context, query string) (*dbengine.QueryResult, error) {
			return &dbengine.QueryResult{
				Columns:      []dbengine.ResultColumn{{Name: "n"}},
				Rows:         [][]any{{1}, {2}, {3}},
				RowsAffected: 0,
			}, nil
		},
	}
	a.putQuerySession("session-1", &querySession{engine: engine, connectionID: &connectionID})

	if _, err := a.RunQuery("session-1", "SELECT * FROM widgets"); err != nil {
		t.Fatalf("RunQuery() failed: %v", err)
	}

	entries, err := storage.ListQueryHistory(db, storage.QueryHistoryFilter{ConnectionID: conn.ID})
	if err != nil {
		t.Fatalf("ListQueryHistory failed: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected exactly 1 logged history entry, got %d", len(entries))
	}

	entry := entries[0]
	if entry.QueryText != "SELECT * FROM widgets" {
		t.Errorf("QueryText mismatch: got %q", entry.QueryText)
	}
	if !entry.Success {
		t.Error("expected a successful execution to log Success=true")
	}
	if entry.ErrorMessage != nil {
		t.Errorf("expected ErrorMessage to be nil for a successful execution, got %v", *entry.ErrorMessage)
	}
	if entry.RowsAffected != 3 {
		t.Errorf("expected RowsAffected to fall back to the returned row count (3) when the engine reports RowsAffected=0, got %d", entry.RowsAffected)
	}
}

func TestApp_RunQuery_LogsFailedExecutionToHistory(t *testing.T) {
	db := openHistoryTestDB(t)
	conn := createHistoryTestConnection(t, db)

	a := &App{ctx: context.Background(), db: db}
	connectionID := conn.ID
	wantErr := errors.New("syntax error near FROM")
	engine := &fakeQueryEngine{
		queryFunc: func(ctx context.Context, query string) (*dbengine.QueryResult, error) {
			return nil, wantErr
		},
	}
	a.putQuerySession("session-1", &querySession{engine: engine, connectionID: &connectionID})

	if _, err := a.RunQuery("session-1", "SELEKT * FROM widgets"); err == nil {
		t.Fatal("expected RunQuery to propagate the engine's error")
	}

	entries, err := storage.ListQueryHistory(db, storage.QueryHistoryFilter{ConnectionID: conn.ID})
	if err != nil {
		t.Fatalf("ListQueryHistory failed: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected exactly 1 logged history entry, got %d", len(entries))
	}

	entry := entries[0]
	if entry.Success {
		t.Error("expected a failed execution to log Success=false")
	}
	if entry.ErrorMessage == nil || *entry.ErrorMessage != wantErr.Error() {
		t.Errorf("expected ErrorMessage to carry the engine's error text, got %v", entry.ErrorMessage)
	}
	if entry.RowsAffected != 0 {
		t.Errorf("expected RowsAffected=0 for a failed execution, got %d", entry.RowsAffected)
	}
}

func TestApp_RunQuery_DoesNotLogAdHocSessionsWithoutAConnectionID(t *testing.T) {
	db := openHistoryTestDB(t)

	a := &App{ctx: context.Background(), db: db}
	engine := &fakeQueryEngine{}
	a.putQuerySession("session-1", &querySession{engine: engine})

	if _, err := a.RunQuery("session-1", "SELECT 1"); err != nil {
		t.Fatalf("RunQuery() failed: %v", err)
	}

	entries, err := storage.ListQueryHistory(db, storage.QueryHistoryFilter{})
	if err != nil {
		t.Fatalf("ListQueryHistory failed: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected an ad-hoc session (no SavedConnectionID) to log nothing, got %d entries", len(entries))
	}
}

func TestApp_RunQuery_DoesNotFailWhenStorageIsUnavailable(t *testing.T) {
	a := &App{ctx: context.Background()}
	connectionID := int64(1)
	engine := &fakeQueryEngine{}
	a.putQuerySession("session-1", &querySession{engine: engine, connectionID: &connectionID})

	if _, err := a.RunQuery("session-1", "SELECT 1"); err != nil {
		t.Fatalf("RunQuery() should succeed even when a.db is nil (best-effort logging), got error: %v", err)
	}
}

func TestConnectionFormFieldsFromStored_PopulatesSavedConnectionID(t *testing.T) {
	c := storage.Connection{ID: 42, Engine: storage.EnginePostgres, Host: "localhost", Port: 5432, ParamsJSON: "{}"}

	fields, err := connectionFormFieldsFromStored(c)
	if err != nil {
		t.Fatalf("connectionFormFieldsFromStored failed: %v", err)
	}
	if fields.SavedConnectionID != 42 {
		t.Errorf("expected SavedConnectionID to be populated from the stored connection's ID (task 3.5's ConnectUsingSavedConnection path), got %d", fields.SavedConnectionID)
	}
}
