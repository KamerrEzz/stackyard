import {useCallback, useEffect, useRef, useState} from 'react'
import {
    BuildMongoStructureDiagram,
    BuildSchemaDiagram,
    CloseConnectionSession,
    CloseMongoSession,
    ListMongoDatabases,
    ListSchemasForSession,
    OpenConnection,
    OpenMongoConnection,
    ParseConnectionURL,
} from '../../../wailsjs/go/main/App'
import type {main} from '../../../wailsjs/go/models'
import MermaidDiagram from './MermaidDiagram'

type SchemaDiagramEngine = 'postgres' | 'mysql' | 'mongodb'

interface EngineOption {
    engine: SchemaDiagramEngine
    label: string
    defaultPort: number
    namespaceLabel: string
}

const ENGINE_OPTIONS: EngineOption[] = [
    {engine: 'postgres', label: 'PostgreSQL', defaultPort: 5432, namespaceLabel: 'Schema'},
    {engine: 'mysql', label: 'MySQL', defaultPort: 3306, namespaceLabel: 'Schema'},
    {engine: 'mongodb', label: 'MongoDB', defaultPort: 27017, namespaceLabel: 'Database'},
]

/**
 * Default document sample size the Mongo structure diagram uses when the
 * user hasn't changed the "Sample size" input, mirroring
 * `defaultMongoSampleSize` on the Go side (mongo_session.go) — spec.md
 * §4.11's "N configurable, with a sensible default" requirement.
 */
const DEFAULT_MONGO_SAMPLE_SIZE = 100

/**
 * The exact label spec.md §4.11 requires be visible on every MongoDB
 * structure diagram, distinguishing it at a glance from the relational ER
 * diagram: MongoDB has no real foreign keys, so this diagram is a sampled
 * inference, never an enforced schema.
 */
const MONGO_INFERRED_STRUCTURE_BADGE = 'Inferred structure — not an enforced relationship'

type ConnectState = 'idle' | 'connecting' | 'connected' | 'error'
type DiagramState = 'idle' | 'generating' | 'ready' | 'error'

function isMongoEngine(engine: SchemaDiagramEngine): engine is 'mongodb' {
    return engine === 'mongodb'
}

function namespaceLabelFor(engine: SchemaDiagramEngine): string {
    return ENGINE_OPTIONS.find((option) => option.engine === engine)?.namespaceLabel ?? 'Schema'
}

/**
 * Top-level view for the schema-diagram feature (spec.md §4.11, tasks.md
 * 4.5/5.6): connects to a Postgres/MySQL/MongoDB database independently of
 * the main DB Client's tabs (a separate session, not a shared one — this
 * view shares no state with `db-client/QueryEditor.tsx`), lets the user pick
 * which schema (relational) or database (MongoDB) to diagram, and renders it
 * via `MermaidDiagram`. Generation only ever happens on an explicit user
 * action — connecting, changing the namespace, or clicking "Regenerate" —
 * never automatically on a timer or file watch (spec.md §4.11's explicit
 * "not a live/auto-updating view" requirement).
 *
 * PostgreSQL/MySQL render a live `erDiagram` from real tables/columns/
 * foreign keys (`BuildSchemaDiagram`). MongoDB has no foreign keys to
 * introspect at all, so its diagram instead samples `sampleSize` documents
 * per collection and infers a shape from them (`BuildMongoStructureDiagram`,
 * tasks.md 5.6) — `MONGO_INFERRED_STRUCTURE_BADGE` is passed to
 * `MermaidDiagram` specifically so this distinction is visible on-screen
 * for every Mongo diagram, not just documented here.
 */
function SchemaDiagramView() {
    const [pasteValue, setPasteValue] = useState('')
    const [parseError, setParseError] = useState<string | null>(null)

    const [engine, setEngine] = useState<SchemaDiagramEngine>('postgres')
    const [host, setHost] = useState('')
    const [port, setPort] = useState('')
    const [username, setUsername] = useState('')
    const [password, setPassword] = useState('')
    const [database, setDatabase] = useState('')
    const [sampleSize, setSampleSize] = useState(String(DEFAULT_MONGO_SAMPLE_SIZE))

    const [connectState, setConnectState] = useState<ConnectState>('idle')
    const [connectError, setConnectError] = useState<string | null>(null)

    const [namespaces, setNamespaces] = useState<string[]>([])
    const [selectedNamespace, setSelectedNamespace] = useState('')

    const [diagramState, setDiagramState] = useState<DiagramState>('idle')
    const [diagramError, setDiagramError] = useState<string | null>(null)
    const [mermaidText, setMermaidText] = useState('')

    const sessionIdRef = useRef<string | null>(null)
    const sessionEngineRef = useRef<SchemaDiagramEngine | null>(null)

    const closeSession = useCallback((sessionId: string, sessionEngine: SchemaDiagramEngine) => {
        if (isMongoEngine(sessionEngine)) {
            void CloseMongoSession(sessionId)
        } else {
            void CloseConnectionSession(sessionId)
        }
    }, [])

    useEffect(() => {
        return () => {
            const sessionId = sessionIdRef.current
            const sessionEngine = sessionEngineRef.current
            if (sessionId && sessionEngine) {
                closeSession(sessionId, sessionEngine)
            }
        }
    }, [closeSession])

    const applyParsedFields = useCallback((fields: main.ConnectionFormFields) => {
        if (fields.Engine !== 'postgres' && fields.Engine !== 'mysql' && fields.Engine !== 'mongodb') {
            setParseError(`Schema diagrams support PostgreSQL, MySQL, and MongoDB only, got "${fields.Engine}"`)
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

    const generateDiagram = useCallback(
        async (sessionId: string, sessionEngine: SchemaDiagramEngine, namespace: string) => {
            setDiagramState('generating')
            setDiagramError(null)
            try {
                const text = isMongoEngine(sessionEngine)
                    ? await BuildMongoStructureDiagram(sessionId, namespace, Number(sampleSize) || 0)
                    : await BuildSchemaDiagram(sessionId, namespace)
                setMermaidText(text)
                setDiagramState('ready')
            } catch (err) {
                setDiagramError(String(err))
                setDiagramState('error')
            }
        },
        [sampleSize],
    )

    const handleConnect = useCallback(async () => {
        setConnectState('connecting')
        setConnectError(null)
        setNamespaces([])
        setMermaidText('')
        setDiagramState('idle')

        if (sessionIdRef.current && sessionEngineRef.current) {
            closeSession(sessionIdRef.current, sessionEngineRef.current)
            sessionIdRef.current = null
            sessionEngineRef.current = null
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
            const sessionId = isMongoEngine(engine) ? await OpenMongoConnection(fields) : await OpenConnection(fields)
            sessionIdRef.current = sessionId
            sessionEngineRef.current = engine

            const availableNamespaces = isMongoEngine(engine)
                ? await ListMongoDatabases(sessionId)
                : await ListSchemasForSession(sessionId)
            setNamespaces(availableNamespaces)
            setConnectState('connected')

            const firstNamespace = availableNamespaces[0] ?? ''
            setSelectedNamespace(firstNamespace)
            if (firstNamespace) {
                await generateDiagram(sessionId, engine, firstNamespace)
            }
        } catch (err) {
            setConnectState('error')
            setConnectError(String(err))
        }
    }, [closeSession, database, engine, generateDiagram, host, password, port, username])

    const handleNamespaceChange = useCallback(
        async (namespace: string) => {
            setSelectedNamespace(namespace)
            const sessionId = sessionIdRef.current
            const sessionEngine = sessionEngineRef.current
            if (sessionId && sessionEngine && namespace) {
                await generateDiagram(sessionId, sessionEngine, namespace)
            }
        },
        [generateDiagram],
    )

    const handleRegenerate = useCallback(async () => {
        const sessionId = sessionIdRef.current
        const sessionEngine = sessionEngineRef.current
        if (sessionId && sessionEngine && selectedNamespace) {
            await generateDiagram(sessionId, sessionEngine, selectedNamespace)
        }
    }, [generateDiagram, selectedNamespace])

    const isConnected = connectState === 'connected'
    const isMongo = isMongoEngine(engine)

    return (
        <div className="flex flex-col gap-6">
            <div>
                <h1 className="text-xl font-semibold text-ink-100">Schema Diagram</h1>
                <p className="text-sm text-ink-400">
                    Connect to a PostgreSQL, MySQL, or MongoDB database to generate a diagram: a live
                    entity-relationship diagram from real tables/columns/foreign keys for PostgreSQL/MySQL, or an
                    inferred document-structure diagram sampled from real documents for MongoDB.
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
                            onChange={(e) => setEngine(e.target.value as SchemaDiagramEngine)}
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

                    {isMongo && (
                        <div className="flex flex-col gap-1">
                            <label
                                htmlFor="schema-diagram-sample-size"
                                className="text-xs uppercase tracking-widest text-ink-400"
                            >
                                Sample size (per collection)
                            </label>
                            <input
                                id="schema-diagram-sample-size"
                                type="number"
                                min={1}
                                value={sampleSize}
                                onChange={(e) => setSampleSize(e.target.value)}
                                placeholder={String(DEFAULT_MONGO_SAMPLE_SIZE)}
                                className="rounded border border-ink-700 bg-ink-950 px-3 py-2 text-sm text-ink-100 outline-none focus:border-brass-500"
                            />
                        </div>
                    )}
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

                    {isConnected && namespaces.length > 0 && (
                        <div className="flex items-center gap-2">
                            <label
                                htmlFor="schema-diagram-namespace-select"
                                className="text-xs uppercase tracking-widest text-ink-400"
                            >
                                {namespaceLabelFor(engine)}
                            </label>
                            <select
                                id="schema-diagram-namespace-select"
                                value={selectedNamespace}
                                onChange={(e) => void handleNamespaceChange(e.target.value)}
                                className="rounded border border-ink-700 bg-ink-950 px-3 py-2 text-sm text-ink-100 outline-none focus:border-brass-500"
                            >
                                {namespaces.map((namespace) => (
                                    <option key={namespace} value={namespace}>
                                        {namespace}
                                    </option>
                                ))}
                            </select>
                        </div>
                    )}

                    {isConnected && (
                        <button
                            type="button"
                            onClick={() => void handleRegenerate()}
                            disabled={diagramState === 'generating' || !selectedNamespace}
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
                <MermaidDiagram
                    source={mermaidText}
                    schemaName={selectedNamespace}
                    badge={isMongo ? MONGO_INFERRED_STRUCTURE_BADGE : undefined}
                />
            )}

            {isConnected && namespaces.length === 0 && (
                <p className="text-sm text-ink-500">
                    Connected, but no {namespaceLabelFor(engine).toLowerCase()}s were found — create one first, then
                    Regenerate.
                </p>
            )}
        </div>
    )
}

export default SchemaDiagramView
