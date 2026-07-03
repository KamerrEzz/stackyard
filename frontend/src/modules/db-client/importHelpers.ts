import type {importdata} from '../../../wailsjs/go/models'

/**
 * Renders one mismatch for the validation report list (tasks.md 7.4,
 * spec.md §4.9). `RowIndex` is 0-based per `Validate`'s own doc comment
 * (internal/importdata/validate.go); a row-scoped mismatch is shown 1-based
 * ("Row 1" for the first data row), while a file-scoped mismatch
 * (`RowIndex === -1`, currently only the unknown-column check) is shown as
 * `"File"` since it isn't tied to any one row.
 */
export function formatMismatch(mismatch: importdata.Mismatch): string {
    const location = mismatch.RowIndex < 0 ? 'File' : `Row ${mismatch.RowIndex + 1}`
    return `${location} — column "${mismatch.Column}": ${mismatch.Reason}`
}

/**
 * Sorts mismatches for display: file-level mismatches (RowIndex -1) first,
 * then by ascending row number — so a long report reads top-to-bottom in the
 * same order a user would fix them working through the file.
 */
export function sortMismatches(mismatches: readonly importdata.Mismatch[]): importdata.Mismatch[] {
    return [...mismatches].sort((a, b) => a.RowIndex - b.RowIndex)
}

export interface ValidationSummarySource {
    Mismatches: importdata.Mismatch[]
    RowCount: number
}

/**
 * One-line summary shown above the validation report (tasks.md 7.4's "sees
 * a validation report" requirement): either an all-clear count of rows ready
 * to import, or the count of mismatches blocking it — never both.
 */
export function summarizeValidation(result: ValidationSummarySource): string {
    if (result.Mismatches.length === 0) {
        return `All clear — ${result.RowCount} row${result.RowCount === 1 ? '' : 's'} ready to import.`
    }
    return `${result.Mismatches.length} mismatch${result.Mismatches.length === 1 ? '' : 'es'} found — fix the file and try again.`
}

/**
 * Whether path's extension is one ImportFile/ValidateImportFile (app.go)
 * accept — a client-side check purely for immediate UI feedback, matching
 * parseImportFile's own .csv/.json dispatch on the Go side exactly. The Go
 * side is still the actual source of truth for what it will accept.
 */
export function isImportableFilePath(path: string): boolean {
    const lower = path.toLowerCase()
    return lower.endsWith('.csv') || lower.endsWith('.json')
}

/**
 * Extracts just the file name from a full OS path returned by
 * `PickImportFile` (app.go), for a compact display label — handles both
 * `\` (Windows) and `/` (macOS/Linux) separators since Wails' native file
 * dialog returns a platform-native path.
 */
export function fileNameFromPath(path: string): string {
    const parts = path.split(/[\\/]/)
    return parts[parts.length - 1] ?? path
}
