import type {snippettemplates} from '../../../wailsjs/go/models'

export type GalleryEngine = 'postgres' | 'mysql'

/**
 * Reads the SQL variant for engine out of a built-in Template's SQL map
 * (tasks.md 10.3), returning null when that Template has no variant for
 * engine at all — some templates may only make sense on one engine, and the
 * gallery/DbClientView both need to tell that apart from "this engine's
 * variant happens to be an empty string" (which would also be falsy).
 */
export function sqlForEngine(template: snippettemplates.Template, engine: string): string | null {
    const sql = template.SQL[engine]
    return sql === undefined ? null : sql
}

/**
 * Picks which engine the Template gallery's Postgres/MySQL toggle should
 * default to, given the currently active DB Client tab's engine (or null
 * with no tab active). Only 'mysql' opts into the MySQL variant; every
 * other value (postgres, mongodb, redis, or no active tab) defaults to
 * 'postgres' — templates are relational-only (tasks.md 10.3's clarified
 * scope), so a Mongo/Redis tab has no matching variant either way, and
 * Postgres is as reasonable a default as any for that case.
 */
export function defaultGalleryEngine(activeEngine: string | null): GalleryEngine {
    return activeEngine === 'mysql' ? 'mysql' : 'postgres'
}
