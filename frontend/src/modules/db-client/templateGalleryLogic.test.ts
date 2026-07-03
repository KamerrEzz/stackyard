import {describe, expect, it} from 'vitest'
import type {snippettemplates} from '../../../wailsjs/go/models'
import {defaultGalleryEngine, sqlForEngine} from './templateGalleryLogic'

function template(sql: Record<string, string>): snippettemplates.Template {
    return {ID: 'auth-users-sessions', Name: 'Auth', Description: 'desc', SQL: sql}
}

describe('sqlForEngine', () => {
    it('returns the SQL string for an engine present in the map', () => {
        expect(sqlForEngine(template({postgres: 'CREATE TABLE users (id INT)'}), 'postgres')).toBe(
            'CREATE TABLE users (id INT)',
        )
    })

    it('returns null when the template has no variant for that engine', () => {
        expect(sqlForEngine(template({postgres: 'CREATE TABLE users (id INT)'}), 'mysql')).toBeNull()
    })

    it('distinguishes a missing engine from one that happens to be empty', () => {
        expect(sqlForEngine(template({mysql: ''}), 'mysql')).toBe('')
        expect(sqlForEngine(template({mysql: ''}), 'postgres')).toBeNull()
    })
})

describe('defaultGalleryEngine', () => {
    it('defaults to postgres when no tab is active', () => {
        expect(defaultGalleryEngine(null)).toBe('postgres')
    })

    it('defaults to mysql when the active tab is mysql', () => {
        expect(defaultGalleryEngine('mysql')).toBe('mysql')
    })

    it('defaults to postgres for the active tab already being postgres', () => {
        expect(defaultGalleryEngine('postgres')).toBe('postgres')
    })

    it('defaults to postgres for a non-relational active engine (mongodb/redis)', () => {
        expect(defaultGalleryEngine('mongodb')).toBe('postgres')
        expect(defaultGalleryEngine('redis')).toBe('postgres')
    })
})
