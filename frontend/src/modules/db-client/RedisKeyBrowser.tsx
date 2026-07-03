import {useCallback, useEffect, useRef, useState} from 'react'
import {CloseRedisSession, DeleteRedisKeys, OpenRedisConnection, ScanRedisKeys} from '../../../wailsjs/go/main/App'
import type {main} from '../../../wailsjs/go/models'
import RedisKeyDetail from './RedisKeyDetail'
import {INITIAL_CURSOR_PAGE_STATE, applyCursorPage, canLoadMore} from './redisKeyHelpers'

interface RedisKeyBrowserProps {
    fields: main.ConnectionFormFields
}

const SCAN_COUNT_HINT = 100

/**
 * The Redis-side tab content (tasks.md 6.2-6.4), rendered by `DbClientView`
 * in place of `QueryEditor`/`MongoDocumentView` for a Redis-connected tab.
 *
 * Opens and closes its own Redis session entirely within its own
 * mount/unmount lifecycle, for the exact React 18 StrictMode double-invoke
 * reason `MongoDocumentView`'s own doc comment documents: a session handed in
 * from outside gets closed by the first synthetic unmount before the "real"
 * mount ever uses it. Opening the session inside this component's own effect
 * avoids that.
 *
 * The pattern-driven key scan (spec.md §4.5's "pattern-based key filtering")
 * uses real cursor pagination via `ScanRedisKeys`' `ScanKeysResult` — "Load
 * more" passes the last-returned `NextCursor` back in, never re-fetching from
 * scratch, and stops being offered once a `NextCursor` of `0` signals the
 * scan is complete (see `redisKeyHelpers.canLoadMore`).
 */
function RedisKeyBrowser({fields}: RedisKeyBrowserProps) {
    const [sessionID, setSessionID] = useState<string | null>(null)
    const [connectError, setConnectError] = useState<string | null>(null)
    const sessionIdRef = useRef<string | null>(null)

    const [pattern, setPattern] = useState('*')
    const [scanState, setScanState] = useState(INITIAL_CURSOR_PAGE_STATE)
    const [scanning, setScanning] = useState(false)
    const [scanError, setScanError] = useState<string | null>(null)

    const [selectedKeys, setSelectedKeys] = useState<Set<string>>(new Set())
    const [deleteError, setDeleteError] = useState<string | null>(null)
    const [deleting, setDeleting] = useState(false)

    const [openKey, setOpenKey] = useState<string | null>(null)

    useEffect(() => {
        let cancelled = false
        OpenRedisConnection(fields)
            .then((id) => {
                if (cancelled) {
                    void CloseRedisSession(id)
                    return
                }
                sessionIdRef.current = id
                setSessionID(id)
                setConnectError(null)
            })
            .catch((err) => {
                if (!cancelled) {
                    setConnectError(String(err))
                }
            })
        return () => {
            cancelled = true
            const id = sessionIdRef.current
            if (id) {
                sessionIdRef.current = null
                void CloseRedisSession(id)
            }
        }
    }, [fields])

    const runScan = useCallback(
        async (mode: 'replace' | 'append') => {
            if (!sessionID) {
                return
            }
            setScanning(true)
            setScanError(null)
            try {
                const cursor = mode === 'replace' ? 0 : scanState.cursor
                const result = await ScanRedisKeys(sessionID, pattern, cursor, SCAN_COUNT_HINT)
                setScanState((prev) =>
                    applyCursorPage(mode === 'replace' ? INITIAL_CURSOR_PAGE_STATE : prev, {items: result.Keys, nextCursor: result.NextCursor}, mode),
                )
                if (mode === 'replace') {
                    setSelectedKeys(new Set())
                    setOpenKey(null)
                }
            } catch (err) {
                setScanError(String(err))
            } finally {
                setScanning(false)
            }
        },
        [pattern, scanState.cursor, sessionID],
    )

    useEffect(() => {
        if (sessionID) {
            void runScan('replace')
        }
    }, [sessionID])

    const toggleKeySelected = useCallback((key: string) => {
        setSelectedKeys((prev) => {
            const next = new Set(prev)
            if (next.has(key)) {
                next.delete(key)
            } else {
                next.add(key)
            }
            return next
        })
    }, [])

    const handleDeleteSelected = useCallback(async () => {
        if (!sessionID || selectedKeys.size === 0) {
            return
        }
        const keys = Array.from(selectedKeys)
        if (!window.confirm(`Delete ${keys.length} key${keys.length === 1 ? '' : 's'}? This cannot be undone.`)) {
            return
        }
        setDeleting(true)
        setDeleteError(null)
        try {
            await DeleteRedisKeys(sessionID, keys)
            setScanState((prev) => ({...prev, items: prev.items.filter((k) => !selectedKeys.has(k))}))
            if (openKey && selectedKeys.has(openKey)) {
                setOpenKey(null)
            }
            setSelectedKeys(new Set())
        } catch (err) {
            setDeleteError(String(err))
        } finally {
            setDeleting(false)
        }
    }, [openKey, selectedKeys, sessionID])

    const handleKeyDeleted = useCallback(
        (key: string) => {
            setScanState((prev) => ({...prev, items: prev.items.filter((k) => k !== key)}))
            setSelectedKeys((prev) => {
                const next = new Set(prev)
                next.delete(key)
                return next
            })
            if (openKey === key) {
                setOpenKey(null)
            }
        },
        [openKey],
    )

    const handleKeyRenamed = useCallback((oldKey: string, newKey: string) => {
        setScanState((prev) => ({...prev, items: prev.items.map((k) => (k === oldKey ? newKey : k))}))
        setOpenKey(newKey)
    }, [])

    if (connectError) {
        return (
            <div className="flex flex-col gap-2 rounded border border-red-800 bg-ink-900/40 p-4">
                <h2 className="text-xs uppercase tracking-widest text-ink-400">Redis key browser</h2>
                <p className="text-sm text-red-400">{connectError}</p>
            </div>
        )
    }

    if (!sessionID) {
        return (
            <div className="flex flex-col gap-2 rounded border border-ink-800 bg-ink-900/40 p-4">
                <h2 className="text-xs uppercase tracking-widest text-ink-400">Redis key browser</h2>
                <p className="text-sm text-ink-500">Connecting to Redis…</p>
            </div>
        )
    }

    return (
        <div className="flex flex-col gap-3 rounded border border-ink-800 bg-ink-900/40 p-4">
            <div className="flex items-center justify-between">
                <h2 className="text-xs uppercase tracking-widest text-ink-400">Redis key browser</h2>
                <span className="font-mono text-xs text-ink-500">{fields.Engine}</span>
            </div>

            <div className="flex flex-wrap items-center gap-2">
                <input
                    type="text"
                    value={pattern}
                    onChange={(e) => setPattern(e.target.value)}
                    onKeyDown={(e) => {
                        if (e.key === 'Enter') {
                            void runScan('replace')
                        }
                    }}
                    placeholder="session:*"
                    className="w-64 rounded border border-ink-700 bg-ink-950 px-3 py-1.5 font-mono text-xs text-ink-100 outline-none focus:border-brass-500"
                />
                <button
                    type="button"
                    onClick={() => void runScan('replace')}
                    disabled={scanning}
                    className="rounded border border-ink-700 px-3 py-1.5 text-xs text-ink-200 transition-colors hover:border-brass-500 hover:text-brass-400 disabled:cursor-not-allowed disabled:opacity-50"
                >
                    {scanning ? 'Scanning…' : 'Scan'}
                </button>
                {selectedKeys.size > 0 && (
                    <button
                        type="button"
                        onClick={() => void handleDeleteSelected()}
                        disabled={deleting}
                        className="rounded border border-red-800 px-3 py-1.5 text-xs text-red-400 transition-colors hover:border-red-500 hover:text-red-300 disabled:cursor-not-allowed disabled:opacity-50"
                    >
                        {deleting ? 'Deleting…' : `Delete ${selectedKeys.size} selected`}
                    </button>
                )}
            </div>
            {scanError && <p className="text-xs text-red-400">{scanError}</p>}
            {deleteError && <p className="text-xs text-red-400">{deleteError}</p>}

            <div className="flex flex-col gap-4 sm:flex-row">
                <div className="flex flex-col gap-1 sm:w-72 sm:shrink-0">
                    {scanState.hasScanned && scanState.items.length === 0 && (
                        <p className="text-xs text-ink-500">No keys match this pattern.</p>
                    )}
                    <div className="flex max-h-96 flex-col gap-1 overflow-y-auto">
                        {scanState.items.map((key) => (
                            <div
                                key={key}
                                className={`flex items-center gap-2 rounded border px-2 py-1 text-xs ${
                                    openKey === key ? 'border-brass-500 bg-ink-900' : 'border-ink-800 bg-ink-950/40 hover:border-ink-600'
                                }`}
                            >
                                <input
                                    type="checkbox"
                                    checked={selectedKeys.has(key)}
                                    onChange={() => toggleKeySelected(key)}
                                    aria-label={`Select ${key}`}
                                />
                                <button
                                    type="button"
                                    onClick={() => setOpenKey(key)}
                                    className="flex-1 truncate text-left font-mono text-ink-100 hover:text-brass-400"
                                >
                                    {key}
                                </button>
                            </div>
                        ))}
                    </div>
                    <button
                        type="button"
                        onClick={() => void runScan('append')}
                        disabled={!canLoadMore(scanState) || scanning}
                        className="mt-1 w-fit rounded border border-ink-700 px-3 py-1 text-xs text-ink-300 hover:border-brass-500 hover:text-brass-500 disabled:cursor-not-allowed disabled:opacity-40"
                    >
                        {scanning ? 'Loading…' : 'Load more'}
                    </button>
                </div>

                <div className="flex-1">
                    {openKey ? (
                        <RedisKeyDetail sessionID={sessionID} redisKey={openKey} onDeleted={handleKeyDeleted} onRenamed={handleKeyRenamed} />
                    ) : (
                        <p className="text-sm text-ink-500">Select a key to view and edit its value.</p>
                    )}
                </div>
            </div>
        </div>
    )
}

export default RedisKeyBrowser
