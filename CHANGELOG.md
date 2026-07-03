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
