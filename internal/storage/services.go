package storage

import (
	"database/sql"
	"fmt"
)

// CreateService inserts a new Service row scoped to an existing Profile and
// returns it re-read from the database (so defaults/NULL handling reflect
// what SQLite actually stored, not just the struct passed in).
//
// s.ProfileID must reference an existing Profile — services.profile_id is
// NOT NULL with an ON DELETE CASCADE FK (migrations.go), so a nonexistent
// ProfileID surfaces here as a wrapped FK-constraint driver error rather
// than a distinct sentinel, matching how CreateProfile already surfaces
// the profiles.name UNIQUE violation the same way.
func CreateService(db *sql.DB, s *Service) (*Service, error) {
	res, err := db.Exec(
		`INSERT INTO services (profile_id, engine, image_tag, host_port, username, password_encrypted, db_name, volume_name)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		s.ProfileID, s.Engine, s.ImageTag, s.HostPort, s.Username, s.PasswordEncrypted, s.DBName, s.VolumeName,
	)
	if err != nil {
		return nil, fmt.Errorf("storage: create service for profile %d: %w", s.ProfileID, err)
	}

	id, err := res.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("storage: read new service id: %w", err)
	}

	return GetService(db, id)
}

// GetService reads a single Service back by ID.
func GetService(db *sql.DB, id int64) (*Service, error) {
	var s Service

	err := db.QueryRow(
		`SELECT id, profile_id, engine, image_tag, host_port, username, password_encrypted, db_name, volume_name
		 FROM services WHERE id = ?`, id,
	).Scan(&s.ID, &s.ProfileID, &s.Engine, &s.ImageTag, &s.HostPort, &s.Username, &s.PasswordEncrypted, &s.DBName, &s.VolumeName)
	if err != nil {
		return nil, fmt.Errorf("storage: get service %d: %w", id, err)
	}

	return &s, nil
}

// ListServicesByProfile returns every Service belonging to the given
// Profile, ordered by ID (insertion order) — the order services were added
// to the profile, which is the most predictable default for a UI list.
func ListServicesByProfile(db *sql.DB, profileID int64) ([]Service, error) {
	rows, err := db.Query(
		`SELECT id, profile_id, engine, image_tag, host_port, username, password_encrypted, db_name, volume_name
		 FROM services WHERE profile_id = ? ORDER BY id`, profileID,
	)
	if err != nil {
		return nil, fmt.Errorf("storage: list services for profile %d: %w", profileID, err)
	}
	defer rows.Close()

	var services []Service
	for rows.Next() {
		var s Service
		if err := rows.Scan(&s.ID, &s.ProfileID, &s.Engine, &s.ImageTag, &s.HostPort, &s.Username, &s.PasswordEncrypted, &s.DBName, &s.VolumeName); err != nil {
			return nil, fmt.Errorf("storage: list services for profile %d: scan row: %w", profileID, err)
		}
		services = append(services, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("storage: list services for profile %d: %w", profileID, err)
	}

	return services, nil
}

// UpdateService replaces every mutable field of an existing Service in
// place, keyed by s.ID, and returns the row re-read from the database.
//
// Judgment call: this takes a full *Service rather than individual fields
// or a partial patch struct. Service has 7 mutable columns beyond its ID
// and ProfileID (Engine, ImageTag, HostPort, Username, PasswordEncrypted,
// DBName, VolumeName) — Phase 2 (tasks 2.1-2.3) adds MySQL/MongoDB/Redis
// config on top of the same struct, and a full-struct replace means that
// growth never requires widening this function's parameter list. Callers
// that only want to change one field (e.g. a rename) fetch first via
// GetService, mutate the field, then call UpdateService — the same pattern
// CreateService/GetService already establish for round-tripping state.
//
// s.ProfileID is intentionally NOT part of the UPDATE — a Service moving
// to a different Profile isn't a real use case anywhere in spec.md (§3.1
// only duplicates/renames/deletes a whole profile; §3.4 only resets one
// service's volume in place), so reparenting isn't exposed here. Returns a
// wrapped sql.ErrNoRows if s.ID doesn't exist.
func UpdateService(db *sql.DB, s *Service) (*Service, error) {
	res, err := db.Exec(
		`UPDATE services
		 SET engine = ?, image_tag = ?, host_port = ?, username = ?, password_encrypted = ?, db_name = ?, volume_name = ?
		 WHERE id = ?`,
		s.Engine, s.ImageTag, s.HostPort, s.Username, s.PasswordEncrypted, s.DBName, s.VolumeName, s.ID,
	)
	if err != nil {
		return nil, fmt.Errorf("storage: update service %d: %w", s.ID, err)
	}

	rows, err := res.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("storage: update service %d: read rows affected: %w", s.ID, err)
	}
	if rows == 0 {
		return nil, fmt.Errorf("storage: update service %d: %w", s.ID, sql.ErrNoRows)
	}

	return GetService(db, s.ID)
}

// DeleteService removes a single Service row by ID without touching its
// sibling services or the parent Profile.
//
// Like DeleteProfile, this is pure SQLite row deletion — any Docker
// container/volume cleanup for the service (spec.md §3.4's "reset data")
// is a docker-layer concern for a later task. Returns a wrapped
// sql.ErrNoRows if id doesn't exist.
func DeleteService(db *sql.DB, id int64) error {
	res, err := db.Exec(`DELETE FROM services WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("storage: delete service %d: %w", id, err)
	}

	rows, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("storage: delete service %d: read rows affected: %w", id, err)
	}
	if rows == 0 {
		return fmt.Errorf("storage: delete service %d: %w", id, sql.ErrNoRows)
	}

	return nil
}
