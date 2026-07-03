package main

import (
	"context"
	"errors"
	"strings"
	"testing"

	"stackyard/internal/dbengine"
	"stackyard/internal/storage"
)

func TestApp_RunMultiStatementQuery_UnknownSessionReturnsError(t *testing.T) {
	a := &App{ctx: context.Background()}

	if _, err := a.RunMultiStatementQuery("does-not-exist", "SELECT 1;"); err == nil {
		t.Error("RunMultiStatementQuery() with an unknown session ID: expected an error, got nil")
	} else if !strings.Contains(err.Error(), "does-not-exist") {
		t.Errorf("RunMultiStatementQuery() error = %q, want it to name the missing session ID", err.Error())
	}
}

func TestApp_RunMultiStatementQuery_EmptyQueryReturnsError(t *testing.T) {
	a := &App{ctx: context.Background()}
	engine := &fakeQueryEngine{}
	a.putQuerySession("session-1", &querySession{engine: engine})

	if _, err := a.RunMultiStatementQuery("session-1", "   ;  ;  "); err == nil {
		t.Error("RunMultiStatementQuery() with no real statements: expected an error, got nil")
	}
}

func TestApp_RunMultiStatementQuery_SingleStatementReturnsOneResult(t *testing.T) {
	a := &App{ctx: context.Background()}
	var gotQueries []string
	engine := &fakeQueryEngine{
		queryFunc: func(ctx context.Context, query string) (*dbengine.QueryResult, error) {
			gotQueries = append(gotQueries, query)
			return &dbengine.QueryResult{Columns: []dbengine.ResultColumn{{Name: "n"}}, Rows: [][]any{{1}}}, nil
		},
	}
	a.putQuerySession("session-1", &querySession{engine: engine})

	results, err := a.RunMultiStatementQuery("session-1", "SELECT 1")
	if err != nil {
		t.Fatalf("RunMultiStatementQuery() failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("len(results) = %d, want 1", len(results))
	}
	if !results[0].Success {
		t.Errorf("results[0].Success = false, want true")
	}
	if len(gotQueries) != 1 || gotQueries[0] != "SELECT 1" {
		t.Errorf("engine received queries %v, want exactly one call with %q", gotQueries, "SELECT 1")
	}
}

func TestApp_RunMultiStatementQuery_RunsEachStatementIndependently(t *testing.T) {
	a := &App{ctx: context.Background()}
	boom := errors.New("syntax error near DELETE")
	engine := &fakeQueryEngine{
		queryFunc: func(ctx context.Context, query string) (*dbengine.QueryResult, error) {
			if strings.HasPrefix(query, "DELETE") {
				return nil, boom
			}
			return &dbengine.QueryResult{RowsAffected: 1}, nil
		},
	}
	a.putQuerySession("session-1", &querySession{engine: engine})

	results, err := a.RunMultiStatementQuery("session-1", "INSERT INTO widgets (id) VALUES (1); DELETE FROM widgets; UPDATE widgets SET name = 'x'")
	if err != nil {
		t.Fatalf("RunMultiStatementQuery() failed: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("len(results) = %d, want 3", len(results))
	}
	if !results[0].Success || results[1].Success || !results[2].Success {
		t.Errorf("results = %+v, want statement 1 (index 1) to fail independently of statements 0 and 2 succeeding", results)
	}
	if !strings.Contains(results[1].ErrorMessage, "syntax error near DELETE") {
		t.Errorf("results[1].ErrorMessage = %q, want the real database error text", results[1].ErrorMessage)
	}
}

func TestApp_RunMultiStatementQuery_LogsOneHistoryEntryPerStatement(t *testing.T) {
	db := openHistoryTestDB(t)
	conn := createHistoryTestConnection(t, db)

	a := &App{ctx: context.Background(), db: db}
	connectionID := conn.ID
	engine := &fakeQueryEngine{
		queryFunc: func(ctx context.Context, query string) (*dbengine.QueryResult, error) {
			return &dbengine.QueryResult{RowsAffected: 1}, nil
		},
	}
	a.putQuerySession("session-1", &querySession{engine: engine, connectionID: &connectionID})

	if _, err := a.RunMultiStatementQuery("session-1", "UPDATE a SET x = 1; UPDATE b SET y = 2"); err != nil {
		t.Fatalf("RunMultiStatementQuery() failed: %v", err)
	}

	entries, err := storage.ListQueryHistory(db, storage.QueryHistoryFilter{ConnectionID: conn.ID})
	if err != nil {
		t.Fatalf("ListQueryHistory failed: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected exactly 2 logged history entries (one per statement), got %d", len(entries))
	}
}
