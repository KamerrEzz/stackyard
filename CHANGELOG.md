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

### Changed

- Retroactive comment-style cleanup across all Go and frontend source
  written in Phases 0-1, per the new `CLAUDE.md` convention: only package
  doc comments and doc comments on exported functions/types/consts
  survive; every inline comment removed. No logic, signatures, or JSX
  changed; `go build`/`go vet`/`go test` and `pnpm run build` stayed green
  throughout. Rationale that was previously inline is preserved in
  `docs/STATE.md`.
