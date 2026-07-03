package main

import (
	"context"
	"errors"
	"strings"
	"testing"

	"stackyard/internal/dbengine"
	"stackyard/internal/storage"
)

type fakeGridEngine struct {
	tables    []dbengine.TableInfo
	tablesErr error

	execFunc  func(ctx context.Context, query string, args ...any) (*dbengine.QueryResult, error)
	execCalls []struct {
		query string
		args  []any
	}
}

func (f *fakeGridEngine) Connect(context.Context) error { return nil }
func (f *fakeGridEngine) Ping(context.Context) error    { return nil }
func (f *fakeGridEngine) Query(context.Context, string) (*dbengine.QueryResult, error) {
	return nil, nil
}

func (f *fakeGridEngine) Exec(ctx context.Context, query string, args ...any) (*dbengine.QueryResult, error) {
	f.execCalls = append(f.execCalls, struct {
		query string
		args  []any
	}{query, args})
	if f.execFunc != nil {
		return f.execFunc(ctx, query, args...)
	}
	return &dbengine.QueryResult{RowsAffected: 1}, nil
}

func (f *fakeGridEngine) ListSchemas(context.Context) ([]string, error) { return nil, nil }

func (f *fakeGridEngine) ListTables(context.Context, string) ([]dbengine.TableInfo, error) {
	return f.tables, f.tablesErr
}

func (f *fakeGridEngine) ListForeignKeys(context.Context, string) ([]dbengine.ForeignKey, error) {
	return nil, nil
}

func (f *fakeGridEngine) Close() error { return nil }

func widgetsTableWithPK() []dbengine.TableInfo {
	return []dbengine.TableInfo{
		{
			Name: "widgets",
			Columns: []dbengine.ColumnInfo{
				{Name: "id", DataType: "bigint", IsPrimaryKey: true},
				{Name: "name", DataType: "text"},
			},
		},
	}
}

func widgetsTableWithoutPK() []dbengine.TableInfo {
	return []dbengine.TableInfo{
		{
			Name: "widgets",
			Columns: []dbengine.ColumnInfo{
				{Name: "name", DataType: "text"},
			},
		},
	}
}

func TestApp_BrowseTableRows_UnknownSessionReturnsError(t *testing.T) {
	a := &App{ctx: context.Background()}
	if _, err := a.BrowseTableRows("no-such-session", "public", "widgets", 50, 0); err == nil {
		t.Error("BrowseTableRows() with an unknown session: expected an error, got nil")
	}
}

func TestApp_BrowseTableRows_BuildsAndExecutesSelect(t *testing.T) {
	a := &App{ctx: context.Background()}
	engine := &fakeGridEngine{}
	a.putQuerySession("s1", &querySession{engine: engine, engineType: storage.EnginePostgres})

	want := &dbengine.QueryResult{Columns: []dbengine.ResultColumn{{Name: "id"}}, Rows: [][]any{{1}}}
	engine.execFunc = func(_ context.Context, query string, args ...any) (*dbengine.QueryResult, error) {
		return want, nil
	}

	got, err := a.BrowseTableRows("s1", "public", "widgets", 50, 100)
	if err != nil {
		t.Fatalf("BrowseTableRows() failed: %v", err)
	}
	if got != want {
		t.Errorf("BrowseTableRows() result = %v, want the engine's own result passed through", got)
	}
	if len(engine.execCalls) != 1 {
		t.Fatalf("Exec call count = %d, want 1", len(engine.execCalls))
	}
	if !strings.Contains(engine.execCalls[0].query, "LIMIT") {
		t.Errorf("Exec query = %q, want it to contain LIMIT", engine.execCalls[0].query)
	}
}

func TestApp_UpdateTableRow_NoPrimaryKeyReturnsReadOnlyError(t *testing.T) {
	a := &App{ctx: context.Background()}
	engine := &fakeGridEngine{tables: widgetsTableWithoutPK()}
	a.putQuerySession("s1", &querySession{engine: engine, engineType: storage.EnginePostgres})

	err := a.UpdateTableRow("s1", "public", "widgets", map[string]any{"id": int64(1)}, "name", "new")
	if err == nil {
		t.Fatal("UpdateTableRow() on a PK-less table: expected an error, got nil")
	}
	if !errors.Is(err, ErrTableHasNoPrimaryKey) {
		t.Errorf("UpdateTableRow() error = %v, want it to wrap ErrTableHasNoPrimaryKey", err)
	}
	if !strings.Contains(err.Error(), "read-only: table has no primary key") {
		t.Errorf("UpdateTableRow() error = %q, want it to contain the read-only reason text", err.Error())
	}
	if len(engine.execCalls) != 0 {
		t.Error("UpdateTableRow() should not attempt to execute an UPDATE against a PK-less table")
	}
}

func TestApp_UpdateTableRow_ExecutesParameterizedUpdate(t *testing.T) {
	a := &App{ctx: context.Background()}
	engine := &fakeGridEngine{tables: widgetsTableWithPK()}
	a.putQuerySession("s1", &querySession{engine: engine, engineType: storage.EnginePostgres})

	err := a.UpdateTableRow("s1", "public", "widgets", map[string]any{"id": int64(1)}, "name", "new-name")
	if err != nil {
		t.Fatalf("UpdateTableRow() failed: %v", err)
	}
	if len(engine.execCalls) != 1 {
		t.Fatalf("Exec call count = %d, want 1", len(engine.execCalls))
	}
	gotQuery := engine.execCalls[0].query
	if !strings.Contains(gotQuery, "UPDATE") || !strings.Contains(gotQuery, `"name"`) || !strings.Contains(gotQuery, `"id"`) {
		t.Errorf("Exec query = %q, want an UPDATE naming both the edited and primary key columns", gotQuery)
	}
	wantArgs := []any{"new-name", int64(1)}
	if len(engine.execCalls[0].args) != len(wantArgs) {
		t.Fatalf("Exec args = %v, want %v", engine.execCalls[0].args, wantArgs)
	}
}

func TestApp_UpdateTableRow_NoMatchingRowReturnsError(t *testing.T) {
	a := &App{ctx: context.Background()}
	engine := &fakeGridEngine{
		tables: widgetsTableWithPK(),
		execFunc: func(context.Context, string, ...any) (*dbengine.QueryResult, error) {
			return &dbengine.QueryResult{RowsAffected: 0}, nil
		},
	}
	a.putQuerySession("s1", &querySession{engine: engine, engineType: storage.EnginePostgres})

	err := a.UpdateTableRow("s1", "public", "widgets", map[string]any{"id": int64(999)}, "name", "x")
	if err == nil {
		t.Error("UpdateTableRow() with zero rows affected: expected an error, got nil")
	}
}

func TestApp_UpdateTableRow_PropagatesRealDatabaseError(t *testing.T) {
	a := &App{ctx: context.Background()}
	dbErr := errors.New(`postgres: exec: duplicate key value violates unique constraint "widgets_name_key" (SQLSTATE 23505)`)
	engine := &fakeGridEngine{
		tables: widgetsTableWithPK(),
		execFunc: func(context.Context, string, ...any) (*dbengine.QueryResult, error) {
			return nil, dbErr
		},
	}
	a.putQuerySession("s1", &querySession{engine: engine, engineType: storage.EnginePostgres})

	err := a.UpdateTableRow("s1", "public", "widgets", map[string]any{"id": int64(1)}, "name", "dup")
	if err == nil || !strings.Contains(err.Error(), "unique constraint") {
		t.Errorf("UpdateTableRow() error = %v, want it to surface the real database error text", err)
	}
}

func TestApp_InsertTableRow_PostgresUsesReturningResult(t *testing.T) {
	a := &App{ctx: context.Background()}
	engine := &fakeGridEngine{
		tables: widgetsTableWithPK(),
		execFunc: func(context.Context, string, ...any) (*dbengine.QueryResult, error) {
			return &dbengine.QueryResult{
				Columns: []dbengine.ResultColumn{{Name: "id"}, {Name: "name"}},
				Rows:    [][]any{{int64(1), "widget"}},
			}, nil
		},
	}
	a.putQuerySession("s1", &querySession{engine: engine, engineType: storage.EnginePostgres})

	row, err := a.InsertTableRow("s1", "public", "widgets", map[string]any{"name": "widget"})
	if err != nil {
		t.Fatalf("InsertTableRow() failed: %v", err)
	}
	if row["id"] != int64(1) || row["name"] != "widget" {
		t.Errorf("InsertTableRow() row = %v, want the RETURNING row's values", row)
	}
	if len(engine.execCalls) != 1 || !strings.Contains(engine.execCalls[0].query, "RETURNING") {
		t.Errorf("Exec calls = %v, want exactly one INSERT with RETURNING", engine.execCalls)
	}
}

func TestApp_InsertTableRow_MySQLReSelectsByLastInsertID(t *testing.T) {
	a := &App{ctx: context.Background()}
	callCount := 0
	engine := &fakeGridEngine{
		tables: widgetsTableWithPK(),
		execFunc: func(_ context.Context, query string, _ ...any) (*dbengine.QueryResult, error) {
			callCount++
			if strings.HasPrefix(query, "INSERT") {
				return &dbengine.QueryResult{RowsAffected: 1, LastInsertID: 42}, nil
			}
			return &dbengine.QueryResult{
				Columns: []dbengine.ResultColumn{{Name: "id"}, {Name: "name"}},
				Rows:    [][]any{{int64(42), "widget"}},
			}, nil
		},
	}
	a.putQuerySession("s1", &querySession{engine: engine, engineType: storage.EngineMySQL})

	row, err := a.InsertTableRow("s1", "public", "widgets", map[string]any{"name": "widget"})
	if err != nil {
		t.Fatalf("InsertTableRow() failed: %v", err)
	}
	if callCount != 2 {
		t.Fatalf("Exec call count = %d, want 2 (INSERT then re-SELECT)", callCount)
	}
	if row["id"] != int64(42) {
		t.Errorf("InsertTableRow() row = %v, want id 42 from the re-SELECT", row)
	}
}

func TestApp_InsertTableRow_MySQLNoUsablePrimaryKeyFallsBackToSubmittedValues(t *testing.T) {
	a := &App{ctx: context.Background()}
	engine := &fakeGridEngine{
		tables: widgetsTableWithoutPK(),
		execFunc: func(context.Context, string, ...any) (*dbengine.QueryResult, error) {
			return &dbengine.QueryResult{RowsAffected: 1}, nil
		},
	}
	a.putQuerySession("s1", &querySession{engine: engine, engineType: storage.EngineMySQL})

	row, err := a.InsertTableRow("s1", "public", "widgets", map[string]any{"name": "widget"})
	if err != nil {
		t.Fatalf("InsertTableRow() failed: %v", err)
	}
	if row["name"] != "widget" {
		t.Errorf("InsertTableRow() row = %v, want the submitted values as a best-effort fallback", row)
	}
	if len(engine.execCalls) != 1 {
		t.Errorf("Exec call count = %d, want 1 (no re-SELECT possible without a usable primary key)", len(engine.execCalls))
	}
}

func TestApp_DeleteTableRows_NoPrimaryKeyReturnsReadOnlyError(t *testing.T) {
	a := &App{ctx: context.Background()}
	engine := &fakeGridEngine{tables: widgetsTableWithoutPK()}
	a.putQuerySession("s1", &querySession{engine: engine, engineType: storage.EnginePostgres})

	_, err := a.DeleteTableRows("s1", "public", "widgets", []map[string]any{{"id": int64(1)}})
	if err == nil || !errors.Is(err, ErrTableHasNoPrimaryKey) {
		t.Errorf("DeleteTableRows() error = %v, want ErrTableHasNoPrimaryKey", err)
	}
}

func TestApp_DeleteTableRows_RunsEachRowIndependently(t *testing.T) {
	a := &App{ctx: context.Background()}
	boom := errors.New("update or delete on table \"widgets\" violates foreign key constraint")
	engine := &fakeGridEngine{
		tables: widgetsTableWithPK(),
		execFunc: func(_ context.Context, query string, args ...any) (*dbengine.QueryResult, error) {
			if len(args) > 0 && args[0] == int64(2) {
				return nil, boom
			}
			return &dbengine.QueryResult{RowsAffected: 1}, nil
		},
	}
	a.putQuerySession("s1", &querySession{engine: engine, engineType: storage.EnginePostgres})

	results, err := a.DeleteTableRows("s1", "public", "widgets", []map[string]any{
		{"id": int64(1)},
		{"id": int64(2)},
		{"id": int64(3)},
	})
	if err != nil {
		t.Fatalf("DeleteTableRows() failed: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("len(results) = %d, want 3", len(results))
	}
	if !results[0].Success || results[1].Success || !results[2].Success {
		t.Errorf("results = %+v, want row 1 (index 1) to fail independently of rows 0 and 2 succeeding", results)
	}
	if !strings.Contains(results[1].ErrorMessage, "foreign key constraint") {
		t.Errorf("results[1].ErrorMessage = %q, want the real database error text", results[1].ErrorMessage)
	}
}

func TestApp_DeleteTableRows_EmptyListReturnsError(t *testing.T) {
	a := &App{ctx: context.Background()}
	engine := &fakeGridEngine{tables: widgetsTableWithPK()}
	a.putQuerySession("s1", &querySession{engine: engine, engineType: storage.EnginePostgres})

	if _, err := a.DeleteTableRows("s1", "public", "widgets", nil); err == nil {
		t.Error("DeleteTableRows() with an empty row list: expected an error, got nil")
	}
}

func TestApp_GridBoundMethods_RejectUnsupportedEngines(t *testing.T) {
	a := &App{ctx: context.Background()}
	engine := &fakeGridEngine{}
	a.putQuerySession("s1", &querySession{engine: engine, engineType: storage.EngineMongoDB})

	if _, err := a.BrowseTableRows("s1", "", "widgets", 50, 0); err == nil {
		t.Error("BrowseTableRows() on a MongoDB session: expected an error, got nil")
	}
}
