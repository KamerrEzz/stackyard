import {useCallback, useEffect, useState} from 'react'
import type {KeyboardEvent} from 'react'
import {DeleteQueryHistoryEntry, ListQueryHistory} from '../../../wailsjs/go/main/App'
import type {storage} from '../../../wailsjs/go/models'
import {truncateQueryText} from './queryHistoryHelpers'

interface QueryHistoryPanelProps {
    savedConnections: storage.Connection[]
    onReplay: (entry: storage.QueryHistoryEntry) => void
}

function connectionLabel(conn: storage.Connection): string {
    return `${conn.Name} (${conn.Engine}@${conn.Host}:${conn.Port})`
}

/**
 * Filterable/searchable query history panel (tasks.md 4.5, spec.md §4.10):
 * lists every logged execution most-recent-first, filterable by connection
 * and searchable by query text (both filtered server-side via
 * ListQueryHistory, not fetched-all-then-filtered-in-React — see
 * internal/storage/query_history.go's own doc comment), with a per-row
 * "Replay" action and a per-row "Delete" action. Only executions run
 * through a saved connection ever appear here — see app.go's
 * recordQueryHistory doc comment for why an ad-hoc/never-saved session's
 * queries are never logged in the first place.
 */
function QueryHistoryPanel({savedConnections, onReplay}: QueryHistoryPanelProps) {
    const [entries, setEntries] = useState<storage.QueryHistoryEntry[]>([])
    const [error, setError] = useState<string | null>(null)
    const [loading, setLoading] = useState(false)

    const [connectionFilter, setConnectionFilter] = useState('')
    const [searchInput, setSearchInput] = useState('')
    const [searchText, setSearchText] = useState('')
    const [expandedId, setExpandedId] = useState<number | null>(null)

    const refresh = useCallback(async () => {
        setLoading(true)
        try {
            const result = await ListQueryHistory({
                ConnectionID: connectionFilter ? Number(connectionFilter) : 0,
                SearchText: searchText,
            })
            setError(null)
            setEntries(result)
        } catch (err) {
            setError(String(err))
        } finally {
            setLoading(false)
        }
    }, [connectionFilter, searchText])

    useEffect(() => {
        void refresh()
    }, [refresh])

    const commitSearch = useCallback(() => {
        setSearchText(searchInput.trim())
    }, [searchInput])

    const handleSearchKeyDown = useCallback(
        (e: KeyboardEvent<HTMLInputElement>) => {
            if (e.key === 'Enter') {
                commitSearch()
            }
        },
        [commitSearch],
    )

    const handleDelete = useCallback(
        async (id: number) => {
            if (!window.confirm('Delete this history entry? This cannot be undone.')) {
                return
            }
            try {
                await DeleteQueryHistoryEntry(id)
                await refresh()
            } catch (err) {
                setError(String(err))
            }
        },
        [refresh],
    )

    return (
        <div className="flex flex-col gap-3 rounded border border-ink-800 bg-ink-900/40 p-4">
            <div className="flex items-center justify-between">
                <h2 className="text-xs uppercase tracking-widest text-ink-400">Query history</h2>
                <button
                    type="button"
                    onClick={() => void refresh()}
                    className="rounded border border-ink-700 px-2 py-1 text-xs text-ink-200 hover:border-brass-500 hover:text-brass-400"
                >
                    {loading ? 'Refreshing…' : 'Refresh'}
                </button>
            </div>

            <div className="flex flex-wrap items-center gap-3">
                <input
                    type="text"
                    value={searchInput}
                    onChange={(e) => setSearchInput(e.target.value)}
                    onBlur={commitSearch}
                    onKeyDown={handleSearchKeyDown}
                    placeholder="Search query text…"
                    className="min-w-[200px] flex-1 rounded border border-ink-700 bg-ink-950 px-3 py-2 font-mono text-sm text-ink-100 outline-none focus:border-brass-500"
                />
                <select
                    value={connectionFilter}
                    onChange={(e) => setConnectionFilter(e.target.value)}
                    className="rounded border border-ink-700 bg-ink-950 px-3 py-2 text-sm text-ink-100 outline-none focus:border-brass-500"
                >
                    <option value="">All connections</option>
                    {savedConnections.map((conn) => (
                        <option key={conn.ID} value={conn.ID}>
                            {connectionLabel(conn)}
                        </option>
                    ))}
                </select>
            </div>

            {error && <p className="text-xs text-red-400">{error}</p>}
            {!error && !loading && entries.length === 0 && (
                <p className="text-sm text-ink-500">No query history yet.</p>
            )}

            <div className="flex flex-col gap-2">
                {entries.map((entry) => {
                    const isExpanded = expandedId === entry.ID
                    return (
                        <div
                            key={entry.ID}
                            className="flex flex-col gap-1 rounded border border-ink-800 bg-ink-950/60 px-3 py-2"
                        >
                            <div className="flex flex-wrap items-center justify-between gap-3">
                                <div className="flex flex-wrap items-center gap-2 text-xs text-ink-400">
                                    <span>{entry.ExecutedAt}</span>
                                    <span className={entry.Success ? 'text-emerald-400' : 'text-red-400'}>
                                        {entry.Success ? 'Success' : 'Failed'}
                                    </span>
                                    <span>{entry.DurationMs}ms</span>
                                    <span>{entry.RowsAffected} row(s)</span>
                                </div>
                                <div className="flex items-center gap-2">
                                    <button
                                        type="button"
                                        onClick={() => onReplay(entry)}
                                        className="rounded border border-ink-700 px-2 py-1 text-xs text-ink-200 hover:border-brass-500 hover:text-brass-400"
                                    >
                                        Replay
                                    </button>
                                    <button
                                        type="button"
                                        onClick={() => void handleDelete(entry.ID)}
                                        className="rounded border border-red-800 px-2 py-1 text-xs text-red-400 hover:border-red-500 hover:text-red-300"
                                    >
                                        Delete
                                    </button>
                                </div>
                            </div>
                            <button
                                type="button"
                                onClick={() => setExpandedId(isExpanded ? null : entry.ID)}
                                title={entry.QueryText}
                                className="cursor-pointer text-left font-mono text-xs text-ink-300 hover:text-ink-100"
                            >
                                {isExpanded ? entry.QueryText : truncateQueryText(entry.QueryText)}
                            </button>
                            {entry.ErrorMessage && <p className="text-xs text-red-400">{entry.ErrorMessage}</p>}
                        </div>
                    )
                })}
            </div>
        </div>
    )
}

export default QueryHistoryPanel
