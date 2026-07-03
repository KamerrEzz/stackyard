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

## Session 2 — 2026-07-02 — Phase 1 complete (Environment Manager MVP, Postgres only)

### What's proven to work (actually run, not just compiled)

- `internal/docker/client.go`: wraps `docker/docker/client`; verified live
  against the local Docker Engine over a **Windows named pipe** —
  confirmed via a build-tag-gated integration test
  (`go test -tags=integration ./internal/docker/...`), not just mocked
  (task 1.1).
- `internal/storage`: full `Profile`/`Service` CRUD (create/read/update/
  delete/list), cascade-delete verified at the storage layer (task 1.2).
- `internal/docker/compose.go`: `EnsureNetwork`/`EnsureVolume`/
  `EnsurePostgresContainer`/`StartPostgresEnvironment` — verified against
  the live Docker Engine for all three real paths that matter: create-
  from-scratch, idempotent reuse (calling it again on an existing
  network/volume/container doesn't recreate or error), and stopped-then-
  restarted-in-place (preserves the existing container/volume identity
  instead of recreating), each with full cleanup after the test (task
  1.3).
- `app.go` bound methods `ListProfiles`/`CreateProfile`/`StartProfile`/
  `StopProfile`/`RestartProfile`/`GetProfileStatus` (Postgres-only MVP
  scope) — non-fatal storage/Docker init: a failure is stored as
  `dbErr`/`dockerErr` on the `App` struct rather than panicking, and every
  dependent method checks it first via `requireDB`/`requireDocker` (task
  1.4). `EnvironmentManagerView.tsx` wired to the real profile list plus
  create/Start/Stop UI, replacing the Phase 0 placeholder.
- `internal/netcheck` (real OS-level port-availability probe) +
  `internal/docker/portcheck.go` (conflict detection that exempts a
  service's own already-running container from being a false-positive
  collision): `CheckPortAvailable`/`SuggestFreePort`/
  `CheckProfilePortConflict` bound on `App`; `handleStart` in the frontend
  calls the pre-check before `StartProfile`, so the user sees "port 5432
  is already in use — try 5433" instead of a raw Docker bind error, and
  `StartProfile` re-checks the same condition server-side as defense in
  depth (task 1.5).
- `internal/docker/connstring.go`: `PostgresConnectionString` builder
  (`net/url`, safe percent-encoding) bound via `GetConnectionString`;
  frontend copy button fetches the string fresh on every click (never
  cached, so it can't go stale) and shows a 2s inline "Copied!"
  confirmation (task 1.6).
- `go build ./...`, `go vet ./...`, `go test ./...` and
  `pnpm run build` all green throughout — including after the retroactive
  comment-style cleanup below.

### Task 1.7 — manual pass, now performed and confirmed

qa-reviewer's Phase 1 gap report correctly caught that 1.7 hadn't been
run yet at the time it reviewed — this has since been performed for
real, driving the actual running app (`wails dev`) with Playwright
against `http://localhost:34115` (the real Wails dev server, real IPC
bridge to the Go backend, real local Docker Engine), not simulated:

- **New profile, full flow** (name field + "Create & Start" — a single
  combined button, not two separate steps): typed a profile name, clicked
  "Create & Start" once. Profile created, Postgres container created and
  started, UI showed "Running" **1041ms** after the click. Total
  interactions: 1 text field + 1 click — under spec.md §3.2's 3-click
  criterion.
- **Existing profile, restart** ("select profile → click Start" path):
  clicked the row's "Start" button once on an already-created (stopped)
  profile — reached "Running" in **1063ms**. Total interactions: 1 click.
- **Connection string copy**: clicked "Copy connection string" once;
  clipboard contained exactly `postgres://postgres:postgres@localhost:5432/postgres`,
  confirmed by reading `navigator.clipboard` back in the same browser
  context — matches the format from `internal/docker/connstring.go`
  exactly. Button flipped to "Copied!" as expected.
- **Stop**: clicked "Stop" once, UI reached "Stopped" within the poll
  window.
- **Testing-methodology footnote, not a product bug:** the first pass
  showed the copy button flip to "Copy failed" — this was headless
  Chromium's default clipboard-permission sandboxing (Playwright's
  default browser context doesn't grant `clipboard-write` by default),
  not a Stackyard defect. The app's own `catch` branch handled it
  correctly by showing "Copy failed" instead of crashing. Re-running with
  `context.grantPermissions(['clipboard-read','clipboard-write'], ...)`
  confirmed the underlying flow works; Wails' actual native WebView2
  window (an installed-app context, not a sandboxed browser tab) doesn't
  carry this same default restriction.
- All Docker resources (`stackyard-service-*` container,
  `stackyard-profile-*` network/volume) and the test profile row created
  during this verification were removed afterward — confirmed via
  `docker ps -a`/`network ls`/`volume ls` and a throwaway
  `internal/storage`-based cleanup program (not committed).

Both flows are comfortably under the 3-click bar; no UI adjustment was
needed. `tasks.md`'s Phase 1 checkboxes (1.1-1.7) are now checked.

### Gotchas / non-obvious things for whoever resumes this

- **`docker/go-connections` must stay pinned to `v0.5.0`.** `go-connections`
  v0.6+ unexports the Windows named-pipe `DialPipe` symbol that
  `docker/docker` v28 calls directly — upgrading it breaks Windows
  named-pipe connectivity (task 1.1) at compile time, not runtime, so it's
  an easy trap to hit via an unrelated `go get -u`/`go mod tidy`.
- Docker Desktop's daemon **was** reachable this session (unlike the
  blocker flagged at the end of Phase 0) — all of 1.1's and 1.3's
  integration tests ran against a real live engine, not a mock.

### Command to run the app locally

```
cd D:\CODE\projects\Stackyard
wails dev
```

(Unchanged from Phase 0 — see above for the pnpm/wails.json gotcha.)

### Run tests

```
cd D:\CODE\projects\Stackyard
go test ./...
```

Docker-dependent tests are gated behind a build tag and require a live
local Docker Engine:

```
go test -tags=integration ./internal/docker/...
```

### Next steps

- Confirm/close task 1.7 (see flagged note above) before treating Phase 1
  as fully closed in the strict `tasks.md` sense.
- Phase 2: Environment Manager, full (MySQL/MongoDB/Redis, profile
  wizard, volume reset, live status/stats dashboard) — tasks 2.1-2.8.

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

**Update, Session 2 (2026-07-02):** Phase 1's functional deliverable
(tasks 1.1-1.6 — Docker client, Profile/Service persistence, Postgres
container orchestration, bound App methods + UI, port-conflict pre-check,
connection-string copy) is built and verified against a live Docker
Engine; the tag above is now **due**, with one caveat: task 1.7 (the
manual 3-click timing pass) has no evidence of having been run this
session — see "Task 1.7 — flagged, not confirmed done this session"
above. Confirm 1.7 one way or the other before running the tag command
below, or run it now if it genuinely wasn't done yet:

```
git tag -a v0.1.0 -m "Phase 1: Environment Manager MVP (Postgres-only start/stop/restart, connection string copy)"
```

Still not run by this agent — for the user to execute manually.

---

## Retroactive comment cleanup — rationale preserved (frontend)

Per `CLAUDE.md`'s comment-style rule, all inline comments were stripped from
the frontend source (only a required TS triple-slash directive in
`vite-env.d.ts` was left untouched — not a comment). No logic, JSX, or
class names changed; `pnpm run build` stayed green throughout. Rationale
that was previously inline now lives here instead:

- **`PingCheck.tsx`**: this component is the task 0.3 smoke test — it
  proves the Go↔React IPC round trip and the generated Wails TS bindings
  work end-to-end, before any real feature is built on top of them.
- **`EnvironmentManagerView.tsx`** (task 1.4/1.5/1.6, spec.md §3.1-§3.3):
  - Scope: this view is Postgres-only for the profile list plus
    Start/Stop. Tasks 1.5 (port-conflict pre-check) and 1.6
    (connection-string copy) are both implemented within this same file.
    The multi-engine profile wizard (task 2.4, Phase 2) is deliberately
    out of scope here.
  - `handleStart`'s port-conflict pre-check calls
    `CheckProfilePortConflict` before `StartProfile`, so the user sees
    "port 5432 is already in use — try 5433" instead of a raw Docker bind
    error. `StartProfile` re-checks the same condition server-side
    (defense in depth, see `app.go`) — if the frontend pre-check itself
    fails (e.g. Docker unreachable), the code intentionally falls through
    to `StartProfile` rather than blocking Start, and `StartProfile`'s own
    `requireDocker` guard surfaces a clear error in that case.
  - `CONFIRMATION_MS` (2000ms) is how long the transient "Copied!"
    acknowledgment stays visible, satisfying spec.md §3.3's
    toast/inline-confirmation requirement without a full toast library.
  - `CopyConnectionStringButton` fetches the connection string fresh from
    the Go backend (`GetConnectionString`) on every click rather than
    caching it, so it can't go stale if credentials/port changed since
    the last render (spec.md §3.3's third acceptance criterion).
- **`style.css`**: dark mode is the only theme for v1 (spec.md §5),
  forced at the `html` root rather than behind a class toggle, so there
  is no light-theme token set to accidentally introduce or maintain.
- **`tailwind.config.ts`**: `darkMode: 'class'` is kept explicit (instead
  of `'media'`) even though no toggle exists today, specifically to avoid
  ever accidentally picking up a user's OS light-mode preference.

---

## Retroactive comment cleanup — rationale preserved (Go side)

Per the project's no-inline-comments rule (`CLAUDE.md`), every Go file was
swept: only the package doc comment per file and doc comments on exported
functions/types/consts survive, trimmed for concision. Everything else —
inline comments, comments on unexported helpers, comments in `_test.go`
files — was deleted. Where a deleted comment captured a genuinely
non-obvious decision or gotcha not already covered elsewhere in this
document, it's preserved below, organized by file.

- **`app.go` — `startup()`'s failure handling.** Storage and Docker are both
  initialized in `startup`, but a failure in either does NOT crash the app or
  panic: neither is required to be reachable at app-launch time, only at the
  point a docker-dependent bound method is actually called. Failures are
  stored on the `App` struct (`dbErr`/`dockerErr`) instead, and every bound
  method that needs storage/Docker checks for that stored error first via
  `requireDB`/`requireDocker`, surfacing a real error string to the frontend
  rather than a nil-pointer panic. Additionally, `docker.NewClient()` only
  builds configuration — it doesn't dial the engine — so `startup` follows it
  with a short-timeout `Ping` to actually prove the daemon is reachable; if
  that `Ping` fails, the half-verified client is closed and dropped (not kept
  around), so `docker`-dependent methods report `dockerErr` until the user
  retries (e.g. after starting Docker Desktop).

- **`app.go` — `nextFreeHostPort`/`CreateProfile`'s port defaulting is a
  narrow self-collision guard, not real conflict detection.** It only checks
  ports Stackyard itself has already handed out (via `usedHostPorts`), so a
  second default profile created back-to-back doesn't collide with the
  first. It does NOT probe the OS for arbitrary in-use ports — that's what
  `netcheck.IsPortFree` + `SuggestFreePort`/`CheckProfilePortConflict` are
  for. Any remaining conflict (something else on the machine already bound
  to the port) is expected to surface as Docker's own bind error, or be
  caught by the real pre-start check, not by this helper.

- **`internal/storage/migrations.go` — migration steps must be
  idempotent/forward-only.** Each `schemaMigration`'s statements must be safe
  to run against a database already at a later version having never seen
  that step — in practice this means every statement uses
  `CREATE TABLE/INDEX IF NOT EXISTS`. This is deliberately NOT a full
  migration framework (no down-migrations, no per-connection folders); it
  only ever grows Stackyard's own local schema forward across app versions.

- **`internal/storage/migrations.go` — `applyMigration`'s `PRAGMA
  user_version = %d` uses `fmt.Sprintf`, not a bind parameter.** This is
  intentional and not a SQL-injection risk: `PRAGMA user_version` doesn't
  accept bind parameters at all, and the interpolated value is always a
  compile-time `int` from `schemaMigrations` — never user input.

- **`internal/storage/services.go` — `UpdateService` takes a full `*Service`
  rather than individual fields or a partial patch struct.** `Service` has 7
  mutable columns beyond `ID`/`ProfileID`; Phase 2 (MySQL/MongoDB/Redis
  config) adds more fields to the same struct, and a full-struct replace
  means that growth never requires widening `UpdateService`'s parameter
  list. Callers that want to change one field fetch via `GetService`, mutate
  it, then call `UpdateService` — the same round-trip pattern
  `CreateService`/`GetService` already establish.

- **`internal/storage/sqlite.go` — `buildDSN` encodes PRAGMAs into the DSN
  itself, not as post-connect statements.** SQLite PRAGMAs (`busy_timeout`,
  `foreign_keys`) are per-connection and don't persist in the database file,
  so they're passed as `_pragma` query parameters on the `file:` DSN rather
  than run as separate `PRAGMA` statements after opening — this guarantees
  every new pooled connection gets them applied automatically.

- **`internal/docker/compose.go` — `ensureImage`'s drain of the pull response
  is required, not optional cleanup.** `ImagePull` streams progress as
  newline-delimited JSON; the pull is not actually complete from the
  engine's perspective until that stream is fully read. Skipping the
  `io.Copy(io.Discard, rc)` drain (or returning early) would leave the pull
  racing with whatever tries to use the image next.
