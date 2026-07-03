package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"stackyard/internal/dbengine"
	dbenginemysql "stackyard/internal/dbengine/mysql"
	dbenginepostgres "stackyard/internal/dbengine/postgres"
	"stackyard/internal/docker"
	"stackyard/internal/netcheck"
	"stackyard/internal/storage"

	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
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

	// EnvironmentStatusEventName is the Wails event emitted every poll cycle
	// by the background status watcher (see StartStatusWatcher), carrying a
	// full docker.EnvironmentStatusSnapshot as its single payload (spec.md
	// §3.5, tasks.md 2.8).
	EnvironmentStatusEventName = "environment:status"

	// statusWatchInterval is comfortably under spec.md §3.5's ≤2s refresh
	// target.
	statusWatchInterval = 1500 * time.Millisecond

	// testConnectionTimeout bounds TestConnection's Connect+Ping round trip
	// so an unreachable host fails fast instead of hanging the UI (tasks.md
	// 3.4, spec.md §4.1's "one-click Test connection" requirement).
	testConnectionTimeout = 5 * time.Second

	// openConnectionTimeout bounds OpenConnection's Connect+Ping round trip,
	// the same budget testConnectionTimeout gives TestConnection, so opening
	// a query editor session against an unreachable host fails fast instead
	// of hanging the UI (tasks.md 3.6).
	openConnectionTimeout = 5 * time.Second
)

// App struct is the ONLY surface bound to the frontend — every other package
// stays behind this thin adapter layer.
type App struct {
	ctx context.Context

	db    *sql.DB
	dbErr error

	docker    *docker.Client
	dockerErr error

	// statusWatcher* fields guard the background poller StartStatusWatcher
	// starts and StopStatusWatcher stops (tasks.md 2.8) — see both methods'
	// doc comments for the synchronization contract.
	statusWatcherMu      sync.Mutex
	statusWatcherCancel  context.CancelFunc
	statusWatcherWG      sync.WaitGroup
	statusWatcherRunning bool

	// querySessions/queryCancels back OpenConnection/RunQuery/CancelQuery/
	// CloseConnectionSession (tasks.md 3.6): querySessions holds one live,
	// connected dbengine.Engine per generated session ID, keyed so multiple
	// tabs (task 3.8) can each hold their own independent connection rather
	// than the app assuming exactly one is ever open. queryCancels holds the
	// context.CancelFunc for whichever RunQuery call is currently in flight
	// for a given session ID, so a separate, concurrently-arriving
	// CancelQuery call can reach in and cancel it (spec.md §4.6) — Wails has
	// no built-in primitive for cancelling an in-flight bound-method call,
	// so this map is the cancellation hook. The two maps are guarded by
	// their own mutex rather than sharing one: a query's cancel func churns
	// on every RunQuery call while a session's engine persists across many
	// of them, and sharing one lock would serialize session lookups behind
	// query start/finish for no benefit.
	querySessionsMu sync.Mutex
	querySessions   map[string]*querySession

	queryCancelsMu sync.Mutex
	queryCancels   map[string]context.CancelFunc
}

// querySession holds one live, connected dbengine.Engine bound to a
// generated session ID (see OpenConnection), letting RunQuery/CancelQuery/
// CloseConnectionSession reference it across separate IPC calls without the
// frontend ever seeing the Engine value itself.
type querySession struct {
	engine dbengine.Engine
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
	a.StopStatusWatcher()
	a.closeAllQuerySessions()

	if a.db != nil {
		_ = a.db.Close()
	}
	if a.docker != nil {
		_ = a.docker.Close()
	}
}

// closeAllQuerySessions cancels every in-flight RunQuery call and closes
// every live Engine session (see OpenConnection), called from shutdown() so
// a still-open DB Client tab never leaks a database connection past app
// exit.
func (a *App) closeAllQuerySessions() {
	a.queryCancelsMu.Lock()
	for _, cancel := range a.queryCancels {
		cancel()
	}
	a.queryCancels = nil
	a.queryCancelsMu.Unlock()

	a.querySessionsMu.Lock()
	defer a.querySessionsMu.Unlock()
	for _, session := range a.querySessions {
		_ = session.engine.Close()
	}
	a.querySessions = nil
}

func (a *App) putQuerySession(id string, session *querySession) {
	a.querySessionsMu.Lock()
	defer a.querySessionsMu.Unlock()
	if a.querySessions == nil {
		a.querySessions = make(map[string]*querySession)
	}
	a.querySessions[id] = session
}

func (a *App) getQuerySession(id string) (*querySession, bool) {
	a.querySessionsMu.Lock()
	defer a.querySessionsMu.Unlock()
	session, ok := a.querySessions[id]
	return session, ok
}

func (a *App) deleteQuerySession(id string) (*querySession, bool) {
	a.querySessionsMu.Lock()
	defer a.querySessionsMu.Unlock()
	session, ok := a.querySessions[id]
	if ok {
		delete(a.querySessions, id)
	}
	return session, ok
}

func (a *App) putQueryCancel(id string, cancel context.CancelFunc) {
	a.queryCancelsMu.Lock()
	defer a.queryCancelsMu.Unlock()
	if a.queryCancels == nil {
		a.queryCancels = make(map[string]context.CancelFunc)
	}
	a.queryCancels[id] = cancel
}

func (a *App) popQueryCancel(id string) (context.CancelFunc, bool) {
	a.queryCancelsMu.Lock()
	defer a.queryCancelsMu.Unlock()
	cancel, ok := a.queryCancels[id]
	if ok {
		delete(a.queryCancels, id)
	}
	return cancel, ok
}

// OpenConnection dials fields' engine and keeps the resulting connection
// alive server-side, returning a session ID the frontend passes to
// RunQuery/CancelQuery/CloseConnectionSession for as many queries as it
// wants, across as many separate IPC calls as it wants (tasks.md 3.6).
// Unlike TestConnection, which connects-tests-closes in one shot, the
// dbengine.Engine constructed here is stored in a's session map and stays
// open until CloseConnectionSession closes it (or app shutdown does, via
// closeAllQuerySessions). Every call opens its own new, independent
// session, even for identical fields — callers that want several
// concurrently queryable tabs against the same connection (tasks.md 3.8)
// are expected to open one session per tab rather than sharing a single
// session across them, since RunQuery only tracks one in-flight query's
// cancel func per session ID (see RunQuery's doc comment).
func (a *App) OpenConnection(fields ConnectionFormFields) (string, error) {
	if err := validateConnectionFormFields(fields); err != nil {
		return "", fmt.Errorf("open connection: %w", err)
	}

	engine, err := newTestEngine(fields)
	if err != nil {
		return "", fmt.Errorf("open connection: %w", err)
	}

	ctx, cancel := context.WithTimeout(a.ctx, openConnectionTimeout)
	defer cancel()

	if err := engine.Connect(ctx); err != nil {
		return "", fmt.Errorf("open connection: %w", err)
	}
	if err := engine.Ping(ctx); err != nil {
		_ = engine.Close()
		return "", fmt.Errorf("open connection: %w", err)
	}

	id := uuid.NewString()
	a.putQuerySession(id, &querySession{engine: engine})
	return id, nil
}

// RunQuery executes query against the live Engine behind sessionID (see
// OpenConnection) and returns its raw dbengine.QueryResult. The query runs
// under a context this same App can cancel from a separate, concurrently
// arriving CancelQuery(sessionID) call (spec.md §4.6's "cancellable
// mid-run" requirement): Wails has no built-in primitive for cancelling an
// in-flight bound-method call, so RunQuery itself accepts no cancellation
// token — instead it derives a cancellable context, records that
// context's CancelFunc in a.queryCancels for exactly the duration of this
// call, and CancelQuery looks that same func up by sessionID to invoke it.
// Only one query may be in flight per session at a time: starting a
// second RunQuery on the same sessionID before the first finishes
// overwrites its cancel func, so a CancelQuery call after that point
// cancels the second (newest) query, not the first — callers needing
// independent, simultaneously cancellable queries should open a separate
// session per query rather than share one (see OpenConnection).
func (a *App) RunQuery(sessionID string, query string) (*dbengine.QueryResult, error) {
	session, ok := a.getQuerySession(sessionID)
	if !ok {
		return nil, fmt.Errorf("run query: no open connection session %q", sessionID)
	}

	ctx, cancel := context.WithCancel(a.ctx)
	a.putQueryCancel(sessionID, cancel)
	defer func() {
		a.popQueryCancel(sessionID)
		cancel()
	}()

	result, err := session.engine.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("run query: %w", err)
	}
	return result, nil
}

// CancelQuery cancels the RunQuery call currently in flight for sessionID,
// if any, by invoking the context.CancelFunc RunQuery registered for its
// duration (spec.md §4.6). It is not an error to call this when no query is
// running for sessionID: the cancel window may already have closed by the
// time this call arrives (the query finished, or was already cancelled),
// and that race is expected, ordinary behavior rather than a bug to
// surface as an error.
func (a *App) CancelQuery(sessionID string) error {
	cancel, ok := a.popQueryCancel(sessionID)
	if !ok {
		return nil
	}
	cancel()
	return nil
}

// CloseConnectionSession closes the live Engine behind sessionID and
// removes it from a's session map (tasks.md 3.6). Any query still in
// flight for this session is cancelled first — closing the underlying
// Engine out from under a running Query would otherwise race the driver's
// own connection teardown. Closing an unknown or already-closed sessionID
// is an error, not a silent no-op, since task 3.8's multi-tab shell needs
// to be able to tell when its own bookkeeping has drifted from the
// backend's.
func (a *App) CloseConnectionSession(sessionID string) error {
	if cancel, ok := a.popQueryCancel(sessionID); ok {
		cancel()
	}

	session, ok := a.deleteQuerySession(sessionID)
	if !ok {
		return fmt.Errorf("close connection session: no open connection session %q", sessionID)
	}

	if err := session.engine.Close(); err != nil {
		return fmt.Errorf("close connection session: %w", err)
	}
	return nil
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

// ResetServiceVolume permanently erases a single service's data (spec.md
// §3.4, tasks.md 2.6): it stops the service's container, removes both the
// container and its volume, then recreates both fresh via the same
// startServiceEnvironment/engineStarters dispatch StartProfile already uses.
// Only this one service's own container and volume (looked up by name/
// VolumeName) are touched — no profile-wide enumeration, so sibling services
// in the same profile are never stopped, removed, or otherwise affected.
//
// The container itself is removed, not merely stopped: Docker refuses to
// remove a volume that's still referenced by an existing container, even one
// that's stopped, so removing only the volume while leaving the old
// container in place would fail. Removing the container is safe here because
// startServiceEnvironment recreates it from scratch on the same path an
// entirely new "Start" already takes.
func (a *App) ResetServiceVolume(serviceID int64) error {
	dockerClient, err := a.requireDocker()
	if err != nil {
		return err
	}
	db, err := a.requireDB()
	if err != nil {
		return err
	}

	svc, err := storage.GetService(db, serviceID)
	if err != nil {
		return fmt.Errorf("reset volume for service %d: %w", serviceID, err)
	}

	ctx, cancel := context.WithTimeout(a.ctx, dockerOpTimeout)
	defer cancel()

	containerName := docker.ServiceContainerName(svc.ID)

	if err := dockerClient.StopContainer(ctx, containerName); err != nil {
		return fmt.Errorf("reset volume for service %d: %w", serviceID, err)
	}
	if err := dockerClient.RemoveContainer(ctx, containerName); err != nil {
		return fmt.Errorf("reset volume for service %d: %w", serviceID, err)
	}
	if err := dockerClient.RemoveVolume(ctx, svc.VolumeName); err != nil {
		return fmt.Errorf("reset volume for service %d: %w", serviceID, err)
	}

	if err := startServiceEnvironment(ctx, dockerClient, *svc); err != nil {
		return fmt.Errorf("reset volume for service %d: %w", serviceID, err)
	}
	return nil
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

// StartStatusWatcher starts the background poller that emits
// EnvironmentStatusEventName every statusWatchInterval with a full
// docker.EnvironmentStatusSnapshot across every profile/service (spec.md
// §3.5, tasks.md 2.8). The frontend calls this once when the status
// dashboard view mounts, matching this project's push-over-polling
// event-bus design (plan.md §2) — the frontend only ever subscribes via
// EventsOn, it never calls a bound method on an interval itself.
//
// Idempotent: a second call while the watcher is already running is a no-op
// and returns nil rather than starting a competing poller.
func (a *App) StartStatusWatcher() error {
	if _, err := a.requireDocker(); err != nil {
		return err
	}
	if _, err := a.requireDB(); err != nil {
		return err
	}

	a.statusWatcherMu.Lock()
	defer a.statusWatcherMu.Unlock()
	if a.statusWatcherRunning {
		return nil
	}

	watchCtx, cancel := context.WithCancel(a.ctx)
	a.statusWatcherCancel = cancel
	a.statusWatcherRunning = true

	a.statusWatcherWG.Add(1)
	go func() {
		defer a.statusWatcherWG.Done()
		a.runStatusWatcher(watchCtx)
	}()

	return nil
}

// StopStatusWatcher cancels the background status watcher and blocks until
// its goroutine has actually exited before returning. It is a no-op — not an
// error — both when the watcher was never started and when it was already
// stopped, so calling it from shutdown() is always safe regardless of
// whether StartStatusWatcher ever ran (including a shutdown that races an
// in-flight StartStatusWatcher call, since both hold statusWatcherMu).
func (a *App) StopStatusWatcher() {
	a.statusWatcherMu.Lock()
	defer a.statusWatcherMu.Unlock()
	if !a.statusWatcherRunning {
		return
	}

	a.statusWatcherCancel()
	a.statusWatcherWG.Wait()
	a.statusWatcherRunning = false
}

// runStatusWatcher emits one snapshot immediately (so the dashboard has data
// the moment watching starts, rather than after the first tick's delay),
// then one every statusWatchInterval until ctx is cancelled by
// StopStatusWatcher.
func (a *App) runStatusWatcher(ctx context.Context) {
	a.emitStatusSnapshot(ctx)

	ticker := time.NewTicker(statusWatchInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			a.emitStatusSnapshot(ctx)
		}
	}
}

// emitStatusSnapshot builds one snapshot and emits it as
// EnvironmentStatusEventName. A failure building the snapshot (Docker/DB
// unreachable this cycle) is silently skipped rather than stopping the
// watcher — the next tick tries again.
func (a *App) emitStatusSnapshot(ctx context.Context) {
	snapshot, err := a.buildStatusSnapshot(ctx)
	if err != nil {
		return
	}
	wailsruntime.EventsEmit(ctx, EnvironmentStatusEventName, snapshot)
}

// buildStatusSnapshot re-reads every profile's services and every service's
// REAL current container state/stats directly from Docker on every call —
// never cached App state — so it reflects containers started/stopped
// outside the app within one poll cycle (spec.md §3.5's second acceptance
// criterion). This is also the method status_watch_integration_test.go calls
// directly to verify that behavior against a live Docker Engine, bypassing
// EventsEmit entirely (see that file's doc comment for why).
func (a *App) buildStatusSnapshot(ctx context.Context) (docker.EnvironmentStatusSnapshot, error) {
	dockerClient, err := a.requireDocker()
	if err != nil {
		return docker.EnvironmentStatusSnapshot{}, err
	}
	db, err := a.requireDB()
	if err != nil {
		return docker.EnvironmentStatusSnapshot{}, err
	}

	profiles, err := storage.ListProfiles(db)
	if err != nil {
		return docker.EnvironmentStatusSnapshot{}, fmt.Errorf("build status snapshot: %w", err)
	}

	profileServices := make([]docker.ProfileServices, 0, len(profiles))
	var containerNames []string
	for _, p := range profiles {
		services, err := storage.ListServicesByProfile(db, p.ID)
		if err != nil {
			return docker.EnvironmentStatusSnapshot{}, fmt.Errorf("build status snapshot: %w", err)
		}
		profileServices = append(profileServices, docker.ProfileServices{Profile: p, Services: services})
		for _, svc := range services {
			containerNames = append(containerNames, docker.ServiceContainerName(svc.ID))
		}
	}

	snapshotCtx, cancel := context.WithTimeout(ctx, dockerStatusTimeout)
	defer cancel()

	states := dockerClient.ContainerStatesForNames(snapshotCtx, containerNames)

	runningNames := make([]string, 0, len(containerNames))
	for _, name := range containerNames {
		if states[name] == "running" {
			runningNames = append(runningNames, name)
		}
	}
	stats := dockerClient.StatsForContainers(snapshotCtx, runningNames)

	return docker.BuildEnvironmentStatusSnapshot(profileServices, states, stats), nil
}

// ConnectionFormFields is the Wails-bridge-safe shape of a connection form's
// data (tasks.md 3.4, spec.md §4.1): field-for-field the same as
// dbengine.ConnectionFields, except Params is a flat map[string]string
// instead of url.Values (map[string][]string) — see ParseConnectionURL's
// doc comment for why this bound-method boundary uses a different shape
// than urlparse.go's own type without changing that type itself.
type ConnectionFormFields struct {
	Engine   storage.Engine
	Host     string
	Port     int
	Username string
	Password string
	Database string
	Params   map[string]string
}

// toConnectionFormFields flattens a dbengine.ConnectionFields' url.Values
// Params into a plain map[string]string, keeping only the first value for
// any key that repeats in the query string. Every query param this app's
// connection strings actually use (sslmode, authSource, parseTime, ...) is
// single-valued in practice, and a flat map is simpler both to bind through
// Wails' JSON-based TS generator and for the frontend's key-value list
// editor than an array-valued map would be.
func toConnectionFormFields(fields *dbengine.ConnectionFields) ConnectionFormFields {
	params := make(map[string]string, len(fields.Params))
	for key, values := range fields.Params {
		if len(values) > 0 {
			params[key] = values[0]
		}
	}
	return ConnectionFormFields{
		Engine:   fields.Engine,
		Host:     fields.Host,
		Port:     fields.Port,
		Username: fields.Username,
		Password: fields.Password,
		Database: fields.Database,
		Params:   params,
	}
}

// ParseConnectionURL parses a pasted connection string (tasks.md 3.4,
// spec.md §4.1) via dbengine.ParseConnectionString and returns it as
// ConnectionFormFields rather than *dbengine.ConnectionFields directly.
// Wails' TS-binding generator serializes exported struct fields through
// encoding/json, and dbengine.ConnectionFields.Params is url.Values
// (map[string][]string) — technically JSON-serializable, but every value
// would need array indexing on the TS side for what is, in every
// connection string this app parses, a single-valued key. Flattening to
// map[string]string at this bound-method boundary keeps the frontend's
// key-value editor simple without touching urlparse.go's own type.
func (a *App) ParseConnectionURL(raw string) (*ConnectionFormFields, error) {
	fields, err := dbengine.ParseConnectionString(raw)
	if err != nil {
		return nil, err
	}
	result := toConnectionFormFields(fields)
	return &result, nil
}

// validateConnectionFormFields catches the cases that would otherwise
// surface as a cryptic dial error deep inside a driver, before any engine is
// even constructed.
func validateConnectionFormFields(fields ConnectionFormFields) error {
	if strings.TrimSpace(fields.Host) == "" {
		return fmt.Errorf("host is required")
	}
	if fields.Port < 1 || fields.Port > 65535 {
		return fmt.Errorf("port must be between 1 and 65535, got %d", fields.Port)
	}
	return nil
}

// buildPostgresTestConnString builds a "postgres://user:pass@host:port/db"
// URL directly from fields via net/url, the same safe percent-encoding
// approach internal/docker/connstring.go's PostgresConnectionString already
// uses for Stackyard-managed services. Built fresh from the current form
// state, not the originally pasted string, so edits made after autofill
// (spec.md §4.1's "editable afterward" requirement) are always what gets
// tested.
func buildPostgresTestConnString(fields ConnectionFormFields) string {
	var userInfo *url.Userinfo
	switch {
	case fields.Password != "":
		userInfo = url.UserPassword(fields.Username, fields.Password)
	case fields.Username != "":
		userInfo = url.User(fields.Username)
	}

	u := &url.URL{
		Scheme: "postgres",
		User:   userInfo,
		Host:   fmt.Sprintf("%s:%d", fields.Host, fields.Port),
		Path:   "/" + fields.Database,
	}

	if len(fields.Params) > 0 {
		query := url.Values{}
		for key, value := range fields.Params {
			query.Set(key, value)
		}
		u.RawQuery = query.Encode()
	}

	return u.String()
}

// buildMySQLTestDSN translates fields into go-sql-driver/mysql's own DSN
// grammar ("user:pass@tcp(host:port)/db?params") — this translation does not
// exist anywhere else in the codebase (internal/dbengine/mysql/mysql.go's
// own doc comment on New flags it as a gap left for whoever wires the
// connection form). It builds a mysqldriver.Config and calls its own
// FormatDSN rather than hand-formatting the string with fmt.Sprintf:
// FormatDSN is the exact counterpart of the driver's own ParseDSN, so it is
// guaranteed to produce a string the driver can always re-read correctly,
// including a username or password containing "@", ":", or other characters
// a naively concatenated string could misparse on the way back in.
//
// parseTime is always forced true (Config.ParseTime) — without it, MySQL
// DATETIME/TIMESTAMP columns scan as raw bytes instead of time.Time. Any
// "parseTime" key already present in fields.Params (e.g. pasted from a URL
// with an explicit ?parseTime=false) is dropped before copying the rest
// into Config.Params: FormatDSN would otherwise emit "parseTime=true" from
// Config.ParseTime AND a second "parseTime=<other value>" from Config.Params,
// and re-parsing that DSN lets the second, Params-derived occurrence
// silently win over the forced true.
func buildMySQLTestDSN(fields ConnectionFormFields) string {
	cfg := mysqldriver.NewConfig()
	cfg.User = fields.Username
	cfg.Passwd = fields.Password
	cfg.Net = "tcp"
	cfg.Addr = fmt.Sprintf("%s:%d", fields.Host, fields.Port)
	cfg.DBName = fields.Database
	cfg.ParseTime = true

	if len(fields.Params) > 0 {
		cfg.Params = make(map[string]string, len(fields.Params))
		for key, value := range fields.Params {
			if strings.EqualFold(key, "parseTime") {
				continue
			}
			cfg.Params[key] = value
		}
	}

	return cfg.FormatDSN()
}

// newTestEngine constructs the dbengine.Engine for fields.Engine, translating
// fields into that engine's own connection-string/DSN format. MongoDB and
// Redis have no dbengine.Engine implementation yet (Phase 5/6, tasks.md) —
// pasting a mongodb:// or redis:// URL autofills the form fine
// (urlparse.go already supports all 4 schemes), but Test Connection can't
// dial either one yet, so this returns a clear "not yet supported" error
// instead of a nil dereference or a silent no-op.
func newTestEngine(fields ConnectionFormFields) (dbengine.Engine, error) {
	switch fields.Engine {
	case storage.EnginePostgres:
		return dbenginepostgres.New(buildPostgresTestConnString(fields)), nil
	case storage.EngineMySQL:
		return dbenginemysql.New(buildMySQLTestDSN(fields)), nil
	case storage.EngineMongoDB:
		return nil, fmt.Errorf("MongoDB connections are not yet supported (coming in a later phase)")
	case storage.EngineRedis:
		return nil, fmt.Errorf("Redis connections are not yet supported (coming in a later phase)")
	default:
		return nil, fmt.Errorf("unsupported engine %q", fields.Engine)
	}
}

// TestConnection proves reachability for the given form fields (tasks.md
// 3.4, spec.md §4.1's "Test connection" button). It does NOT persist a
// saved connection — that is task 3.5's job. It builds the right connection
// string/DSN for fields.Engine, constructs that engine's dbengine.Engine,
// then runs Connect followed by Ping under testConnectionTimeout so an
// unreachable host fails fast rather than hanging the UI. The connection is
// always closed before returning, whether the test succeeded or failed.
func (a *App) TestConnection(fields ConnectionFormFields) error {
	if err := validateConnectionFormFields(fields); err != nil {
		return fmt.Errorf("test connection: %w", err)
	}

	engine, err := newTestEngine(fields)
	if err != nil {
		return fmt.Errorf("test connection: %w", err)
	}

	ctx, cancel := context.WithTimeout(a.ctx, testConnectionTimeout)
	defer cancel()

	if err := engine.Connect(ctx); err != nil {
		return fmt.Errorf("test connection: %w", err)
	}
	defer engine.Close()

	if err := engine.Ping(ctx); err != nil {
		return fmt.Errorf("test connection: %w", err)
	}
	return nil
}

// stringPtrOrNil returns nil for an empty string, or a pointer to s
// otherwise — the boundary that turns a connection form's empty text field
// into a NULL column, matching Service's own nullable-field convention
// (models.go) rather than storing an empty string in a Connection's
// Username/PasswordEncrypted/Database.
func stringPtrOrNil(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// stringOrEmpty dereferences an optional *string, returning "" for nil — the
// inverse of stringPtrOrNil, used when rehydrating a stored Connection back
// into ConnectionFormFields.
func stringOrEmpty(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// paramsToJSON marshals a connection form's Params into the JSON string
// storage.Connection.ParamsJSON expects (tasks.md 3.5), defaulting a nil or
// empty map to "{}" rather than storing an empty string, which is not valid
// JSON and would fail to round-trip through paramsFromJSON.
func paramsToJSON(params map[string]string) (string, error) {
	if len(params) == 0 {
		return "{}", nil
	}
	data, err := json.Marshal(params)
	if err != nil {
		return "", fmt.Errorf("encode connection params: %w", err)
	}
	return string(data), nil
}

// paramsFromJSON reverses paramsToJSON, decoding a stored Connection's
// ParamsJSON back into map[string]string.
func paramsFromJSON(raw string) (map[string]string, error) {
	if raw == "" {
		return map[string]string{}, nil
	}
	var params map[string]string
	if err := json.Unmarshal([]byte(raw), &params); err != nil {
		return nil, fmt.Errorf("decode connection params: %w", err)
	}
	return params, nil
}

// connectionFormFieldsFromStored converts a saved storage.Connection back
// into the same ConnectionFormFields shape the connection form UI already
// works with (task 3.4), so loading a saved connection populates the form
// exactly like ParseConnectionURL does for a freshly pasted string.
func connectionFormFieldsFromStored(c storage.Connection) (ConnectionFormFields, error) {
	params, err := paramsFromJSON(c.ParamsJSON)
	if err != nil {
		return ConnectionFormFields{}, fmt.Errorf("saved connection %d: %w", c.ID, err)
	}

	return ConnectionFormFields{
		Engine:   c.Engine,
		Host:     c.Host,
		Port:     c.Port,
		Username: stringOrEmpty(c.Username),
		Password: stringOrEmpty(c.PasswordEncrypted),
		Database: stringOrEmpty(c.Database),
		Params:   params,
	}, nil
}

// ListConnections returns every saved DB Client connection, ordered by name
// (tasks.md 3.5, spec.md §4.1). PasswordEncrypted is returned as-is —
// plaintext-in-practice today, same standing gap TestConnection/SaveConnection
// already carry (see docs/STATE.md); this method doesn't make that gap
// worse, it also doesn't fix it.
func (a *App) ListConnections() ([]storage.Connection, error) {
	db, err := a.requireDB()
	if err != nil {
		return nil, err
	}

	connections, err := storage.ListConnections(db)
	if err != nil {
		return nil, fmt.Errorf("list connections: %w", err)
	}
	if connections == nil {
		connections = []storage.Connection{}
	}
	return connections, nil
}

// SaveConnection persists fields under name as a new saved connection
// (tasks.md 3.5, spec.md §4.1), reusing task 3.4's ConnectionFormFields
// rather than a separate request shape. It re-runs
// validateConnectionFormFields's host/port sanity check, but deliberately
// does NOT itself perform a live connectivity test: Test Connection (3.4)
// and Save are two independent, separately-triggered actions in the UI —
// typically test-then-save, but saving an untested connection is allowed.
func (a *App) SaveConnection(fields ConnectionFormFields, name string) (*storage.Connection, error) {
	db, err := a.requireDB()
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(name) == "" {
		return nil, fmt.Errorf("save connection: name is required")
	}
	if err := validateConnectionFormFields(fields); err != nil {
		return nil, fmt.Errorf("save connection: %w", err)
	}

	paramsJSON, err := paramsToJSON(fields.Params)
	if err != nil {
		return nil, fmt.Errorf("save connection %q: %w", name, err)
	}

	created, err := storage.CreateConnection(db, &storage.Connection{
		Name:              name,
		Engine:            fields.Engine,
		Host:              fields.Host,
		Port:              fields.Port,
		Username:          stringPtrOrNil(fields.Username),
		PasswordEncrypted: stringPtrOrNil(fields.Password),
		Database:          stringPtrOrNil(fields.Database),
		ParamsJSON:        paramsJSON,
	})
	if err != nil {
		return nil, fmt.Errorf("save connection %q: %w", name, err)
	}
	return created, nil
}

// DeleteConnection permanently removes a saved connection (tasks.md 3.5).
func (a *App) DeleteConnection(id int64) error {
	db, err := a.requireDB()
	if err != nil {
		return err
	}
	if err := storage.DeleteConnection(db, id); err != nil {
		return fmt.Errorf("delete connection %d: %w", id, err)
	}
	return nil
}

// ConnectUsingSavedConnection loads a saved connection back into
// ConnectionFormFields shape — the same shape ParseConnectionURL produces —
// so the frontend can populate the connection form from a saved row with one
// click, then Test/edit it exactly like a freshly pasted URL (tasks.md 3.5).
// This is also the single trigger point that bumps LastUsedAt: a saved
// connection counts as "used" the moment the user asks to load/connect
// through it, not merely when it appears in the list, and not tied to a
// second, redundant TestConnection call the UI didn't ask for.
func (a *App) ConnectUsingSavedConnection(id int64) (*ConnectionFormFields, error) {
	db, err := a.requireDB()
	if err != nil {
		return nil, err
	}

	conn, err := storage.TouchConnectionLastUsed(db, id)
	if err != nil {
		return nil, fmt.Errorf("connect using saved connection %d: %w", id, err)
	}

	fields, err := connectionFormFieldsFromStored(*conn)
	if err != nil {
		return nil, fmt.Errorf("connect using saved connection %d: %w", id, err)
	}
	return &fields, nil
}
