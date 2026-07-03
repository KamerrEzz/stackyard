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
	defaultPostgresImageTag = "postgres:16-alpine"
	defaultPostgresHostPort = 5432
	defaultPostgresUsername = "postgres"
	defaultPostgresPassword = "postgres"
	defaultPostgresDBName   = "postgres"

	defaultMySQLImageTag = "mysql:8"
	defaultMySQLHostPort = 3306
	defaultMySQLUsername = "root"
	defaultMySQLPassword = "mysql"
	defaultMySQLDBName   = "mysql"

	defaultMongoImageTag = "mongo:7"
	defaultMongoHostPort = 27017
	defaultMongoUsername = "root"
	defaultMongoPassword = "mongo"

	defaultRedisImageTag = "redis:7-alpine"
	defaultRedisHostPort = 6379

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

// ServiceRequest is one engine instance the caller wants CreateProfile to
// provision within a new profile. HostPort is optional: 0 means "assign this
// engine's own OS-standard default port" (Postgres 5432, MySQL 3306, MongoDB
// 27017, Redis 6379), bumped past whatever's already recorded for another
// Stackyard-managed service — see assignHostPorts. Image tag and credentials
// are not caller-configurable here; each engine gets the same kind of
// sensible built-in default CreateProfile has always given Postgres (see
// defaultsForEngine), consistent with spec.md §3.2's "built-in engine
// template" 3-click flow.
type ServiceRequest struct {
	Engine   storage.Engine
	HostPort int
}

// engineDefaults bundles the per-engine built-in defaults CreateProfile
// applies to a ServiceRequest that doesn't specify them.
type engineDefaults struct {
	imageTag string
	hostPort int
	username *string
	password *string
	dbName   *string
}

// defaultsForEngine returns the built-in image tag, default host port, and
// credential defaults for engine, following the exact credential-mapping
// rules established for each engine's container spec (mysql.go, mongodb.go,
// redis.go):
//
//   - Postgres/MySQL: an explicit username/password/db name, matching
//     buildPostgresContainerSpec/buildMySQLContainerSpec's expectations.
//     MySQL's default username is exactly "root" so buildMySQLContainerSpec's
//     root-vs-regular-user branch takes the root path (only
//     MYSQL_ROOT_PASSWORD/MYSQL_DATABASE are set).
//   - MongoDB: username/password default like the other two, but dbName is
//     left nil — buildMongoContainerSpec omits MONGO_INITDB_DATABASE
//     entirely when nil rather than defaulting it, per this file's own
//     package doc comment on why Mongo has no upfront database-name concept.
//   - Redis: no username, no db name (Redis has neither concept — see
//     redis.go), and no default password either. A password-less default
//     keeps Redis's zero-friction "just start it" ethos that redis.go's own
//     package doc comment establishes as a deliberate, not accidental,
//     choice; a user who wants an authenticated Redis sets one after
//     creation.
func defaultsForEngine(engine storage.Engine) (engineDefaults, error) {
	switch engine {
	case storage.EnginePostgres:
		username, password, dbName := defaultPostgresUsername, defaultPostgresPassword, defaultPostgresDBName
		return engineDefaults{
			imageTag: defaultPostgresImageTag,
			hostPort: defaultPostgresHostPort,
			username: &username,
			password: &password,
			dbName:   &dbName,
		}, nil
	case storage.EngineMySQL:
		username, password, dbName := defaultMySQLUsername, defaultMySQLPassword, defaultMySQLDBName
		return engineDefaults{
			imageTag: defaultMySQLImageTag,
			hostPort: defaultMySQLHostPort,
			username: &username,
			password: &password,
			dbName:   &dbName,
		}, nil
	case storage.EngineMongoDB:
		username, password := defaultMongoUsername, defaultMongoPassword
		return engineDefaults{
			imageTag: defaultMongoImageTag,
			hostPort: defaultMongoHostPort,
			username: &username,
			password: &password,
		}, nil
	case storage.EngineRedis:
		return engineDefaults{
			imageTag: defaultRedisImageTag,
			hostPort: defaultRedisHostPort,
		}, nil
	default:
		return engineDefaults{}, fmt.Errorf("unsupported engine %q", engine)
	}
}

// assignHostPorts resolves the actual host port for every entry in requests,
// in order: an explicit ServiceRequest.HostPort is honored as-is; a zero
// HostPort defaults to that request's engine's own OS-standard port (see
// defaultsForEngine), bumped upward one at a time past any port already in
// used OR already assigned earlier in this same call. The latter is what
// keeps two engines from ever colliding on each other's default port within
// one CreateProfile call, even though today's four engine defaults (5432,
// 3306, 27017, 6379) never actually overlap with each other — it also
// protects a future engine addition or an explicit HostPort collision from
// silently reusing a port. used is read, never mutated.
func assignHostPorts(used map[int]bool, requests []ServiceRequest) ([]int, error) {
	taken := make(map[int]bool, len(used)+len(requests))
	for port := range used {
		taken[port] = true
	}

	ports := make([]int, len(requests))
	for i, req := range requests {
		port := req.HostPort
		if port == 0 {
			defaults, err := defaultsForEngine(req.Engine)
			if err != nil {
				return nil, err
			}
			port = defaults.hostPort
		}
		for taken[port] {
			port++
		}
		taken[port] = true
		ports[i] = port
	}
	return ports, nil
}

// duplicateEngineError reports an error if requests names the same engine
// more than once — a profile is a set of at most one service per engine, so
// e.g. two Postgres services in one CreateProfile call is rejected rather
// than silently creating both (which would also make assignHostPorts bump
// the second one to a surprising, unrequested port).
func duplicateEngineError(requests []ServiceRequest) error {
	seen := make(map[storage.Engine]bool, len(requests))
	for _, req := range requests {
		if seen[req.Engine] {
			return fmt.Errorf("duplicate engine %q requested — a profile may have at most one service per engine", req.Engine)
		}
		seen[req.Engine] = true
	}
	return nil
}

// CreateProfile creates a new profile with one service per entry in
// services, supporting any combination of 1-4 engines in a single call
// (spec.md §3.1/§3.2, tasks.md 2.4). Each service gets its engine's built-in
// image tag and credential defaults (see defaultsForEngine) and a host port
// resolved by assignHostPorts — either the caller's explicit
// ServiceRequest.HostPort or that engine's own default port, bumped past
// anything already recorded for another Stackyard-managed service. This is
// NOT real port-conflict detection (see CheckProfilePortConflict/
// SuggestFreePort for that) — it only avoids colliding with another
// Stackyard-managed profile/service.
func (a *App) CreateProfile(name string, services []ServiceRequest) (*ProfileSummary, error) {
	db, err := a.requireDB()
	if err != nil {
		return nil, err
	}
	if len(services) == 0 {
		return nil, fmt.Errorf("create profile %q: at least one service is required", name)
	}
	if err := duplicateEngineError(services); err != nil {
		return nil, fmt.Errorf("create profile %q: %w", name, err)
	}

	profile, err := storage.CreateProfile(db, name)
	if err != nil {
		return nil, fmt.Errorf("create profile %q: %w", name, err)
	}

	used, err := usedHostPorts(db)
	if err != nil {
		return nil, fmt.Errorf("create profile %q: %w", name, err)
	}

	ports, err := assignHostPorts(used, services)
	if err != nil {
		return nil, fmt.Errorf("create profile %q: %w", name, err)
	}

	created := make([]storage.Service, 0, len(services))
	for i, req := range services {
		defaults, err := defaultsForEngine(req.Engine)
		if err != nil {
			return nil, fmt.Errorf("create profile %q: %w", name, err)
		}

		svc := &storage.Service{
			ProfileID:         profile.ID,
			Engine:            req.Engine,
			ImageTag:          defaults.imageTag,
			HostPort:          ports[i],
			Username:          defaults.username,
			PasswordEncrypted: defaults.password,
			DBName:            defaults.dbName,
			VolumeName:        fmt.Sprintf("stackyard-vol-profile-%d-%s", profile.ID, req.Engine),
		}

		savedSvc, err := storage.CreateService(db, svc)
		if err != nil {
			return nil, fmt.Errorf("create profile %q: create %s service: %w", name, req.Engine, err)
		}
		created = append(created, *savedSvc)
	}

	return &ProfileSummary{Profile: *profile, Services: created}, nil
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

// GetConnectionString returns the canonical connection URL for a service, in
// its engine's own format (spec.md §3.3), by dispatching to
// connectionStringForService.
func (a *App) GetConnectionString(serviceID int64) (string, error) {
	db, err := a.requireDB()
	if err != nil {
		return "", err
	}

	svc, err := storage.GetService(db, serviceID)
	if err != nil {
		return "", fmt.Errorf("get connection string for service %d: %w", serviceID, err)
	}

	return connectionStringForService(*svc)
}

// connectionStringForService dispatches to the right
// internal/docker.<Engine>ConnectionString builder for svc.Engine. Kept as
// its own function (rather than inlined into GetConnectionString) so the
// dispatch itself is unit-testable without a database.
func connectionStringForService(svc storage.Service) (string, error) {
	switch svc.Engine {
	case storage.EnginePostgres:
		return docker.PostgresConnectionString(svc), nil
	case storage.EngineMySQL:
		return docker.MySQLConnectionString(svc), nil
	case storage.EngineMongoDB:
		return docker.MongoConnectionString(svc), nil
	case storage.EngineRedis:
		return docker.RedisConnectionString(svc), nil
	default:
		return "", fmt.Errorf("get connection string for service %d: unsupported engine %q", svc.ID, svc.Engine)
	}
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

// engineStarters maps each supported storage.Engine to the
// internal/docker.Client method that starts it (StartPostgresEnvironment,
// StartMySQLEnvironment, StartMongoEnvironment, StartRedisEnvironment) —
// this is what lets StartProfile start any combination of the 4 engines in
// one profile as a unit (tasks.md 2.4), dispatching each service to its own
// engine's Start<Engine>Environment rather than assuming Postgres.
//
// Built from method EXPRESSIONS (`(*docker.Client).StartXEnvironment`), not
// bound method values — a method expression's function pointer is the same
// every time it's taken, regardless of receiver, which is what makes
// engineStarters' contents reflect-comparable in a unit test without ever
// constructing a live *docker.Client (see app_test.go).
var engineStarters = map[storage.Engine]func(*docker.Client, context.Context, storage.Service) error{
	storage.EnginePostgres: (*docker.Client).StartPostgresEnvironment,
	storage.EngineMySQL:    (*docker.Client).StartMySQLEnvironment,
	storage.EngineMongoDB:  (*docker.Client).StartMongoEnvironment,
	storage.EngineRedis:    (*docker.Client).StartRedisEnvironment,
}

// startServiceEnvironment starts svc's Docker environment by dispatching to
// its engine's entry in engineStarters. Returns an error naming the engine if
// svc.Engine isn't one of the 4 supported engines.
func startServiceEnvironment(ctx context.Context, dockerClient *docker.Client, svc storage.Service) error {
	starter, ok := engineStarters[svc.Engine]
	if !ok {
		return fmt.Errorf("start service %d: unsupported engine %q", svc.ID, svc.Engine)
	}
	return starter(dockerClient, ctx, svc)
}

// StartProfile starts every service in the profile as a single unit,
// creating Docker resources (network/volume/container) on first run and
// reusing/starting them in place otherwise (see internal/docker/compose.go
// and its per-engine counterparts mysql.go/mongodb.go/redis.go), dispatching
// each service to its own engine via startServiceEnvironment/engineStarters
// — a profile mixing e.g. Postgres and Redis starts both containers from one
// call. Before starting each service, this re-runs the same port-conflict
// check CheckProfilePortConflict exposes to the frontend, as defense in
// depth against a stale or skipped frontend pre-check.
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

		if err := startServiceEnvironment(ctx, dockerClient, svc); err != nil {
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
