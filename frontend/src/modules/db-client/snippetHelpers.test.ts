import {describe, expect, it} from 'vitest'
import {parseTagsInput, parseTagsJSON, tagsToInput} from './snippetHelpers'

describe('parseTagsInput', () => {
    it('splits a comma-separated string into trimmed tags', () => {
        expect(parseTagsInput('postgres, reporting , users')).toEqual(['postgres', 'reporting', 'users'])
    })

    it('drops empty entries from stray/trailing commas', () => {
        expect(parseTagsInput('a,,b,')).toEqual(['a', 'b'])
    })

    it('deduplicates repeated tags, keeping first occurrence order', () => {
        expect(parseTagsInput('a, b, a, c, b')).toEqual(['a', 'b', 'c'])
    })

    it('returns an empty array for blank input', () => {
        expect(parseTagsInput('')).toEqual([])
        expect(parseTagsInput('   ')).toEqual([])
    })

    it('is a no-op for a single tag with no commas', () => {
        expect(parseTagsInput('reporting')).toEqual(['reporting'])
    })
})

describe('tagsToInput', () => {
    it('joins tags with a comma and space', () => {
        expect(tagsToInput(['a', 'b', 'c'])).toBe('a, b, c')
    })

    it('returns an empty string for undefined or empty tags', () => {
        expect(tagsToInput(undefined)).toBe('')
        expect(tagsToInput([])).toBe('')
    })

    it('round-trips through parseTagsInput', () => {
        const original = ['postgres', 'reporting', 'users']
        expect(parseTagsInput(tagsToInput(original))).toEqual(original)
    })
})

describe('parseTagsJSON', () => {
    it('parses a JSON array string into a string array', () => {
        expect(parseTagsJSON('["postgres","reporting"]')).toEqual(['postgres', 'reporting'])
    })

    it('returns an empty array for undefined, empty, or malformed input', () => {
        expect(parseTagsJSON(undefined)).toEqual([])
        expect(parseTagsJSON('')).toEqual([])
        expect(parseTagsJSON('not-json')).toEqual([])
    })

    it('returns an empty array for valid JSON that is not an array', () => {
        expect(parseTagsJSON('{"a":1}')).toEqual([])
    })

    it('filters out non-string entries from the parsed array', () => {
        expect(parseTagsJSON('["a", 1, "b", null]')).toEqual(['a', 'b'])
    })
})
