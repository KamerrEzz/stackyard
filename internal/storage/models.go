package storage

// Engine identifies which database engine a Service or Connection targets.
type Engine string

// Supported engines, matching the CHECK constraints in migrations.go.
const (
	EnginePostgres Engine = "postgres"
	EngineMySQL    Engine = "mysql"
	EngineMongoDB  Engine = "mongodb"
	EngineRedis    Engine = "redis"
)

// Profile is a named, reusable set of Services.
type Profile struct {
	ID        int64
	Name      string
	CreatedAt string // stored as an ISO-8601 / RFC3339 string, see migrations.go
}

// Service is one engine instance within a Profile.
//
// Username, PasswordEncrypted, and DBName are nullable because not every
// engine needs all three — Redis in particular has neither a username nor a
// database name in the same sense Postgres/MySQL/Mongo do.
type Service struct {
	ID                int64
	ProfileID         int64
	Engine            Engine
	ImageTag          string
	HostPort          int
	Username          *string
	PasswordEncrypted *string
	DBName            *string
	VolumeName        string
}

// Connection is a DB Client saved connection — either pointing at a
// Stackyard-managed Service or an arbitrary external host.
//
// MigrationsFolder is nil until explicitly set via
// SetConnectionMigrationsFolder — it is the on-disk folder internal/
// migrations (Phase 8) scaffolds and discovers migration file pairs in for
// this connection (plan.md §4: migration file content lives on disk, not in
// this SQLite database). It is deliberately excluded from
// CreateConnection/UpdateConnection's generic column list, the same way
// LastUsedAt is excluded from UpdateConnection — see
// SetConnectionMigrationsFolder's doc comment.
type Connection struct {
	ID                int64
	Name              string
	Engine            Engine
	Host              string
	Port              int
	Username          *string
	PasswordEncrypted *string
	Database          *string
	ParamsJSON        string // raw JSON object, e.g. {"sslmode":"require"}
	LastUsedAt        *string
	MigrationsFolder  *string
}

// Snippet is a saved, reusable query.
//
// ConnectionID is nil for a snippet marked "global" (usable from any
// connection of a compatible Engine). ON DELETE SET NULL on the FK means
// deleting the connection a snippet was scoped to demotes it to global
// rather than deleting it.
type Snippet struct {
	ID           int64
	ConnectionID *int64
	Engine       Engine
	Name         string
	Body         string
	TagsJSON     string // raw JSON array, e.g. ["postgres","reporting"]
	CreatedAt    string
	UpdatedAt    string
}

// QueryHistoryEntry is one logged query execution.
type QueryHistoryEntry struct {
	ID           int64
	ConnectionID int64
	QueryText    string
	ExecutedAt   string
	DurationMs   int64
	Success      bool
	RowsAffected int64
	ErrorMessage *string
}
