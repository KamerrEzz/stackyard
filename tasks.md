# Stackyard — Task Breakdown

Status: DRAFT — pending approval
Depends on: `spec.md`, `plan.md` (both approved before work starts)

Each task targets a single 2-4h session and should leave the app in a
runnable state. Update `docs/STATE.md` at the end of every session
regardless of whether a task fully closes.

---

## Phase 0 — Toolchain & Shell

- [x] **0.1** Install Go, Node/pnpm, Wails CLI; scaffold project with
      `wails init` (React-TS template); confirm `wails dev` opens a window.
- [x] **0.2** Set up Tailwind in the Wails frontend; build the app shell
      (sidebar nav: Environments / DB Client, top bar, dark mode as the
      only theme for v1).
- [x] **0.3** Add one trivial bound Go method (e.g. `App.Ping() string`)
      called from a React button, confirming the full IPC round trip and
      Wails' generated TS bindings work end-to-end.
- [x] **0.4** Set up `internal/storage` with `modernc.org/sqlite`; create
      the schema from `plan.md` §4 via a migration/init script run on
      first launch; verify the DB file lands in the OS app-data path.
- [x] **0.5** Create `docs/STATE.md` and write the first entry (empty
      baseline: what's proven to work, what command runs the app).

## Phase 1 — Environment Manager MVP (Postgres only)

- [x] **1.1** `internal/docker/client.go`: wrap `docker/docker/client`,
      confirm connectivity to the local Docker Engine from Go (list
      containers) — validate Windows named-pipe access specifically.
- [x] **1.2** Define the `Profile`/`Service` Go structs and their SQLite
      persistence (create/read/update/delete for a Postgres-only profile).
- [x] **1.3** `internal/docker/compose.go`: given one Postgres `Service`,
      programmatically create network + volume + container (equivalent of
      `docker run`, no compose file ever written to disk).
- [x] **1.4** Bind start/stop/restart methods on `App`; wire a minimal
      React profile list + "Start"/"Stop" buttons.
- [x] **1.5** Port-conflict pre-check before start; surface a suggested
      free port in the UI instead of a raw Docker error.
- [x] **1.6** Connection-string generator for Postgres + one-click
      clipboard copy with a confirmation toast.
- [x] **1.7** Manual pass: time the "select profile → Start → copy
      connection string" flow and confirm it meets the 3-click criterion
      (spec.md §3.2); adjust UI if it doesn't.

## Phase 2 — Environment Manager, Full

- [ ] **2.1** Extend `Service` config + container creation for MySQL.
- [ ] **2.2** Extend for MongoDB.
- [ ] **2.3** Extend for Redis.
- [ ] **2.4** Profile creation wizard supporting any combination of the 4
      engines in one profile (multi-service start/stop as a unit).
- [ ] **2.5** Profile duplicate/rename/delete in the UI, backed by 1.2's
      persistence layer.
- [ ] **2.6** "Reset volume" for a single service: stop → remove volume →
      leave recreated fresh on next start; explicit confirmation dialog;
      verify sibling services in the same profile stay running throughout.
- [ ] **2.7** `internal/docker/stats.go`: poll CPU/RAM per container via
      the Docker stats API.
- [ ] **2.8** Real-time status dashboard: all profiles/services, state,
      port, CPU/RAM, refreshed via Wails events (not frontend polling);
      confirm it reflects containers started/stopped outside the app.

## Phase 3 — DB Client MVP (Postgres + MySQL)

- [ ] **3.1** `internal/dbengine/engine.go`: define the `Engine` interface
      (Connect, Ping, Query, ListSchemas, ListTables, Close).
- [ ] **3.2** Implement the interface for Postgres (`pgx`) and MySQL
      (`go-sql-driver/mysql`).
- [ ] **3.3** `urlparse.go`: parse the 4 connection-string formats into
      form fields; unit-test malformed-string error messages.
- [ ] **3.4** Connection form UI: paste-URL autofill + manual fields +
      "Test connection" button.
- [ ] **3.5** Saved connections list backed by the `connections` table;
      persist across restarts.
- [ ] **3.6** Integrate Monaco editor (`@monaco-editor/react`) with SQL
      syntax highlighting; wire "Run query" to `Engine.Query`.
- [ ] **3.7** Read-only results grid rendering query output (types,
      pagination) for both engines.
- [ ] **3.8** Multi-tab shell: open/close tabs, each bound to one
      connection + one editor + one result pane; verify independence
      between tabs (spec.md §4.2).

## Phase 4 — Relational DB Client, Complete

- [ ] **4.1** Editable grid: in-place cell edit → `UPDATE` by primary key;
      read-only fallback + visible reason for PK-less tables.
- [ ] **4.2** Grid row insert (blank row bound to column defaults/types).
- [ ] **4.3** Grid row delete with confirmation for multi-row deletes.
- [ ] **4.4** Inline error surfacing: failed writes show the DB's actual
      error message on the offending cell/row.
- [ ] **4.5** Query history: log every execution to `query_history`;
      build the filterable/searchable history panel; "replay into new
      tab" action.
- [ ] **4.6** Snippets CRUD (name, tags, connection-scoped or global);
      snippet search by name/tag.
- [ ] **4.7** "Run snippet" loads it into the current tab, or a new tab if
      the current one is dirty.
- [ ] **4.8** Autocomplete: table/column suggestions in Monaco sourced
      from `ListSchemas`/`ListTables`.

## Phase 4.5 — Schema Diagram (Relational)

Owned by the `erd-builder` subagent; can run in parallel with the rest of
Phase 4 once Phase 3's `ListSchemas`/`ListTables` exist — it shares no
code surface with the editable-grid work above.

- [ ] **4.5.1** Extend the `Engine` interface with `ListForeignKeys`
      (Postgres + MySQL) to obtain relationship metadata, not just
      tables/columns.
- [ ] **4.5.2** `internal/diagram`: function that translates schema + FK
      metadata into valid Mermaid `erDiagram` text.
- [ ] **4.5.3** Frontend Mermaid rendering component with zoom/pan.
- [ ] **4.5.4** Export to PNG/SVG and copy raw Mermaid text to clipboard.
- [ ] **4.5.5** "Regenerate" button; legibility pass at projector scale
      (validate a minimum legible font size).

## Phase 5 — MongoDB

- [ ] **5.1** Implement `Engine` for MongoDB (official `mongo-go-driver`);
      map its query model onto the existing tab/connection shell.
- [ ] **5.2** Document tree/JSON viewer component (expand/collapse nested
      objects and arrays, typed scalar rendering).
- [ ] **5.3** In-place document editing with JSON-structure validation
      before save.
- [ ] **5.4** New document creation (blank `{}` or duplicate-selected) and
      delete-with-confirmation.
- [ ] **5.5** Collection browser (list collections, basic find/filter bar)
      wired into the multi-tab shell.
- [ ] **5.6** Schema Diagram — MongoDB inferred structure: sample N
      documents per collection, infer shape, render reusing Phase 4.5's
      Mermaid component with the visual "inferred, not an enforced
      relationship" label.

## Phase 6 — Redis

- [ ] **6.1** Implement `Engine` for Redis (`go-redis/redis`); key-space
      scan (pattern-based filtering, e.g. `session:*`).
- [ ] **6.2** Per-type detail views: string, hash, list, set, sorted set.
- [ ] **6.3** TTL display and edit (set/persist/change) per key.
- [ ] **6.4** Key rename and delete with confirmation.

## Phase 7 — Import / Export

- [ ] **7.1** CSV export for a full table and for a query result set,
      type-preserving (dates/numbers/nulls distinguishable).
- [ ] **7.2** JSON export, same two scopes.
- [ ] **7.3** SQL dump export (`CREATE TABLE` + `INSERT`) for Postgres and
      MySQL; round-trip-tested against a fresh instance.
- [ ] **7.4** Import: CSV/JSON with pre-commit validation against target
      table columns; abort-before-write on mismatch.

## Phase 8 — Migrations (Postgres + MySQL)

- [ ] **8.1** `internal/migrations`: scaffold "create migration" (paired
      timestamped up/down files) tied to a connection profile's chosen
      folder.
- [ ] **8.2** `schema_migrations` tracking-table bootstrap inside the
      target database on first use (plan.md §4).
- [ ] **8.3** "Apply": run all pending migrations in order; verify a
      mid-run failure leaves tracking state accurate and surfaces the DB
      error.
- [ ] **8.4** "Rollback": revert exactly one migration step.
- [ ] **8.5** Migrations UI panel: pending/applied list, apply/rollback
      actions, per-connection scoping.

## Phase 9 — Polish & Ship v1

- [ ] **9.1** Performance pass: idle memory footprint and cold-start time
      measured and recorded against the NFR bar (spec.md §5).
- [ ] **9.2** Visual polish pass across both modules against the "not
      generic/AI-template" bar — typography, spacing, dark-mode contrast.
- [ ] **9.3** Windows installer build via Wails' packaging; smoke-test a
      clean install on a machine without the dev toolchain.
- [ ] **9.4** Dogfood run: replace your own next new-project setup with
      Stackyard end-to-end (spec.md §7 success definition); log friction
      points to `docs/STATE.md` as a v1.1 backlog, not mid-flight scope
      creep.
