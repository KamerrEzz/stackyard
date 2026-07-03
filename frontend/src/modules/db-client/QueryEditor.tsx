import Editor from '@monaco-editor/react'
import type * as monaco from 'monaco-editor'
import {forwardRef, useCallback, useEffect, useImperativeHandle, useRef, useState} from 'react'
import '../../lib/monacoSetup'
import {
    BrowseTableRows,
    CancelQuery,
    CloseConnectionSession,
    ExportQueryResultAsCSV,
    ExportQueryResultAsJSON,
    ExportSchemaAsDrizzle,
    ExportSchemaAsPrisma,
    ExportTableAsCSV,
    ExportTableAsJSON,
    ExportTableAsSQLDump,
    ListSchemasForSession,
    ListTablesForSession,
    OpenConnection,
    RunMultiStatementQuery,
} from '../../../wailsjs/go/main/App'
import type {dbengine, main} from '../../../wailsjs/go/models'
import CreateTableDialog from './CreateTableDialog'
import ExportControls from './ExportControls'
import {buildQueryResultExportPayload, describeExportOutcome, describeExportScope, type ExportFormat} from './exportHelpers'
import ImportDialog from './ImportDialog'
import {collapseStatementResults, summarizeStatementResults} from './multiStatementHelpers'
import {RESULTS_PAGE_SIZE} from './resultsGridHelpers'
import ResultsGrid, {type EditableGridContext} from './ResultsGrid'
import {registerModelSchemaProvider, unregisterModelSchemaProvider} from './schemaCompletion'
import {ensureSqlCompletionProviderRegistered} from './schemaCompletionProvider'

export type SchemaState = 'idle' | 'loading' | 'loaded' | 'error'
export type SchemaExportState = 'idle' | 'exporting' | 'saved' | 'error'
export type SchemaExportTarget = 'prisma' | 'drizzle'

export interface SchemaEntry {
    schema: string
    tables: dbengine.TableInfo[]
}

/**
 * Mirrors this tab's own schema/session state up to `DbClientView` (tasks.md
 * 11.1) so the left sidebar's `SchemaTree` — rendered outside this
 * component's own subtree entirely — can show the active tab's schema
 * without owning any connection/session state of its own. Pushed via
 * `onSchemaUpdate` any time the underlying state changes; `DbClientView`
 * only ever reads the most recently pushed snapshot per tab id.
 */
export interface SchemaSnapshot {
    state: SchemaState
    error: string | null
    entries: SchemaEntry[]
    exportState: SchemaExportState
    exportMessage: string | null
}

export type QueryEditorSubTab = 'query' | 'data'

interface QueryEditorProps {
    fields: main.ConnectionFormFields
    initialQuery?: string
    onSchemaUpdate?: (snapshot: SchemaSnapshot) => void
    /**
     * Which of this tab's two views to render — "Query" (the SQL editor +
     * run/results) or "Data" (the browsed table grid) — now controlled by
     * `DbClientView` (tasks.md 11.1's revised top-level tab nav) rather than
     * owned internally, since "Query"/"Data"/"Tools" are peer tabs at the
     * DbClientView level, not nested inside this component.
     */
    activeSubTab: QueryEditorSubTab
    /**
     * Lets this component ask the parent to switch the shared workspace tab
     * back to "Query" — used when the user clicks "Run query" while viewing
     * "Data", so the just-produced result is actually visible.
     */
    onRequestWorkspaceTab?: (tab: QueryEditorSubTab) => void
}

/**
 * Imperative surface a parent can use to command an already-mounted tab's
 * editor from outside (tasks.md 4.7, extended by 11.1's sidebar schema
 * tree): checking whether the tab has unsaved work, replacing its query
 * text (e.g. loading a snippet's body into the current tab), and driving
 * every schema-tree quick action (refresh schema, new table, browse a
 * table, import into a table, export a schema) from the sidebar without
 * that sidebar needing to know anything about sessions or connections
 * itself — this component still owns all of that internally.
 */
export interface QueryEditorHandle {
    isDirty(): boolean
    loadQuery(text: string): void
    refreshSchema(): Promise<void>
    openCreateTable(): Promise<void>
    browseTable(schema: string, table: dbengine.TableInfo): Promise<void>
    openImport(schema: string, table: dbengine.TableInfo): Promise<void>
    exportSchema(schema: string, target: SchemaExportTarget): Promise<void>
}

type RunState = 'idle' | 'connecting' | 'running' | 'success' | 'error'

const DEFAULT_QUERY = '-- Write a query and click "Run query"\nSELECT 1;'

/**
 * Row count fetched per `BrowseTableRows` page (tasks.md 4.1, spec.md §4.3).
 * Matches `ResultsGrid`'s own client-side page size so both pagination modes
 * (see ResultsGrid.tsx's doc comment) present the same number of rows per
 * page. Every Prev/Next click re-fetches a fresh page at a new offset from
 * the backend (see handleRequestBrowsePage) rather than ever fetching more
 * than one page's worth of rows up front — a table with more rows than a
 * single page stays fully reachable this way, unlike a one-shot fetch with a
 * fixed row cap.
 */
const BROWSE_PAGE_SIZE = RESULTS_PAGE_SIZE

ensureSqlCompletionProviderRegistered()

function monacoLanguageForEngine(engine: main.ConnectionFormFields['Engine']): string {
    switch (engine) {
        case 'postgres':
        case 'mysql':
            return 'sql'
        default:
            return 'plaintext'
    }
}

const QueryEditor = forwardRef<QueryEditorHandle, QueryEditorProps>(function QueryEditor(
    {fields, initialQuery, onSchemaUpdate, activeSubTab, onRequestWorkspaceTab},
    ref,
) {
    const [query, setQuery] = useState(initialQuery ?? DEFAULT_QUERY)
    const [runState, setRunState] = useState<RunState>('idle')
    const [errorMessage, setErrorMessage] = useState<string | null>(null)
    const [result, setResult] = useState<dbengine.QueryResult | null>(null)
    const [multiStatementResults, setMultiStatementResults] = useState<dbengine.StatementResult[] | null>(null)

    const [schemaState, setSchemaState] = useState<SchemaState>('idle')
    const [schemaError, setSchemaError] = useState<string | null>(null)
    const [schemaEntries, setSchemaEntries] = useState<SchemaEntry[]>([])

    const [browseResult, setBrowseResult] = useState<dbengine.QueryResult | null>(null)
    const [browseContext, setBrowseContext] = useState<EditableGridContext | null>(null)
    const [browseOffset, setBrowseOffset] = useState(0)
    const [browseError, setBrowseError] = useState<string | null>(null)
    const [browsing, setBrowsing] = useState(false)

    const [importTarget, setImportTarget] = useState<{sessionId: string; schema: string; table: dbengine.TableInfo} | null>(null)
    const [createTableSessionId, setCreateTableSessionId] = useState<string | null>(null)

    const [schemaExportState, setSchemaExportState] = useState<SchemaExportState>('idle')
    const [schemaExportMessage, setSchemaExportMessage] = useState<string | null>(null)

    const sessionIdRef = useRef<string | null>(null)
    const schemaTablesRef = useRef<dbengine.TableInfo[]>([])
    const editorRef = useRef<monaco.editor.IStandaloneCodeEditor | null>(null)
    const browseTableRef = useRef<{schema: string; table: string} | null>(null)

    /**
     * The text this tab is considered "clean" against (tasks.md 4.7's
     * dirty-tab check): the query text it was created/last explicitly
     * loaded with, never updated just by running the query. Diverging from
     * this baseline is what "unsaved work in that tab" means here — there
     * is no separate saved/unsaved concept for query text beyond this.
     */
    const baselineQueryRef = useRef(initialQuery ?? DEFAULT_QUERY)


    useEffect(() => {
        return () => {
            const sessionId = sessionIdRef.current
            if (sessionId) {
                void CloseConnectionSession(sessionId)
            }
            const model = editorRef.current?.getModel()
            if (model) {
                unregisterModelSchemaProvider(model)
            }
        }
    }, [])

    /**
     * Loads every schema's tables (with columns) for sessionId and stores
     * them in schemaTablesRef, the snapshot the registered completion
     * provider reads from for this tab's editor model (tasks.md 4.8).
     * Fetched once per session — right after OpenConnection succeeds (see
     * ensureSession) — and again only on an explicit "Refresh schema" click,
     * never on every keystroke: schema introspection is comparatively slow
     * and doesn't change often enough to justify refetching it continuously.
     */
    const loadSchema = useCallback(async (sessionId: string) => {
        setSchemaState('loading')
        try {
            const schemas = await ListSchemasForSession(sessionId)
            const tablesBySchema = await Promise.all(schemas.map((schema) => ListTablesForSession(sessionId, schema)))
            schemaTablesRef.current = tablesBySchema.flat()
            setSchemaEntries(schemas.map((schema, index) => ({schema, tables: tablesBySchema[index]})))
            setSchemaError(null)
            setSchemaState('loaded')
        } catch (err) {
            setSchemaError(String(err))
            setSchemaState('error')
        }
    }, [])

    const ensureSession = useCallback(async () => {
        if (sessionIdRef.current) {
            return sessionIdRef.current
        }
        const sessionId = await OpenConnection(fields)
        sessionIdRef.current = sessionId
        void loadSchema(sessionId)
        return sessionId
    }, [fields, loadSchema])

    const handleRefreshSchema = useCallback(async () => {
        try {
            const sessionId = await ensureSession()
            await loadSchema(sessionId)
        } catch (err) {
            setSchemaError(String(err))
            setSchemaState('error')
        }
    }, [ensureSession, loadSchema])

    const handleEditorMount = useCallback((editor: monaco.editor.IStandaloneCodeEditor) => {
        editorRef.current = editor
        const model = editor.getModel()
        if (model) {
            registerModelSchemaProvider(model, () => schemaTablesRef.current)
        }
    }, [])

    /**
     * Runs `query` through `RunMultiStatementQuery` (spec.md §4.6), which
     * treats every semicolon-separated statement as an independent execution
     * and returns one `dbengine.StatementResult` per statement. A script
     * with exactly one statement collapses back to the pre-existing
     * single-result view via `collapseStatementResults`, so the common case
     * — one statement, success or failure — renders exactly like it did
     * before multi-statement support existed; a script with 2+ statements
     * renders as the per-statement list instead (see the render below).
     */
    const handleRunQuery = useCallback(async () => {
        onRequestWorkspaceTab?.('query')
        setErrorMessage(null)
        setRunState(sessionIdRef.current ? 'running' : 'connecting')
        try {
            const sessionId = await ensureSession()
            setRunState('running')
            const statementResults = await RunMultiStatementQuery(sessionId, query)
            setBrowseResult(null)
            setBrowseContext(null)

            const view = collapseStatementResults(statementResults)
            if (view.mode === 'single-success') {
                setResult(view.result ?? null)
                setMultiStatementResults(null)
                setErrorMessage(null)
                setRunState('success')
            } else if (view.mode === 'single-failure') {
                setResult(null)
                setMultiStatementResults(null)
                setErrorMessage(view.errorMessage)
                setRunState('error')
            } else {
                setResult(null)
                setMultiStatementResults(view.results)
                setErrorMessage(null)
                setRunState('success')
            }
        } catch (err) {
            setResult(null)
            setMultiStatementResults(null)
            setErrorMessage(String(err))
            setRunState('error')
        }
    }, [ensureSession, query, onRequestWorkspaceTab])

    /**
     * "Browse" (tasks.md 4.1's View requirement, wired from the Tables list
     * below): fetches one table's rows directly by schema/table name via
     * BrowseTableRows, unlike RunQuery's arbitrary-SQL path, so the grid
     * knows unambiguously which table/schema/session to target for
     * edit/insert/delete (tasks.md 4.1-4.4). Replaces whichever result pane
     * — ad-hoc query or a previous browse — is currently showing, since only
     * one result pane is rendered at a time.
     */
    const handleBrowseTable = useCallback(
        async (schema: string, table: dbengine.TableInfo) => {
            onRequestWorkspaceTab?.('data')
            setBrowseError(null)
            setBrowsing(true)
            try {
                const sessionId = await ensureSession()
                const queryResult = await BrowseTableRows(sessionId, schema, table.Name, BROWSE_PAGE_SIZE, 0)
                setResult(null)
                setMultiStatementResults(null)
                setRunState('idle')
                browseTableRef.current = {schema, table: table.Name}
                setBrowseOffset(0)
                setBrowseResult(queryResult)
                setBrowseContext({sessionID: sessionId, schema, table: table.Name, columns: table.Columns})
            } catch (err) {
                setBrowseError(String(err))
            } finally {
                setBrowsing(false)
            }
        },
        [ensureSession, onRequestWorkspaceTab],
    )

    /**
     * `ResultsGrid`'s `onRequestPage` callback for the current table browse
     * (see ResultsGrid.tsx's server-pagination mode): re-calls
     * `BrowseTableRows` at the requested offset/limit against the same
     * session/schema/table the grid is already bound to, rather than
     * slicing the already-fetched page client-side — a browsed table can
     * have far more rows than any single page holds, and this is what keeps
     * every row reachable via Prev/Next instead of only the first page ever
     * fetched.
     */
    const handleRequestBrowsePage = useCallback(async (offset: number, limit: number) => {
        const sessionId = sessionIdRef.current
        const target = browseTableRef.current
        if (!sessionId || !target) {
            return
        }
        setBrowseError(null)
        setBrowsing(true)
        try {
            const queryResult = await BrowseTableRows(sessionId, target.schema, target.table, limit, offset)
            setBrowseResult(queryResult)
            setBrowseOffset(offset)
        } catch (err) {
            setBrowseError(String(err))
        } finally {
            setBrowsing(false)
        }
    }, [])

    /**
     * "Import" (tasks.md 7.4's file-picker + target-table-selector entry
     * point, wired from the same Tables list Browse/Export already use):
     * ensures a live session exists (same lazy-connect contract every other
     * per-table action here follows) then opens ImportDialog bound to that
     * exact schema/table. The dialog owns the rest of the flow — pick file,
     * validate, confirm — this handler's only job is picking the target.
     */
    const handleOpenImport = useCallback(
        async (schema: string, table: dbengine.TableInfo) => {
            try {
                const sessionId = await ensureSession()
                setImportTarget({sessionId, schema, table})
            } catch (err) {
                setSchemaError(String(err))
            }
        },
        [ensureSession],
    )

    /**
     * "+ New table" (tasks.md 10.2, wired from the same Tables panel
     * Browse/Import already live in): ensures a live session exists (the
     * same lazy-connect contract every other per-table action here
     * follows), then opens CreateTableDialog bound to that session. The
     * dialog owns the rest of the flow — table name, columns, submit — this
     * handler's only job is making sure a session exists first.
     */
    const handleOpenCreateTable = useCallback(async () => {
        try {
            const sessionId = await ensureSession()
            setCreateTableSessionId(sessionId)
        } catch (err) {
            setSchemaError(String(err))
        }
    }, [ensureSession])

    /**
     * After a table is successfully created, reloads the Tables list the
     * same way "Refresh schema" does (loadSchema), so the new table shows
     * up immediately without the user having to click Refresh themselves.
     */
    const handleTableCreated = useCallback(() => {
        const sessionId = sessionIdRef.current
        if (sessionId) {
            void loadSchema(sessionId)
        }
    }, [loadSchema])

    /**
     * "Export schema" (tasks.md 10.4/10.5, wired from the same Tables panel
     * every other schema-level action lives in): ensures a live session
     * exists (the same lazy-connect contract every other action here
     * follows), then dispatches to whichever ExportSchemaAs* bound method
     * matches target, targeting schema — the same schema entry's own tables
     * this Tables panel already lists, not just whichever table is
     * currently browsed. A cancelled native save dialog (an empty path, see
     * exportHelpers.describeExportOutcome) is shown as no-op, not an error,
     * matching ExportControls' own per-table/per-result export outcome
     * handling.
     */
    const handleExportSchema = useCallback(
        async (schema: string, target: SchemaExportTarget) => {
            setSchemaExportState('exporting')
            setSchemaExportMessage(null)
            try {
                const sessionId = await ensureSession()
                const path =
                    target === 'prisma' ? await ExportSchemaAsPrisma(sessionId, schema) : await ExportSchemaAsDrizzle(sessionId, schema)
                const outcome = describeExportOutcome(path)
                if (outcome.status === 'cancelled') {
                    setSchemaExportState('idle')
                    return
                }
                setSchemaExportState('saved')
                setSchemaExportMessage(`Saved to ${outcome.path}`)
            } catch (err) {
                setSchemaExportState('error')
                setSchemaExportMessage(String(err))
            }
        },
        [ensureSession],
    )

    useImperativeHandle(
        ref,
        () => ({
            isDirty: () => query !== baselineQueryRef.current,
            loadQuery: (text: string) => {
                baselineQueryRef.current = text
                setQuery(text)
            },
            refreshSchema: () => handleRefreshSchema(),
            openCreateTable: () => handleOpenCreateTable(),
            browseTable: (schema: string, table: dbengine.TableInfo) => handleBrowseTable(schema, table),
            openImport: (schema: string, table: dbengine.TableInfo) => handleOpenImport(schema, table),
            exportSchema: (schema: string, target: SchemaExportTarget) => handleExportSchema(schema, target),
        }),
        [query, handleRefreshSchema, handleOpenCreateTable, handleBrowseTable, handleOpenImport, handleExportSchema],
    )

    /**
     * Pushes this tab's schema/session state up to `DbClientView` (tasks.md
     * 11.1) every time it changes, so the left sidebar's `SchemaTree` can
     * render the active tab's schema without this component rendering that
     * tree itself. `onSchemaUpdate` is expected to be a stable per-tab
     * function reference (see `DbClientView`'s callback cache) — if it
     * weren't, this effect would still be correct, just less efficient.
     */
    useEffect(() => {
        onSchemaUpdate?.({
            state: schemaState,
            error: schemaError,
            entries: schemaEntries,
            exportState: schemaExportState,
            exportMessage: schemaExportMessage,
        })
    }, [schemaState, schemaError, schemaEntries, schemaExportState, schemaExportMessage, onSchemaUpdate])

    /**
     * After a successful import, refreshes the currently browsed grid only
     * when it is browsing the exact table just imported into — otherwise an
     * import into some other table would pointlessly re-fetch whatever
     * unrelated browse/query result happens to be on screen.
     */
    const handleImported = useCallback(() => {
        const target = browseTableRef.current
        if (importTarget && target && target.schema === importTarget.schema && target.table === importTarget.table.Name) {
            void handleRequestBrowsePage(0, BROWSE_PAGE_SIZE)
        }
    }, [handleRequestBrowsePage, importTarget])

    /**
     * "Export table" (tasks.md 7.1-7.3's full-table scope): dispatches to
     * whichever ExportTableAs* bound method matches format, targeting the
     * table/schema the grid is currently browsing — see
     * export.go/ExportTableAsCSV's own doc comment for why the full row set
     * is fetched fresh on the Go side rather than reusing browseResult's
     * single already-fetched page.
     */
    const handleExportTable = useCallback(
        async (format: ExportFormat): Promise<string> => {
            if (!browseContext) {
                return ''
            }
            const {sessionID, schema, table} = browseContext
            if (format === 'json') {
                return ExportTableAsJSON(sessionID, schema, table)
            }
            if (format === 'sql') {
                return ExportTableAsSQLDump(sessionID, schema, table)
            }
            return ExportTableAsCSV(sessionID, schema, table)
        },
        [browseContext],
    )

    /**
     * "Export result" (tasks.md 7.1-7.2's current-query-result scope):
     * dispatches to ExportQueryResultAsCSV/JSON using data this tab already
     * fetched (see exportHelpers.buildQueryResultExportPayload) — never
     * re-runs the query or asks the Go side for a cached last result.
     */
    const handleExportResult = useCallback(async (queryResult: dbengine.QueryResult, format: ExportFormat): Promise<string> => {
        const {columnNames, rows} = buildQueryResultExportPayload(queryResult)
        if (format === 'json') {
            return ExportQueryResultAsJSON(columnNames, rows)
        }
        return ExportQueryResultAsCSV(columnNames, rows)
    }, [])

    const handleCancelQuery = useCallback(async () => {
        const sessionId = sessionIdRef.current
        if (!sessionId) {
            return
        }
        try {
            await CancelQuery(sessionId)
        } catch (err) {
            setErrorMessage(String(err))
        }
    }, [])

    const isRunning = runState === 'connecting' || runState === 'running'

    return (
        <div className="flex flex-col gap-3 rounded border border-ink-800 bg-ink-900/40 p-4">
            <div className="flex items-center justify-between">
                <span className="text-xs uppercase tracking-widest text-ink-400">
                    {activeSubTab === 'query'
                        ? 'Query'
                        : `Data${browseContext ? ` — ${browseContext.schema}.${browseContext.table}` : ''}`}
                </span>
                <span className="font-mono text-xs text-ink-500">{fields.Engine}</span>
            </div>

            {activeSubTab === 'query' ? (
                <>
                    <div className="overflow-hidden rounded border border-ink-700">
                        <Editor
                            height="220px"
                            language={monacoLanguageForEngine(fields.Engine)}
                            theme="vs-dark"
                            value={query}
                            onChange={(value) => setQuery(value ?? '')}
                            onMount={handleEditorMount}
                            options={{
                                minimap: {enabled: false},
                                fontSize: 13,
                                scrollBeyondLastLine: false,
                                automaticLayout: true,
                            }}
                        />
                    </div>

                    <div className="flex items-center gap-3">
                        <button
                            type="button"
                            onClick={() => void handleRunQuery()}
                            disabled={isRunning || query.trim().length === 0}
                            className="rounded bg-brass-600 px-4 py-2 text-sm font-medium text-ink-950 transition-colors hover:bg-brass-500 disabled:cursor-not-allowed disabled:opacity-50"
                        >
                            {runState === 'connecting' ? 'Connecting…' : runState === 'running' ? 'Running…' : 'Run query'}
                        </button>
                        {isRunning && (
                            <button
                                type="button"
                                onClick={() => void handleCancelQuery()}
                                className="rounded border border-red-800 px-4 py-2 text-sm font-medium text-red-400 transition-colors hover:border-red-500 hover:text-red-300"
                            >
                                Cancel
                            </button>
                        )}
                        {runState === 'success' && result && (
                            <span className="text-sm text-emerald-400">
                                {result.RowsAffected > 0
                                    ? `${result.RowsAffected} row(s) affected`
                                    : `${result.Rows?.length ?? 0} row(s) returned`}{' '}
                                in {(result.Duration / 1_000_000).toFixed(1)}ms
                            </span>
                        )}
                        {runState === 'success' && multiStatementResults && (
                            <span className="text-sm text-emerald-400">{summarizeStatementResults(multiStatementResults)}</span>
                        )}
                        {runState === 'error' && errorMessage && <span className="text-sm text-red-400">{errorMessage}</span>}
                    </div>

                    {multiStatementResults ? (
                        <div className="flex flex-col gap-2">
                            {multiStatementResults.map((statementResult, index) => (
                                <StatementResultItem key={index} result={statementResult} onExportResult={handleExportResult} />
                            ))}
                        </div>
                    ) : (
                        result && (
                            <div className="flex flex-col gap-2">
                                <ExportControls
                                    formats={describeExportScope(false).availableFormats}
                                    onExport={(format) => handleExportResult(result, format)}
                                />
                                <ResultsGrid result={result} />
                            </div>
                        )
                    )}
                </>
            ) : (
                <div className="flex flex-col gap-2">
                    {browseError && <p className="text-xs text-red-400">{browseError}</p>}
                    {browseResult && browseContext ? (
                        <>
                            <ExportControls formats={describeExportScope(true).availableFormats} onExport={handleExportTable} />
                            <ResultsGrid
                                result={browseResult}
                                editable={browseContext}
                                onRequestPage={handleRequestBrowsePage}
                                pageOffset={browseOffset}
                                pageLimit={BROWSE_PAGE_SIZE}
                                pageLoading={browsing}
                            />
                        </>
                    ) : (
                        <p className="text-sm text-ink-500">
                            {browsing
                                ? 'Loading table data…'
                                : "Click a table in the left sidebar's schema tree to browse its data here."}
                        </p>
                    )}
                </div>
            )}

            {importTarget && (
                <ImportDialog
                    sessionID={importTarget.sessionId}
                    schema={importTarget.schema}
                    table={importTarget.table}
                    onClose={() => setImportTarget(null)}
                    onImported={handleImported}
                />
            )}

            {createTableSessionId && (
                <CreateTableDialog
                    sessionID={createTableSessionId}
                    schemas={schemaEntries.map((entry) => entry.schema)}
                    onClose={() => setCreateTableSessionId(null)}
                    onCreated={handleTableCreated}
                />
            )}
        </div>
    )
})

/**
 * One entry in the multi-statement results list (spec.md §4.6), shown only
 * when a Run produced 2+ statements (see `collapseStatementResults`).
 * Collapsed by default when the statement succeeded, expanded by default
 * when it failed, so a failing statement's error message is immediately
 * visible without an extra click.
 */
function StatementResultItem({
    result,
    onExportResult,
}: {
    result: dbengine.StatementResult
    onExportResult: (queryResult: dbengine.QueryResult, format: ExportFormat) => Promise<string>
}) {
    return (
        <details
            open={!result.Success}
            className="rounded border border-ink-800 bg-ink-950/40 p-2"
        >
            <summary className="flex cursor-pointer items-center gap-2 text-xs text-ink-300">
                <span
                    className={
                        result.Success
                            ? 'rounded border border-emerald-700 px-1.5 py-0.5 text-emerald-400'
                            : 'rounded border border-red-700 px-1.5 py-0.5 text-red-400'
                    }
                >
                    {result.Success ? 'OK' : 'Failed'}
                </span>
                <span className="truncate font-mono">{result.Statement}</span>
            </summary>
            <div className="mt-2 flex flex-col gap-2">
                {result.Success ? (
                    result.Result ? (
                        <>
                            <ExportControls
                                formats={describeExportScope(false).availableFormats}
                                onExport={(format) => onExportResult(result.Result!, format)}
                            />
                            <ResultsGrid result={result.Result} />
                        </>
                    ) : (
                        <p className="text-xs text-ink-500">Statement succeeded with no rows returned.</p>
                    )
                ) : (
                    <p className="text-xs text-red-400">{result.ErrorMessage}</p>
                )}
            </div>
        </details>
    )
}

export default QueryEditor
