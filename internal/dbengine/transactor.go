package dbengine

import "context"

// Transactor is implemented by an Engine that can bind a sequence of
// statements to one underlying connection and commit or roll all of them
// back together as a single atomic unit, via that engine's own native
// transaction support (Postgres's pgxpool.Pool.Begin, MySQL's
// sql.DB.BeginTx) rather than a sequence of separate Engine.Exec calls —
// a pooled Engine.Exec has no guarantee two calls land on the same
// underlying connection, so raw "BEGIN"/"COMMIT" statements sent as
// independent Exec calls would not reliably form one real transaction.
//
// This is deliberately a separate, optional interface rather than an
// addition to Engine itself: the migrations Apply/Rollback engine (tasks.md
// 8.3-8.4) is the only caller that needs cross-statement atomicity, so
// every existing Engine consumer and test double that only exercises
// Connect/Query/Exec/ListSchemas/ListTables/ListForeignKeys/Close keeps
// compiling and behaving unchanged. A caller that needs atomicity type-
// asserts an Engine value against Transactor and handles the "this engine
// doesn't support it" case explicitly, the same way the standard library's
// optional interfaces (e.g. io.ReaderFrom) work.
type Transactor interface {
	// BeginTx starts a new transaction bound to a single connection. The
	// returned Tx's Exec calls all run against that same connection until
	// Commit or Rollback ends it.
	BeginTx(ctx context.Context) (Tx, error)
}

// Tx is one atomic sequence of statements bound to a single connection,
// returned by Transactor.BeginTx.
type Tx interface {
	// Exec runs query (with args bound via the engine's own placeholder
	// syntax, exactly like Engine.Exec) against this transaction's
	// connection.
	Exec(ctx context.Context, query string, args ...any) (*QueryResult, error)

	// Commit ends the transaction, making every Exec call since BeginTx
	// permanent. Calling any method on this Tx after Commit is undefined.
	Commit(ctx context.Context) error

	// Rollback ends the transaction, discarding every Exec call since
	// BeginTx. Calling any method on this Tx after Rollback is undefined.
	Rollback(ctx context.Context) error
}
