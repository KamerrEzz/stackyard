import Editor from '@monaco-editor/react'
import type * as monaco from 'monaco-editor'
import {useCallback, useEffect, useRef, useState} from 'react'
import '../../lib/monacoSetup'
import {
    CancelQuery,
    CloseConnectionSession,
    ListSchemasForSession,
    ListTablesForSession,
    OpenConnection,
    RunQuery,
} from '../../../wailsjs/go/main/App'
import type {dbengine, main} from '../../../wailsjs/go/models'
import ResultsGrid from './ResultsGrid'
import {registerModelSchemaProvider, unregisterModelSchemaProvider} from './schemaCompletion'
import {ensureSqlCompletionProviderRegistered} from './schemaCompletionProvider'

interface QueryEditorProps {
    fields: main.ConnectionFormFields
    initialQuery?: string
}

type RunState = 'idle' | 'connecting' | 'running' | 'success' | 'error'
type SchemaState = 'idle' | 'loading' | 'loaded' | 'error'

const DEFAULT_QUERY = '-- Write a query and click "Run query"\nSELECT 1;'

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

function QueryEditor({fields, initialQuery}: QueryEditorProps) {
    const [query, setQuery] = useState(initialQuery ?? DEFAULT_QUERY)
    const [runState, setRunState] = useState<RunState>('idle')
    const [errorMessage, setErrorMessage] = useState<string | null>(null)
    const [result, setResult] = useState<dbengine.QueryResult | null>(null)

    const [schemaState, setSchemaState] = useState<SchemaState>('idle')
    const [schemaError, setSchemaError] = useState<string | null>(null)

    const sessionIdRef = useRef<string | null>(null)
    const schemaTablesRef = useRef<dbengine.TableInfo[]>([])
    const editorRef = useRef<monaco.editor.IStandaloneCodeEditor | null>(null)

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
            setResult(queryResult)
            setRunState('success')
        } catch (err) {
            setResult(null)
            setErrorMessage(String(err))
            setRunState('error')
        }
    }, [ensureSession, query])

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

            {result && <ResultsGrid result={result} />}
        </div>
    )
}

export default QueryEditor
