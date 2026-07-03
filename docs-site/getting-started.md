# Getting Started

Stackyard is a [Wails](https://wails.io) desktop application: a Go backend
paired with a React/TypeScript frontend, packaged as a single native binary.
Running it from source requires the same toolchain any Wails app needs, plus
Docker for the environments it manages.

## Prerequisites

- **Go** 1.25 or newer
- **Node.js** and **pnpm** (the frontend package manager)
- **Wails CLI** — `go install github.com/wailsapp/wails/v2/cmd/wails@latest`
- **Docker Desktop** (or a local Docker Engine) — running, before you start
  any environment from Stackyard

## Run in development mode

```sh
git clone https://github.com/KamerrEzz/stackyard.git
cd stackyard
wails dev
```

`wails dev` builds the Go backend, starts a Vite dev server for the
frontend, and opens the app window with hot reload enabled. If you'd rather
develop in a browser and inspect the bound Go methods directly, it also
exposes a dev server at `http://localhost:34115`.

## Build a production binary

```sh
wails build
```

This produces a redistributable native binary (see `wails.json` for the
exact output name and platform target).

## Using the app

1. Open **Environments**, name a profile, pick one or more engines
   (PostgreSQL, MySQL, MongoDB, Redis), and click **Create & Start**.
2. Copy the generated connection string, or open **DB Client** and paste
   it directly — Stackyard parses the URL and fills the connection form.
3. Browse the schema tree, edit data in the spreadsheet-style grid, write
   and save queries, or generate a schema diagram — all from the same
   window, without a separate GUI client.

See the [Features](/features/environment-manager) section for a full tour
of each module.
