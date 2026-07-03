import {describe, expect, it} from 'vitest'
import {
    fileNameFromPath,
    formatMismatch,
    isImportableFilePath,
    sortMismatches,
    summarizeValidation,
} from './importHelpers'

describe('formatMismatch', () => {
    it('renders a file-level mismatch (RowIndex -1) without a row number', () => {
        const text = formatMismatch({RowIndex: -1, Column: 'ghost_column', Reason: 'column "ghost_column" does not exist on the target table'})
        expect(text).toBe('File — column "ghost_column": column "ghost_column" does not exist on the target table')
    })

    it('renders a row-scoped mismatch as 1-based', () => {
        const text = formatMismatch({RowIndex: 0, Column: 'weight', Reason: 'value "abc" for column "weight" does not look like a valid numeric'})
        expect(text).toBe('Row 1 — column "weight": value "abc" for column "weight" does not look like a valid numeric')
    })

    it('renders a later row correctly', () => {
        const text = formatMismatch({RowIndex: 4, Column: 'name', Reason: 'column "name" is not nullable but this row has no value for it'})
        expect(text).toContain('Row 5')
    })
})

describe('sortMismatches', () => {
    it('puts file-level mismatches first, then ascending row order', () => {
        const input = [
            {RowIndex: 2, Column: 'b', Reason: 'x'},
            {RowIndex: -1, Column: 'unknown', Reason: 'y'},
            {RowIndex: 0, Column: 'a', Reason: 'z'},
        ]
        const sorted = sortMismatches(input)
        expect(sorted.map((m) => m.RowIndex)).toEqual([-1, 0, 2])
    })

    it('does not mutate the input array', () => {
        const input = [
            {RowIndex: 2, Column: 'b', Reason: 'x'},
            {RowIndex: 0, Column: 'a', Reason: 'z'},
        ]
        sortMismatches(input)
        expect(input[0].RowIndex).toBe(2)
    })
})

describe('summarizeValidation', () => {
    it('reports all-clear with a singular row count', () => {
        expect(summarizeValidation({Mismatches: [], RowCount: 1})).toBe('All clear — 1 row ready to import.')
    })

    it('reports all-clear with a plural row count', () => {
        expect(summarizeValidation({Mismatches: [], RowCount: 12})).toBe('All clear — 12 rows ready to import.')
    })

    it('reports a singular mismatch count', () => {
        const result = {Mismatches: [{RowIndex: 0, Column: 'a', Reason: 'x'}], RowCount: 3}
        expect(summarizeValidation(result)).toBe('1 mismatch found — fix the file and try again.')
    })

    it('reports a plural mismatch count', () => {
        const result = {
            Mismatches: [
                {RowIndex: 0, Column: 'a', Reason: 'x'},
                {RowIndex: 1, Column: 'b', Reason: 'y'},
            ],
            RowCount: 3,
        }
        expect(summarizeValidation(result)).toBe('2 mismatches found — fix the file and try again.')
    })
})

describe('isImportableFilePath', () => {
    it('accepts .csv and .json regardless of case', () => {
        expect(isImportableFilePath('data.csv')).toBe(true)
        expect(isImportableFilePath('DATA.CSV')).toBe(true)
        expect(isImportableFilePath('data.json')).toBe(true)
        expect(isImportableFilePath('DATA.JSON')).toBe(true)
    })

    it('rejects any other extension', () => {
        expect(isImportableFilePath('data.txt')).toBe(false)
        expect(isImportableFilePath('data.xlsx')).toBe(false)
        expect(isImportableFilePath('data')).toBe(false)
    })
})

describe('fileNameFromPath', () => {
    it('extracts the file name from a Windows path', () => {
        expect(fileNameFromPath('C:\\Users\\ada\\Documents\\widgets.csv')).toBe('widgets.csv')
    })

    it('extracts the file name from a POSIX path', () => {
        expect(fileNameFromPath('/home/ada/widgets.json')).toBe('widgets.json')
    })

    it('returns the input unchanged when there is no separator', () => {
        expect(fileNameFromPath('widgets.csv')).toBe('widgets.csv')
    })
})
