import {describe, expect, it} from 'vitest'
import {
    classifyMongoValue,
    formatDocumentForDuplicate,
    formatDocumentForEdit,
    stripId,
    summarizeContainer,
    toggleExpandedPath,
    validateDocumentJSON,
} from './mongoDocumentHelpers'

describe('classifyMongoValue', () => {
    it('classifies null and undefined as null', () => {
        expect(classifyMongoValue(null)).toBe('null')
        expect(classifyMongoValue(undefined)).toBe('null')
    })

    it('classifies primitives', () => {
        expect(classifyMongoValue(42)).toBe('number')
        expect(classifyMongoValue(3.14)).toBe('number')
        expect(classifyMongoValue(true)).toBe('boolean')
        expect(classifyMongoValue(false)).toBe('boolean')
    })

    it('classifies arrays and objects as containers', () => {
        expect(classifyMongoValue([1, 2, 3])).toBe('array')
        expect(classifyMongoValue({a: 1})).toBe('object')
        expect(classifyMongoValue({})).toBe('object')
        expect(classifyMongoValue([])).toBe('array')
    })

    it('classifies a 24-char hex string as an ObjectID', () => {
        expect(classifyMongoValue('507f1f77bcf86cd799439011')).toBe('objectid')
        expect(classifyMongoValue('507F1F77BCF86CD799439011')).toBe('objectid')
    })

    it('classifies an RFC3339-ish string as a date', () => {
        expect(classifyMongoValue('2026-07-02T10:15:30Z')).toBe('date')
        expect(classifyMongoValue('2026-07-02T10:15:30.123Z')).toBe('date')
        expect(classifyMongoValue('2026-07-02T10:15:30+02:00')).toBe('date')
    })

    it('classifies an ordinary string as a plain string', () => {
        expect(classifyMongoValue('hello world')).toBe('string')
        expect(classifyMongoValue('not-24-hex')).toBe('string')
        expect(classifyMongoValue('')).toBe('string')
    })

    it('does not misclassify a 24-char non-hex string as an ObjectID', () => {
        expect(classifyMongoValue('this-is-not-a-hex-string')).toBe('string')
    })
})

describe('summarizeContainer', () => {
    it('describes an object by key count', () => {
        expect(summarizeContainer({a: 1, b: 2})).toBe('{2 keys}')
        expect(summarizeContainer({a: 1})).toBe('{1 key}')
        expect(summarizeContainer({})).toBe('{0 keys}')
    })

    it('describes an array by item count', () => {
        expect(summarizeContainer([1, 2, 3])).toBe('[3 items]')
        expect(summarizeContainer([1])).toBe('[1 item]')
        expect(summarizeContainer([])).toBe('[0 items]')
    })
})

describe('validateDocumentJSON', () => {
    it('accepts a well-formed JSON object', () => {
        const result = validateDocumentJSON('{"name": "Ada", "age": 30}')
        expect(result.ok).toBe(true)
        if (result.ok) {
            expect(result.value).toEqual({name: 'Ada', age: 30})
        }
    })

    it('accepts an empty object', () => {
        const result = validateDocumentJSON('{}')
        expect(result.ok).toBe(true)
    })

    it('rejects malformed JSON (dangling comma)', () => {
        const result = validateDocumentJSON('{"name": "Ada",}')
        expect(result.ok).toBe(false)
        if (!result.ok) {
            expect(result.error.length).toBeGreaterThan(0)
        }
    })

    it('rejects malformed JSON (unmatched brace)', () => {
        const result = validateDocumentJSON('{"name": "Ada"')
        expect(result.ok).toBe(false)
    })

    it('rejects a well-formed JSON array', () => {
        const result = validateDocumentJSON('[1, 2, 3]')
        expect(result.ok).toBe(false)
        if (!result.ok) {
            expect(result.error).toMatch(/object/i)
        }
    })

    it('rejects a well-formed bare scalar', () => {
        expect(validateDocumentJSON('42').ok).toBe(false)
        expect(validateDocumentJSON('"just a string"').ok).toBe(false)
        expect(validateDocumentJSON('null').ok).toBe(false)
    })
})

describe('stripId', () => {
    it('removes _id and keeps other fields', () => {
        const result = stripId({_id: 'abc123', name: 'Ada'})
        expect(result).toEqual({name: 'Ada'})
        expect('_id' in result).toBe(false)
    })

    it('is a no-op when _id is absent', () => {
        const doc = {name: 'Ada'}
        expect(stripId(doc)).toEqual({name: 'Ada'})
    })
})

describe('formatDocumentForDuplicate / formatDocumentForEdit', () => {
    it('duplicate formatting omits _id', () => {
        const text = formatDocumentForDuplicate({_id: 'abc123', name: 'Ada'})
        expect(text).not.toContain('_id')
        expect(JSON.parse(text)).toEqual({name: 'Ada'})
    })

    it('edit formatting keeps _id', () => {
        const text = formatDocumentForEdit({_id: 'abc123', name: 'Ada'})
        expect(JSON.parse(text)).toEqual({_id: 'abc123', name: 'Ada'})
    })
})

describe('toggleExpandedPath', () => {
    it('adds a path not yet present', () => {
        const result = toggleExpandedPath(new Set(), 'address')
        expect(result.has('address')).toBe(true)
    })

    it('removes a path already present', () => {
        const result = toggleExpandedPath(new Set(['address']), 'address')
        expect(result.has('address')).toBe(false)
    })

    it('never mutates the input set', () => {
        const input = new Set(['a'])
        toggleExpandedPath(input, 'b')
        expect(input.has('b')).toBe(false)
        expect(input.size).toBe(1)
    })
})
