import Editor from '@monaco-editor/react'
import {useCallback, useEffect, useRef, useState} from 'react'
import '../../lib/monacoSetup'
import {CancelQuery, CloseConnectionSession, OpenConnection, RunQuery} from '../../../wailsjs/go/main/App'
import type {dbengine, main} from '../../../wailsjs/go/models'
import ResultsGrid from './ResultsGrid'

interface QueryEditorProps {
    fields: main.ConnectionFormFields
}

type RunState = 'idle' | 'connecting' | 'running' | 'success' | 'error'

const DEFAULT_QUERY = '-- Write a query and click "Run query"\nSELECT 1;'

function monacoLanguageForEngine(engine: main.ConnectionFormFields['Engine']): string {
    switch (engine) {
        case 'postgres':
        case 'mysql':
            return 'sql'
        default:
            return 'plaintext'
    }
}

function QueryEditor({fields}: QueryEditorProps) {
    const [query, setQuery] = useState(DEFAULT_QUERY)
    const [runState, setRunState] = useState<RunState>('idle')
    const [errorMessage, setErrorMessage] = useState<string | null>(null)
    const [result, setResult] = useState<dbengine.QueryResult | null>(null)

    const sessionIdRef = useRef<string | null>(null)

    useEffect(() => {
        return () => {
            const sessionId = sessionIdRef.current
            if (sessionId) {
                void CloseConnectionSession(sessionId)
            }
        }
    }, [])

    const ensureSession = useCallback(async () => {
        if (sessionIdRef.current) {
            return sessionIdRef.current
        }
        const sessionId = await OpenConnection(fields)
        sessionIdRef.current = sessionId
        return sessionId
    }, [fields])

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
                <span className="font-mono text-xs text-ink-500">{fields.Engine}</span>
            </div>

            <div className="overflow-hidden rounded border border-ink-700">
                <Editor
                    height="220px"
                    language={monacoLanguageForEngine(fields.Engine)}
                    theme="vs-dark"
                    value={query}
                    onChange={(value) => setQuery(value ?? '')}
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
