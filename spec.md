# Stackyard — Functional Specification

Status: DRAFT — pending approval
Version: v1 (MVP scope)

## 1. Problem Statement

Every time a new project starts, the developer manually writes or edits
`docker-compose.yml` files, tracks down which ports are free, remembers
passwords/usernames per project, and opens a separate GUI client to inspect
the resulting databases. This is repetitive, error-prone, and breaks flow.

Stackyard is a native desktop application that owns this entire loop: spin
up a database environment without touching YAML, then browse/query that
data without leaving the app.

## 2. Goals (v1)

1. Start a fully working local database environment (1-4 engines) in **3
   clicks or fewer**, with zero manual Docker Compose editing.
2. Connect to and operate on MySQL, PostgreSQL, MongoDB, and Redis from a
   single, coherent UI — no per-engine tool switching.
3. Everything runs 100% locally: no cloud backend, no telemetry, no
   account/login.
4. The codebase and documentation stay resumable after multi-week gaps
   between work sprints (5-10h/week cadence).

## 3. Module 1 — Environment Manager

### 3.1 Profiles

**Feature:** A profile is a named, reusable set of services (any
combination of MySQL, PostgreSQL, MongoDB, Redis), each with its own
configuration (image/version, host port, credentials, initial DB/schema
name, volume name).

Acceptance criteria:
- Given no existing profiles, the user can create a new profile by naming
  it and adding 1+ services with a form (no raw YAML/JSON editing exposed).
- A profile persists locally and survives an app restart.
- A profile can be duplicated, renamed, and deleted.
- Deleting a profile does not silently delete its Docker volumes; the user
  is asked explicitly (see 3.4).

### 3.2 Environment Lifecycle (start/stop/restart)

**Feature:** Start, stop, and restart all services in a profile as a unit,
or individually per service.

Acceptance criteria:
- Starting a profile that has never run before creates the required Docker
  resources (containers, network, named volumes) automatically —
  equivalent to an implicit `docker compose up -d`, but the user never
  sees or edits a compose file.
- **3-click success criterion:** for an already-configured profile, the
  path "open app → select profile → click Start" is the complete flow
  (2 clicks after launch). For a brand-new environment using a built-in
  engine default (no custom ports/passwords needed), "select engine
  template → name it → Start" is 3 clicks total.
- Port conflicts are detected before start and surfaced with a suggested
  free port, not a raw Docker error.
- Stop/restart apply within a visible time budget with clear per-service
  status feedback (starting/running/stopping/stopped/error).

### 3.3 Connection Strings

**Feature:** Auto-generate the connection string for every running
service, in each engine's canonical URL format.

Acceptance criteria:
- Format matches `mysql://user:pass@host:port/db`,
  `postgres://user:pass@host:port/db`, `mongodb://user:pass@host:port/db`,
  `redis://[:pass@]host:port[/db]`.
- One click copies the string to the clipboard; a toast/inline confirmation
  acknowledges the copy.
- Strings update immediately if the user edits credentials/port and
  restarts the service.

### 3.4 Volume / Data Management

**Feature:** Reset the data of one specific service without touching
sibling services in the same profile.

Acceptance criteria:
- "Reset data" on a service stops it, removes only its volume, and
  recreates it fresh on next start.
- The action requires an explicit confirmation step (destructive,
  irreversible).
- Other running services in the same profile are unaffected and remain
  running throughout.

### 3.5 Real-Time Status View

**Feature:** A live dashboard of all managed containers across all
profiles.

Acceptance criteria:
- Shows: service name, engine/version, state, mapped host port, CPU %, RAM
  usage — refreshed on an interval (target: ≤2s) without manual refresh.
- Reflects containers stopped/started outside the app (e.g. via Docker
  Desktop or CLI) within one refresh cycle.
- Clicking a running service reveals its connection string (3.3) inline.

## 4. Module 2 — DB Client

### 4.1 Connect by URL

**Feature:** Paste a connection string; the app parses it and fills every
field of the connection form.

Acceptance criteria:
- Accepts the four URL schemes from 3.3, including query-string params
  (e.g. `?sslmode=require`, `?authSource=admin`).
- On paste, host/port/user/password/database fields populate immediately,
  editable afterward.
- Malformed strings show an inline parse error naming the offending part,
  not a generic failure.
- A one-click "Test connection" validates reachability before saving.

### 4.2 Multi-Tab Sessions

**Feature:** Multiple connections and/or multiple query tabs open
concurrently.

Acceptance criteria:
- Tabs are independent: closing one does not affect others' open
  transactions or unsent edits.
- Tabs persist across app restart (reopened, not re-connected — reconnect
  is explicit) OR are clearly marked closed-on-exit; behavior must be
  consistent and documented (decision made in `plan.md`).
- Switching tabs preserves scroll position and unsaved query text.

### 4.3 Editable Data Grid (MySQL / PostgreSQL)

**Feature:** Excel-like grid for viewing and editing table data directly.

Acceptance criteria:
- View: paginated row browsing with column type indicators.
- Edit: in-place cell edit commits as an `UPDATE` on blur/enter, using the
  table's primary key; rows without a usable primary key are read-only
  with a visible reason.
- Insert: an "add row" affordance opens a blank row bound to column
  defaults/types.
- Delete: row delete requires confirmation for >1 row at a time.
- Failed writes (constraint violation, type mismatch) surface the
  database's actual error message inline on the offending cell/row.

### 4.4 Document View (MongoDB)

**Feature:** Tree/JSON viewer and editor for collections — not a flattened
grid.

Acceptance criteria:
- Documents render as an expandable/collapsible tree matching BSON
  structure (nested objects, arrays, typed scalars).
- In-place edits validate JSON structure before allowing save.
- New document creation starts from an empty `{}` or a duplicate of a
  selected document.
- Delete requires confirmation.

### 4.5 Key Browser (Redis)

**Feature:** Browse and edit keys across all supported data types.

Acceptance criteria:
- Supports string, hash, list, set, sorted set.
- TTL is visible per key and editable (set/persist/change).
- Key rename and delete supported; delete requires confirmation.
- Pattern-based key filtering (e.g. `session:*`) for large keyspaces.

### 4.6 Query Editor

**Feature:** Monaco-based editor with syntax highlighting and
autocomplete.

Acceptance criteria:
- Highlighting matches the active connection's engine (SQL dialect vs.
  Mongo shell-style vs. Redis commands).
- Autocomplete suggests table/column names (SQL engines) or
  collection/field names (Mongo) discovered from the live schema.
- Multi-statement execution (SQL) runs statements independently and
  reports per-statement success/failure.
- Query execution is cancellable mid-run.

### 4.7 Saved Snippets

**Feature:** Name, tag, and store frequently used queries for reuse.

Acceptance criteria:
- A snippet can be scoped to one connection or marked global (usable from
  any connection of a compatible engine).
- Snippets are searchable by name and tag.
- Running a snippet loads it into the current tab's editor without losing
  unsaved work in that tab (opens a new tab if the current one is dirty).

### 4.8 Migrations (PostgreSQL, MySQL only — v1)

**Feature:** Minimal schema versioning: create, apply, rollback.

Acceptance criteria:
- "Create migration" scaffolds a timestamped up/down file pair tied to a
  connection profile.
- "Apply" runs all pending migrations in order and records applied state
  in a tracking table inside the target database (see `plan.md` for the
  tracking design).
- "Rollback" reverts exactly one migration step at a time (no bulk
  rollback in v1).
- Applying a migration that fails mid-way leaves the tracking table
  accurate (failed migration is not marked applied) and surfaces the DB
  error.

### 4.9 Import / Export

**Feature:** Move data in and out via CSV, JSON, and SQL dump.

Acceptance criteria:
- Export scope: full table, or the result set of the currently executed
  query — user picks explicitly.
- CSV/JSON export preserves column types faithfully enough to round-trip
  (dates, numbers, nulls distinguishable from empty string).
- SQL dump export produces valid `CREATE TABLE` + `INSERT` statements
  importable into a fresh instance of the same engine.
- Import validates the file against the target table's columns before
  committing; mismatches are reported before any row is written.

### 4.10 Query History

**Feature:** Per-connection log of executed queries.

Acceptance criteria:
- Each entry records: query text, timestamp, duration, success/failure,
  row count affected/returned.
- History is filterable by connection and searchable by text.
- A history entry can be replayed into a new tab with one click.

### 4.11 Schema Diagram

**Feature: ER diagram for relational connections (PostgreSQL, MySQL)**

Generates an entity-relationship diagram from live schema introspection —
no manual modeling. Primary motivation: visualizing table relationships is
one of the hardest things for a student to grasp early on, so the diagram
must be a byproduct of connecting, not a separate modeling exercise.

Acceptance criteria:
- Generated from live introspection of the active schema: tables,
  columns, types, primary keys, foreign keys.
- Rendered inside the app using Mermaid `erDiagram` syntax — no external
  file is required to view it.
- Zoom and pan for large schemas; text must stay legible even when the
  app is projected on a classroom screen (a real usage requirement, not
  cosmetic).
- A "Regenerate" button refreshes the diagram; it is explicitly **not**
  a live/auto-updating view — this avoids invalidation complexity that
  the classroom use case doesn't need.
- Exportable as PNG/SVG, and as raw copyable Mermaid text (so a student
  can paste it directly into their own notes or a README).

**Feature: Structure diagram for MongoDB**

Acceptance criteria:
- Unlike the relational ER diagram, there are no real foreign keys — the
  diagram infers each collection's shape from a sample of N documents
  (N configurable, with a sensible default).
- Must be visually labeled as **"inferred structure, not an enforced
  relationship"** — this is intentional: reinforcing the difference
  between relational and document modeling is part of the feature's
  pedagogical value, not an afterthought.
- Same export capabilities as the relational ER diagram.

## 5. Non-Functional Requirements

- **Native desktop, not Electron-class weight.** Idle memory footprint and
  cold-start time are explicit review criteria at the end of every phase.
- **Local-only.** No outbound network calls except to Docker daemon
  (local) and to databases the user explicitly configures (which may be
  remote — see `plan.md` §7 for the local-vs-remote boundary).
- **Dark mode by default**, deliberate typography and visual identity —
  treated as a hard requirement, not a polish pass. UI is reviewed against
  this bar at the end of every phase, not only at the end of the project.
- **Resumability.** Each phase in `plan.md`/`tasks.md` must be
  independently completable and leave the app in a runnable state, so a
  3-week pause does not strand in-progress work.

## 6. Out of Scope for v1

Explicitly excluded to prevent scope creep. Revisit only after v1 ships:

- Cloud sync or multi-device sharing of profiles/snippets/history.
- Team features: multi-user access, shared connections, permissions/roles
  UI for target databases.
- SSH tunneling / bastion-host support for reaching remote databases.
- Remote Docker hosts / Docker context switching (v1 assumes a local
  Docker Engine or Docker Desktop only).
- Query performance profiling / `EXPLAIN` visualizer.
- Backup scheduling or automated recurring exports.
- Dedicated SSL/TLS certificate manager UI (raw connection-string params
  are supported; no cert picker/wizard).
- Plugin or extension system.
- Auto-update mechanism (manual reinstall is acceptable for v1).
- Any database engine beyond MySQL, PostgreSQL, MongoDB, Redis.
- AI-assisted query generation or natural-language-to-SQL.
- Bulk/multi-step migration rollback (single-step rollback only, see 4.8).
- Full cross-platform QA: Windows is the primary target; macOS/Linux
  builds are best-effort via the cross-platform toolchain but not
  validated in v1 acceptance.
- Editing the schema from the diagram (4.11 is a view, not a model
  editor).
- Schema diagrams for Redis (the ER/document-structure concept doesn't
  apply to a key-value store).
- Manual/drag-and-drop node layout in diagrams (layout is automatic via
  Mermaid, not user-arranged).

## 7. Success Definition for v1

v1 is "done" when the developer's own real-world workflow — starting a new
side project — goes through Stackyard end to end at least once: create a
profile, start an environment, connect via the DB Client, run and save a
few snippets, and tear it down — without ever opening Docker Desktop, a
terminal, or another DB client.
