import {describe, expect, it} from 'vitest'
import {describeCell, paginateRows} from './resultsGridHelpers'

describe('paginateRows', () => {
    it('returns all rows on a single page when total is under the page size', () => {
        const rows = Array.from({length: 50}, (_, i) => i)
        const result = paginateRows(rows, 1, 100)

        expect(result.pageRows).toEqual(rows)
        expect(result.totalRows).toBe(50)
        expect(result.pageCount).toBe(1)
        expect(result.currentPage).toBe(1)
        expect(result.startIndex).toBe(1)
        expect(result.endIndex).toBe(50)
    })

    it('splits rows across exact page-size boundaries', () => {
        const rows = Array.from({length: 200}, (_, i) => i)

        const firstPage = paginateRows(rows, 1, 100)
        expect(firstPage.pageRows).toEqual(rows.slice(0, 100))
        expect(firstPage.pageCount).toBe(2)
        expect(firstPage.startIndex).toBe(1)
        expect(firstPage.endIndex).toBe(100)

        const secondPage = paginateRows(rows, 2, 100)
        expect(secondPage.pageRows).toEqual(rows.slice(100, 200))
        expect(secondPage.startIndex).toBe(101)
        expect(secondPage.endIndex).toBe(200)
    })

    it('returns a partial last page when total is not a multiple of the page size', () => {
        const rows = Array.from({length: 250}, (_, i) => i)
        const result = paginateRows(rows, 3, 100)

        expect(result.pageRows).toEqual(rows.slice(200, 250))
        expect(result.pageRows).toHaveLength(50)
        expect(result.pageCount).toBe(3)
        expect(result.startIndex).toBe(201)
        expect(result.endIndex).toBe(250)
    })

    it('handles zero rows without producing an invalid page', () => {
        const result = paginateRows([], 1, 100)

        expect(result.pageRows).toEqual([])
        expect(result.totalRows).toBe(0)
        expect(result.pageCount).toBe(1)
        expect(result.currentPage).toBe(1)
        expect(result.startIndex).toBe(0)
        expect(result.endIndex).toBe(0)
    })

    it('clamps an out-of-range requested page into bounds', () => {
        const rows = Array.from({length: 120}, (_, i) => i)

        const tooHigh = paginateRows(rows, 99, 100)
        expect(tooHigh.currentPage).toBe(2)
        expect(tooHigh.pageRows).toEqual(rows.slice(100, 120))

        const tooLow = paginateRows(rows, 0, 100)
        expect(tooLow.currentPage).toBe(1)
    })
})

describe('describeCell', () => {
    it('marks null as NULL', () => {
        expect(describeCell(null)).toEqual({isNull: true})
    })

    it('marks undefined as NULL', () => {
        expect(describeCell(undefined)).toEqual({isNull: true})
    })

    it('renders an empty string as genuinely empty, not NULL', () => {
        expect(describeCell('')).toEqual({isNull: false, text: ''})
    })

    it('renders a non-empty string as-is', () => {
        expect(describeCell('hello')).toEqual({isNull: false, text: 'hello'})
    })

    it('stringifies numeric and boolean values', () => {
        expect(describeCell(0)).toEqual({isNull: false, text: '0'})
        expect(describeCell(false)).toEqual({isNull: false, text: 'false'})
    })
})
