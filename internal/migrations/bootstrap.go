package migrations

import (
	"context"
	"fmt"

	"stackyard/internal/dbengine"
)

// createSchemaMigrationsTableSQL is deliberately a single, dialect-neutral
// statement: BIGINT, VARCHAR(255), and TIMESTAMP column types plus
// "CREATE TABLE IF NOT EXISTS" are all valid, standard DDL on both Postgres
// and MySQL, so BootstrapTrackingTable never needs a per-engine branch —
// matching this package's stated goal of going through dbengine.Engine.Exec
// exactly like every other feature that touches the target database, with
// no new driver-specific code path.
//
// version holds the same 14-digit timestamp Migration.Version carries (see
// migrations.go), so a tracking-table row and the migration file it
// corresponds to are identified by the exact same number. applied_at
// records when that migration was actually run — this package only creates
// the table; populating rows into it is tasks.md 8.3's ("Apply") job, not
// this one's.
const createSchemaMigrationsTableSQL = `CREATE TABLE IF NOT EXISTS schema_migrations (
	version BIGINT NOT NULL PRIMARY KEY,
	name VARCHAR(255) NOT NULL,
	applied_at TIMESTAMP NOT NULL
)`

// BootstrapTrackingTable creates the schema_migrations table inside the
// database engine is connected to, if it doesn't already exist (tasks.md
// 8.2, plan.md §4). It is idempotent — calling it repeatedly against the
// same database never errors or duplicates the table, since the underlying
// statement is itself a "CREATE TABLE IF NOT EXISTS".
func BootstrapTrackingTable(ctx context.Context, engine dbengine.Engine) error {
	if _, err := engine.Exec(ctx, createSchemaMigrationsTableSQL); err != nil {
		return fmt.Errorf("migrations: bootstrap schema_migrations table: %w", err)
	}
	return nil
}
