package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	dbenginemongo "stackyard/internal/dbengine/mongo"
	"stackyard/internal/diagram"
	"stackyard/internal/storage"

	"github.com/google/uuid"
)

// defaultMongoSampleSize is the sample size SampleMongoDocuments and
// BuildMongoStructureDiagram fall back to when the caller requests n <= 0,
// satisfying spec.md §4.11's "N configurable, with a sensible default"
// requirement at the Go layer, not only via the frontend's own input
// default (tasks.md 5.6).
const defaultMongoSampleSize = 100

// buildMongoConnectionURI translates fields into a "mongodb://" connection
// string, the same way buildPostgresTestConnString translates
// ConnectionFormFields into a Postgres URL — except, unlike Postgres/MySQL,
// this is the ONLY translation Mongo ever needs: dbenginemongo.New (and
// mongo-go-driver's own options.Client().ApplyURI underneath it) parses a
// "mongodb://" URI natively, so there is no separate DSN grammar to target
// the way buildMySQLTestDSN has to. Built fresh from the current form-field
// state on every call, matching the Postgres/MySQL builders' own "always
// rebuilt, never the originally pasted string" convention (spec.md §4.1's
// "editable afterward" requirement).
func buildMongoConnectionURI(fields ConnectionFormFields) string {
	var userInfo *url.Userinfo
	switch {
	case fields.Password != "":
		userInfo = url.UserPassword(fields.Username, fields.Password)
	case fields.Username != "":
		userInfo = url.User(fields.Username)
	}

	u := &url.URL{
		Scheme: "mongodb",
		User:   userInfo,
		Host:   fmt.Sprintf("%s:%d", fields.Host, fields.Port),
	}
	if fields.Database != "" {
		u.Path = "/" + fields.Database
	}

	if len(fields.Params) > 0 {
		query := url.Values{}
		for key, value := range fields.Params {
			query.Set(key, value)
		}
		u.RawQuery = query.Encode()
	}

	return u.String()
}

// OpenMongoConnection dials fields (which must name storage.EngineMongoDB)
// and keeps the resulting *dbenginemongo.Engine alive server-side, returning
// a session ID the frontend passes to every other Mongo bound method below,
// across as many separate IPC calls as it wants (tasks.md 5.1) — the
// Mongo-side counterpart of OpenConnection. Every call opens its own new,
// independent session, even for identical fields, matching OpenConnection's
// own one-session-per-tab convention (tasks.md 3.8).
func (a *App) OpenMongoConnection(fields ConnectionFormFields) (string, error) {
	if fields.Engine != storage.EngineMongoDB {
		return "", fmt.Errorf("open mongo connection: expected engine %q, got %q", storage.EngineMongoDB, fields.Engine)
	}
	if err := validateConnectionFormFields(fields); err != nil {
		return "", fmt.Errorf("open mongo connection: %w", err)
	}

	engine := dbenginemongo.New(buildMongoConnectionURI(fields))

	ctx, cancel := context.WithTimeout(a.ctx, openMongoConnectionTimeout)
	defer cancel()

	if err := engine.Connect(ctx); err != nil {
		return "", fmt.Errorf("open mongo connection: %w", err)
	}
	if err := engine.Ping(ctx); err != nil {
		_ = engine.Close()
		return "", fmt.Errorf("open mongo connection: %w", err)
	}

	id := uuid.NewString()
	a.putMongoSession(id, &mongoSession{engine: engine})
	return id, nil
}

// CloseMongoSession closes the live Mongo session behind sessionID and
// removes it from a's session map (tasks.md 5.1) — the Mongo-side
// counterpart of CloseConnectionSession. Closing an unknown or
// already-closed sessionID is an error, not a silent no-op, for the same
// bookkeeping-drift-should-be-detectable reason CloseConnectionSession's own
// doc comment gives.
func (a *App) CloseMongoSession(sessionID string) error {
	session, ok := a.deleteMongoSession(sessionID)
	if !ok {
		return fmt.Errorf("close mongo session: no open mongo session %q", sessionID)
	}
	if err := session.engine.Close(); err != nil {
		return fmt.Errorf("close mongo session: %w", err)
	}
	return nil
}

// decodeMongoJSONObject parses raw as a JSON object into map[string]any, for
// bound methods that accept a filter or document body as a JSON string from
// the frontend (FindMongoDocuments' filterJSON, InsertMongoDocument/
// UpdateMongoDocument's docJSON) rather than a Wails-bridged map — the
// tree/JSON document editor (tasks.md 5.2/5.3) naturally works with raw JSON
// text, and unmarshaling it here surfaces a malformed-JSON error inline
// rather than the frontend needing its own separate validation pass before
// ever calling into Go. A blank/whitespace-only raw is treated as "{}" (no
// filter, or an empty document), not an error.
func decodeMongoJSONObject(raw string) (map[string]any, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return map[string]any{}, nil
	}
	var obj map[string]any
	if err := json.Unmarshal([]byte(trimmed), &obj); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}
	return obj, nil
}

// ListMongoDatabases returns every database visible on the live Mongo
// session behind sessionID (see OpenMongoConnection) — the Mongo-side
// counterpart of ListSchemasForSession.
func (a *App) ListMongoDatabases(sessionID string) ([]string, error) {
	session, ok := a.getMongoSession(sessionID)
	if !ok {
		return nil, fmt.Errorf("list mongo databases: no open mongo session %q", sessionID)
	}

	ctx, cancel := context.WithTimeout(a.ctx, mongoOperationTimeout)
	defer cancel()

	databases, err := session.engine.ListDatabases(ctx)
	if err != nil {
		return nil, fmt.Errorf("list mongo databases: %w", err)
	}
	if databases == nil {
		databases = []string{}
	}
	return databases, nil
}

// ListMongoCollections returns every collection in database, visible on the
// live Mongo session behind sessionID (see OpenMongoConnection) — the
// Mongo-side counterpart of ListTablesForSession.
func (a *App) ListMongoCollections(sessionID string, database string) ([]string, error) {
	session, ok := a.getMongoSession(sessionID)
	if !ok {
		return nil, fmt.Errorf("list mongo collections: no open mongo session %q", sessionID)
	}

	ctx, cancel := context.WithTimeout(a.ctx, mongoOperationTimeout)
	defer cancel()

	collections, err := session.engine.ListCollections(ctx, database)
	if err != nil {
		return nil, fmt.Errorf("list mongo collections: %w", err)
	}
	if collections == nil {
		collections = []string{}
	}
	return collections, nil
}

// FindMongoDocuments is the collection browser's (tasks.md 5.5) "browse
// documents" call: filterJSON is a JSON object as text (an empty string
// means "match everything"), unmarshaled here into the map[string]any
// dbenginemongo.Engine.FindDocuments expects, so the frontend never builds a
// Wails-bridged filter map itself. Every returned document has already been
// sanitized (BSON types converted to JSON-safe values) by FindDocuments
// itself before this method ever sees it.
func (a *App) FindMongoDocuments(sessionID string, database string, collection string, filterJSON string, limit int, skip int) ([]map[string]any, error) {
	session, ok := a.getMongoSession(sessionID)
	if !ok {
		return nil, fmt.Errorf("find mongo documents: no open mongo session %q", sessionID)
	}

	filter, err := decodeMongoJSONObject(filterJSON)
	if err != nil {
		return nil, fmt.Errorf("find mongo documents: filter: %w", err)
	}

	ctx, cancel := context.WithTimeout(a.ctx, mongoOperationTimeout)
	defer cancel()

	docs, err := session.engine.FindDocuments(ctx, database, collection, filter, limit, skip)
	if err != nil {
		return nil, fmt.Errorf("find mongo documents: %w", err)
	}
	if docs == nil {
		docs = []map[string]any{}
	}
	return docs, nil
}

// CountMongoDocuments returns how many documents in collection match
// filterJSON (same JSON-object-as-text convention as FindMongoDocuments).
// Not currently called by the collection browser, which paginates via a
// fetched-a-full-page heuristic instead — exposed for a future caller that
// wants an exact total.
func (a *App) CountMongoDocuments(sessionID string, database string, collection string, filterJSON string) (int64, error) {
	session, ok := a.getMongoSession(sessionID)
	if !ok {
		return 0, fmt.Errorf("count mongo documents: no open mongo session %q", sessionID)
	}

	filter, err := decodeMongoJSONObject(filterJSON)
	if err != nil {
		return 0, fmt.Errorf("count mongo documents: filter: %w", err)
	}

	ctx, cancel := context.WithTimeout(a.ctx, mongoOperationTimeout)
	defer cancel()

	count, err := session.engine.CountDocuments(ctx, database, collection, filter)
	if err != nil {
		return 0, fmt.Errorf("count mongo documents: %w", err)
	}
	return count, nil
}

// InsertMongoDocument creates a new document in collection from docJSON (a
// JSON object as text — the tree/JSON editor's "new document from an empty
// {} or a duplicate" flow, tasks.md 5.4) and returns the inserted document,
// including its generated _id as a hex string, already sanitized the same
// way FindMongoDocuments' results are.
func (a *App) InsertMongoDocument(sessionID string, database string, collection string, docJSON string) (map[string]any, error) {
	session, ok := a.getMongoSession(sessionID)
	if !ok {
		return nil, fmt.Errorf("insert mongo document: no open mongo session %q", sessionID)
	}

	doc, err := decodeMongoJSONObject(docJSON)
	if err != nil {
		return nil, fmt.Errorf("insert mongo document: %w", err)
	}

	ctx, cancel := context.WithTimeout(a.ctx, mongoOperationTimeout)
	defer cancel()

	inserted, err := session.engine.InsertDocument(ctx, database, collection, doc)
	if err != nil {
		return nil, fmt.Errorf("insert mongo document: %w", err)
	}
	return inserted, nil
}

// UpdateMongoDocument replaces the document identified by id (the
// hex-encoded ObjectID string FindMongoDocuments' output carries as _id)
// with docJSON — the in-place document editing flow (tasks.md 5.3), where
// the frontend has already validated docJSON as structurally sound JSON
// before calling this; decodeMongoJSONObject's own parse error is a second,
// server-side backstop, not the primary validation path.
func (a *App) UpdateMongoDocument(sessionID string, database string, collection string, id string, docJSON string) error {
	session, ok := a.getMongoSession(sessionID)
	if !ok {
		return fmt.Errorf("update mongo document: no open mongo session %q", sessionID)
	}

	doc, err := decodeMongoJSONObject(docJSON)
	if err != nil {
		return fmt.Errorf("update mongo document: %w", err)
	}

	ctx, cancel := context.WithTimeout(a.ctx, mongoOperationTimeout)
	defer cancel()

	if err := session.engine.UpdateDocument(ctx, database, collection, id, doc); err != nil {
		return fmt.Errorf("update mongo document: %w", err)
	}
	return nil
}

// DeleteMongoDocuments deletes every document in collection whose _id is in
// ids (each the hex-encoded ObjectID string FindMongoDocuments' output
// carries as _id) — supporting the multi-document delete the frontend's
// confirmation dialog requires (tasks.md 5.4).
func (a *App) DeleteMongoDocuments(sessionID string, database string, collection string, ids []string) error {
	session, ok := a.getMongoSession(sessionID)
	if !ok {
		return fmt.Errorf("delete mongo documents: no open mongo session %q", sessionID)
	}

	ctx, cancel := context.WithTimeout(a.ctx, mongoOperationTimeout)
	defer cancel()

	if err := session.engine.DeleteDocuments(ctx, database, collection, ids); err != nil {
		return fmt.Errorf("delete mongo documents: %w", err)
	}
	return nil
}

// SampleMongoDocuments returns up to n randomly sampled documents from
// collection via the live Mongo session behind sessionID (see
// OpenMongoConnection) — the raw sampling primitive the Mongo Schema
// Diagram feature (tasks.md 5.6, spec.md §4.11) is built from.
// BuildMongoStructureDiagram below is the single call the frontend's Mongo
// schema-diagram view actually needs for rendering; this method is exposed
// separately since the sampling primitive is independently useful (e.g. a
// future "preview a few raw sample documents" affordance) and mirrors
// dbenginemongo.Engine.SampleDocuments one-to-one, the same way
// FindMongoDocuments mirrors FindDocuments. n <= 0 falls back to
// defaultMongoSampleSize.
func (a *App) SampleMongoDocuments(sessionID string, database string, collection string, n int) ([]map[string]any, error) {
	session, ok := a.getMongoSession(sessionID)
	if !ok {
		return nil, fmt.Errorf("sample mongo documents: no open mongo session %q", sessionID)
	}
	if n <= 0 {
		n = defaultMongoSampleSize
	}

	ctx, cancel := context.WithTimeout(a.ctx, mongoOperationTimeout)
	defer cancel()

	docs, err := session.engine.SampleDocuments(ctx, database, collection, n)
	if err != nil {
		return nil, fmt.Errorf("sample mongo documents: %w", err)
	}
	if docs == nil {
		docs = []map[string]any{}
	}
	return docs, nil
}

// BuildMongoStructureDiagram samples sampleSize documents from every
// collection in database (via the live Mongo session behind sessionID),
// infers each collection's shape with diagram.InferCollectionShape, and
// renders the result as Mermaid erDiagram text via
// diagram.BuildMongoStructureDiagram (tasks.md 5.6, spec.md §4.11) — the
// Mongo-side counterpart of BuildSchemaDiagram. Every collection in database
// is always included, even one whose sample comes back empty (matching
// diagram.BuildMongoStructureDiagram's own "every collection gets a block"
// precedent, itself matching BuildRelationalERDiagram's "every table gets a
// block"). sampleSize <= 0 falls back to defaultMongoSampleSize. This is
// called once on the frontend view's connect/mount and again on every
// explicit "Regenerate" click — there is no cached/auto-updating diagram
// state on the Go side, matching spec.md §4.11's explicit "not a live/
// auto-updating view" requirement.
func (a *App) BuildMongoStructureDiagram(sessionID string, database string, sampleSize int) (string, error) {
	session, ok := a.getMongoSession(sessionID)
	if !ok {
		return "", fmt.Errorf("build mongo structure diagram: no open mongo session %q", sessionID)
	}
	if sampleSize <= 0 {
		sampleSize = defaultMongoSampleSize
	}

	ctx, cancel := context.WithTimeout(a.ctx, mongoOperationTimeout)
	defer cancel()

	collections, err := session.engine.ListCollections(ctx, database)
	if err != nil {
		return "", fmt.Errorf("build mongo structure diagram: %w", err)
	}

	shapes := make(map[string]diagram.CollectionShape, len(collections))
	for _, collection := range collections {
		docs, err := session.engine.SampleDocuments(ctx, database, collection, sampleSize)
		if err != nil {
			return "", fmt.Errorf("build mongo structure diagram: collection %q: %w", collection, err)
		}
		shapes[collection] = diagram.InferCollectionShape(docs)
	}

	return diagram.BuildMongoStructureDiagram(shapes), nil
}
