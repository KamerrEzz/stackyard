// Package redis is Redis's own client for this project's DB Client module
// (spec.md §4.5, plan.md §3). Redis is key-value-oriented with typed
// values (string/hash/list/set/sorted-set), not the row/column shape
// dbengine.Engine's Query(ctx, query string) contract assumes, and not the
// document shape mongo.Engine covers either — "get the hash at this key" or
// "scan the keyspace for a pattern" has no sensible mapping onto either of
// those interfaces. Engine therefore does NOT implement dbengine.Engine; it
// is a deliberately separate type with its own key-value-oriented surface
// (key-space scanning plus one method pair per Redis value type), mirroring
// mongo.Engine's own precedent for the same reason (see mongo.go's package
// doc comment).
//
// A *Engine value is constructed already bound to one address/URI via
// NewFromURL; Connect performs the actual dial and an initial ping,
// matching the same lifecycle shape postgres.Engine/mysql.Engine/
// mongo.Engine all use, for consistency across engines, even though this
// type implements no shared interface with them.
package redis

import (
	"context"
	"errors"
	"fmt"
	"time"

	redisdriver "github.com/redis/go-redis/v9"
)

// ErrKeyNotFound is returned when an operation targets a key that does not
// exist in Redis (e.g. TTL on a missing key).
var ErrKeyNotFound = errors.New("redis: key not found")

// ErrKeyExists is returned by RenameKey when newKey already exists — RENAME
// itself would silently overwrite it, and this layer deliberately refuses
// that instead (see RenameKey's own doc comment).
var ErrKeyExists = errors.New("redis: key already exists")

// Engine is a Redis client bound to one address/URI.
type Engine struct {
	options *redisdriver.Options
	client  *redisdriver.Client
}

// NewFromURL returns an Engine bound to uri, a standard "redis://" (or
// "rediss://") connection string, parsed via the driver's own
// redisdriver.ParseURL. It does not dial; call Connect to establish the
// client.
func NewFromURL(uri string) (*Engine, error) {
	options, err := redisdriver.ParseURL(uri)
	if err != nil {
		return nil, fmt.Errorf("redis: parse url: %w", err)
	}
	return &Engine{options: options}, nil
}

// Connect establishes the underlying driver client and confirms it is
// reachable with an initial ping. Calling Connect again after a prior
// successful call closes the existing client before replacing it.
func (e *Engine) Connect(ctx context.Context) error {
	client := redisdriver.NewClient(e.options)
	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return fmt.Errorf("redis: connect: initial ping: %w", err)
	}
	if e.client != nil {
		_ = e.client.Close()
	}
	e.client = client
	return nil
}

// Ping confirms the connection is still reachable.
func (e *Engine) Ping(ctx context.Context) error {
	if e.client == nil {
		return ErrNotConnected
	}
	if err := e.client.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("redis: ping: %w", err)
	}
	return nil
}

// ErrNotConnected is returned by every method below except Connect when
// called before a successful Connect.
var ErrNotConnected = errors.New("redis: not connected")

// Close closes the underlying client. It is safe to call more than once.
func (e *Engine) Close() error {
	if e.client == nil {
		return nil
	}
	err := e.client.Close()
	e.client = nil
	if err != nil {
		return fmt.Errorf("redis: close: %w", err)
	}
	return nil
}

// ScanKeys returns up to count keys matching pattern (e.g. "session:*"),
// continuing from cursor (0 to start a fresh scan), plus the cursor to pass
// on the next call — a nextCursor of 0 means the scan has completed.
//
// This uses SCAN, never KEYS: KEYS walks the entire keyspace in one
// blocking call, which is unsafe on a production-sized database (it blocks
// every other client for the duration). SCAN is cursor-based and
// non-blocking, incrementally walking the keyspace across as many calls as
// the caller wants to make, at the cost of not guaranteeing every matching
// key is returned in one round trip — callers must loop until nextCursor is
// 0 to see every match, which the key browser's pattern filter (spec.md
// §4.5) does.
func (e *Engine) ScanKeys(ctx context.Context, pattern string, cursor uint64, count int64) ([]string, uint64, error) {
	if e.client == nil {
		return nil, 0, ErrNotConnected
	}
	keys, nextCursor, err := e.client.Scan(ctx, cursor, pattern, count).Result()
	if err != nil {
		return nil, 0, fmt.Errorf("redis: scan keys: %w", err)
	}
	return keys, nextCursor, nil
}

// KeyType returns Redis's own TYPE string for key: "string", "hash",
// "list", "set", "zset", or "none" if key does not exist.
func (e *Engine) KeyType(ctx context.Context, key string) (string, error) {
	if e.client == nil {
		return "", ErrNotConnected
	}
	keyType, err := e.client.Type(ctx, key).Result()
	if err != nil {
		return "", fmt.Errorf("redis: key type: %w", err)
	}
	return keyType, nil
}

// GetString returns the string value stored at key.
func (e *Engine) GetString(ctx context.Context, key string) (string, error) {
	if e.client == nil {
		return "", ErrNotConnected
	}
	value, err := e.client.Get(ctx, key).Result()
	if err != nil {
		if errors.Is(err, redisdriver.Nil) {
			return "", ErrKeyNotFound
		}
		return "", fmt.Errorf("redis: get string: %w", err)
	}
	return value, nil
}

// SetString sets key to value, the string-value editing primitive (spec.md
// §4.5). It does not touch any existing TTL on key — use SetTTL separately
// if the edit should also change expiry.
func (e *Engine) SetString(ctx context.Context, key, value string) error {
	if e.client == nil {
		return ErrNotConnected
	}
	if err := e.client.Set(ctx, key, value, 0).Err(); err != nil {
		return fmt.Errorf("redis: set string: %w", err)
	}
	return nil
}

// GetHash returns every field/value pair in the hash stored at key.
func (e *Engine) GetHash(ctx context.Context, key string) (map[string]string, error) {
	if e.client == nil {
		return nil, ErrNotConnected
	}
	fields, err := e.client.HGetAll(ctx, key).Result()
	if err != nil {
		return nil, fmt.Errorf("redis: get hash: %w", err)
	}
	return fields, nil
}

// SetHash bulk-replaces every field in fields on the hash stored at key via
// a single HSET call. This is an intentional bulk-edit simplification, the
// same spirit as Mongo's whole-document JSON editing choice (see
// mongo.Engine.UpdateDocument's doc comment): the key browser edits a hash
// as one field/value table and commits the whole thing at once, rather than
// this layer exposing a separate per-field HSET/HDEL pair the frontend would
// have to diff against the previous state itself. Note that HSET only adds
// or overwrites the given fields — it does not remove fields present in the
// existing hash but absent from fields; a caller wanting an exact replace
// must delete the key first.
func (e *Engine) SetHash(ctx context.Context, key string, fields map[string]string) error {
	if e.client == nil {
		return ErrNotConnected
	}
	if len(fields) == 0 {
		return nil
	}
	values := make(map[string]interface{}, len(fields))
	for field, value := range fields {
		values[field] = value
	}
	if err := e.client.HSet(ctx, key, values).Err(); err != nil {
		return fmt.Errorf("redis: set hash: %w", err)
	}
	return nil
}

// GetList returns the elements of the list stored at key between start and
// stop (LRANGE's own inclusive, 0-based, negative-index-from-the-end
// semantics), real pagination rather than a full-list fetch — a large list
// only ever has the requested window transferred.
func (e *Engine) GetList(ctx context.Context, key string, start, stop int64) ([]string, error) {
	if e.client == nil {
		return nil, ErrNotConnected
	}
	values, err := e.client.LRange(ctx, key, start, stop).Result()
	if err != nil {
		return nil, fmt.Errorf("redis: get list: %w", err)
	}
	return values, nil
}

// PushList appends values to the end of the list stored at key via RPUSH,
// creating the list if it does not already exist.
func (e *Engine) PushList(ctx context.Context, key string, values ...string) error {
	if e.client == nil {
		return ErrNotConnected
	}
	if len(values) == 0 {
		return nil
	}
	args := make([]interface{}, len(values))
	for i, value := range values {
		args[i] = value
	}
	if err := e.client.RPush(ctx, key, args...).Err(); err != nil {
		return fmt.Errorf("redis: push list: %w", err)
	}
	return nil
}

// SetListElement overwrites the element at index in the list stored at key
// via LSET, the single-element in-place edit the key browser's list-value
// editor uses (spec.md §4.5).
func (e *Engine) SetListElement(ctx context.Context, key string, index int64, value string) error {
	if e.client == nil {
		return ErrNotConnected
	}
	if err := e.client.LSet(ctx, key, index, value).Err(); err != nil {
		return fmt.Errorf("redis: set list element: %w", err)
	}
	return nil
}

// GetSet returns up to count members of the set stored at key, continuing
// from cursor (0 to start a fresh scan), plus the cursor to pass on the next
// call — the same cursor shape ScanKeys uses, for consistency. SSCAN is
// chosen over SMEMBERS-with-a-limit deliberately: SMEMBERS has no limit
// parameter at all (it always returns the entire set in one call, exactly
// the KEYS-style blocking-on-large-collections problem ScanKeys' own doc
// comment explains for the keyspace), while SSCAN gives the same
// incremental, cursor-based pagination for a set's members that SCAN gives
// for the keyspace — one mental model for both.
func (e *Engine) GetSet(ctx context.Context, key string, cursor uint64, count int64) ([]string, uint64, error) {
	if e.client == nil {
		return nil, 0, ErrNotConnected
	}
	members, nextCursor, err := e.client.SScan(ctx, key, cursor, "", count).Result()
	if err != nil {
		return nil, 0, fmt.Errorf("redis: get set: %w", err)
	}
	return members, nextCursor, nil
}

// AddToSet adds members to the set stored at key via SADD, creating the set
// if it does not already exist.
func (e *Engine) AddToSet(ctx context.Context, key string, members ...string) error {
	if e.client == nil {
		return ErrNotConnected
	}
	if len(members) == 0 {
		return nil
	}
	args := make([]interface{}, len(members))
	for i, member := range members {
		args[i] = member
	}
	if err := e.client.SAdd(ctx, key, args...).Err(); err != nil {
		return fmt.Errorf("redis: add to set: %w", err)
	}
	return nil
}

// RemoveFromSet removes members from the set stored at key via SREM.
func (e *Engine) RemoveFromSet(ctx context.Context, key string, members ...string) error {
	if e.client == nil {
		return ErrNotConnected
	}
	if len(members) == 0 {
		return nil
	}
	args := make([]interface{}, len(members))
	for i, member := range members {
		args[i] = member
	}
	if err := e.client.SRem(ctx, key, args...).Err(); err != nil {
		return fmt.Errorf("redis: remove from set: %w", err)
	}
	return nil
}

// SortedSetMember is one member/score pair of a Redis sorted set (ZSET).
type SortedSetMember struct {
	Member string
	Score  float64
}

// GetSortedSet returns the member/score pairs of the sorted set stored at
// key between start and stop (ZRANGE's own inclusive, 0-based,
// negative-index-from-the-end semantics, ordered by score ascending),
// paginated the same way GetList paginates a list.
func (e *Engine) GetSortedSet(ctx context.Context, key string, start, stop int64) ([]SortedSetMember, error) {
	if e.client == nil {
		return nil, ErrNotConnected
	}
	values, err := e.client.ZRangeWithScores(ctx, key, start, stop).Result()
	if err != nil {
		return nil, fmt.Errorf("redis: get sorted set: %w", err)
	}
	members := make([]SortedSetMember, len(values))
	for i, z := range values {
		member, ok := z.Member.(string)
		if !ok {
			member = fmt.Sprint(z.Member)
		}
		members[i] = SortedSetMember{Member: member, Score: z.Score}
	}
	return members, nil
}

// AddToSortedSet adds members to the sorted set stored at key via ZADD,
// creating the set if it does not already exist. Adding a member that
// already exists updates its score.
func (e *Engine) AddToSortedSet(ctx context.Context, key string, members ...SortedSetMember) error {
	if e.client == nil {
		return ErrNotConnected
	}
	if len(members) == 0 {
		return nil
	}
	values := make([]redisdriver.Z, len(members))
	for i, m := range members {
		values[i] = redisdriver.Z{Score: m.Score, Member: m.Member}
	}
	if err := e.client.ZAdd(ctx, key, values...).Err(); err != nil {
		return fmt.Errorf("redis: add to sorted set: %w", err)
	}
	return nil
}

// RemoveFromSortedSet removes members from the sorted set stored at key via
// ZREM.
func (e *Engine) RemoveFromSortedSet(ctx context.Context, key string, members ...string) error {
	if e.client == nil {
		return ErrNotConnected
	}
	if len(members) == 0 {
		return nil
	}
	args := make([]interface{}, len(members))
	for i, member := range members {
		args[i] = member
	}
	if err := e.client.ZRem(ctx, key, args...).Err(); err != nil {
		return fmt.Errorf("redis: remove from sorted set: %w", err)
	}
	return nil
}

// translateTTL turns the driver's own raw DurationCmd result for TTL
// (already unit-translated by the driver for a real expiry, but left as a
// raw -2/-1 sentinel in nanoseconds otherwise — see go-redis's
// DurationCmd.readReply) into this package's own contract: -2 becomes
// ErrKeyNotFound (a real Go error, so a caller can't mistake "key gone" for
// "no TTL"), -1 becomes (-1, nil) (a negative-but-not-error sentinel meaning
// "no TTL / persistent" that a caller — eventually the frontend — can check
// for), and every other value passes through unchanged.
func translateTTL(raw time.Duration) (time.Duration, error) {
	switch raw {
	case -2 * time.Nanosecond:
		return 0, ErrKeyNotFound
	case -1 * time.Nanosecond:
		return -1, nil
	default:
		return raw, nil
	}
}

// TTL returns the remaining time to live of key. A never-expiring key
// returns (-1, nil); a nonexistent key returns (0, ErrKeyNotFound) — see
// translateTTL's own doc comment for the sentinel translation this wraps.
func (e *Engine) TTL(ctx context.Context, key string) (time.Duration, error) {
	if e.client == nil {
		return 0, ErrNotConnected
	}
	raw, err := e.client.TTL(ctx, key).Result()
	if err != nil {
		return 0, fmt.Errorf("redis: ttl: %w", err)
	}
	return translateTTL(raw)
}

// SetTTL sets key's time to live to ttl via EXPIRE, the TTL-editing
// primitive (spec.md §4.5).
func (e *Engine) SetTTL(ctx context.Context, key string, ttl time.Duration) error {
	if e.client == nil {
		return ErrNotConnected
	}
	if err := e.client.Expire(ctx, key, ttl).Err(); err != nil {
		return fmt.Errorf("redis: set ttl: %w", err)
	}
	return nil
}

// PersistKey removes key's existing TTL via PERSIST, making it never expire
// — the "remove expiry" half of spec.md §4.5's "TTL visible and editable
// (set/persist/change)" requirement.
func (e *Engine) PersistKey(ctx context.Context, key string) error {
	if e.client == nil {
		return ErrNotConnected
	}
	if err := e.client.Persist(ctx, key).Err(); err != nil {
		return fmt.Errorf("redis: persist key: %w", err)
	}
	return nil
}

// RenameKey renames oldKey to newKey via RENAME, after first confirming
// newKey does not already exist. RENAME itself would silently overwrite an
// existing newKey, which this layer deliberately refuses: it returns
// ErrKeyExists instead, so the frontend's rename flow (spec.md §4.5) can
// surface a clear conflict rather than quietly destroying another key's
// value. This check-then-act is not atomic against a concurrent writer
// creating newKey between the EXISTS check and the RENAME call — an
// acceptable race for a single-user local dev tool, not a guarantee this
// method makes under concurrent access.
func (e *Engine) RenameKey(ctx context.Context, oldKey, newKey string) error {
	if e.client == nil {
		return ErrNotConnected
	}
	exists, err := e.client.Exists(ctx, newKey).Result()
	if err != nil {
		return fmt.Errorf("redis: rename key: check existing: %w", err)
	}
	if exists > 0 {
		return ErrKeyExists
	}
	if err := e.client.Rename(ctx, oldKey, newKey).Err(); err != nil {
		return fmt.Errorf("redis: rename key: %w", err)
	}
	return nil
}

// DeleteKeys deletes every key in keys via a single DEL call — supporting
// the multi-key delete the key browser's confirmation dialog requires
// (spec.md §4.5), not just one key at a time. Deleting zero keys is a
// no-op, not an error.
func (e *Engine) DeleteKeys(ctx context.Context, keys ...string) error {
	if e.client == nil {
		return ErrNotConnected
	}
	if len(keys) == 0 {
		return nil
	}
	if err := e.client.Del(ctx, keys...).Err(); err != nil {
		return fmt.Errorf("redis: delete keys: %w", err)
	}
	return nil
}
