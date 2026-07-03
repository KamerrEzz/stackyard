package dbengine

import (
	"context"
	"strings"
)

// PreparedStatement is one statement plus its bound parameters, run
// independently by ExecuteBatch — the unit the editable data grid's
// multi-row writes (tasks.md 4.1-4.4, e.g. DeleteTableRows in app.go) are
// built from.
type PreparedStatement struct {
	Text string
	Args []any
}

// StatementResult is the outcome of one statement executed independently
// within a batch (spec.md §4.6, tasks.md 4.1-4.4): Result and ErrorMessage
// are mutually exclusive depending on Success. ErrorMessage is a plain
// string rather than an error so this type marshals cleanly across the
// Wails/JSON bound-method boundary, matching the rest of this codebase's
// convention of surfacing the database's actual error text (see
// translatePgError/translateMySQLError) directly to the frontend.
type StatementResult struct {
	Statement    string
	Result       *QueryResult
	Success      bool
	ErrorMessage string
}

// ExecuteBatch runs every entry in statements against engine independently
// via engine.Exec, in order, collecting one StatementResult per entry
// regardless of whether an earlier statement failed — this is what
// satisfies spec.md §4.6's "runs statements independently and reports
// per-statement success/failure for each" for a batch of generated
// statements (as opposed to ExecuteMultiStatementText, which does the same
// for a single string of semicolon-separated raw SQL).
func ExecuteBatch(ctx context.Context, engine Engine, statements []PreparedStatement) []StatementResult {
	results := make([]StatementResult, len(statements))
	for i, stmt := range statements {
		result, err := engine.Exec(ctx, stmt.Text, stmt.Args...)
		if err != nil {
			results[i] = StatementResult{Statement: stmt.Text, Success: false, ErrorMessage: err.Error()}
			continue
		}
		results[i] = StatementResult{Statement: stmt.Text, Result: result, Success: true}
	}
	return results
}

// SplitStatements splits raw multi-statement SQL text on semicolons into
// individual statement strings, trimming surrounding whitespace and
// dropping empty segments (e.g. a trailing semicolon or a blank line
// between statements). This is a deliberately naive, text-level split — it
// does not understand string literals, so a statement containing a literal
// ";" inside a quoted value would be split incorrectly. That is an
// accepted, explicit scope limitation: every caller of
// ExecuteMultiStatementText in this codebase today runs batches of simple,
// programmatically generated statements (see app.go's grid-editing bound
// methods), never arbitrary user-typed SQL where a string literal might
// contain a semicolon. If a future Query Editor feature wires raw,
// user-typed multi-statement SQL through this same path, it will need a
// real SQL tokenizer (aware of quoting/escaping) instead of this function.
func SplitStatements(sql string) []string {
	parts := strings.Split(sql, ";")
	statements := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		statements = append(statements, trimmed)
	}
	return statements
}

// ExecuteMultiStatementText splits sql via SplitStatements and runs each
// resulting statement independently through ExecuteBatch, with no bound
// parameters — the entry point spec.md §4.6 describes for a single string
// containing multiple semicolon-separated statements (see SplitStatements'
// doc comment for the naive-split scope this accepts).
func ExecuteMultiStatementText(ctx context.Context, engine Engine, sql string) []StatementResult {
	texts := SplitStatements(sql)
	statements := make([]PreparedStatement, len(texts))
	for i, text := range texts {
		statements[i] = PreparedStatement{Text: text}
	}
	return ExecuteBatch(ctx, engine, statements)
}
