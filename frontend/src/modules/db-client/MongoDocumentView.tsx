import {useCallback, useEffect, useRef, useState} from 'react'
import {
    CloseMongoSession,
    DeleteMongoDocuments,
    FindMongoDocuments,
    InsertMongoDocument,
    ListMongoCollections,
    ListMongoDatabases,
    OpenMongoConnection,
    UpdateMongoDocument,
} from '../../../wailsjs/go/main/App'
import type {main} from '../../../wailsjs/go/models'
import MongoDocumentTree from './MongoDocumentTree'
import MongoJSONEditor from './MongoJSONEditor'
import {
    formatDocumentForDuplicate,
    formatDocumentForEdit,
    parseFilterInput,
    toggleExpandedPath,
    validateDocumentJSON,
} from './mongoDocumentHelpers'
import {describeServerPage} from './resultsGridHelpers'

interface MongoDocumentViewProps {
    fields: main.ConnectionFormFields
}

type MongoDoc = Record<string, unknown>

/**
 * Documents render as trees, not grid rows, so a page holds far fewer of
 * them than `RESULTS_PAGE_SIZE` (100) does for SQL rows — 20 keeps a page of
 * expanded documents from turning into an unscannable wall.
 */
const MONGO_PAGE_SIZE = 20

function documentId(doc: MongoDoc): string {
    return typeof doc._id === 'string' ? doc._id : ''
}

/**
 * The MongoDB-side tab content (tasks.md 5.2-5.5), rendered by `DbClientView`
 * in place of `QueryEditor`+`ResultsGrid` for a Mongo-connected tab — see
 * `DbClientView`'s own doc comment for why the tab strip stays unified
 * instead of forking into a parallel Mongo tab list.
 *
 * This component opens and closes its own Mongo session entirely within its
 * own mount/unmount lifecycle, rather than receiving an already-open session
 * from `DbClientView` the way an earlier version of this code did. That
 * earlier design broke under React 18 StrictMode's dev-only double-invoke of
 * effects (mount → cleanup → mount): the session existed before this
 * component ever mounted, so the first synthetic unmount's cleanup closed it
 * immediately, and the "real" mount was left listing databases against an
 * already-closed session — caught during this feature's own manual
 * verification pass against a real MongoDB container, not a hypothetical.
 * Opening the session inside this component's own effect (see the first
 * `useEffect` below) fixes it the standard way: StrictMode's synthetic
 * mount opens session A and its synthetic cleanup closes A again before the
 * real mount opens session B, which is the one that ever reaches
 * `sessionID` state and the one the real unmount closes.
 */
function MongoDocumentView({fields}: MongoDocumentViewProps) {
    const [sessionID, setSessionID] = useState<string | null>(null)
    const [connectError, setConnectError] = useState<string | null>(null)
    const sessionIdRef = useRef<string | null>(null)

    const [databases, setDatabases] = useState<string[]>([])
    const [databasesError, setDatabasesError] = useState<string | null>(null)
    const [selectedDatabase, setSelectedDatabase] = useState<string | null>(null)

    const [collections, setCollections] = useState<string[]>([])
    const [collectionsError, setCollectionsError] = useState<string | null>(null)
    const [selectedCollection, setSelectedCollection] = useState<string | null>(null)

    const [documents, setDocuments] = useState<MongoDoc[]>([])
    const [documentsError, setDocumentsError] = useState<string | null>(null)
    const [loadingDocuments, setLoadingDocuments] = useState(false)
    const [skip, setSkip] = useState(0)
    const [lastFetchCount, setLastFetchCount] = useState(0)

    const [filterText, setFilterText] = useState('')
    const [appliedFilter, setAppliedFilter] = useState('')
    const [filterError, setFilterError] = useState<string | null>(null)

    const [expandedByDoc, setExpandedByDoc] = useState<Map<string, Set<string>>>(new Map())

    const [editingDocId, setEditingDocId] = useState<string | null>(null)
    const [editingText, setEditingText] = useState('')
    const [editSaving, setEditSaving] = useState(false)
    const [editError, setEditError] = useState<string | null>(null)

    const [creating, setCreating] = useState(false)
    const [createText, setCreateText] = useState('{}')
    const [createSaving, setCreateSaving] = useState(false)
    const [createError, setCreateError] = useState<string | null>(null)

    const [deletingId, setDeletingId] = useState<string | null>(null)
    const [deleteError, setDeleteError] = useState<string | null>(null)

    useEffect(() => {
        let cancelled = false
        OpenMongoConnection(fields)
            .then((id) => {
                if (cancelled) {
                    void CloseMongoSession(id)
                    return
                }
                sessionIdRef.current = id
                setSessionID(id)
                setConnectError(null)
            })
            .catch((err) => {
                if (!cancelled) {
                    setConnectError(String(err))
                }
            })
        return () => {
            cancelled = true
            const id = sessionIdRef.current
            if (id) {
                sessionIdRef.current = null
                void CloseMongoSession(id)
            }
        }
    }, [fields])

    useEffect(() => {
        if (!sessionID) {
            return
        }
        let cancelled = false
        ListMongoDatabases(sessionID)
            .then((names) => {
                if (!cancelled) {
                    setDatabases(names)
                    setDatabasesError(null)
                }
            })
            .catch((err) => {
                if (!cancelled) {
                    setDatabasesError(String(err))
                }
            })
        return () => {
            cancelled = true
        }
    }, [sessionID])

    useEffect(() => {
        if (!sessionID || !selectedDatabase) {
            setCollections([])
            return
        }
        let cancelled = false
        ListMongoCollections(sessionID, selectedDatabase)
            .then((names) => {
                if (!cancelled) {
                    setCollections(names)
                    setCollectionsError(null)
                }
            })
            .catch((err) => {
                if (!cancelled) {
                    setCollectionsError(String(err))
                }
            })
        return () => {
            cancelled = true
        }
    }, [sessionID, selectedDatabase])

    const loadDocuments = useCallback(
        async (database: string, collection: string, nextSkip: number, filterJSON: string) => {
            if (!sessionID) {
                return
            }
            setLoadingDocuments(true)
            setDocumentsError(null)
            try {
                const docs = await FindMongoDocuments(sessionID, database, collection, filterJSON, MONGO_PAGE_SIZE, nextSkip)
                setDocuments(docs)
                setLastFetchCount(docs.length)
                setSkip(nextSkip)
                setExpandedByDoc(new Map())
                setEditingDocId(null)
                setCreating(false)
            } catch (err) {
                setDocumentsError(String(err))
            } finally {
                setLoadingDocuments(false)
            }
        },
        [sessionID],
    )

    useEffect(() => {
        setFilterText('')
        setAppliedFilter('')
        setFilterError(null)
        if (selectedDatabase && selectedCollection) {
            void loadDocuments(selectedDatabase, selectedCollection, 0, '')
        } else {
            setDocuments([])
        }
    }, [selectedDatabase, selectedCollection, loadDocuments])

    const handleSelectDatabase = useCallback((database: string) => {
        setSelectedDatabase(database)
        setSelectedCollection(null)
    }, [])

    const handleSelectCollection = useCallback((collection: string) => {
        setSelectedCollection(collection)
    }, [])

    const handleToggleExpand = useCallback((docId: string, path: string) => {
        setExpandedByDoc((prev) => {
            const current = prev.get(docId) ?? new Set<string>()
            const next = new Map(prev)
            next.set(docId, toggleExpandedPath(current, path))
            return next
        })
    }, [])

    const handleStartEdit = useCallback((doc: MongoDoc) => {
        setEditingDocId(documentId(doc))
        setEditingText(formatDocumentForEdit(doc))
        setEditError(null)
        setCreating(false)
    }, [])

    const handleCancelEdit = useCallback(() => {
        setEditingDocId(null)
        setEditingText('')
        setEditError(null)
    }, [])

    const handleSaveEdit = useCallback(async () => {
        if (!sessionID || !editingDocId || !selectedDatabase || !selectedCollection) {
            return
        }
        const validation = validateDocumentJSON(editingText)
        if (!validation.ok) {
            setEditError(validation.error)
            return
        }
        setEditSaving(true)
        setEditError(null)
        try {
            await UpdateMongoDocument(sessionID, selectedDatabase, selectedCollection, editingDocId, JSON.stringify(validation.value))
            setDocuments((prev) => prev.map((doc) => (documentId(doc) === editingDocId ? {...validation.value, _id: editingDocId} : doc)))
            setEditingDocId(null)
            setEditingText('')
        } catch (err) {
            setEditError(String(err))
        } finally {
            setEditSaving(false)
        }
    }, [editingDocId, editingText, selectedCollection, selectedDatabase, sessionID])

    const handleStartCreate = useCallback((duplicateFrom?: MongoDoc) => {
        setCreating(true)
        setCreateText(duplicateFrom ? formatDocumentForDuplicate(duplicateFrom) : '{}')
        setCreateError(null)
        setEditingDocId(null)
    }, [])

    const handleCancelCreate = useCallback(() => {
        setCreating(false)
        setCreateText('{}')
        setCreateError(null)
    }, [])

    const handleSaveCreate = useCallback(async () => {
        if (!sessionID || !selectedDatabase || !selectedCollection) {
            return
        }
        const validation = validateDocumentJSON(createText)
        if (!validation.ok) {
            setCreateError(validation.error)
            return
        }
        setCreateSaving(true)
        setCreateError(null)
        try {
            const inserted = await InsertMongoDocument(sessionID, selectedDatabase, selectedCollection, JSON.stringify(validation.value))
            setDocuments((prev) => [inserted, ...prev])
            setCreating(false)
            setCreateText('{}')
        } catch (err) {
            setCreateError(String(err))
        } finally {
            setCreateSaving(false)
        }
    }, [createText, selectedCollection, selectedDatabase, sessionID])

    const handleDelete = useCallback(
        async (doc: MongoDoc) => {
            if (!sessionID || !selectedDatabase || !selectedCollection) {
                return
            }
            const id = documentId(doc)
            if (!id) {
                return
            }
            if (!window.confirm('Delete this document? This cannot be undone.')) {
                return
            }
            setDeletingId(id)
            setDeleteError(null)
            try {
                await DeleteMongoDocuments(sessionID, selectedDatabase, selectedCollection, [id])
                setDocuments((prev) => prev.filter((d) => documentId(d) !== id))
            } catch (err) {
                setDeleteError(String(err))
            } finally {
                setDeletingId(null)
            }
        },
        [selectedCollection, selectedDatabase, sessionID],
    )

    const handlePrevPage = useCallback(() => {
        if (!selectedDatabase || !selectedCollection) {
            return
        }
        void loadDocuments(selectedDatabase, selectedCollection, Math.max(0, skip - MONGO_PAGE_SIZE), appliedFilter)
    }, [appliedFilter, loadDocuments, selectedCollection, selectedDatabase, skip])

    const handleNextPage = useCallback(() => {
        if (!selectedDatabase || !selectedCollection) {
            return
        }
        void loadDocuments(selectedDatabase, selectedCollection, skip + MONGO_PAGE_SIZE, appliedFilter)
    }, [appliedFilter, loadDocuments, selectedCollection, selectedDatabase, skip])

    const handleApplyFilter = useCallback(() => {
        if (!selectedDatabase || !selectedCollection) {
            return
        }
        const parsed = parseFilterInput(filterText)
        if (!parsed.ok) {
            setFilterError(parsed.error)
            return
        }
        setFilterError(null)
        setAppliedFilter(parsed.filterJSON)
        void loadDocuments(selectedDatabase, selectedCollection, 0, parsed.filterJSON)
    }, [filterText, loadDocuments, selectedCollection, selectedDatabase])

    const handleClearFilter = useCallback(() => {
        if (!selectedDatabase || !selectedCollection) {
            return
        }
        setFilterText('')
        setFilterError(null)
        setAppliedFilter('')
        void loadDocuments(selectedDatabase, selectedCollection, 0, '')
    }, [loadDocuments, selectedCollection, selectedDatabase])

    const pageInfo = describeServerPage(skip, MONGO_PAGE_SIZE, lastFetchCount, documents.length)

    if (connectError) {
        return (
            <div className="flex flex-col gap-2 rounded border border-red-800 bg-ink-900/40 p-4">
                <h2 className="text-xs uppercase tracking-widest text-ink-400">Mongo document browser</h2>
                <p className="text-sm text-red-400">{connectError}</p>
            </div>
        )
    }

    if (!sessionID) {
        return (
            <div className="flex flex-col gap-2 rounded border border-ink-800 bg-ink-900/40 p-4">
                <h2 className="text-xs uppercase tracking-widest text-ink-400">Mongo document browser</h2>
                <p className="text-sm text-ink-500">Connecting to MongoDB…</p>
            </div>
        )
    }

    return (
        <div className="flex flex-col gap-3 rounded border border-ink-800 bg-ink-900/40 p-4">
            <div className="flex items-center justify-between">
                <h2 className="text-xs uppercase tracking-widest text-ink-400">Mongo document browser</h2>
                <span className="font-mono text-xs text-ink-500">{fields.Engine}</span>
            </div>

            <div className="flex flex-wrap items-center gap-3">
                <div className="flex flex-col gap-1">
                    <label htmlFor="mongo-database" className="text-xs uppercase tracking-widest text-ink-400">
                        Database
                    </label>
                    <select
                        id="mongo-database"
                        value={selectedDatabase ?? ''}
                        onChange={(e) => handleSelectDatabase(e.target.value)}
                        className="rounded border border-ink-700 bg-ink-950 px-3 py-1.5 text-xs text-ink-100 outline-none focus:border-brass-500"
                    >
                        <option value="" disabled>
                            Select a database…
                        </option>
                        {databases.map((name) => (
                            <option key={name} value={name}>
                                {name}
                            </option>
                        ))}
                    </select>
                </div>

                <div className="flex flex-col gap-1">
                    <label htmlFor="mongo-collection" className="text-xs uppercase tracking-widest text-ink-400">
                        Collection
                    </label>
                    <select
                        id="mongo-collection"
                        value={selectedCollection ?? ''}
                        onChange={(e) => handleSelectCollection(e.target.value)}
                        disabled={!selectedDatabase}
                        className="rounded border border-ink-700 bg-ink-950 px-3 py-1.5 text-xs text-ink-100 outline-none focus:border-brass-500 disabled:cursor-not-allowed disabled:opacity-50"
                    >
                        <option value="" disabled>
                            Select a collection…
                        </option>
                        {collections.map((name) => (
                            <option key={name} value={name}>
                                {name}
                            </option>
                        ))}
                    </select>
                </div>

                {selectedDatabase && selectedCollection && (
                    <button
                        type="button"
                        onClick={() => handleStartCreate()}
                        className="mt-4 rounded border border-ink-700 px-3 py-1.5 text-xs text-ink-200 transition-colors hover:border-brass-500 hover:text-brass-400"
                    >
                        + New document
                    </button>
                )}
            </div>

            {databasesError && <p className="text-xs text-red-400">{databasesError}</p>}
            {collectionsError && <p className="text-xs text-red-400">{collectionsError}</p>}

            {selectedDatabase && selectedCollection && (
                <div className="flex flex-col gap-1">
                    <label htmlFor="mongo-filter" className="text-xs uppercase tracking-widest text-ink-400">
                        Filter (JSON)
                    </label>
                    <div className="flex flex-wrap items-center gap-2">
                        <input
                            id="mongo-filter"
                            type="text"
                            value={filterText}
                            onChange={(e) => setFilterText(e.target.value)}
                            onKeyDown={(e) => {
                                if (e.key === 'Enter') {
                                    handleApplyFilter()
                                }
                            }}
                            placeholder='{"status": "active"}'
                            className="w-72 rounded border border-ink-700 bg-ink-950 px-3 py-1.5 font-mono text-xs text-ink-100 outline-none focus:border-brass-500"
                        />
                        <button
                            type="button"
                            onClick={handleApplyFilter}
                            className="rounded border border-ink-700 px-3 py-1.5 text-xs text-ink-200 transition-colors hover:border-brass-500 hover:text-brass-400"
                        >
                            Find
                        </button>
                        {(filterText || appliedFilter) && (
                            <button
                                type="button"
                                onClick={handleClearFilter}
                                className="rounded border border-ink-700 px-3 py-1.5 text-xs text-ink-200 transition-colors hover:border-brass-500 hover:text-brass-400"
                            >
                                Clear
                            </button>
                        )}
                    </div>
                    {filterError && <p className="text-xs text-red-400">{filterError}</p>}
                </div>
            )}

            {creating && (
                <MongoJSONEditor
                    text={createText}
                    onTextChange={setCreateText}
                    onSave={() => void handleSaveCreate()}
                    onCancel={handleCancelCreate}
                    saving={createSaving}
                    saveError={createError}
                    saveLabel="Create"
                />
            )}

            {selectedDatabase && selectedCollection && (
                <div className="flex flex-col gap-2">
                    {loadingDocuments && <p className="text-xs text-ink-500">Loading documents…</p>}
                    {documentsError && <p className="text-xs text-red-400">{documentsError}</p>}
                    {!loadingDocuments && !documentsError && documents.length === 0 && (
                        <p className="text-xs text-ink-500">This collection has no documents on this page.</p>
                    )}
                    {deleteError && <p className="text-xs text-red-400">{deleteError}</p>}

                    {documents.map((doc) => {
                        const id = documentId(doc)
                        const isEditing = editingDocId === id
                        return (
                            <div key={id || JSON.stringify(doc)} className="rounded border border-ink-800 bg-ink-950/40 p-3">
                                <div className="mb-2 flex items-center justify-between gap-2">
                                    <span className="font-mono text-[10px] text-ink-500">_id: {id || '(none)'}</span>
                                    <div className="flex items-center gap-2">
                                        <button
                                            type="button"
                                            onClick={() => handleStartEdit(doc)}
                                            disabled={isEditing}
                                            className="rounded border border-ink-700 px-2 py-0.5 text-[10px] text-ink-200 transition-colors hover:border-brass-500 hover:text-brass-400 disabled:cursor-not-allowed disabled:opacity-40"
                                        >
                                            Edit
                                        </button>
                                        <button
                                            type="button"
                                            onClick={() => handleStartCreate(doc)}
                                            className="rounded border border-ink-700 px-2 py-0.5 text-[10px] text-ink-200 transition-colors hover:border-brass-500 hover:text-brass-400"
                                        >
                                            Duplicate
                                        </button>
                                        <button
                                            type="button"
                                            onClick={() => void handleDelete(doc)}
                                            disabled={deletingId === id}
                                            className="rounded border border-red-800 px-2 py-0.5 text-[10px] text-red-400 transition-colors hover:border-red-500 hover:text-red-300 disabled:cursor-not-allowed disabled:opacity-40"
                                        >
                                            {deletingId === id ? 'Deleting…' : 'Delete'}
                                        </button>
                                    </div>
                                </div>

                                {isEditing ? (
                                    <MongoJSONEditor
                                        text={editingText}
                                        onTextChange={setEditingText}
                                        onSave={() => void handleSaveEdit()}
                                        onCancel={handleCancelEdit}
                                        saving={editSaving}
                                        saveError={editError}
                                        saveLabel="Save"
                                    />
                                ) : (
                                    <MongoDocumentTree
                                        document={doc}
                                        expandedPaths={expandedByDoc.get(id) ?? new Set()}
                                        onToggle={(path) => handleToggleExpand(id, path)}
                                    />
                                )}
                            </div>
                        )
                    })}

                    {documents.length > 0 && (
                        <div className="flex items-center justify-between text-xs text-ink-400">
                            <span>
                                Showing {pageInfo.startIndex}-{pageInfo.endIndex} documents
                            </span>
                            <div className="flex items-center gap-2">
                                <button
                                    type="button"
                                    onClick={handlePrevPage}
                                    disabled={!pageInfo.hasPrevPage || loadingDocuments}
                                    className="rounded border border-ink-700 px-2 py-1 text-ink-300 transition-colors hover:border-brass-500 hover:text-brass-500 disabled:cursor-not-allowed disabled:opacity-40"
                                >
                                    Prev
                                </button>
                                <span className="text-ink-500">Page {pageInfo.pageNumber}</span>
                                <button
                                    type="button"
                                    onClick={handleNextPage}
                                    disabled={!pageInfo.hasNextPage || loadingDocuments}
                                    className="rounded border border-ink-700 px-2 py-1 text-ink-300 transition-colors hover:border-brass-500 hover:text-brass-500 disabled:cursor-not-allowed disabled:opacity-40"
                                >
                                    Next
                                </button>
                            </div>
                        </div>
                    )}
                </div>
            )}
        </div>
    )
}

export default MongoDocumentView
