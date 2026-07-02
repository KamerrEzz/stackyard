# Stackyard — Technical Plan

Status: DRAFT — pending approval
Depends on: `spec.md` (approved before this is finalized)

## 1. Stack Decision: Wails (Go) over Tauri (Rust)

### 1.1 The comparison, on the merits

| Dimension | Tauri (Rust) | Wails (Go) |
|---|---|---|
| Community/ecosystem size | Larger; more plugins, more Stack Overflow/Discord history | Smaller, but sufficient for this scope |
| IPC/bindings ergonomics | Very good (`#[tauri::command]`, optional `tauri-specta` for typed bindings) | Good (`wails.Bind` auto-generates JS bindings from Go struct methods) |
| Compile times | Slow, especially with heavy macro use (e.g. `sqlx` compile-time query checks) | Fast (seconds) |
| Learning curve from JS/TS, zero prior Rust/Go | Steep — ownership/borrowing/lifetimes are a genuinely different mental model, and they block you (not just slow you down) until internalized | Shallow — GC'd, imperative, structurally close enough to TS that a JS dev is writing useful code in the first sitting |
| Mobile targets | Yes (Tauri 2.0) | No |
| Binary size / footprint | Small (system webview) | Small (system webview) |
| Docker control library | `bollard` — community-maintained, good, not official | `docker/docker/client` — the **actual SDK Docker Engine itself is built on**; this is as canonical and well-documented as a Docker API client gets in any language |
| Postgres driver | `sqlx` / `tokio-postgres` — mature, async, compile-time query checking is a real strength once you know Rust | `pgx` — mature, fast, extremely well documented, huge example corpus |
| MySQL driver | `sqlx` (MySQL feature) — mature | `go-sql-driver/mysql` — the de facto standard driver, used everywhere, huge example corpus |
| MongoDB driver | Official `mongodb` Rust crate — mature | Official `mongo-go-driver` — mature, same vendor-maintained tier |
| Redis driver | `redis-rs` — mature | `go-redis/redis` — mature, arguably the most widely deployed Redis client outside of the CLI itself |

Conclusion on drivers: **both ecosystems are production-grade for all four
engines.** This is not the deciding factor — don't let it be. The deciding
factors are the learning curve and the Docker SDK fit.

### 1.2 Why this matters for *this* profile specifically

You described: strong JS/TS, zero Rust or Go experience, you learn by
building and shipping visible things, you work in 5-10h/week sprints, and
motivation depends on seeing progress regularly (not after fighting the
toolchain for two sessions).

Rust's borrow checker is not "syntax you'll pick up" — it actively rejects
programs that are logically correct until you restructure ownership. For a
newcomer, that fight is exactly the kind of session-eating friction that
kills momentum in a low-frequency sprint cadence: you can lose an entire
2-4h session to a lifetime error with zero visible UI progress to show for
it. Go has no equivalent wall. It is close enough to TypeScript
(interfaces, structs, explicit error returns instead of exceptions) that a
JS/TS developer is productive — genuinely productive, not just
copy-pasting — inside the first session.

On top of that, Module 1's core dependency is Docker control, and Go's
`docker/docker/client` package is not just "a good option" — it is the
literal client library the Docker CLI and Docker Compose are built on top
of. Using it means the best-documented, most battle-tested Docker
automation code in existence, in the same language your core feature
lives in.

### 1.3 Recommendation

**Wails v2, Go backend, React + TypeScript + Tailwind frontend.** This is
a clear call, not a coin flip left open for you — the combination of (a)
gentle ramp for a first-timer under sprint-momentum constraints and (b)
best-in-class Docker SDK fit outweighs Tauri's larger ecosystem and mobile
story, neither of which this project needs.

**Known trade-off accepted knowingly:** Wails has a smaller maintainer
base and plugin ecosystem than Tauri. If Wails stalls or a hard blocker
surfaces, the fallback is Tauri + `bollard`/`sqlx` — the domain logic in
`internal/` (§3) is written in a way that isolates the Docker/DB layer
behind interfaces specifically so a framework swap doesn't require a full
rewrite (see §3.3).

## 2. Architecture Overview

```
┌─────────────────────────────┐        Wails bindings (generated)       ┌──────────────────────────┐
│  Frontend (React + TS)      │ ───────────────────────────────────────▶ │  Go backend (App struct)  │
│  Tailwind, Monaco editor    │ ◀─────────────────────────────────────── │  bound methods = API      │
└─────────────────────────────┘   events (EventsEmit/EventsOn) for push  └──────────────────────────┘
                                                                                    │
                                                        ┌───────────────────────────┼───────────────────────────┐
                                                        ▼                           ▼                           ▼
                                              internal/docker              internal/dbengine             internal/storage
                                              (docker/docker/client)       (pgx, mysql, mongo, redis)     (SQLite, app-local state)
```

- **Frontend ↔ backend:** Wails generates TypeScript bindings from Go
  methods exposed via `wails.Bind`. Calls read as plain async function
  calls from React — no manual JSON-RPC/IPC plumbing.
- **Push updates** (container status, resource usage, long-running query
  progress) use Wails' event bus (`runtime.EventsEmit` in Go →
  `EventsOn` in React), not polling from the frontend.
- **Go backend is organized in `internal/` packages by domain**, each
  behind a small interface, so the App struct (the only thing bound to the
  frontend) stays a thin adapter layer. This is what makes a future
  framework swap (§1.3 fallback) contained instead of total.

## 3. Folder Structure

```
stackyard/
├── wails.json
├── main.go
├── app.go                      # App struct — the ONLY surface bound to the frontend
├── internal/
│   ├── docker/                 # Module 1 domain logic
│   │   ├── client.go            # thin wrapper over docker/docker/client
│   │   ├── compose.go           # translates a Profile into containers/network/volumes (no YAML file ever written)
│   │   └── stats.go             # CPU/RAM polling per container
│   ├── dbengine/                # Module 2 domain logic
│   │   ├── engine.go             # Engine interface: Connect, Query, ListSchemas, ...
│   │   ├── postgres/, mysql/, mongo/, redis/   # one implementation per engine
│   │   └── urlparse.go           # connection-string → form fields (feature 4.1)
│   ├── migrations/              # feature 4.8, Postgres/MySQL only
│   ├── diagram/                  # feature 4.11 — schema metadata → Mermaid syntax, owned by the erd-builder subagent
│   │   ├── relational.go          # Postgres/MySQL: tables + FKs → erDiagram
│   │   └── mongo.go               # sampled document shape → labeled "inferred structure" diagram
│   └── storage/                 # local app state (profiles, snippets, history)
│       ├── sqlite.go
│       └── models.go
├── frontend/
│   ├── src/
│   │   ├── modules/
│   │   │   ├── environment-manager/
│   │   │   ├── db-client/
│   │   │   │   ├── grid/            # 4.3 editable SQL grid
│   │   │   │   ├── document-view/   # 4.4 Mongo tree/JSON
│   │   │   │   └── key-browser/     # 4.5 Redis
│   │   │   └── schema-diagram/      # 4.11 — Mermaid renderer, zoom/pan, export; owned by erd-builder subagent
│   │   ├── components/          # shared design-system pieces
│   │   ├── lib/                 # typed wrappers around generated Wails bindings
│   │   └── styles/
│   ├── tailwind.config.ts
│   └── vite.config.ts
└── docs/
    └── STATE.md                 # living doc — see §8 (pause/resume)
```

**New frontend dependency:** `mermaid` — renders the `erDiagram` /
structure-diagram text produced by `internal/diagram` (feature 4.11). No
other new dependency is introduced by this module; PNG/SVG export uses
Mermaid's own rendering output rather than a second charting library.

## 4. Local Data Model

All app-local state lives in one SQLite file at the OS-standard app-data
path (Windows: `%APPDATA%\Stackyard\stackyard.db`), accessed via
`modernc.org/sqlite` — a **pure-Go, CGO-free** driver. This is a deliberate
choice over `mattn/go-sqlite3`: it avoids requiring a C compiler toolchain
on the dev machine, which matters for keeping onboarding/resume friction
near zero after a multi-week pause (spec.md §5, resumability NFR).

```sql
-- profiles: one row per named environment
profiles(id, name, created_at)

-- services: one row per service within a profile
services(id, profile_id, engine, image_tag, host_port, username,
         password_encrypted, db_name, volume_name)

-- connections: DB Client saved connections (may point at a Stackyard-
-- managed service OR an arbitrary external host)
connections(id, name, engine, host, port, username, password_encrypted,
            database, params_json, last_used_at)

-- snippets
snippets(id, connection_id NULL, engine, name, body, tags_json,
         created_at, updated_at)

-- query_history
query_history(id, connection_id, query_text, executed_at, duration_ms,
               success, rows_affected, error_message NULL)
```

Passwords are encrypted at rest (OS keychain where available via a
lightweight wrapper; AES-256 with a locally-generated key file as the
cross-platform fallback) — never stored plaintext, even though this is a
local-only tool, because the SQLite file itself could end up in a backup
or synced folder outside the app's control.

**Migrations (feature 4.8) are a deliberate exception**: their applied/
pending state is tracked in a `schema_migrations` table created **inside
the target database**, not in Stackyard's local SQLite. This matches how
every established migration tool (golang-migrate, Flyway, etc.) works, and
avoids a split-brain where the app's local state and the target DB's
actual schema can silently disagree. Migration *file content* (up/down
SQL) is stored on disk under a per-connection folder the user points the
app at — not inside the SQLite file.

## 5. Local-vs-Remote Boundary (NFR clarification)

"100% local, no cloud backend" (spec.md §5) describes **Stackyard's own
infrastructure**: no telemetry, no account, no server Stackyard talks to
on your behalf. It does **not** mean the DB Client can only reach
Docker-managed local containers — connecting to an already-running remote
Postgres/Mongo/etc. via a pasted connection string (feature 4.1) is
in-scope and expected. What's explicitly out of scope for v1 is Stackyard
*managing* remote Docker hosts (spec.md §6).

## 6. Phased Roadmap

Ordered for earliest visible payoff first, hardest/most novel UI paradigms
(Mongo tree view, Redis key browser, migrations) last — matching the
request directly.

| Phase | Delivers | Why here |
|---|---|---|
| 0 | Wails scaffold, Tailwind dark-mode shell, one Go method round-tripped to React | Proves the whole toolchain end-to-end before any real feature — cheapest possible motivation win |
| 1 | Environment Manager MVP: **one engine (Postgres)**, one profile, start/stop, connection string + copy | Validates the 3-click criterion on the simplest possible slice |
| 2 | Environment Manager full: all 4 engines, profile CRUD, volume reset, live status/stats view | Completes Module 1 |
| 3 | DB Client MVP: connect-by-URL, Postgres + MySQL only (share grid code), Monaco editor, read-only results grid | Two engines share almost all UI — cheap second win |
| 4 | Editable grid (insert/update/delete), multi-tab, query history, snippets | Completes the relational half of Module 2 |
| 4.5 | Schema Diagram — relational ER (Postgres + MySQL) | Depends on `ListSchemas`/`ListTables` (Phase 3) and FK metadata — building it here avoids re-deriving introspection later; runs in parallel with Phase 4's editable-grid work via the `erd-builder` subagent, since the two share no code surface |
| 5 | MongoDB support: document tree/JSON view | New UI paradigm, isolated to its own module folder |
| 5.6 | Schema Diagram — MongoDB inferred structure | Depends on the Mongo `Engine` existing (Phase 5); added as that phase's closing task rather than a separate phase, since it reuses Phase 4.5's renderer |
| 6 | Redis support: key browser, per-type views, TTL | New UI paradigm again |
| 7 | Import/export: CSV, JSON, SQL dump | Cuts across engines already built |
| 8 | Migrations (Postgres + MySQL) | Most complex/least novel-value feature — last by design, per your explicit ask |
| 9 | Polish pass: performance, packaging (Windows installer via Wails build), first real dogfood run | Closes the success definition in spec.md §7 |

## 7. Risks & Mitigations

- **Wails project momentum risk** (smaller team than Tauri): domain logic
  isolated behind interfaces (§2, §3) so a Tauri fallback is a rewrite of
  the adapter layer, not the whole app.
- **Docker Desktop licensing/availability on the dev machine**: the app
  talks to the Docker Engine API directly, not Docker Desktop
  specifically — works against any compliant local engine.
- **Windows named-pipe vs Unix-socket Docker API access**: `docker/docker/
  client` handles both transparently; verify in Phase 0 on this machine
  specifically (Windows 11), not assumed.
- **Scope creep across 4 engines**: spec.md §6 is the standing guard;
  revisit it at the start of every phase, don't silently expand mid-phase.

## 8. Pause/Resume Strategy

Given 5-10h/week sprints with possible multi-week gaps:

- `docs/STATE.md` is updated at the **end of every session** (not just
  every phase) with: current phase, last task completed, any in-flight
  decision not yet finalized, and the exact command to run the app locally.
- Each phase (§6) is scoped to leave the app in a runnable, demoable state
  — no phase ends mid-refactor.
- `tasks.md` tasks are sized so a single session finishes a whole task,
  never leaves one half-done across a gap.
- No task depends on transient local state (e.g. an unpushed branch, a
  manually-run script) surviving between sessions — anything needed to
  resume is either in git or in `docs/STATE.md`.
