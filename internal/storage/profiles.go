package storage

import (
	"database/sql"
	"fmt"
	"time"
)

// CreateProfile inserts a new Profile row and returns it with its
// generated ID and CreatedAt populated.
//
// This is intentionally minimal — just enough persistence to exercise and
// prove the schema in this package's tests. Task 1.2 owns the full
// Profile/Service persistence layer (update, delete, list, and Service
// CRUD); avoid growing this beyond what task 0.4 needs.
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
