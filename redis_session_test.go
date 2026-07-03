package main

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	dbengineredis "stackyard/internal/dbengine/redis"
	"stackyard/internal/storage"
)

// fakeRedisEngine is a redisEngine test double used to exercise
// OpenRedisConnection/CloseRedisSession/ScanRedisKeys/GetRedis*/SetRedis*'s
// session-map bookkeeping without a live Redis connection, mirroring
// mongo_session_test.go's fakeMongoEngine pattern.
type fakeRedisEngine struct {
	mu     sync.Mutex
	closed bool

	closeErr error

	scanKeysFunc            func(ctx context.Context, pattern string, cursor uint64, count int64) ([]string, uint64, error)
	keyTypeFunc             func(ctx context.Context, key string) (string, error)
	getStringFunc           func(ctx context.Context, key string) (string, error)
	setStringFunc           func(ctx context.Context, key, value string) error
	getHashFunc             func(ctx context.Context, key string) (map[string]string, error)
	setHashFunc             func(ctx context.Context, key string, fields map[string]string) error
	getListFunc             func(ctx context.Context, key string, start, stop int64) ([]string, error)
	pushListFunc            func(ctx context.Context, key string, values ...string) error
	setListElementFunc      func(ctx context.Context, key string, index int64, value string) error
	getSetFunc              func(ctx context.Context, key string, cursor uint64, count int64) ([]string, uint64, error)
	addToSetFunc            func(ctx context.Context, key string, members ...string) error
	removeFromSetFunc       func(ctx context.Context, key string, members ...string) error
	getSortedSetFunc        func(ctx context.Context, key string, start, stop int64) ([]dbengineredis.SortedSetMember, error)
	addToSortedSetFunc      func(ctx context.Context, key string, members ...dbengineredis.SortedSetMember) error
	removeFromSortedSetFunc func(ctx context.Context, key string, members ...string) error
	ttlFunc                 func(ctx context.Context, key string) (time.Duration, error)
	setTTLFunc              func(ctx context.Context, key string, ttl time.Duration) error
	persistKeyFunc          func(ctx context.Context, key string) error
	renameKeyFunc           func(ctx context.Context, oldKey, newKey string) error
	deleteKeysFunc          func(ctx context.Context, keys ...string) error
}

func (f *fakeRedisEngine) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed = true
	return f.closeErr
}

func (f *fakeRedisEngine) isClosed() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.closed
}

func (f *fakeRedisEngine) ScanKeys(ctx context.Context, pattern string, cursor uint64, count int64) ([]string, uint64, error) {
	if f.scanKeysFunc != nil {
		return f.scanKeysFunc(ctx, pattern, cursor, count)
	}
	return nil, 0, nil
}

func (f *fakeRedisEngine) KeyType(ctx context.Context, key string) (string, error) {
	if f.keyTypeFunc != nil {
		return f.keyTypeFunc(ctx, key)
	}
	return "", nil
}

func (f *fakeRedisEngine) GetString(ctx context.Context, key string) (string, error) {
	if f.getStringFunc != nil {
		return f.getStringFunc(ctx, key)
	}
	return "", nil
}

func (f *fakeRedisEngine) SetString(ctx context.Context, key, value string) error {
	if f.setStringFunc != nil {
		return f.setStringFunc(ctx, key, value)
	}
	return nil
}

func (f *fakeRedisEngine) GetHash(ctx context.Context, key string) (map[string]string, error) {
	if f.getHashFunc != nil {
		return f.getHashFunc(ctx, key)
	}
	return nil, nil
}

func (f *fakeRedisEngine) SetHash(ctx context.Context, key string, fields map[string]string) error {
	if f.setHashFunc != nil {
		return f.setHashFunc(ctx, key, fields)
	}
	return nil
}

func (f *fakeRedisEngine) GetList(ctx context.Context, key string, start, stop int64) ([]string, error) {
	if f.getListFunc != nil {
		return f.getListFunc(ctx, key, start, stop)
	}
	return nil, nil
}

func (f *fakeRedisEngine) PushList(ctx context.Context, key string, values ...string) error {
	if f.pushListFunc != nil {
		return f.pushListFunc(ctx, key, values...)
	}
	return nil
}

func (f *fakeRedisEngine) SetListElement(ctx context.Context, key string, index int64, value string) error {
	if f.setListElementFunc != nil {
		return f.setListElementFunc(ctx, key, index, value)
	}
	return nil
}

func (f *fakeRedisEngine) GetSet(ctx context.Context, key string, cursor uint64, count int64) ([]string, uint64, error) {
	if f.getSetFunc != nil {
		return f.getSetFunc(ctx, key, cursor, count)
	}
	return nil, 0, nil
}

func (f *fakeRedisEngine) AddToSet(ctx context.Context, key string, members ...string) error {
	if f.addToSetFunc != nil {
		return f.addToSetFunc(ctx, key, members...)
	}
	return nil
}

func (f *fakeRedisEngine) RemoveFromSet(ctx context.Context, key string, members ...string) error {
	if f.removeFromSetFunc != nil {
		return f.removeFromSetFunc(ctx, key, members...)
	}
	return nil
}

func (f *fakeRedisEngine) GetSortedSet(ctx context.Context, key string, start, stop int64) ([]dbengineredis.SortedSetMember, error) {
	if f.getSortedSetFunc != nil {
		return f.getSortedSetFunc(ctx, key, start, stop)
	}
	return nil, nil
}

func (f *fakeRedisEngine) AddToSortedSet(ctx context.Context, key string, members ...dbengineredis.SortedSetMember) error {
	if f.addToSortedSetFunc != nil {
		return f.addToSortedSetFunc(ctx, key, members...)
	}
	return nil
}

func (f *fakeRedisEngine) RemoveFromSortedSet(ctx context.Context, key string, members ...string) error {
	if f.removeFromSortedSetFunc != nil {
		return f.removeFromSortedSetFunc(ctx, key, members...)
	}
	return nil
}

func (f *fakeRedisEngine) TTL(ctx context.Context, key string) (time.Duration, error) {
	if f.ttlFunc != nil {
		return f.ttlFunc(ctx, key)
	}
	return 0, nil
}

func (f *fakeRedisEngine) SetTTL(ctx context.Context, key string, ttl time.Duration) error {
	if f.setTTLFunc != nil {
		return f.setTTLFunc(ctx, key, ttl)
	}
	return nil
}

func (f *fakeRedisEngine) PersistKey(ctx context.Context, key string) error {
	if f.persistKeyFunc != nil {
		return f.persistKeyFunc(ctx, key)
	}
	return nil
}

func (f *fakeRedisEngine) RenameKey(ctx context.Context, oldKey, newKey string) error {
	if f.renameKeyFunc != nil {
		return f.renameKeyFunc(ctx, oldKey, newKey)
	}
	return nil
}

func (f *fakeRedisEngine) DeleteKeys(ctx context.Context, keys ...string) error {
	if f.deleteKeysFunc != nil {
		return f.deleteKeysFunc(ctx, keys...)
	}
	return nil
}

func TestApp_RedisSessionBookkeeping_PutGetDelete(t *testing.T) {
	a := &App{}
	engine := &fakeRedisEngine{}

	a.putRedisSession("session-1", &redisSession{engine: engine})

	got, ok := a.getRedisSession("session-1")
	if !ok || got.engine != engine {
		t.Fatalf("getRedisSession(\"session-1\") = (%v, %v), want the stored session", got, ok)
	}

	deleted, ok := a.deleteRedisSession("session-1")
	if !ok || deleted.engine != engine {
		t.Fatalf("deleteRedisSession(\"session-1\") = (%v, %v), want the stored session", deleted, ok)
	}

	if _, ok := a.getRedisSession("session-1"); ok {
		t.Error("getRedisSession(\"session-1\") after delete: expected not found, got a session")
	}
}

func TestApp_OpenRedisConnection_RejectsNonRedisEngine(t *testing.T) {
	a := &App{ctx: context.Background()}

	fields := ConnectionFormFields{Engine: storage.EnginePostgres, Host: "localhost", Port: 5432}
	if _, err := a.OpenRedisConnection(fields); err == nil {
		t.Error("OpenRedisConnection() with a non-Redis engine: expected an error, got nil")
	} else if !strings.Contains(err.Error(), "expected engine") {
		t.Errorf("OpenRedisConnection() error = %q, want it to name the engine mismatch", err.Error())
	}
}

func TestApp_OpenRedisConnection_RejectsMissingHost(t *testing.T) {
	a := &App{ctx: context.Background()}

	fields := ConnectionFormFields{Engine: storage.EngineRedis, Port: 6379}
	if _, err := a.OpenRedisConnection(fields); err == nil {
		t.Error("OpenRedisConnection() with a blank host: expected an error, got nil")
	}
}

func TestApp_CloseRedisSession_NotFoundReturnsError(t *testing.T) {
	a := &App{ctx: context.Background()}

	if err := a.CloseRedisSession("does-not-exist"); err == nil {
		t.Error("CloseRedisSession() with an unknown session ID: expected an error, got nil")
	} else if !strings.Contains(err.Error(), "does-not-exist") {
		t.Errorf("CloseRedisSession() error = %q, want it to name the missing session ID", err.Error())
	}
}

func TestApp_CloseRedisSession_ClosesEngineAndRemovesSession(t *testing.T) {
	a := &App{ctx: context.Background()}
	engine := &fakeRedisEngine{}
	a.putRedisSession("session-1", &redisSession{engine: engine})

	if err := a.CloseRedisSession("session-1"); err != nil {
		t.Fatalf("CloseRedisSession() failed: %v", err)
	}
	if !engine.isClosed() {
		t.Error("CloseRedisSession() did not close the underlying engine")
	}
	if _, ok := a.getRedisSession("session-1"); ok {
		t.Error("CloseRedisSession() left the session in the map")
	}
}

func TestApp_CloseRedisSession_PropagatesEngineCloseError(t *testing.T) {
	a := &App{ctx: context.Background()}
	wantErr := errors.New("boom")
	engine := &fakeRedisEngine{closeErr: wantErr}
	a.putRedisSession("session-1", &redisSession{engine: engine})

	if err := a.CloseRedisSession("session-1"); err == nil || !strings.Contains(err.Error(), "boom") {
		t.Errorf("CloseRedisSession() error = %v, want it to wrap the engine's own close error", err)
	}
}

func TestApp_CloseAllRedisSessions_ClosesEveryEngine(t *testing.T) {
	a := &App{ctx: context.Background()}
	engineA := &fakeRedisEngine{}
	engineB := &fakeRedisEngine{}
	a.putRedisSession("session-a", &redisSession{engine: engineA})
	a.putRedisSession("session-b", &redisSession{engine: engineB})

	a.closeAllRedisSessions()

	if !engineA.isClosed() || !engineB.isClosed() {
		t.Error("closeAllRedisSessions() did not close every registered engine")
	}
	if _, ok := a.getRedisSession("session-a"); ok {
		t.Error("closeAllRedisSessions() left session-a in the map")
	}
}

func TestApp_ScanRedisKeys_NotFoundSessionReturnsError(t *testing.T) {
	a := &App{ctx: context.Background()}

	if _, err := a.ScanRedisKeys("does-not-exist", "session:*", 0, 10); err == nil {
		t.Error("ScanRedisKeys() with an unknown session ID: expected an error, got nil")
	} else if !strings.Contains(err.Error(), "does-not-exist") {
		t.Errorf("ScanRedisKeys() error = %q, want it to name the missing session ID", err.Error())
	}
}

func TestApp_ScanRedisKeys_PassesPatternCursorAndCountThrough(t *testing.T) {
	a := &App{ctx: context.Background()}
	var gotPattern string
	var gotCursor uint64
	var gotCount int64
	engine := &fakeRedisEngine{
		scanKeysFunc: func(ctx context.Context, pattern string, cursor uint64, count int64) ([]string, uint64, error) {
			gotPattern = pattern
			gotCursor = cursor
			gotCount = count
			return []string{"session:1"}, 42, nil
		},
	}
	a.putRedisSession("session-1", &redisSession{engine: engine})

	result, err := a.ScanRedisKeys("session-1", "session:*", 7, 25)
	if err != nil {
		t.Fatalf("ScanRedisKeys() failed: %v", err)
	}
	if gotPattern != "session:*" || gotCursor != 7 || gotCount != 25 {
		t.Errorf("engine.ScanKeys() received pattern=%q cursor=%d count=%d, want pattern=session:* cursor=7 count=25", gotPattern, gotCursor, gotCount)
	}
	if len(result.Keys) != 1 || result.Keys[0] != "session:1" {
		t.Errorf("ScanRedisKeys() keys = %v, want [session:1]", result.Keys)
	}
	if result.NextCursor != 42 {
		t.Errorf("ScanRedisKeys() nextCursor = %d, want 42", result.NextCursor)
	}
}

func TestApp_GetRedisTTL_NotFoundSessionReturnsError(t *testing.T) {
	a := &App{ctx: context.Background()}

	if _, err := a.GetRedisTTL("does-not-exist", "some-key"); err == nil {
		t.Error("GetRedisTTL() with an unknown session ID: expected an error, got nil")
	} else if !strings.Contains(err.Error(), "does-not-exist") {
		t.Errorf("GetRedisTTL() error = %q, want it to name the missing session ID", err.Error())
	}
}

func TestApp_GetRedisTTL_PropagatesEngineResult(t *testing.T) {
	a := &App{ctx: context.Background()}
	engine := &fakeRedisEngine{
		ttlFunc: func(ctx context.Context, key string) (time.Duration, error) {
			return -1, nil
		},
	}
	a.putRedisSession("session-1", &redisSession{engine: engine})

	got, err := a.GetRedisTTL("session-1", "some-key")
	if err != nil {
		t.Fatalf("GetRedisTTL() failed: %v", err)
	}
	if got != -1 {
		t.Errorf("GetRedisTTL() = %v, want -1", got)
	}
}

func TestApp_SetRedisTTL_ConvertsSecondsToDuration(t *testing.T) {
	a := &App{ctx: context.Background()}
	var gotTTL time.Duration
	engine := &fakeRedisEngine{
		setTTLFunc: func(ctx context.Context, key string, ttl time.Duration) error {
			gotTTL = ttl
			return nil
		},
	}
	a.putRedisSession("session-1", &redisSession{engine: engine})

	if err := a.SetRedisTTL("session-1", "some-key", 30); err != nil {
		t.Fatalf("SetRedisTTL() failed: %v", err)
	}
	if gotTTL != 30*time.Second {
		t.Errorf("engine.SetTTL() received ttl=%v, want 30s", gotTTL)
	}
}

func TestApp_RenameRedisKey_NotFoundSessionReturnsError(t *testing.T) {
	a := &App{ctx: context.Background()}

	if err := a.RenameRedisKey("does-not-exist", "old", "new"); err == nil {
		t.Error("RenameRedisKey() with an unknown session ID: expected an error, got nil")
	} else if !strings.Contains(err.Error(), "does-not-exist") {
		t.Errorf("RenameRedisKey() error = %q, want it to name the missing session ID", err.Error())
	}
}

func TestApp_RenameRedisKey_PropagatesKeyExistsError(t *testing.T) {
	a := &App{ctx: context.Background()}
	engine := &fakeRedisEngine{
		renameKeyFunc: func(ctx context.Context, oldKey, newKey string) error {
			return dbengineredis.ErrKeyExists
		},
	}
	a.putRedisSession("session-1", &redisSession{engine: engine})

	if err := a.RenameRedisKey("session-1", "old", "new"); !errors.Is(err, dbengineredis.ErrKeyExists) {
		t.Errorf("RenameRedisKey() error = %v, want it to wrap dbengineredis.ErrKeyExists", err)
	}
}

func TestApp_DeleteRedisKeys_NotFoundSessionReturnsError(t *testing.T) {
	a := &App{ctx: context.Background()}

	if err := a.DeleteRedisKeys("does-not-exist", []string{"a", "b"}); err == nil {
		t.Error("DeleteRedisKeys() with an unknown session ID: expected an error, got nil")
	} else if !strings.Contains(err.Error(), "does-not-exist") {
		t.Errorf("DeleteRedisKeys() error = %q, want it to name the missing session ID", err.Error())
	}
}

func TestApp_DeleteRedisKeys_SupportsMultipleKeys(t *testing.T) {
	a := &App{ctx: context.Background()}
	var gotKeys []string
	engine := &fakeRedisEngine{
		deleteKeysFunc: func(ctx context.Context, keys ...string) error {
			gotKeys = keys
			return nil
		},
	}
	a.putRedisSession("session-1", &redisSession{engine: engine})

	if err := a.DeleteRedisKeys("session-1", []string{"key1", "key2"}); err != nil {
		t.Fatalf("DeleteRedisKeys() failed: %v", err)
	}
	if len(gotKeys) != 2 || gotKeys[0] != "key1" || gotKeys[1] != "key2" {
		t.Errorf("engine.DeleteKeys() received keys %v, want [key1 key2]", gotKeys)
	}
}

func TestApp_GetRedisHash_ReturnsEmptyMapNotNil(t *testing.T) {
	a := &App{ctx: context.Background()}
	engine := &fakeRedisEngine{
		getHashFunc: func(ctx context.Context, key string) (map[string]string, error) {
			return nil, nil
		},
	}
	a.putRedisSession("session-1", &redisSession{engine: engine})

	got, err := a.GetRedisHash("session-1", "some-key")
	if err != nil {
		t.Fatalf("GetRedisHash() failed: %v", err)
	}
	if got == nil {
		t.Error("GetRedisHash() returned nil, want a non-nil empty map")
	}
}

func TestApp_GetRedisSortedSet_PassesStartStopThrough(t *testing.T) {
	a := &App{ctx: context.Background()}
	var gotStart, gotStop int64
	engine := &fakeRedisEngine{
		getSortedSetFunc: func(ctx context.Context, key string, start, stop int64) ([]dbengineredis.SortedSetMember, error) {
			gotStart = start
			gotStop = stop
			return []dbengineredis.SortedSetMember{{Member: "gold", Score: 30}}, nil
		},
	}
	a.putRedisSession("session-1", &redisSession{engine: engine})

	got, err := a.GetRedisSortedSet("session-1", "leaderboard", 0, -1)
	if err != nil {
		t.Fatalf("GetRedisSortedSet() failed: %v", err)
	}
	if gotStart != 0 || gotStop != -1 {
		t.Errorf("engine.GetSortedSet() received start=%d stop=%d, want start=0 stop=-1", gotStart, gotStop)
	}
	if len(got) != 1 || got[0].Member != "gold" {
		t.Errorf("GetRedisSortedSet() = %v, want [{gold 30}]", got)
	}
}

func TestApp_PushRedisList_PassesValuesThrough(t *testing.T) {
	a := &App{ctx: context.Background()}
	var gotValues []string
	engine := &fakeRedisEngine{
		pushListFunc: func(ctx context.Context, key string, values ...string) error {
			gotValues = values
			return nil
		},
	}
	a.putRedisSession("session-1", &redisSession{engine: engine})

	if err := a.PushRedisList("session-1", "queue:jobs", []string{"job1", "job2"}); err != nil {
		t.Fatalf("PushRedisList() failed: %v", err)
	}
	if len(gotValues) != 2 || gotValues[0] != "job1" || gotValues[1] != "job2" {
		t.Errorf("engine.PushList() received values %v, want [job1 job2]", gotValues)
	}
}
