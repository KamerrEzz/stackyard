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

**Update, Session 3 (2026-07-02) — Phase 2 closed, `v0.2.0` now due:**
Phase 2 ("Environment Manager, Full," tasks 2.1-2.8) is complete and
manually verified (see "Phase 2 — manual verification pass" further
below); per `plan.md` §6 this phase **completes Module 1 — Environment
Manager** in full. Checked `git tag -l` directly: **no tags exist in this
repo yet** — `v0.1.0` from the note above has still not been run.

That does not block proposing `v0.2.0` now. The semver mapping (end of
Phase N → `v0.N.0`) is keyed to which phase closed, not to whether the
previous phase's tag command was actually executed — a git tag is just a
named ref to a specific commit, and both commits already exist in history
regardless of tagging order:

- Phase 1's closing commit: `e743c6b` ("docs: close Phase 1 - qa-reviewer
  pass, changelog, task 1.7 manual verification")
- Phase 2's closing commit: `92ff4bc` ("docs: manual Phase 2 verification
  pass (multi-engine, reset volume, dashboard)") — current `HEAD`

The user can run both tag commands in either order, or just `v0.2.0` now
and `v0.1.0` later pointing at `e743c6b` — the resulting tags will be
historically accurate either way since each points at the commit where
that phase actually closed, not at "whenever the tag command ran."

```
git tag -a v0.1.0 -m "Phase 1: Environment Manager MVP (Postgres-only start/stop/restart, connection string copy)" e743c6b
git tag -a v0.2.0 -m "Phase 2: Environment Manager, full (MySQL/MongoDB/Redis orchestration, multi-engine wizard, profile duplicate/rename/delete, reset volume, live status/stats dashboard) - completes Module 1"
```

Neither has been run by this agent — both are for the user to execute
manually, in whatever order/timing they prefer.

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

---

## Session 3 — 2026-07-02 — Phase 2 wave 1 (parallel)

Five tasks ran concurrently, each scoped to a disjoint file set to avoid
collisions: MySQL orchestration (2.1), MongoDB orchestration (2.2), Redis
orchestration (2.3), profile duplicate/rename/delete UI (2.5), and Docker
stats polling (2.7). All five landed; `go build ./...`, `go vet ./...`,
`gofmt -l .`, `go test ./...`, `go test -tags=integration ./internal/docker/...`
(run twice to check for flakiness), and `pnpm run build` are all green.

### Real bug found and fixed: test container-ID collisions

Each engine's integration test hardcodes a `testProfileID`/`testServiceID`
constant to build deterministic Docker resource names
(`stackyard-profile-<id>`, `stackyard-service-<id>`). Three of the five
parallel tasks independently picked `999002` (colliding with the
pre-existing `lifecycle_integration_test.go`), and a later pick collided
with Redis's `999003` too — there is no central registry for these IDs,
just convention, so parallel tasks with no visibility into each other's
choices collided. Fixed by assigning each file a unique ID:
`compose_integration_test.go`=999001, `lifecycle_integration_test.go`=999002,
`redis_integration_test.go`=999003, `mysql_integration_test.go`=999004,
`mongodb_integration_test.go`=999005. **Whoever adds the next
Docker-integration test file must pick 999006 or higher** — there is
still no automated guard against this, just this note.

### MySQL (2.1) — `internal/docker/mysql.go`

- Container port `3306/tcp`, data dir `/var/lib/mysql`, image from
  `svc.ImageTag` (e.g. `mysql:8`).
- **Credential mapping** (`storage.Service` has one username/password slot
  shared across all 4 engines, but MySQL's official image distinguishes a
  mandatory root account from an optional regular user): if
  `svc.Username` is nil/empty/exactly `"root"`, connect as root — only
  `MYSQL_ROOT_PASSWORD` and `MYSQL_DATABASE` are set (the image rejects
  `MYSQL_USER=root`). Otherwise, `svc.Username`/`PasswordEncrypted` map to
  `MYSQL_USER`/`MYSQL_PASSWORD`, and `PasswordEncrypted` is *also* reused
  as `MYSQL_ROOT_PASSWORD` since the image requires a root password
  unconditionally and `Service` has no separate root-password field —
  practical effect: root and the regular user share one password.
  `MySQLConnectionString`'s fallbacks mirror this (nil username → `"root"`,
  nil db → `"mysql"`).

### MongoDB (2.2) — `internal/docker/mongodb.go`

- Container port `27017/tcp`, data dir `/data/db`, image from
  `svc.ImageTag` (e.g. `mongo:7`).
- `MONGO_INITDB_DATABASE` is **omitted entirely** when `svc.DBName` is
  nil/empty (not defaulted) — unlike Postgres, Mongo doesn't need a
  database name upfront; databases are created lazily on first write.
- `MongoConnectionString`'s fallback path segment is **`"admin"`**, not a
  cosmetic placeholder — it's the actual database the root user
  (`MONGO_INITDB_ROOT_USERNAME`) authenticates against, so the generated
  string is functionally correct for login.
- **Test-environment gotchas worth knowing**: the official `mongo:7`
  image briefly runs a no-auth `mongod` for init setup, then restarts as
  the real auth-enabled `mongod` — the TCP port opens before this
  finishes, so a test that stops the container too early can hit a
  spurious "No such container." Also, `ContainerRemove(Force: true)` can
  race with a container's `RestartPolicyUnlessStopped` on this
  Windows/Docker Desktop setup, producing transient "removal already in
  progress"/volume-in-use errors — the same `RestartPolicy` exists in the
  Postgres container spec already, so this is a latent risk there too,
  just unobserved so far. Retry-with-timeout helpers in
  `mongodb_integration_test.go` work around this for test cleanup; product
  code that ever needs to force-remove a container/volume synchronously
  should expect the same race.

### Redis (2.3) — `internal/docker/redis.go`

- Container port `6379/tcp`, data dir `/data`, image from `svc.ImageTag`
  (e.g. `redis:7-alpine`).
- **No-auth when `PasswordEncrypted` is nil.** Redis's official image has
  no `REDIS_PASSWORD` env var; auth requires overriding `Cmd` to
  `redis-server --requirepass <password>`. With no password set, the
  container runs with zero authentication — a real security-vs-convenience
  tradeoff (an unauthenticated Redis on a bound host port is reachable by
  anything on the machine/LAN that can hit that port) worth revisiting
  before ship, even though it matches the "local dev, zero friction" ethos
  Postgres's nil-credentials path already has.
- `svc.DBName` and `svc.Username` are both fully ignored for Redis (Redis
  "databases" are numbered indices selected per-connection, not
  provisioned at container-start; Redis has no username concept at all).
- `RedisConnectionString` omits the trailing `/db` segment entirely
  (rather than defaulting to `/0`) so the string never implies a database
  selection Stackyard didn't actually make.

### Profile duplicate/rename/delete (2.5) — `app.go` + `EnvironmentManagerView.tsx`

- **Duplicate naming**: `"<original> (copy)"`, falling back to
  `"(copy 2)"`, `"(copy 3)"`, ... on collision (`profiles.name` is
  `UNIQUE`).
- **Duplicate volume names are regenerated, not copied verbatim** —
  copying `VolumeName` as-is would make the duplicate silently mount the
  *same* Docker volume as its source (permanent, silent data sharing, not
  just a start-time port conflict like the host-port field, which IS left
  as-is since task 1.5's `CheckProfilePortConflict` already handles that).
  New volume name follows `CreateProfile`'s existing convention:
  `stackyard-vol-profile-<newID>-<engine>`.
- **Delete-while-running is refused, not silently orphaned.**
  `DeleteProfile` requires `GetProfileStatus` to read exactly `"stopped"`
  before deleting SQLite rows; `"running"`/`"partial"`/`"unknown"`
  (including when the Docker status check itself errors) all block
  deletion with a clear message — an orphaned running container with no
  UI reference left is worse than an explicit "stop it first" error. The
  decision logic itself (`deleteProfileGuardError`) is a pure,
  dependency-free function so it's unit-tested without live Docker; the
  one Docker touchpoint (`GetProfileStatus`) is read-only — `DeleteProfile`
  performs zero Docker *mutations*, matching spec.md §3.1's volume
  guarantee exactly. If a stricter "zero Docker calls whatsoever" reading
  is ever wanted, the guard would need to move entirely into the frontend
  using the status it already polls for display.
- Delete confirmation is a native `window.confirm(...)` whose copy states
  explicitly that Docker volumes are NOT deleted and points at "Reset
  volume" (task 2.6, not yet built) as the actual data-erasing action; the
  Delete button is also disabled unless status is `"stopped"`. Rename is
  an inline edit (click → text input, Enter/Escape, Save/Cancel) — no
  modal library.

### Docker stats polling (2.7) — `internal/docker/stats.go`

- Used `ContainerStatsOneShot` (not `ContainerStats(..., stream=false)`)
  — the SDK's purpose-built single-snapshot call skips a daemon-side
  cgroup-priming delay that the streaming variant incurs even in
  non-stream mode, which matters for spec.md §3.5's ≤2s refresh target
  once polling many containers.
- **CPU% formula** (the same one `docker stats` itself uses):
  `cpuPercent = (cpuDelta / systemDelta) * onlineCPUs * 100`, where
  `cpuDelta`/`systemDelta` are deltas between the current and previous
  cgroup CPU counters. Computed in `float64` specifically to avoid
  unsigned-integer underflow wraparound on a counter reset; `cpuDelta <= 0`
  or `systemDelta <= 0` both return `0` rather than dividing by zero or
  reporting nonsense. `onlineCPUs` falls back from `online_cpus` (Linux
  cgroup-specific) to `len(percpu_usage)` to a hardcoded `1`.
- **Memory formula**: `MemoryUsageBytes = mem.Usage - inactive_file_cache`
  (tries cgroup v1's `total_inactive_file`, then cgroup v2's
  `inactive_file`) — matches `docker stats`, which subtracts reclaimable
  page-cache pages so the number reflects real application memory
  pressure, not incidental disk cache.
- **Batch polling (`StatsForContainers`) returns no top-level error** —
  only a `map[string]ContainerStatsResult`, where each entry independently
  carries its own `Usage`/`Err`. A container that's gone or errors doesn't
  block the batch or get silently dropped from the map; task 2.8's
  dashboard can tell "this service errored" apart from "this service was
  never requested," which a silently-omitted entry would obscure.

---

## Session 3 continued — Multi-engine profile wizard (2.4)

`CreateProfile` now accepts `(name string, services []ServiceRequest)`
where `ServiceRequest{Engine storage.Engine, HostPort int}` (`HostPort: 0`
means auto-default) — supports any combination of 1-4 engines in one
profile, rejecting empty lists and duplicate engines in the same call (not
explicitly required by spec.md, but implied by "any combination of the 4
engines" — a profile isn't specified to need two Postgres services).

**Start/stop/status dispatch across heterogeneous services**: a
`map[storage.Engine]func(*docker.Client, context.Context, storage.Service) error`
built from Go method expressions (`(*docker.Client).StartPostgresEnvironment`
etc., not bound method values) so the dispatch table is reflect-comparable
and unit-testable without a live Docker client. `StartProfile` loops a
profile's services and starts each through this table — a profile mixing
e.g. Postgres+Redis starts/stops both as a unit from one click.
`StopProfile`/`GetProfileStatus` needed no changes — they were already
container-name-only, hence already engine-agnostic. `GetConnectionString`
now dispatches to the right `<Engine>ConnectionString` builder (was
Postgres-only before this task).

**Default port assignment**: each engine gets its OS-standard default
(Postgres 5432, MySQL 3306, MongoDB 27017, Redis 6379) via
`assignHostPorts`, a pure/DB-free function, bumping past any port already
recorded by another Stackyard-managed service (same self-collision-avoidance
philosophy as task 1.4's original `nextFreeHostPort`, extended to 4 base
ports instead of 1). An explicit `HostPort` in a `ServiceRequest` is
honored as-is.

**Per-engine defaults for MySQL/MongoDB/Redis follow the patterns their
own tasks (2.1/2.2/2.3) already established**, not new decisions: MySQL
and MongoDB get explicit default usernames/passwords (root, explicit
password), matching Postgres's existing explicit-credentials default;
Redis stays password-nil by default (the "zero-friction local dev"
behavior `redis.go`'s doc comment already documents as intentional, not
an oversight — a user can add a password after creation); Mongo's
`DBName` stays nil by default, matching `mongodb.go`'s "omit entirely,
don't default" `MONGO_INITDB_DATABASE` behavior.

**Create & Start stayed one combined button** (not split into separate
Create/Start steps) — preserves the exact UX pattern the task 1.7 manual
pass already validated and timed.

**New file `internal/docker/cleanup.go`**: `RemoveContainer`/`RemoveVolume`/
`RemoveNetwork` on `*docker.Client`. Added because the new multi-engine
integration test needed real teardown capability a package-`main` test
can't get by reaching into `docker.Client`'s unexported `cli` field the
way same-package (`internal/docker`) integration tests do. This is also
exactly the primitive task 2.6 ("Reset volume") will need — that task
should reuse `RemoveVolume` rather than reimplementing it.

**Test-ID note**: the new `profile_multiengine_integration_test.go` uses
999006/999007 — next new integration test file should use 999008+ (see
the running note on this earlier in this document; still no automated
guard against collisions, just this convention).

**Manual verification note**: `wails dev` was launched for real (native
window opened, both dev servers responded), but no browser-automation
tool was available to this particular subagent invocation, so the wizard
UI itself was not click-tested this round — confirmed instead via a
throwaway Go program that the real app-data SQLite DB has zero leaked
profiles from this session. The actual wizard UI click-through should
still get a manual pass before Phase 2 is considered fully closed,
similar to task 1.7's pass for Phase 1.

---

## Session 3 continued — Reset volume (2.6) and status dashboard (2.8)

Both tasks ran in parallel and both landed clean, but they concurrently
edited `app.go` (2.6 added `ResetServiceVolume`, 2.8 added
`StartStatusWatcher`/`StopStatusWatcher` plus new imports) — worth
flagging as a process note even though the merge turned out coherent:
**two parallel tasks editing the same shared file is a real collision
risk**, tolerable here only because both diffs were additive and the
final `go build`/`go vet`/`go test ./...` (including the full
`-tags=integration` suite) were reverified clean *after* both landed, not
assumed clean from each task's own isolated report.

### Reset volume (2.6) — `app.go` + `reset_volume_integration_test.go`

- `ResetServiceVolume(serviceID int64) error`: stop → `RemoveContainer` →
  `RemoveVolume` → recreate via `startServiceEnvironment` (the same
  dispatch `StartProfile` uses — note this replaced whatever table task
  2.4 first introduced; the name in code is `startServiceEnvironment`,
  not "`engineStarters`" as earlier notes in this document called it —
  if a name mismatch is confusing later, this is why).
- **Volume removal requires removing the container first**, not just
  stopping it — Docker refuses `volume rm` while a stopped container
  still references it. This is why the sequence is stop→remove
  container→remove volume, not stop→remove volume.
- **Sibling isolation was proven, not assumed**: the integration test
  starts a target service plus a sibling in the same profile, polls the
  sibling's container state every 150ms *while* the reset runs on the
  target, and confirms the sibling stayed `running` throughout — this is
  spec.md §3.4's core acceptance criterion, verified under concurrent
  load, not just "the code doesn't touch the sibling's ID."
- **Freshness of the recreated volume was proven** via a marker value
  written before reset (through a hand-rolled minimal RESP client — no
  Redis driver exists in `go.mod` yet since Phase 3+ hasn't started) that
  was confirmed gone after the reset.
- Test IDs 999008/999009 — the next new integration test file should
  grep the whole repo for every `9990\d\d` literal first (the running
  convention noted earlier in this document had already drifted once by
  the time this task started; don't trust the last-recorded number
  alone).

### Real-time status dashboard (2.8) — `internal/docker/snapshot.go` +
`StatusDashboard.tsx`

- **Event contract**: Wails event `"environment:status"`, emitted every
  ~1.5s (under spec.md §3.5's ≤2s target). Payload has no JSON tags, so
  keys arrive PascalCase on the frontend:
  `{"Profiles":[{"ProfileID","ProfileName","Services":[{"ServiceID","ServiceName","Engine","EngineVersion","State","HostPort","CPUPercent","MemoryUsageBytes","MemoryLimitBytes","MemoryPercent","StatsAvailable"}]}]}`.
- **Poller lifecycle**: `StartStatusWatcher()`/`StopStatusWatcher()` bound
  methods; a mutex-guarded running flag + stored `context.CancelFunc` +
  `sync.WaitGroup`. `Start` is idempotent (calling it twice doesn't spawn
  two pollers); `Stop` cancels the context and blocks on `wg.Wait()` so
  no goroutine outlives the call. `shutdown()` calls `StopStatusWatcher()`
  before closing the DB/Docker clients, so there's no window where the
  poller could touch a closed Docker client.
- **Watching starts lazily on dashboard mount**, not in `startup(ctx)` —
  deliberate: Docker isn't polled every ~1.5s while the user is in the DB
  Client module with the dashboard never opened.
- **"Reflects containers stopped outside the app" was proven for real**:
  the integration test starts a container, confirms a snapshot poll
  reports it running with plausible stats, stops it via a direct
  `exec.Command("docker","stop",...)` call that bypasses the app's own
  `StopProfile` entirely (simulating a user running `docker stop` from a
  separate terminal), and confirms the next poll reports it stopped.
- **Dashboard placement**: a third top-level sidebar item ("Status"),
  since `EnvironmentManagerView.tsx` was off-limits (task 2.6 was
  concurrently making additive edits there). Clicking a running service
  row reveals its connection string inline (distinct UX from the
  existing copy-to-clipboard button in the Environments view, per
  spec.md §3.5's specific "reveals... inline" wording vs. §3.3's
  "copies to clipboard" wording for the other view).
- **Known minor gap, not fixed**: if a service's connection-string row is
  expanded on the dashboard and that service then stops, the row doesn't
  auto-collapse. Cosmetic; flagged for an optional follow-up.

---

## Phase 2 — manual verification pass (all of 2.1-2.8, real click-through)

Same approach as task 1.7: drove the real running app (`wails dev`) with
Playwright against `http://localhost:34115`, not simulated.

- **Multi-engine wizard**: checked PostgreSQL + Redis, named the profile,
  clicked "Create & Start" once — both services came up under one
  aggregate "Running" badge, each with its own row (engine, port, its own
  "Copy connection string" and "Reset volume" buttons). Confirms the
  "multi-service start/stop as a unit" requirement visually, not just via
  the Go-side integration test.
- **Reset volume**: clicked "Reset volume" on the Postgres row. Confirm
  dialog text (verified verbatim):
  *"Reset volume for PostgreSQL (localhost:5432)? This PERMANENTLY
  DELETES all data in this service. It will be stopped, its Docker
  volume erased, and a fresh empty one created on next start. This
  cannot be undone. Other services in this profile are not affected and
  stay running."* — settled in ~2.1s. The Redis row never left
  "Running" throughout, and the profile's aggregate status stayed
  "Running" — sibling-isolation confirmed visually, matching the
  integration test's concurrent-polling proof.
- **Status dashboard**: navigated via the new "Status" sidebar item —
  showed a live table with both services (`postgres`/PostgreSQL and
  `redis`/Redis), correct ports (5432/6379), real CPU%/RAM readings
  (e.g. "29.1 MiB / 6.72 GiB (0.4%)"), no manual refresh needed.
- **Stop**: clicked "Stop" once on the multi-engine profile — both
  services stopped as a unit within ~1.05s.
- All Docker resources (2 containers, 1 network, 2 volumes) and the test
  profile row were removed afterward — confirmed via
  `docker ps -a`/`network ls`/`volume ls` and the same throwaway
  `internal/storage`-based cleanup program pattern used for task 1.7.

Phase 2 (tasks 2.1-2.8) is confirmed working end-to-end, not just
unit/integration tested in isolation. `tasks.md`'s Phase 2 checkboxes are
all checked.

---

## Session 3 close-out — current phase, last task, next steps

**Current phase:** Phase 2 (Environment Manager, Full) is complete and
closed — `tasks.md` 2.1-2.8 all checked, manually verified end-to-end
(see the "Phase 2 — manual verification pass" section directly above).
Per `plan.md` §6, this closes **Module 1 — Environment Manager** in full
(all 4 engines, profile CRUD, volume reset, live status/stats dashboard).

**Last task completed:** 2.8 (real-time status dashboard), followed by
the manual Phase 2 verification pass covering all of 2.1-2.8 together.

**In-flight / undecided items carried forward (not blockers, just
flagged):**

- Redis's no-auth-by-default behavior (task 2.3) is a real
  security-vs-convenience tradeoff worth revisiting before ship — an
  unauthenticated Redis on a bound host port is reachable by anything on
  the machine/LAN that can hit that port.
- Docker's `ContainerRemove(Force: true)` racing with
  `RestartPolicyUnlessStopped` (observed during MongoDB task 2.2's
  integration testing) is a latent risk in the Postgres container spec
  too, just unobserved there so far — no fix applied, just documented.
- The status dashboard's connection-string row doesn't auto-collapse if
  its service stops while expanded — cosmetic, optional follow-up.
- Integration-test container-ID collisions (the `9990\d\d` convention)
  have no automated guard — still just a convention documented in this
  file; the next new integration test file should grep the whole repo for
  every `9990\d\d` literal before picking a number, not trust the
  last-recorded one.

**Command to run the app locally:**

```
cd D:\CODE\projects\Stackyard
wails dev
```

(Unchanged since Phase 0 — see the pnpm/`wails.json` gotcha noted in
Session 1 if this fails with an `EUNSUPPORTEDPROTOCOL`-style error.)

**Run tests:**

```
cd D:\CODE\projects\Stackyard
go test ./...
go test -tags=integration ./internal/docker/...
```

**Next steps:** Phase 3 — DB Client MVP (Postgres + MySQL only, shared
grid code): `internal/dbengine` `Engine` interface, connection-string
parsing (`urlparse.go`), connection form UI, Monaco editor integration,
read-only results grid, multi-tab shell (tasks 3.1-3.8). This is the
first Module 2 (DB Client) work and the first place frontend logic
non-trivial enough to warrant a Vitest suite is expected to appear (see
Session 1's testing note).

**Standing to-do, not yet scheduled to a specific task**: `plan.md` §4
commits to encrypting passwords at rest ("never stored plaintext, even
though this is a local-only tool"). This is still unimplemented —
`Service.PasswordEncrypted`/`Connection.PasswordEncrypted` hold whatever
plaintext value is written to them; every engine's container-spec
builder (`compose.go`, `mysql.go`, `mongodb.go`, `redis.go`) and
connection-string builder treats the field as already-usable plaintext,
each with its own comment/report noting this as a known gap owned by
"whichever task ends up owning credential storage properly." No task in
`tasks.md` 1.1-9.4 explicitly names this — it should get a real task
slot (most naturally either late in Phase 2's aftermath or during Phase
9's polish pass) before v1 ships, rather than continuing to be a
distributed TODO scattered across 4 files with no single owner.
