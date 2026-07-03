// Package snippettemplates holds Stackyard's built-in gallery of starter SQL
// snippet templates (tasks.md 10.3): a curated, read-only library the DB
// Client's Template gallery lets a user browse and insert when starting a
// new project from scratch. This is deliberately separate from
// internal/storage's Snippet, which is the user's own saved snippets (task
// 4.6/spec.md §4.7) backed by a SQLite table the user creates, edits, and
// deletes. A Template here is fixed data compiled into the binary — never
// persisted, never mutated at runtime, and never stored as a row in the
// snippets table. A user who likes a Template can still turn it into their
// own editable Snippet through the existing CreateSnippet bound method; see
// app.go's ListSnippetTemplates doc comment for how the two connect.
package snippettemplates

import "stackyard/internal/storage"

// Template is one built-in starter SQL snippet. Its underlying schema idea
// (e.g. "a users table") is often portable across engines, but the SQL text
// itself rarely is verbatim — SERIAL/BIGSERIAL vs AUTO_INCREMENT, TIMESTAMPTZ
// vs TIMESTAMP, JSONB vs JSON, and reserved-word quoting all differ enough
// between Postgres and MySQL that one shared string would either be wrong
// for one engine or avoid every engine-specific feature worth using. Each
// Template therefore carries one dialect-correct SQL string per engine in
// SQL, keyed by storage.Engine so a caller already holding a connection's
// engine value (main.ConnectionFormFields.Engine, a storage.Engine string
// under the hood) can index directly. A Template that doesn't make sense on
// one engine simply omits that key rather than forcing an approximate or
// broken variant into existence.
type Template struct {
	ID          string
	Name        string
	Description string
	SQL         map[storage.Engine]string
}

var templates = []Template{
	{
		ID:   "auth-users-sessions",
		Name: "Auth: users + sessions + tokens",
		Description: "A users table with typical auth fields (email, password hash, " +
			"created_at) plus a refresh_tokens table referencing it via foreign key " +
			"— a starting point for a from-scratch authentication schema.",
		SQL: map[storage.Engine]string{
			storage.EnginePostgres: `CREATE TABLE users (
    id BIGSERIAL PRIMARY KEY,
    email TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE refresh_tokens (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL UNIQUE,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);`,
			storage.EngineMySQL: `CREATE TABLE users (
    id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    email VARCHAR(255) NOT NULL UNIQUE,
    password_hash VARCHAR(255) NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
) ENGINE=InnoDB;

CREATE TABLE refresh_tokens (
    id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    user_id BIGINT UNSIGNED NOT NULL,
    token_hash VARCHAR(255) NOT NULL UNIQUE,
    expires_at TIMESTAMP NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT fk_refresh_tokens_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
) ENGINE=InnoDB;`,
		},
	},
	{
		ID:          "audit-log",
		Name:        "Audit log",
		Description: "An append-only audit_log table recording who did what to which record — basic change tracking from day one.",
		SQL: map[storage.Engine]string{
			storage.EnginePostgres: `CREATE TABLE audit_log (
    id BIGSERIAL PRIMARY KEY,
    actor TEXT NOT NULL,
    action TEXT NOT NULL,
    target_type TEXT NOT NULL,
    target_id TEXT,
    metadata JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);`,
			// metadata is left nullable (no DEFAULT) on MySQL: MySQL's JSON
			// column type does not accept a simple literal DEFAULT — only an
			// expression default (DEFAULT (JSON_OBJECT())), which requires
			// MySQL 8.0.13+ and is brittle to depend on for a template meant
			// to run unmodified on "MySQL", not one specific patch line.
			// Callers that want a non-null starting value insert one
			// explicitly, same as any other nullable column.
			storage.EngineMySQL: `CREATE TABLE audit_log (
    id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    actor VARCHAR(255) NOT NULL,
    action VARCHAR(255) NOT NULL,
    target_type VARCHAR(255) NOT NULL,
    target_id VARCHAR(255),
    metadata JSON NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
) ENGINE=InnoDB;`,
		},
	},
	{
		ID:          "settings-kv",
		Name:        "Settings / key-value config",
		Description: "A generic settings table for arbitrary key-value application configuration, keyed by a unique text key.",
		SQL: map[storage.Engine]string{
			storage.EnginePostgres: `CREATE TABLE settings (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);`,
			// key is quoted with backticks on MySQL, where it is a reserved
			// word, and declared VARCHAR(255) rather than TEXT: MySQL refuses
			// a BLOB/TEXT column as a primary key without an explicit key
			// length, unlike Postgres's TEXT PRIMARY KEY above.
			storage.EngineMySQL: "CREATE TABLE settings (\n" +
				"    `key` VARCHAR(255) PRIMARY KEY,\n" +
				"    value TEXT NOT NULL,\n" +
				"    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP\n" +
				") ENGINE=InnoDB;",
		},
	},
}

// List returns every built-in Template, in the fixed order they are defined
// above. The returned slice is a defensive copy of the slice header (not a
// deep copy of each Template's SQL map) — every caller reached through
// app.go's ListSnippetTemplates only ever reads a Template, so sharing the
// same underlying values is safe.
func List() []Template {
	out := make([]Template, len(templates))
	copy(out, templates)
	return out
}
