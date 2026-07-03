import {useCallback, useEffect, useRef, useState} from 'react'
import {
    ApplyMigrations,
    CloseConnectionSession,
    ConnectUsingSavedConnection,
    CreateMigrationFile,
    EnsureMigrationsTable,
    ListAppliedMigrationVersions,
    ListConnections,
    ListMigrations,
    OpenConnection,
    PickMigrationsFolder,
    RollbackMigration,
    SetConnectionMigrationsFolder,
} from '../../../wailsjs/go/main/App'
import type {migrations, storage} from '../../../wailsjs/go/models'
import {computeMigrationStatuses, hasAnyAppliedMigration, isRelationalConnection, migrationName} from './migrationHelpers'

type SessionState = 'idle' | 'connecting' | 'ready' | 'error'
type ActionState = 'idle' | 'busy'

/**
 * Top-level module for the migrations feature (spec.md §4.8, tasks.md 8.5),
 * given its own sidebar entry rather than living inside `DbClientView`'s
 * tab strip (see `Sidebar.tsx`/`App.tsx`). Every other DB Client feature —
 * SQL tabs, Mongo/Redis browsing, schema diagrams — is scoped to a live
 * connection SESSION opened ad hoc from the connection form; migrations are
 * scoped to a saved connection RECORD instead (`connectionID`, tied to
 * `schema_migrations` state living in that target database and migration
 * files living in a folder on disk, plan.md §4). A query-editor tab has
 * nothing to attach that folder/state pairing to — there is no "new
 * migrations tab" the way there's a "new SQL tab" — so this is a standalone
 * module with its own connection-selection UI, the same shape
 * `SchemaDiagramView` established, except it picks from already-SAVED
 * connections (`ListConnections`) rather than composing a fresh one, since
 * a migrations folder is persisted per saved `storage.Connection` row, not
 * per throwaway session.
 *
 * Only PostgreSQL/MySQL connections are offered at all (spec.md §4.8's own
 * title) — Mongo/Redis rows are filtered out of the picker entirely rather
 * than shown disabled, since there is no migrations feature for them in v1,
 * not merely an unavailable one.
 */
function MigrationsView() {
    const [connections, setConnections] = useState<storage.Connection[]>([])
    const [connectionsError, setConnectionsError] = useState<string | null>(null)

    const [selectedId, setSelectedId] = useState<number | null>(null)

    const [sessionState, setSessionState] = useState<SessionState>('idle')
    const [sessionError, setSessionError] = useState<string | null>(null)
    const sessionIdRef = useRef<string | null>(null)

    const [folderBusy, setFolderBusy] = useState(false)
    const [folderError, setFolderError] = useState<string | null>(null)

    const [migrationList, setMigrationList] = useState<migrations.Migration[]>([])
    const [appliedVersions, setAppliedVersions] = useState<number[]>([])
    const [listError, setListError] = useState<string | null>(null)

    const [newMigrationName, setNewMigrationName] = useState('')
    const [createState, setCreateState] = useState<ActionState>('idle')
    const [createError, setCreateError] = useState<string | null>(null)

    const [applyState, setApplyState] = useState<ActionState>('idle')
    const [applyError, setApplyError] = useState<string | null>(null)
    const [applyResult, setApplyResult] = useState<migrations.ApplyResult | null>(null)

    const [rollbackState, setRollbackState] = useState<ActionState>('idle')
    const [rollbackError, setRollbackError] = useState<string | null>(null)
    const [rollbackMessage, setRollbackMessage] = useState<string | null>(null)

    const relationalConnections = connections.filter(isRelationalConnection)
    const selectedConnection = relationalConnections.find((conn) => conn.ID === selectedId) ?? null
    const items = computeMigrationStatuses(migrationList, appliedVersions)

    const refreshConnections = useCallback(async () => {
        try {
            const all = await ListConnections()
            setConnectionsError(null)
            setConnections(all)
            return all
        } catch (err) {
            setConnectionsError(String(err))
            return []
        }
    }, [])

    useEffect(() => {
        void refreshConnections()
    }, [refreshConnections])

    const closeSession = useCallback(() => {
        const id = sessionIdRef.current
        if (id) {
            void CloseConnectionSession(id)
            sessionIdRef.current = null
        }
    }, [])

    const refreshMigrationList = useCallback(async (connectionId: number, sessionId: string | null) => {
        try {
            const all = await ListMigrations(connectionId)
            setListError(null)
            setMigrationList(all)
        } catch (err) {
            setListError(String(err))
            setMigrationList([])
            return
        }

        if (!sessionId) {
            setAppliedVersions([])
            return
        }
        try {
            const applied = await ListAppliedMigrationVersions(sessionId)
            setAppliedVersions(applied)
        } catch (err) {
            setListError(String(err))
            setAppliedVersions([])
        }
    }, [])

    /**
     * Opens a live session for `selectedConnection` and bootstraps its
     * `schema_migrations` table (tasks.md 8.2-8.5), following the exact
     * `RedisKeyBrowser`/`MongoDocumentView` mount-lifecycle pattern — a
     * `cancelled` guard so React 18 StrictMode's dev-only double-invoke of
     * effects never lets a stale first-mount session leak past this
     * component swapping to a second, "real" mount.
     */
    useEffect(() => {
        sessionIdRef.current = null
        setSessionState('idle')
        setSessionError(null)
        setMigrationList([])
        setAppliedVersions([])
        setListError(null)
        setApplyResult(null)
        setApplyError(null)
        setRollbackMessage(null)
        setRollbackError(null)

        if (!selectedConnection || !selectedConnection.MigrationsFolder) {
            return
        }

        let cancelled = false
        setSessionState('connecting')

        void (async () => {
            try {
                const fields = await ConnectUsingSavedConnection(selectedConnection.ID)
                const sessionId = await OpenConnection(fields)
                if (cancelled) {
                    void CloseConnectionSession(sessionId)
                    return
                }
                sessionIdRef.current = sessionId
                await EnsureMigrationsTable(sessionId)
                if (cancelled) {
                    return
                }
                setSessionState('ready')
                await refreshMigrationList(selectedConnection.ID, sessionId)
            } catch (err) {
                if (!cancelled) {
                    setSessionState('error')
                    setSessionError(String(err))
                }
            }
        })()

        return () => {
            cancelled = true
            closeSession()
        }
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [selectedConnection?.ID, selectedConnection?.MigrationsFolder])

    const handleSelectConnection = useCallback((id: number) => {
        setSelectedId(id)
        setFolderError(null)
        setCreateError(null)
        setNewMigrationName('')
    }, [])

    const handlePickFolder = useCallback(async () => {
        if (!selectedConnection) {
            return
        }
        setFolderBusy(true)
        setFolderError(null)
        try {
            const folder = await PickMigrationsFolder()
            if (!folder) {
                return
            }
            const updated = await SetConnectionMigrationsFolder(selectedConnection.ID, folder)
            setConnections((prev) => prev.map((conn) => (conn.ID === updated.ID ? updated : conn)))
        } catch (err) {
            setFolderError(String(err))
        } finally {
            setFolderBusy(false)
        }
    }, [selectedConnection])

    const handleCreateMigration = useCallback(async () => {
        if (!selectedConnection || !newMigrationName.trim()) {
            return
        }
        setCreateState('busy')
        setCreateError(null)
        try {
            await CreateMigrationFile(selectedConnection.ID, newMigrationName.trim())
            setNewMigrationName('')
            await refreshMigrationList(selectedConnection.ID, sessionIdRef.current)
        } catch (err) {
            setCreateError(String(err))
        } finally {
            setCreateState('idle')
        }
    }, [newMigrationName, refreshMigrationList, selectedConnection])

    const handleApply = useCallback(async () => {
        const sessionId = sessionIdRef.current
        if (!selectedConnection || !sessionId) {
            return
        }
        setApplyState('busy')
        setApplyError(null)
        setApplyResult(null)
        try {
            const result = await ApplyMigrations(sessionId)
            setApplyResult(result)
            await refreshMigrationList(selectedConnection.ID, sessionId)
        } catch (err) {
            setApplyError(String(err))
        } finally {
            setApplyState('idle')
        }
    }, [refreshMigrationList, selectedConnection])

    /**
     * Rollback reverts a real, applied schema change against a real
     * database and cannot be undone automatically (unlike, say, closing a
     * tab) — this project's established destructive-action pattern
     * (`window.confirm`, see `RedisKeyBrowser`/`MongoDocumentView`) applies
     * here too, even though spec.md doesn't call out a confirmation
     * requirement for Rollback the way it does for delete operations. The
     * button itself is only enabled once `hasAnyAppliedMigration` is true
     * (computed client-side from the same list this panel already renders),
     * so the confirm dialog only ever appears when a real rollback is about
     * to run — never a confirm-then-"nothing to roll back" dead end.
     */
    const handleRollback = useCallback(async () => {
        const sessionId = sessionIdRef.current
        if (!selectedConnection || !sessionId) {
            return
        }
        if (!window.confirm('Roll back the most recently applied migration? This runs its down.sql against the real database and cannot be undone automatically.')) {
            return
        }
        setRollbackState('busy')
        setRollbackError(null)
        setRollbackMessage(null)
        try {
            const reverted = await RollbackMigration(sessionId)
            setRollbackMessage(reverted ? `Rolled back ${migrationName(reverted)}.` : 'Nothing to roll back.')
            await refreshMigrationList(selectedConnection.ID, sessionId)
        } catch (err) {
            setRollbackError(String(err))
        } finally {
            setRollbackState('idle')
        }
    }, [refreshMigrationList, selectedConnection])

    const canRollback = sessionState === 'ready' && hasAnyAppliedMigration(items) && rollbackState === 'idle'

    return (
        <div className="flex flex-col gap-6">
            <div>
                <h1 className="text-xl font-semibold text-ink-100">Migrations</h1>
                <p className="text-sm text-ink-400">
                    Create, apply, and roll back schema migrations for a saved PostgreSQL or MySQL connection.
                    Applied/pending state is tracked inside the target database itself; migration files live in a
                    folder on disk you point each connection at.
                </p>
            </div>

            <div className="flex flex-col gap-3 rounded border border-ink-800 bg-ink-900/40 p-4">
                <h2 className="text-xs uppercase tracking-widest text-ink-400">Saved connections</h2>
                {connectionsError && <p className="text-xs text-red-400">{connectionsError}</p>}
                {relationalConnections.length === 0 && !connectionsError && (
                    <p className="text-sm text-ink-500">
                        No saved PostgreSQL or MySQL connections yet — save one from DB Client first.
                    </p>
                )}
                {relationalConnections.map((conn) => (
                    <button
                        key={conn.ID}
                        type="button"
                        onClick={() => handleSelectConnection(conn.ID)}
                        className={`flex items-center justify-between gap-3 rounded border px-3 py-2 text-left transition-colors ${
                            conn.ID === selectedId
                                ? 'border-brass-500 bg-ink-800'
                                : 'border-ink-800 bg-ink-950/60 hover:border-brass-500/60'
                        }`}
                    >
                        <div className="flex flex-col">
                            <span className="text-sm font-medium text-ink-100">{conn.Name}</span>
                            <span className="font-mono text-xs text-ink-400">
                                {conn.Engine}://{conn.Host}:{conn.Port}
                                {conn.Database ? `/${conn.Database}` : ''}
                            </span>
                        </div>
                        <span className="text-xs text-ink-500">
                            {conn.MigrationsFolder ? conn.MigrationsFolder : 'No folder configured'}
                        </span>
                    </button>
                ))}
            </div>

            {selectedConnection && (
                <div className="flex flex-col gap-4 rounded border border-ink-800 bg-ink-900/40 p-4">
                    <div className="flex items-center justify-between gap-3">
                        <div className="flex flex-col">
                            <h2 className="text-sm font-medium text-ink-100">{selectedConnection.Name}</h2>
                            <span className="font-mono text-xs text-ink-400">
                                {selectedConnection.MigrationsFolder ?? 'No migrations folder configured yet'}
                            </span>
                        </div>
                        <button
                            type="button"
                            onClick={() => void handlePickFolder()}
                            disabled={folderBusy}
                            className="rounded border border-ink-700 px-4 py-2 text-sm font-medium text-ink-200 transition-colors hover:border-brass-500 hover:text-brass-400 disabled:cursor-not-allowed disabled:opacity-50"
                        >
                            {folderBusy ? 'Choosing…' : selectedConnection.MigrationsFolder ? 'Change folder' : 'Choose folder'}
                        </button>
                    </div>
                    {folderError && <p className="text-xs text-red-400">{folderError}</p>}

                    {!selectedConnection.MigrationsFolder && (
                        <p className="text-sm text-ink-500">
                            Choose a folder to store this connection's migration files before creating or running any
                            migration.
                        </p>
                    )}

                    {selectedConnection.MigrationsFolder && (
                        <>
                            {sessionState === 'connecting' && <p className="text-sm text-ink-500">Connecting…</p>}
                            {sessionState === 'error' && sessionError && (
                                <p className="text-sm text-red-400">{sessionError}</p>
                            )}

                            <div className="flex items-center gap-3 border-t border-ink-800 pt-3">
                                <input
                                    type="text"
                                    value={newMigrationName}
                                    onChange={(e) => setNewMigrationName(e.target.value)}
                                    placeholder="Migration name, e.g. create users table"
                                    className="flex-1 rounded border border-ink-700 bg-ink-950 px-3 py-2 text-sm text-ink-100 outline-none focus:border-brass-500"
                                />
                                <button
                                    type="button"
                                    onClick={() => void handleCreateMigration()}
                                    disabled={createState === 'busy' || newMigrationName.trim().length === 0}
                                    className="rounded border border-ink-700 px-4 py-2 text-sm font-medium text-ink-200 transition-colors hover:border-brass-500 hover:text-brass-400 disabled:cursor-not-allowed disabled:opacity-50"
                                >
                                    {createState === 'busy' ? 'Creating…' : 'Create migration'}
                                </button>
                            </div>
                            {createError && <p className="text-xs text-red-400">{createError}</p>}

                            <div className="flex items-center gap-3">
                                <button
                                    type="button"
                                    onClick={() => void handleApply()}
                                    disabled={sessionState !== 'ready' || applyState === 'busy'}
                                    className="rounded bg-brass-600 px-4 py-2 text-sm font-medium text-ink-950 transition-colors hover:bg-brass-500 disabled:cursor-not-allowed disabled:opacity-50"
                                >
                                    {applyState === 'busy' ? 'Applying…' : 'Apply pending migrations'}
                                </button>
                                <button
                                    type="button"
                                    onClick={() => void handleRollback()}
                                    disabled={!canRollback}
                                    className="rounded border border-red-800 px-4 py-2 text-sm font-medium text-red-400 transition-colors hover:border-red-500 hover:text-red-300 disabled:cursor-not-allowed disabled:opacity-50"
                                >
                                    {rollbackState === 'busy' ? 'Rolling back…' : 'Rollback last migration'}
                                </button>
                            </div>
                            {applyError && <p className="text-sm text-red-400">{applyError}</p>}
                            {rollbackError && <p className="text-sm text-red-400">{rollbackError}</p>}
                            {rollbackMessage && (
                                <p className={rollbackMessage === 'Nothing to roll back.' ? 'text-sm text-ink-400' : 'text-sm text-emerald-400'}>
                                    {rollbackMessage}
                                </p>
                            )}

                            {applyResult && (
                                <div className="flex flex-col gap-1 rounded border border-ink-800 bg-ink-950/60 p-3">
                                    {applyResult.Applied.length > 0 && (
                                        <p className="text-sm text-emerald-400">
                                            Applied: {applyResult.Applied.map((m) => migrationName(m)).join(', ')}
                                        </p>
                                    )}
                                    {applyResult.Applied.length === 0 && !applyResult.Failed && (
                                        <p className="text-sm text-ink-400">Nothing pending — already up to date.</p>
                                    )}
                                    {applyResult.Failed && (
                                        <p className="text-sm text-red-400">
                                            Failed on {migrationName(applyResult.Failed)}: {applyResult.FailedError}
                                        </p>
                                    )}
                                </div>
                            )}

                            <div className="flex flex-col gap-2 border-t border-ink-800 pt-3">
                                <h3 className="text-xs uppercase tracking-widest text-ink-400">Migrations</h3>
                                {listError && <p className="text-xs text-red-400">{listError}</p>}
                                {items.length === 0 && !listError && (
                                    <p className="text-sm text-ink-500">
                                        No migrations yet — create one above.
                                    </p>
                                )}
                                {items.map((item) => (
                                    <div
                                        key={item.version}
                                        className="flex items-center justify-between rounded border border-ink-800 bg-ink-950/60 px-3 py-2"
                                    >
                                        <span className="font-mono text-sm text-ink-100">{item.name}</span>
                                        <span
                                            className={`rounded px-2 py-0.5 text-xs font-medium uppercase tracking-widest ${
                                                item.status === 'applied'
                                                    ? 'bg-emerald-900/40 text-emerald-400'
                                                    : 'bg-ink-800 text-ink-400'
                                            }`}
                                        >
                                            {item.status}
                                        </span>
                                    </div>
                                ))}
                            </div>
                        </>
                    )}
                </div>
            )}
        </div>
    )
}

export default MigrationsView
