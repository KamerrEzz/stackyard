import {describe, expect, it} from 'vitest'
import {
    INITIAL_CURSOR_PAGE_STATE,
    applyCursorPage,
    canLoadMore,
    formatTTL,
    parseLineValues,
    parseScoreInput,
    validateHashJSON,
} from './redisKeyHelpers'

describe('formatTTL', () => {
    it('reports no expiry for any negative value', () => {
        expect(formatTTL(-1)).toBe('No expiry')
        expect(formatTTL(-1_000_000_000)).toBe('No expiry')
    })

    it('formats zero as 0s', () => {
        expect(formatTTL(0)).toBe('0s')
    })

    it('formats a sub-minute duration as seconds only', () => {
        expect(formatTTL(5_000_000_000)).toBe('5s')
    })

    it('formats a duration with minutes and seconds', () => {
        expect(formatTTL(65_000_000_000)).toBe('1m 5s')
    })

    it('formats a duration with hours, minutes, and seconds', () => {
        expect(formatTTL(3_661_000_000_000)).toBe('1h 1m 1s')
    })

    it('rounds sub-second remainders to the nearest second', () => {
        expect(formatTTL(1_499_000_000)).toBe('1s')
        expect(formatTTL(1_500_000_000)).toBe('2s')
    })
})

describe('applyCursorPage / canLoadMore', () => {
    it('starts with an empty, not-yet-scanned state', () => {
        expect(INITIAL_CURSOR_PAGE_STATE.items).toEqual([])
        expect(canLoadMore(INITIAL_CURSOR_PAGE_STATE)).toBe(false)
    })

    it('replace mode discards prior items and keeps the new cursor', () => {
        const afterFirst = applyCursorPage(INITIAL_CURSOR_PAGE_STATE, {items: ['a', 'b'], nextCursor: 7}, 'replace')
        expect(afterFirst.items).toEqual(['a', 'b'])
        expect(afterFirst.cursor).toBe(7)
        expect(canLoadMore(afterFirst)).toBe(true)
    })

    it('append mode concatenates onto prior items', () => {
        const afterFirst = applyCursorPage(INITIAL_CURSOR_PAGE_STATE, {items: ['a'], nextCursor: 7}, 'replace')
        const afterSecond = applyCursorPage(afterFirst, {items: ['b'], nextCursor: 0}, 'append')
        expect(afterSecond.items).toEqual(['a', 'b'])
        expect(afterSecond.cursor).toBe(0)
    })

    it('a returned cursor of 0 means the scan is complete, not that it never started', () => {
        const complete = applyCursorPage(INITIAL_CURSOR_PAGE_STATE, {items: ['a'], nextCursor: 0}, 'replace')
        expect(complete.hasScanned).toBe(true)
        expect(canLoadMore(complete)).toBe(false)
    })

    it('distinguishes "never scanned" from "scan complete", both cursor 0', () => {
        expect(canLoadMore(INITIAL_CURSOR_PAGE_STATE)).toBe(false)
        const scannedAndDone = applyCursorPage(INITIAL_CURSOR_PAGE_STATE, {items: [], nextCursor: 0}, 'replace')
        expect(scannedAndDone.cursor).toBe(INITIAL_CURSOR_PAGE_STATE.cursor)
        expect(canLoadMore(scannedAndDone)).toBe(false)
        expect(scannedAndDone.hasScanned).not.toBe(INITIAL_CURSOR_PAGE_STATE.hasScanned)
    })
})

describe('validateHashJSON', () => {
    it('accepts a well-formed string-valued object', () => {
        const result = validateHashJSON('{"name": "Ada", "role": "admin"}')
        expect(result.ok).toBe(true)
        if (result.ok) {
            expect(result.value).toEqual({name: 'Ada', role: 'admin'})
        }
    })

    it('accepts an empty object', () => {
        expect(validateHashJSON('{}').ok).toBe(true)
    })

    it('rejects malformed JSON', () => {
        const result = validateHashJSON('{"name": "Ada",}')
        expect(result.ok).toBe(false)
    })

    it('rejects a well-formed JSON array', () => {
        const result = validateHashJSON('[1, 2, 3]')
        expect(result.ok).toBe(false)
        if (!result.ok) {
            expect(result.error).toMatch(/object/i)
        }
    })

    it('rejects a well-formed bare scalar', () => {
        expect(validateHashJSON('42').ok).toBe(false)
        expect(validateHashJSON('null').ok).toBe(false)
    })

    it('rejects a non-string field value, naming the offending field', () => {
        const result = validateHashJSON('{"name": "Ada", "age": 30}')
        expect(result.ok).toBe(false)
        if (!result.ok) {
            expect(result.error).toContain('age')
        }
    })
})

describe('parseLineValues', () => {
    it('splits non-blank lines', () => {
        expect(parseLineValues('a\nb\nc')).toEqual(['a', 'b', 'c'])
    })

    it('drops blank lines', () => {
        expect(parseLineValues('a\n\n\nb\n')).toEqual(['a', 'b'])
    })

    it('returns an empty array for whitespace-only input', () => {
        expect(parseLineValues('   \n\n  ')).toEqual([])
    })

    it('preserves internal/trailing whitespace on a non-blank line', () => {
        expect(parseLineValues('  padded value  \n')).toEqual(['  padded value  '])
    })
})

describe('parseScoreInput', () => {
    it('accepts a well-formed integer or float', () => {
        expect(parseScoreInput('42')).toEqual({ok: true, score: 42})
        expect(parseScoreInput('3.14')).toEqual({ok: true, score: 3.14})
        expect(parseScoreInput('-5')).toEqual({ok: true, score: -5})
    })

    it('rejects blank input', () => {
        expect(parseScoreInput('').ok).toBe(false)
        expect(parseScoreInput('   ').ok).toBe(false)
    })

    it('rejects non-numeric input', () => {
        expect(parseScoreInput('not-a-number').ok).toBe(false)
    })
})
