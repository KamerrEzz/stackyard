import {useCallback, useEffect, useRef, useState} from 'react'
import {
    CheckProfilePortConflict,
    CreateProfile,
    GetConnectionString,
    GetProfileStatus,
    ListProfiles,
    StartProfile,
    StopProfile,
} from '../../../wailsjs/go/main/App'
import type {main} from '../../../wailsjs/go/models'

// Task 1.4 scope: Postgres-only profile list + Start/Stop (spec.md §3.1/
// §3.2). Task 1.5 (port-conflict pre-check) and task 1.6 (connection-string
// copy) are implemented below. No multi-engine wizard (task 2.4, Phase 2) —
// deliberately out of scope here.

type ProfileStatus = 'running' | 'stopped' | 'partial' | 'unknown' | 'loading'

interface ProfileRow {
    summary: main.ProfileSummary
    status: ProfileStatus
    busy: boolean
    error: string | null
}

function summaryToRow(summary: main.ProfileSummary): ProfileRow {
    return {summary, status: 'loading', busy: false, error: null}
}

function statusLabel(status: ProfileStatus): string {
    switch (status) {
        case 'running':
            return 'Running'
        case 'stopped':
            return 'Stopped'
        case 'partial':
            return 'Partially running'
        case 'unknown':
            return 'Unknown'
        default:
            return 'Checking…'
    }
}

function statusColor(status: ProfileStatus): string {
    switch (status) {
        case 'running':
            return 'text-emerald-400'
        case 'stopped':
            return 'text-ink-400'
        case 'partial':
            return 'text-brass-400'
        case 'unknown':
            return 'text-red-400'
        default:
            return 'text-ink-400'
    }
}

function EnvironmentManagerView() {
    const [rows, setRows] = useState<ProfileRow[]>([])
    const [listError, setListError] = useState<string | null>(null)
    const [newProfileName, setNewProfileName] = useState('')
    const [creating, setCreating] = useState(false)
    const [createError, setCreateError] = useState<string | null>(null)

    const refreshStatus = useCallback(async (profileId: number) => {
        try {
            const status = await GetProfileStatus(profileId)
            setRows((prev) =>
                prev.map((row) =>
                    row.summary.Profile.ID === profileId
                        ? {...row, status: status as ProfileStatus, error: null}
                        : row,
                ),
            )
        } catch (err) {
            setRows((prev) =>
                prev.map((row) =>
                    row.summary.Profile.ID === profileId ? {...row, status: 'unknown', error: String(err)} : row,
                ),
            )
        }
    }, [])

    const loadProfiles = useCallback(async () => {
        try {
            const summaries = await ListProfiles()
            setListError(null)
            const list = summaries ?? []
            setRows(list.map(summaryToRow))
            for (const summary of list) {
                void refreshStatus(summary.Profile.ID)
            }
        } catch (err) {
            setListError(String(err))
        }
    }, [refreshStatus])

    useEffect(() => {
        void loadProfiles()
    }, [loadProfiles])

    const setRowBusy = useCallback((profileId: number, busy: boolean, error: string | null = null) => {
        setRows((prev) => prev.map((row) => (row.summary.Profile.ID === profileId ? {...row, busy, error} : row)))
    }, [])

    const handleStart = useCallback(
        async (profileId: number) => {
            setRowBusy(profileId, true)

            // Task 1.5 / spec.md §3.2: detect a port conflict BEFORE
            // attempting Start, so the user sees "port 5432 is already in
            // use — try 5433" instead of a raw Docker bind error. StartProfile
            // itself re-checks the same thing server-side (defense in
            // depth, see app.go), so skipping this pre-check on error here
            // still leaves that guarantee intact.
            try {
                const conflict = await CheckProfilePortConflict(profileId)
                if (conflict.HasConflict) {
                    const suggestion =
                        conflict.SuggestedPort > 0 ? ` Try port ${conflict.SuggestedPort} instead.` : ''
                    setRowBusy(profileId, false, `Port ${conflict.Port} is already in use by another process.${suggestion}`)
                    return
                }
            } catch {
                // If the pre-check itself fails (e.g. Docker unreachable),
                // fall through to StartProfile — it surfaces its own clear
                // error (requireDocker) rather than silently blocking Start.
            }

            try {
                await StartProfile(profileId)
            } catch (err) {
                setRowBusy(profileId, false, String(err))
                await refreshStatus(profileId)
                return
            }
            await refreshStatus(profileId)
            setRowBusy(profileId, false)
        },
        [refreshStatus, setRowBusy],
    )

    const handleStop = useCallback(
        async (profileId: number) => {
            setRowBusy(profileId, true)
            try {
                await StopProfile(profileId)
            } catch (err) {
                setRowBusy(profileId, false, String(err))
                await refreshStatus(profileId)
                return
            }
            await refreshStatus(profileId)
            setRowBusy(profileId, false)
        },
        [refreshStatus, setRowBusy],
    )

    const handleCreateAndStart = useCallback(async () => {
        const name = newProfileName.trim()
        if (!name) {
            return
        }
        setCreating(true)
        setCreateError(null)
        try {
            const summary = await CreateProfile(name)
            setRows((prev) => [...prev, summaryToRow(summary)])
            setNewProfileName('')
            await handleStart(summary.Profile.ID)
        } catch (err) {
            setCreateError(String(err))
        } finally {
            setCreating(false)
        }
    }, [handleStart, newProfileName])

    return (
        <div className="flex flex-col gap-6">
            <div>
                <h1 className="text-xl font-semibold text-ink-100">Environments</h1>
                <p className="text-sm text-ink-400">
                    Postgres-only for now — MySQL, MongoDB, and Redis arrive in Phase 2.
                </p>
            </div>

            <div className="flex items-end gap-3 rounded border border-ink-800 bg-ink-900/40 p-4">
                <div className="flex flex-1 flex-col gap-1">
                    <label htmlFor="new-profile-name" className="text-xs uppercase tracking-widest text-ink-400">
                        New profile name
                    </label>
                    <input
                        id="new-profile-name"
                        type="text"
                        value={newProfileName}
                        onChange={(e) => setNewProfileName(e.target.value)}
                        placeholder="e.g. my-side-project"
                        className="rounded border border-ink-700 bg-ink-950 px-3 py-2 text-sm text-ink-100 outline-none focus:border-brass-500"
                    />
                </div>
                <button
                    type="button"
                    onClick={() => void handleCreateAndStart()}
                    disabled={creating || newProfileName.trim().length === 0}
                    className="rounded bg-brass-600 px-4 py-2 text-sm font-medium text-ink-950 transition-colors hover:bg-brass-500 disabled:cursor-not-allowed disabled:opacity-50"
                >
                    {creating ? 'Creating…' : 'Create & Start'}
                </button>
            </div>
            {createError && <p className="text-sm text-red-400">{createError}</p>}
            {listError && <p className="text-sm text-red-400">{listError}</p>}

            <div className="flex flex-col gap-2">
                {rows.length === 0 && !listError && (
                    <p className="text-sm text-ink-400">No profiles yet — create one above.</p>
                )}
                {rows.map((row) => (
                    <ProfileCard
                        key={row.summary.Profile.ID}
                        row={row}
                        onStart={() => void handleStart(row.summary.Profile.ID)}
                        onStop={() => void handleStop(row.summary.Profile.ID)}
                    />
                ))}
            </div>
        </div>
    )
}

interface ProfileCardProps {
    row: ProfileRow
    onStart: () => void
    onStop: () => void
}

function ProfileCard({row, onStart, onStop}: ProfileCardProps) {
    const {summary, status, busy, error} = row
    const postgresService = summary.Services.find((s) => s.Engine === 'postgres')

    return (
        <div className="flex flex-col gap-2 rounded border border-ink-800 bg-ink-900/40 p-4">
            <div className="flex items-center justify-between">
                <div>
                    <h2 className="text-sm font-semibold text-ink-100">{summary.Profile.Name}</h2>
                    {postgresService && (
                        <p className="font-mono text-xs text-ink-400">
                            postgres · localhost:{postgresService.HostPort}
                        </p>
                    )}
                </div>
                <div className="flex items-center gap-3">
                    <span className={`text-xs font-medium ${statusColor(status)}`}>{statusLabel(status)}</span>
                    {status === 'running' && postgresService && (
                        <CopyConnectionStringButton serviceId={postgresService.ID} />
                    )}
                    <button
                        type="button"
                        onClick={onStart}
                        disabled={busy || status === 'running' || status === 'loading'}
                        className="rounded border border-ink-700 px-3 py-1 text-xs text-ink-200 transition-colors hover:border-brass-500 hover:text-brass-400 disabled:cursor-not-allowed disabled:opacity-40"
                    >
                        {busy ? '…' : 'Start'}
                    </button>
                    <button
                        type="button"
                        onClick={onStop}
                        disabled={busy || status === 'stopped' || status === 'loading'}
                        className="rounded border border-ink-700 px-3 py-1 text-xs text-ink-200 transition-colors hover:border-brass-500 hover:text-brass-400 disabled:cursor-not-allowed disabled:opacity-40"
                    >
                        {busy ? '…' : 'Stop'}
                    </button>
                </div>
            </div>
            {error && <p className="text-xs text-red-400">{error}</p>}
        </div>
    )
}

// CONFIRMATION_MS is how long the transient "Copied!" acknowledgment stays
// visible (spec.md §3.3's "toast/inline confirmation" — a full toast/
// notification library is explicitly not required for this task).
const CONFIRMATION_MS = 2000

interface CopyConnectionStringButtonProps {
    serviceId: number
}

// CopyConnectionStringButton fetches the connection string fresh from the Go
// backend (GetConnectionString — see app.go/internal/docker/connstring.go)
// on every click, so it's never a stale/cached value even if credentials or
// the port changed since the last render (spec.md §3.3's third acceptance
// criterion). One click copies it to the clipboard via the Wails webview's
// navigator.clipboard and shows an inline "Copied!" confirmation for
// CONFIRMATION_MS.
function CopyConnectionStringButton({serviceId}: CopyConnectionStringButtonProps) {
    const [state, setState] = useState<'idle' | 'copying' | 'copied' | 'error'>('idle')
    const timeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null)

    useEffect(() => {
        return () => {
            if (timeoutRef.current) {
                clearTimeout(timeoutRef.current)
            }
        }
    }, [])

    const handleCopy = useCallback(async () => {
        setState('copying')
        try {
            const connectionString = await GetConnectionString(serviceId)
            await navigator.clipboard.writeText(connectionString)
            setState('copied')
        } catch {
            setState('error')
        } finally {
            if (timeoutRef.current) {
                clearTimeout(timeoutRef.current)
            }
            timeoutRef.current = setTimeout(() => setState('idle'), CONFIRMATION_MS)
        }
    }, [serviceId])

    const label = state === 'copied' ? 'Copied!' : state === 'error' ? 'Copy failed' : 'Copy connection string'

    return (
        <button
            type="button"
            onClick={() => void handleCopy()}
            disabled={state === 'copying'}
            className={`rounded border px-3 py-1 text-xs transition-colors disabled:cursor-not-allowed disabled:opacity-40 ${
                state === 'copied'
                    ? 'border-emerald-600 text-emerald-400'
                    : state === 'error'
                      ? 'border-red-600 text-red-400'
                      : 'border-ink-700 text-ink-200 hover:border-brass-500 hover:text-brass-400'
            }`}
        >
            {label}
        </button>
    )
}

export default EnvironmentManagerView
