import {describe, expect, it} from 'vitest'
import type {dbengine} from '../../../wailsjs/go/models'
import {collapseStatementResults, summarizeStatementResults} from './multiStatementHelpers'

function statementResult(overrides: Partial<dbengine.StatementResult>): dbengine.StatementResult {
    return {
        Statement: 'SELECT 1',
        Success: true,
        ErrorMessage: '',
        ...overrides,
    } as dbengine.StatementResult
}

describe('collapseStatementResults', () => {
    it('collapses a single successful statement to the legacy single-result view', () => {
        const result = {Columns: [], Rows: [], RowsAffected: 1, LastInsertID: 0, Duration: 100} as dbengine.QueryResult
        const view = collapseStatementResults([statementResult({Success: true, Result: result})])

        expect(view).toEqual({mode: 'single-success', result})
    })

    it('collapses a single failed statement to a plain error message view', () => {
        const view = collapseStatementResults([statementResult({Success: false, ErrorMessage: 'syntax error'})])

        expect(view).toEqual({mode: 'single-failure', errorMessage: 'syntax error'})
    })

    it('renders as the multi-statement list once there are 2+ statements, even if all succeeded', () => {
        const results = [statementResult({Success: true}), statementResult({Success: true})]
        const view = collapseStatementResults(results)

        expect(view.mode).toBe('multi')
        expect(view.mode === 'multi' && view.results).toHaveLength(2)
    })

    it('renders as the multi-statement list when some of several statements failed', () => {
        const results = [
            statementResult({Success: true}),
            statementResult({Success: false, ErrorMessage: 'boom'}),
            statementResult({Success: true}),
        ]
        const view = collapseStatementResults(results)

        expect(view.mode).toBe('multi')
        expect(view.mode === 'multi' && view.results.filter((r) => !r.Success)).toHaveLength(1)
    })
})

describe('summarizeStatementResults', () => {
    it('counts successes and failures across the batch', () => {
        const results = [
            statementResult({Success: true}),
            statementResult({Success: false}),
            statementResult({Success: true}),
        ]

        expect(summarizeStatementResults(results)).toBe('3 statement(s) executed — 2 succeeded, 1 failed')
    })

    it('reports all-success and all-failure batches correctly', () => {
        expect(summarizeStatementResults([statementResult({Success: true}), statementResult({Success: true})])).toBe(
            '2 statement(s) executed — 2 succeeded, 0 failed',
        )
        expect(summarizeStatementResults([statementResult({Success: false})])).toBe(
            '1 statement(s) executed — 0 succeeded, 1 failed',
        )
    })
})
