import {useCallback, useEffect, useState} from 'react'
import {DeleteRedisKeys, GetRedisKeyType, GetRedisTTL, PersistRedisKey, RenameRedisKey, SetRedisTTL} from '../../../wailsjs/go/main/App'
import {RedisHashValue, RedisListValue, RedisSetValue, RedisSortedSetValue, RedisStringValue} from './RedisValueViews'
import {formatTTL} from './redisKeyHelpers'

interface RedisKeyDetailProps {
    sessionID: string
    redisKey: string
    onDeleted: (key: string) => void
    onRenamed: (oldKey: string, newKey: string) => void
}

/**
 * The per-key panel opened from `RedisKeyBrowser` (tasks.md 6.2-6.4, spec.md
 * §4.5): resolves `redisKey`'s type via `GetRedisKeyType` first, then renders
 * the matching typed viewer from `RedisValueViews`, plus the TTL and
 * rename/delete chrome shared across every type. TTL/rename/delete are kept
 * in this orchestrator file rather than duplicated into each of the five
 * typed views, since none of them differ by value type.
 */
function RedisKeyDetail({sessionID, redisKey, onDeleted, onRenamed}: RedisKeyDetailProps) {
    const [keyType, setKeyType] = useState<string | null>(null)
    const [typeError, setTypeError] = useState<string | null>(null)

    const [ttlNs, setTtlNs] = useState<number | null>(null)
    const [ttlError, setTtlError] = useState<string | null>(null)
    const [ttlInputSeconds, setTtlInputSeconds] = useState('')
    const [ttlSaving, setTtlSaving] = useState(false)

    const [renameValue, setRenameValue] = useState(redisKey)
    const [renameError, setRenameError] = useState<string | null>(null)
    const [renameSaving, setRenameSaving] = useState(false)

    const [deleteError, setDeleteError] = useState<string | null>(null)
    const [deleting, setDeleting] = useState(false)

    const refreshTTL = useCallback(async () => {
        try {
            const ttl = await GetRedisTTL(sessionID, redisKey)
            setTtlNs(ttl as unknown as number)
            setTtlError(null)
        } catch (err) {
            setTtlError(String(err))
        }
    }, [redisKey, sessionID])

    useEffect(() => {
        setRenameValue(redisKey)
        setRenameError(null)
        setDeleteError(null)
        setTypeError(null)
        setKeyType(null)
        setTtlNs(null)
        let cancelled = false
        GetRedisKeyType(sessionID, redisKey)
            .then((t) => {
                if (!cancelled) {
                    setKeyType(t)
                }
            })
            .catch((err) => {
                if (!cancelled) {
                    setTypeError(String(err))
                }
            })
        void refreshTTL()
        return () => {
            cancelled = true
        }
    }, [redisKey, refreshTTL, sessionID])

    const handleSetTTL = useCallback(async () => {
        const seconds = Number(ttlInputSeconds)
        if (ttlInputSeconds.trim() === '' || !Number.isFinite(seconds) || seconds <= 0) {
            setTtlError('Enter a positive number of seconds.')
            return
        }
        setTtlSaving(true)
        setTtlError(null)
        try {
            await SetRedisTTL(sessionID, redisKey, Math.trunc(seconds))
            setTtlInputSeconds('')
            await refreshTTL()
        } catch (err) {
            setTtlError(String(err))
        } finally {
            setTtlSaving(false)
        }
    }, [redisKey, refreshTTL, sessionID, ttlInputSeconds])

    const handlePersist = useCallback(async () => {
        setTtlSaving(true)
        setTtlError(null)
        try {
            await PersistRedisKey(sessionID, redisKey)
            await refreshTTL()
        } catch (err) {
            setTtlError(String(err))
        } finally {
            setTtlSaving(false)
        }
    }, [redisKey, refreshTTL, sessionID])

    const handleRename = useCallback(async () => {
        const newKey = renameValue.trim()
        if (!newKey || newKey === redisKey) {
            return
        }
        setRenameSaving(true)
        setRenameError(null)
        try {
            await RenameRedisKey(sessionID, redisKey, newKey)
            onRenamed(redisKey, newKey)
        } catch (err) {
            setRenameError(String(err))
        } finally {
            setRenameSaving(false)
        }
    }, [onRenamed, redisKey, renameValue, sessionID])

    const handleDelete = useCallback(async () => {
        if (!window.confirm(`Delete key "${redisKey}"? This cannot be undone.`)) {
            return
        }
        setDeleting(true)
        setDeleteError(null)
        try {
            await DeleteRedisKeys(sessionID, [redisKey])
            onDeleted(redisKey)
        } catch (err) {
            setDeleteError(String(err))
        } finally {
            setDeleting(false)
        }
    }, [onDeleted, redisKey, sessionID])

    return (
        <div className="flex flex-col gap-3 rounded border border-ink-800 bg-ink-950/40 p-4">
            <div className="flex items-center justify-between gap-2">
                <span className="truncate font-mono text-sm text-ink-100">{redisKey}</span>
                {keyType && (
                    <span className="shrink-0 rounded border border-ink-700 px-2 py-0.5 text-[10px] uppercase tracking-widest text-ink-400">
                        {keyType}
                    </span>
                )}
            </div>
            {typeError && <p className="text-xs text-red-400">{typeError}</p>}

            <div className="flex flex-wrap items-center gap-2 border-t border-ink-800 pt-2 text-xs">
                <span className="text-ink-400">TTL: {ttlNs === null ? '…' : formatTTL(ttlNs)}</span>
                <input
                    type="number"
                    min={1}
                    value={ttlInputSeconds}
                    onChange={(e) => setTtlInputSeconds(e.target.value)}
                    placeholder="seconds"
                    className="w-24 rounded border border-ink-700 bg-ink-950 px-2 py-1 text-ink-100 outline-none focus:border-brass-500"
                />
                <button
                    type="button"
                    onClick={() => void handleSetTTL()}
                    disabled={ttlSaving}
                    className="rounded border border-ink-700 px-2 py-1 text-ink-200 transition-colors hover:border-brass-500 hover:text-brass-400 disabled:cursor-not-allowed disabled:opacity-50"
                >
                    Set TTL
                </button>
                <button
                    type="button"
                    onClick={() => void handlePersist()}
                    disabled={ttlSaving}
                    className="rounded border border-ink-700 px-2 py-1 text-ink-200 transition-colors hover:border-brass-500 hover:text-brass-400 disabled:cursor-not-allowed disabled:opacity-50"
                >
                    Persist
                </button>
                {ttlError && <span className="text-red-400">{ttlError}</span>}
            </div>

            <div className="flex flex-wrap items-center gap-2 border-t border-ink-800 pt-2 text-xs">
                <input
                    type="text"
                    value={renameValue}
                    onChange={(e) => setRenameValue(e.target.value)}
                    className="w-56 rounded border border-ink-700 bg-ink-950 px-2 py-1 font-mono text-ink-100 outline-none focus:border-brass-500"
                />
                <button
                    type="button"
                    onClick={() => void handleRename()}
                    disabled={renameSaving}
                    className="rounded border border-ink-700 px-2 py-1 text-ink-200 transition-colors hover:border-brass-500 hover:text-brass-400 disabled:cursor-not-allowed disabled:opacity-50"
                >
                    Rename
                </button>
                <button
                    type="button"
                    onClick={() => void handleDelete()}
                    disabled={deleting}
                    className="rounded border border-red-800 px-2 py-1 text-red-400 transition-colors hover:border-red-500 hover:text-red-300 disabled:cursor-not-allowed disabled:opacity-50"
                >
                    {deleting ? 'Deleting…' : 'Delete key'}
                </button>
                {renameError && <span className="text-red-400">{renameError}</span>}
                {deleteError && <span className="text-red-400">{deleteError}</span>}
            </div>

            <div className="border-t border-ink-800 pt-2">
                {keyType === 'string' && <RedisStringValue sessionID={sessionID} redisKey={redisKey} />}
                {keyType === 'hash' && <RedisHashValue sessionID={sessionID} redisKey={redisKey} />}
                {keyType === 'list' && <RedisListValue sessionID={sessionID} redisKey={redisKey} />}
                {keyType === 'set' && <RedisSetValue sessionID={sessionID} redisKey={redisKey} />}
                {keyType === 'zset' && <RedisSortedSetValue sessionID={sessionID} redisKey={redisKey} />}
                {keyType === 'none' && <p className="text-xs text-ink-500">This key no longer exists.</p>}
            </div>
        </div>
    )
}

export default RedisKeyDetail
