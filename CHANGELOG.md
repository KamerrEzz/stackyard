# Changelog

All notable changes to Stackyard are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [1.0.0] - 2026-07-03

First public release.

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

- Redis Engine (`internal/dbengine/redis/redis.go`, official `go-redis/v9`): a
  key-value-oriented surface deliberately separate from both the relational
  `dbengine.Engine` interface and the Mongo engine — supports all 5 Redis
  data types (string, hash, list, set, sorted set). Cursor-based `SCAN` for
  pattern-based key-space scanning (never the blocking `KEYS` command);
  per-type paginated reads (`LRANGE` for lists, `SSCAN` for sets, `ZRANGE
  WITHSCORES` for sorted sets); TTL read/set/persist with `-1`/`-2` sentinel
  translation; key rename (guarded by an `EXISTS` check first) and multi-key
  delete via one `DEL` call. `app.go` gained a third parallel session map
  (`redisSessions`), alongside the existing SQL (`querySessions`) and Mongo
  (`mongoSessions`) maps — still no attempt to unify all three into one
  polymorphic abstraction (task 6.1).
- Redis key browser and per-type detail views (`RedisKeyBrowser.tsx`,
  `RedisKeyDetail.tsx`, `RedisValueViews.tsx`, `redisKeyHelpers.ts`):
  pattern-driven key scan with real cursor-based "Load more" pagination, a
  checkbox multi-select key list with confirmation-gated multi-key delete,
  and one detail view per Redis type (string/hash/list/set/sorted-set) with
  TTL display/edit/persist and rename. `DbClientTab` extended to a three-way
  discriminated union (`SqlTab | MongoTab | RedisTab`), reusing the exact
  pattern the Mongo tab established in Phase 5 — `TabBar`/`tabState.ts`
  needed zero changes (tasks 6.2-6.4).

This completes **Phase 6 — Redis** (tasks 6.1-6.4) and, together with Phases
3/4/4.5/5, delivers all of **Module 2 — DB Client** (spec.md §4) for every
engine: Postgres, MySQL, MongoDB, and Redis.

- Export (`internal/export/`, `export.go`): CSV/JSON/SQL-dump export for
  both a full table and the current query result, with a user-selectable
  scope. A shared, engine-agnostic formatting layer
  (`ToCSV`/`ToJSON`/`ToSQLDump`, taking only `(columnNames []string, rows
  [][]any)`, no DB dependency) feeds two entry points: full-table export
  pages through `Engine.Exec` directly at 1000 rows/page (deliberately
  bypassing `BrowseTableRows` to avoid spamming `query_history` with one
  entry per internal page), and current-query-result export reuses data
  the frontend already holds from `RunQuery`/`RunMultiStatementQuery` (Go
  keeps no last-result cache, avoiding a second cache that could drift
  from what's on screen). CSV's NULL-vs-empty-string convention is
  exactly Postgres's own `COPY ... CSV` convention (unquoted-empty field =
  NULL, quoted `""` = empty string), chosen because it's unambiguous to
  reverse on import; JSON needs no such convention since `null`/`""` are
  already distinguishable via JSON's own grammar. SQL dump is scoped to
  full-table export only (a query result can join multiple tables with no
  single `CREATE TABLE` target), with per-engine `CREATE TABLE` type
  mapping (Postgres's `information_schema.columns.data_type` is valid DDL
  as-is; MySQL needs an added `COLUMN_TYPE` lookup for length/precision)
  and per-engine escaping (single quotes doubled for both; backslashes
  additionally escaped for MySQL only, matching its default `sql_mode` so
  a trailing backslash can't swallow the closing quote), INSERTs batched
  at 500 rows/statement. Round-trip tested for real: a dump generated
  from a live seeded table was executed against a genuinely separate
  fresh container of the same engine and the resulting rows compared to
  the source via exact string equality, including an explicit
  NULL-vs-`''` fidelity check — verified for both Postgres and MySQL, zero
  Docker leftovers after. File save uses `runtime.SaveFileDialog`
  (`ExportControls.tsx`), first use in this codebase (tasks 7.1-7.3).
- Import (`internal/importdata/`, `import.go`, `ImportDialog.tsx`):
  CSV/JSON import with pre-commit validation against the target table's
  columns, collecting every mismatch across the whole file before
  reporting rather than stopping at the first one, and a hard block on
  any mismatch — no partial commits. `ImportFile` fully re-validates from
  scratch immediately before writing, regardless of any prior
  `ValidateImportFile` call, so there's no window where a stale
  validation result could be trusted. Uses a bulk single-statement INSERT
  (not N calls to `InsertTableRow`), atomic on both Postgres and
  MySQL/InnoDB, so abort-before-write holds even against DB-level
  constraints (UNIQUE/CHECK) the validator itself can't see. A custom CSV
  tokenizer is used instead of stdlib `encoding/csv`, since the standard
  library discards the quoting information needed to distinguish an
  unquoted-empty NULL from a quoted-empty `""` string on the way back in.
  Type-plausibility validation categorizes `ColumnInfo.DataType` against
  Postgres/MySQL's `information_schema` vocabulary into
  integer/numeric/boolean/datetime/text buckets (unknown types always
  pass rather than being rejected); a known gap is that MySQL reports
  `BOOLEAN` as `tinyint`, indistinguishable from a genuine tinyint column,
  so only `0`/`1` passes there, not `"true"`/`"false"`. Verified for real:
  an integration test seeded a file with one deliberately bad row among
  several good ones and confirmed via `SELECT COUNT(*)` that zero rows
  landed, not "all but the bad one" (task 7.4).

This completes **Phase 7 — Import/Export** (tasks 7.1-7.4), cutting
across every engine already built.

- `internal/migrations`: migration file scaffolding — "Create migration"
  generates a timestamped up/down SQL file pair
  (`<14-digit UTC timestamp>_<slug>.up.sql`/`.down.sql`);
  `DiscoverMigrations` sorts strictly by parsed version (never file
  mtime) and hard-errors on an incomplete up/down pair (task 8.1).
- `schema_migrations` tracking-table bootstrap inside the target
  database (Postgres/MySQL): one dialect-neutral
  `CREATE TABLE IF NOT EXISTS` via `Engine.Exec` shared by both engines;
  `Connection.MigrationsFolder` added as SQLite schema version 2 via a
  dedicated `SetConnectionMigrationsFolder` setter, mirroring the
  existing `LastUsedAt` isolation pattern so nobody silently clobbers it
  through a generic update (task 8.2).
- "Apply": runs all pending migrations in version order, with each
  migration's schema change and its `schema_migrations` tracking row
  committed atomically together via a new optional
  `dbengine.Transactor` interface (real per-connection transactions for
  both `pgxpool` and `database/sql`) — added as a separate, optional
  interface rather than a breaking change to the shared `Engine`
  interface, so existing test doubles and the Mongo/Redis engines
  (which don't implement `Engine` at all) are unaffected. A mid-run
  failure stops immediately: verified against real containers that an
  already-applied migration's schema change and tracking row both land,
  the failing migration's schema change does NOT land and has no
  tracking row, and later pending migrations are never attempted (task
  8.3).
- "Rollback": reverts exactly one migration step, most-recently-applied
  first — verified against real containers across 3 sequential calls;
  returns `(nil, nil)` rather than a sentinel error when nothing is left
  to roll back, since Wails serializes Go errors to plain strings and a
  nil check is simpler for the frontend than string-matching an error
  message (task 8.4).
- Migrations UI panel (`MigrationsView.tsx`, a new top-level sidebar
  item scoped to a saved Postgres/MySQL connection record, not an
  ad-hoc DB Client tab): native OS folder-picker
  (`PickMigrationsFolder`), pending/applied status per migration
  computed by merging `ListMigrations`' file list against
  `ListAppliedMigrationVersions`, "Apply pending migrations"/"Rollback
  last migration" actions with a confirmation dialog on Rollback
  (matching this project's established destructive-action pattern), and
  the Rollback button disabled whenever nothing is applied. Manually
  verified end-to-end against a real Postgres container, including
  direct `\dt`/`schema_migrations` queries confirming the database
  itself (not just the UI) reflects Apply/Rollback correctly (task 8.5).

This completes **Phase 8 — Migrations** (tasks 8.1-8.5) for Postgres and
MySQL.

### Added

- Performance pass (task 9.1): production-build (`wails build`, not
  `wails dev`) cold-start and idle-memory measurements against spec.md
  §5's NFR bar, multiple runs each with raw per-run numbers recorded, not
  just averages. Cold-start steady-state average ≈ 913ms (6 runs,
  897.5-928.7ms); a 7th run showed a 5.5s outlier on the very first
  launch of a freshly built binary, attributed to a one-time
  Windows Defender/SmartScreen scan (confirmed non-representative by
  rebuilding and re-timing a second binary's own first launch, which came
  back at 924ms). Idle memory (45s settle, 3 runs each): main
  `stackyard.exe` process alone ≈ 58MB working set / 72MB private —
  genuinely light, this is the part of the footprint that is actually
  Stackyard's own code; the full resident process tree (main process +
  6 WebView2 Chromium-model helper processes: browser host, GPU, network
  service, crashpad, renderer) totals ≈ 407MB working set / 267MB
  private. Reported honestly rather than spun: this full-tree total sits
  within-to-above the "150-300MB+" Electron-class range spec.md's NFR
  bar is framed against — not a code bug, but a platform characteristic
  of WebView2's shared multi-process architecture, structurally the same
  Chromium multi-process model Electron itself uses. No speculative
  code changes were made; the app's one existing poller
  (`StatusDashboard`'s 1.5s status watcher) was confirmed already scoped
  to component mount/unmount, not running by default at idle.
- Visual polish pass (task 9.2): a cross-module read of every `.tsx` file
  in both modules plus the shared shell and design tokens, comparing
  visually-equivalent elements (buttons, section headers, form-field
  labels) across modules for drift against the "not generic/AI-template"
  bar. Confirmed the codebase was already highly consistent (button
  variants, card padding, border-radius, semantic status colors) despite
  incremental multi-session authorship; unified three real one-off
  styling inconsistencies found during the pass — see Fixed below for the
  bug this pass also caught.
- Dogfood run (task 9.4): personally drove spec.md §7's full
  success-definition flow end-to-end through the app's own UI only (no
  `docker`/`psql` CLI, no Docker Desktop, no external DB client) — create
  a profile (3 clicks, ~1.6s to "Running"), connect via the DB Client,
  run real `CREATE TABLE`/`INSERT`/`SELECT` queries against the live
  container, save and run a snippet, tear down with an honest
  volume-preservation disclosure in the delete confirmation dialog. Every
  step genuinely exercised, not simulated. Surfaced three small
  friction points, logged as a v1.1 backlog rather than fixed mid-flight
  per this task's own instruction: saved connections have no
  uniqueness guard on name (repeated saves silently create duplicate
  rows); the saved-connection row visually suggests it's clickable
  (name, connection string, list-item layout) but only the small "Load"
  button actually does anything — the row itself has no click handler;
  Query History
  requires a manual refresh to show queries just run in the same session
  (consistent with this app's existing no-live-polling design elsewhere,
  not itself a bug).

- **Windows installer (task 9.3)**: NSIS installed by the user (an
  elevated terminal was required for `winget install --id NSIS.NSIS -e`,
  which this session couldn't approve non-interactively on its own —
  see the "blocked" account below, now resolved). `wails build -nsis`
  produced `build/bin/stackyard-amd64-installer.exe` (~16.6 MiB). The
  installer itself also requests admin elevation by default
  (per-machine install into `Program Files`), so the user ran it
  directly rather than the agent attempting to bypass that boundary
  too. Verified afterward: the installed directory
  (`C:\Program Files\Kamerr Ezz\Stackyard\`) contains only
  `stackyard.exe` (byte-identical to the dev-built binary) and
  `uninstall.exe` — no dev-only path dependency of any kind, confirming
  the frontend is fully embedded via `go:embed`. Launched the installed
  executable directly and confirmed it starts and runs correctly.

Task 9.3's original blocker (kept here for the historical record — now
resolved above): NSIS was not installed on this machine, and its
installer requires interactive administrator elevation (a UAC consent
prompt) that this session couldn't approve on its own. A non-interactive
`winget install --id NSIS.NSIS -e` was attempted, verified safe (official
package, checksum-verified installer), and stalled at the elevation
prompt — the hung process was killed and confirmed to have left zero
partial/half-installed state behind. `wails.json` was prepared with an
`info` block (company/product name, version, copyright) so the NSIS
template has real values once a build is possible, keeping
`productVersion` at `"0.0.0"` pending this project's still-unresolved
real versioning decision.

### Fixed

- Missing `ink-500`/`ink-300` Tailwind color-scale shades (task 9.2):
  `frontend/tailwind.config.ts`'s custom `ink` color scale defined only
  `950/900/850/800/700/600/400/200/100` — `ink-500` and `ink-300` were
  never defined, despite being referenced 75+ times across 17 files
  (`text-ink-500`/`text-ink-300` and their `bg-`/`border-` variants).
  Tailwind has no fallback for a custom-named color family the way it
  does for built-in palettes, so every one of those classes silently
  compiled to nothing app-wide — the intended muted/tertiary text tier
  for hints, placeholders, badges, and secondary annotations across
  nearly every module was un-styled with no build error or visual
  warning. Fixed by adding both shades, linearly interpolated between
  their existing neighbors (`ink-300` = midpoint of `ink-200`/`ink-400`;
  `ink-500` = midpoint of `ink-400`/`ink-600`) so the fix restores every
  existing callsite's originally-intended appearance from one config
  change rather than touching 75+ individual classNames.
- Three smaller one-off styling inconsistencies unified during the same
  pass (task 9.2), all `className`-only, zero logic/state/props changes:
  `MongoDocumentView.tsx`'s three form-field labels used a smaller/dimmer
  typographic tier than every other bound form-field label in the app;
  `ImportDialog.tsx`'s modal `<h2>` used a one-off larger/bolder/brighter
  size than every other panel/section header; `ImportDialog.tsx`'s
  "Confirm import" button used a bordered green-outline variant found
  nowhere else in the app as a static button style (emerald elsewhere is
  reserved for transient success feedback) instead of the filled-brass
  primary style every other CTA in the app uses, including its own
  sibling button in the same dialog.
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
- Wails v2.12.0 bound methods silently drop a 3rd return value: the vendored
  dispatcher (`internal/binding/boundMethod.go`'s `OutputCount()` switch) only
  implements `case 1`/`case 2`, with no `case 3` and no `default`. A bound
  method declared with 3 return values (originally `ScanRedisKeys(...)
  ([]string, uint64, error)`) compiled cleanly, ran correctly server-side, and
  even appeared correctly typed in the generated `.d.ts` — but the JS caller
  silently received `undefined` with no error, regardless of what actually
  happened. No build error, no runtime panic, no console error. Caught by
  reading Wails' own vendored source directly, before any frontend code
  depended on the broken method. Fixed by wrapping the extra return value in
  a small result struct (`ScanKeysResult{Keys, NextCursor}`,
  `RedisSetPage{Members, NextCursor}`) to keep every bound method's
  `OutputCount()` at 2; the underlying `redis.Engine` methods kept their
  plain 3-value Go signatures unchanged — only the `App`-bound wrapper layer
  needed the struct. **Standing rule for any future bound method: never
  return more than 2 values (data + error) — wrap additional data in a
  struct instead** (task 6.1).
- `isImportableFilePath` dead-code cleanup: this file-extension check was
  written and unit-tested during task 7.4 but never actually called from
  anywhere. Wired into `ImportDialog.tsx`'s file-pick handler as a
  defensive client-side extension check — the OS file dialog already
  filters by extension, but a user can override that filter to "All
  files," so the helper now actually gates the UI instead of being inert.
- Docker-integration test host-port collisions across packages (Phase
  8): two new integration test files correctly grepped the established
  `9990\d\d` test-ID convention, but each also picked a hardcoded host
  port that was never cross-checked against other packages' tests — a
  separate, previously-unchecked number space. Running the full
  integration suite concurrently (`go test -tags=integration ./...`)
  surfaced flaky "port is already allocated" failures in unrelated
  tests (`TestIntegration_App_EditableGrid_Postgres`,
  `TestIntegration_MySQLEngine_ForeignKeys`, among others). Fixed by
  reassigning the 4 colliding ports to verified-free ones. **Standing
  rule for any future integration test in this repo: grep both the
  `9990\d\d` test-ID convention AND `HostPort\s*=\s*\d+` literals before
  picking values — they are separate number spaces and checking one does
  not cover the other.**

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

## Phase 10 — v1.1: user-requested enhancements (post-v1 scope)

Genuinely new scope requested by the user after real hands-on use of the
shipped v1 build — not part of the original `spec.md`/`plan.md` v1
definition. Each item was clarified with the user before implementation
(see `docs/STATE.md`'s Phase 10 sections for the full clarification
trail).

### Added

- Environments: optional custom username/password/database-name fields
  on the "Create profile" form for Postgres/MySQL/MongoDB, set once at
  creation time and fixed afterward — deliberately not live credential
  rotation on an already-running container, since Postgres/MySQL don't
  expose an easy way to change the bootstrap superuser's password
  without a stop + recreate + likely volume reset. Redis rejects
  username/database-name (no such concept for Redis) but still allows a
  custom password (task 10.1).
- DB Client "Create table" UI: a form (table name + columns, each with
  type/nullable/primary-key/default) that generates and runs a real
  `CREATE TABLE` against Postgres/MySQL. `internal/dbengine/createtable.go`'s
  `BuildCreateTableDDL` uses a curated column-type list
  (text/varchar(255)/integer/bigint/serial/bigserial/boolean/timestamp/
  numeric) with per-engine DDL handling (Postgres `SERIAL`/`BIGSERIAL`
  vs. MySQL `AUTO_INCREMENT`, `TIMESTAMP` vs. `DATETIME`, etc.) (task
  10.2).
- DB Client SQL snippet template gallery: three built-in templates (auth
  — users + sessions + tokens; audit log; settings key-value), each
  loadable directly into the query editor or saveable as a real user
  snippet with one click, Postgres/MySQL. MySQL's `audit_log.metadata`
  column intentionally differs in nullability from Postgres's — MySQL
  pre-8.0.13 cannot take a literal JSON `DEFAULT` value (task 10.3).
- DB Client schema export: export an existing connection's schema as a
  real `schema.prisma` file (task 10.4) or a real Drizzle `schema.ts`
  file (task 10.5), both built on the existing `ListTables`/
  `ListForeignKeys` introspection with no new schema-reading code. New
  `internal/schemaexport` package (`prisma.go`, `drizzle.go`,
  `typemap.go`, `identifiers.go`) renders foreign keys as Prisma relation
  fields (with back-relations) and Drizzle `.references(() => ...)`
  calls; composite (multi-column) foreign keys are deliberately not
  merged into a single relation, matching the existing Schema Diagram
  generator's own documented limitation. Exposed as
  `ExportSchemaAsPrisma`/`ExportSchemaAsDrizzle` bound methods, reachable
  from a new per-schema export action pair in `QueryEditor.tsx`'s Tables
  panel. Documented lossy judgment calls: MySQL's `tinyint`-as-boolean
  convention isn't detected, Postgres array columns and MySQL
  varchar/char real lengths fall back to generic defaults, `bigint`
  defaults to Drizzle's `{ mode: "number" }`, and column defaults are
  never emitted in either target (`ColumnInfo` has no default-value
  field to translate).

This completes **Phase 10** (tasks 10.1-10.5) — see `docs/STATE.md` for
why this phase does not map to a `plan.md` §6 roadmap phase and is not
tied to a version-bump tag the way Phases 1-9 were.

### Fixed

- Integration-test ID collision between two of Phase 10's four parallel
  work streams: `schemaexport_integration_test.go` and
  `internal/snippettemplates/templates_integration_test.go` both
  independently used profile/service IDs 999033/999034, causing
  intermittent "connection actively refused" failures on a full
  `go test -tags=integration ./...` rerun that were initially
  (incompletely) attributed to transient Docker/WSL2 resource
  contention. Found by grepping the literal ID-assignment pattern
  (`int64 = 9990\d\d`) rather than any occurrence of the digit sequence,
  which can't distinguish one file reusing its own ID from two files
  colliding. Fixed by reassigning `schemaexport_integration_test.go` to
  999035/999036 (its host ports were already unique); confirmed stable
  across repeated back-to-back full integration-suite runs afterward.
