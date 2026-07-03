import {useCallback, useState} from 'react'
import {CreateTable} from '../../../wailsjs/go/main/App'
import {
    buildColumnDefinitions,
    type ColumnFormRow,
    COLUMN_TYPE_OPTIONS,
    createBlankColumnRow,
    isAutoIncrementType,
    setPrimaryKey,
    validateCreateTableForm,
} from './createTableHelpers'

interface CreateTableDialogProps {
    sessionID: string
    schemas: string[]
    onClose: () => void
    onCreated: () => void
}

type Stage = 'idle' | 'creating' | 'error'

/**
 * The "Create table" form (tasks.md 10.2): a table name plus a repeatable
 * list of column rows (name/type/nullable/primary-key/default), submitted
 * via CreateTable — a UI convenience over hand-writing a CREATE TABLE
 * statement, the same "form generates real DDL" framing docs/STATE.md's
 * Session 22 scoped this feature under. Client-side validation
 * (validateCreateTableForm) mirrors BuildCreateTableDDL's own rules so most
 * mistakes are caught before the round trip to the backend; any error the
 * backend still reports (e.g. a database-level constraint this form doesn't
 * pre-validate) is shown exactly as returned.
 */
function CreateTableDialog({sessionID, schemas, onClose, onCreated}: CreateTableDialogProps) {
    const [tableName, setTableName] = useState('')
    const [schema, setSchema] = useState(schemas[0] ?? '')
    const [rows, setRows] = useState<ColumnFormRow[]>([createBlankColumnRow()])
    const [stage, setStage] = useState<Stage>('idle')
    const [errorMessage, setErrorMessage] = useState<string | null>(null)

    const updateRow = useCallback((index: number, patch: Partial<ColumnFormRow>) => {
        setRows((current) => current.map((row, i) => (i === index ? {...row, ...patch} : row)))
    }, [])

    const handleAddRow = useCallback(() => {
        setRows((current) => [...current, createBlankColumnRow()])
    }, [])

    const handleRemoveRow = useCallback((index: number) => {
        setRows((current) => current.filter((_, i) => i !== index))
    }, [])

    const handleTogglePrimaryKey = useCallback((index: number, checked: boolean) => {
        setRows((current) => setPrimaryKey(current, index, checked))
    }, [])

    const validationError = validateCreateTableForm(tableName, rows)

    const handleSubmit = useCallback(async () => {
        if (validationError) {
            return
        }
        setStage('creating')
        setErrorMessage(null)
        try {
            await CreateTable(sessionID, schema, tableName.trim(), buildColumnDefinitions(rows))
            onCreated()
            onClose()
        } catch (err) {
            setErrorMessage(String(err))
            setStage('error')
        }
    }, [onClose, onCreated, rows, schema, sessionID, tableName, validationError])

    return (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 p-4">
            <div className="flex max-h-[85vh] w-full max-w-3xl flex-col gap-4 overflow-auto rounded border border-ink-700 bg-ink-950 p-5">
                <div className="flex items-center justify-between">
                    <h2 className="text-xs uppercase tracking-widest text-ink-400">Create table</h2>
                    <button
                        type="button"
                        onClick={onClose}
                        className="rounded border border-ink-700 px-2 py-1 text-xs text-ink-300 hover:border-brass-500 hover:text-brass-400"
                    >
                        Close
                    </button>
                </div>

                <div className="flex flex-wrap gap-3">
                    <div className="flex flex-1 flex-col gap-1">
                        <label htmlFor="create-table-name" className="text-xs uppercase tracking-widest text-ink-400">
                            Table name
                        </label>
                        <input
                            id="create-table-name"
                            type="text"
                            value={tableName}
                            onChange={(e) => setTableName(e.target.value)}
                            placeholder="widgets"
                            className="rounded border border-ink-700 bg-ink-950 px-3 py-2 text-sm text-ink-100 outline-none focus:border-brass-500"
                        />
                    </div>
                    {schemas.length > 1 && (
                        <div className="flex flex-col gap-1">
                            <label htmlFor="create-table-schema" className="text-xs uppercase tracking-widest text-ink-400">
                                Schema
                            </label>
                            <select
                                id="create-table-schema"
                                value={schema}
                                onChange={(e) => setSchema(e.target.value)}
                                className="rounded border border-ink-700 bg-ink-950 px-3 py-2 text-sm text-ink-100 outline-none focus:border-brass-500"
                            >
                                {schemas.map((s) => (
                                    <option key={s} value={s}>
                                        {s}
                                    </option>
                                ))}
                            </select>
                        </div>
                    )}
                </div>

                <div className="flex flex-col gap-2">
                    <span className="text-xs uppercase tracking-widest text-ink-400">Columns</span>
                    <div className="flex flex-col gap-2">
                        {rows.map((row, index) => (
                            <div
                                key={index}
                                className="flex flex-wrap items-center gap-2 rounded border border-ink-800 bg-ink-900/40 p-2"
                            >
                                <input
                                    type="text"
                                    value={row.name}
                                    onChange={(e) => updateRow(index, {name: e.target.value})}
                                    placeholder="column name"
                                    className="w-32 rounded border border-ink-700 bg-ink-950 px-2 py-1 text-xs text-ink-100 outline-none focus:border-brass-500"
                                />
                                <select
                                    value={row.type}
                                    onChange={(e) =>
                                        updateRow(index, {
                                            type: e.target.value,
                                            nullable: isAutoIncrementType(e.target.value) ? false : row.nullable,
                                        })
                                    }
                                    className="rounded border border-ink-700 bg-ink-950 px-2 py-1 text-xs text-ink-100 outline-none focus:border-brass-500"
                                >
                                    {COLUMN_TYPE_OPTIONS.map((option) => (
                                        <option key={option.value} value={option.value}>
                                            {option.label}
                                        </option>
                                    ))}
                                </select>
                                <label className="flex items-center gap-1 text-xs text-ink-300">
                                    <input
                                        type="checkbox"
                                        checked={row.nullable}
                                        disabled={isAutoIncrementType(row.type)}
                                        onChange={(e) => updateRow(index, {nullable: e.target.checked})}
                                        className="h-4 w-4 rounded border-ink-700 bg-ink-950 text-brass-500 focus:ring-brass-500"
                                    />
                                    Nullable
                                </label>
                                <label className="flex items-center gap-1 text-xs text-ink-300">
                                    <input
                                        type="checkbox"
                                        checked={row.isPrimaryKey}
                                        onChange={(e) => handleTogglePrimaryKey(index, e.target.checked)}
                                        className="h-4 w-4 rounded border-ink-700 bg-ink-950 text-brass-500 focus:ring-brass-500"
                                    />
                                    Primary key
                                </label>
                                <input
                                    type="text"
                                    value={row.defaultValue}
                                    onChange={(e) => updateRow(index, {defaultValue: e.target.value})}
                                    placeholder="default (SQL expr, e.g. 'active')"
                                    disabled={isAutoIncrementType(row.type)}
                                    className="w-48 rounded border border-ink-700 bg-ink-950 px-2 py-1 text-xs text-ink-100 outline-none focus:border-brass-500 disabled:opacity-50"
                                />
                                <button
                                    type="button"
                                    onClick={() => handleRemoveRow(index)}
                                    disabled={rows.length === 1}
                                    className="ml-auto rounded border border-red-800 px-2 py-1 text-xs text-red-400 transition-colors hover:border-red-500 hover:text-red-300 disabled:cursor-not-allowed disabled:opacity-50"
                                >
                                    Remove
                                </button>
                            </div>
                        ))}
                    </div>
                    <button
                        type="button"
                        onClick={handleAddRow}
                        className="self-start rounded border border-ink-700 px-3 py-1.5 text-xs text-ink-200 transition-colors hover:border-brass-500 hover:text-brass-400"
                    >
                        + Add column
                    </button>
                </div>

                {validationError && <p className="text-xs text-amber-400">{validationError}</p>}
                {errorMessage && <p className="text-sm text-red-400">{errorMessage}</p>}

                <div className="flex items-center gap-3 border-t border-ink-800 pt-3">
                    <button
                        type="button"
                        onClick={() => void handleSubmit()}
                        disabled={stage === 'creating' || validationError !== null}
                        className="rounded bg-brass-600 px-4 py-2 text-sm font-medium text-ink-950 transition-colors hover:bg-brass-500 disabled:cursor-not-allowed disabled:opacity-50"
                    >
                        {stage === 'creating' ? 'Creating…' : 'Create table'}
                    </button>
                </div>
            </div>
        </div>
    )
}

export default CreateTableDialog
