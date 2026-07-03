// Package mongo is MongoDB's own client for this project's DB Client module
// (spec.md §4.4, plan.md §3). MongoDB is document-oriented, not the
// row/column shape dbengine.Engine's Query(ctx, query string) contract
// assumes — "find documents matching a filter" has no sensible mapping onto
// that interface. Engine therefore does NOT implement dbengine.Engine; it is
// a deliberately separate type with its own document-oriented surface
// (databases/collections listing, find/insert/update/delete/count/sample by
// database+collection), not a forced shim over the relational interface.
// A *Engine value is constructed already bound to one "mongodb://" URI via
// New; Connect performs the actual dial and an initial ping — construction
// itself never dials, matching the same lifecycle shape postgres.Engine/
// mysql.Engine use, for consistency across engines, even though this type
// implements no shared interface with them.
package mongo

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	mongodriver "go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// ErrNotConnected is returned by every method below except Connect when
// called before a successful Connect.
var ErrNotConnected = errors.New("mongo: not connected")

// disconnectTimeout bounds Close's own Disconnect call, since Close's
// signature (matching dbengine.Engine's Close for consistency, see this
// package's doc comment) carries no context/timeout of its own.
const disconnectTimeout = 10 * time.Second

// Engine is a MongoDB client bound to one "mongodb://" URI.
type Engine struct {
	uri    string
	client *mongodriver.Client
}

// New returns an Engine bound to uri, a standard "mongodb://" (or
// "mongodb+srv://") connection string — mongo-go-driver's own
// options.Client().ApplyURI parses this natively, so unlike mysql.New (which
// needs a separate URI-to-DSN translation layer, see mysql.go), no
// translation step is needed here. It does not dial; call Connect to
// establish the client.
func New(uri string) *Engine {
	return &Engine{uri: uri}
}

// Connect establishes the underlying driver client and confirms it is
// reachable with an initial ping. Calling Connect again after a prior
// successful call disconnects the existing client before replacing it.
func (e *Engine) Connect(ctx context.Context) error {
	client, err := mongodriver.Connect(ctx, options.Client().ApplyURI(e.uri))
	if err != nil {
		return fmt.Errorf("mongo: connect: %w", err)
	}
	if err := client.Ping(ctx, nil); err != nil {
		_ = client.Disconnect(ctx)
		return fmt.Errorf("mongo: connect: initial ping: %w", err)
	}
	if e.client != nil {
		_ = e.client.Disconnect(ctx)
	}
	e.client = client
	return nil
}

// Ping confirms the connection is still reachable.
func (e *Engine) Ping(ctx context.Context) error {
	if e.client == nil {
		return ErrNotConnected
	}
	if err := e.client.Ping(ctx, nil); err != nil {
		return fmt.Errorf("mongo: ping: %w", err)
	}
	return nil
}

// Close disconnects the underlying client under disconnectTimeout. It is
// safe to call more than once.
func (e *Engine) Close() error {
	if e.client == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), disconnectTimeout)
	defer cancel()
	err := e.client.Disconnect(ctx)
	e.client = nil
	if err != nil {
		return fmt.Errorf("mongo: close: %w", err)
	}
	return nil
}

// ListDatabases returns every database visible on this connection, the
// document-store analogue of dbengine.Engine's ListSchemas.
func (e *Engine) ListDatabases(ctx context.Context) ([]string, error) {
	if e.client == nil {
		return nil, ErrNotConnected
	}
	names, err := e.client.ListDatabaseNames(ctx, bson.M{})
	if err != nil {
		return nil, fmt.Errorf("mongo: list databases: %w", err)
	}
	return names, nil
}

// ListCollections returns every collection in database, the document-store
// analogue of dbengine.Engine's ListTables.
func (e *Engine) ListCollections(ctx context.Context, database string) ([]string, error) {
	if e.client == nil {
		return nil, ErrNotConnected
	}
	names, err := e.client.Database(database).ListCollectionNames(ctx, bson.M{})
	if err != nil {
		return nil, fmt.Errorf("mongo: list collections: %w", err)
	}
	return names, nil
}

// FindDocuments is the "browse collection" primitive (spec.md §4.4): it
// returns up to limit documents matching filter, skipping the first skip
// matches, sorted ascending by _id for a stable order across successive
// limit/skip calls — MongoDB gives no ordering guarantee across paginated
// queries without an explicit sort, and this is real limit/skip pagination
// from the start rather than one fixed large limit (the mistake
// BrowseTableRows' relational counterpart had to be corrected for, tasks.md
// 4.1). A limit or skip of 0 or less is treated as "no limit"/"no skip"
// respectively, matching MongoDB's own Find semantics rather than being
// rejected as invalid input.
//
// Every returned document has already been passed through sanitizeDocument
// (convert.go): BSON-specific types (primitive.ObjectID, primitive.DateTime,
// ...) are converted into JSON-marshalable, human-readable values before
// this method returns, since the raw BSON types don't serialize sensibly as
// JSON — callers never need to perform this conversion themselves.
func (e *Engine) FindDocuments(ctx context.Context, database, collection string, filter map[string]any, limit, skip int) ([]map[string]any, error) {
	if e.client == nil {
		return nil, ErrNotConnected
	}

	opts := options.Find().SetSort(bson.D{{Key: "_id", Value: 1}})
	if limit > 0 {
		opts.SetLimit(int64(limit))
	}
	if skip > 0 {
		opts.SetSkip(int64(skip))
	}

	coll := e.client.Database(database).Collection(collection)
	cursor, err := coll.Find(ctx, toBSONFilter(filter), opts)
	if err != nil {
		return nil, fmt.Errorf("mongo: find documents: %w", err)
	}
	defer cursor.Close(ctx)

	return decodeDocuments(ctx, cursor, "find documents")
}

// CountDocuments returns how many documents in collection match filter —
// used by the collection browser (tasks.md 5.5) to show a total count and
// support pagination alongside FindDocuments' limit/skip.
func (e *Engine) CountDocuments(ctx context.Context, database, collection string, filter map[string]any) (int64, error) {
	if e.client == nil {
		return 0, ErrNotConnected
	}
	coll := e.client.Database(database).Collection(collection)
	count, err := coll.CountDocuments(ctx, toBSONFilter(filter))
	if err != nil {
		return 0, fmt.Errorf("mongo: count documents: %w", err)
	}
	return count, nil
}

// InsertDocument inserts doc into collection and returns the inserted
// document including its generated _id (or the caller-supplied _id, if doc
// already had one) — the frontend's "new document" flow (spec.md §4.4)
// starts from an empty {} or a duplicate of a selected document and calls
// this once the user commits it. The returned document is sanitized the
// same way FindDocuments' results are (see convert.go).
func (e *Engine) InsertDocument(ctx context.Context, database, collection string, doc map[string]any) (map[string]any, error) {
	if e.client == nil {
		return nil, ErrNotConnected
	}
	coll := e.client.Database(database).Collection(collection)
	result, err := coll.InsertOne(ctx, toBSONM(doc))
	if err != nil {
		return nil, fmt.Errorf("mongo: insert document: %w", err)
	}

	inserted := make(map[string]any, len(doc)+1)
	for k, v := range doc {
		inserted[k] = v
	}
	inserted["_id"] = result.InsertedID

	return sanitizeDocument(inserted), nil
}

// UpdateDocument replaces the document whose _id equals id with doc — the
// in-place document editing flow (spec.md §4.4), where the frontend has
// already validated doc as structurally sound JSON before calling this. id
// is the hex-encoded ObjectID string FindDocuments' output carries as _id;
// it is converted back into a primitive.ObjectID here for the actual _id
// filter, so callers never handle primitive.ObjectID themselves. Any _id key
// present in doc is ignored — MongoDB's _id is immutable, and the document
// is matched and kept under its existing _id regardless of what doc itself
// contains for that key.
func (e *Engine) UpdateDocument(ctx context.Context, database, collection, id string, doc map[string]any) error {
	if e.client == nil {
		return ErrNotConnected
	}
	objID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return fmt.Errorf("mongo: update document: invalid id %q: %w", id, err)
	}

	replacement := make(bson.M, len(doc))
	for k, v := range doc {
		if k == "_id" {
			continue
		}
		replacement[k] = v
	}

	coll := e.client.Database(database).Collection(collection)
	result, err := coll.ReplaceOne(ctx, bson.M{"_id": objID}, replacement)
	if err != nil {
		return fmt.Errorf("mongo: update document: %w", err)
	}
	if result.MatchedCount == 0 {
		return fmt.Errorf("mongo: update document: no document found with id %q", id)
	}
	return nil
}

// DeleteDocuments deletes every document in collection whose _id is in ids —
// supporting the multi-document delete the frontend's confirmation dialog
// requires (spec.md §4.4, tasks.md 5.4), not just a single id at a time.
// Each id is the hex-encoded ObjectID string FindDocuments' output carries
// as _id, converted back into a primitive.ObjectID here. Deleting zero ids
// is a no-op, not an error.
func (e *Engine) DeleteDocuments(ctx context.Context, database, collection string, ids []string) error {
	if e.client == nil {
		return ErrNotConnected
	}
	if len(ids) == 0 {
		return nil
	}

	objIDs := make([]primitive.ObjectID, len(ids))
	for i, id := range ids {
		objID, err := primitive.ObjectIDFromHex(id)
		if err != nil {
			return fmt.Errorf("mongo: delete documents: invalid id %q: %w", id, err)
		}
		objIDs[i] = objID
	}

	coll := e.client.Database(database).Collection(collection)
	if _, err := coll.DeleteMany(ctx, bson.M{"_id": bson.M{"$in": objIDs}}); err != nil {
		return fmt.Errorf("mongo: delete documents: %w", err)
	}
	return nil
}

// SampleDocuments returns n randomly sampled documents from collection using
// MongoDB's own $sample aggregation stage — the idiomatic way to get a
// random sample efficiently without scanning the whole collection. This
// exists now, alongside the rest of this package, so task 5.6 (Schema
// Diagram — MongoDB inferred structure) doesn't need to touch this package
// again; the shape-inference logic that turns sampled documents into an
// inferred schema is that task's own job, layered on top of this raw
// sampling primitive, not implemented here.
func (e *Engine) SampleDocuments(ctx context.Context, database, collection string, n int) ([]map[string]any, error) {
	if e.client == nil {
		return nil, ErrNotConnected
	}

	pipeline := mongodriver.Pipeline{
		{{Key: "$sample", Value: bson.D{{Key: "size", Value: n}}}},
	}

	coll := e.client.Database(database).Collection(collection)
	cursor, err := coll.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, fmt.Errorf("mongo: sample documents: %w", err)
	}
	defer cursor.Close(ctx)

	return decodeDocuments(ctx, cursor, "sample documents")
}

// decodeDocuments drains cursor into a sanitized []map[string]any, shared by
// every method here that returns a document list (FindDocuments,
// SampleDocuments). op names the caller in wrapped error messages.
func decodeDocuments(ctx context.Context, cursor *mongodriver.Cursor, op string) ([]map[string]any, error) {
	var docs []map[string]any
	for cursor.Next(ctx) {
		var raw bson.M
		if err := cursor.Decode(&raw); err != nil {
			return nil, fmt.Errorf("mongo: %s: decode: %w", op, err)
		}
		docs = append(docs, sanitizeDocument(raw))
	}
	if err := cursor.Err(); err != nil {
		return nil, fmt.Errorf("mongo: %s: cursor: %w", op, err)
	}
	return docs, nil
}

// toBSONFilter converts filter into a bson.M the driver's Find/
// CountDocuments accept, treating a nil filter as "match everything" ({})
// rather than passing a nil map through to the driver.
func toBSONFilter(filter map[string]any) bson.M {
	return toBSONM(filter)
}

// toBSONM converts m into a bson.M, normalizing nil to an empty (but
// non-nil) map — bson.M and map[string]any share the same underlying type,
// so this is a plain conversion plus a nil guard, not a deep copy.
func toBSONM(m map[string]any) bson.M {
	if m == nil {
		return bson.M{}
	}
	return bson.M(m)
}
