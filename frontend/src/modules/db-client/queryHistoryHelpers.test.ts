import {describe, expect, it} from 'vitest'
import {truncateQueryText} from './queryHistoryHelpers'

describe('truncateQueryText', () => {
    it('returns short text unchanged', () => {
        expect(truncateQueryText('SELECT 1')).toBe('SELECT 1')
    })

    it('returns text at exactly maxLength unchanged', () => {
        const text = 'x'.repeat(80)
        expect(truncateQueryText(text)).toBe(text)
    })

    it('truncates text over maxLength and appends an ellipsis', () => {
        const text = 'x'.repeat(100)
        const result = truncateQueryText(text)
        expect(result).toHaveLength(80)
        expect(result.endsWith('…')).toBe(true)
        expect(result.slice(0, 79)).toBe('x'.repeat(79))
    })

    it('collapses newlines and repeated whitespace into single spaces', () => {
        expect(truncateQueryText('SELECT *\n  FROM   widgets\nWHERE id = 1')).toBe(
            'SELECT * FROM widgets WHERE id = 1',
        )
    })

    it('trims leading and trailing whitespace', () => {
        expect(truncateQueryText('   SELECT 1   ')).toBe('SELECT 1')
    })

    it('honors a custom maxLength', () => {
        expect(truncateQueryText('SELECT * FROM widgets', 10)).toBe('SELECT * …')
    })

    it('handles an empty string', () => {
        expect(truncateQueryText('')).toBe('')
    })
})
