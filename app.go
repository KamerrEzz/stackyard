package main

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"stackyard/internal/docker"
	"stackyard/internal/netcheck"
	"stackyard/internal/storage"
)

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

// App struct is the ONLY surface bound to the frontend — every other package
// stays behind this thin adapter layer.
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

	pingCtx, cancel := context.WithTimeout(ctx, dockerStartupPingTimeout)
	defer cancel()
	if err := dockerClient.Ping(pingCtx); err != nil {
		a.dockerErr = fmt.Errorf("docker engine unreachable at startup: %w", err)
		_ = dockerClient.Close()
		return
	}

	a.docker = dockerClient
}

func (a *App) shutdown(_ context.Context) {
	if a.db != nil {
		_ = a.db.Close()
	}
	if a.docker != nil {
		_ = a.docker.Close()
	}
}

func (a *App) requireDB() (*sql.DB, error) {
	if a.db == nil {
		if a.dbErr != nil {
			return nil, fmt.Errorf("local storage is not available: %w", a.dbErr)
		}
		return nil, fmt.Errorf("local storage is not available")
	}
	return a.db, nil
}

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
// sensible defaults (Phase 1 MVP scope is Postgres-only). The host port
// defaults to 5432 but is bumped past any port already used by another
// Stackyard-managed service, per nextFreeHostPort. This is NOT real
// port-conflict detection (see CheckProfilePortConflict/SuggestFreePort for
// that) — it only avoids colliding with another Stackyard-managed profile.
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

// DuplicateProfile copies an existing profile and all of its services under
// a new, auto-generated name (see storage.DuplicateProfile), returning the
// new profile's summary. The copy is a fully independent row with its own
// ID — not an alias of the original — but its services keep the same host
// ports as their source, so starting the duplicate before changing its
// ports is expected to surface the same port-conflict pre-check
// (CheckProfilePortConflict) a manually recreated profile would.
func (a *App) DuplicateProfile(profileID int64) (*ProfileSummary, error) {
	db, err := a.requireDB()
	if err != nil {
		return nil, err
	}

	profile, err := storage.DuplicateProfile(db, profileID)
	if err != nil {
		return nil, fmt.Errorf("duplicate profile %d: %w", profileID, err)
	}

	services, err := storage.ListServicesByProfile(db, profile.ID)
	if err != nil {
		return nil, fmt.Errorf("duplicate profile %d: %w", profileID, err)
	}

	return &ProfileSummary{Profile: *profile, Services: services}, nil
}

// RenameProfile renames an existing profile in place and returns its
// refreshed summary.
func (a *App) RenameProfile(profileID int64, newName string) (*ProfileSummary, error) {
	db, err := a.requireDB()
	if err != nil {
		return nil, err
	}

	profile, err := storage.UpdateProfile(db, profileID, newName)
	if err != nil {
		return nil, fmt.Errorf("rename profile %d: %w", profileID, err)
	}

	services, err := storage.ListServicesByProfile(db, profile.ID)
	if err != nil {
		return nil, fmt.Errorf("rename profile %d: %w", profileID, err)
	}

	return &ProfileSummary{Profile: *profile, Services: services}, nil
}

// deleteProfileGuardError decides whether DeleteProfile may proceed, given a
// profile's current aggregate status (as GetProfileStatus reports it) and
// any error encountered while determining it. Deletion is refused unless
// the profile is confirmed "stopped": allowing deletion while a container
// is still running would leave that container orphaned with no remaining
// UI reference to stop or reconnect to it, which is worse than surfacing a
// clear "stop it first" error — and if status can't be confirmed at all
// (e.g. Docker is unreachable), that same uncertainty means deletion isn't
// safe to allow either. Kept as its own pure function (no Docker/DB access)
// so this decision is unit-testable without a live Docker engine.
func deleteProfileGuardError(profileID int64, status string, statusErr error) error {
	if statusErr != nil {
		return fmt.Errorf("delete profile %d: could not confirm the profile is stopped: %w", profileID, statusErr)
	}
	if status != "stopped" {
		return fmt.Errorf("delete profile %d: profile must be stopped before it can be deleted (current status: %s)", profileID, status)
	}
	return nil
}

// DeleteProfile removes a profile and its services from local storage only.
// It never touches Docker resources — deleting a profile does not delete
// its Docker volumes (spec.md §3.1); that is a decision the user makes
// explicitly and separately (task 2.6, "reset volume"). The one Docker
// interaction this method performs is a read-only status check
// (GetProfileStatus) used purely to decide whether deletion may proceed at
// all — see deleteProfileGuardError — it never starts, stops, or removes
// any container or volume itself.
func (a *App) DeleteProfile(profileID int64) error {
	db, err := a.requireDB()
	if err != nil {
		return err
	}

	status, statusErr := a.GetProfileStatus(profileID)
	if guardErr := deleteProfileGuardError(profileID, status, statusErr); guardErr != nil {
		return guardErr
	}

	if err := storage.DeleteProfile(db, profileID); err != nil {
		return fmt.Errorf("delete profile %d: %w", profileID, err)
	}
	return nil
}

// GetConnectionString returns the canonical connection URL for a service.
// Phase 1 MVP is Postgres-only; calling this for a non-Postgres service
// returns an error naming the unsupported engine.
func (a *App) GetConnectionString(serviceID int64) (string, error) {
	db, err := a.requireDB()
	if err != nil {
		return "", err
	}

	svc, err := storage.GetService(db, serviceID)
	if err != nil {
		return "", fmt.Errorf("get connection string for service %d: %w", serviceID, err)
	}

	if svc.Engine != storage.EnginePostgres {
		return "", fmt.Errorf("get connection string for service %d: engine %q is not supported yet", serviceID, svc.Engine)
	}

	return docker.PostgresConnectionString(*svc), nil
}

func (a *App) nextFreeHostPort(db *sql.DB, defaultPort int) (int, error) {
	used, err := usedHostPorts(db)
	if err != nil {
		return 0, err
	}

	port := defaultPort
	for used[port] {
		port++
	}
	return port, nil
}

func usedHostPorts(db *sql.DB) (map[int]bool, error) {
	profiles, err := storage.ListProfiles(db)
	if err != nil {
		return nil, err
	}

	used := make(map[int]bool)
	for _, p := range profiles {
		services, err := storage.ListServicesByProfile(db, p.ID)
		if err != nil {
			return nil, err
		}
		for _, s := range services {
			used[s.HostPort] = true
		}
	}
	return used, nil
}

const maxPortScanAttempts = 1000

// CheckPortAvailable reports whether port is free to bind at the OS level
// right now, with no per-service "own container already running" exemption
// (see CheckProfilePortConflict for that).
func (a *App) CheckPortAvailable(port int) (bool, error) {
	return netcheck.IsPortFree(port), nil
}

// SuggestFreePort scans upward from startingFrom and returns the first port
// that is both free at the OS level and not already recorded as another
// Stackyard service's host port.
func (a *App) SuggestFreePort(startingFrom int) (int, error) {
	db, err := a.requireDB()
	if err != nil {
		return 0, err
	}

	used, err := usedHostPorts(db)
	if err != nil {
		return 0, fmt.Errorf("suggest free port: %w", err)
	}

	port := startingFrom
	for attempts := 0; attempts < maxPortScanAttempts; attempts++ {
		if !used[port] && netcheck.IsPortFree(port) {
			return port, nil
		}
		port++
	}
	return 0, fmt.Errorf("suggest free port: no free port found scanning from %d (%d attempts)", startingFrom, maxPortScanAttempts)
}

// PortConflictInfo is what the frontend receives from
// CheckProfilePortConflict.
type PortConflictInfo struct {
	HasConflict bool
	Port        int
	// SuggestedPort is 0 when HasConflict is false, or when a suggestion
	// couldn't be computed; the frontend treats 0 as "no suggestion".
	SuggestedPort int
}

// CheckProfilePortConflict is the frontend's pre-start check: it reports a
// port conflict with a suggested alternative before StartProfile is called.
// StartProfile re-runs the same check itself as defense in depth.
func (a *App) CheckProfilePortConflict(profileID int64) (*PortConflictInfo, error) {
	dockerClient, err := a.requireDocker()
	if err != nil {
		return nil, err
	}
	db, err := a.requireDB()
	if err != nil {
		return nil, err
	}

	services, err := storage.ListServicesByProfile(db, profileID)
	if err != nil {
		return nil, fmt.Errorf("check port conflict for profile %d: %w", profileID, err)
	}

	ctx, cancel := context.WithTimeout(a.ctx, dockerStatusTimeout)
	defer cancel()

	for _, svc := range services {
		info, err := a.checkServicePortConflict(ctx, dockerClient, svc)
		if err != nil {
			return nil, fmt.Errorf("check port conflict for profile %d: %w", profileID, err)
		}
		if info.HasConflict {
			return info, nil
		}
	}

	return &PortConflictInfo{HasConflict: false}, nil
}

func (a *App) checkServicePortConflict(ctx context.Context, dockerClient *docker.Client, svc storage.Service) (*PortConflictInfo, error) {
	containerName := docker.ServiceContainerName(svc.ID)

	result, err := dockerClient.CheckServicePortConflict(ctx, svc.HostPort, containerName)
	if err != nil {
		return nil, err
	}
	if !result.Conflict {
		return &PortConflictInfo{HasConflict: false}, nil
	}

	info := &PortConflictInfo{HasConflict: true, Port: svc.HostPort}
	if suggested, sErr := a.SuggestFreePort(svc.HostPort + 1); sErr == nil {
		info.SuggestedPort = suggested
	}
	return info, nil
}

// StartProfile starts every service in the profile, creating Docker
// resources (network/volume/container) on first run and reusing/starting
// them in place otherwise (see internal/docker/compose.go). Phase 1 MVP only
// understands Postgres services; a non-Postgres service returns an error
// naming the unsupported engine. Before starting each service, this re-runs
// the same port-conflict check CheckProfilePortConflict exposes to the
// frontend, as defense in depth against a stale or skipped frontend
// pre-check.
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

		conflict, err := a.checkServicePortConflict(ctx, dockerClient, svc)
		if err != nil {
			return fmt.Errorf("start profile %d: %w", profileID, err)
		}
		if conflict.HasConflict {
			if conflict.SuggestedPort > 0 {
				return fmt.Errorf("start profile %d: port %d is already in use by another process — try port %d instead", profileID, conflict.Port, conflict.SuggestedPort)
			}
			return fmt.Errorf("start profile %d: port %d is already in use by another process", profileID, conflict.Port)
		}

		if err := dockerClient.StartPostgresEnvironment(ctx, svc); err != nil {
			return fmt.Errorf("start profile %d: %w", profileID, err)
		}
	}
	return nil
}

// StopProfile stops every service's container in the profile. Unlike
// starting, stopping is engine-agnostic, so this loop needs no per-engine
// switch.
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

// RestartProfile stops then starts every service in the profile.
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
//   - "partial" — some are running and some aren't.
//   - "unknown" — Docker itself isn't reachable; the returned error explains
//     why.
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
