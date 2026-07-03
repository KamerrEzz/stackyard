package main

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"stackyard/internal/docker"
	"stackyard/internal/storage"
)

// Default values for the single Postgres Service task 1.4's CreateProfile
// creates. Phase 1 MVP scope is Postgres-only (plan.md §6); MySQL/MongoDB/
// Redis defaults arrive with their own engines in Phase 2 (tasks 2.1-2.3).
const (
	defaultPostgresImageTag  = "postgres:16-alpine"
	defaultPostgresHostPort  = 5432
	defaultPostgresUsername  = "postgres"
	defaultPostgresPassword  = "postgres"
	defaultPostgresDBName    = "postgres"
	dockerOpTimeout          = 60 * time.Second
	dockerStopTimeout        = 30 * time.Second
	dockerStatusTimeout      = 15 * time.Second
	dockerStartupPingTimeout = 3 * time.Second
)

// App struct is the ONLY surface bound to the frontend (plan.md §2/§3) —
// every other package stays behind this thin adapter layer.
type App struct {
	ctx context.Context

	db    *sql.DB
	dbErr error

	docker    *docker.Client
	dockerErr error
}

// NewApp creates a new App application struct
func NewApp() *App {
	return &App{}
}

// startup is called when the app starts. The context is saved so we can
// call the runtime methods.
//
// Storage and Docker are both initialized here, but a failure in either one
// does NOT crash the app or panic: spec.md doesn't require Docker (or,
// arguably, storage) to be reachable at app-launch time — only at the point
// a docker-dependent bound method is actually called (e.g. "Start profile").
// Failures are stored on the App struct instead, and every bound method that
// needs storage/Docker checks for that stored error first via
// requireDB/requireDocker, surfacing a real error string to the frontend
// rather than a nil-pointer panic.
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	db, err := storage.Open()
	if err != nil {
		a.dbErr = fmt.Errorf("open local storage: %w", err)
	} else {
		a.db = db
	}

	dockerClient, err := docker.NewClient()
	if err != nil {
		a.dockerErr = fmt.Errorf("construct docker client: %w", err)
		return
	}

	// NewClient only builds configuration — it doesn't dial the engine
	// (see docker.NewClient's doc comment). A short-timeout Ping here is
	// what actually proves the daemon is reachable at startup. If it isn't
	// (Docker Desktop not running yet, first-run setup, etc.), the client is
	// closed and dropped rather than kept around in a half-verified state;
	// docker-dependent bound methods will report dockerErr until the user
	// retries (e.g. by starting a profile after starting Docker Desktop).
	pingCtx, cancel := context.WithTimeout(ctx, dockerStartupPingTimeout)
	defer cancel()
	if err := dockerClient.Ping(pingCtx); err != nil {
		a.dockerErr = fmt.Errorf("docker engine unreachable at startup: %w", err)
		_ = dockerClient.Close()
		return
	}

	a.docker = dockerClient
}

// shutdown is called when the app is closing, releasing the storage
// connection and Docker transport cleanly.
func (a *App) shutdown(_ context.Context) {
	if a.db != nil {
		_ = a.db.Close()
	}
	if a.docker != nil {
		_ = a.docker.Close()
	}
}

// requireDB returns the open storage handle, or a descriptive error if
// storage.Open() failed during startup.
func (a *App) requireDB() (*sql.DB, error) {
	if a.db == nil {
		if a.dbErr != nil {
			return nil, fmt.Errorf("local storage is not available: %w", a.dbErr)
		}
		return nil, fmt.Errorf("local storage is not available")
	}
	return a.db, nil
}

// requireDocker returns the connected Docker client, or a descriptive error
// if Docker wasn't reachable at startup — see the startup doc comment for
// why this doesn't crash the app.
func (a *App) requireDocker() (*docker.Client, error) {
	if a.docker == nil {
		if a.dockerErr != nil {
			return nil, fmt.Errorf("docker is not available: %w", a.dockerErr)
		}
		return nil, fmt.Errorf("docker is not available")
	}
	return a.docker, nil
}

// Ping is the smoke-test method for task 0.3: confirms the full
// frontend-to-Go IPC round trip and Wails' generated TS bindings work.
func (a *App) Ping() string {
	return "pong"
}

// ProfileSummary bundles a Profile with its Services for the frontend's
// profile list, so the UI doesn't need a second round trip per profile just
// to know what engine(s)/port(s) it has.
type ProfileSummary struct {
	Profile  storage.Profile
	Services []storage.Service
}

// ListProfiles returns every profile with its services attached, ordered by
// name (see storage.ListProfiles).
func (a *App) ListProfiles() ([]ProfileSummary, error) {
	db, err := a.requireDB()
	if err != nil {
		return nil, err
	}

	profiles, err := storage.ListProfiles(db)
	if err != nil {
		return nil, fmt.Errorf("list profiles: %w", err)
	}

	summaries := make([]ProfileSummary, 0, len(profiles))
	for _, p := range profiles {
		services, err := storage.ListServicesByProfile(db, p.ID)
		if err != nil {
			return nil, fmt.Errorf("list profiles: %w", err)
		}
		summaries = append(summaries, ProfileSummary{Profile: p, Services: services})
	}
	return summaries, nil
}

// CreateProfile creates a new profile with a single Postgres service using
// sensible defaults (Phase 1 MVP scope, spec.md §3.1/§3.2 — a form for
// engine/image/credentials and multi-engine profiles is task 2.4's wizard,
// not this task's).
//
// The host port defaults to 5432 but is bumped past any port already used
// by another Stackyard-managed service, per nextFreeHostPort's doc comment
// below. This is explicitly NOT task 1.5's real port-conflict detection —
// it only avoids the single most common self-inflicted collision (creating
// a second default profile back to back) and lets any remaining conflict
// (e.g. something else on the machine already bound to that port) surface
// as Docker's own bind error when the profile is started, per this task's
// scope.
func (a *App) CreateProfile(name string) (*ProfileSummary, error) {
	db, err := a.requireDB()
	if err != nil {
		return nil, err
	}

	profile, err := storage.CreateProfile(db, name)
	if err != nil {
		return nil, fmt.Errorf("create profile %q: %w", name, err)
	}

	hostPort, err := a.nextFreeHostPort(db, defaultPostgresHostPort)
	if err != nil {
		return nil, fmt.Errorf("create profile %q: %w", name, err)
	}

	username := defaultPostgresUsername
	password := defaultPostgresPassword
	dbName := defaultPostgresDBName

	svc := &storage.Service{
		ProfileID:         profile.ID,
		Engine:            storage.EnginePostgres,
		ImageTag:          defaultPostgresImageTag,
		HostPort:          hostPort,
		Username:          &username,
		PasswordEncrypted: &password,
		DBName:            &dbName,
		VolumeName:        fmt.Sprintf("stackyard-vol-profile-%d-postgres", profile.ID),
	}

	created, err := storage.CreateService(db, svc)
	if err != nil {
		return nil, fmt.Errorf("create profile %q: create default postgres service: %w", name, err)
	}

	return &ProfileSummary{Profile: *profile, Services: []storage.Service{*created}}, nil
}

// nextFreeHostPort returns the smallest port >= defaultPort not already used
// by any Stackyard-managed service recorded in local storage.
//
// Judgment call: this only checks ports Stackyard itself has already handed
// out — it does NOT probe the OS/network for arbitrary in-use ports (that
// real check, plus surfacing a suggested free port in the UI instead of a
// raw Docker error, is task 1.5). It's still worth doing here, cheaply,
// because without it every new default profile would collide on 5432 the
// moment a second one is created, which would be an obviously bad first
// impression to ship even for an admittedly temporary MVP default.
func (a *App) nextFreeHostPort(db *sql.DB, defaultPort int) (int, error) {
	profiles, err := storage.ListProfiles(db)
	if err != nil {
		return 0, err
	}

	used := make(map[int]bool)
	for _, p := range profiles {
		services, err := storage.ListServicesByProfile(db, p.ID)
		if err != nil {
			return 0, err
		}
		for _, s := range services {
			used[s.HostPort] = true
		}
	}

	port := defaultPort
	for used[port] {
		port++
	}
	return port, nil
}

// StartProfile starts every service in the profile, creating Docker
// resources (network/volume/container) on first run and reusing/starting
// them in place otherwise — see internal/docker/compose.go's idempotency
// doc comment for the exact create-vs-reuse-vs-start decision tree.
//
// Phase 1 MVP only understands Postgres services (plan.md §6); a profile
// containing a non-Postgres engine is not reachable from today's UI, but if
// one ever exists, this returns an error naming the unsupported engine
// rather than silently skipping it.
func (a *App) StartProfile(profileID int64) error {
	dockerClient, err := a.requireDocker()
	if err != nil {
		return err
	}
	db, err := a.requireDB()
	if err != nil {
		return err
	}

	services, err := storage.ListServicesByProfile(db, profileID)
	if err != nil {
		return fmt.Errorf("start profile %d: %w", profileID, err)
	}
	if len(services) == 0 {
		return fmt.Errorf("start profile %d: profile has no services", profileID)
	}

	ctx, cancel := context.WithTimeout(a.ctx, dockerOpTimeout)
	defer cancel()

	for _, svc := range services {
		if svc.Engine != storage.EnginePostgres {
			return fmt.Errorf("start profile %d: engine %q is not supported yet", profileID, svc.Engine)
		}
		if err := dockerClient.StartPostgresEnvironment(ctx, svc); err != nil {
			return fmt.Errorf("start profile %d: %w", profileID, err)
		}
	}
	return nil
}

// StopProfile stops every service's container in the profile.
//
// Unlike starting, stopping is engine-agnostic — internal/docker.
// StopContainer only needs the deterministic container name (see
// docker.ServiceContainerName), not engine-specific container specs — so
// this loop doesn't need a per-engine switch the way StartProfile does.
func (a *App) StopProfile(profileID int64) error {
	dockerClient, err := a.requireDocker()
	if err != nil {
		return err
	}
	db, err := a.requireDB()
	if err != nil {
		return err
	}

	services, err := storage.ListServicesByProfile(db, profileID)
	if err != nil {
		return fmt.Errorf("stop profile %d: %w", profileID, err)
	}

	ctx, cancel := context.WithTimeout(a.ctx, dockerStopTimeout)
	defer cancel()

	for _, svc := range services {
		name := docker.ServiceContainerName(svc.ID)
		if err := dockerClient.StopContainer(ctx, name); err != nil {
			return fmt.Errorf("stop profile %d: %w", profileID, err)
		}
	}
	return nil
}

// RestartProfile stops then starts every service in the profile
// (spec.md §3.2's "restart all services in a profile as a unit").
func (a *App) RestartProfile(profileID int64) error {
	if err := a.StopProfile(profileID); err != nil {
		return err
	}
	return a.StartProfile(profileID)
}

// GetProfileStatus reports an aggregate status across every service's
// container in the profile:
//
//   - "running" — every service's container is running.
//   - "stopped" — no service's container is running (including a profile
//     whose containers have never been created yet).
//   - "partial" — some are running and some aren't; only reachable today via
//     an out-of-band Docker action (e.g. `docker stop` on one container from
//     the CLI/Docker Desktop) since this MVP always starts/stops a whole
//     profile as a unit.
//   - "unknown" — Docker itself isn't reachable, so no real answer is
//     available; the returned error explains why.
func (a *App) GetProfileStatus(profileID int64) (string, error) {
	dockerClient, err := a.requireDocker()
	if err != nil {
		return "unknown", err
	}
	db, err := a.requireDB()
	if err != nil {
		return "unknown", err
	}

	services, err := storage.ListServicesByProfile(db, profileID)
	if err != nil {
		return "unknown", fmt.Errorf("get status for profile %d: %w", profileID, err)
	}
	if len(services) == 0 {
		return "stopped", nil
	}

	ctx, cancel := context.WithTimeout(a.ctx, dockerStatusTimeout)
	defer cancel()

	running := 0
	for _, svc := range services {
		name := docker.ServiceContainerName(svc.ID)
		state, err := dockerClient.ContainerState(ctx, name)
		if err != nil {
			return "unknown", fmt.Errorf("get status for profile %d: %w", profileID, err)
		}
		if state == "running" {
			running++
		}
	}

	switch {
	case running == 0:
		return "stopped", nil
	case running == len(services):
		return "running", nil
	default:
		return "partial", nil
	}
}
