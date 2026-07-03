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
    RunQuery,
} from '../../../wailsjs/go/main/App'
import type {dbengine, main} from '../../../wailsjs/go/models'
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
const BROWSE_ROW_LIMIT = 1000

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

    const [schemaState, setSchemaState] = useState<SchemaState>('idle')
    const [schemaError, setSchemaError] = useState<string | null>(null)
    const [schemaEntries, setSchemaEntries] = useState<SchemaEntry[]>([])

    const [browseResult, setBrowseResult] = useState<dbengine.QueryResult | null>(null)
    const [browseContext, setBrowseContext] = useState<EditableGridContext | null>(null)
    const [browseError, setBrowseError] = useState<string | null>(null)
    const [browsing, setBrowsing] = useState(false)

    const sessionIdRef = useRef<string | null>(null)
    const schemaTablesRef = useRef<dbengine.TableInfo[]>([])
    const editorRef = useRef<monaco.editor.IStandaloneCodeEditor | null>(null)

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

    const handleRunQuery = useCallback(async () => {
        setErrorMessage(null)
        setRunState(sessionIdRef.current ? 'running' : 'connecting')
        try {
            const sessionId = await ensureSession()
            setRunState('running')
            const queryResult = await RunQuery(sessionId, query)
            setBrowseResult(null)
            setBrowseContext(null)
            setResult(queryResult)
            setRunState('success')
        } catch (err) {
            setResult(null)
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
                const queryResult = await BrowseTableRows(sessionId, schema, table.Name, BROWSE_ROW_LIMIT, 0)
                setResult(null)
                setRunState('idle')
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
                {runState === 'error' && errorMessage && <span className="text-sm text-red-400">{errorMessage}</span>}
            </div>

            {browseResult && browseContext ? (
                <ResultsGrid result={browseResult} editable={browseContext} />
            ) : (
                result && <ResultsGrid result={result} />
            )}
        </div>
    )
})

export default QueryEditor
