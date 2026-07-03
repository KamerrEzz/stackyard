import type {migrations, storage} from '../../../wailsjs/go/models'

export type MigrationStatus = 'applied' | 'pending'

export interface MigrationListItem {
    version: number
    name: string
    status: MigrationStatus
}

/**
 * Reconstructs `migrations.Migration.Name()`'s exact format
 * (`<version>_<slug>`, e.g. `20260703120000_create_users_table`) on the
 * frontend — Wails' bindgen only carries a Go struct's fields across the
 * IPC boundary, never its methods, so `Migration.Name()` itself isn't
 * available here.
 */
export function migrationName(m: Pick<migrations.Migration, 'Version' | 'Slug'>): string {
    return `${m.Version}_${m.Slug}`
}

/**
 * Cross-references `ListMigrations`' on-disk file list against
 * `ListAppliedMigrationVersions`' applied-versions set (tasks.md 8.5) to
 * compute each migration's pending/applied status, mirroring the same
 * split `internal/migrations.Apply` already uses server-side
 * (`PendingMigrations`/`LoadAppliedVersions`) — this is that same logic,
 * reimplemented as a pure frontend function since the two IPC calls it
 * combines return independently and neither reports the other's half.
 * Preserves `all`'s given order (the caller passes `ListMigrations`'
 * already-ascending-by-version result), so no sorting happens here.
 */
export function computeMigrationStatuses(
    all: migrations.Migration[],
    appliedVersions: number[],
): MigrationListItem[] {
    const applied = new Set(appliedVersions)
    return all.map((m) => ({
        version: m.Version,
        name: migrationName(m),
        status: applied.has(m.Version) ? 'applied' : 'pending',
    }))
}

/**
 * A saved connection is eligible for the migrations panel only when it's
 * PostgreSQL or MySQL (spec.md §4.8's own title: "Migrations (PostgreSQL,
 * MySQL only — v1)") — MongoDB/Redis connections are filtered out of the
 * connection picker entirely rather than shown disabled, since there is no
 * migrations feature for them at all in v1, not merely a disabled one.
 */
export function isRelationalConnection(conn: Pick<storage.Connection, 'Engine'>): boolean {
    return conn.Engine === 'postgres' || conn.Engine === 'mysql'
}

/**
 * Whether Rollback should be enabled at all (tasks.md 8.5's confirmation
 * judgment call): Rollback is only offered once at least one migration is
 * known to be applied, computed from the same cross-reference
 * `computeMigrationStatuses` produces. This lets the panel show a
 * confirmation dialog only when a real, non-empty rollback is about to
 * run — never a confirm-then-"nothing to roll back" dead end — since the
 * frontend already knows the applied count before calling
 * `RollbackMigration`.
 */
export function hasAnyAppliedMigration(items: MigrationListItem[]): boolean {
    return items.some((item) => item.status === 'applied')
}
