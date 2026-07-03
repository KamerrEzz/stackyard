package migrations

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	"stackyard/internal/dbengine"
)

// selectAppliedVersionsSQL is dialect-neutral (see bootstrap.go's own doc
// comment for why schema_migrations' shape allows this), so loadAppliedVersions
// never needs a per-engine branch either.
const selectAppliedVersionsSQL = `SELECT version FROM schema_migrations`

// ApplyResult reports the outcome of one Apply run (tasks.md 8.3). Applied
// lists every migration that was successfully applied, in the order it ran.
// If a migration's up-SQL or its schema_migrations tracking row failed to
// write, Failed identifies exactly which migration and FailedError carries
// the underlying database error's message — every migration after it (in
// DiscoverMigrations' ascending-Version order) is left untouched, never
// attempted. Failed and FailedError are both zero when every pending
// migration applied successfully.
type ApplyResult struct {
	Applied     []Migration
	Failed      *Migration
	FailedError string
}

// PendingMigrations returns the subset of all (in the order given — callers
// pass DiscoverMigrations' ascending-Version result) whose Version is not a
// key of applied. It is pure: no I/O, no database access, so the "compute
// what's pending" logic can be tested directly against arbitrary
// all/applied inputs, independent of Apply's DB-touching execution loop
// below.
func PendingMigrations(all []Migration, applied map[int64]bool) []Migration {
	pending := make([]Migration, 0, len(all))
	for _, m := range all {
		if !applied[m.Version] {
			pending = append(pending, m)
		}
	}
	return pending
}

// Apply discovers every migration in folder, computes the pending set
// against schema_migrations' already-applied versions, and runs each
// pending migration's up.sql content in ascending Version order (spec.md
// §4.8). Each migration's up-SQL and its schema_migrations tracking row
// insert are executed inside one transaction via engine's
// dbengine.Transactor support — engine's own native BEGIN/COMMIT/ROLLBACK,
// not a sequence of separate Exec calls — so a migration can never end up
// with its schema change applied but no tracking row, or a tracking row but
// no schema change. Apply stops at the first migration whose transaction
// fails to commit; every migration after it is never attempted, and the
// failure is reported via ApplyResult.Failed/FailedError rather than a bare
// error, so a caller still learns which earlier migrations (if any)
// succeeded. Apply's own non-nil error return is reserved for failures
// outside any single migration's SQL: folder discovery, reading
// already-applied versions, or engine not implementing
// dbengine.Transactor at all.
func Apply(ctx context.Context, engine dbengine.Engine, dialect dbengine.Dialect, folder string) (*ApplyResult, error) {
	transactor, ok := engine.(dbengine.Transactor)
	if !ok {
		return nil, fmt.Errorf("migrations: apply: engine %T does not support transactions", engine)
	}

	all, err := DiscoverMigrations(folder)
	if err != nil {
		return nil, fmt.Errorf("migrations: apply: %w", err)
	}

	applied, err := LoadAppliedVersions(ctx, engine)
	if err != nil {
		return nil, fmt.Errorf("migrations: apply: %w", err)
	}

	pending := PendingMigrations(all, applied)

	result := &ApplyResult{Applied: make([]Migration, 0, len(pending))}
	for _, m := range pending {
		if err := applyOne(ctx, transactor, dialect, m); err != nil {
			result.Failed = &m
			result.FailedError = err.Error()
			return result, nil
		}
		result.Applied = append(result.Applied, m)
	}
	return result, nil
}

// applyOne runs m's up.sql content and records its schema_migrations
// tracking row inside a single transaction, rolling the transaction back on
// either statement's failure so neither side of the "schema change +
// tracking row" pair is ever left half-done.
func applyOne(ctx context.Context, transactor dbengine.Transactor, dialect dbengine.Dialect, m Migration) error {
	upSQL, err := os.ReadFile(m.UpPath)
	if err != nil {
		return fmt.Errorf("read up file %q: %w", m.UpPath, err)
	}

	tx, err := transactor.BeginTx(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction for migration %s: %w", m.Name(), err)
	}

	if _, err := tx.Exec(ctx, string(upSQL)); err != nil {
		_ = tx.Rollback(ctx)
		return fmt.Errorf("apply migration %s: %w", m.Name(), err)
	}

	if _, err := tx.Exec(ctx, trackingRowInsertSQL(dialect), m.Version, m.Name(), time.Now().UTC()); err != nil {
		_ = tx.Rollback(ctx)
		return fmt.Errorf("record migration %s as applied: %w", m.Name(), err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit migration %s: %w", m.Name(), err)
	}
	return nil
}

// trackingRowInsertSQL returns the schema_migrations INSERT statement for
// dialect's own placeholder convention ("$1,$2,$3" for Postgres, "?,?,?" for
// MySQL) — the one piece of Apply/Rollback that must branch per dialect,
// since bootstrap.go's CREATE TABLE is the only schema_migrations statement
// that gets to stay dialect-neutral.
func trackingRowInsertSQL(dialect dbengine.Dialect) string {
	if dialect == dbengine.DialectPostgres {
		return `INSERT INTO schema_migrations (version, name, applied_at) VALUES ($1, $2, $3)`
	}
	return `INSERT INTO schema_migrations (version, name, applied_at) VALUES (?, ?, ?)`
}

// LoadAppliedVersions reads every version already recorded in
// schema_migrations and returns it as a set, ready for PendingMigrations.
// Exported (rather than kept package-private like the rest of this file's
// helpers) specifically for tasks.md 8.5's migrations panel: the frontend
// needs to cross-reference ListMigrations' on-disk file list against the
// applied set to render pending/applied status, which is otherwise not
// exposed anywhere Apply/Rollback's own control flow needs it as a public
// return value.
func LoadAppliedVersions(ctx context.Context, engine dbengine.Engine) (map[int64]bool, error) {
	result, err := engine.Query(ctx, selectAppliedVersionsSQL)
	if err != nil {
		return nil, fmt.Errorf("load applied versions: %w", err)
	}

	applied := make(map[int64]bool, len(result.Rows))
	for _, row := range result.Rows {
		if len(row) == 0 {
			continue
		}
		version, err := toInt64(row[0])
		if err != nil {
			return nil, fmt.Errorf("load applied versions: %w", err)
		}
		applied[version] = true
	}
	return applied, nil
}

// toInt64 converts one schema_migrations.version cell into int64. Postgres
// (pgx) and MySQL (go-sql-driver, depending on protocol/column metadata)
// don't necessarily report a BIGINT column as the same Go type, so every
// shape either driver is known to produce is handled explicitly rather than
// assuming one.
func toInt64(v any) (int64, error) {
	switch t := v.(type) {
	case int64:
		return t, nil
	case int32:
		return int64(t), nil
	case int:
		return int64(t), nil
	case string:
		n, err := strconv.ParseInt(t, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("parse version %q as int64: %w", t, err)
		}
		return n, nil
	case []byte:
		n, err := strconv.ParseInt(string(t), 10, 64)
		if err != nil {
			return 0, fmt.Errorf("parse version %q as int64: %w", string(t), err)
		}
		return n, nil
	default:
		return 0, fmt.Errorf("unsupported schema_migrations.version value type %T", v)
	}
}
