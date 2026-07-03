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

### Fixed

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

### Changed

- Retroactive comment-style cleanup across all Go and frontend source
  written in Phases 0-1, per the new `CLAUDE.md` convention: only package
  doc comments and doc comments on exported functions/types/consts
  survive; every inline comment removed. No logic, signatures, or JSX
  changed; `go build`/`go vet`/`go test` and `pnpm run build` stayed green
  throughout. Rationale that was previously inline is preserved in
  `docs/STATE.md`.
