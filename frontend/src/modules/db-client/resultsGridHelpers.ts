export const RESULTS_PAGE_SIZE = 100

export interface PaginationResult<T> {
    pageRows: T[]
    totalRows: number
    currentPage: number
    pageCount: number
    startIndex: number
    endIndex: number
}

/**
 * Slices `rows` into the page requested (1-indexed), clamping out-of-range
 * page numbers into bounds. `startIndex`/`endIndex` are 1-indexed and both
 * `0` when `rows` is empty, ready to feed a "Showing X-Y of Z rows" label.
 */
export function paginateRows<T>(rows: readonly T[], page: number, pageSize: number): PaginationResult<T> {
    const totalRows = rows.length
    const pageCount = totalRows === 0 ? 1 : Math.ceil(totalRows / pageSize)
    const currentPage = Math.min(Math.max(page, 1), pageCount)
    const startOffset = (currentPage - 1) * pageSize
    const pageRows = rows.slice(startOffset, startOffset + pageSize)

    return {
        pageRows,
        totalRows,
        currentPage,
        pageCount,
        startIndex: totalRows === 0 ? 0 : startOffset + 1,
        endIndex: totalRows === 0 ? 0 : startOffset + pageRows.length,
    }
}

export type CellDisplay = {isNull: true} | {isNull: false; text: string}

/**
 * Distinguishes a genuinely absent value (`null`/`undefined`, rendered as a
 * "NULL" label) from an empty string (rendered as empty text) so the two
 * are never visually confused in the grid.
 */
export function describeCell(value: unknown): CellDisplay {
    if (value === null || value === undefined) {
        return {isNull: true}
    }
    return {isNull: false, text: String(value)}
}
