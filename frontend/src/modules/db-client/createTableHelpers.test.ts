import {describe, expect, it} from 'vitest'
import {
    buildColumnDefinitions,
    type ColumnFormRow,
    createBlankColumnRow,
    isAutoIncrementType,
    setPrimaryKey,
    validateCreateTableForm,
} from './createTableHelpers'

function row(overrides: Partial<ColumnFormRow>): ColumnFormRow {
    return {...createBlankColumnRow(), ...overrides}
}

describe('createBlankColumnRow', () => {
    it('starts as a nullable text column with no name and no primary key', () => {
        expect(createBlankColumnRow()).toEqual({
            name: '',
            type: 'text',
            nullable: true,
            isPrimaryKey: false,
            defaultValue: '',
        })
    })
})

describe('isAutoIncrementType', () => {
    it('flags serial and bigserial as auto-increment', () => {
        expect(isAutoIncrementType('serial')).toBe(true)
        expect(isAutoIncrementType('bigserial')).toBe(true)
    })

    it('does not flag other types', () => {
        expect(isAutoIncrementType('integer')).toBe(false)
        expect(isAutoIncrementType('text')).toBe(false)
    })
})

describe('setPrimaryKey', () => {
    it('marks the given row as primary key without touching others when none were set', () => {
        const rows = [row({name: 'id'}), row({name: 'name'})]
        const result = setPrimaryKey(rows, 0, true)
        expect(result[0].isPrimaryKey).toBe(true)
        expect(result[1].isPrimaryKey).toBe(false)
    })

    it('clears every other row when a new row is marked primary key (radio behavior)', () => {
        const rows = [row({name: 'id', isPrimaryKey: true}), row({name: 'name'})]
        const result = setPrimaryKey(rows, 1, true)
        expect(result[0].isPrimaryKey).toBe(false)
        expect(result[1].isPrimaryKey).toBe(true)
    })

    it('unmarking a row leaves every other row untouched', () => {
        const rows = [row({name: 'id', isPrimaryKey: true}), row({name: 'name'})]
        const result = setPrimaryKey(rows, 0, false)
        expect(result[0].isPrimaryKey).toBe(false)
        expect(result[1].isPrimaryKey).toBe(false)
    })

    it('does not mutate the original array', () => {
        const rows = [row({name: 'id'})]
        const result = setPrimaryKey(rows, 0, true)
        expect(rows[0].isPrimaryKey).toBe(false)
        expect(result).not.toBe(rows)
    })
})

describe('validateCreateTableForm', () => {
    it('requires a table name', () => {
        expect(validateCreateTableForm('', [row({name: 'id'})])).toMatch(/table name/i)
        expect(validateCreateTableForm('   ', [row({name: 'id'})])).toMatch(/table name/i)
    })

    it('requires at least one column', () => {
        expect(validateCreateTableForm('widgets', [])).toMatch(/at least one column/i)
    })

    it('requires every column to have a name', () => {
        expect(validateCreateTableForm('widgets', [row({name: ''})])).toMatch(/needs a name/i)
        expect(validateCreateTableForm('widgets', [row({name: '   '})])).toMatch(/needs a name/i)
    })

    it('rejects duplicate column names within the table', () => {
        const rows = [row({name: 'id'}), row({name: 'id'})]
        expect(validateCreateTableForm('widgets', rows)).toMatch(/duplicate column name/i)
    })

    it('rejects more than one primary key column', () => {
        const rows = [row({name: 'id', isPrimaryKey: true}), row({name: 'code', isPrimaryKey: true})]
        expect(validateCreateTableForm('widgets', rows)).toMatch(/only one primary key/i)
    })

    it('rejects an auto-increment column that is not the primary key', () => {
        const rows = [row({name: 'id', type: 'serial', isPrimaryKey: false})]
        expect(validateCreateTableForm('widgets', rows)).toMatch(/auto-increment and must be the primary key/i)
    })

    it('accepts a valid table with one primary key and no duplicates', () => {
        const rows = [
            row({name: 'id', type: 'serial', isPrimaryKey: true, nullable: false}),
            row({name: 'label', type: 'text'}),
        ]
        expect(validateCreateTableForm('widgets', rows)).toBeNull()
    })

    it('accepts a valid table with no primary key at all', () => {
        const rows = [row({name: 'note', type: 'text'})]
        expect(validateCreateTableForm('widgets', rows)).toBeNull()
    })
})

describe('buildColumnDefinitions', () => {
    it('builds one ColumnDefinition per row, trimming names and defaults', () => {
        const rows = [
            row({name: ' id ', type: 'serial', isPrimaryKey: true, nullable: false, defaultValue: '  '}),
            row({name: 'status', type: 'varchar', nullable: false, defaultValue: " 'active' "}),
        ]
        const defs = buildColumnDefinitions(rows)
        expect(defs).toHaveLength(2)
        expect(defs[0].Name).toBe('id')
        expect(defs[0].Type).toBe('serial')
        expect(defs[0].IsPrimaryKey).toBe(true)
        expect(defs[0].Nullable).toBe(false)
        expect(defs[0].Default).toBeUndefined()
        expect(defs[1].Name).toBe('status')
        expect(defs[1].Default).toBe("'active'")
    })
})
