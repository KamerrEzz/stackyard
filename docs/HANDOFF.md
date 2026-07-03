# Stackyard — v1 Handoff

This document is the final deliverable of the autonomous build session that
took Stackyard from an empty repository through all 9 phases of
`tasks.md`. It is written for two audiences equally: the human maintainer
picking this up weeks later, and a future Claude session with zero memory
of this one. Read this before re-reading any code.

For the full, session-by-session narrative (every judgment call, every
bug found and fixed, every real test run's raw output), see
`docs/STATE.md` — this document is a synthesis of it, not a replacement.
`docs/STATE.md` is the primary source; if the two ever disagree, trust
`docs/STATE.md`'s dated entries over this summary.

---

## 1. Real state vs. spec.md §7's success definition

> *"v1 is 'done' when the developer's own real-world workflow — starting
> a new side project — goes through Stackyard end to end at least once:
> create a profile, start an environment, connect via the DB Client, run
> and save a few snippets, and tear it down — without ever opening Docker
> Desktop, a terminal, or another DB client."*

**This was personally driven end-to-end via `wails dev` + Playwright in
Session 19 (`docs/STATE.md`, "Session 19"), not just asserted.** Every
step below was genuinely exercised against a real Docker container, not
mocked:

| Step | Result |
|---|---|
| Create a profile | 3 UI interactions (name, engine checkbox, "Create & Start") — matches spec.md §3.2's 3-click bar |
| Start an environment | Reached "Running" in ~1.6s after the create click — real Docker orchestration, not an optimistic UI state |
| Connect via DB Client | "Test connection" → "Connected successfully" → saved → "Load" opened a real query-editor tab |
| Run queries | `CREATE TABLE`, `INSERT`, `SELECT` all executed against the live container; results reflected genuine database state |
| Save and run a snippet | Created "list all notes," clicked "Run" (loads into the tab per task 4.7's actual design — doesn't auto-execute), then "Run query" executed it and returned the real row |
| Tear down | Stop → Delete, with an accurate confirmation dialog disclosing that the Docker volume is preserved, not deleted |

All of this happened without opening Docker Desktop, a terminal, or
another DB client. **The core promise of the project holds up under an
actual run-through**, not just isolated per-phase tests.

### What's built, phase by phase (all with real Docker/Playwright verification, not just unit tests)

- **Module 1 — Environment Manager** (Phases 1-2): profile CRUD for all
  4 engines (Postgres, MySQL, MongoDB, Redis) in any combination, one
  Docker container/volume/network per service with no compose file ever
  written to disk, port-conflict pre-check, volume reset with
  confirmation, live status/stats dashboard reflecting containers
  stopped outside the app.
- **Module 2 — DB Client** (Phases 3-8): a shared multi-tab shell across
  all 4 engines (`SqlTab | MongoTab | RedisTab` discriminated union).
  - **Relational** (Postgres/MySQL): Monaco editor with cancellable
    queries, editable results grid (insert/update/delete with real
    error surfacing), multi-statement execution, query history,
    snippets (CRUD + Run), schema-based autocomplete, a Schema Diagram
    (FK-derived Mermaid ER diagram with zoom/pan/export).
  - **MongoDB**: a document-oriented Engine (deliberately not
    implementing the SQL-shaped `Engine` interface), tree/JSON document
    viewer with in-place editing, create (blank/duplicate), delete,
    collection browser with a working filter bar, an inferred-structure
    Schema Diagram (samples documents, reports every observed type
    rather than collapsing to "mixed," labeled "inferred, not
    enforced").
  - **Redis**: a key-value Engine, pattern-based key scan (cursor-based
    `SCAN`, never blocking `KEYS`), per-type views for all 5 data types,
    TTL display/set/persist, key rename (collision-guarded) and delete.
  - **Import/Export**: CSV/JSON/SQL-dump export for a full table or the
    current query result; CSV/JSON import with pre-commit validation
    that's a genuine hard guarantee (a bad row anywhere in the file
    leaves zero rows committed, verified via direct `SELECT COUNT(*)`).
  - **Migrations** (Postgres/MySQL only, per spec.md §4.8's own scope):
    create/apply/rollback with atomic per-migration commit of both the
    schema change and its tracking row, a dedicated top-level UI panel.
- **Polish** (Phase 9): performance measured and recorded (not
  guessed), a real cross-module visual bug found and fixed (see
  ambiguities log below), the dogfood run above. **The Windows installer
  (task 9.3) is the one incomplete item** — see §4/§5 below.

### Test suite coverage areas (detail in §3)

Every phase's business logic has real Go tests (unit + `-tags=integration`
against live Docker containers) and every non-trivial frontend
transform/state-machine has real Vitest tests — no placeholder tests
anywhere in the codebase.

---

## 2. Every self-resolved ambiguity, for you to confirm or correct

Nothing below was silently decided and buried — each is flagged here
specifically so you can override it if it's wrong. Full rationale for
each lives in `docs/STATE.md` at the session noted.

### Architecture-level (affect the whole codebase)

1. **MongoDB and Redis each get their own Engine interface, not
   `dbengine.Engine`.** `dbengine.Engine` is SQL-shaped (`Query`,
   `ListSchemas`, `ListTables`); document/key-value stores don't fit it.
   Each got a parallel session map in `app.go` (`mongoSessions`,
   `redisSessions`, alongside the original `querySessions`) rather than
   one polymorphic abstraction across three genuinely different data
   models. *(Sessions 8, 10.)*
2. **Migrations use an optional `dbengine.Transactor` interface, not a
   breaking change to `dbengine.Engine`.** Adding `BeginTx` directly to
   `Engine` would have forced every existing test double (and the
   Mongo/Redis engines, which don't implement `Engine` at all) to
   implement it. `postgres.Engine`/`mysql.Engine` type-assert against
   the optional interface instead. *(Session 14.)*
3. **Wails v2.12.0 bound methods cannot return 3 values — this is now a
   hard, load-bearing rule for any future bound method.** Verified
   directly in the vendored source
   (`internal/binding/boundMethod.go:88-106`): the dispatcher's
   `OutputCount()` switch only handles 1 or 2, silently returning
   `nil`/`nil` to JS for anything else, with zero build or runtime
   error. Any method needing 2 data values plus an error must wrap the
   data in a result struct (e.g. `ScanKeysResult`, `RedisSetPage`,
   `ImportCommitResult`). *(Session 10 — read this before adding ANY new
   bound method.)*
4. **Stackyard's own local SQLite migration scheme (`PRAGMA
   user_version`) and Phase 8's `internal/migrations` (for the user's
   TARGET database) are deliberately two unrelated systems** — conflating
   them would be a real design mistake. *(Session 1, `sqlite.go`'s own
   doc comment calls this out.)*

### Data-model judgment calls (Session 1, Phase 0 — SQLite schema)

5. `profiles.name` is `UNIQUE` (inferred from spec.md §3.1's
   rename/duplicate language) — relax if duplicate-named profiles should
   be legal.
6. `engine` columns use a `CHECK` constraint restricted to the 4
   supported engines — a 5th engine needs this updated alongside a
   migration.
7. `snippets.connection_id` is `ON DELETE SET NULL` (demotes to global);
   `query_history.connection_id` is `ON DELETE CASCADE` (deleted with
   its connection) — genuinely different judgment calls, `plan.md`
   didn't specify either.
8. Timestamps stored as ISO-8601 `TEXT`, not SQLite's native `DATETIME`.
9. `db.SetMaxOpenConns(1)` on the local SQLite pool — avoids
   `SQLITE_BUSY` under `modernc.org/sqlite`'s pooled-writer limitations.

### Feature-scope narrowings (each a deliberate choice, not an oversight)

10. **SQL-dump export is scoped to full-table only**, not the
    current-query-result scope CSV/JSON export both support — an
    arbitrary query result can join multiple tables (no single `CREATE
    TABLE` target) and would risk violating spec.md's literal
    "importable into a fresh instance" requirement. *(Session 12.)*
11. **The CSV null-vs-empty-string convention is Postgres's own `COPY
    CSV` convention**: an unquoted-empty field is NULL, a quoted-empty
    `""` is an empty string. Chosen specifically so import can reverse
    it unambiguously — both the export and import tasks independently
    converged on this exact convention. *(Session 12.)*
12. **Import is a hard block on any validation mismatch, no
    soft-confirm-and-proceed option** — `ImportFile` fully re-validates
    from scratch immediately before writing, and uses one atomic
    bulk-`INSERT` (not N calls to a per-row insert) so the guarantee
    holds even against DB-level constraints the validator can't see.
    *(Session 12.)*
13. **Migration files must contain exactly one SQL statement** — neither
    pgx's default protocol nor MySQL's default DSN config support
    multi-statement `Exec`. A real, unsolved v1.1-candidate limitation,
    not silently ignored — flagged explicitly since real migrations
    often need more than one statement. *(Session 14.)*
14. **Migration Rollback is gated by a confirmation dialog** even though
    spec.md §4.8 doesn't explicitly require one (only Delete-type
    operations are called out elsewhere) — judged appropriate since
    reverting a migration is a real, hard-to-undo action against a live
    schema, matching this project's established destructive-action
    pattern. *(Session 15.)*
15. **Hash/list/set/sorted-set editing (Mongo and Redis both) uses
    bulk/whole-value replace, not per-field/per-element editing** —
    mirrors the simpler whole-document JSON-edit pattern rather than
    building granular diff-based editors. A real, documented scope
    reduction. *(Sessions 9, 11.)*

### A self-corrected mistake, not a pre-existing ambiguity

16. **Session 19's dogfood notes originally claimed the saved-connection
    row's text and its "Load" button both open a tab — this was false.**
    An independent qa-reviewer pass in Session 20 caught it: the row is
    a plain `<div>` with no click handler at all (confirmed unchanged
    since Phase 6); only "Load" does anything. Corrected in both
    `docs/STATE.md` and `CHANGELOG.md`. The real finding is arguably
    *worse* than originally written — the row visually suggests it's
    clickable but isn't — logged as a v1.1 candidate. This is flagged
    here specifically as a caution: even this document's own source
    material had one factual error that only surfaced under adversarial
    review, so treat every claim in `docs/STATE.md` as "verified once,"
    not "infallible."

### v1.1 backlog (explicitly logged, not fixed mid-flight, per the dogfood task's own instruction)

- Saved connections have no uniqueness guard on name — repeated saves
  silently create duplicate rows.
- The saved-connection row visually suggests it's clickable but isn't —
  only "Load" works (see #16 above).
- Query History requires a manual "Refresh" click to show queries just
  run in the same session (consistent with this app's existing
  no-live-polling design elsewhere — e.g. Schema Diagram's "Regenerate"
  — so not a bug, but worth a UX look).
- `TestConnection`/`newTestEngine` still return "not yet supported" for
  MongoDB and Redis (only the tab-open path — `OpenMongoConnection`/
  `OpenRedisConnection` — validates reachability; there's no
  pre-save "Test Connection" button feedback for these two engines the
  way there is for Postgres/MySQL).
- MongoDB's mongo-driver dependency is pinned to v1; the module itself
  recommends v2 — not upgraded, flagged for a future pass.
- Password-encryption-at-rest (`plan.md`'s own stated intent — OS
  keychain or AES-256 local key file) was never implemented; connection
  passwords are stored in cleartext in Stackyard's local SQLite. This
  gap was flagged as early as Phase 2 and never picked up by a specific
  task — it should be treated as a real pre-v1-ship security gap, not a
  cosmetic backlog item, if this app will ever store credentials for
  connections a user cares about protecting.
- Redis defaults to no-auth (matching Postgres's zero-friction local-dev
  default) — a deliberate v1 choice, not a gap, but worth knowing.
- MySQL's `BOOLEAN` reports as `tinyint` in `information_schema`,
  indistinguishable from a genuine tinyint column for import
  validation — only `0`/`1` passes there, not `"true"`/`"false"`.

---

## 3. Test suite results

**All numbers below were re-run fresh at the end of this session** (not
carried over from earlier claims) — see the final commits' verification
passes.

### Go

```
go build ./...      → clean
go vet ./...         → clean
gofmt -l .            → no output (fully formatted)
go test ./...          → ok, all 13 packages
go test -tags=integration ./...  → ok, all 13 packages (real Docker containers, self-cleaning)
```

13 packages: `stackyard` (root), `internal/dbengine`,
`internal/dbengine/{mongo,mysql,postgres,redis}`, `internal/diagram`,
`internal/docker`, `internal/export`, `internal/importdata`,
`internal/migrations`, `internal/netcheck`, `internal/storage`.

466 individual Go test cases pass under plain `go test ./...` (unit
tests only; the integration suite adds real-container tests on top,
gated behind the `integration` build tag specifically because they
require Docker and take ~80s+ to run).

Coverage areas: every `internal/` package's business logic (URL
parsing, batch/multi-statement SQL splitting, BSON/CSV/SQL-dump
conversion, migration file discovery/versioning, TTL sentinel
translation, port-conflict detection, Docker orchestration), plus
`app.go`'s session-management bookkeeping (open/close/lookup for all 3
session-map families, error paths for unknown sessions). Every
integration test tears down its own Docker resources in `t.Cleanup` —
confirmed zero leftover containers/networks/volumes after every full
suite run this session.

### Frontend (Vitest)

```
pnpm run build   → clean (only a pre-existing >500KB chunk-size advisory, unrelated to correctness)
pnpm run test     → 202/202 passing, 15 test files
```

Coverage areas: URL/connection-form validation, grid edit logic,
multi-statement result collapsing, snippet filter/run logic, query
history helpers, schema-diagram Mermaid generation + real
`mermaid.parse()` validation, Mongo document helpers (BSON display
heuristics, JSON validation), Redis key helpers (TTL formatting, cursor
pagination), export/import helpers (scope/payload construction,
validation-report formatting), migration helpers (pending/applied
cross-referencing).

### What was NOT covered by automated tests (honestly disclosed)

- No end-to-end/UI-automation test suite exists as a checked-in project
  asset — every "manual verification" claim in `docs/STATE.md` came
  from an ad-hoc Playwright script driven interactively during this
  session, not a repeatable CI-style E2E suite. If you want regression
  protection against UI breakage, that's a real gap to close.
- MySQL's import path has no live integration test (Postgres-only
  integration coverage for task 7.4/import) — MySQL's bulk-insert SQL
  dialect is unit-tested only.
- No load/stress testing was performed on any feature.

---

## 4. Proposed version tags, in order (none executed — for you to run)

Confirmed via `git tag -l`: **zero tags exist in this repository.**
Every tag below is a proposal only, pinned to the exact commit where
that phase's work actually closed. Run them in order if you want the
tag history to read cleanly, though `git tag` doesn't require that —
each is just a named ref to an already-existing commit.

```bash
git tag -a v0.1.0 -m "Phase 1: Environment Manager MVP (Postgres-only start/stop/restart, connection string copy)" e743c6b
git tag -a v0.2.0 -m "Phase 2: Environment Manager, full (MySQL/MongoDB/Redis orchestration, multi-engine wizard, profile duplicate/rename/delete, reset volume, live status/stats dashboard) - completes Module 1" 92ff4bc
git tag -a v0.3.0 -m "Phase 3: DB Client MVP for Postgres+MySQL (Engine interface, connection-string parser, connection form, saved connections, Monaco editor with cancellable queries, typed results grid, multi-tab shell)" c89a91a
git tag -a v0.4.0 -m "Phase 4 + 4.5: Relational DB Client, complete (editable grid, multi-statement execution engine at the Go layer, query history, snippets CRUD + Run snippet, Monaco autocomplete) and Schema Diagram for Postgres/MySQL (FK introspection, Mermaid erDiagram generation, zoom/pan, PNG/SVG export) - completes Module 2's relational feature set" 749f127
git tag -a v0.5.0 -m "Phase 5: MongoDB support (document-oriented Engine via mongo-go-driver, unified multi-tab shell shared with SQL connections, document tree/JSON viewer with in-place editing/create/delete, collection browser with filter bar, inferred-structure Schema Diagram) - completes Module 2's DB Client feature set for every engine except Redis" 2b568ff
git tag -a v0.6.0 -m "Phase 6: Redis support (key-value Engine via go-redis/v9, all 5 data types, cursor-based SCAN, TTL display/edit/persist, key rename/delete) - completes Module 2, DB Client, in full for all 4 engines" 0d0197f
git tag -a v0.7.0 -m "Phase 7: Import/Export (CSV/JSON/SQL-dump export for full-table and current-query-result scope, CSV/JSON import with pre-commit validation and atomic bulk-insert), verified via real round-trip tests against fresh Postgres and MySQL instances" 225c80f
git tag -a v0.8.0 -m "Phase 8: Migrations for Postgres+MySQL (create-migration scaffolding, schema_migrations tracking table, atomic Apply/Rollback via a new optional dbengine.Transactor interface, Migrations UI panel with folder-picker and pending/applied status), manually verified end-to-end including direct database-level checks" e056136
```

**`v1.0.0` is intentionally NOT proposed above.** Phase 9 is not fully
closed — task 9.3 (Windows installer) remains blocked (see §5). Once
you resolve that blocker and confirm the installer builds/smoke-tests
cleanly, `v1.0.0` should be tagged at that point's closing commit, not
retroactively at `e19f0d6` (this session's last commit) — the actual v1
ship commit should be the one where 9.3 genuinely finishes.

---

## 5. Resuming this

### Run the app locally (development)

```bash
cd D:\CODE\projects\Stackyard
wails dev
```
Opens a real native window backed by a live Go process; also reachable
at `http://localhost:34115` in a browser (with the real IPC bridge to
Go) for tooling like Playwright. Requires Docker Desktop running for
any Environment Manager / DB Client feature that touches a real
database.

**Known quirk**: if you kill `wails dev`'s process tree directly (rather
than its own graceful exit), the gitignored `frontend/dist/` can end up
empty, which then breaks `go build ./...` via its `//go:embed
frontend/dist` directive. Fix: `cd frontend && pnpm run build` to
regenerate it, then `go build ./...` again.

### Build a production binary

```bash
wails build
```
Produces `build/bin/stackyard.exe` (last measured at ~47.5 MiB).
Production-mode cold-start averaged ~913ms; idle memory ~58MB for the
main process alone, ~267MB private across the full WebView2 process
tree (7 processes total) — see `docs/STATE.md` Session 18 for full raw
data and methodology.

### Run the full test suite

```bash
# Go — unit tests (fast, no Docker required)
go test ./...

# Go — integration tests (requires Docker Desktop running; ~80s+)
go test -tags=integration ./...

# Frontend
cd frontend
pnpm run build
pnpm run test
```

If you add ANY new integration test with a hardcoded Docker host port,
grep BOTH conventions before picking one — these are two independent
number spaces that have collided before in this project:
```bash
grep -rn "9990\d\d" --include="*.go" .        # test/profile/service IDs
grep -rn "HostPort\s*=\s*[0-9]\{4,5\}" --include="*.go" .   # hardcoded ports
```
(Session 14 found and fixed a real flaky-test bug from checking only
the first convention.)

### Resolve the one open blocker: task 9.3, Windows installer

NSIS is required for Wails' Windows installer packaging and is not
installed on the machine this session ran on. A non-interactive
`winget install --id NSIS.NSIS -e --silent` was attempted and verified
safe (official package, hash-checked) but stalled at a UAC elevation
prompt this session couldn't approve non-interactively.

To unblock:
1. Open an **elevated** ("Run as Administrator") terminal.
2. `winget install --id NSIS.NSIS -e --accept-package-agreements --accept-source-agreements`
   (or install NSIS manually from its official site if you prefer not
   to use winget).
3. From `D:\CODE\projects\Stackyard`: `wails build -nsis`
4. Confirm `build/bin/stackyard-amd64-installer.exe` (or similarly
   named) is produced.
5. Smoke-test it: install to a throwaway location, launch the
   *installed* executable (not `build/bin/stackyard.exe`) to confirm it
   doesn't depend on any dev-only path (`frontend/node_modules`, etc.).
   A real clean VM is the only fully rigorous test of "a machine without
   the dev toolchain" — this session's environment couldn't fully
   replicate that.
6. Once confirmed, tag the resulting commit as `v1.0.0` (see §4).

`wails.json` already has an `info` block prepared (company/product
name, version placeholder, copyright) so the NSIS template has real
values the moment a build becomes possible — no further config change
should be needed before step 3.

### If you're a fresh Claude session picking this up

Read, in this order: this document, then `docs/STATE.md` from the most
recent session backward only as far as you need context (it's long —
~3900 lines across 20 sessions; each session's heading names its phase
and task numbers, so you can jump directly to the relevant one via
`grep -n "^## Session" docs/STATE.md`), then `tasks.md` to confirm which
checkboxes are actually ticked (9.1/9.2/9.4 checked, 9.3 unchecked with
its blocker documented, everything in Phases 0-8 checked). `spec.md` and
`plan.md` are the original requirements/architecture documents and
haven't been modified during implementation — they remain the source of
truth for "what was this supposed to do," while `docs/STATE.md` and this
document are the source of truth for "what actually happened."

Do not re-derive architectural decisions already made and documented
above (§2) — if you disagree with one, say so to the user and get
explicit confirmation before changing it, since each was a deliberate,
reasoned choice, not an oversight waiting to be corrected.
