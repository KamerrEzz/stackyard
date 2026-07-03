import {describe, expect, it} from 'vitest'
import {
    computeMigrationStatuses,
    hasAnyAppliedMigration,
    isRelationalConnection,
    migrationName,
} from './migrationHelpers'

function migration(version: number, slug: string) {
    return {Version: version, Slug: slug, UpPath: `${version}_${slug}.up.sql`, DownPath: `${version}_${slug}.down.sql`}
}

describe('migrationName', () => {
    it('joins version and slug with an underscore', () => {
        expect(migrationName({Version: 20260703120000, Slug: 'create_users_table'})).toBe(
            '20260703120000_create_users_table',
        )
    })
})

describe('computeMigrationStatuses', () => {
    it('marks every migration pending when nothing is applied', () => {
        const all = [migration(1, 'first'), migration(2, 'second')]
        const result = computeMigrationStatuses(all, [])
        expect(result).toEqual([
            {version: 1, name: '1_first', status: 'pending'},
            {version: 2, name: '2_second', status: 'pending'},
        ])
    })

    it('marks matching versions applied and leaves the rest pending', () => {
        const all = [migration(1, 'first'), migration(2, 'second'), migration(3, 'third')]
        const result = computeMigrationStatuses(all, [1, 3])
        expect(result.map((r) => r.status)).toEqual(['applied', 'pending', 'applied'])
    })

    it('ignores an applied version with no matching file (defensive, should not happen)', () => {
        const all = [migration(1, 'first')]
        const result = computeMigrationStatuses(all, [1, 999])
        expect(result).toEqual([{version: 1, name: '1_first', status: 'applied'}])
    })

    it('preserves the input order rather than re-sorting', () => {
        const all = [migration(3, 'third'), migration(1, 'first')]
        const result = computeMigrationStatuses(all, [])
        expect(result.map((r) => r.version)).toEqual([3, 1])
    })

    it('returns an empty list for an empty migration set', () => {
        expect(computeMigrationStatuses([], [1, 2])).toEqual([])
    })
})

describe('isRelationalConnection', () => {
    it('accepts postgres and mysql', () => {
        expect(isRelationalConnection({Engine: 'postgres'})).toBe(true)
        expect(isRelationalConnection({Engine: 'mysql'})).toBe(true)
    })

    it('rejects mongodb and redis', () => {
        expect(isRelationalConnection({Engine: 'mongodb'})).toBe(false)
        expect(isRelationalConnection({Engine: 'redis'})).toBe(false)
    })
})

describe('hasAnyAppliedMigration', () => {
    it('is false when the list is empty or entirely pending', () => {
        expect(hasAnyAppliedMigration([])).toBe(false)
        expect(hasAnyAppliedMigration([{version: 1, name: '1_a', status: 'pending'}])).toBe(false)
    })

    it('is true when at least one migration is applied', () => {
        expect(
            hasAnyAppliedMigration([
                {version: 1, name: '1_a', status: 'pending'},
                {version: 2, name: '2_b', status: 'applied'},
            ]),
        ).toBe(true)
    })
})
