import Editor from '@monaco-editor/react'
import type * as monaco from 'monaco-editor'
import {forwardRef, useCallback, useEffect, useImperativeHandle, useRef, useState} from 'react'
import '../../lib/monacoSetup'
import {
    BrowseTableRows,
    CancelQuery,
    CloseConnectionSession,
    ListSchemasForSession,
    ListTablesForSession,
    OpenConnection,
    RunMultiStatementQuery,
} from '../../../wailsjs/go/main/App'
import type {dbengine, main} from '../../../wailsjs/go/models'
import {collapseStatementResults, summarizeStatementResults} from './multiStatementHelpers'
import {RESULTS_PAGE_SIZE} from './resultsGridHelpers'
import ResultsGrid, {type EditableGridContext} from './ResultsGrid'
import {registerModelSchemaProvider, unregisterModelSchemaProvider} from './schemaCompletion'
import {ensureSqlCompletionProviderRegistered} from './schemaCompletionProvider'

interface QueryEditorProps {
    fields: main.ConnectionFormFields
    initialQuery?: string
}

/**
 * Imperative surface a parent can use to command an already-mounted tab's
 * editor from outside (tasks.md 4.7): checking whether the tab has unsaved
 * work, and replacing its query text (e.g. loading a snippet's body into
 * the current tab) without remounting the tab or touching its connection.
 */
export interface QueryEditorHandle {
    isDirty(): boolean
    loadQuery(text: string): void
}

type RunState = 'idle' | 'connecting' | 'running' | 'success' | 'error'
type SchemaState = 'idle' | 'loading' | 'loaded' | 'error'

interface SchemaEntry {
    schema: string
    tables: dbengine.TableInfo[]
}

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

const QueryEditor = forwardRef<QueryEditorHandle, QueryEditorProps>(function QueryEditor({fields, initialQuery}, ref) {
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

    useImperativeHandle(
        ref,
        () => ({
            isDirty: () => query !== baselineQueryRef.current,
            loadQuery: (text: string) => {
                baselineQueryRef.current = text
                setQuery(text)
            },
        }),
        [query],
    )

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
    }, [ensureSession, query])

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
        [ensureSession],
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
                <h2 className="text-xs uppercase tracking-widest text-ink-400">Query editor</h2>
                <div className="flex items-center gap-3">
                    {schemaState === 'error' && schemaError && (
                        <span className="text-xs text-red-400" title={schemaError}>
                            Schema unavailable
                        </span>
                    )}
                    <button
                        type="button"
                        onClick={() => void handleRefreshSchema()}
                        disabled={schemaState === 'loading'}
                        className="rounded border border-ink-700 px-2 py-1 text-xs text-ink-200 transition-colors hover:border-brass-500 hover:text-brass-400 disabled:cursor-not-allowed disabled:opacity-50"
                    >
                        {schemaState === 'loading' ? 'Loading schema…' : 'Refresh schema'}
                    </button>
                    <span className="font-mono text-xs text-ink-500">{fields.Engine}</span>
                </div>
            </div>

            {schemaEntries.length > 0 && (
                <div className="flex flex-col gap-1 rounded border border-ink-800 bg-ink-950/40 p-2">
                    <span className="text-[10px] uppercase tracking-widest text-ink-500">Tables</span>
                    <div className="flex max-h-32 flex-col gap-1 overflow-auto">
                        {schemaEntries.map((entry) =>
                            entry.tables.map((table) => (
                                <div
                                    key={`${entry.schema}.${table.Name}`}
                                    className="flex items-center justify-between gap-2 text-xs text-ink-300"
                                >
                                    <span className="font-mono">
                                        {entry.schema}.{table.Name}
                                    </span>
                                    <button
                                        type="button"
                                        onClick={() => void handleBrowseTable(entry.schema, table)}
                                        disabled={browsing}
                                        className="rounded border border-ink-700 px-2 py-0.5 text-[10px] text-ink-200 transition-colors hover:border-brass-500 hover:text-brass-400 disabled:cursor-not-allowed disabled:opacity-50"
                                    >
                                        Browse
                                    </button>
                                </div>
                            )),
                        )}
                    </div>
                    {browseError && <span className="text-xs text-red-400">{browseError}</span>}
                </div>
            )}

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

            {browseResult && browseContext ? (
                <ResultsGrid
                    result={browseResult}
                    editable={browseContext}
                    onRequestPage={handleRequestBrowsePage}
                    pageOffset={browseOffset}
                    pageLimit={BROWSE_PAGE_SIZE}
                    pageLoading={browsing}
                />
            ) : multiStatementResults ? (
                <div className="flex flex-col gap-2">
                    {multiStatementResults.map((statementResult, index) => (
                        <StatementResultItem key={index} result={statementResult} />
                    ))}
                </div>
            ) : (
                result && <ResultsGrid result={result} />
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
function StatementResultItem({result}: {result: dbengine.StatementResult}) {
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
            <div className="mt-2">
                {result.Success ? (
                    result.Result ? (
                        <ResultsGrid result={result.Result} />
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
