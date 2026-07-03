# Changelog

All notable changes to Stackyard are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- Wails v2 + React + TypeScript project scaffold (`wails init`, React-TS
  template), merged into the repo root; `wails dev` launches a real native
  window (task 0.1).
- Tailwind-based dark-mode app shell: sidebar navigation (Environments / DB
  Client), top bar, dark mode as the only v1 theme (task 0.2).
- Go↔React IPC smoke test: `App.Ping() string` bound via Wails and called
  from a "Ping backend" button, exercising the generated TypeScript
  bindings end-to-end (task 0.3).
- `internal/storage`: SQLite persistence layer (`modernc.org/sqlite`,
  pure-Go/CGO-free) with schema for `profiles`, `services`, `connections`,
  `snippets`, `query_history`; idempotent migration via
  `PRAGMA user_version`; DB file resolved to the OS app-data path
  (`%APPDATA%\Stackyard\stackyard.db` on Windows) (task 0.4).
- `docs/STATE.md` created as the living pause/resume state document
  (task 0.5).
- `internal/docker/client.go`: thin wrapper over `docker/docker/client`,
  verified live against the local Docker Engine over a Windows named pipe
  (task 1.1).
- `internal/storage`: full `Profile`/`Service` CRUD (create/read/update/
  delete/list) with cascade-delete verified at the storage layer (task
  1.2).
- `internal/docker/compose.go`: network/volume/container orchestration for
  a Postgres service — `EnsureNetwork`/`EnsureVolume`/
  `EnsurePostgresContainer`/`StartPostgresEnvironment`, no compose file
  ever written to disk; verified against the live Docker Engine for
  create-from-scratch, idempotent reuse, and stopped-then-restarted-
  in-place, all with full cleanup (task 1.3).
- App-bound Environment Manager methods (`ListProfiles`, `CreateProfile`,
  `StartProfile`, `StopProfile`, `RestartProfile`, `GetProfileStatus`,
  Postgres-only MVP scope) with non-fatal storage/Docker initialization —
  a failed `dbErr`/`dockerErr` is surfaced through `requireDB`/
  `requireDocker` guards on every dependent method instead of crashing the
  app at startup; real React profile list + Start/Stop UI replacing the
  Phase 0 placeholder (task 1.4).
- OS-level port-conflict pre-check: `internal/netcheck` (real port
  availability probe) and `internal/docker/portcheck.go` (conflict
  detection that exempts a service's own already-running container);
  `CheckPortAvailable`/`SuggestFreePort`/`CheckProfilePortConflict` bound
  on `App`, so `StartProfile` surfaces an actionable "port already in
  use — try 5433" message instead of a raw Docker bind error (task 1.5).
- `internal/docker/connstring.go`: Postgres connection-string builder
  (`net/url`, safe percent-encoding) bound via `GetConnectionString`,
  with a one-click clipboard copy and inline "Copied!" confirmation in the
  frontend (task 1.6).
- `CLAUDE.md`: project-wide comment-style convention (doc comments only —
  package/exported-symbol doc comments per Go/TS convention; no inline
  "why" comments, rationale goes in `docs/STATE.md` instead).
- `internal/docker/mysql.go`: MySQL Docker orchestration (container port
  `3306/tcp`, data dir `/var/lib/mysql`), extending the Postgres pattern
  from task 1.3 — root vs. regular-user credential mapping handled since
  `storage.Service` has one shared username/password slot across all 4
  engines (task 2.1).
- `internal/docker/mongodb.go`: MongoDB Docker orchestration (container
  port `27017/tcp`, data dir `/data/db`); `MONGO_INITDB_DATABASE` omitted
  entirely (not defaulted) when no DB name is set, since Mongo creates
  databases lazily on first write (task 2.2).
- `internal/docker/redis.go`: Redis Docker orchestration (container port
  `6379/tcp`, data dir `/data`); no-auth by default when no password is
  set, matching Postgres's zero-friction local-dev default (task 2.3).
- Multi-engine profile creation wizard: `CreateProfile` now accepts any
  combination of 1-4 engines in a single profile via
  `(name string, services []ServiceRequest)`; a per-engine start/stop/
  status dispatch table drives multi-service start/stop as one unit from
  a single click; `GetConnectionString` dispatches to the right
  per-engine builder; each engine gets its OS-standard default port
  (Postgres 5432, MySQL 3306, MongoDB 27017, Redis 6379) via
  `assignHostPorts` (task 2.4).
- Profile duplicate/rename/delete in the UI (`app.go` +
  `EnvironmentManagerView.tsx`), backed by task 1.2's persistence layer:
  duplicate generates a fresh volume name per profile/service (never
  copies the source volume, which would silently share live data) and a
  collision-safe `"<name> (copy)"`/`"(copy 2)"` name; delete is refused
  while a profile is running/partial/unknown, not silently orphaned
  (task 2.5).
- "Reset volume" for a single service (`app.go`'s `ResetServiceVolume` +
  `internal/docker/cleanup.go`'s `RemoveContainer`/`RemoveVolume`/
  `RemoveNetwork`): stop → remove container → remove volume → recreate
  fresh on next start, with an explicit confirmation dialog; sibling
  services in the same profile verified to stay running throughout,
  including under concurrent integration-test load (task 2.6).
- `internal/docker/stats.go`: CPU%/RAM polling per container via
  `ContainerStatsOneShot`, using the same formula `docker stats` itself
  uses; batch polling (`StatsForContainers`) reports per-container errors
  independently instead of failing or dropping the whole batch (task
  2.7).
- Real-time status dashboard (`internal/docker/snapshot.go` +
  `StatusDashboard.tsx`, new "Status" sidebar item): all profiles/
  services, state, port, CPU/RAM, pushed via a Wails event
  (`"environment:status"`, ~1.5s cadence) rather than frontend polling;
  verified to reflect containers stopped outside the app (e.g. a direct
  `docker stop` from another terminal), not just app-initiated state
  changes (task 2.8).

This completes **Module 1 — Environment Manager** in full (spec.md §3):
all 4 engines (Postgres, MySQL, MongoDB, Redis), profile CRUD, volume
reset, and a live status/stats dashboard.

- `internal/dbengine/engine.go`: the `Engine` interface (`Connect`, `Ping`,
  `Query`, `ListSchemas`, `ListTables`, `Close`, plus `QueryResult`/
  `ColumnInfo`/`TableInfo`) shared by every Module 2 (DB Client) engine
  implementation (task 3.1).
- Real Postgres (`pgx`) and MySQL (`go-sql-driver/mysql`) `Engine`
  implementations, including schema/table listing via
  `information_schema` for both engines; server-side query cancellation
  verified against a live Docker Engine (a 30s `pg_sleep`/`SLEEP` aborted
  in ~1s under a 1s-timeout context, not just abandoned client-side)
  (task 3.2).
- `internal/dbengine/urlparse.go`: connection-string parser for all 4
  engine URL schemes into `ConnectionFields`, with 12 distinct
  malformed-input cases each individually tested against their exact
  error string (empty input, missing scheme separator, empty/unsupported
  scheme, missing host, non-numeric/out-of-range port, trailing colon
  with no port digits, malformed userinfo, username-on-redis, missing
  database for postgres/mysql, multi-segment database path) (task 3.3).
- Connection form UI (`DbClientView.tsx` + `app.go`'s
  `ParseConnectionURL`/`TestConnection`): paste a connection URL to
  autofill all fields (still editable afterward), or fill fields
  manually; "Test Connection" reports success/failure without saving
  anything (task 3.4).
- Saved connections list (`internal/storage/connections.go` +
  `app.go`), persisted across restarts — verified for real by killing
  and relaunching the whole `wails dev` process tree and confirming a
  saved connection was still listed, loadable, and deletable (task 3.5).
- Monaco-based query editor (`@monaco-editor/react`, bundled locally —
  see Fixed below) wired to a real per-session run/cancel API
  (`OpenConnection`/`RunQuery`/`CancelQuery`/`CloseConnectionSession`),
  designed from the start for multi-tab independence: every
  `OpenConnection` call creates its own session with no implicit sharing
  (task 3.6).
- Read-only results grid (`ResultsGrid` + `resultsGridHelpers.ts`, this
  project's first Vitest suite) with real per-column type metadata
  (`QueryResult.Columns` now `[]ResultColumn{Name, DatabaseType,
  Nullable *bool}` — see Changed below), client-side pagination
  (100 rows/page), and NULL visually distinct from an empty string (task
  3.7).
- Multi-tab shell (`TabBar.tsx` + `tabState.ts` + `DbClientView.tsx`):
  open/close tabs, each bound to its own connection session; tabs stay
  mounted-and-hidden (not swapped) so scroll position and unsaved query
  text survive a tab switch; cross-tab independence verified for real
  against two live containers (Postgres + MySQL) — running a query and
  typing a draft in one tab left a second tab's own query/result
  completely untouched (task 3.8).

This completes the **DB Client MVP slice of Module 2** (spec.md §4) for
the two engines built so far (Postgres, MySQL) — the full relational
feature set (editable grid, schema diagrams, MongoDB/Redis support) is
Phase 4/4.5's job, not this one.

- Editable relational data grid (`grid.go`, a dedicated table-browse
  architecture — new bound methods scoped to a named table/schema, not
  detection of arbitrary query results): in-place cell edit → `UPDATE` by
  primary key; row insert (blank row bound to column defaults/types); row
  delete with confirmation for multi-row deletes; inline surfacing of the
  database's actual error message on the offending cell/row; PK-less
  tables fall back to read-only with a distinguishable, visible reason
  (`ErrTableHasNoPrimaryKey`). Scoped explicitly to Postgres/MySQL —
  MongoDB/Redis sessions are rejected outright (tasks 4.1-4.4).
- Multi-statement SQL execution engine (`internal/dbengine/batch.go`'s
  `ExecuteBatch`/`ExecuteMultiStatementText`, `multiquery.go`'s
  `RunMultiStatementQuery` bound method): runs each statement
  independently regardless of earlier failures, shares `RunQuery`'s
  cancellation mechanism, logs one `query_history` entry per statement.
  `QueryEditor.tsx`'s "Run query" now calls this instead of
  single-statement `RunQuery`, collapsing to the pre-existing single-
  result view when there's exactly one statement and rendering a
  per-statement result list otherwise — closes spec.md §4.6's
  previously-flagged gap in the UI, not just at the Go layer.
- Query history (`internal/storage/query_history.go` + `app.go`): every
  execution logged per saved connection (ad-hoc, never-saved connections
  are intentionally excluded from logging), a filterable/searchable
  history panel, and a "replay into new tab" action (task 4.5).
- Snippets CRUD (`internal/storage/snippets.go` + `SnippetsPanel.tsx`):
  name/tags, connection-scoped or global, compatible-engine filtering (a
  scoped snippet is offered only to its own connection, a global snippet
  only to connections of a matching engine) applied in the UI based on
  the active tab's connection, case-insensitive search on name/tags
  (task 4.6).
- "Run snippet" (`snippetRunLogic.ts`): loads a snippet into the current
  tab, or a new tab when the current one is dirty, with precise
  dirty-tab detection (a tab's baseline only updates on an explicit load,
  never on running a query) and a connection-selection fallback chain for
  global snippets opened into a new tab; the snippet is never
  auto-executed, only loaded into the editor (task 4.7).
- Monaco autocomplete (`schemaCompletion.ts`/`schemaCompletionProvider.ts`):
  table/column suggestions sourced from `ListSchemas`/`ListTables`, with
  proven cross-tab isolation — each `QueryEditor` instance registers its
  own schema closure against its own Monaco model and deregisters at
  unmount, so one tab's tables never leak into another tab's suggestions
  (task 4.8).

This completes **Phase 4 — Relational DB Client, Complete** (tasks
4.1-4.8).

- Schema Diagram (`internal/diagram/relational.go` + `schema-diagram/`):
  `Engine.ListForeignKeys` (Postgres + MySQL) added to the `Engine`
  interface for FK relationship metadata; `BuildRelationalERDiagram`
  translates schema + FK metadata into valid Mermaid `erDiagram` text,
  verified both via exact-string Go tests and by feeding that exact
  output through Mermaid's own real `mermaid.parse()` in Node (not just
  string-equality in Go); zoom/pan via CSS `transform` (no new library);
  export to PNG/SVG and copy raw Mermaid text to clipboard; a
  "Regenerate" button — diagrams do not auto-refresh live (tasks
  4.5.1-4.5.5).

This completes **Phase 4.5 — Schema Diagram (Relational)** and, together
with Phase 4 above, **Module 2's relational feature set for Postgres and
MySQL** (spec.md §4) — MongoDB and Redis support remain Phases 5/6's job.

- MongoDB Engine (`internal/dbengine/mongo/mongo.go`, official
  `go.mongodb.org/mongo-driver`, v1): a document-oriented surface deliberately
  separate from the relational `dbengine.Engine` interface — `ListDatabases`,
  `ListCollections`, `FindDocuments`/`CountDocuments` with real `limit`/`skip`
  pagination from the start, `InsertDocument`, `UpdateDocument`,
  `DeleteDocuments`, `SampleDocuments` (via `$sample`, for the Schema Diagram
  below). `app.go` gained a parallel `mongoSessions` map alongside the
  existing SQL `querySessions` map, mirroring the Schema Diagram's earlier
  precedent of an independent session type rather than one polymorphic
  abstraction (task 5.1).
- `internal/dbengine/mongo/convert.go`: recursive BSON→JSON-safe conversion
  (`primitive.ObjectID`→hex string, `DateTime`/`time.Time`→RFC3339Nano,
  `Decimal128`→decimal string, `Binary`→base64, `Regex`→pattern, recursing into
  nested `bson.M`/`bson.A`) so every document crosses the Wails/JSON boundary
  safely, with `_id` carried end-to-end as a plain hex string (task 5.1).
- Unified multi-tab shell for MongoDB: `DbClientTab` became a discriminated
  union (`SqlTab | MongoTab`) sharing the same tab strip as SQL connections —
  `TabBar`/`tabState.ts` needed zero changes since both were already
  engine-agnostic. Matches spec.md's "single, coherent UI — no per-engine tool
  switching" goal more directly than a second, Mongo-only tab strip would have
  (tasks 5.2-5.4).
- Document tree/JSON viewer (`MongoDocumentTree.tsx`/`MongoJSONEditor.tsx`/
  `MongoDocumentView.tsx`): expandable/collapsible document tree with typed
  scalar rendering (an ObjectID/date display heuristic keyed off `convert.go`'s
  exact output shapes) and collapsible `array [N items]`/`object {N keys}`
  summaries; whole-document JSON editing (not per-leaf) with structural
  validation before save; new-document creation (blank `{}` or
  duplicate-selected, with `_id` stripped on duplicate); delete with a
  per-document confirmation dialog (tasks 5.2-5.4).
- Collection browser with a working find/filter bar (`mongoDocumentHelpers.ts`,
  `DbClientView.tsx`): reuses the document editor's "must be a JSON object"
  validation for filter input; a blank string (not `'{}'`) is the canonical
  "no filter" value, matching the existing server-side
  `decodeMongoJSONObject` convention; applying a filter resets pagination to
  `skip=0`, and switching database/collection clears the active filter (task
  5.5).
- MongoDB Schema Diagram (`internal/diagram/mongo.go`): samples N documents
  per collection and infers field shapes, reporting **every observed type**
  for a field rather than collapsing type variance to "mixed" (a deliberate
  teaching-oriented choice per spec.md's framing of this feature) — e.g. a
  field that's a string in some documents and an int in others renders as
  `int_or_string`; optionality and explicit `null` are tracked as two distinct
  signals. Nested objects flatten into the same Mermaid entity block
  (`address.street` → `address_street`); reuses the relational Schema
  Diagram's Mermaid renderer (`MermaidDiagram.tsx`) rather than a second
  rendering component. No PK/FK markers are ever emitted for Mongo attributes,
  and the exact phrase **"Inferred structure - not an enforced relationship"**
  is baked into both the on-screen badge and the raw generated Mermaid text
  itself (as a `%%` comment banner), so the caveat survives into a copied/
  exported diagram, not just the UI. Verified both via exact-string Go tests
  and by feeding the generated text through Mermaid's real `mermaid.parse()`
  in Node (task 5.6).

This completes **Phase 5 — MongoDB** (tasks 5.1-5.6) and, together with
Phases 3/4/4.5, delivers all of **Module 2 — DB Client** (spec.md §4) except
Redis, which is Phase 6's job.

### Fixed

- MongoDB auth/`authSource` conflict: `MongoConnectionString`'s database-path
  segment doubles as the driver's SCRAM `authSource`. Setting `svc.DBName` to
  a non-admin value while authenticating as the `MONGO_INITDB_ROOT_USER`
  (which only exists in the `admin` database) failed authentication. Fixed by
  leaving `DBName` nil for container creation (matching `mongodb.go`'s
  already-documented Phase 2 fallback) while still exercising a separate named
  database for document operations, since Mongo creates databases lazily on
  first write (task 5.1).
- React 18 StrictMode session-lifecycle race: the Mongo session was opened
  eagerly in `DbClientView` but only closed in `MongoDocumentView`'s unmount
  effect — StrictMode's dev-only double-invoke of effects (mount→cleanup→
  mount) closed the session immediately after opening it, so the "real" mount
  then tried to list databases against an already-closed session. Fixed by
  having `MongoDocumentView` open **and** close its own session within one
  effect, so StrictMode's synthetic cycle opens/closes a throwaway session and
  the real mount's session is the one actually closed on real unmount — the
  same pattern that already made `QueryEditor` StrictMode-safe, adapted here
  for Mongo's eager (not lazy) session-opening need (tasks 5.2-5.4).
- Docker-integration test container-ID collisions: three of Phase 2's
  parallel tasks independently picked the same hardcoded
  `testProfileID`/`testServiceID` constant (`999002`), and a later pick
  collided with another file's `999003` — there was no central registry
  for these IDs. Reassigned each integration test file a unique ID
  (`999001`-`999009` across `compose_integration_test.go`,
  `lifecycle_integration_test.go`, `redis_integration_test.go`,
  `mysql_integration_test.go`, `mongodb_integration_test.go`,
  `profile_multiengine_integration_test.go`,
  `reset_volume_integration_test.go`); no automated guard against future
  collisions exists yet — see `docs/STATE.md`.
- MySQL DSN construction (task 3.4): forcing `cfg.ParseTime = true` while
  also copying a pasted `?parseTime=false` into `go-sql-driver/mysql`'s
  `Config.Params` produced a DSN with `parseTime` appearing twice —
  `FormatDSN()` writes the struct field first and `Params` (sorted
  alphabetically) after, so the second occurrence silently won on
  re-parse, undoing the forced `true`. Fixed by stripping any
  `parseTime` key from `Params` before copying it in.
- `ListConnections()` returned Go's `nil` for an empty slice, which
  JSON-encodes to `null` and crashed the frontend on
  `savedConnections.length` (task 3.5). Fixed by normalizing to an empty
  slice before returning — the second occurrence of this exact
  nil-slice-serializes-to-`null` pattern in this project (first in
  `ListProfiles`'s `ProfileSummary` wrapping, Phase 2).
- Monaco defaulted to CDN loading (task 3.6): `@monaco-editor/react`'s
  default loader fetches Monaco from `cdn.jsdelivr.net` at runtime, a
  silent violation of spec.md §5's local-only NFR. Fixed by installing
  `monaco-editor` directly and adding
  `frontend/src/lib/monacoSetup.ts` to wire the base editor worker and
  call `loader.config({monaco})` before any `<Editor>` mounts — verified
  via captured network traffic showing zero external requests during a
  full manual test pass.
- Cross-project TypeScript build blocker (task 4.5.2): installing
  `mermaid` pulled in `@types/d3-dispatch` as a transitive dependency
  using TS 5.0+-only syntax that this project's pinned
  `typescript@4.6.4` cannot parse — broke `tsc` for the **whole**
  project, not just the schema-diagram code. Fixed via a
  `pnpm-workspace.yaml` `overrides` entry pinning `@types/d3-dispatch` to
  `3.0.1`. Same root cause as the already-known `@types/node@26`/vitest
  issue from task 3.7 (a transitive dependency's types using newer TS
  syntax than the pinned compiler) — worth resolving categorically (e.g.
  bumping `typescript` itself) if this keeps recurring rather than
  patching one `overrides` entry at a time.
- Semicolons inside string literals broke the Query Editor for ordinary
  single statements once "Run query" started routing through the new
  multi-statement path — `INSERT INTO widgets (name) VALUES ('hello;
  world')` mis-split into two broken fragments. Fixed with a byte-level
  quote-tracking scanner in `internal/dbengine/batch.go`'s
  `SplitStatements` that does not split inside a single- or
  double-quoted region and treats a doubled quote (`''`/`""`) as an
  escaped literal, not a close.
- Compatible-engine snippet filtering was implemented at the storage
  layer (task 4.6) but never reached the UI — `SnippetsPanel.tsx` always
  requested the unscoped snippet list, so a global Postgres snippet
  stayed visible and runnable with only a MySQL tab open. Fixed by
  deriving the active tab's connection/engine and passing it through to
  the existing `ListSnippets` bound method. A second, more subtle bug
  surfaced while fixing this: the bound method's scoping gate used
  `ConnectionID != 0`, which is ambiguous (also true for a legitimate
  ad-hoc/never-saved connection) — corrected to gate on `Engine != ""`
  instead.

### Changed

- Retroactive comment-style cleanup across all Go and frontend source
  written in Phases 0-1, per the new `CLAUDE.md` convention: only package
  doc comments and doc comments on exported functions/types/consts
  survive; every inline comment removed. No logic, signatures, or JSX
  changed; `go build`/`go vet`/`go test` and `pnpm run build` stayed green
  throughout. Rationale that was previously inline is preserved in
  `docs/STATE.md`.
- **Breaking:** `dbengine.QueryResult.Columns` changed from `[]string` to
  `[]ResultColumn{Name, DatabaseType, Nullable *bool}` (task 3.7), to
  carry real per-column type metadata into the results grid. This
  rippled through `engine.go`, both Postgres/MySQL implementations and
  their tests, and the generated `frontend/wailsjs/go/models.ts` — the
  full ripple was independently re-verified by a fresh-context
  adversarial reviewer (repo-wide grep for `.Columns\b`), not just
  trusted from the implementing task's own report.
