import type {dbengine} from '../../../wailsjs/go/models'

export type StatementResultsView =
    | {mode: 'single-success'; result: dbengine.QueryResult | undefined}
    | {mode: 'single-failure'; errorMessage: string}
    | {mode: 'multi'; results: dbengine.StatementResult[]}

/**
 * Decides how the Query Editor renders `RunMultiStatementQuery`'s result
 * (spec.md §4.6). A script containing exactly one statement collapses back
 * to the pre-existing single-result view — a bare `QueryResult` on success,
 * or a plain error message on failure — so the common single-statement case
 * never visually regresses from before multi-statement support existed. A
 * script with 2+ statements always renders as the per-statement list
 * instead, even when every statement happened to succeed, since there is no
 * single `QueryResult` to collapse to once there's more than one.
 */
export function collapseStatementResults(results: readonly dbengine.StatementResult[]): StatementResultsView {
    if (results.length === 1) {
        const [only] = results
        if (only.Success) {
            return {mode: 'single-success', result: only.Result}
        }
        return {mode: 'single-failure', errorMessage: only.ErrorMessage}
    }
    return {mode: 'multi', results: [...results]}
}

/**
 * One-line summary of a multi-statement run's outcome, shown next to the Run
 * button in place of the single-statement "N row(s) affected/returned"
 * summary (see `collapseStatementResults`'s `'multi'` branch).
 */
export function summarizeStatementResults(results: readonly dbengine.StatementResult[]): string {
    const succeeded = results.filter((r) => r.Success).length
    const failed = results.length - succeeded
    return `${results.length} statement(s) executed — ${succeeded} succeeded, ${failed} failed`
}
