import {describe, expect, it} from 'vitest'
import {resolveSnippetFilterScope} from './snippetFilterLogic'

describe('resolveSnippetFilterScope', () => {
    it('returns the unscoped filter when no tab is active', () => {
        expect(resolveSnippetFilterScope(null)).toEqual({connectionId: 0, engine: ''})
    })

    it('scopes to the engine only for an ad-hoc active tab (no saved connection id)', () => {
        expect(resolveSnippetFilterScope({savedConnectionId: 0, engine: 'mysql'})).toEqual({
            connectionId: 0,
            engine: 'mysql',
        })
    })

    it('scopes to both the connection id and engine for a saved-connection active tab', () => {
        expect(resolveSnippetFilterScope({savedConnectionId: 7, engine: 'postgres'})).toEqual({
            connectionId: 7,
            engine: 'postgres',
        })
    })
})
