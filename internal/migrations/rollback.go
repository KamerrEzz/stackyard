package migrations

import (
	"context"
	"fmt"
	"os"

	"stackyard/internal/dbengine"
)

// selectMostRecentAppliedSQL is dialect-neutral: ORDER BY ... DESC LIMIT 1
// is valid, identical syntax on both Postgres and MySQL, so
// loadMostRecentApplied never needs a per-engine branch.
const selectMostRecentAppliedSQL = `SELECT version, name FROM schema_migrations ORDER BY version DESC LIMIT 1`

// appliedRow is one row read back from schema_migrations by
// loadMostRecentApplied.
type appliedRow struct {
	version int64
	name    string
}

// Rollback reverts exactly the most-recently-applied migration — the
// highest applied version recorded in schema_migrations — never more than
// one step per call (spec.md §4.8: "no bulk rollback in v1"). It runs the
// migration's down.sql content and removes its tracking row inside a single
// transaction via engine's dbengine.Transactor support, with the same
// atomicity guarantee Apply's applyOne makes: the schema change and the
// tracking-row delete either both land or neither does.
//
// Rollback returns (nil, nil) — not an error — when schema_migrations has
// no rows: "nothing to roll back" is a normal, expected state (e.g. right
// after BootstrapTrackingTable, or after rolling every applied migration
// back), not a failure a caller should render as a red error banner. Every
// other returned error is a genuine failure: engine not supporting
// transactions, folder discovery, a tracking row whose version has no
// matching file pair in folder anymore, or the down-SQL/tracking-row
// removal itself failing.
func Rollback(ctx context.Context, engine dbengine.Engine, dialect dbengine.Dialect, folder string) (*Migration, error) {
	transactor, ok := engine.(dbengine.Transactor)
	if !ok {
		return nil, fmt.Errorf("migrations: rollback: engine %T does not support transactions", engine)
	}

	mostRecent, ok, err := loadMostRecentApplied(ctx, engine)
	if err != nil {
		return nil, fmt.Errorf("migrations: rollback: %w", err)
	}
	if !ok {
		return nil, nil
	}

	all, err := DiscoverMigrations(folder)
	if err != nil {
		return nil, fmt.Errorf("migrations: rollback: %w", err)
	}

	var target *Migration
	for i := range all {
		if all[i].Version == mostRecent.version {
			target = &all[i]
			break
		}
	}
	if target == nil {
		return nil, fmt.Errorf("migrations: rollback: migration version %d (%q) is recorded as applied but has no matching file pair in %q", mostRecent.version, mostRecent.name, folder)
	}

	if err := rollbackOne(ctx, transactor, dialect, *target); err != nil {
		return nil, fmt.Errorf("migrations: rollback: %w", err)
	}

	return target, nil
}

// rollbackOne runs target's down.sql content and removes its
// schema_migrations tracking row inside a single transaction, rolling the
// transaction back on either statement's failure.
func rollbackOne(ctx context.Context, transactor dbengine.Transactor, dialect dbengine.Dialect, target Migration) error {
	downSQL, err := os.ReadFile(target.DownPath)
	if err != nil {
		return fmt.Errorf("read down file %q: %w", target.DownPath, err)
	}

	tx, err := transactor.BeginTx(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction for migration %s: %w", target.Name(), err)
	}

	if _, err := tx.Exec(ctx, string(downSQL)); err != nil {
		_ = tx.Rollback(ctx)
		return fmt.Errorf("revert migration %s: %w", target.Name(), err)
	}

	if _, err := tx.Exec(ctx, trackingRowDeleteSQL(dialect), target.Version); err != nil {
		_ = tx.Rollback(ctx)
		return fmt.Errorf("remove tracking row for migration %s: %w", target.Name(), err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit rollback of migration %s: %w", target.Name(), err)
	}
	return nil
}

// trackingRowDeleteSQL returns the schema_migrations DELETE statement for
// dialect's own placeholder convention, mirroring trackingRowInsertSQL in
// apply.go.
func trackingRowDeleteSQL(dialect dbengine.Dialect) string {
	if dialect == dbengine.DialectPostgres {
		return `DELETE FROM schema_migrations WHERE version = $1`
	}
	return `DELETE FROM schema_migrations WHERE version = ?`
}

// loadMostRecentApplied reads the highest-version row from
// schema_migrations. Its second return value is false (with a zero
// appliedRow and nil error) when the table has no rows at all — the
// "nothing to roll back" case Rollback surfaces as (nil, nil).
func loadMostRecentApplied(ctx context.Context, engine dbengine.Engine) (appliedRow, bool, error) {
	result, err := engine.Query(ctx, selectMostRecentAppliedSQL)
	if err != nil {
		return appliedRow{}, false, fmt.Errorf("load most recent applied migration: %w", err)
	}
	if len(result.Rows) == 0 {
		return appliedRow{}, false, nil
	}

	row := result.Rows[0]
	if len(row) < 2 {
		return appliedRow{}, false, fmt.Errorf("load most recent applied migration: expected 2 columns, got %d", len(row))
	}

	version, err := toInt64(row[0])
	if err != nil {
		return appliedRow{}, false, fmt.Errorf("load most recent applied migration: %w", err)
	}
	name, err := toString(row[1])
	if err != nil {
		return appliedRow{}, false, fmt.Errorf("load most recent applied migration: %w", err)
	}
	return appliedRow{version: version, name: name}, true, nil
}

// toString converts one schema_migrations.name cell into string, handling
// both drivers' possible representations the same way toInt64 does for the
// version column.
func toString(v any) (string, error) {
	switch t := v.(type) {
	case string:
		return t, nil
	case []byte:
		return string(t), nil
	default:
		return "", fmt.Errorf("unsupported schema_migrations.name value type %T", v)
	}
}
