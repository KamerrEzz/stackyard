package main

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"stackyard/internal/dbengine"
	"stackyard/internal/storage"
)

// fakeQueryEngine is a dbengine.Engine test double used to exercise
// OpenConnection/RunQuery/CancelQuery/CloseConnectionSession's session-map
// bookkeeping without a live database connection.
type fakeQueryEngine struct {
	mu        sync.Mutex
	closeErr  error
	closed    bool
	queryFunc func(ctx context.Context, query string) (*dbengine.QueryResult, error)
}

func (f *fakeQueryEngine) Connect(ctx context.Context) error { return nil }

func (f *fakeQueryEngine) Ping(ctx context.Context) error { return nil }

func (f *fakeQueryEngine) Query(ctx context.Context, query string) (*dbengine.QueryResult, error) {
	if f.queryFunc != nil {
		return f.queryFunc(ctx, query)
	}
	return &dbengine.QueryResult{Columns: []dbengine.ResultColumn{{Name: "col"}}, Rows: [][]any{{query}}}, nil
}

func (f *fakeQueryEngine) Exec(ctx context.Context, query string, args ...any) (*dbengine.QueryResult, error) {
	return f.Query(ctx, query)
}

func (f *fakeQueryEngine) ListSchemas(ctx context.Context) ([]string, error) { return nil, nil }

func (f *fakeQueryEngine) ListTables(ctx context.Context, schema string) ([]dbengine.TableInfo, error) {
	return nil, nil
}

func (f *fakeQueryEngine) ListForeignKeys(ctx context.Context, schema string) ([]dbengine.ForeignKey, error) {
	return nil, nil
}

func (f *fakeQueryEngine) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed = true
	return f.closeErr
}

func (f *fakeQueryEngine) isClosed() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.closed
}

func TestApp_QuerySessionBookkeeping_PutGetDelete(t *testing.T) {
	a := &App{}
	engine := &fakeQueryEngine{}

	a.putQuerySession("session-1", &querySession{engine: engine})

	got, ok := a.getQuerySession("session-1")
	if !ok || got.engine != engine {
		t.Fatalf("getQuerySession(\"session-1\") = (%v, %v), want the stored session", got, ok)
	}

	deleted, ok := a.deleteQuerySession("session-1")
	if !ok || deleted.engine != engine {
		t.Fatalf("deleteQuerySession(\"session-1\") = (%v, %v), want the stored session", deleted, ok)
	}

	if _, ok := a.getQuerySession("session-1"); ok {
		t.Error("getQuerySession(\"session-1\") after delete: expected not found, got a session")
	}
}

func TestApp_RunQuery_NotFoundSessionReturnsError(t *testing.T) {
	a := &App{ctx: context.Background()}

	if _, err := a.RunQuery("does-not-exist", "SELECT 1"); err == nil {
		t.Error("RunQuery() with an unknown session ID: expected an error, got nil")
	} else if !strings.Contains(err.Error(), "does-not-exist") {
		t.Errorf("RunQuery() error = %q, want it to name the missing session ID", err.Error())
	}
}

func TestApp_RunQuery_CallsEngineQueryAndClearsCancelAfterwards(t *testing.T) {
	a := &App{ctx: context.Background()}
	var gotQuery string
	engine := &fakeQueryEngine{
		queryFunc: func(ctx context.Context, query string) (*dbengine.QueryResult, error) {
			gotQuery = query
			return &dbengine.QueryResult{Columns: []dbengine.ResultColumn{{Name: "n"}}, Rows: [][]any{{1}}}, nil
		},
	}
	a.putQuerySession("session-1", &querySession{engine: engine})

	result, err := a.RunQuery("session-1", "SELECT 1")
	if err != nil {
		t.Fatalf("RunQuery() failed: %v", err)
	}
	if gotQuery != "SELECT 1" {
		t.Errorf("engine.Query() received %q, want %q", gotQuery, "SELECT 1")
	}
	if len(result.Rows) != 1 {
		t.Errorf("RunQuery() result = %+v, want the fake engine's result passed through", result)
	}

	if _, ok := a.popQueryCancel("session-1"); ok {
		t.Error("expected RunQuery to clear its cancel func after finishing, but one was still registered")
	}
}

func TestApp_RunQuery_PropagatesEngineError(t *testing.T) {
	a := &App{ctx: context.Background()}
	wantErr := errors.New("boom")
	engine := &fakeQueryEngine{
		queryFunc: func(ctx context.Context, query string) (*dbengine.QueryResult, error) {
			return nil, wantErr
		},
	}
	a.putQuerySession("session-1", &querySession{engine: engine})

	if _, err := a.RunQuery("session-1", "SELECT 1"); err == nil || !strings.Contains(err.Error(), "boom") {
		t.Errorf("RunQuery() error = %v, want it to wrap the engine's own error", err)
	}
}

func TestApp_CancelQuery_CancelsTheInFlightQuery(t *testing.T) {
	a := &App{ctx: context.Background()}
	started := make(chan struct{})
	engine := &fakeQueryEngine{
		queryFunc: func(ctx context.Context, query string) (*dbengine.QueryResult, error) {
			close(started)
			<-ctx.Done()
			return nil, ctx.Err()
		},
	}
	a.putQuerySession("session-1", &querySession{engine: engine})

	done := make(chan error, 1)
	go func() {
		_, err := a.RunQuery("session-1", "SELECT pg_sleep(30)")
		done <- err
	}()

	<-started
	if err := a.CancelQuery("session-1"); err != nil {
		t.Fatalf("CancelQuery() failed: %v", err)
	}

	select {
	case err := <-done:
		if err == nil {
			t.Error("RunQuery() after CancelQuery: expected an error, got nil")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("RunQuery() did not return within 2s of CancelQuery being called")
	}
}

func TestApp_CancelQuery_NoOpWhenNothingInFlight(t *testing.T) {
	a := &App{ctx: context.Background()}

	if err := a.CancelQuery("no-such-session"); err != nil {
		t.Errorf("CancelQuery() with nothing in flight = %v, want nil (a no-op, not an error)", err)
	}
}

func TestApp_CloseConnectionSession_NotFoundReturnsError(t *testing.T) {
	a := &App{ctx: context.Background()}

	if err := a.CloseConnectionSession("does-not-exist"); err == nil {
		t.Error("CloseConnectionSession() with an unknown session ID: expected an error, got nil")
	}
}

func TestApp_CloseConnectionSession_ClosesEngineAndRemovesSession(t *testing.T) {
	a := &App{ctx: context.Background()}
	engine := &fakeQueryEngine{}
	a.putQuerySession("session-1", &querySession{engine: engine})

	if err := a.CloseConnectionSession("session-1"); err != nil {
		t.Fatalf("CloseConnectionSession() failed: %v", err)
	}
	if !engine.isClosed() {
		t.Error("CloseConnectionSession() did not close the underlying engine")
	}
	if _, ok := a.getQuerySession("session-1"); ok {
		t.Error("CloseConnectionSession() left the session in the map")
	}
}

func TestApp_CloseConnectionSession_CancelsInFlightQueryFirst(t *testing.T) {
	a := &App{ctx: context.Background()}
	started := make(chan struct{})
	engine := &fakeQueryEngine{
		queryFunc: func(ctx context.Context, query string) (*dbengine.QueryResult, error) {
			close(started)
			<-ctx.Done()
			return nil, ctx.Err()
		},
	}
	a.putQuerySession("session-1", &querySession{engine: engine})

	done := make(chan error, 1)
	go func() {
		_, err := a.RunQuery("session-1", "SELECT pg_sleep(30)")
		done <- err
	}()

	<-started
	if err := a.CloseConnectionSession("session-1"); err != nil {
		t.Fatalf("CloseConnectionSession() failed: %v", err)
	}

	select {
	case err := <-done:
		if err == nil {
			t.Error("RunQuery() after CloseConnectionSession: expected an error, got nil")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("RunQuery() did not return within 2s of CloseConnectionSession cancelling it")
	}
}

func TestApp_OpenConnection_RejectsMissingHost(t *testing.T) {
	a := &App{ctx: context.Background()}

	if _, err := a.OpenConnection(ConnectionFormFields{Engine: storage.EnginePostgres, Port: 5432}); err == nil {
		t.Error("OpenConnection() with a blank host: expected an error, got nil")
	}
}

func TestApp_CloseAllQuerySessions_ClosesEveryEngineAndCancelsInFlightQueries(t *testing.T) {
	a := &App{ctx: context.Background()}
	engineA := &fakeQueryEngine{}
	engineB := &fakeQueryEngine{}
	a.putQuerySession("session-a", &querySession{engine: engineA})
	a.putQuerySession("session-b", &querySession{engine: engineB})

	started := make(chan struct{})
	blocking := &fakeQueryEngine{
		queryFunc: func(ctx context.Context, query string) (*dbengine.QueryResult, error) {
			close(started)
			<-ctx.Done()
			return nil, ctx.Err()
		},
	}
	a.putQuerySession("session-c", &querySession{engine: blocking})
	done := make(chan error, 1)
	go func() {
		_, err := a.RunQuery("session-c", "SELECT pg_sleep(30)")
		done <- err
	}()
	<-started

	a.closeAllQuerySessions()

	if !engineA.isClosed() || !engineB.isClosed() || !blocking.isClosed() {
		t.Error("closeAllQuerySessions() did not close every registered engine")
	}

	select {
	case err := <-done:
		if err == nil {
			t.Error("RunQuery() after closeAllQuerySessions: expected an error, got nil")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("RunQuery() did not return within 2s of closeAllQuerySessions cancelling it")
	}

	if _, ok := a.getQuerySession("session-a"); ok {
		t.Error("closeAllQuerySessions() left session-a in the map")
	}
}
