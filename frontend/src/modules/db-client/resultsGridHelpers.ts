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

export interface ServerPageInfo {
    pageNumber: number
    startIndex: number
    endIndex: number
    hasPrevPage: boolean
    hasNextPage: boolean
}

/**
 * Describes one page of a backend-paginated `BrowseTableRows` result (tasks.md
 * 4.1's "View: paginated row browsing" requirement, spec.md §4.3) for the
 * grid's Prev/Next controls. `BrowseTableRows` returns only one page at a
 * time and the backend never reports a total row count, so unlike
 * `paginateRows` (which slices an already-fully-fetched array), there is no
 * `pageCount`/`totalRows` here to compute from — `hasNextPage` is instead a
 * heuristic: the most recently fetched page came back with exactly `limit`
 * rows (`fetchedRowCount === limit`), meaning more rows may exist beyond it,
 * versus fewer than `limit` rows, meaning this was the last page. `offset`
 * and `limit` are the values the last `BrowseTableRows` call was made with;
 * `fetchedRowCount` is that call's own returned row count (captured before
 * any local edit/delete mutates the displayed rows, so a row deleted from the
 * current page never flips `hasNextPage` back to false); `displayedRowCount`
 * is the (possibly locally mutated) row count actually rendered, used only
 * for the human-readable `startIndex`/`endIndex` range.
 */
export function describeServerPage(
    offset: number,
    limit: number,
    fetchedRowCount: number,
    displayedRowCount: number,
): ServerPageInfo {
    return {
        pageNumber: Math.floor(offset / limit) + 1,
        startIndex: displayedRowCount === 0 ? 0 : offset + 1,
        endIndex: displayedRowCount === 0 ? 0 : offset + displayedRowCount,
        hasPrevPage: offset > 0,
        hasNextPage: fetchedRowCount === limit,
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
