import {useMemo} from 'react'
import {validateDocumentJSON} from './mongoDocumentHelpers'

interface MongoJSONEditorProps {
    text: string
    onTextChange: (text: string) => void
    onSave: () => void
    onCancel: () => void
    saving: boolean
    saveError: string | null
    saveLabel?: string
}

/**
 * Shared raw-JSON editing surface for both the in-place edit flow (tasks.md
 * 5.3) and the new-document flow (tasks.md 5.4, from blank `{}` or a
 * duplicated document) — the whole document is edited as one JSON text
 * block rather than per-leaf, a simpler interpretation of spec.md §4.4's "in-
 * place edits validate JSON structure before allowing save" that still
 * satisfies it literally; per-leaf editing would need its own type-aware
 * input per scalar kind (see `MongoDocumentTree`'s type badges) without
 * changing what "valid" means, so it was deliberately deferred rather than
 * built here. A plain `<textarea>` (not Monaco) is used deliberately,
 * mirroring `ResultsGrid`'s own choice of plain `<input>` over Monaco for
 * structured-data editing — Monaco's mount/highlight machinery isn't worth
 * it for a form field that can appear once per visible document.
 *
 * Validation runs on every keystroke via `validateDocumentJSON`, but only
 * gates the Save button — it never blocks typing, so an intermediate
 * "invalid" state (e.g. mid-edit with an unmatched brace) is always visible
 * and always recoverable. `saveError` is a distinct, separate slot for the
 * backend's own error (e.g. `UpdateMongoDocument`/`InsertMongoDocument`
 * failing after validation already passed) so the two error sources are
 * never confused with each other.
 */
function MongoJSONEditor({text, onTextChange, onSave, onCancel, saving, saveError, saveLabel}: MongoJSONEditorProps) {
    const validation = useMemo(() => validateDocumentJSON(text), [text])

    return (
        <div className="flex flex-col gap-2 rounded border border-ink-700 bg-ink-950/60 p-3">
            <textarea
                value={text}
                onChange={(e) => onTextChange(e.target.value)}
                spellCheck={false}
                rows={10}
                className="w-full resize-y rounded border border-ink-700 bg-ink-950 px-3 py-2 font-mono text-xs text-ink-100 outline-none focus:border-brass-500"
            />
            {!validation.ok && <p className="text-xs text-red-400">{validation.error}</p>}
            {saveError && <p className="text-xs text-red-400">{saveError}</p>}
            <div className="flex items-center gap-2">
                <button
                    type="button"
                    onClick={onSave}
                    disabled={!validation.ok || saving}
                    className="rounded bg-brass-600 px-3 py-1.5 text-xs font-medium text-ink-950 transition-colors hover:bg-brass-500 disabled:cursor-not-allowed disabled:opacity-50"
                >
                    {saving ? 'Saving…' : (saveLabel ?? 'Save')}
                </button>
                <button
                    type="button"
                    onClick={onCancel}
                    disabled={saving}
                    className="rounded border border-ink-700 px-3 py-1.5 text-xs text-ink-300 transition-colors hover:border-red-500 hover:text-red-300 disabled:cursor-not-allowed disabled:opacity-50"
                >
                    Cancel
                </button>
            </div>
        </div>
    )
}

export default MongoJSONEditor
