package main

import (
	"context"
	"fmt"
	"net/url"
	"time"

	dbengineredis "stackyard/internal/dbengine/redis"
	"stackyard/internal/storage"

	"github.com/google/uuid"
)

// buildRedisConnectionURI translates fields into a "redis://" connection
// string, the same way buildMongoConnectionURI translates ConnectionFormFields
// into a Mongo URI — Redis has no username concept (see
// internal/docker/redis.go's own doc comment on RedisConnectionString), so
// fields.Username is deliberately never consulted here, matching
// urlparse.go's own rejection of a username on a redis:// scheme. Built
// fresh from the current form-field state on every call, matching the
// Postgres/MySQL/Mongo builders' own "always rebuilt, never the originally
// pasted string" convention (spec.md §4.1's "editable afterward"
// requirement).
func buildRedisConnectionURI(fields ConnectionFormFields) string {
	var userInfo *url.Userinfo
	if fields.Password != "" {
		userInfo = url.UserPassword("", fields.Password)
	}

	u := &url.URL{
		Scheme: "redis",
		User:   userInfo,
		Host:   fmt.Sprintf("%s:%d", fields.Host, fields.Port),
	}
	if fields.Database != "" {
		u.Path = "/" + fields.Database
	}

	return u.String()
}

// OpenRedisConnection dials fields (which must name storage.EngineRedis) and
// keeps the resulting *dbengineredis.Engine alive server-side, returning a
// session ID the frontend passes to every other Redis bound method below,
// across as many separate IPC calls as it wants (tasks.md 6.1) — the
// Redis-side counterpart of OpenMongoConnection. Every call opens its own
// new, independent session, even for identical fields, matching
// OpenConnection's own one-session-per-tab convention (tasks.md 3.8).
func (a *App) OpenRedisConnection(fields ConnectionFormFields) (string, error) {
	if fields.Engine != storage.EngineRedis {
		return "", fmt.Errorf("open redis connection: expected engine %q, got %q", storage.EngineRedis, fields.Engine)
	}
	if err := validateConnectionFormFields(fields); err != nil {
		return "", fmt.Errorf("open redis connection: %w", err)
	}

	engine, err := dbengineredis.NewFromURL(buildRedisConnectionURI(fields))
	if err != nil {
		return "", fmt.Errorf("open redis connection: %w", err)
	}

	ctx, cancel := context.WithTimeout(a.ctx, openRedisConnectionTimeout)
	defer cancel()

	if err := engine.Connect(ctx); err != nil {
		return "", fmt.Errorf("open redis connection: %w", err)
	}
	if err := engine.Ping(ctx); err != nil {
		_ = engine.Close()
		return "", fmt.Errorf("open redis connection: %w", err)
	}

	id := uuid.NewString()
	a.putRedisSession(id, &redisSession{engine: engine})
	return id, nil
}

// CloseRedisSession closes the live Redis session behind sessionID and
// removes it from a's session map (tasks.md 6.1) — the Redis-side
// counterpart of CloseMongoSession. Closing an unknown or already-closed
// sessionID is an error, not a silent no-op, for the same
// bookkeeping-drift-should-be-detectable reason CloseConnectionSession's own
// doc comment gives.
func (a *App) CloseRedisSession(sessionID string) error {
	session, ok := a.deleteRedisSession(sessionID)
	if !ok {
		return fmt.Errorf("close redis session: no open redis session %q", sessionID)
	}
	if err := session.engine.Close(); err != nil {
		return fmt.Errorf("close redis session: %w", err)
	}
	return nil
}

// ScanKeysResult is ScanRedisKeys' return value, wrapping its two logical
// results (the page of matching keys and the cursor to continue scanning
// from) into a single struct — Wails v2.12.0's IPC dispatcher
// (internal/binding/boundMethod.go) only handles bound methods with one or
// two return values (a result plus an error), silently returning (nil, nil)
// for any bound method with three, so a three-return-value shape here would
// break at the JS/Go boundary despite compiling and testing fine in Go.
type ScanKeysResult struct {
	Keys       []string
	NextCursor uint64
}

// ScanRedisKeys is the key browser's (spec.md §4.5) pattern-based key
// filtering call: pattern is a Redis glob pattern (e.g. "session:*"), cursor
// is 0 to start a fresh scan and the previously-returned cursor otherwise,
// and count is a hint for how many keys to examine per call — the frontend
// loops, passing the returned ScanKeysResult.NextCursor back in, until a
// returned cursor of 0 signals the scan is complete (see
// dbengineredis.Engine.ScanKeys' own doc comment for why SCAN is used
// instead of KEYS).
func (a *App) ScanRedisKeys(sessionID string, pattern string, cursor uint64, count int64) (ScanKeysResult, error) {
	session, ok := a.getRedisSession(sessionID)
	if !ok {
		return ScanKeysResult{}, fmt.Errorf("scan redis keys: no open redis session %q", sessionID)
	}

	ctx, cancel := context.WithTimeout(a.ctx, redisOperationTimeout)
	defer cancel()

	keys, nextCursor, err := session.engine.ScanKeys(ctx, pattern, cursor, count)
	if err != nil {
		return ScanKeysResult{}, fmt.Errorf("scan redis keys: %w", err)
	}
	if keys == nil {
		keys = []string{}
	}
	return ScanKeysResult{Keys: keys, NextCursor: nextCursor}, nil
}

// GetRedisKeyType returns Redis's own TYPE string for key ("string", "hash",
// "list", "set", "zset", or "none") — the key browser uses this to decide
// which value-editing view to render for a selected key.
func (a *App) GetRedisKeyType(sessionID string, key string) (string, error) {
	session, ok := a.getRedisSession(sessionID)
	if !ok {
		return "", fmt.Errorf("get redis key type: no open redis session %q", sessionID)
	}

	ctx, cancel := context.WithTimeout(a.ctx, redisOperationTimeout)
	defer cancel()

	keyType, err := session.engine.KeyType(ctx, key)
	if err != nil {
		return "", fmt.Errorf("get redis key type: %w", err)
	}
	return keyType, nil
}

// GetRedisString returns the string value stored at key.
func (a *App) GetRedisString(sessionID string, key string) (string, error) {
	session, ok := a.getRedisSession(sessionID)
	if !ok {
		return "", fmt.Errorf("get redis string: no open redis session %q", sessionID)
	}

	ctx, cancel := context.WithTimeout(a.ctx, redisOperationTimeout)
	defer cancel()

	value, err := session.engine.GetString(ctx, key)
	if err != nil {
		return "", fmt.Errorf("get redis string: %w", err)
	}
	return value, nil
}

// SetRedisString sets key's string value — the string-value editing flow
// (spec.md §4.5).
func (a *App) SetRedisString(sessionID string, key string, value string) error {
	session, ok := a.getRedisSession(sessionID)
	if !ok {
		return fmt.Errorf("set redis string: no open redis session %q", sessionID)
	}

	ctx, cancel := context.WithTimeout(a.ctx, redisOperationTimeout)
	defer cancel()

	if err := session.engine.SetString(ctx, key, value); err != nil {
		return fmt.Errorf("set redis string: %w", err)
	}
	return nil
}

// GetRedisHash returns every field/value pair in the hash stored at key.
func (a *App) GetRedisHash(sessionID string, key string) (map[string]string, error) {
	session, ok := a.getRedisSession(sessionID)
	if !ok {
		return nil, fmt.Errorf("get redis hash: no open redis session %q", sessionID)
	}

	ctx, cancel := context.WithTimeout(a.ctx, redisOperationTimeout)
	defer cancel()

	fields, err := session.engine.GetHash(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("get redis hash: %w", err)
	}
	if fields == nil {
		fields = map[string]string{}
	}
	return fields, nil
}

// SetRedisHash bulk-replaces every field in fields on the hash stored at key
// — the hash-value editing flow (spec.md §4.5), see
// dbengineredis.Engine.SetHash's own doc comment for the bulk-edit scope
// this intentionally takes.
func (a *App) SetRedisHash(sessionID string, key string, fields map[string]string) error {
	session, ok := a.getRedisSession(sessionID)
	if !ok {
		return fmt.Errorf("set redis hash: no open redis session %q", sessionID)
	}

	ctx, cancel := context.WithTimeout(a.ctx, redisOperationTimeout)
	defer cancel()

	if err := session.engine.SetHash(ctx, key, fields); err != nil {
		return fmt.Errorf("set redis hash: %w", err)
	}
	return nil
}

// GetRedisList returns the elements of the list stored at key between start
// and stop — real, paginated windowing rather than a full-list fetch (see
// dbengineredis.Engine.GetList's own doc comment).
func (a *App) GetRedisList(sessionID string, key string, start int64, stop int64) ([]string, error) {
	session, ok := a.getRedisSession(sessionID)
	if !ok {
		return nil, fmt.Errorf("get redis list: no open redis session %q", sessionID)
	}

	ctx, cancel := context.WithTimeout(a.ctx, redisOperationTimeout)
	defer cancel()

	values, err := session.engine.GetList(ctx, key, start, stop)
	if err != nil {
		return nil, fmt.Errorf("get redis list: %w", err)
	}
	if values == nil {
		values = []string{}
	}
	return values, nil
}

// PushRedisList appends values to the end of the list stored at key.
// values is a plain slice (not variadic) since Wails-exposed methods don't
// bridge variadic parameters cleanly.
func (a *App) PushRedisList(sessionID string, key string, values []string) error {
	session, ok := a.getRedisSession(sessionID)
	if !ok {
		return fmt.Errorf("push redis list: no open redis session %q", sessionID)
	}

	ctx, cancel := context.WithTimeout(a.ctx, redisOperationTimeout)
	defer cancel()

	if err := session.engine.PushList(ctx, key, values...); err != nil {
		return fmt.Errorf("push redis list: %w", err)
	}
	return nil
}

// SetRedisListElement overwrites the element at index in the list stored at
// key — the single-element in-place edit the key browser's list-value
// editor uses (spec.md §4.5).
func (a *App) SetRedisListElement(sessionID string, key string, index int64, value string) error {
	session, ok := a.getRedisSession(sessionID)
	if !ok {
		return fmt.Errorf("set redis list element: no open redis session %q", sessionID)
	}

	ctx, cancel := context.WithTimeout(a.ctx, redisOperationTimeout)
	defer cancel()

	if err := session.engine.SetListElement(ctx, key, index, value); err != nil {
		return fmt.Errorf("set redis list element: %w", err)
	}
	return nil
}

// RedisSetPage is GetRedisSet's return value, wrapping its two logical
// results (the page of set members and the cursor to continue scanning
// from) into a single struct for the same Wails IPC three-return-value
// limitation ScanKeysResult's own doc comment describes.
type RedisSetPage struct {
	Members    []string
	NextCursor uint64
}

// GetRedisSet returns up to count members of the set stored at key,
// continuing from cursor — the same cursor-based pagination shape
// ScanRedisKeys uses (see dbengineredis.Engine.GetSet's own doc comment for
// why SSCAN, not SMEMBERS).
func (a *App) GetRedisSet(sessionID string, key string, cursor uint64, count int64) (RedisSetPage, error) {
	session, ok := a.getRedisSession(sessionID)
	if !ok {
		return RedisSetPage{}, fmt.Errorf("get redis set: no open redis session %q", sessionID)
	}

	ctx, cancel := context.WithTimeout(a.ctx, redisOperationTimeout)
	defer cancel()

	members, nextCursor, err := session.engine.GetSet(ctx, key, cursor, count)
	if err != nil {
		return RedisSetPage{}, fmt.Errorf("get redis set: %w", err)
	}
	if members == nil {
		members = []string{}
	}
	return RedisSetPage{Members: members, NextCursor: nextCursor}, nil
}

// AddRedisSetMembers adds members to the set stored at key. members is a
// plain slice (not variadic) since Wails-exposed methods don't bridge
// variadic parameters cleanly.
func (a *App) AddRedisSetMembers(sessionID string, key string, members []string) error {
	session, ok := a.getRedisSession(sessionID)
	if !ok {
		return fmt.Errorf("add redis set members: no open redis session %q", sessionID)
	}

	ctx, cancel := context.WithTimeout(a.ctx, redisOperationTimeout)
	defer cancel()

	if err := session.engine.AddToSet(ctx, key, members...); err != nil {
		return fmt.Errorf("add redis set members: %w", err)
	}
	return nil
}

// RemoveRedisSetMembers removes members from the set stored at key. members
// is a plain slice (not variadic) since Wails-exposed methods don't bridge
// variadic parameters cleanly.
func (a *App) RemoveRedisSetMembers(sessionID string, key string, members []string) error {
	session, ok := a.getRedisSession(sessionID)
	if !ok {
		return fmt.Errorf("remove redis set members: no open redis session %q", sessionID)
	}

	ctx, cancel := context.WithTimeout(a.ctx, redisOperationTimeout)
	defer cancel()

	if err := session.engine.RemoveFromSet(ctx, key, members...); err != nil {
		return fmt.Errorf("remove redis set members: %w", err)
	}
	return nil
}

// GetRedisSortedSet returns the member/score pairs of the sorted set stored
// at key between start and stop, paginated the same way GetRedisList
// paginates a list.
func (a *App) GetRedisSortedSet(sessionID string, key string, start int64, stop int64) ([]dbengineredis.SortedSetMember, error) {
	session, ok := a.getRedisSession(sessionID)
	if !ok {
		return nil, fmt.Errorf("get redis sorted set: no open redis session %q", sessionID)
	}

	ctx, cancel := context.WithTimeout(a.ctx, redisOperationTimeout)
	defer cancel()

	members, err := session.engine.GetSortedSet(ctx, key, start, stop)
	if err != nil {
		return nil, fmt.Errorf("get redis sorted set: %w", err)
	}
	if members == nil {
		members = []dbengineredis.SortedSetMember{}
	}
	return members, nil
}

// AddRedisSortedSetMembers adds members to the sorted set stored at key.
// members is a plain slice (not variadic) since Wails-exposed methods don't
// bridge variadic parameters cleanly.
func (a *App) AddRedisSortedSetMembers(sessionID string, key string, members []dbengineredis.SortedSetMember) error {
	session, ok := a.getRedisSession(sessionID)
	if !ok {
		return fmt.Errorf("add redis sorted set members: no open redis session %q", sessionID)
	}

	ctx, cancel := context.WithTimeout(a.ctx, redisOperationTimeout)
	defer cancel()

	if err := session.engine.AddToSortedSet(ctx, key, members...); err != nil {
		return fmt.Errorf("add redis sorted set members: %w", err)
	}
	return nil
}

// RemoveRedisSortedSetMembers removes members from the sorted set stored at
// key. members is a plain slice (not variadic) since Wails-exposed methods
// don't bridge variadic parameters cleanly.
func (a *App) RemoveRedisSortedSetMembers(sessionID string, key string, members []string) error {
	session, ok := a.getRedisSession(sessionID)
	if !ok {
		return fmt.Errorf("remove redis sorted set members: no open redis session %q", sessionID)
	}

	ctx, cancel := context.WithTimeout(a.ctx, redisOperationTimeout)
	defer cancel()

	if err := session.engine.RemoveFromSortedSet(ctx, key, members...); err != nil {
		return fmt.Errorf("remove redis sorted set members: %w", err)
	}
	return nil
}

// GetRedisTTL returns the remaining time to live of key. A never-expiring
// key returns (-1, nil); a nonexistent key returns a
// dbengineredis.ErrKeyNotFound-wrapped error (see
// dbengineredis.Engine.TTL's own doc comment for the full sentinel
// contract).
func (a *App) GetRedisTTL(sessionID string, key string) (time.Duration, error) {
	session, ok := a.getRedisSession(sessionID)
	if !ok {
		return 0, fmt.Errorf("get redis ttl: no open redis session %q", sessionID)
	}

	ctx, cancel := context.WithTimeout(a.ctx, redisOperationTimeout)
	defer cancel()

	ttl, err := session.engine.TTL(ctx, key)
	if err != nil {
		return 0, fmt.Errorf("get redis ttl: %w", err)
	}
	return ttl, nil
}

// SetRedisTTL sets key's time to live, expressed as ttlSeconds — a plain
// int64 rather than time.Duration directly, since Wails' TS-binding
// generator represents Go's time.Duration as its underlying int64
// nanosecond count, which would force the frontend to multiply by
// 1_000_000_000 itself; ttlSeconds keeps the bound-method boundary in the
// unit the frontend's TTL editor (spec.md §4.5) actually works in.
func (a *App) SetRedisTTL(sessionID string, key string, ttlSeconds int64) error {
	session, ok := a.getRedisSession(sessionID)
	if !ok {
		return fmt.Errorf("set redis ttl: no open redis session %q", sessionID)
	}

	ctx, cancel := context.WithTimeout(a.ctx, redisOperationTimeout)
	defer cancel()

	if err := session.engine.SetTTL(ctx, key, time.Duration(ttlSeconds)*time.Second); err != nil {
		return fmt.Errorf("set redis ttl: %w", err)
	}
	return nil
}

// PersistRedisKey removes key's existing TTL, making it never expire.
func (a *App) PersistRedisKey(sessionID string, key string) error {
	session, ok := a.getRedisSession(sessionID)
	if !ok {
		return fmt.Errorf("persist redis key: no open redis session %q", sessionID)
	}

	ctx, cancel := context.WithTimeout(a.ctx, redisOperationTimeout)
	defer cancel()

	if err := session.engine.PersistKey(ctx, key); err != nil {
		return fmt.Errorf("persist redis key: %w", err)
	}
	return nil
}

// RenameRedisKey renames oldKey to newKey, refusing to silently overwrite an
// existing newKey (see dbengineredis.Engine.RenameKey's own doc comment for
// the guard this delegates to) — the key rename flow (spec.md §4.5).
func (a *App) RenameRedisKey(sessionID string, oldKey string, newKey string) error {
	session, ok := a.getRedisSession(sessionID)
	if !ok {
		return fmt.Errorf("rename redis key: no open redis session %q", sessionID)
	}

	ctx, cancel := context.WithTimeout(a.ctx, redisOperationTimeout)
	defer cancel()

	if err := session.engine.RenameKey(ctx, oldKey, newKey); err != nil {
		return fmt.Errorf("rename redis key: %w", err)
	}
	return nil
}

// DeleteRedisKeys deletes every key in keys — supporting the multi-key
// delete the key browser's confirmation dialog requires (spec.md §4.5).
func (a *App) DeleteRedisKeys(sessionID string, keys []string) error {
	session, ok := a.getRedisSession(sessionID)
	if !ok {
		return fmt.Errorf("delete redis keys: no open redis session %q", sessionID)
	}

	ctx, cancel := context.WithTimeout(a.ctx, redisOperationTimeout)
	defer cancel()

	if err := session.engine.DeleteKeys(ctx, keys...); err != nil {
		return fmt.Errorf("delete redis keys: %w", err)
	}
	return nil
}
