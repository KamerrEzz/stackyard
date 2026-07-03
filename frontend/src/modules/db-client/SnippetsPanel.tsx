import {useCallback, useEffect, useState} from 'react'
import {CreateSnippet, DeleteSnippet, ListSnippets, UpdateSnippet} from '../../../wailsjs/go/main/App'
import type {main, storage} from '../../../wailsjs/go/models'
import {parseTagsInput, parseTagsJSON, tagsToInput} from './snippetHelpers'

type Engine = 'postgres' | 'mysql' | 'mongodb' | 'redis'

const ENGINE_OPTIONS: {engine: Engine; label: string}[] = [
    {engine: 'postgres', label: 'PostgreSQL'},
    {engine: 'mysql', label: 'MySQL'},
    {engine: 'mongodb', label: 'MongoDB'},
    {engine: 'redis', label: 'Redis'},
]

const GLOBAL_SCOPE_VALUE = ''

interface SnippetFormState {
    name: string
    engine: Engine
    body: string
    tagsInput: string
    scopeConnectionId: string
}

function emptyForm(defaultEngine: Engine): SnippetFormState {
    return {name: '', engine: defaultEngine, body: '', tagsInput: '', scopeConnectionId: GLOBAL_SCOPE_VALUE}
}

function formFromSnippet(snippet: storage.Snippet): SnippetFormState {
    return {
        name: snippet.Name,
        engine: snippet.Engine as Engine,
        body: snippet.Body,
        tagsInput: tagsToInput(parseTagsJSON(snippet.TagsJSON)),
        scopeConnectionId: snippet.ConnectionID ? String(snippet.ConnectionID) : GLOBAL_SCOPE_VALUE,
    }
}

interface SnippetsPanelProps {
    savedConnections: storage.Connection[]
}

function SnippetsPanel({savedConnections}: SnippetsPanelProps) {
    const [searchText, setSearchText] = useState('')
    const [snippets, setSnippets] = useState<storage.Snippet[]>([])
    const [listError, setListError] = useState<string | null>(null)

    const [editingId, setEditingId] = useState<number | null>(null)
    const [form, setForm] = useState<SnippetFormState>(emptyForm('postgres'))
    const [saveState, setSaveState] = useState<'idle' | 'saving' | 'error'>('idle')
    const [saveError, setSaveError] = useState<string | null>(null)

    const refreshSnippets = useCallback(async () => {
        try {
            const filter: main.SnippetFilter = {SearchText: searchText, ConnectionID: 0, Engine: ''}
            const results = await ListSnippets(filter)
            setListError(null)
            setSnippets(results)
        } catch (err) {
            setListError(String(err))
        }
    }, [searchText])

    useEffect(() => {
        void refreshSnippets()
    }, [refreshSnippets])

    const connectionNameById = useCallback(
        (id: number) => savedConnections.find((conn) => conn.ID === id)?.Name ?? `connection #${id}`,
        [savedConnections],
    )

    const startEdit = useCallback((snippet: storage.Snippet) => {
        setEditingId(snippet.ID)
        setForm(formFromSnippet(snippet))
        setSaveState('idle')
        setSaveError(null)
    }, [])

    const startCreate = useCallback(() => {
        setEditingId(null)
        setForm(emptyForm('postgres'))
        setSaveState('idle')
        setSaveError(null)
    }, [])

    const handleScopeChange = useCallback(
        (scopeConnectionId: string) => {
            setForm((prev) => {
                if (scopeConnectionId === GLOBAL_SCOPE_VALUE) {
                    return {...prev, scopeConnectionId}
                }
                const conn = savedConnections.find((c) => String(c.ID) === scopeConnectionId)
                return {...prev, scopeConnectionId, engine: (conn?.Engine as Engine) ?? prev.engine}
            })
        },
        [savedConnections],
    )

    const handleSave = useCallback(async () => {
        setSaveState('saving')
        setSaveError(null)
        try {
            const tags = parseTagsInput(form.tagsInput)
            const connectionID = form.scopeConnectionId === GLOBAL_SCOPE_VALUE ? null : Number(form.scopeConnectionId)

            if (editingId === null) {
                await CreateSnippet(form.name.trim(), form.engine, form.body, tags, connectionID)
            } else {
                await UpdateSnippet(editingId, form.name.trim(), form.engine, form.body, tags, connectionID)
            }

            setSaveState('idle')
            startCreate()
            await refreshSnippets()
        } catch (err) {
            setSaveState('error')
            setSaveError(String(err))
        }
    }, [editingId, form, refreshSnippets, startCreate])

    const handleDelete = useCallback(
        async (id: number, name: string) => {
            if (!window.confirm(`Delete snippet "${name}"? This cannot be undone.`)) {
                return
            }
            try {
                await DeleteSnippet(id)
                if (editingId === id) {
                    startCreate()
                }
                await refreshSnippets()
            } catch (err) {
                setListError(String(err))
            }
        },
        [editingId, refreshSnippets, startCreate],
    )

    const canSave = form.name.trim().length > 0 && form.body.trim().length > 0 && saveState !== 'saving'

    return (
        <div className="flex flex-col gap-3 rounded border border-ink-800 bg-ink-900/40 p-4">
            <div className="flex items-center justify-between">
                <h2 className="text-xs uppercase tracking-widest text-ink-400">Snippets</h2>
                <button
                    type="button"
                    onClick={startCreate}
                    className="rounded border border-ink-700 px-2 py-1 text-xs text-ink-200 hover:border-brass-500 hover:text-brass-400"
                >
                    + New snippet
                </button>
            </div>

            <div className="flex flex-col gap-1">
                <label htmlFor="snippet-search" className="text-xs uppercase tracking-widest text-ink-400">
                    Search by name or tag
                </label>
                <input
                    id="snippet-search"
                    type="text"
                    value={searchText}
                    onChange={(e) => setSearchText(e.target.value)}
                    placeholder="reporting"
                    className="rounded border border-ink-700 bg-ink-950 px-3 py-2 text-sm text-ink-100 outline-none focus:border-brass-500"
                />
            </div>

            {listError && <p className="text-xs text-red-400">{listError}</p>}
            {snippets.length === 0 && !listError && <p className="text-sm text-ink-500">No snippets found.</p>}

            <div className="flex flex-col gap-2">
                {snippets.map((snippet) => {
                    const tags = parseTagsJSON(snippet.TagsJSON)
                    const isGlobal = !snippet.ConnectionID
                    return (
                        <div
                            key={snippet.ID}
                            className="flex items-center justify-between gap-3 rounded border border-ink-800 bg-ink-950/60 px-3 py-2"
                        >
                            <div className="flex flex-col gap-1">
                                <div className="flex items-center gap-2">
                                    <span className="text-sm font-medium text-ink-100">{snippet.Name}</span>
                                    <span className="rounded border border-ink-700 px-1.5 py-0.5 text-[10px] uppercase text-ink-400">
                                        {snippet.Engine}
                                    </span>
                                    <span
                                        className={
                                            isGlobal
                                                ? 'rounded border border-emerald-800 px-1.5 py-0.5 text-[10px] uppercase text-emerald-400'
                                                : 'rounded border border-sky-800 px-1.5 py-0.5 text-[10px] uppercase text-sky-400'
                                        }
                                    >
                                        {isGlobal ? 'Global' : `Scoped: ${connectionNameById(snippet.ConnectionID as number)}`}
                                    </span>
                                </div>
                                {tags.length > 0 && (
                                    <div className="flex flex-wrap gap-1">
                                        {tags.map((tag) => (
                                            <span
                                                key={tag}
                                                className="rounded bg-ink-800 px-1.5 py-0.5 text-[10px] text-ink-300"
                                            >
                                                {tag}
                                            </span>
                                        ))}
                                    </div>
                                )}
                            </div>
                            <div className="flex items-center gap-2">
                                <button
                                    type="button"
                                    onClick={() => startEdit(snippet)}
                                    className="rounded border border-ink-700 px-3 py-1 text-xs text-ink-200 hover:border-brass-500 hover:text-brass-400"
                                >
                                    Edit
                                </button>
                                <button
                                    type="button"
                                    onClick={() => void handleDelete(snippet.ID, snippet.Name)}
                                    className="rounded border border-red-800 px-3 py-1 text-xs text-red-400 hover:border-red-500 hover:text-red-300"
                                >
                                    Delete
                                </button>
                            </div>
                        </div>
                    )
                })}
            </div>

            <div className="flex flex-col gap-3 border-t border-ink-800 pt-3">
                <h3 className="text-xs uppercase tracking-widest text-ink-400">
                    {editingId === null ? 'New snippet' : `Editing snippet #${editingId}`}
                </h3>

                <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
                    <div className="flex flex-col gap-1 sm:col-span-2">
                        <label htmlFor="snippet-name" className="text-xs uppercase tracking-widest text-ink-400">
                            Name
                        </label>
                        <input
                            id="snippet-name"
                            type="text"
                            value={form.name}
                            onChange={(e) => setForm((prev) => ({...prev, name: e.target.value}))}
                            className="rounded border border-ink-700 bg-ink-950 px-3 py-2 text-sm text-ink-100 outline-none focus:border-brass-500"
                        />
                    </div>

                    <div className="flex flex-col gap-1">
                        <label htmlFor="snippet-engine" className="text-xs uppercase tracking-widest text-ink-400">
                            Engine
                        </label>
                        <select
                            id="snippet-engine"
                            value={form.engine}
                            disabled={form.scopeConnectionId !== GLOBAL_SCOPE_VALUE}
                            onChange={(e) => setForm((prev) => ({...prev, engine: e.target.value as Engine}))}
                            className="rounded border border-ink-700 bg-ink-950 px-3 py-2 text-sm text-ink-100 outline-none focus:border-brass-500 disabled:opacity-50"
                        >
                            {ENGINE_OPTIONS.map((option) => (
                                <option key={option.engine} value={option.engine}>
                                    {option.label}
                                </option>
                            ))}
                        </select>
                    </div>

                    <div className="flex flex-col gap-1">
                        <label htmlFor="snippet-scope" className="text-xs uppercase tracking-widest text-ink-400">
                            Scope
                        </label>
                        <select
                            id="snippet-scope"
                            value={form.scopeConnectionId}
                            onChange={(e) => handleScopeChange(e.target.value)}
                            className="rounded border border-ink-700 bg-ink-950 px-3 py-2 text-sm text-ink-100 outline-none focus:border-brass-500"
                        >
                            <option value={GLOBAL_SCOPE_VALUE}>Global</option>
                            {savedConnections.map((conn) => (
                                <option key={conn.ID} value={String(conn.ID)}>
                                    {conn.Name}
                                </option>
                            ))}
                        </select>
                    </div>
                </div>

                <div className="flex flex-col gap-1">
                    <label htmlFor="snippet-tags" className="text-xs uppercase tracking-widest text-ink-400">
                        Tags (comma-separated)
                    </label>
                    <input
                        id="snippet-tags"
                        type="text"
                        value={form.tagsInput}
                        onChange={(e) => setForm((prev) => ({...prev, tagsInput: e.target.value}))}
                        placeholder="reporting, users"
                        className="rounded border border-ink-700 bg-ink-950 px-3 py-2 text-sm text-ink-100 outline-none focus:border-brass-500"
                    />
                </div>

                <div className="flex flex-col gap-1">
                    <label htmlFor="snippet-body" className="text-xs uppercase tracking-widest text-ink-400">
                        Body
                    </label>
                    <textarea
                        id="snippet-body"
                        value={form.body}
                        onChange={(e) => setForm((prev) => ({...prev, body: e.target.value}))}
                        rows={5}
                        className="rounded border border-ink-700 bg-ink-950 px-3 py-2 font-mono text-sm text-ink-100 outline-none focus:border-brass-500"
                    />
                </div>

                <div className="flex items-center gap-3">
                    <button
                        type="button"
                        onClick={() => void handleSave()}
                        disabled={!canSave}
                        className="rounded bg-brass-600 px-4 py-2 text-sm font-medium text-ink-950 transition-colors hover:bg-brass-500 disabled:cursor-not-allowed disabled:opacity-50"
                    >
                        {saveState === 'saving' ? 'Saving…' : editingId === null ? 'Create snippet' : 'Save changes'}
                    </button>
                    {editingId !== null && (
                        <button
                            type="button"
                            onClick={startCreate}
                            className="rounded border border-ink-700 px-3 py-2 text-xs text-ink-200 hover:border-brass-500 hover:text-brass-400"
                        >
                            Cancel edit
                        </button>
                    )}
                    {saveState === 'error' && <p className="text-sm text-red-400">{saveError}</p>}
                </div>
            </div>
        </div>
    )
}

export default SnippetsPanel
