package storage

import (
	"database/sql"
	"fmt"
	"time"
)

// CreateConnection inserts a new Connection row and returns it re-read from
// the database. c.ParamsJSON is expected to already be a JSON-encoded
// object (see app.go's paramsToJSON/paramsFromJSON for the
// map[string]string <-> JSON boundary); an empty string defaults to "{}",
// matching Snippet.TagsJSON's existing raw-JSON-string convention rather
// than introducing a typed Params field on Connection itself.
func CreateConnection(db *sql.DB, c *Connection) (*Connection, error) {
	paramsJSON := c.ParamsJSON
	if paramsJSON == "" {
		paramsJSON = "{}"
	}

	res, err := db.Exec(
		`INSERT INTO connections (name, engine, host, port, username, password_encrypted, database, params_json)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		c.Name, c.Engine, c.Host, c.Port, c.Username, c.PasswordEncrypted, c.Database, paramsJSON,
	)
	if err != nil {
		return nil, fmt.Errorf("storage: create connection %q: %w", c.Name, err)
	}

	id, err := res.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("storage: read new connection id: %w", err)
	}

	return GetConnection(db, id)
}

// GetConnection reads a single Connection back by ID.
func GetConnection(db *sql.DB, id int64) (*Connection, error) {
	var c Connection

	err := db.QueryRow(
		`SELECT id, name, engine, host, port, username, password_encrypted, database, params_json, last_used_at, migrations_folder
		 FROM connections WHERE id = ?`, id,
	).Scan(&c.ID, &c.Name, &c.Engine, &c.Host, &c.Port, &c.Username, &c.PasswordEncrypted, &c.Database, &c.ParamsJSON, &c.LastUsedAt, &c.MigrationsFolder)
	if err != nil {
		return nil, fmt.Errorf("storage: get connection %d: %w", id, err)
	}

	return &c, nil
}

// ListConnections returns every saved Connection, ordered by name.
func ListConnections(db *sql.DB) ([]Connection, error) {
	rows, err := db.Query(
		`SELECT id, name, engine, host, port, username, password_encrypted, database, params_json, last_used_at, migrations_folder
		 FROM connections ORDER BY name`,
	)
	if err != nil {
		return nil, fmt.Errorf("storage: list connections: %w", err)
	}
	defer rows.Close()

	var connections []Connection
	for rows.Next() {
		var c Connection
		if err := rows.Scan(&c.ID, &c.Name, &c.Engine, &c.Host, &c.Port, &c.Username, &c.PasswordEncrypted, &c.Database, &c.ParamsJSON, &c.LastUsedAt, &c.MigrationsFolder); err != nil {
			return nil, fmt.Errorf("storage: list connections: scan row: %w", err)
		}
		connections = append(connections, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("storage: list connections: %w", err)
	}

	return connections, nil
}

// TouchConnectionLastUsed sets a Connection's LastUsedAt to the current UTC
// time and returns the row re-read from the database. This is the only
// storage-layer function that ever changes LastUsedAt — see app.go's
// ConnectUsingSavedConnection for the single call site that decides when a
// saved connection counts as "used". Returns a wrapped sql.ErrNoRows if id
// doesn't exist.
func TouchConnectionLastUsed(db *sql.DB, id int64) (*Connection, error) {
	lastUsedAt := time.Now().UTC().Format(time.RFC3339Nano)

	res, err := db.Exec(`UPDATE connections SET last_used_at = ? WHERE id = ?`, lastUsedAt, id)
	if err != nil {
		return nil, fmt.Errorf("storage: touch connection %d last_used_at: %w", id, err)
	}

	rows, err := res.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("storage: touch connection %d last_used_at: read rows affected: %w", id, err)
	}
	if rows == 0 {
		return nil, fmt.Errorf("storage: touch connection %d last_used_at: %w", id, sql.ErrNoRows)
	}

	return GetConnection(db, id)
}

// SetConnectionMigrationsFolder sets a Connection's MigrationsFolder to
// folder and returns the row re-read from the database. This is the only
// storage-layer function that ever changes MigrationsFolder — kept isolated
// from CreateConnection's generic column list the same way
// TouchConnectionLastUsed is the sole writer of LastUsedAt, so saving a
// connection's name/host/credentials from the DB Client's connection form
// never resets or clobbers the migrations folder a user separately pointed
// internal/migrations (Phase 8) at. Returns a wrapped sql.ErrNoRows if id
// doesn't exist.
func SetConnectionMigrationsFolder(db *sql.DB, id int64, folder string) (*Connection, error) {
	res, err := db.Exec(`UPDATE connections SET migrations_folder = ? WHERE id = ?`, folder, id)
	if err != nil {
		return nil, fmt.Errorf("storage: set connection %d migrations folder: %w", id, err)
	}

	rows, err := res.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("storage: set connection %d migrations folder: read rows affected: %w", id, err)
	}
	if rows == 0 {
		return nil, fmt.Errorf("storage: set connection %d migrations folder: %w", id, sql.ErrNoRows)
	}

	return GetConnection(db, id)
}

// DeleteConnection removes a Connection row by ID. Snippets scoped to it are
// demoted to global rather than deleted (ON DELETE SET NULL), and its query
// history rows are removed as a consequence of ON DELETE CASCADE — see
// migrations.go's schema for both FK behaviors. Returns a wrapped
// sql.ErrNoRows if id doesn't exist.
func DeleteConnection(db *sql.DB, id int64) error {
	res, err := db.Exec(`DELETE FROM connections WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("storage: delete connection %d: %w", id, err)
	}

	rows, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("storage: delete connection %d: read rows affected: %w", id, err)
	}
	if rows == 0 {
		return fmt.Errorf("storage: delete connection %d: %w", id, sql.ErrNoRows)
	}

	return nil
}
