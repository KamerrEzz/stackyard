import type {dbengine} from '../../../wailsjs/go/models'
import type {SchemaSnapshot} from './QueryEditor'

interface SchemaTreeProps {
    snapshot: SchemaSnapshot | null
    onRefresh: () => void
    onNewTable: () => void
    onOpenTable: (schema: string, table: dbengine.TableInfo) => void
    onImport: (schema: string, table: dbengine.TableInfo) => void
    onExportSchema: (schema: string, target: 'prisma' | 'drizzle') => void
}

/**
 * Left sidebar's schema/table tree for the active SQL tab (tasks.md 11.1):
 * schemas grouped with their tables, "+ New table"/"Refresh schema" quick
 * actions up top, and clicking a table opens it in the center panel's "Data"
 * sub-tab (tasks.md 11.2) — the discoverability fix for what used to be a
 * small, easy-to-miss "+ New table" button buried in a long scrolling page.
 *
 * Deliberately stateless and connection-agnostic: every action here is a
 * callback into the active tab's own `QueryEditorHandle` (see
 * `DbClientView`), which owns the actual session/schema state. This
 * component only ever renders whatever `snapshot` it's handed.
 */
function SchemaTree({snapshot, onRefresh, onNewTable, onOpenTable, onImport, onExportSchema}: SchemaTreeProps) {
    const state = snapshot?.state ?? 'idle'
    const entries = snapshot?.entries ?? []

    return (
        <div className="flex flex-col gap-2">
            <div className="flex items-center justify-between">
                <h2 className="text-xs uppercase tracking-widest text-ink-400">Schema</h2>
                <div className="flex items-center gap-1">
                    <button
                        type="button"
                        onClick={onRefresh}
                        disabled={state === 'loading'}
                        className="rounded border border-ink-700 px-2 py-1 text-[10px] text-ink-200 transition-colors hover:border-brass-500 hover:text-brass-400 disabled:cursor-not-allowed disabled:opacity-50"
                    >
                        {state === 'loading' ? 'Loading…' : 'Refresh'}
                    </button>
                    <button
                        type="button"
                        onClick={onNewTable}
                        className="rounded border border-ink-700 px-2 py-1 text-[10px] text-ink-200 transition-colors hover:border-brass-500 hover:text-brass-400"
                    >
                        + New table
                    </button>
                </div>
            </div>

            {state === 'error' && snapshot?.error && (
                <p className="text-xs text-red-400" title={snapshot.error}>
                    Schema unavailable
                </p>
            )}

            {state === 'idle' && (
                <p className="text-xs text-ink-500">Open a query tab to load its schema tree here.</p>
            )}

            {entries.length === 0 && state === 'loaded' && <p className="text-xs text-ink-500">No tables found.</p>}

            <div className="flex flex-col gap-3 overflow-auto">
                {entries.map((entry) => (
                    <div key={entry.schema} className="flex flex-col gap-1">
                        <div className="flex items-center justify-between gap-2">
                            <span className="font-mono text-[10px] text-ink-500">{entry.schema}</span>
                            <div className="flex items-center gap-1">
                                <button
                                    type="button"
                                    onClick={() => onExportSchema(entry.schema, 'prisma')}
                                    className="rounded border border-ink-700 px-1.5 py-0.5 text-[9px] text-ink-300 transition-colors hover:border-brass-500 hover:text-brass-400"
                                >
                                    Prisma
                                </button>
                                <button
                                    type="button"
                                    onClick={() => onExportSchema(entry.schema, 'drizzle')}
                                    className="rounded border border-ink-700 px-1.5 py-0.5 text-[9px] text-ink-300 transition-colors hover:border-brass-500 hover:text-brass-400"
                                >
                                    Drizzle
                                </button>
                            </div>
                        </div>
                        <div className="flex flex-col gap-0.5">
                            {entry.tables.map((table) => (
                                <div
                                    key={`${entry.schema}.${table.Name}`}
                                    className="flex items-center justify-between gap-2 rounded px-1.5 py-1 hover:bg-ink-800/60"
                                >
                                    <button
                                        type="button"
                                        onClick={() => onOpenTable(entry.schema, table)}
                                        className="truncate text-left font-mono text-xs text-ink-200 hover:text-brass-400"
                                        title={`Browse ${entry.schema}.${table.Name}`}
                                    >
                                        {table.Name}
                                    </button>
                                    <button
                                        type="button"
                                        onClick={() => onImport(entry.schema, table)}
                                        className="shrink-0 rounded border border-ink-700 px-1.5 py-0.5 text-[9px] text-ink-400 transition-colors hover:border-brass-500 hover:text-brass-400"
                                    >
                                        Import
                                    </button>
                                </div>
                            ))}
                        </div>
                    </div>
                ))}
            </div>

            {snapshot?.exportState === 'saved' && snapshot.exportMessage && (
                <p className="text-xs text-emerald-400">{snapshot.exportMessage}</p>
            )}
            {snapshot?.exportState === 'error' && snapshot.exportMessage && (
                <p className="text-xs text-red-400">{snapshot.exportMessage}</p>
            )}
        </div>
    )
}

export default SchemaTree
