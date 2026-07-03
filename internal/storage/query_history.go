package storage

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// QueryHistoryFilter narrows ListQueryHistory's results (tasks.md 4.5,
// spec.md §4.10); every field is optional and combined with AND when set,
// mirroring SnippetFilter's own convention. ConnectionID, when non-zero,
// restricts results to that one saved Connection ("filterable by
// connection"); SearchText, when non-empty, matches (case-insensitively)
// against the logged query text ("searchable by text").
type QueryHistoryFilter struct {
	ConnectionID int64
	SearchText   string
}

// boolToSQLInt converts Success to the 0/1 representation
// query_history.success's CHECK constraint requires (see migrations.go).
func boolToSQLInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// CreateQueryHistoryEntry inserts a new QueryHistoryEntry row and returns it
// re-read from the database. ExecutedAt is always set here to the current
// UTC time, following CreateProfile/CreateSnippet's existing convention of
// generating timestamps in Go rather than relying on the schema's DEFAULT
// clause — any ExecutedAt already set on e is ignored.
func CreateQueryHistoryEntry(db *sql.DB, e *QueryHistoryEntry) (*QueryHistoryEntry, error) {
	executedAt := time.Now().UTC().Format(time.RFC3339Nano)

	res, err := db.Exec(
		`INSERT INTO query_history (connection_id, query_text, executed_at, duration_ms, success, rows_affected, error_message)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		e.ConnectionID, e.QueryText, executedAt, e.DurationMs, boolToSQLInt(e.Success), e.RowsAffected, e.ErrorMessage,
	)
	if err != nil {
		return nil, fmt.Errorf("storage: create query history entry for connection %d: %w", e.ConnectionID, err)
	}

	id, err := res.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("storage: read new query history entry id: %w", err)
	}

	return GetQueryHistoryEntry(db, id)
}

// GetQueryHistoryEntry reads a single QueryHistoryEntry back by ID.
func GetQueryHistoryEntry(db *sql.DB, id int64) (*QueryHistoryEntry, error) {
	var e QueryHistoryEntry
	var success int

	err := db.QueryRow(
		`SELECT id, connection_id, query_text, executed_at, duration_ms, success, rows_affected, error_message
		 FROM query_history WHERE id = ?`, id,
	).Scan(&e.ID, &e.ConnectionID, &e.QueryText, &e.ExecutedAt, &e.DurationMs, &success, &e.RowsAffected, &e.ErrorMessage)
	if err != nil {
		return nil, fmt.Errorf("storage: get query history entry %d: %w", id, err)
	}

	e.Success = success != 0
	return &e, nil
}

// ListQueryHistory returns every QueryHistoryEntry matching filter, most
// recently executed first. See QueryHistoryFilter's doc comment for how
// each field narrows the result. This is a Go-side SQL LIKE filter, not a
// fetch-all-then-filter-in-Go approach — the same design choice
// ListSnippets (task 4.6) and the saved-connections list (task 3.5) already
// made, since it scales better once a user accumulates a large history.
func ListQueryHistory(db *sql.DB, filter QueryHistoryFilter) ([]QueryHistoryEntry, error) {
	query := `SELECT id, connection_id, query_text, executed_at, duration_ms, success, rows_affected, error_message FROM query_history WHERE 1 = 1`
	var args []any

	if filter.ConnectionID != 0 {
		query += ` AND connection_id = ?`
		args = append(args, filter.ConnectionID)
	}

	if search := strings.TrimSpace(filter.SearchText); search != "" {
		query += ` AND query_text LIKE ? ESCAPE '\'`
		args = append(args, "%"+escapeLike(search)+"%")
	}

	query += ` ORDER BY executed_at DESC, id DESC`

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("storage: list query history: %w", err)
	}
	defer rows.Close()

	var entries []QueryHistoryEntry
	for rows.Next() {
		var e QueryHistoryEntry
		var success int
		if err := rows.Scan(&e.ID, &e.ConnectionID, &e.QueryText, &e.ExecutedAt, &e.DurationMs, &success, &e.RowsAffected, &e.ErrorMessage); err != nil {
			return nil, fmt.Errorf("storage: list query history: scan row: %w", err)
		}
		e.Success = success != 0
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("storage: list query history: %w", err)
	}

	return entries, nil
}

// DeleteQueryHistoryEntry removes a single QueryHistoryEntry row by ID,
// mirroring DeleteConnection/DeleteSnippet's existing per-row delete
// pattern. There is no bulk "clear all history" operation — a user removes
// entries one at a time. Returns a wrapped sql.ErrNoRows if id doesn't
// exist.
func DeleteQueryHistoryEntry(db *sql.DB, id int64) error {
	res, err := db.Exec(`DELETE FROM query_history WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("storage: delete query history entry %d: %w", id, err)
	}

	rows, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("storage: delete query history entry %d: read rows affected: %w", id, err)
	}
	if rows == 0 {
		return fmt.Errorf("storage: delete query history entry %d: %w", id, sql.ErrNoRows)
	}

	return nil
}
