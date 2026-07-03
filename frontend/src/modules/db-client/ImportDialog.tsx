import {useCallback, useState} from 'react'
import {ImportFile, PickImportFile, ValidateImportFile} from '../../../wailsjs/go/main/App'
import type {dbengine, importdata} from '../../../wailsjs/go/models'
import {fileNameFromPath, formatMismatch, isImportableFilePath, sortMismatches, summarizeValidation, type ValidationSummarySource} from './importHelpers'

interface ImportDialogProps {
    sessionID: string
    schema: string
    table: dbengine.TableInfo
    onClose: () => void
    onImported: () => void
}

type Stage = 'idle' | 'validating' | 'validated' | 'importing' | 'imported' | 'error'

/**
 * The CSV/JSON import flow (tasks.md 7.4, spec.md §4.9): pick a file, see a
 * validation report (mismatches, or an all-clear row count), then confirm
 * the actual import. `ValidateImportFile`/`ImportFile` (app.go) both run the
 * full pre-commit validation independently — this component never decides
 * on its own whether a file is safe to import, it only renders whatever the
 * backend reports and gates the "Confirm import" button on the most recent
 * report being clean (`canConfirm` below).
 */
function ImportDialog({sessionID, schema, table, onClose, onImported}: ImportDialogProps) {
    const [filePath, setFilePath] = useState<string | null>(null)
    const [stage, setStage] = useState<Stage>('idle')
    const [validation, setValidation] = useState<ValidationSummarySource | null>(null)
    const [rowsInserted, setRowsInserted] = useState<number | null>(null)
    const [errorMessage, setErrorMessage] = useState<string | null>(null)

    const handlePickFile = useCallback(async () => {
        setErrorMessage(null)
        setValidation(null)
        setRowsInserted(null)
        try {
            const path = await PickImportFile()
            if (!path) {
                return
            }
            if (!isImportableFilePath(path)) {
                setErrorMessage(`Unsupported file type: ${fileNameFromPath(path)}. Choose a .csv or .json file.`)
                setStage('error')
                return
            }
            setFilePath(path)
            setStage('validating')
            const result = await ValidateImportFile(sessionID, schema, table.Name, path)
            setValidation(result)
            setStage('validated')
        } catch (err) {
            setErrorMessage(String(err))
            setStage('error')
        }
    }, [schema, sessionID, table.Name])

    const handleConfirmImport = useCallback(async () => {
        if (!filePath) {
            return
        }
        setStage('importing')
        setErrorMessage(null)
        try {
            const result = await ImportFile(sessionID, schema, table.Name, filePath)
            if (result.Mismatches.length > 0) {
                setValidation({Mismatches: result.Mismatches, RowCount: validation?.RowCount ?? 0})
                setStage('validated')
                return
            }
            setRowsInserted(result.RowsInserted)
            setStage('imported')
            onImported()
        } catch (err) {
            setErrorMessage(String(err))
            setStage('error')
        }
    }, [filePath, onImported, schema, sessionID, table.Name, validation])

    const mismatches = validation ? sortMismatches(validation.Mismatches) : []
    const canConfirm = stage === 'validated' && mismatches.length === 0

    return (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 p-4">
            <div className="flex max-h-[80vh] w-full max-w-2xl flex-col gap-4 overflow-auto rounded border border-ink-700 bg-ink-950 p-5">
                <div className="flex items-center justify-between">
                    <h2 className="text-sm font-semibold uppercase tracking-widest text-ink-200">
                        Import into {schema}.{table.Name}
                    </h2>
                    <button
                        type="button"
                        onClick={onClose}
                        className="rounded border border-ink-700 px-2 py-1 text-xs text-ink-300 hover:border-brass-500 hover:text-brass-400"
                    >
                        Close
                    </button>
                </div>

                <div className="flex items-center gap-3">
                    <button
                        type="button"
                        onClick={() => void handlePickFile()}
                        disabled={stage === 'validating' || stage === 'importing'}
                        className="rounded bg-brass-600 px-4 py-2 text-sm font-medium text-ink-950 transition-colors hover:bg-brass-500 disabled:cursor-not-allowed disabled:opacity-50"
                    >
                        {stage === 'validating' ? 'Validating…' : 'Choose CSV/JSON file'}
                    </button>
                    {filePath && <span className="truncate font-mono text-xs text-ink-400">{fileNameFromPath(filePath)}</span>}
                </div>

                {errorMessage && <p className="text-sm text-red-400">{errorMessage}</p>}

                {validation && (
                    <div className="flex flex-col gap-2 rounded border border-ink-800 bg-ink-900/40 p-3">
                        <p className={mismatches.length === 0 ? 'text-sm text-emerald-400' : 'text-sm text-red-400'}>
                            {summarizeValidation(validation)}
                        </p>
                        {mismatches.length > 0 && (
                            <ul className="flex max-h-48 flex-col gap-1 overflow-auto text-xs text-ink-300">
                                {mismatches.map((mismatch, index) => (
                                    <li key={index} className="font-mono">
                                        {formatMismatch(mismatch)}
                                    </li>
                                ))}
                            </ul>
                        )}
                    </div>
                )}

                {stage === 'imported' && rowsInserted !== null && (
                    <p className="text-sm text-emerald-400">Imported {rowsInserted} row(s) successfully.</p>
                )}

                <div className="flex items-center gap-3 border-t border-ink-800 pt-3">
                    <button
                        type="button"
                        onClick={() => void handleConfirmImport()}
                        disabled={!canConfirm}
                        className="rounded border border-emerald-700 px-4 py-2 text-sm font-medium text-emerald-400 transition-colors hover:border-emerald-500 hover:text-emerald-300 disabled:cursor-not-allowed disabled:opacity-50"
                    >
                        {stage === 'importing' ? 'Importing…' : 'Confirm import'}
                    </button>
                    {!canConfirm && stage === 'validated' && (
                        <span className="text-xs text-ink-500">Fix the mismatches above, then choose the file again.</span>
                    )}
                </div>
            </div>
        </div>
    )
}

export default ImportDialog
