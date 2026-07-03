import {describe, expect, it} from 'vitest'
import type {dbengine} from '../../../wailsjs/go/models'
import {buildQueryResultExportPayload, describeExportOutcome, describeExportScope} from './exportHelpers'

describe('describeExportScope', () => {
    it('offers csv, json, and sql for a table scope', () => {
        expect(describeExportScope(true)).toEqual({scope: 'table', availableFormats: ['csv', 'json', 'sql']})
    })

    it('offers only csv and json for a query result scope', () => {
        expect(describeExportScope(false)).toEqual({scope: 'result', availableFormats: ['csv', 'json']})
    })
})

describe('buildQueryResultExportPayload', () => {
    it('extracts column names and rows from a QueryResult', () => {
        const result = {
            Columns: [{Name: 'id', DatabaseType: 'int4', Nullable: null}, {Name: 'name', DatabaseType: 'text', Nullable: null}],
            Rows: [
                [1, 'bolt'],
                [2, null],
            ],
            RowsAffected: 0,
            LastInsertID: 0,
            Duration: 0,
        } as unknown as dbengine.QueryResult

        expect(buildQueryResultExportPayload(result)).toEqual({
            columnNames: ['id', 'name'],
            rows: [
                [1, 'bolt'],
                [2, null],
            ],
        })
    })

    it('defaults to empty arrays when Columns/Rows are missing', () => {
        const result = {Columns: undefined, Rows: undefined} as unknown as dbengine.QueryResult
        expect(buildQueryResultExportPayload(result)).toEqual({columnNames: [], rows: []})
    })
})

describe('describeExportOutcome', () => {
    it('reports cancelled for an empty path', () => {
        expect(describeExportOutcome('')).toEqual({status: 'cancelled'})
    })

    it('reports saved with the path otherwise', () => {
        expect(describeExportOutcome('C:/exports/widgets.csv')).toEqual({status: 'saved', path: 'C:/exports/widgets.csv'})
    })
})
