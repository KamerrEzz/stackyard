import {describe, expect, it} from 'vitest'
import type {storage} from '../../../wailsjs/go/models'
import {findMostRecentCompatibleConnection, resolveRunSnippetTarget, resolveSnippetConnectionSource} from './snippetRunLogic'

function connection(overrides: Partial<storage.Connection> & {ID: number}): storage.Connection {
    return {
        ID: overrides.ID,
        Name: overrides.Name ?? `conn-${overrides.ID}`,
        Engine: overrides.Engine ?? 'postgres',
        Host: overrides.Host ?? 'localhost',
        Port: overrides.Port ?? 5432,
        Username: overrides.Username,
        PasswordEncrypted: overrides.PasswordEncrypted,
        Database: overrides.Database,
        ParamsJSON: overrides.ParamsJSON ?? '{}',
        LastUsedAt: overrides.LastUsedAt,
    }
}

describe('resolveRunSnippetTarget', () => {
    it('targets the current tab when it is active and not dirty', () => {
        expect(resolveRunSnippetTarget('tab-1', false)).toEqual({kind: 'current-tab', tabId: 'tab-1'})
    })

    it('opens a new tab when the active tab is dirty', () => {
        expect(resolveRunSnippetTarget('tab-1', true)).toEqual({kind: 'new-tab'})
    })

    it('opens a new tab when there is no active tab at all', () => {
        expect(resolveRunSnippetTarget(null, false)).toEqual({kind: 'new-tab'})
        expect(resolveRunSnippetTarget(null, true)).toEqual({kind: 'new-tab'})
    })
})

describe('resolveSnippetConnectionSource', () => {
    it('always uses the snippet own connection when it is scoped, regardless of tab state', () => {
        expect(resolveSnippetConnectionSource(42, true, 7)).toEqual({kind: 'scoped', connectionId: 42})
        expect(resolveSnippetConnectionSource(42, false, null)).toEqual({kind: 'scoped', connectionId: 42})
    })

    it('reuses the active tab connection for a global snippet when a tab is open', () => {
        expect(resolveSnippetConnectionSource(null, true, 7)).toEqual({kind: 'reuse-active-tab'})
    })

    it('falls back to the most recently used compatible connection with no active tab', () => {
        expect(resolveSnippetConnectionSource(null, false, 9)).toEqual({
            kind: 'most-recent-compatible',
            connectionId: 9,
        })
    })

    it('gives up when the snippet is global, no tab is open, and no compatible connection exists', () => {
        expect(resolveSnippetConnectionSource(null, false, null)).toEqual({kind: 'none'})
    })
})

describe('findMostRecentCompatibleConnection', () => {
    it('returns null when no connection matches the engine', () => {
        const connections = [connection({ID: 1, Engine: 'mysql'})]
        expect(findMostRecentCompatibleConnection(connections, 'postgres')).toBeNull()
    })

    it('picks the most recently used connection among matching-engine candidates', () => {
        const older = connection({ID: 1, Engine: 'postgres', LastUsedAt: '2026-01-01T00:00:00Z'})
        const newer = connection({ID: 2, Engine: 'postgres', LastUsedAt: '2026-06-01T00:00:00Z'})
        const otherEngine = connection({ID: 3, Engine: 'mysql', LastUsedAt: '2026-12-01T00:00:00Z'})
        expect(findMostRecentCompatibleConnection([older, newer, otherEngine], 'postgres')).toEqual(newer)
    })

    it('treats a connection that was never used as older than one that was', () => {
        const neverUsed = connection({ID: 1, Engine: 'postgres', LastUsedAt: undefined})
        const used = connection({ID: 2, Engine: 'postgres', LastUsedAt: '2026-01-01T00:00:00Z'})
        expect(findMostRecentCompatibleConnection([neverUsed, used], 'postgres')).toEqual(used)
        expect(findMostRecentCompatibleConnection([used, neverUsed], 'postgres')).toEqual(used)
    })

    it('returns the only candidate even if it was never used', () => {
        const neverUsed = connection({ID: 1, Engine: 'postgres', LastUsedAt: undefined})
        expect(findMostRecentCompatibleConnection([neverUsed], 'postgres')).toEqual(neverUsed)
    })
})
