package main

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"stackyard/internal/storage"
)

// fakeMongoEngine is a mongoEngine test double used to exercise
// OpenMongoConnection/CloseMongoSession/ListMongoDatabases/
// ListMongoCollections/FindMongoDocuments/CountMongoDocuments/
// InsertMongoDocument/UpdateMongoDocument/DeleteMongoDocuments's session-map
// bookkeeping without a live MongoDB connection, mirroring
// query_session_test.go's fakeQueryEngine pattern.
type fakeMongoEngine struct {
	mu     sync.Mutex
	closed bool

	closeErr error

	listDatabasesFunc   func(ctx context.Context) ([]string, error)
	listCollectionsFunc func(ctx context.Context, database string) ([]string, error)
	findDocumentsFunc   func(ctx context.Context, database, collection string, filter map[string]any, limit, skip int) ([]map[string]any, error)
	countDocumentsFunc  func(ctx context.Context, database, collection string, filter map[string]any) (int64, error)
	insertDocumentFunc  func(ctx context.Context, database, collection string, doc map[string]any) (map[string]any, error)
	updateDocumentFunc  func(ctx context.Context, database, collection, id string, doc map[string]any) error
	deleteDocumentsFunc func(ctx context.Context, database, collection string, ids []string) error
}

func (f *fakeMongoEngine) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed = true
	return f.closeErr
}

func (f *fakeMongoEngine) isClosed() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.closed
}

func (f *fakeMongoEngine) ListDatabases(ctx context.Context) ([]string, error) {
	if f.listDatabasesFunc != nil {
		return f.listDatabasesFunc(ctx)
	}
	return nil, nil
}

func (f *fakeMongoEngine) ListCollections(ctx context.Context, database string) ([]string, error) {
	if f.listCollectionsFunc != nil {
		return f.listCollectionsFunc(ctx, database)
	}
	return nil, nil
}

func (f *fakeMongoEngine) FindDocuments(ctx context.Context, database, collection string, filter map[string]any, limit, skip int) ([]map[string]any, error) {
	if f.findDocumentsFunc != nil {
		return f.findDocumentsFunc(ctx, database, collection, filter, limit, skip)
	}
	return nil, nil
}

func (f *fakeMongoEngine) CountDocuments(ctx context.Context, database, collection string, filter map[string]any) (int64, error) {
	if f.countDocumentsFunc != nil {
		return f.countDocumentsFunc(ctx, database, collection, filter)
	}
	return 0, nil
}

func (f *fakeMongoEngine) InsertDocument(ctx context.Context, database, collection string, doc map[string]any) (map[string]any, error) {
	if f.insertDocumentFunc != nil {
		return f.insertDocumentFunc(ctx, database, collection, doc)
	}
	return nil, nil
}

func (f *fakeMongoEngine) UpdateDocument(ctx context.Context, database, collection, id string, doc map[string]any) error {
	if f.updateDocumentFunc != nil {
		return f.updateDocumentFunc(ctx, database, collection, id, doc)
	}
	return nil
}

func (f *fakeMongoEngine) DeleteDocuments(ctx context.Context, database, collection string, ids []string) error {
	if f.deleteDocumentsFunc != nil {
		return f.deleteDocumentsFunc(ctx, database, collection, ids)
	}
	return nil
}

func TestApp_MongoSessionBookkeeping_PutGetDelete(t *testing.T) {
	a := &App{}
	engine := &fakeMongoEngine{}

	a.putMongoSession("session-1", &mongoSession{engine: engine})

	got, ok := a.getMongoSession("session-1")
	if !ok || got.engine != engine {
		t.Fatalf("getMongoSession(\"session-1\") = (%v, %v), want the stored session", got, ok)
	}

	deleted, ok := a.deleteMongoSession("session-1")
	if !ok || deleted.engine != engine {
		t.Fatalf("deleteMongoSession(\"session-1\") = (%v, %v), want the stored session", deleted, ok)
	}

	if _, ok := a.getMongoSession("session-1"); ok {
		t.Error("getMongoSession(\"session-1\") after delete: expected not found, got a session")
	}
}

func TestApp_OpenMongoConnection_RejectsNonMongoEngine(t *testing.T) {
	a := &App{ctx: context.Background()}

	fields := ConnectionFormFields{Engine: storage.EnginePostgres, Host: "localhost", Port: 5432}
	if _, err := a.OpenMongoConnection(fields); err == nil {
		t.Error("OpenMongoConnection() with a non-Mongo engine: expected an error, got nil")
	} else if !strings.Contains(err.Error(), "expected engine") {
		t.Errorf("OpenMongoConnection() error = %q, want it to name the engine mismatch", err.Error())
	}
}

func TestApp_OpenMongoConnection_RejectsMissingHost(t *testing.T) {
	a := &App{ctx: context.Background()}

	fields := ConnectionFormFields{Engine: storage.EngineMongoDB, Port: 27017}
	if _, err := a.OpenMongoConnection(fields); err == nil {
		t.Error("OpenMongoConnection() with a blank host: expected an error, got nil")
	}
}

func TestApp_CloseMongoSession_NotFoundReturnsError(t *testing.T) {
	a := &App{ctx: context.Background()}

	if err := a.CloseMongoSession("does-not-exist"); err == nil {
		t.Error("CloseMongoSession() with an unknown session ID: expected an error, got nil")
	} else if !strings.Contains(err.Error(), "does-not-exist") {
		t.Errorf("CloseMongoSession() error = %q, want it to name the missing session ID", err.Error())
	}
}

func TestApp_CloseMongoSession_ClosesEngineAndRemovesSession(t *testing.T) {
	a := &App{ctx: context.Background()}
	engine := &fakeMongoEngine{}
	a.putMongoSession("session-1", &mongoSession{engine: engine})

	if err := a.CloseMongoSession("session-1"); err != nil {
		t.Fatalf("CloseMongoSession() failed: %v", err)
	}
	if !engine.isClosed() {
		t.Error("CloseMongoSession() did not close the underlying engine")
	}
	if _, ok := a.getMongoSession("session-1"); ok {
		t.Error("CloseMongoSession() left the session in the map")
	}
}

func TestApp_CloseMongoSession_PropagatesEngineCloseError(t *testing.T) {
	a := &App{ctx: context.Background()}
	wantErr := errors.New("boom")
	engine := &fakeMongoEngine{closeErr: wantErr}
	a.putMongoSession("session-1", &mongoSession{engine: engine})

	if err := a.CloseMongoSession("session-1"); err == nil || !strings.Contains(err.Error(), "boom") {
		t.Errorf("CloseMongoSession() error = %v, want it to wrap the engine's own close error", err)
	}
}

func TestApp_CloseAllMongoSessions_ClosesEveryEngine(t *testing.T) {
	a := &App{ctx: context.Background()}
	engineA := &fakeMongoEngine{}
	engineB := &fakeMongoEngine{}
	a.putMongoSession("session-a", &mongoSession{engine: engineA})
	a.putMongoSession("session-b", &mongoSession{engine: engineB})

	a.closeAllMongoSessions()

	if !engineA.isClosed() || !engineB.isClosed() {
		t.Error("closeAllMongoSessions() did not close every registered engine")
	}
	if _, ok := a.getMongoSession("session-a"); ok {
		t.Error("closeAllMongoSessions() left session-a in the map")
	}
}

func TestApp_ListMongoDatabases_NotFoundSessionReturnsError(t *testing.T) {
	a := &App{ctx: context.Background()}

	if _, err := a.ListMongoDatabases("does-not-exist"); err == nil {
		t.Error("ListMongoDatabases() with an unknown session ID: expected an error, got nil")
	} else if !strings.Contains(err.Error(), "does-not-exist") {
		t.Errorf("ListMongoDatabases() error = %q, want it to name the missing session ID", err.Error())
	}
}

func TestApp_ListMongoDatabases_ReturnsEmptySliceNotNil(t *testing.T) {
	a := &App{ctx: context.Background()}
	engine := &fakeMongoEngine{
		listDatabasesFunc: func(ctx context.Context) ([]string, error) { return nil, nil },
	}
	a.putMongoSession("session-1", &mongoSession{engine: engine})

	got, err := a.ListMongoDatabases("session-1")
	if err != nil {
		t.Fatalf("ListMongoDatabases() failed: %v", err)
	}
	if got == nil {
		t.Error("ListMongoDatabases() returned nil, want a non-nil empty slice")
	}
}

func TestApp_ListMongoCollections_CallsEngineWithDatabase(t *testing.T) {
	a := &App{ctx: context.Background()}
	var gotDatabase string
	engine := &fakeMongoEngine{
		listCollectionsFunc: func(ctx context.Context, database string) ([]string, error) {
			gotDatabase = database
			return []string{"widgets"}, nil
		},
	}
	a.putMongoSession("session-1", &mongoSession{engine: engine})

	got, err := a.ListMongoCollections("session-1", "mydb")
	if err != nil {
		t.Fatalf("ListMongoCollections() failed: %v", err)
	}
	if gotDatabase != "mydb" {
		t.Errorf("engine.ListCollections() received database %q, want %q", gotDatabase, "mydb")
	}
	if len(got) != 1 || got[0] != "widgets" {
		t.Errorf("ListMongoCollections() = %v, want [\"widgets\"]", got)
	}
}

func TestApp_FindMongoDocuments_DecodesFilterJSONAndPassesLimitSkip(t *testing.T) {
	a := &App{ctx: context.Background()}
	var gotFilter map[string]any
	var gotLimit, gotSkip int
	engine := &fakeMongoEngine{
		findDocumentsFunc: func(ctx context.Context, database, collection string, filter map[string]any, limit, skip int) ([]map[string]any, error) {
			gotFilter = filter
			gotLimit = limit
			gotSkip = skip
			return []map[string]any{{"name": "bolt"}}, nil
		},
	}
	a.putMongoSession("session-1", &mongoSession{engine: engine})

	got, err := a.FindMongoDocuments("session-1", "mydb", "widgets", `{"name":"bolt"}`, 50, 10)
	if err != nil {
		t.Fatalf("FindMongoDocuments() failed: %v", err)
	}
	if gotFilter["name"] != "bolt" {
		t.Errorf("engine.FindDocuments() received filter %v, want {name: bolt}", gotFilter)
	}
	if gotLimit != 50 || gotSkip != 10 {
		t.Errorf("engine.FindDocuments() received limit=%d skip=%d, want limit=50 skip=10", gotLimit, gotSkip)
	}
	if len(got) != 1 {
		t.Errorf("FindMongoDocuments() = %v, want the fake engine's result passed through", got)
	}
}

func TestApp_FindMongoDocuments_BlankFilterMeansMatchEverything(t *testing.T) {
	a := &App{ctx: context.Background()}
	var gotFilter map[string]any
	engine := &fakeMongoEngine{
		findDocumentsFunc: func(ctx context.Context, database, collection string, filter map[string]any, limit, skip int) ([]map[string]any, error) {
			gotFilter = filter
			return nil, nil
		},
	}
	a.putMongoSession("session-1", &mongoSession{engine: engine})

	if _, err := a.FindMongoDocuments("session-1", "mydb", "widgets", "", 0, 0); err != nil {
		t.Fatalf("FindMongoDocuments() failed: %v", err)
	}
	if len(gotFilter) != 0 {
		t.Errorf("engine.FindDocuments() received filter %v, want an empty map for a blank filterJSON", gotFilter)
	}
}

func TestApp_FindMongoDocuments_InvalidFilterJSONReturnsError(t *testing.T) {
	a := &App{ctx: context.Background()}
	a.putMongoSession("session-1", &mongoSession{engine: &fakeMongoEngine{}})

	if _, err := a.FindMongoDocuments("session-1", "mydb", "widgets", "{not json", 10, 0); err == nil {
		t.Error("FindMongoDocuments() with malformed filter JSON: expected an error, got nil")
	}
}

func TestApp_CountMongoDocuments_NotFoundSessionReturnsError(t *testing.T) {
	a := &App{ctx: context.Background()}

	if _, err := a.CountMongoDocuments("does-not-exist", "mydb", "widgets", ""); err == nil {
		t.Error("CountMongoDocuments() with an unknown session ID: expected an error, got nil")
	}
}

func TestApp_CountMongoDocuments_PropagatesEngineCount(t *testing.T) {
	a := &App{ctx: context.Background()}
	engine := &fakeMongoEngine{
		countDocumentsFunc: func(ctx context.Context, database, collection string, filter map[string]any) (int64, error) {
			return 42, nil
		},
	}
	a.putMongoSession("session-1", &mongoSession{engine: engine})

	got, err := a.CountMongoDocuments("session-1", "mydb", "widgets", "")
	if err != nil {
		t.Fatalf("CountMongoDocuments() failed: %v", err)
	}
	if got != 42 {
		t.Errorf("CountMongoDocuments() = %d, want 42", got)
	}
}

func TestApp_InsertMongoDocument_DecodesDocJSONAndReturnsEngineResult(t *testing.T) {
	a := &App{ctx: context.Background()}
	var gotDoc map[string]any
	engine := &fakeMongoEngine{
		insertDocumentFunc: func(ctx context.Context, database, collection string, doc map[string]any) (map[string]any, error) {
			gotDoc = doc
			return map[string]any{"_id": "abc123", "name": "bolt"}, nil
		},
	}
	a.putMongoSession("session-1", &mongoSession{engine: engine})

	got, err := a.InsertMongoDocument("session-1", "mydb", "widgets", `{"name":"bolt"}`)
	if err != nil {
		t.Fatalf("InsertMongoDocument() failed: %v", err)
	}
	if gotDoc["name"] != "bolt" {
		t.Errorf("engine.InsertDocument() received doc %v, want {name: bolt}", gotDoc)
	}
	if got["_id"] != "abc123" {
		t.Errorf("InsertMongoDocument() = %v, want it to pass through the engine's returned document", got)
	}
}

func TestApp_InsertMongoDocument_InvalidDocJSONReturnsError(t *testing.T) {
	a := &App{ctx: context.Background()}
	a.putMongoSession("session-1", &mongoSession{engine: &fakeMongoEngine{}})

	if _, err := a.InsertMongoDocument("session-1", "mydb", "widgets", "{not json"); err == nil {
		t.Error("InsertMongoDocument() with malformed document JSON: expected an error, got nil")
	}
}

func TestApp_UpdateMongoDocument_NotFoundSessionReturnsError(t *testing.T) {
	a := &App{ctx: context.Background()}

	if err := a.UpdateMongoDocument("does-not-exist", "mydb", "widgets", "abc123", "{}"); err == nil {
		t.Error("UpdateMongoDocument() with an unknown session ID: expected an error, got nil")
	}
}

func TestApp_UpdateMongoDocument_CallsEngineWithIDAndDoc(t *testing.T) {
	a := &App{ctx: context.Background()}
	var gotID string
	var gotDoc map[string]any
	engine := &fakeMongoEngine{
		updateDocumentFunc: func(ctx context.Context, database, collection, id string, doc map[string]any) error {
			gotID = id
			gotDoc = doc
			return nil
		},
	}
	a.putMongoSession("session-1", &mongoSession{engine: engine})

	if err := a.UpdateMongoDocument("session-1", "mydb", "widgets", "abc123", `{"name":"bolt2"}`); err != nil {
		t.Fatalf("UpdateMongoDocument() failed: %v", err)
	}
	if gotID != "abc123" {
		t.Errorf("engine.UpdateDocument() received id %q, want %q", gotID, "abc123")
	}
	if gotDoc["name"] != "bolt2" {
		t.Errorf("engine.UpdateDocument() received doc %v, want {name: bolt2}", gotDoc)
	}
}

func TestApp_DeleteMongoDocuments_NotFoundSessionReturnsError(t *testing.T) {
	a := &App{ctx: context.Background()}

	if err := a.DeleteMongoDocuments("does-not-exist", "mydb", "widgets", []string{"abc123"}); err == nil {
		t.Error("DeleteMongoDocuments() with an unknown session ID: expected an error, got nil")
	}
}

func TestApp_DeleteMongoDocuments_SupportsMultipleIDs(t *testing.T) {
	a := &App{ctx: context.Background()}
	var gotIDs []string
	engine := &fakeMongoEngine{
		deleteDocumentsFunc: func(ctx context.Context, database, collection string, ids []string) error {
			gotIDs = ids
			return nil
		},
	}
	a.putMongoSession("session-1", &mongoSession{engine: engine})

	if err := a.DeleteMongoDocuments("session-1", "mydb", "widgets", []string{"id1", "id2"}); err != nil {
		t.Fatalf("DeleteMongoDocuments() failed: %v", err)
	}
	if len(gotIDs) != 2 || gotIDs[0] != "id1" || gotIDs[1] != "id2" {
		t.Errorf("engine.DeleteDocuments() received ids %v, want [id1 id2]", gotIDs)
	}
}

func TestDecodeMongoJSONObject_BlankReturnsEmptyMap(t *testing.T) {
	got, err := decodeMongoJSONObject("   ")
	if err != nil {
		t.Fatalf("decodeMongoJSONObject(blank) failed: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("decodeMongoJSONObject(blank) = %v, want an empty map", got)
	}
}

func TestDecodeMongoJSONObject_ValidJSONDecodes(t *testing.T) {
	got, err := decodeMongoJSONObject(`{"name":"bolt","weight":5}`)
	if err != nil {
		t.Fatalf("decodeMongoJSONObject() failed: %v", err)
	}
	if got["name"] != "bolt" {
		t.Errorf("decodeMongoJSONObject() = %v, want name=bolt", got)
	}
}

func TestDecodeMongoJSONObject_InvalidJSONReturnsError(t *testing.T) {
	if _, err := decodeMongoJSONObject("{not json"); err == nil {
		t.Error("decodeMongoJSONObject(malformed) expected an error, got nil")
	}
}

func TestBuildMongoConnectionURI_IncludesHostPortUserAndDatabase(t *testing.T) {
	fields := ConnectionFormFields{
		Engine:   storage.EngineMongoDB,
		Host:     "localhost",
		Port:     27017,
		Username: "root",
		Password: "secret",
		Database: "mydb",
	}

	got := buildMongoConnectionURI(fields)

	want := "mongodb://root:secret@localhost:27017/mydb"
	if got != want {
		t.Errorf("buildMongoConnectionURI() = %q, want %q", got, want)
	}
}

func TestBuildMongoConnectionURI_OmitsDatabaseWhenBlank(t *testing.T) {
	fields := ConnectionFormFields{
		Engine: storage.EngineMongoDB,
		Host:   "localhost",
		Port:   27017,
	}

	got := buildMongoConnectionURI(fields)

	want := "mongodb://localhost:27017"
	if got != want {
		t.Errorf("buildMongoConnectionURI() = %q, want %q", got, want)
	}
}
