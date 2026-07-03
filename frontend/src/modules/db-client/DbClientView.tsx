import {useCallback, useEffect, useState} from 'react'
import {
    ConnectUsingSavedConnection,
    DeleteConnection,
    ListConnections,
    ParseConnectionURL,
    SaveConnection,
    TestConnection,
} from '../../../wailsjs/go/main/App'
import type {main, storage} from '../../../wailsjs/go/models'
import QueryEditor from './QueryEditor'
import QueryHistoryPanel from './QueryHistoryPanel'
import SnippetsPanel from './SnippetsPanel'
import TabBar from './TabBar'
import {closeTab, openTab} from './tabState'

type Engine = 'postgres' | 'mysql' | 'mongodb' | 'redis'

interface EngineOption {
    engine: Engine
    label: string
    defaultPort: number
}

const ENGINE_OPTIONS: EngineOption[] = [
    {engine: 'postgres', label: 'PostgreSQL', defaultPort: 5432},
    {engine: 'mysql', label: 'MySQL', defaultPort: 3306},
    {engine: 'mongodb', label: 'MongoDB', defaultPort: 27017},
    {engine: 'redis', label: 'Redis', defaultPort: 6379},
]

interface ParamRow {
    key: string
    value: string
}

interface DbClientTab {
    id: string
    label: string
    fields: main.ConnectionFormFields
    initialQuery?: string
}

function labelForFields(fields: main.ConnectionFormFields): string {
    return `${fields.Engine}@${fields.Host}:${fields.Port}`
}

let tabIdSequence = 0

function nextTabId(): string {
    tabIdSequence += 1
    return `tab-${Date.now()}-${tabIdSequence}`
}

function paramsToRows(params: Record<string, string> | undefined): ParamRow[] {
    if (!params) {
        return []
    }
    return Object.entries(params).map(([key, value]) => ({key, value}))
}

function rowsToParams(rows: ParamRow[]): Record<string, string> {
    const params: Record<string, string> = {}
    for (const row of rows) {
        const key = row.key.trim()
        if (key) {
            params[key] = row.value
        }
    }
    return params
}

type TestState = 'idle' | 'testing' | 'success' | 'error'

function DbClientView() {
    const [pasteValue, setPasteValue] = useState('')
    const [parseError, setParseError] = useState<string | null>(null)

    const [engine, setEngine] = useState<Engine>('postgres')
    const [host, setHost] = useState('')
    const [port, setPort] = useState('')
    const [username, setUsername] = useState('')
    const [password, setPassword] = useState('')
    const [database, setDatabase] = useState('')
    const [paramRows, setParamRows] = useState<ParamRow[]>([])

    const [testState, setTestState] = useState<TestState>('idle')
    const [testMessage, setTestMessage] = useState<string | null>(null)

    const [savedConnections, setSavedConnections] = useState<storage.Connection[]>([])
    const [savedConnectionsError, setSavedConnectionsError] = useState<string | null>(null)
    const [saveName, setSaveName] = useState('')
    const [saveState, setSaveState] = useState<TestState>('idle')
    const [saveMessage, setSaveMessage] = useState<string | null>(null)

    const [tabs, setTabs] = useState<DbClientTab[]>([])
    const [activeTabId, setActiveTabId] = useState<string | null>(null)

    const addTab = useCallback((fields: main.ConnectionFormFields, label: string, initialQuery?: string) => {
        const tab: DbClientTab = {id: nextTabId(), label, fields, initialQuery}
        setTabs((prev) => openTab(prev, tab).tabs)
        setActiveTabId(tab.id)
    }, [])

    const applyParsedFields = useCallback((fields: main.ConnectionFormFields) => {
        setEngine(fields.Engine as Engine)
        setHost(fields.Host)
        setPort(fields.Port > 0 ? String(fields.Port) : '')
        setUsername(fields.Username)
        setPassword(fields.Password)
        setDatabase(fields.Database)
        setParamRows(paramsToRows(fields.Params))
    }, [])

    const handlePasteBlur = useCallback(async () => {
        const raw = pasteValue.trim()
        if (!raw) {
            setParseError(null)
            return
        }
        try {
            const fields = await ParseConnectionURL(raw)
            setParseError(null)
            applyParsedFields(fields)
        } catch (err) {
            setParseError(String(err))
        }
    }, [applyParsedFields, pasteValue])

    const handleAddParamRow = useCallback(() => {
        setParamRows((prev) => [...prev, {key: '', value: ''}])
    }, [])

    const handleParamKeyChange = useCallback((index: number, key: string) => {
        setParamRows((prev) => prev.map((row, i) => (i === index ? {...row, key} : row)))
    }, [])

    const handleParamValueChange = useCallback((index: number, value: string) => {
        setParamRows((prev) => prev.map((row, i) => (i === index ? {...row, value} : row)))
    }, [])

    const handleRemoveParamRow = useCallback((index: number) => {
        setParamRows((prev) => prev.filter((_, i) => i !== index))
    }, [])

    const refreshSavedConnections = useCallback(async () => {
        try {
            const connections = await ListConnections()
            setSavedConnectionsError(null)
            setSavedConnections(connections)
        } catch (err) {
            setSavedConnectionsError(String(err))
        }
    }, [])

    useEffect(() => {
        void refreshSavedConnections()
    }, [refreshSavedConnections])

    const handleSaveConnection = useCallback(async () => {
        setSaveState('testing')
        setSaveMessage(null)
        try {
            await SaveConnection(
                {
                    Engine: engine,
                    Host: host,
                    Port: Number(port) || 0,
                    Username: username,
                    Password: password,
                    Database: database,
                    Params: rowsToParams(paramRows),
                    SavedConnectionID: 0,
                },
                saveName.trim(),
            )
            setSaveState('success')
            setSaveMessage('Connection saved.')
            setSaveName('')
            await refreshSavedConnections()
        } catch (err) {
            setSaveState('error')
            setSaveMessage(String(err))
        }
    }, [database, engine, host, paramRows, password, port, refreshSavedConnections, saveName, username])

    const handleLoadConnection = useCallback(
        async (id: number, name: string) => {
            try {
                const fields = await ConnectUsingSavedConnection(id)
                setParseError(null)
                setPasteValue('')
                applyParsedFields(fields)
                addTab(fields, name)
                await refreshSavedConnections()
            } catch (err) {
                setSavedConnectionsError(String(err))
            }
        },
        [addTab, applyParsedFields, refreshSavedConnections],
    )

    const handleReplayEntry = useCallback(
        async (entry: storage.QueryHistoryEntry) => {
            try {
                const fields = await ConnectUsingSavedConnection(entry.ConnectionID)
                setParseError(null)
                setPasteValue('')
                applyParsedFields(fields)
                const savedConn = savedConnections.find((conn) => conn.ID === entry.ConnectionID)
                addTab(fields, savedConn ? savedConn.Name : labelForFields(fields), entry.QueryText)
                await refreshSavedConnections()
            } catch (err) {
                setSavedConnectionsError(String(err))
            }
        },
        [addTab, applyParsedFields, refreshSavedConnections, savedConnections],
    )

    const handleDeleteConnection = useCallback(
        async (id: number, name: string) => {
            if (!window.confirm(`Delete saved connection "${name}"? This cannot be undone.`)) {
                return
            }
            try {
                await DeleteConnection(id)
                await refreshSavedConnections()
            } catch (err) {
                setSavedConnectionsError(String(err))
            }
        },
        [refreshSavedConnections],
    )

    const handleTestConnection = useCallback(async () => {
        setTestState('testing')
        setTestMessage(null)
        const fields: main.ConnectionFormFields = {
            Engine: engine,
            Host: host,
            Port: Number(port) || 0,
            Username: username,
            Password: password,
            Database: database,
            Params: rowsToParams(paramRows),
            SavedConnectionID: 0,
        }
        try {
            await TestConnection(fields)
            setTestState('success')
            setTestMessage('Connected successfully.')
            addTab(fields, labelForFields(fields))
        } catch (err) {
            setTestState('error')
            setTestMessage(String(err))
        }
    }, [addTab, database, engine, host, paramRows, password, port, username])

    const handleNewTab = useCallback(() => {
        setActiveTabId(null)
    }, [])

    const handleCloseTab = useCallback(
        (id: string) => {
            const result = closeTab(tabs, activeTabId, id)
            setTabs(result.tabs)
            setActiveTabId(result.activeTabId)
        },
        [activeTabId, tabs],
    )

    return (
        <div className="flex flex-col gap-6">
            <div>
                <h1 className="text-xl font-semibold text-ink-100">DB Client</h1>
                <p className="text-sm text-ink-400">
                    Paste a connection string to autofill the form below, or fill in the fields manually.
                </p>
            </div>

            <div className="flex flex-col gap-3 rounded border border-ink-800 bg-ink-900/40 p-4">
                <div className="flex flex-col gap-1">
                    <label htmlFor="paste-connection-url" className="text-xs uppercase tracking-widest text-ink-400">
                        Paste connection URL
                    </label>
                    <input
                        id="paste-connection-url"
                        type="text"
                        value={pasteValue}
                        onChange={(e) => setPasteValue(e.target.value)}
                        onBlur={() => void handlePasteBlur()}
                        placeholder="postgres://user:password@host:5432/dbname?sslmode=require"
                        className="rounded border border-ink-700 bg-ink-950 px-3 py-2 font-mono text-sm text-ink-100 outline-none focus:border-brass-500"
                    />
                    {parseError && <p className="text-xs text-red-400">{parseError}</p>}
                </div>

                <div className="grid grid-cols-2 gap-3 sm:grid-cols-3">
                    <div className="flex flex-col gap-1">
                        <label htmlFor="conn-engine" className="text-xs uppercase tracking-widest text-ink-400">
                            Engine
                        </label>
                        <select
                            id="conn-engine"
                            value={engine}
                            onChange={(e) => setEngine(e.target.value as Engine)}
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
                        <label htmlFor="conn-host" className="text-xs uppercase tracking-widest text-ink-400">
                            Host
                        </label>
                        <input
                            id="conn-host"
                            type="text"
                            value={host}
                            onChange={(e) => setHost(e.target.value)}
                            placeholder="localhost"
                            className="rounded border border-ink-700 bg-ink-950 px-3 py-2 text-sm text-ink-100 outline-none focus:border-brass-500"
                        />
                    </div>

                    <div className="flex flex-col gap-1">
                        <label htmlFor="conn-port" className="text-xs uppercase tracking-widest text-ink-400">
                            Port
                        </label>
                        <input
                            id="conn-port"
                            type="number"
                            value={port}
                            onChange={(e) => setPort(e.target.value)}
                            placeholder={String(ENGINE_OPTIONS.find((o) => o.engine === engine)?.defaultPort ?? '')}
                            className="rounded border border-ink-700 bg-ink-950 px-3 py-2 text-sm text-ink-100 outline-none focus:border-brass-500"
                        />
                    </div>

                    <div className="flex flex-col gap-1">
                        <label htmlFor="conn-username" className="text-xs uppercase tracking-widest text-ink-400">
                            Username
                        </label>
                        <input
                            id="conn-username"
                            type="text"
                            value={username}
                            onChange={(e) => setUsername(e.target.value)}
                            className="rounded border border-ink-700 bg-ink-950 px-3 py-2 text-sm text-ink-100 outline-none focus:border-brass-500"
                        />
                    </div>

                    <div className="flex flex-col gap-1">
                        <label htmlFor="conn-password" className="text-xs uppercase tracking-widest text-ink-400">
                            Password
                        </label>
                        <input
                            id="conn-password"
                            type="password"
                            value={password}
                            onChange={(e) => setPassword(e.target.value)}
                            className="rounded border border-ink-700 bg-ink-950 px-3 py-2 text-sm text-ink-100 outline-none focus:border-brass-500"
                        />
                    </div>

                    <div className="flex flex-col gap-1">
                        <label htmlFor="conn-database" className="text-xs uppercase tracking-widest text-ink-400">
                            Database
                        </label>
                        <input
                            id="conn-database"
                            type="text"
                            value={database}
                            onChange={(e) => setDatabase(e.target.value)}
                            className="rounded border border-ink-700 bg-ink-950 px-3 py-2 text-sm text-ink-100 outline-none focus:border-brass-500"
                        />
                    </div>
                </div>

                <div className="flex flex-col gap-2">
                    <div className="flex items-center justify-between">
                        <span className="text-xs uppercase tracking-widest text-ink-400">Query params</span>
                        <button
                            type="button"
                            onClick={handleAddParamRow}
                            className="rounded border border-ink-700 px-2 py-1 text-xs text-ink-200 hover:border-brass-500 hover:text-brass-400"
                        >
                            + Add param
                        </button>
                    </div>
                    {paramRows.length === 0 && <p className="text-xs text-ink-500">No query params.</p>}
                    {paramRows.map((row, index) => (
                        <div key={index} className="flex items-center gap-2">
                            <input
                                type="text"
                                value={row.key}
                                onChange={(e) => handleParamKeyChange(index, e.target.value)}
                                placeholder="sslmode"
                                className="w-1/3 rounded border border-ink-700 bg-ink-950 px-2 py-1 font-mono text-xs text-ink-100 outline-none focus:border-brass-500"
                            />
                            <input
                                type="text"
                                value={row.value}
                                onChange={(e) => handleParamValueChange(index, e.target.value)}
                                placeholder="require"
                                className="flex-1 rounded border border-ink-700 bg-ink-950 px-2 py-1 font-mono text-xs text-ink-100 outline-none focus:border-brass-500"
                            />
                            <button
                                type="button"
                                onClick={() => handleRemoveParamRow(index)}
                                className="rounded border border-red-800 px-2 py-1 text-xs text-red-400 hover:border-red-500 hover:text-red-300"
                            >
                                Remove
                            </button>
                        </div>
                    ))}
                </div>

                <div className="flex items-center gap-3 pt-1">
                    <button
                        type="button"
                        onClick={() => void handleTestConnection()}
                        disabled={testState === 'testing' || host.trim().length === 0}
                        className="rounded bg-brass-600 px-4 py-2 text-sm font-medium text-ink-950 transition-colors hover:bg-brass-500 disabled:cursor-not-allowed disabled:opacity-50"
                    >
                        {testState === 'testing' ? 'Testing…' : 'Test connection'}
                    </button>
                    {testState === 'success' && <p className="text-sm text-emerald-400">{testMessage}</p>}
                    {testState === 'error' && <p className="text-sm text-red-400">{testMessage}</p>}
                </div>

                <div className="flex items-center gap-3 border-t border-ink-800 pt-3">
                    <input
                        type="text"
                        value={saveName}
                        onChange={(e) => setSaveName(e.target.value)}
                        placeholder="Name this connection"
                        className="rounded border border-ink-700 bg-ink-950 px-3 py-2 text-sm text-ink-100 outline-none focus:border-brass-500"
                    />
                    <button
                        type="button"
                        onClick={() => void handleSaveConnection()}
                        disabled={saveState === 'testing' || host.trim().length === 0 || saveName.trim().length === 0}
                        className="rounded border border-ink-700 px-4 py-2 text-sm font-medium text-ink-200 transition-colors hover:border-brass-500 hover:text-brass-400 disabled:cursor-not-allowed disabled:opacity-50"
                    >
                        {saveState === 'testing' ? 'Saving…' : 'Save connection'}
                    </button>
                    {saveState === 'success' && <p className="text-sm text-emerald-400">{saveMessage}</p>}
                    {saveState === 'error' && <p className="text-sm text-red-400">{saveMessage}</p>}
                </div>
            </div>

            <div className="flex flex-col gap-3 rounded border border-ink-800 bg-ink-900/40 p-4">
                <h2 className="text-xs uppercase tracking-widest text-ink-400">Saved connections</h2>
                {savedConnectionsError && <p className="text-xs text-red-400">{savedConnectionsError}</p>}
                {savedConnections.length === 0 && !savedConnectionsError && (
                    <p className="text-sm text-ink-500">No saved connections yet.</p>
                )}
                {savedConnections.map((conn) => (
                    <div
                        key={conn.ID}
                        className="flex items-center justify-between gap-3 rounded border border-ink-800 bg-ink-950/60 px-3 py-2"
                    >
                        <div className="flex flex-col">
                            <span className="text-sm font-medium text-ink-100">{conn.Name}</span>
                            <span className="font-mono text-xs text-ink-400">
                                {conn.Engine}://{conn.Host}:{conn.Port}
                                {conn.Database ? `/${conn.Database}` : ''}
                            </span>
                            {conn.LastUsedAt && (
                                <span className="text-xs text-ink-500">Last used {conn.LastUsedAt}</span>
                            )}
                        </div>
                        <div className="flex items-center gap-2">
                            <button
                                type="button"
                                onClick={() => void handleLoadConnection(conn.ID, conn.Name)}
                                className="rounded border border-ink-700 px-3 py-1 text-xs text-ink-200 hover:border-brass-500 hover:text-brass-400"
                            >
                                Load
                            </button>
                            <button
                                type="button"
                                onClick={() => void handleDeleteConnection(conn.ID, conn.Name)}
                                className="rounded border border-red-800 px-3 py-1 text-xs text-red-400 hover:border-red-500 hover:text-red-300"
                            >
                                Delete
                            </button>
                        </div>
                    </div>
                ))}
            </div>

            <SnippetsPanel savedConnections={savedConnections} />

            <QueryHistoryPanel savedConnections={savedConnections} onReplay={(entry) => void handleReplayEntry(entry)} />

            {tabs.length > 0 && (
                <div className="flex flex-col gap-3">
                    <TabBar
                        tabs={tabs}
                        activeTabId={activeTabId}
                        onSelect={setActiveTabId}
                        onClose={handleCloseTab}
                        onNewTab={handleNewTab}
                    />
                    {tabs.map((tab) => (
                        <div key={tab.id} className={tab.id === activeTabId ? '' : 'hidden'}>
                            <QueryEditor fields={tab.fields} initialQuery={tab.initialQuery} />
                        </div>
                    ))}
                    {activeTabId === null && (
                        <p className="text-sm text-ink-500">
                            Fill in the connection form above, then Test connection or Load a saved connection to
                            open a new tab.
                        </p>
                    )}
                </div>
            )}
        </div>
    )
}

export default DbClientView
