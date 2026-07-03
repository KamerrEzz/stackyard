package storage

import (
	"database/sql"
	"fmt"
	"time"
)

// CreateProfile inserts a new Profile row and returns it with its
// generated ID and CreatedAt populated.
func CreateProfile(db *sql.DB, name string) (*Profile, error) {
	createdAt := time.Now().UTC().Format(time.RFC3339Nano)

	res, err := db.Exec(
		`INSERT INTO profiles (name, created_at) VALUES (?, ?)`,
		name, createdAt,
	)
	if err != nil {
		return nil, fmt.Errorf("storage: create profile %q: %w", name, err)
	}

	id, err := res.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("storage: read new profile id: %w", err)
	}

	return &Profile{ID: id, Name: name, CreatedAt: createdAt}, nil
}

// GetProfile reads a single Profile back by ID.
func GetProfile(db *sql.DB, id int64) (*Profile, error) {
	var p Profile

	err := db.QueryRow(
		`SELECT id, name, created_at FROM profiles WHERE id = ?`, id,
	).Scan(&p.ID, &p.Name, &p.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("storage: get profile %d: %w", id, err)
	}

	return &p, nil
}

// UpdateProfile renames an existing Profile in place and returns the
// updated row.
//
// Only Name is mutable here — CreatedAt is set once at creation and never
// changes, and ID is the immutable primary key — so "update" and "rename"
// are the same operation for Profile (unlike Service, which has several
// mutable fields; see UpdateService's doc comment in services.go for that
// judgment call). Returns a wrapped sql.ErrNoRows if id doesn't exist.
func UpdateProfile(db *sql.DB, id int64, name string) (*Profile, error) {
	res, err := db.Exec(`UPDATE profiles SET name = ? WHERE id = ?`, name, id)
	if err != nil {
		return nil, fmt.Errorf("storage: update profile %d: %w", id, err)
	}

	rows, err := res.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("storage: update profile %d: read rows affected: %w", id, err)
	}
	if rows == 0 {
		return nil, fmt.Errorf("storage: update profile %d: %w", id, sql.ErrNoRows)
	}

	return GetProfile(db, id)
}

// DeleteProfile removes a Profile row by ID.
//
// This is pure SQLite row deletion — the services table's ON DELETE CASCADE
// FK (migrations.go) removes that profile's Services as a consequence, but
// DeleteProfile itself does nothing Docker-related. spec.md §3.1 requires
// the user be asked explicitly before a profile's Docker volumes are
// removed; that confirmation and the actual volume teardown belong to a
// later docker-layer task, not this storage-layer function. Returns a
// wrapped sql.ErrNoRows if id doesn't exist.
func DeleteProfile(db *sql.DB, id int64) error {
	res, err := db.Exec(`DELETE FROM profiles WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("storage: delete profile %d: %w", id, err)
	}

	rows, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("storage: delete profile %d: read rows affected: %w", id, err)
	}
	if rows == 0 {
		return fmt.Errorf("storage: delete profile %d: %w", id, sql.ErrNoRows)
	}

	return nil
}

// ListProfiles returns every Profile, ordered by name.
//
// profiles.name is UNIQUE (migrations.go), so ordering by it is both
// deterministic and the most useful default for a UI list — callers don't
// need a separate sort step to show profiles alphabetically.
func ListProfiles(db *sql.DB) ([]Profile, error) {
	rows, err := db.Query(`SELECT id, name, created_at FROM profiles ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("storage: list profiles: %w", err)
	}
	defer rows.Close()

	var profiles []Profile
	for rows.Next() {
		var p Profile
		if err := rows.Scan(&p.ID, &p.Name, &p.CreatedAt); err != nil {
			return nil, fmt.Errorf("storage: list profiles: scan row: %w", err)
		}
		profiles = append(profiles, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("storage: list profiles: %w", err)
	}

	return profiles, nil
}
