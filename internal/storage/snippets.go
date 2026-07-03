package storage

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// ConnectionScope narrows ListSnippets to snippets usable from one specific
// connection: exactly-scoped snippets (ConnectionID) plus global snippets
// (ConnectionID IS NULL) of a compatible Engine. "Compatible" means the
// global snippet's Engine matches the connection's own Engine — a global
// Postgres snippet is not offered to a MySQL connection, since the query
// text itself is very likely dialect-specific (spec.md §4.7: "usable from
// any connection of a compatible engine").
type ConnectionScope struct {
	ConnectionID int64
	Engine       Engine
}

// SnippetFilter narrows ListSnippets' results; every field is optional and
// combined with AND when set.
//
//   - SearchText matches (case-insensitively) against Name or the raw
//     TagsJSON text, so one search box covers spec.md §4.7's "searchable by
//     name and tag" without a separate tags column/table. This is a Go-side
//     filtered SQL query (LIKE clauses), not a fetch-all-then-filter-in-Go
//     approach — same design choice task 3.5 made for saved connections'
//     list, kept for consistency and because it scales better once a user
//     accumulates many snippets.
//   - ForConnection, when non-nil, applies the compatible-engine scoping
//     described on ConnectionScope. A nil ForConnection returns snippets
//     regardless of scope, which is what the snippet-management UI (task
//     4.6) needs; app.go's ListSnippets sets ForConnection itself for the
//     scoped case (task 4.7's "run snippet" flow and any "snippets available
//     here" panel).
type SnippetFilter struct {
	SearchText    string
	ForConnection *ConnectionScope
}

// CreateSnippet inserts a new Snippet row and returns it re-read from the
// database. s.TagsJSON is expected to already be a JSON-encoded array (see
// app.go's tagsToJSON for the []string -> JSON boundary); an empty string
// defaults to "[]", matching Connection.ParamsJSON's existing
// empty-defaults convention.
func CreateSnippet(db *sql.DB, s *Snippet) (*Snippet, error) {
	tagsJSON := s.TagsJSON
	if tagsJSON == "" {
		tagsJSON = "[]"
	}

	res, err := db.Exec(
		`INSERT INTO snippets (connection_id, engine, name, body, tags_json)
		 VALUES (?, ?, ?, ?, ?)`,
		s.ConnectionID, s.Engine, s.Name, s.Body, tagsJSON,
	)
	if err != nil {
		return nil, fmt.Errorf("storage: create snippet %q: %w", s.Name, err)
	}

	id, err := res.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("storage: read new snippet id: %w", err)
	}

	return GetSnippet(db, id)
}

// GetSnippet reads a single Snippet back by ID.
func GetSnippet(db *sql.DB, id int64) (*Snippet, error) {
	var s Snippet

	err := db.QueryRow(
		`SELECT id, connection_id, engine, name, body, tags_json, created_at, updated_at
		 FROM snippets WHERE id = ?`, id,
	).Scan(&s.ID, &s.ConnectionID, &s.Engine, &s.Name, &s.Body, &s.TagsJSON, &s.CreatedAt, &s.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("storage: get snippet %d: %w", id, err)
	}

	return &s, nil
}

// ListSnippets returns every Snippet matching filter, ordered by name. See
// SnippetFilter's doc comment for how each field narrows the result.
func ListSnippets(db *sql.DB, filter SnippetFilter) ([]Snippet, error) {
	query := `SELECT id, connection_id, engine, name, body, tags_json, created_at, updated_at FROM snippets WHERE 1 = 1`
	var args []any

	if search := strings.TrimSpace(filter.SearchText); search != "" {
		query += ` AND (name LIKE ? ESCAPE '\' OR tags_json LIKE ? ESCAPE '\')`
		pattern := "%" + escapeLike(search) + "%"
		args = append(args, pattern, pattern)
	}

	if scope := filter.ForConnection; scope != nil {
		query += ` AND (connection_id = ? OR (connection_id IS NULL AND engine = ?))`
		args = append(args, scope.ConnectionID, scope.Engine)
	}

	query += ` ORDER BY name COLLATE NOCASE`

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("storage: list snippets: %w", err)
	}
	defer rows.Close()

	var snippets []Snippet
	for rows.Next() {
		var s Snippet
		if err := rows.Scan(&s.ID, &s.ConnectionID, &s.Engine, &s.Name, &s.Body, &s.TagsJSON, &s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, fmt.Errorf("storage: list snippets: scan row: %w", err)
		}
		snippets = append(snippets, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("storage: list snippets: %w", err)
	}

	return snippets, nil
}

// escapeLike escapes a user-supplied LIKE-pattern fragment's own wildcard
// characters (%, _) and escape character (\) so SearchText is matched as a
// literal substring rather than as a pattern the caller didn't intend to
// write.
func escapeLike(s string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return replacer.Replace(s)
}

// UpdateSnippet replaces every mutable field of an existing Snippet in
// place, keyed by s.ID, sets UpdatedAt to the current UTC time, and returns
// the row re-read from the database. CreatedAt is deliberately not part of
// this UPDATE. An empty s.TagsJSON defaults to "[]", same as CreateSnippet.
// Returns a wrapped sql.ErrNoRows if s.ID doesn't exist.
func UpdateSnippet(db *sql.DB, s *Snippet) (*Snippet, error) {
	tagsJSON := s.TagsJSON
	if tagsJSON == "" {
		tagsJSON = "[]"
	}
	updatedAt := time.Now().UTC().Format(time.RFC3339Nano)

	res, err := db.Exec(
		`UPDATE snippets
		 SET connection_id = ?, engine = ?, name = ?, body = ?, tags_json = ?, updated_at = ?
		 WHERE id = ?`,
		s.ConnectionID, s.Engine, s.Name, s.Body, tagsJSON, updatedAt, s.ID,
	)
	if err != nil {
		return nil, fmt.Errorf("storage: update snippet %d: %w", s.ID, err)
	}

	rows, err := res.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("storage: update snippet %d: read rows affected: %w", s.ID, err)
	}
	if rows == 0 {
		return nil, fmt.Errorf("storage: update snippet %d: %w", s.ID, sql.ErrNoRows)
	}

	return GetSnippet(db, s.ID)
}

// DeleteSnippet removes a Snippet row by ID. Returns a wrapped
// sql.ErrNoRows if id doesn't exist.
func DeleteSnippet(db *sql.DB, id int64) error {
	res, err := db.Exec(`DELETE FROM snippets WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("storage: delete snippet %d: %w", id, err)
	}

	rows, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("storage: delete snippet %d: read rows affected: %w", id, err)
	}
	if rows == 0 {
		return fmt.Errorf("storage: delete snippet %d: %w", id, sql.ErrNoRows)
	}

	return nil
}
