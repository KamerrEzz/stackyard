import type {dbengine} from '../../../wailsjs/go/models'

export type ExportFormat = 'csv' | 'json' | 'sql'

/**
 * Which export scope applies and which formats it supports (tasks.md
 * 7.1-7.3, spec.md §4.9's "export scope: full table, or the current query
 * result set, user picks explicitly"). SQL dump is deliberately restricted
 * to the table scope — see export.go's ExportTableAsSQLDump doc comment for
 * why an arbitrary query result (possibly a join across tables, and only
 * carrying bare driver type names with no length/precision) can't produce a
 * dump spec.md's "importable into a fresh instance" requirement would
 * actually hold for.
 */
export interface ExportScopeInfo {
    scope: 'table' | 'result'
    availableFormats: ExportFormat[]
}

export function describeExportScope(hasTableContext: boolean): ExportScopeInfo {
    if (hasTableContext) {
        return {scope: 'table', availableFormats: ['csv', 'json', 'sql']}
    }
    return {scope: 'result', availableFormats: ['csv', 'json']}
}

/**
 * Builds ExportQueryResultAsCSV/ExportQueryResultAsJSON's argument shape
 * from a `dbengine.QueryResult` already held by this tab — the "current
 * query result" export scope reuses data the frontend already fetched via
 * RunQuery/RunMultiStatementQuery, rather than asking the Go side to re-run
 * the query or cache a last result of its own (see export.go's
 * ExportQueryResultAsCSV doc comment for the full rationale).
 */
export function buildQueryResultExportPayload(result: dbengine.QueryResult): {columnNames: string[]; rows: unknown[][]} {
    return {
        columnNames: (result.Columns ?? []).map((column) => column.Name),
        rows: result.Rows ?? [],
    }
}

export type ExportOutcome = {status: 'saved'; path: string} | {status: 'cancelled'}

/**
 * Interprets an Export* bound method's returned path: an empty string means
 * the user cancelled the native save dialog (see export.go's
 * saveExportFile doc comment — this is not an error condition), anything
 * else is the path the file was actually written to.
 */
export function describeExportOutcome(path: string): ExportOutcome {
    return path === '' ? {status: 'cancelled'} : {status: 'saved', path}
}
