import {describe, expect, it} from 'vitest'
import {dbengine} from '../../../wailsjs/go/models'
import {
    buildBlankRowValues,
    buildInsertPayload,
    coerceCellInput,
    defaultValueForColumn,
    extractPkValues,
    isBooleanColumnType,
    isNumericColumnType,
    isPkLessError,
    isTableEditable,
    primaryKeyColumnNames,
} from './gridEditHelpers'

function column(overrides: Partial<dbengine.ColumnInfo>): dbengine.ColumnInfo {
    return new dbengine.ColumnInfo({
        Name: 'col',
        DataType: 'text',
        Nullable: false,
        IsPrimaryKey: false,
        HasDefault: false,
        ...overrides,
    })
}

describe('isNumericColumnType', () => {
    it('matches common numeric SQL type names', () => {
        expect(isNumericColumnType('integer')).toBe(true)
        expect(isNumericColumnType('BIGINT')).toBe(true)
        expect(isNumericColumnType('serial')).toBe(true)
        expect(isNumericColumnType('numeric(10,2)')).toBe(true)
        expect(isNumericColumnType('double precision')).toBe(true)
    })

    it('does not match non-numeric type names', () => {
        expect(isNumericColumnType('varchar(255)')).toBe(false)
        expect(isNumericColumnType('text')).toBe(false)
        expect(isNumericColumnType('boolean')).toBe(false)
    })
})

describe('isBooleanColumnType', () => {
    it('matches boolean type names', () => {
        expect(isBooleanColumnType('boolean')).toBe(true)
        expect(isBooleanColumnType('BOOL')).toBe(true)
        expect(isBooleanColumnType('tinyint(1)')).toBe(false)
    })
})

describe('isTableEditable', () => {
    it('is true when at least one column is a primary key', () => {
        const columns = [column({Name: 'id', IsPrimaryKey: true}), column({Name: 'name'})]
        expect(isTableEditable(columns)).toBe(true)
    })

    it('is false when no column is a primary key', () => {
        const columns = [column({Name: 'a'}), column({Name: 'b'})]
        expect(isTableEditable(columns)).toBe(false)
    })

    it('is false for an empty column list', () => {
        expect(isTableEditable([])).toBe(false)
    })
})

describe('primaryKeyColumnNames', () => {
    it('returns only primary key column names', () => {
        const columns = [
            column({Name: 'id', IsPrimaryKey: true}),
            column({Name: 'tenant_id', IsPrimaryKey: true}),
            column({Name: 'name'}),
        ]
        expect(primaryKeyColumnNames(columns)).toEqual(['id', 'tenant_id'])
    })
})

describe('extractPkValues', () => {
    const columns = [column({Name: 'id', DataType: 'integer', IsPrimaryKey: true}), column({Name: 'name'})]
    const resultColumnNames = ['name', 'id']

    it('builds a pk map by matching column names regardless of row column order', () => {
        const row = ['Ada', 7]
        expect(extractPkValues(columns, resultColumnNames, row)).toEqual({id: 7})
    })

    it('returns null when the table has no primary key columns', () => {
        expect(extractPkValues([column({Name: 'name'})], ['name'], ['Ada'])).toBeNull()
    })

    it('returns null when a primary key value in the row is null', () => {
        expect(extractPkValues(columns, resultColumnNames, ['Ada', null])).toBeNull()
    })

    it('returns null when a primary key column is missing from the result columns', () => {
        expect(extractPkValues(columns, ['name'], ['Ada'])).toBeNull()
    })
})

describe('defaultValueForColumn', () => {
    it('defaults nullable columns to null regardless of type', () => {
        expect(defaultValueForColumn(column({Nullable: true, DataType: 'integer'}))).toBeNull()
        expect(defaultValueForColumn(column({Nullable: true, DataType: 'text'}))).toBeNull()
    })

    it('defaults non-nullable numeric columns to 0', () => {
        expect(defaultValueForColumn(column({Nullable: false, DataType: 'bigint'}))).toBe(0)
    })

    it('defaults non-nullable boolean columns to false', () => {
        expect(defaultValueForColumn(column({Nullable: false, DataType: 'boolean'}))).toBe(false)
    })

    it('defaults non-nullable text-like columns to an empty string', () => {
        expect(defaultValueForColumn(column({Nullable: false, DataType: 'varchar(255)'}))).toBe('')
    })
})

describe('buildBlankRowValues', () => {
    it('generates a default value for every column, keyed by name', () => {
        const columns = [
            column({Name: 'id', DataType: 'integer', Nullable: false, IsPrimaryKey: true}),
            column({Name: 'label', DataType: 'text', Nullable: false}),
            column({Name: 'archived', DataType: 'boolean', Nullable: false}),
            column({Name: 'note', DataType: 'text', Nullable: true}),
        ]
        expect(buildBlankRowValues(columns)).toEqual({id: 0, label: '', archived: false, note: null})
    })
})

describe('coerceCellInput', () => {
    it('coerces an empty string to null for a nullable column', () => {
        expect(coerceCellInput(column({Nullable: true}), '')).toBeNull()
    })

    it('coerces an empty string to an empty string for a non-nullable column', () => {
        expect(coerceCellInput(column({Nullable: false}), '')).toBe('')
    })

    it('coerces numeric-looking input to a number for a numeric column', () => {
        expect(coerceCellInput(column({DataType: 'integer'}), '42')).toBe(42)
    })

    it('leaves non-numeric input untouched for a numeric column so the DB reports the type mismatch', () => {
        expect(coerceCellInput(column({DataType: 'integer'}), 'not-a-number')).toBe('not-a-number')
    })

    it('coerces true/false input to booleans for a boolean column', () => {
        expect(coerceCellInput(column({DataType: 'boolean'}), 'true')).toBe(true)
        expect(coerceCellInput(column({DataType: 'boolean'}), 'false')).toBe(false)
    })

    it('passes through text input for a text column unchanged', () => {
        expect(coerceCellInput(column({DataType: 'text'}), 'hello world')).toBe('hello world')
    })
})

describe('buildInsertPayload', () => {
    const columns = [
        column({Name: 'id', DataType: 'integer', Nullable: false, IsPrimaryKey: true}),
        column({Name: 'label', DataType: 'text', Nullable: false}),
    ]

    it('omits an untouched primary key column so the database can auto-generate it', () => {
        const payload = buildInsertPayload(columns, {id: 0, label: 'hello'}, new Set(['label']))
        expect(payload).toEqual({label: 'hello'})
    })

    it('includes a primary key column the user explicitly touched', () => {
        const payload = buildInsertPayload(columns, {id: 99, label: 'hello'}, new Set(['id', 'label']))
        expect(payload).toEqual({id: 99, label: 'hello'})
    })

    it('includes untouched non-primary-key columns without a database default, using their current value', () => {
        const payload = buildInsertPayload(columns, {id: 0, label: ''}, new Set())
        expect(payload).toEqual({label: ''})
    })

    it('omits an untouched column with a database-level default so the database can apply it', () => {
        const columnsWithDefault = [
            column({Name: 'id', DataType: 'integer', Nullable: false, IsPrimaryKey: true}),
            column({Name: 'label', DataType: 'text', Nullable: false}),
            column({Name: 'status', DataType: 'text', Nullable: false, HasDefault: true}),
        ]
        const payload = buildInsertPayload(
            columnsWithDefault,
            {id: 0, label: 'hello', status: ''},
            new Set(['label']),
        )
        expect(payload).toEqual({label: 'hello'})
    })

    it('includes a column with a database-level default when the user explicitly touched it', () => {
        const columnsWithDefault = [
            column({Name: 'id', DataType: 'integer', Nullable: false, IsPrimaryKey: true}),
            column({Name: 'label', DataType: 'text', Nullable: false}),
            column({Name: 'status', DataType: 'text', Nullable: false, HasDefault: true}),
        ]
        const payload = buildInsertPayload(
            columnsWithDefault,
            {id: 0, label: 'hello', status: 'active'},
            new Set(['label', 'status']),
        )
        expect(payload).toEqual({label: 'hello', status: 'active'})
    })
})

describe('isPkLessError', () => {
    it('matches the backend PK-less error substring wrapped with extra context', () => {
        expect(isPkLessError('update table row: read-only: table has no primary key')).toBe(true)
        expect(isPkLessError('delete table rows: read-only: table has no primary key')).toBe(true)
    })

    it('does not match unrelated error messages', () => {
        expect(isPkLessError('update table row: no row matched the given primary key values')).toBe(false)
    })
})
