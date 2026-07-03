import {useState} from 'react'
import type {ExportFormat} from './exportHelpers'
import {describeExportOutcome} from './exportHelpers'

interface ExportControlsProps {
    formats: ExportFormat[]
    onExport: (format: ExportFormat) => Promise<string>
}

const FORMAT_LABELS: Record<ExportFormat, string> = {
    csv: 'CSV',
    json: 'JSON',
    sql: 'SQL dump',
}

type ExportState = 'idle' | 'exporting' | 'saved' | 'error'

/**
 * Format-picker export buttons (tasks.md 7.1-7.3, spec.md §4.9), shared by
 * both export scopes (see exportHelpers.describeExportScope): the caller
 * supplies which formats apply and an `onExport` callback that calls the
 * right bound method for its scope (full table vs. current query result)
 * and format. A cancelled native save dialog (an empty path, see
 * exportHelpers.describeExportOutcome) is shown as no-op, not an error.
 */
function ExportControls({formats, onExport}: ExportControlsProps) {
    const [state, setState] = useState<ExportState>('idle')
    const [message, setMessage] = useState<string | null>(null)

    async function handleExport(format: ExportFormat) {
        setState('exporting')
        setMessage(null)
        try {
            const path = await onExport(format)
            const outcome = describeExportOutcome(path)
            if (outcome.status === 'cancelled') {
                setState('idle')
                return
            }
            setState('saved')
            setMessage(`Saved to ${outcome.path}`)
        } catch (err) {
            setState('error')
            setMessage(String(err))
        }
    }

    return (
        <div className="flex items-center gap-2">
            <span className="text-[10px] uppercase tracking-widest text-ink-500">Export</span>
            {formats.map((format) => (
                <button
                    key={format}
                    type="button"
                    onClick={() => void handleExport(format)}
                    disabled={state === 'exporting'}
                    className="rounded border border-ink-700 px-2 py-1 text-[10px] text-ink-200 transition-colors hover:border-brass-500 hover:text-brass-400 disabled:cursor-not-allowed disabled:opacity-50"
                >
                    {FORMAT_LABELS[format]}
                </button>
            ))}
            {state === 'saved' && message && <span className="text-[10px] text-emerald-400">{message}</span>}
            {state === 'error' && message && <span className="text-[10px] text-red-400">{message}</span>}
        </div>
    )
}

export default ExportControls
