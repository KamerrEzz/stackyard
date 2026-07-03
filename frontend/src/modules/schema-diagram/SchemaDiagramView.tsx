import {useCallback, useEffect, useRef, useState} from 'react'
import {
    BuildSchemaDiagram,
    CloseConnectionSession,
    ListSchemasForSession,
    OpenConnection,
    ParseConnectionURL,
} from '../../../wailsjs/go/main/App'
import type {main} from '../../../wailsjs/go/models'
import MermaidDiagram from './MermaidDiagram'

type RelationalEngine = 'postgres' | 'mysql'

interface EngineOption {
    engine: RelationalEngine
    label: string
    defaultPort: number
}

const ENGINE_OPTIONS: EngineOption[] = [
    {engine: 'postgres', label: 'PostgreSQL', defaultPort: 5432},
    {engine: 'mysql', label: 'MySQL', defaultPort: 3306},
]

type ConnectState = 'idle' | 'connecting' | 'connected' | 'error'
type DiagramState = 'idle' | 'generating' | 'ready' | 'error'

/**
 * Top-level view for the relational schema-diagram feature (spec.md §4.11,
 * tasks.md 4.5): connects to a Postgres/MySQL database independently of the
 * main DB Client's tabs (a separate `OpenConnection` session, not a shared
 * one — this view shares no state with `db-client/QueryEditor.tsx`, matching
 * tasks.md's note that this feature "shares no code surface" with the rest
 * of Phase 4), lets the user pick which schema/database to diagram, and
 * renders it via `MermaidDiagram`. Generation only ever happens on an
 * explicit user action — connecting, changing the schema, or clicking
 * "Regenerate" — never automatically on a timer or file watch (spec.md
 * §4.11's explicit "not a live/auto-updating view" requirement).
 */
function SchemaDiagramView() {
    const [pasteValue, setPasteValue] = useState('')
    const [parseError, setParseError] = useState<string | null>(null)

    const [engine, setEngine] = useState<RelationalEngine>('postgres')
    const [host, setHost] = useState('')
    const [port, setPort] = useState('')
    const [username, setUsername] = useState('')
    const [password, setPassword] = useState('')
    const [database, setDatabase] = useState('')

    const [connectState, setConnectState] = useState<ConnectState>('idle')
    const [connectError, setConnectError] = useState<string | null>(null)

    const [schemas, setSchemas] = useState<string[]>([])
    const [selectedSchema, setSelectedSchema] = useState('')

    const [diagramState, setDiagramState] = useState<DiagramState>('idle')
    const [diagramError, setDiagramError] = useState<string | null>(null)
    const [mermaidText, setMermaidText] = useState('')

    const sessionIdRef = useRef<string | null>(null)

    useEffect(() => {
        return () => {
            const sessionId = sessionIdRef.current
            if (sessionId) {
                void CloseConnectionSession(sessionId)
            }
        }
    }, [])

    const applyParsedFields = useCallback((fields: main.ConnectionFormFields) => {
        if (fields.Engine !== 'postgres' && fields.Engine !== 'mysql') {
            setParseError(`Schema diagrams support PostgreSQL and MySQL only, got "${fields.Engine}"`)
            return
        }
        setParseError(null)
        setEngine(fields.Engine)
        setHost(fields.Host)
        setPort(fields.Port > 0 ? String(fields.Port) : '')
        setUsername(fields.Username)
        setPassword(fields.Password)
        setDatabase(fields.Database)
    }, [])

    const handlePasteBlur = useCallback(async () => {
        const raw = pasteValue.trim()
        if (!raw) {
            setParseError(null)
            return
        }
        try {
            const fields = await ParseConnectionURL(raw)
            applyParsedFields(fields)
        } catch (err) {
            setParseError(String(err))
        }
    }, [applyParsedFields, pasteValue])

    const generateDiagram = useCallback(async (sessionId: string, schema: string) => {
        setDiagramState('generating')
        setDiagramError(null)
        try {
            const text = await BuildSchemaDiagram(sessionId, schema)
            setMermaidText(text)
            setDiagramState('ready')
        } catch (err) {
            setDiagramError(String(err))
            setDiagramState('error')
        }
    }, [])

    const handleConnect = useCallback(async () => {
        setConnectState('connecting')
        setConnectError(null)
        setSchemas([])
        setMermaidText('')
        setDiagramState('idle')

        if (sessionIdRef.current) {
            void CloseConnectionSession(sessionIdRef.current)
            sessionIdRef.current = null
        }

        const fields: main.ConnectionFormFields = {
            Engine: engine,
            Host: host,
            Port: Number(port) || 0,
            Username: username,
            Password: password,
            Database: database,
            Params: {},
            SavedConnectionID: 0,
        }

        try {
            const sessionId = await OpenConnection(fields)
            sessionIdRef.current = sessionId

            const availableSchemas = await ListSchemasForSession(sessionId)
            setSchemas(availableSchemas)
            setConnectState('connected')

            const firstSchema = availableSchemas[0] ?? ''
            setSelectedSchema(firstSchema)
            if (firstSchema) {
                await generateDiagram(sessionId, firstSchema)
            }
        } catch (err) {
            setConnectState('error')
            setConnectError(String(err))
        }
    }, [database, engine, generateDiagram, host, password, port, username])

    const handleSchemaChange = useCallback(
        async (schema: string) => {
            setSelectedSchema(schema)
            const sessionId = sessionIdRef.current
            if (sessionId && schema) {
                await generateDiagram(sessionId, schema)
            }
        },
        [generateDiagram],
    )

    const handleRegenerate = useCallback(async () => {
        const sessionId = sessionIdRef.current
        if (sessionId && selectedSchema) {
            await generateDiagram(sessionId, selectedSchema)
        }
    }, [generateDiagram, selectedSchema])

    const isConnected = connectState === 'connected'

    return (
        <div className="flex flex-col gap-6">
            <div>
                <h1 className="text-xl font-semibold text-ink-100">Schema Diagram</h1>
                <p className="text-sm text-ink-400">
                    Connect to a PostgreSQL or MySQL database to generate a live entity-relationship diagram from its
                    real tables, columns, and foreign keys.
                </p>
            </div>

            <div className="flex flex-col gap-3 rounded border border-ink-800 bg-ink-900/40 p-4">
                <div className="flex flex-col gap-1">
                    <label htmlFor="schema-diagram-paste-url" className="text-xs uppercase tracking-widest text-ink-400">
                        Paste connection URL
                    </label>
                    <input
                        id="schema-diagram-paste-url"
                        type="text"
                        value={pasteValue}
                        onChange={(e) => setPasteValue(e.target.value)}
                        onBlur={() => void handlePasteBlur()}
                        placeholder="postgres://user:password@host:5432/dbname"
                        className="rounded border border-ink-700 bg-ink-950 px-3 py-2 font-mono text-sm text-ink-100 outline-none focus:border-brass-500"
                    />
                    {parseError && <p className="text-xs text-red-400">{parseError}</p>}
                </div>

                <div className="grid grid-cols-2 gap-3 sm:grid-cols-3">
                    <div className="flex flex-col gap-1">
                        <label htmlFor="schema-diagram-engine" className="text-xs uppercase tracking-widest text-ink-400">
                            Engine
                        </label>
                        <select
                            id="schema-diagram-engine"
                            value={engine}
                            onChange={(e) => setEngine(e.target.value as RelationalEngine)}
                            className="rounded border border-ink-700 bg-ink-950 px-3 py-2 text-sm text-ink-100 outline-none focus:border-brass-500"
                        >
                            {ENGINE_OPTIONS.map((option) => (
                                <option key={option.engine} value={option.engine}>
                                    {option.label}
                                </option>
                            ))}
                        </select>
                    </div>

                    <div className="flex flex-col gap-1">
                        <label htmlFor="schema-diagram-host" className="text-xs uppercase tracking-widest text-ink-400">
                            Host
                        </label>
                        <input
                            id="schema-diagram-host"
                            type="text"
                            value={host}
                            onChange={(e) => setHost(e.target.value)}
                            placeholder="localhost"
                            className="rounded border border-ink-700 bg-ink-950 px-3 py-2 text-sm text-ink-100 outline-none focus:border-brass-500"
                        />
                    </div>

                    <div className="flex flex-col gap-1">
                        <label htmlFor="schema-diagram-port" className="text-xs uppercase tracking-widest text-ink-400">
                            Port
                        </label>
                        <input
                            id="schema-diagram-port"
                            type="number"
                            value={port}
                            onChange={(e) => setPort(e.target.value)}
                            placeholder={String(ENGINE_OPTIONS.find((o) => o.engine === engine)?.defaultPort ?? '')}
                            className="rounded border border-ink-700 bg-ink-950 px-3 py-2 text-sm text-ink-100 outline-none focus:border-brass-500"
                        />
                    </div>

                    <div className="flex flex-col gap-1">
                        <label htmlFor="schema-diagram-username" className="text-xs uppercase tracking-widest text-ink-400">
                            Username
                        </label>
                        <input
                            id="schema-diagram-username"
                            type="text"
                            value={username}
                            onChange={(e) => setUsername(e.target.value)}
                            className="rounded border border-ink-700 bg-ink-950 px-3 py-2 text-sm text-ink-100 outline-none focus:border-brass-500"
                        />
                    </div>

                    <div className="flex flex-col gap-1">
                        <label htmlFor="schema-diagram-password" className="text-xs uppercase tracking-widest text-ink-400">
                            Password
                        </label>
                        <input
                            id="schema-diagram-password"
                            type="password"
                            value={password}
                            onChange={(e) => setPassword(e.target.value)}
                            className="rounded border border-ink-700 bg-ink-950 px-3 py-2 text-sm text-ink-100 outline-none focus:border-brass-500"
                        />
                    </div>

                    <div className="flex flex-col gap-1">
                        <label htmlFor="schema-diagram-database" className="text-xs uppercase tracking-widest text-ink-400">
                            Database
                        </label>
                        <input
                            id="schema-diagram-database"
                            type="text"
                            value={database}
                            onChange={(e) => setDatabase(e.target.value)}
                            className="rounded border border-ink-700 bg-ink-950 px-3 py-2 text-sm text-ink-100 outline-none focus:border-brass-500"
                        />
                    </div>
                </div>

                <div className="flex items-center gap-3 pt-1">
                    <button
                        type="button"
                        onClick={() => void handleConnect()}
                        disabled={connectState === 'connecting' || host.trim().length === 0}
                        className="rounded bg-brass-600 px-4 py-2 text-sm font-medium text-ink-950 transition-colors hover:bg-brass-500 disabled:cursor-not-allowed disabled:opacity-50"
                    >
                        {connectState === 'connecting' ? 'Connecting…' : 'Connect'}
                    </button>

                    {isConnected && schemas.length > 0 && (
                        <div className="flex items-center gap-2">
                            <label htmlFor="schema-diagram-schema-select" className="text-xs uppercase tracking-widest text-ink-400">
                                Schema
                            </label>
                            <select
                                id="schema-diagram-schema-select"
                                value={selectedSchema}
                                onChange={(e) => void handleSchemaChange(e.target.value)}
                                className="rounded border border-ink-700 bg-ink-950 px-3 py-2 text-sm text-ink-100 outline-none focus:border-brass-500"
                            >
                                {schemas.map((schema) => (
                                    <option key={schema} value={schema}>
                                        {schema}
                                    </option>
                                ))}
                            </select>
                        </div>
                    )}

                    {isConnected && (
                        <button
                            type="button"
                            onClick={() => void handleRegenerate()}
                            disabled={diagramState === 'generating' || !selectedSchema}
                            className="rounded border border-ink-700 px-4 py-2 text-sm font-medium text-ink-200 transition-colors hover:border-brass-500 hover:text-brass-400 disabled:cursor-not-allowed disabled:opacity-50"
                        >
                            {diagramState === 'generating' ? 'Regenerating…' : 'Regenerate'}
                        </button>
                    )}

                    {connectState === 'error' && connectError && <p className="text-sm text-red-400">{connectError}</p>}
                </div>
            </div>

            {diagramState === 'error' && diagramError && (
                <p className="text-sm text-red-400">Failed to generate diagram: {diagramError}</p>
            )}

            {diagramState === 'ready' && mermaidText && (
                <MermaidDiagram source={mermaidText} schemaName={selectedSchema} />
            )}

            {isConnected && schemas.length === 0 && (
                <p className="text-sm text-ink-500">
                    Connected, but no user schemas were found — create a table first, then Regenerate.
                </p>
            )}
        </div>
    )
}

export default SchemaDiagramView
