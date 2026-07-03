# Stackyard — Project State

Living document. Updated at the end of every work session (per `plan.md`
§8). Read this first before resuming work — it should be enough on its
own, without re-reading code, to know what's proven and what's next.

---

## Session 1 — 2026-07-02 — Phase 0 complete

### What's proven to work (actually run, not just compiled)

- `wails dev` builds and launches the app: a real native window titled
  "stackyard" opens (confirmed via `Get-Process`/`MainWindowTitle`), Vite
  dev server on `:5173` and the Wails dev server on `:34115` both respond
  `200`. Process was stopped cleanly afterward — no leftover processes.
- Frontend↔backend IPC round trip: the "Ping backend" button in the top
  bar calls the bound `App.Ping()` Go method through the Wails-generated
  TS bindings (`frontend/wailsjs/go/main/App.d.ts`) and displays the
  response. This is the task 0.3 smoke test and it is wired for real, not
  stubbed.
- `internal/storage`: SQLite schema (profiles, services, connections,
  snippets, query_history) creates idempotently via `PRAGMA user_version`
  migration tracking. DB file lands at
  `%APPDATA%\Stackyard\stackyard.db` — verified with a real run, not just
  a unit test assertion.
- `go build ./...` and `go test ./...` are both green. 8/8 storage tests
  passing (round-trip CRUD, idempotent init, FK enforcement, app-data
  path resolution).
- `pnpm run build` (tsc + vite build) is clean, zero TS errors.

### Command to run the app locally

```
cd D:\CODE\projects\Stackyard
wails dev
```

Frontend deps are managed with **pnpm**, not npm — `wails.json`'s
`frontend:install`/`frontend:build`/`frontend:dev:watcher` were changed
from the Wails scaffold default (`npm ...`) to `pnpm ...` (see "Gotchas"
below for why this matters, not just a style preference).

### Run tests

```
cd D:\CODE\projects\Stackyard
go test ./...
```

No frontend test suite yet — Phase 0's frontend work (Tailwind + shell)
is pure layout/JSX with no non-trivial logic, so no Vitest suite was
required for it per the session's testing directive. The first frontend
logic worth unit-testing (URL parsing, data transforms) arrives in Phase
3 (`urlparse.go` is Go-side; frontend logic tests will start wherever the
first non-trivial TS transform appears).

### Files/structure created this phase

- Wails React-TS scaffold merged into the project root (`main.go`,
  `app.go`, `wails.json`, `frontend/`) — scaffolded into a scratch temp
  dir via `wails init -d`, then copied in, since `wails init` requires an
  empty target and the repo root already held `spec.md`/`plan.md`/
  `tasks.md`/`.claude/`.
- `frontend/tailwind.config.ts`, `postcss.config.js` — Tailwind v3, dark
  mode forced (`darkMode: 'class'`, no toggle exists in the UI at all).
- `frontend/src/components/Sidebar.tsx`, `TopBar.tsx`, `PingCheck.tsx`.
- `frontend/src/modules/environment-manager/EnvironmentManagerView.tsx`,
  `frontend/src/modules/db-client/DbClientView.tsx` — placeholders only,
  filled in Phases 1–3+.
- `internal/storage/sqlite.go`, `migrations.go`, `models.go`,
  `profiles.go` (+ `_test.go` for the first two).
- `app.go`: `Greet` (scaffold placeholder) replaced with `Ping() string`.

### Ambiguities resolved this phase (flagged for your review, not buried)

`plan.md` §4's schema sketch is abbreviated SQL, not full DDL. The
storage subagent made these interpretation calls when writing the real
`CREATE TABLE` statements — none contradict `plan.md`, but none are
spelled out in it either:

1. `profiles.name` is `UNIQUE` — inferred from `spec.md` §3.1's rename/
   duplicate language implying names are how a user distinguishes
   profiles. Easy to relax if duplicate-named profiles should be legal.
2. `engine` columns (`services`, `connections`, `snippets`) use
   `CHECK (engine IN ('postgres','mysql','mongodb','redis'))` — stricter
   than `plan.md`'s sketch. A 5th engine would need this CHECK updated
   alongside a migration, not just Go-side changes.
3. `services.username`/`password_encrypted`/`db_name` are nullable
   (Redis has no equivalent of a "database name" or username in the same
   sense as the other 3 engines).
4. `snippets.connection_id` is `ON DELETE SET NULL` (deleting a
   connection demotes its snippets to global, doesn't delete them);
   `query_history.connection_id` is `ON DELETE CASCADE` (history without
   its connection was judged much less useful than a snippet's body).
   `plan.md` doesn't specify FK delete behavior at all — flagging both
   choices explicitly since they're genuinely different judgment calls.
5. Timestamps stored as `TEXT` (ISO-8601/RFC3339), not SQLite's native
   `DATETIME` — more portable, directly sortable as text.
6. Migration tracking uses SQLite's built-in `PRAGMA user_version`
   (a plain integer) instead of a bespoke `schema_migrations`-style
   table, per this session's explicit "don't over-engineer" instruction.
   **Note:** this is Stackyard's *own* local-storage versioning — it is
   deliberately unrelated to Phase 8's `internal/migrations`, which
   tracks migrations for the *user's target database*, not this app's
   SQLite file. Conflating the two would be a real design mistake later;
   `sqlite.go`'s package doc comment calls this out explicitly.
7. `db.SetMaxOpenConns(1)` on the SQLite connection pool —
   `modernc.org/sqlite` doesn't gracefully multiplex concurrent writers
   across pooled connections; avoids intermittent `SQLITE_BUSY` errors in
   a single-process desktop app rather than chasing them under load later.

### Gotchas / non-obvious things for whoever resumes this

- **Package manager mismatch breaks `wails dev`.** The Wails scaffold
  defaults `wails.json`'s frontend commands to `npm`, but this project
  uses **pnpm** (already installed, `pnpm-lock.yaml` committed). Running
  `wails dev` with the default `npm install` fails with
  `EUNSUPPORTEDPROTOCOL` / `Unsupported URL Type "workspace:"` because
  npm's arborist chokes trying to read pnpm's `.pnpm` virtual-store
  layout in `node_modules`. Fixed by changing `frontend:install`,
  `frontend:build`, `frontend:dev:watcher` in `wails.json` to `pnpm ...`.
  **Do not switch back to npm** without also deleting `node_modules` and
  `pnpm-lock.yaml` — mixing the two package managers in the same
  `node_modules` tree is what breaks it.
- **`wails.json`'s `outputfilename` was `wails-scaffold`** — a leftover
  from scaffolding into a scratch temp directory before merging into the
  project root. Corrected to `stackyard`.
- Docker Desktop's daemon is **not currently running** on this machine
  (`docker version` succeeds for the CLI but fails to reach
  `npipe:////./pipe/dockerDesktopLinuxEngine`). Phase 0 didn't need it.
  Phase 1 (`internal/docker/client.go`, task 1.1) explicitly does — it
  needs to list real containers against a live daemon. **This is a real
  blocker candidate**: starting Docker Desktop is a GUI action outside
  this session's control if it requires interactive first-run setup;
  documenting here rather than silently assuming/mocking connectivity.
- `go.mod`'s `go` directive was auto-bumped `1.23.0` → `1.25.0` by
  `go mod tidy` when adding `modernc.org/sqlite` (a transitive dependency
  required it). Still satisfies "Go 1.23+" from `tasks.md` 0.1, but
  flagging since it was a toolchain side effect, not a deliberate choice.

### Parallelization note (Phase 0)

Ran two subagents concurrently in the background: Tailwind/app-shell
(0.2) and `internal/storage` (0.4). These share no files (frontend-only
vs. Go-`internal/storage`-only) and neither depends on the other's
output, so this was genuine parallelism, not overhead — both finished
inside roughly the same wall-clock window instead of sequentially.
Toolchain install, `wails init` scaffold, and the `Ping` IPC wiring
(0.1/0.3) were done inline/sequentially since each is either a one-shot
CLI step or genuinely depends on the scaffold existing first — forcing
subagents onto those would have been coordination overhead with no
benefit, consistent with the session's own parallelization guidance.

### Next steps

- Phase 1: Environment Manager MVP (Postgres only). Task 1.1 needs
  Docker Desktop's daemon running — verify before starting, see Gotchas
  above.
- qa-reviewer and docs-changelog to run against this phase before it's
  marked fully closed (see below — done same session).

---

## Proposed version tags

**NOT YET EXECUTED — for the user to review and run manually.**

Phase 0 ("Toolchain & Shell", tasks 0.1-0.5) completed this session and is
confirmed closed per `plan.md` §6's phased roadmap.

**No tag is proposed for this phase.** This changelog/state-tracking
agent's own operating rules define the semver mapping explicitly as: *"end
of Phase 1 → `v0.1.0`, end of Phase 2 → `v0.2.0`, ... **Phase 0 is pure
setup and never gets a tag**."* Phase 0 being this project's first-ever
completed phase doesn't change that — the rule already accounts for
"first phase" by excluding it categorically, since it's scaffolding/
toolchain proof, not a shippable slice of product behavior (spec.md's
Module 1/Module 2 features start at Phase 1). Minting a `v0.x.0`
pre-release tag here would front-load a version number onto "the
toolchain works," not onto any user-facing capability from `spec.md`.

**When the next tag becomes due:** at the close of Phase 1 (Environment
Manager MVP, Postgres-only — tasks 1.1-1.7), propose:

```
git tag -a v0.1.0 -m "Phase 1: Environment Manager MVP (Postgres-only start/stop/restart, connection string copy)"
```

Do not run this now — Phase 1 has not started.
