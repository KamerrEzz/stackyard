package main

import (
	"context"
	"fmt"

	"stackyard/internal/dbengine"
)

// RunMultiStatementQuery executes every semicolon-separated statement in
// query independently against the live Engine behind sessionID (spec.md
// §4.6's "Multi-statement execution... runs statements independently and
// reports per-statement success/failure for each"), building on
// internal/dbengine's ExecuteMultiStatementText/SplitStatements. Unlike
// RunQuery, which executes exactly one statement and returns its single
// dbengine.QueryResult, this always returns one dbengine.StatementResult per
// statement found in query, in source order, regardless of whether an
// earlier statement failed — the same "keep going, report per-entry"
// contract ExecuteBatch/DeleteTableRows already establish. A script
// containing exactly one statement still goes through this same path and
// still returns a single-element slice; collapsing that back to the
// pre-existing single-result view is the frontend's job (see
// QueryEditor.tsx), not this method's.
//
// This shares RunQuery's cancellation mechanism: the context this executes
// under is derived here and its CancelFunc registered in a.queryCancels for
// exactly the duration of the call, so a concurrently-arriving
// CancelQuery(sessionID) call cancels a multi-statement run exactly like it
// would a single-statement one.
//
// The one top-level error this returns (as opposed to a per-statement
// failure inside the returned slice) is reserved for conditions that mean
// execution never started at all: an unknown session, or a query containing
// no statements once SplitStatements discards blank segments (e.g. an
// empty string or one consisting only of whitespace/semicolons).
//
// Every statement's outcome is logged to query_history individually via
// recordStatementResultHistory, one entry per statement — the same
// per-entry logging precedent DeleteTableRows already established for a
// batch of independently-run statements, rather than one aggregate entry for
// the whole script.
func (a *App) RunMultiStatementQuery(sessionID, query string) ([]dbengine.StatementResult, error) {
	session, ok := a.getQuerySession(sessionID)
	if !ok {
		return nil, fmt.Errorf("run multi-statement query: no open connection session %q", sessionID)
	}
	if len(dbengine.SplitStatements(query)) == 0 {
		return nil, fmt.Errorf("run multi-statement query: no SQL statements found")
	}

	ctx, cancel := context.WithCancel(a.ctx)
	a.putQueryCancel(sessionID, cancel)
	defer func() {
		a.popQueryCancel(sessionID)
		cancel()
	}()

	results := dbengine.ExecuteMultiStatementText(ctx, session.engine, query)
	for _, result := range results {
		a.recordStatementResultHistory(session.connectionID, result)
	}

	return results, nil
}
