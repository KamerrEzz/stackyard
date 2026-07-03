import {useEffect, useMemo, useState} from 'react'
import {DeleteTableRows, InsertTableRow, UpdateTableRow} from '../../../wailsjs/go/main/App'
import type {dbengine} from '../../../wailsjs/go/models'
import {
    buildBlankRowValues,
    buildInsertPayload,
    coerceCellInput,
    extractPkValues,
    isPkLessError,
    isTableEditable,
} from './gridEditHelpers'
import {RESULTS_PAGE_SIZE, describeCell, paginateRows} from './resultsGridHelpers'

/**
 * Identifies exactly which table/schema/session a `ResultsGrid` instance's
 * rows came from (tasks.md 4.1-4.4). Only ever set for a genuine
 * single-table `BrowseTableRows` result — ad-hoc `RunQuery` output (typed/
 * run SQL, including joins/aggregates with no single backing table) always
 * renders without this prop and therefore stays 100% read-only, matching
 * today's behavior exactly; there is no SQL-text parsing anywhere to guess
 * at editability from an arbitrary query's shape.
 */
export interface EditableGridContext {
    sessionID: string
    schema: string
    table: string
    columns: dbengine.ColumnInfo[]
}

interface ResultsGridProps {
    result: dbengine.QueryResult
    editable?: EditableGridContext
}

interface PendingRow {
    id: string
    values: Record<string, unknown>
    touched: Set<string>
    error: string | null
    saving: boolean
}

let pendingRowSequence = 0

function nextPendingRowId(): string {
    pendingRowSequence += 1
    return `pending-row-${pendingRowSequence}`
}

function cellErrorKey(rowIndex: number, colIndex: number): string {
    return `${rowIndex}:${colIndex}`
}

/**
 * Grid for one `Engine.Query`/`BrowseTableRows` result: column headers with
 * type metadata, NULL-aware cell rendering, and client-side pagination over
 * the already-fully-fetched `result.Rows` (see resultsGridHelpers.ts for the
 * pagination scope note). When `editable` is supplied, adds in-place cell
 * editing, row insert, and row delete (tasks.md 4.1-4.4) bound to that exact
 * table/schema/session; a table with no primary key column renders a
 * visible read-only banner instead of silently disabling interaction.
 */
function ResultsGrid({result, editable}: ResultsGridProps) {
    const [page, setPage] = useState(1)
    const [rows, setRows] = useState<unknown[][]>(result.Rows ?? [])
    const [editingCell, setEditingCell] = useState<{rowIndex: number; colIndex: number} | null>(null)
    const [editingValue, setEditingValue] = useState('')
    const [cellErrors, setCellErrors] = useState<Map<string, string>>(new Map())
    const [rowErrors, setRowErrors] = useState<Map<number, string>>(new Map())
    const [selectedRows, setSelectedRows] = useState<Set<number>>(new Set())
    const [pendingRows, setPendingRows] = useState<PendingRow[]>([])
    const [deleting, setDeleting] = useState(false)
    const [deleteError, setDeleteError] = useState<string | null>(null)
    const [forcedReadOnly, setForcedReadOnly] = useState(false)

    useEffect(() => {
        setPage(1)
        setRows(result.Rows ?? [])
        setEditingCell(null)
        setEditingValue('')
        setCellErrors(new Map())
        setRowErrors(new Map())
        setSelectedRows(new Set())
        setPendingRows([])
        setDeleteError(null)
        setForcedReadOnly(false)
    }, [result])

    const columns = result.Columns ?? []
    const columnNames = useMemo(() => columns.map((column) => column.Name), [columns])
    const editableColumns = editable?.columns ?? []
    const tableIsEditable = editable !== undefined && isTableEditable(editableColumns) && !forcedReadOnly

    const {pageRows, totalRows, currentPage, pageCount, startIndex, endIndex} = paginateRows(
        rows,
        page,
        RESULTS_PAGE_SIZE,
    )
    const pageStartOffset = (currentPage - 1) * RESULTS_PAGE_SIZE

    function markForcedReadOnlyIfPkLess(message: string) {
        if (isPkLessError(message)) {
            setForcedReadOnly(true)
        }
    }

    function startEditingCell(rowIndex: number, colIndex: number, currentValue: unknown) {
        if (!tableIsEditable) {
            return
        }
        const pkValues = extractPkValues(editableColumns, columnNames, rows[rowIndex])
        if (!pkValues) {
            return
        }
        setEditingCell({rowIndex, colIndex})
        setEditingValue(currentValue === null || currentValue === undefined ? '' : String(currentValue))
    }

    function cancelCellEdit() {
        setEditingCell(null)
        setEditingValue('')
    }

    async function commitCellEdit() {
        if (!editingCell || !editable) {
            return
        }
        const {rowIndex, colIndex} = editingCell
        const columnName = columnNames[colIndex]
        const column = editableColumns.find((c) => c.Name === columnName)
        const pkValues = extractPkValues(editableColumns, columnNames, rows[rowIndex])
        if (!column || !pkValues) {
            cancelCellEdit()
            return
        }

        const newValue = coerceCellInput(column, editingValue)
        cancelCellEdit()

        try {
            await UpdateTableRow(editable.sessionID, editable.schema, editable.table, pkValues, columnName, newValue)
            setRows((prev) => {
                const next = prev.map((row) => [...row])
                next[rowIndex][colIndex] = newValue
                return next
            })
            setCellErrors((prev) => {
                const next = new Map(prev)
                next.delete(cellErrorKey(rowIndex, colIndex))
                return next
            })
        } catch (err) {
            const message = String(err)
            markForcedReadOnlyIfPkLess(message)
            setCellErrors((prev) => new Map(prev).set(cellErrorKey(rowIndex, colIndex), message))
        }
    }

    function handleAddRow() {
        if (!editable) {
            return
        }
        setPendingRows((prev) => [
            ...prev,
            {
                id: nextPendingRowId(),
                values: buildBlankRowValues(editableColumns),
                touched: new Set(),
                error: null,
                saving: false,
            },
        ])
    }

    function updatePendingCell(rowId: string, column: dbengine.ColumnInfo, raw: string) {
        setPendingRows((prev) =>
            prev.map((pending) => {
                if (pending.id !== rowId) {
                    return pending
                }
                const touched = new Set(pending.touched)
                touched.add(column.Name)
                return {
                    ...pending,
                    values: {...pending.values, [column.Name]: coerceCellInput(column, raw)},
                    touched,
                }
            }),
        )
    }

    function cancelPendingRow(rowId: string) {
        setPendingRows((prev) => prev.filter((pending) => pending.id !== rowId))
    }

    async function savePendingRow(rowId: string) {
        if (!editable) {
            return
        }
        const pending = pendingRows.find((p) => p.id === rowId)
        if (!pending) {
            return
        }
        setPendingRows((prev) => prev.map((p) => (p.id === rowId ? {...p, saving: true, error: null} : p)))

        try {
            const payload = buildInsertPayload(editableColumns, pending.values, pending.touched)
            const inserted = await InsertTableRow(editable.sessionID, editable.schema, editable.table, payload)
            const newRow = columnNames.map((name) => (name in inserted ? inserted[name] : null))
            setRows((prev) => [...prev, newRow])
            setPendingRows((prev) => prev.filter((p) => p.id !== rowId))
        } catch (err) {
            const message = String(err)
            markForcedReadOnlyIfPkLess(message)
            setPendingRows((prev) => prev.map((p) => (p.id === rowId ? {...p, saving: false, error: message} : p)))
        }
    }

    function toggleRowSelected(rowIndex: number) {
        setSelectedRows((prev) => {
            const next = new Set(prev)
            if (next.has(rowIndex)) {
                next.delete(rowIndex)
            } else {
                next.add(rowIndex)
            }
            return next
        })
    }

    async function handleDeleteSelected() {
        if (!editable || selectedRows.size === 0) {
            return
        }
        if (selectedRows.size > 1) {
            const confirmed = window.confirm(`Delete ${selectedRows.size} rows? This cannot be undone.`)
            if (!confirmed) {
                return
            }
        }

        const indexes = Array.from(selectedRows).sort((a, b) => a - b)
        const pkValuesList: Record<string, unknown>[] = []
        const validIndexes: number[] = []
        for (const index of indexes) {
            const pkValues = extractPkValues(editableColumns, columnNames, rows[index])
            if (pkValues) {
                pkValuesList.push(pkValues)
                validIndexes.push(index)
            }
        }
        if (pkValuesList.length === 0) {
            return
        }

        setDeleting(true)
        setDeleteError(null)
        try {
            const results = await DeleteTableRows(editable.sessionID, editable.schema, editable.table, pkValuesList)
            const resultByIndex = new Map<number, dbengine.StatementResult>()
            validIndexes.forEach((rowIndex, i) => resultByIndex.set(rowIndex, results[i]))

            const nextRows: unknown[][] = []
            const nextRowErrors = new Map<number, string>()
            rows.forEach((row, originalIndex) => {
                const rowResult = resultByIndex.get(originalIndex)
                if (rowResult && rowResult.Success) {
                    return
                }
                const newIndex = nextRows.length
                nextRows.push(row)
                if (rowResult && !rowResult.Success) {
                    nextRowErrors.set(newIndex, rowResult.ErrorMessage)
                }
            })
            setRows(nextRows)
            setRowErrors(nextRowErrors)
            setSelectedRows(new Set())
        } catch (err) {
            const message = String(err)
            markForcedReadOnlyIfPkLess(message)
            setDeleteError(message)
        } finally {
            setDeleting(false)
        }
    }

    return (
        <div className="flex flex-col gap-2">
            {editable && !tableIsEditable && (
                <div className="rounded border border-amber-800 bg-amber-950/40 px-3 py-2 text-xs text-amber-300">
                    This table has no primary key — editing, inserting, and deleting rows is disabled. Showing
                    read-only results.
                </div>
            )}

            {tableIsEditable && (
                <div className="flex items-center gap-3">
                    <button
                        type="button"
                        onClick={handleAddRow}
                        className="rounded border border-ink-700 px-2 py-1 text-xs text-ink-200 transition-colors hover:border-brass-500 hover:text-brass-400"
                    >
                        + Add row
                    </button>
                    <button
                        type="button"
                        onClick={() => void handleDeleteSelected()}
                        disabled={selectedRows.size === 0 || deleting}
                        className="rounded border border-red-800 px-2 py-1 text-xs text-red-400 transition-colors hover:border-red-500 hover:text-red-300 disabled:cursor-not-allowed disabled:opacity-40"
                    >
                        {deleting ? 'Deleting…' : `Delete selected (${selectedRows.size})`}
                    </button>
                    {deleteError && <span className="text-xs text-red-400">{deleteError}</span>}
                </div>
            )}

            <div className="overflow-auto rounded border border-ink-800">
                <table className="w-full border-collapse text-left text-xs">
                    <thead>
                        <tr className="bg-ink-900">
                            {tableIsEditable && <th className="border-b border-ink-800 px-2 py-2" />}
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
                        {tableIsEditable &&
                            pendingRows.map((pending) => (
                                <tr key={pending.id} className="bg-brass-900/10">
                                    <td className="border-b border-ink-900 px-2 py-1.5 align-top">
                                        <div className="flex flex-col gap-1">
                                            <button
                                                type="button"
                                                onClick={() => void savePendingRow(pending.id)}
                                                disabled={pending.saving}
                                                className="rounded border border-brass-600 px-2 py-0.5 text-[10px] text-brass-400 transition-colors hover:border-brass-500 disabled:cursor-not-allowed disabled:opacity-40"
                                            >
                                                {pending.saving ? 'Saving…' : 'Save'}
                                            </button>
                                            <button
                                                type="button"
                                                onClick={() => cancelPendingRow(pending.id)}
                                                className="rounded border border-ink-700 px-2 py-0.5 text-[10px] text-ink-300 hover:border-red-500 hover:text-red-300"
                                            >
                                                Cancel
                                            </button>
                                        </div>
                                    </td>
                                    {editableColumns.map((column, colIndex) => (
                                        <td key={colIndex} className="border-b border-ink-900 px-3 py-1.5 align-top">
                                            <input
                                                type="text"
                                                value={
                                                    pending.values[column.Name] === null ||
                                                    pending.values[column.Name] === undefined
                                                        ? ''
                                                        : String(pending.values[column.Name])
                                                }
                                                onChange={(e) => updatePendingCell(pending.id, column, e.target.value)}
                                                placeholder={column.Nullable ? 'NULL' : ''}
                                                className="w-full rounded border border-ink-700 bg-ink-950 px-2 py-1 font-mono text-ink-200 outline-none focus:border-brass-500"
                                            />
                                        </td>
                                    ))}
                                    {pending.error && (
                                        <td className="border-b border-ink-900 px-3 py-1.5 align-top text-red-400">
                                            {pending.error}
                                        </td>
                                    )}
                                </tr>
                            ))}

                        {pageRows.map((row, rowIndexInPage) => {
                            const rowIndex = pageStartOffset + rowIndexInPage
                            const rowError = rowErrors.get(rowIndex)
                            return (
                                <tr key={rowIndex} className="odd:bg-ink-950/40">
                                    {tableIsEditable && (
                                        <td className="border-b border-ink-900 px-2 py-1.5">
                                            <input
                                                type="checkbox"
                                                checked={selectedRows.has(rowIndex)}
                                                onChange={() => toggleRowSelected(rowIndex)}
                                            />
                                        </td>
                                    )}
                                    {row.map((cell, cellIndex) => {
                                        const display = describeCell(cell)
                                        const isEditing =
                                            editingCell?.rowIndex === rowIndex && editingCell?.colIndex === cellIndex
                                        const cellError = cellErrors.get(cellErrorKey(rowIndex, cellIndex))

                                        return (
                                            <td
                                                key={cellIndex}
                                                onClick={() => !isEditing && startEditingCell(rowIndex, cellIndex, cell)}
                                                className="border-b border-ink-900 px-3 py-1.5 font-mono text-ink-200"
                                            >
                                                {isEditing ? (
                                                    <input
                                                        type="text"
                                                        autoFocus
                                                        value={editingValue}
                                                        onChange={(e) => setEditingValue(e.target.value)}
                                                        onBlur={() => void commitCellEdit()}
                                                        onKeyDown={(e) => {
                                                            if (e.key === 'Enter') {
                                                                e.currentTarget.blur()
                                                            } else if (e.key === 'Escape') {
                                                                cancelCellEdit()
                                                            }
                                                        }}
                                                        className="w-full rounded border border-brass-500 bg-ink-950 px-1 py-0.5 font-mono text-ink-100 outline-none"
                                                    />
                                                ) : (
                                                    <>
                                                        {display.isNull ? (
                                                            <span className="italic text-ink-600">NULL</span>
                                                        ) : (
                                                            display.text
                                                        )}
                                                        {cellError && (
                                                            <div className="mt-0.5 text-[10px] text-red-400">
                                                                {cellError}
                                                            </div>
                                                        )}
                                                    </>
                                                )}
                                            </td>
                                        )
                                    })}
                                    {rowError && (
                                        <td className="border-b border-ink-900 px-3 py-1.5 text-[10px] text-red-400">
                                            {rowError}
                                        </td>
                                    )}
                                </tr>
                            )
                        })}
                    </tbody>
                </table>
                {totalRows === 0 && pendingRows.length === 0 && (
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
