import {useEffect, useState} from 'react'
import type {dbengine} from '../../../wailsjs/go/models'
import {RESULTS_PAGE_SIZE, describeCell, paginateRows} from './resultsGridHelpers'

interface ResultsGridProps {
    result: dbengine.QueryResult
}

/**
 * Read-only grid for a single `Engine.Query` result: column headers with
 * type metadata, NULL-aware cell rendering, and client-side pagination over
 * the already-fully-fetched `result.Rows` (see resultsGridHelpers.ts for the
 * pagination scope note).
 */
function ResultsGrid({result}: ResultsGridProps) {
    const [page, setPage] = useState(1)

    useEffect(() => {
        setPage(1)
    }, [result])

    const columns = result.Columns ?? []
    const rows = result.Rows ?? []
    const {pageRows, totalRows, currentPage, pageCount, startIndex, endIndex} = paginateRows(
        rows,
        page,
        RESULTS_PAGE_SIZE,
    )

    return (
        <div className="flex flex-col gap-2">
            <div className="overflow-auto rounded border border-ink-800">
                <table className="w-full border-collapse text-left text-xs">
                    <thead>
                        <tr className="bg-ink-900">
                            {columns.map((column, index) => (
                                <th key={index} className="border-b border-ink-800 px-3 py-2 font-medium text-ink-300">
                                    <div className="flex items-center gap-1.5">
                                        <span>{column.Name}</span>
                                        {column.Nullable === true && (
                                            <span className="rounded border border-ink-700 px-1 text-[10px] font-normal normal-case text-ink-500">
                                                NULL
                                            </span>
                                        )}
                                    </div>
                                    <div className="text-[10px] font-normal normal-case text-ink-500">
                                        {column.DatabaseType}
                                    </div>
                                </th>
                            ))}
                        </tr>
                    </thead>
                    <tbody>
                        {pageRows.map((row, rowIndex) => (
                            <tr key={rowIndex} className="odd:bg-ink-950/40">
                                {row.map((cell, cellIndex) => {
                                    const display = describeCell(cell)
                                    return (
                                        <td
                                            key={cellIndex}
                                            className="border-b border-ink-900 px-3 py-1.5 font-mono text-ink-200"
                                        >
                                            {display.isNull ? (
                                                <span className="italic text-ink-600">NULL</span>
                                            ) : (
                                                display.text
                                            )}
                                        </td>
                                    )
                                })}
                            </tr>
                        ))}
                    </tbody>
                </table>
                {totalRows === 0 && (
                    <p className="p-3 text-xs text-ink-500">Query succeeded with no rows returned.</p>
                )}
            </div>

            {totalRows > 0 && (
                <div className="flex items-center justify-between text-xs text-ink-400">
                    <span>
                        Showing {startIndex}-{endIndex} of {totalRows} rows
                    </span>
                    <div className="flex items-center gap-2">
                        <button
                            type="button"
                            onClick={() => setPage((current) => Math.max(1, current - 1))}
                            disabled={currentPage <= 1}
                            className="rounded border border-ink-700 px-2 py-1 text-ink-300 transition-colors hover:border-brass-500 hover:text-brass-500 disabled:cursor-not-allowed disabled:opacity-40"
                        >
                            Prev
                        </button>
                        <span className="text-ink-500">
                            Page {currentPage} of {pageCount}
                        </span>
                        <button
                            type="button"
                            onClick={() => setPage((current) => Math.min(pageCount, current + 1))}
                            disabled={currentPage >= pageCount}
                            className="rounded border border-ink-700 px-2 py-1 text-ink-300 transition-colors hover:border-brass-500 hover:text-brass-500 disabled:cursor-not-allowed disabled:opacity-40"
                        >
                            Next
                        </button>
                    </div>
                </div>
            )}
        </div>
    )
}

export default ResultsGrid
