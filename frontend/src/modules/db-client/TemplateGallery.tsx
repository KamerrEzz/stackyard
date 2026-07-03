import {useCallback, useEffect, useState} from 'react'
import {CreateSnippet, ListSnippetTemplates} from '../../../wailsjs/go/main/App'
import type {snippettemplates} from '../../../wailsjs/go/models'
import {defaultGalleryEngine, sqlForEngine, type GalleryEngine} from './templateGalleryLogic'

const ENGINE_TOGGLE_OPTIONS: {engine: GalleryEngine; label: string}[] = [
    {engine: 'postgres', label: 'PostgreSQL'},
    {engine: 'mysql', label: 'MySQL'},
]

type SaveState = 'idle' | 'saving' | 'saved' | 'error'

interface TemplateGalleryProps {
    activeEngine: string | null
    onLoad: (sql: string, templateName: string, engine: GalleryEngine) => void
    loadError?: string | null
    onSaved: () => void
}

/**
 * The DB Client's built-in Template gallery (tasks.md 10.3): a curated,
 * read-only library of starter SQL snippets fetched once from the Go side's
 * static snippettemplates.List() (via ListSnippetTemplates), rendered as its
 * own bordered section clearly separate from SnippetsPanel's "your own
 * saved snippets" list right above it — same visual language, different
 * data source, never mixed into the same list.
 *
 * Each template offers two actions: "Load into editor" (onLoad, resolved by
 * the parent DbClientView against whichever tab is active, since only that
 * parent owns the tab/editor-handle state) and "Save as my snippet" (handled
 * entirely here via the existing CreateSnippet bound method, tagged
 * "template" so a saved copy is traceable back to its origin in the user's
 * own snippet list). Both actions operate on whichever engine variant the
 * gallery's own Postgres/MySQL toggle currently selects — defaulted to the
 * active tab's engine (defaultGalleryEngine) but always overridable, since a
 * user may want to inspect or save the other engine's variant without
 * switching tabs.
 */
function TemplateGallery({activeEngine, onLoad, loadError, onSaved}: TemplateGalleryProps) {
    const [templates, setTemplates] = useState<snippettemplates.Template[]>([])
    const [listError, setListError] = useState<string | null>(null)
    const [engine, setEngine] = useState<GalleryEngine>(defaultGalleryEngine(activeEngine))
    const [saveState, setSaveState] = useState<Record<string, SaveState>>({})
    const [saveError, setSaveError] = useState<string | null>(null)

    useEffect(() => {
        ListSnippetTemplates()
            .then((result) => {
                setTemplates(result)
                setListError(null)
            })
            .catch((err) => setListError(String(err)))
    }, [])

    useEffect(() => {
        setEngine(defaultGalleryEngine(activeEngine))
    }, [activeEngine])

    const handleSaveAsSnippet = useCallback(
        async (template: snippettemplates.Template) => {
            const sql = sqlForEngine(template, engine)
            if (sql === null) {
                setSaveError(`"${template.Name}" has no ${engine} variant.`)
                return
            }
            setSaveState((prev) => ({...prev, [template.ID]: 'saving'}))
            setSaveError(null)
            try {
                await CreateSnippet(template.Name, engine, sql, ['template'], null)
                setSaveState((prev) => ({...prev, [template.ID]: 'saved'}))
                onSaved()
            } catch (err) {
                setSaveState((prev) => ({...prev, [template.ID]: 'error'}))
                setSaveError(String(err))
            }
        },
        [engine, onSaved],
    )

    return (
        <div className="flex flex-col gap-3 rounded border border-ink-800 bg-ink-900/40 p-4">
            <div className="flex items-center justify-between">
                <h2 className="text-xs uppercase tracking-widest text-ink-400">Template gallery</h2>
                <div className="flex items-center gap-1">
                    {ENGINE_TOGGLE_OPTIONS.map((option) => (
                        <button
                            key={option.engine}
                            type="button"
                            onClick={() => setEngine(option.engine)}
                            className={
                                engine === option.engine
                                    ? 'rounded border border-brass-500 px-2 py-1 text-xs text-brass-400'
                                    : 'rounded border border-ink-700 px-2 py-1 text-xs text-ink-200 hover:border-brass-500 hover:text-brass-400'
                            }
                        >
                            {option.label}
                        </button>
                    ))}
                </div>
            </div>

            <p className="text-xs text-ink-500">
                Built-in starter templates — read-only. Load one into the current tab's editor, or save it as your
                own editable snippet.
            </p>

            {listError && <p className="text-xs text-red-400">{listError}</p>}
            {loadError && <p className="text-xs text-red-400">{loadError}</p>}
            {saveError && <p className="text-xs text-red-400">{saveError}</p>}
            {templates.length === 0 && !listError && <p className="text-sm text-ink-500">Loading templates…</p>}

            <div className="flex flex-col gap-2">
                {templates.map((template) => {
                    const sql = sqlForEngine(template, engine)
                    const state = saveState[template.ID] ?? 'idle'
                    return (
                        <div
                            key={template.ID}
                            className="flex items-center justify-between gap-3 rounded border border-ink-800 bg-ink-950/60 px-3 py-2"
                        >
                            <div className="flex flex-col gap-1">
                                <span className="text-sm font-medium text-ink-100">{template.Name}</span>
                                <span className="text-xs text-ink-400">{template.Description}</span>
                                {sql === null && (
                                    <span className="text-[10px] uppercase text-ink-500">No {engine} variant</span>
                                )}
                            </div>
                            <div className="flex items-center gap-2">
                                <button
                                    type="button"
                                    onClick={() => sql !== null && onLoad(sql, template.Name, engine)}
                                    disabled={sql === null}
                                    className="rounded border border-brass-700 px-3 py-1 text-xs text-brass-400 hover:border-brass-500 hover:text-brass-300 disabled:cursor-not-allowed disabled:opacity-50"
                                >
                                    Load into editor
                                </button>
                                <button
                                    type="button"
                                    onClick={() => void handleSaveAsSnippet(template)}
                                    disabled={sql === null || state === 'saving'}
                                    className="rounded border border-ink-700 px-3 py-1 text-xs text-ink-200 hover:border-brass-500 hover:text-brass-400 disabled:cursor-not-allowed disabled:opacity-50"
                                >
                                    {state === 'saving' ? 'Saving…' : state === 'saved' ? 'Saved ✓' : 'Save as my snippet'}
                                </button>
                            </div>
                        </div>
                    )
                })}
            </div>
        </div>
    )
}

export default TemplateGallery
