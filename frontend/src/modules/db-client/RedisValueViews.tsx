import {useCallback, useEffect, useState} from 'react'
import {
    AddRedisSetMembers,
    AddRedisSortedSetMembers,
    GetRedisHash,
    GetRedisList,
    GetRedisSet,
    GetRedisSortedSet,
    GetRedisString,
    PushRedisList,
    RemoveRedisSetMembers,
    RemoveRedisSortedSetMembers,
    SetRedisHash,
    SetRedisListElement,
    SetRedisString,
} from '../../../wailsjs/go/main/App'
import type {redis} from '../../../wailsjs/go/models'
import {describeServerPage} from './resultsGridHelpers'
import {INITIAL_CURSOR_PAGE_STATE, applyCursorPage, canLoadMore, parseLineValues, parseScoreInput, validateHashJSON} from './redisKeyHelpers'

interface RedisValueViewProps {
    sessionID: string
    redisKey: string
}

const LIST_PAGE_SIZE = 50
const SORTED_SET_PAGE_SIZE = 50
const SET_SCAN_COUNT_HINT = 100

/**
 * String-value editor (tasks.md 6.2, spec.md §4.5): a plain textarea over the
 * whole value, matching `MongoJSONEditor`'s own "a form field that appears
 * once per view isn't worth Monaco" precedent — a Redis string has no
 * internal structure to justify a syntax-aware editor either.
 */
export function RedisStringValue({sessionID, redisKey}: RedisValueViewProps) {
    const [value, setValue] = useState('')
    const [loadError, setLoadError] = useState<string | null>(null)
    const [saveError, setSaveError] = useState<string | null>(null)
    const [saving, setSaving] = useState(false)
    const [loading, setLoading] = useState(true)

    useEffect(() => {
        let cancelled = false
        setLoading(true)
        GetRedisString(sessionID, redisKey)
            .then((v) => {
                if (!cancelled) {
                    setValue(v)
                    setLoadError(null)
                }
            })
            .catch((err) => {
                if (!cancelled) {
                    setLoadError(String(err))
                }
            })
            .finally(() => {
                if (!cancelled) {
                    setLoading(false)
                }
            })
        return () => {
            cancelled = true
        }
    }, [redisKey, sessionID])

    const handleSave = useCallback(async () => {
        setSaving(true)
        setSaveError(null)
        try {
            await SetRedisString(sessionID, redisKey, value)
        } catch (err) {
            setSaveError(String(err))
        } finally {
            setSaving(false)
        }
    }, [redisKey, sessionID, value])

    if (loading) {
        return <p className="text-xs text-ink-500">Loading value…</p>
    }

    return (
        <div className="flex flex-col gap-2">
            {loadError && <p className="text-xs text-red-400">{loadError}</p>}
            <textarea
                value={value}
                onChange={(e) => setValue(e.target.value)}
                spellCheck={false}
                rows={8}
                className="w-full resize-y rounded border border-ink-700 bg-ink-950 px-3 py-2 font-mono text-xs text-ink-100 outline-none focus:border-brass-500"
            />
            {saveError && <p className="text-xs text-red-400">{saveError}</p>}
            <button
                type="button"
                onClick={() => void handleSave()}
                disabled={saving}
                className="w-fit rounded bg-brass-600 px-3 py-1.5 text-xs font-medium text-ink-950 transition-colors hover:bg-brass-500 disabled:cursor-not-allowed disabled:opacity-50"
            >
                {saving ? 'Saving…' : 'Save'}
            </button>
        </div>
    )
}

/**
 * Hash-value editor (tasks.md 6.2): field/value pairs edited as one bulk JSON
 * object, the same "whole-document JSON edit" precedent `MongoJSONEditor`
 * already established for Mongo — chosen over a per-field inline editor
 * because `SetRedisHash` is itself a bulk `HSET` call (see `redis_session.go`'s
 * doc comment), so a bulk editor maps onto the bound method one-to-one
 * instead of the frontend synthesizing per-field calls the backend doesn't
 * expose. The one-way limitation this inherits from `HSET` — removing a
 * field from the JSON text and saving does NOT delete it in Redis, since
 * `HSET` only adds/overwrites fields, never removes ones absent from the
 * call — is surfaced directly in the UI rather than silently causing a
 * "vanishes then reappears on next load" surprise.
 */
export function RedisHashValue({sessionID, redisKey}: RedisValueViewProps) {
    const [text, setText] = useState('{}')
    const [loadError, setLoadError] = useState<string | null>(null)
    const [saveError, setSaveError] = useState<string | null>(null)
    const [saving, setSaving] = useState(false)
    const [loading, setLoading] = useState(true)

    useEffect(() => {
        let cancelled = false
        setLoading(true)
        GetRedisHash(sessionID, redisKey)
            .then((fields) => {
                if (!cancelled) {
                    setText(JSON.stringify(fields, null, 2))
                    setLoadError(null)
                }
            })
            .catch((err) => {
                if (!cancelled) {
                    setLoadError(String(err))
                }
            })
            .finally(() => {
                if (!cancelled) {
                    setLoading(false)
                }
            })
        return () => {
            cancelled = true
        }
    }, [redisKey, sessionID])

    const validation = validateHashJSON(text)

    const handleSave = useCallback(async () => {
        const result = validateHashJSON(text)
        if (!result.ok) {
            setSaveError(result.error)
            return
        }
        setSaving(true)
        setSaveError(null)
        try {
            await SetRedisHash(sessionID, redisKey, result.value)
        } catch (err) {
            setSaveError(String(err))
        } finally {
            setSaving(false)
        }
    }, [redisKey, sessionID, text])

    if (loading) {
        return <p className="text-xs text-ink-500">Loading fields…</p>
    }

    return (
        <div className="flex flex-col gap-2">
            {loadError && <p className="text-xs text-red-400">{loadError}</p>}
            <p className="text-[10px] text-ink-500">
                Bulk field/value edit. Removing a field here does not delete it in Redis — only added/changed fields
                are written. Delete the key to clear it entirely.
            </p>
            <textarea
                value={text}
                onChange={(e) => setText(e.target.value)}
                spellCheck={false}
                rows={10}
                className="w-full resize-y rounded border border-ink-700 bg-ink-950 px-3 py-2 font-mono text-xs text-ink-100 outline-none focus:border-brass-500"
            />
            {!validation.ok && <p className="text-xs text-red-400">{validation.error}</p>}
            {saveError && <p className="text-xs text-red-400">{saveError}</p>}
            <button
                type="button"
                onClick={() => void handleSave()}
                disabled={!validation.ok || saving}
                className="w-fit rounded bg-brass-600 px-3 py-1.5 text-xs font-medium text-ink-950 transition-colors hover:bg-brass-500 disabled:cursor-not-allowed disabled:opacity-50"
            >
                {saving ? 'Saving…' : 'Save'}
            </button>
        </div>
    )
}

/**
 * List-value viewer/editor (tasks.md 6.2): a real `start`/`stop`-windowed
 * page (via `GetRedisList`), never a full-list fetch, with per-index
 * in-place edit (`SetRedisListElement`) and a bulk append textarea (one value
 * per line, via `PushRedisList` in a single call).
 */
export function RedisListValue({sessionID, redisKey}: RedisValueViewProps) {
    const [start, setStart] = useState(0)
    const [values, setValues] = useState<string[]>([])
    const [fetchedCount, setFetchedCount] = useState(0)
    const [loadError, setLoadError] = useState<string | null>(null)
    const [loading, setLoading] = useState(true)

    const [editingIndex, setEditingIndex] = useState<number | null>(null)
    const [editingValue, setEditingValue] = useState('')
    const [editError, setEditError] = useState<string | null>(null)
    const [editSaving, setEditSaving] = useState(false)

    const [appendText, setAppendText] = useState('')
    const [appendError, setAppendError] = useState<string | null>(null)
    const [appending, setAppending] = useState(false)

    const loadPage = useCallback(
        async (pageStart: number) => {
            setLoading(true)
            setLoadError(null)
            try {
                const page = await GetRedisList(sessionID, redisKey, pageStart, pageStart + LIST_PAGE_SIZE - 1)
                setValues(page)
                setFetchedCount(page.length)
                setStart(pageStart)
                setEditingIndex(null)
            } catch (err) {
                setLoadError(String(err))
            } finally {
                setLoading(false)
            }
        },
        [redisKey, sessionID],
    )

    useEffect(() => {
        void loadPage(0)
    }, [loadPage])

    const pageInfo = describeServerPage(start, LIST_PAGE_SIZE, fetchedCount, values.length)

    const handleStartEdit = useCallback((index: number, value: string) => {
        setEditingIndex(index)
        setEditingValue(value)
        setEditError(null)
    }, [])

    const handleSaveEdit = useCallback(async () => {
        if (editingIndex === null) {
            return
        }
        setEditSaving(true)
        setEditError(null)
        try {
            await SetRedisListElement(sessionID, redisKey, start + editingIndex, editingValue)
            setValues((prev) => prev.map((v, i) => (i === editingIndex ? editingValue : v)))
            setEditingIndex(null)
        } catch (err) {
            setEditError(String(err))
        } finally {
            setEditSaving(false)
        }
    }, [editingIndex, editingValue, redisKey, sessionID, start])

    const handleAppend = useCallback(async () => {
        const newValues = parseLineValues(appendText)
        if (newValues.length === 0) {
            return
        }
        setAppending(true)
        setAppendError(null)
        try {
            await PushRedisList(sessionID, redisKey, newValues)
            setAppendText('')
            await loadPage(start)
        } catch (err) {
            setAppendError(String(err))
        } finally {
            setAppending(false)
        }
    }, [appendText, loadPage, redisKey, sessionID, start])

    return (
        <div className="flex flex-col gap-2">
            {loading && <p className="text-xs text-ink-500">Loading list…</p>}
            {loadError && <p className="text-xs text-red-400">{loadError}</p>}
            {!loading && !loadError && values.length === 0 && <p className="text-xs text-ink-500">This list has no elements on this page.</p>}

            {values.map((value, i) => {
                const absoluteIndex = start + i
                const isEditing = editingIndex === i
                return (
                    <div key={absoluteIndex} className="flex items-center gap-2 text-xs">
                        <span className="w-10 shrink-0 text-right font-mono text-ink-500">{absoluteIndex}</span>
                        {isEditing ? (
                            <>
                                <input
                                    type="text"
                                    value={editingValue}
                                    onChange={(e) => setEditingValue(e.target.value)}
                                    className="flex-1 rounded border border-ink-700 bg-ink-950 px-2 py-1 font-mono text-ink-100 outline-none focus:border-brass-500"
                                />
                                <button type="button" onClick={() => void handleSaveEdit()} disabled={editSaving} className="rounded border border-ink-700 px-2 py-1 text-ink-200 hover:border-brass-500 hover:text-brass-400 disabled:opacity-50">
                                    Save
                                </button>
                                <button type="button" onClick={() => setEditingIndex(null)} disabled={editSaving} className="rounded border border-ink-700 px-2 py-1 text-ink-300 hover:border-red-500 hover:text-red-300">
                                    Cancel
                                </button>
                            </>
                        ) : (
                            <>
                                <span className="flex-1 truncate font-mono text-ink-100">{value}</span>
                                <button type="button" onClick={() => handleStartEdit(i, value)} className="rounded border border-ink-700 px-2 py-1 text-ink-200 hover:border-brass-500 hover:text-brass-400">
                                    Edit
                                </button>
                            </>
                        )}
                    </div>
                )
            })}
            {editError && <p className="text-xs text-red-400">{editError}</p>}

            {values.length > 0 && (
                <div className="flex items-center justify-between text-xs text-ink-400">
                    <span>
                        Showing {pageInfo.startIndex}-{pageInfo.endIndex}
                    </span>
                    <div className="flex items-center gap-2">
                        <button
                            type="button"
                            onClick={() => void loadPage(Math.max(0, start - LIST_PAGE_SIZE))}
                            disabled={!pageInfo.hasPrevPage || loading}
                            className="rounded border border-ink-700 px-2 py-1 text-ink-300 hover:border-brass-500 hover:text-brass-500 disabled:cursor-not-allowed disabled:opacity-40"
                        >
                            Prev
                        </button>
                        <button
                            type="button"
                            onClick={() => void loadPage(start + LIST_PAGE_SIZE)}
                            disabled={!pageInfo.hasNextPage || loading}
                            className="rounded border border-ink-700 px-2 py-1 text-ink-300 hover:border-brass-500 hover:text-brass-500 disabled:cursor-not-allowed disabled:opacity-40"
                        >
                            Next
                        </button>
                    </div>
                </div>
            )}

            <div className="flex flex-col gap-1 border-t border-ink-800 pt-2">
                <span className="text-[10px] uppercase tracking-widest text-ink-500">Append (one value per line)</span>
                <textarea
                    value={appendText}
                    onChange={(e) => setAppendText(e.target.value)}
                    spellCheck={false}
                    rows={3}
                    className="w-full resize-y rounded border border-ink-700 bg-ink-950 px-3 py-2 font-mono text-xs text-ink-100 outline-none focus:border-brass-500"
                />
                {appendError && <p className="text-xs text-red-400">{appendError}</p>}
                <button
                    type="button"
                    onClick={() => void handleAppend()}
                    disabled={appending || parseLineValues(appendText).length === 0}
                    className="w-fit rounded border border-ink-700 px-3 py-1.5 text-xs text-ink-200 transition-colors hover:border-brass-500 hover:text-brass-400 disabled:cursor-not-allowed disabled:opacity-50"
                >
                    {appending ? 'Appending…' : 'Append'}
                </button>
            </div>
        </div>
    )
}

/**
 * Set-value viewer/editor (tasks.md 6.2): paginated via `GetRedisSet`'s
 * `SSCAN` cursor. Add/remove is a simple one-member-at-a-time interaction
 * (an add input plus a per-row Remove button), deliberately NOT a diff
 * against a client-side-edited full member list: unlike Mongo's whole-
 * document edit (which always holds the complete document), this view only
 * ever holds one page of a potentially much larger set, so diffing "what's
 * shown" against "what the user typed" could silently target members that
 * were never loaded in the first place. `AddRedisSetMembers`/
 * `RemoveRedisSetMembers` are still called with plain one-element arrays,
 * not a hidden single-member bound method — the bulk API is reused, just
 * invoked with a batch size of one from this UI.
 */
export function RedisSetValue({sessionID, redisKey}: RedisValueViewProps) {
    const [state, setState] = useState(INITIAL_CURSOR_PAGE_STATE)
    const [loading, setLoading] = useState(true)
    const [loadError, setLoadError] = useState<string | null>(null)

    const [newMember, setNewMember] = useState('')
    const [addError, setAddError] = useState<string | null>(null)
    const [adding, setAdding] = useState(false)
    const [removingMember, setRemovingMember] = useState<string | null>(null)
    const [removeError, setRemoveError] = useState<string | null>(null)

    const runScan = useCallback(
        async (mode: 'replace' | 'append') => {
            setLoading(true)
            setLoadError(null)
            try {
                const cursor = mode === 'replace' ? 0 : state.cursor
                const page = await GetRedisSet(sessionID, redisKey, cursor, SET_SCAN_COUNT_HINT)
                setState((prev) => applyCursorPage(mode === 'replace' ? INITIAL_CURSOR_PAGE_STATE : prev, {items: page.Members, nextCursor: page.NextCursor}, mode))
            } catch (err) {
                setLoadError(String(err))
            } finally {
                setLoading(false)
            }
        },
        [redisKey, sessionID, state.cursor],
    )

    useEffect(() => {
        void runScan('replace')
    }, [redisKey, sessionID])

    const handleAdd = useCallback(async () => {
        const member = newMember.trim()
        if (!member) {
            return
        }
        setAdding(true)
        setAddError(null)
        try {
            await AddRedisSetMembers(sessionID, redisKey, [member])
            setNewMember('')
            await runScan('replace')
        } catch (err) {
            setAddError(String(err))
        } finally {
            setAdding(false)
        }
    }, [newMember, redisKey, runScan, sessionID])

    const handleRemove = useCallback(
        async (member: string) => {
            setRemovingMember(member)
            setRemoveError(null)
            try {
                await RemoveRedisSetMembers(sessionID, redisKey, [member])
                setState((prev) => ({...prev, items: prev.items.filter((m) => m !== member)}))
            } catch (err) {
                setRemoveError(String(err))
            } finally {
                setRemovingMember(null)
            }
        },
        [redisKey, sessionID],
    )

    return (
        <div className="flex flex-col gap-2">
            {loadError && <p className="text-xs text-red-400">{loadError}</p>}
            {removeError && <p className="text-xs text-red-400">{removeError}</p>}
            {!loading && state.items.length === 0 && <p className="text-xs text-ink-500">This set has no members on this page.</p>}

            {state.items.map((member) => (
                <div key={member} className="flex items-center justify-between gap-2 text-xs">
                    <span className="truncate font-mono text-ink-100">{member}</span>
                    <button
                        type="button"
                        onClick={() => void handleRemove(member)}
                        disabled={removingMember === member}
                        className="rounded border border-red-800 px-2 py-0.5 text-red-400 hover:border-red-500 hover:text-red-300 disabled:opacity-50"
                    >
                        {removingMember === member ? 'Removing…' : 'Remove'}
                    </button>
                </div>
            ))}

            <button
                type="button"
                onClick={() => void runScan('append')}
                disabled={!canLoadMore(state) || loading}
                className="w-fit rounded border border-ink-700 px-3 py-1 text-xs text-ink-300 hover:border-brass-500 hover:text-brass-500 disabled:cursor-not-allowed disabled:opacity-40"
            >
                {loading ? 'Loading…' : 'Load more'}
            </button>

            <div className="flex items-center gap-2 border-t border-ink-800 pt-2">
                <input
                    type="text"
                    value={newMember}
                    onChange={(e) => setNewMember(e.target.value)}
                    placeholder="New member"
                    className="flex-1 rounded border border-ink-700 bg-ink-950 px-2 py-1 font-mono text-xs text-ink-100 outline-none focus:border-brass-500"
                />
                <button
                    type="button"
                    onClick={() => void handleAdd()}
                    disabled={adding || newMember.trim().length === 0}
                    className="rounded border border-ink-700 px-3 py-1 text-xs text-ink-200 hover:border-brass-500 hover:text-brass-400 disabled:cursor-not-allowed disabled:opacity-50"
                >
                    {adding ? 'Adding…' : 'Add'}
                </button>
            </div>
            {addError && <p className="text-xs text-red-400">{addError}</p>}
        </div>
    )
}

/**
 * Sorted-set viewer/editor (tasks.md 6.2): `start`/`stop`-windowed via
 * `GetRedisSortedSet`, the same pagination shape `RedisListValue` uses.
 * Add/update reuses `ZADD`'s own "adding an existing member updates its
 * score" semantics (see `redis.go`'s `AddToSortedSet` doc comment) — the same
 * one-member-at-a-time input as the set editor, for the same partial-page
 * diffing-is-unsafe reasoning `RedisSetValue`'s doc comment gives.
 */
export function RedisSortedSetValue({sessionID, redisKey}: RedisValueViewProps) {
    const [start, setStart] = useState(0)
    const [members, setMembers] = useState<redis.SortedSetMember[]>([])
    const [fetchedCount, setFetchedCount] = useState(0)
    const [loading, setLoading] = useState(true)
    const [loadError, setLoadError] = useState<string | null>(null)

    const [newMember, setNewMember] = useState('')
    const [newScore, setNewScore] = useState('')
    const [addError, setAddError] = useState<string | null>(null)
    const [adding, setAdding] = useState(false)
    const [removingMember, setRemovingMember] = useState<string | null>(null)
    const [removeError, setRemoveError] = useState<string | null>(null)

    const loadPage = useCallback(
        async (pageStart: number) => {
            setLoading(true)
            setLoadError(null)
            try {
                const page = await GetRedisSortedSet(sessionID, redisKey, pageStart, pageStart + SORTED_SET_PAGE_SIZE - 1)
                setMembers(page)
                setFetchedCount(page.length)
                setStart(pageStart)
            } catch (err) {
                setLoadError(String(err))
            } finally {
                setLoading(false)
            }
        },
        [redisKey, sessionID],
    )

    useEffect(() => {
        void loadPage(0)
    }, [loadPage])

    const pageInfo = describeServerPage(start, SORTED_SET_PAGE_SIZE, fetchedCount, members.length)

    const handleAdd = useCallback(async () => {
        const member = newMember.trim()
        if (!member) {
            return
        }
        const scoreResult = parseScoreInput(newScore)
        if (!scoreResult.ok) {
            setAddError(scoreResult.error)
            return
        }
        setAdding(true)
        setAddError(null)
        try {
            await AddRedisSortedSetMembers(sessionID, redisKey, [{Member: member, Score: scoreResult.score}])
            setNewMember('')
            setNewScore('')
            await loadPage(start)
        } catch (err) {
            setAddError(String(err))
        } finally {
            setAdding(false)
        }
    }, [loadPage, newMember, newScore, redisKey, sessionID, start])

    const handleRemove = useCallback(
        async (member: string) => {
            setRemovingMember(member)
            setRemoveError(null)
            try {
                await RemoveRedisSortedSetMembers(sessionID, redisKey, [member])
                setMembers((prev) => prev.filter((m) => m.Member !== member))
            } catch (err) {
                setRemoveError(String(err))
            } finally {
                setRemovingMember(null)
            }
        },
        [redisKey, sessionID],
    )

    return (
        <div className="flex flex-col gap-2">
            {loadError && <p className="text-xs text-red-400">{loadError}</p>}
            {removeError && <p className="text-xs text-red-400">{removeError}</p>}
            {!loading && members.length === 0 && <p className="text-xs text-ink-500">This sorted set has no members on this page.</p>}

            {members.map((m) => (
                <div key={m.Member} className="flex items-center justify-between gap-2 text-xs">
                    <span className="truncate font-mono text-ink-100">{m.Member}</span>
                    <span className="font-mono text-ink-400">{m.Score}</span>
                    <button
                        type="button"
                        onClick={() => void handleRemove(m.Member)}
                        disabled={removingMember === m.Member}
                        className="rounded border border-red-800 px-2 py-0.5 text-red-400 hover:border-red-500 hover:text-red-300 disabled:opacity-50"
                    >
                        {removingMember === m.Member ? 'Removing…' : 'Remove'}
                    </button>
                </div>
            ))}

            {members.length > 0 && (
                <div className="flex items-center justify-between text-xs text-ink-400">
                    <span>
                        Showing {pageInfo.startIndex}-{pageInfo.endIndex}
                    </span>
                    <div className="flex items-center gap-2">
                        <button
                            type="button"
                            onClick={() => void loadPage(Math.max(0, start - SORTED_SET_PAGE_SIZE))}
                            disabled={!pageInfo.hasPrevPage || loading}
                            className="rounded border border-ink-700 px-2 py-1 text-ink-300 hover:border-brass-500 hover:text-brass-500 disabled:cursor-not-allowed disabled:opacity-40"
                        >
                            Prev
                        </button>
                        <button
                            type="button"
                            onClick={() => void loadPage(start + SORTED_SET_PAGE_SIZE)}
                            disabled={!pageInfo.hasNextPage || loading}
                            className="rounded border border-ink-700 px-2 py-1 text-ink-300 hover:border-brass-500 hover:text-brass-500 disabled:cursor-not-allowed disabled:opacity-40"
                        >
                            Next
                        </button>
                    </div>
                </div>
            )}

            <div className="flex items-center gap-2 border-t border-ink-800 pt-2">
                <input
                    type="text"
                    value={newMember}
                    onChange={(e) => setNewMember(e.target.value)}
                    placeholder="Member"
                    className="flex-1 rounded border border-ink-700 bg-ink-950 px-2 py-1 font-mono text-xs text-ink-100 outline-none focus:border-brass-500"
                />
                <input
                    type="number"
                    value={newScore}
                    onChange={(e) => setNewScore(e.target.value)}
                    placeholder="Score"
                    className="w-24 rounded border border-ink-700 bg-ink-950 px-2 py-1 font-mono text-xs text-ink-100 outline-none focus:border-brass-500"
                />
                <button
                    type="button"
                    onClick={() => void handleAdd()}
                    disabled={adding || newMember.trim().length === 0}
                    className="rounded border border-ink-700 px-3 py-1 text-xs text-ink-200 hover:border-brass-500 hover:text-brass-400 disabled:cursor-not-allowed disabled:opacity-50"
                >
                    {adding ? 'Adding…' : 'Add / Update'}
                </button>
            </div>
            {addError && <p className="text-xs text-red-400">{addError}</p>}
        </div>
    )
}
