package storage

import (
	"database/sql"
	"fmt"
)

// CreateService inserts a new Service row scoped to an existing Profile and
// returns it re-read from the database. s.ProfileID must reference an
// existing Profile; a nonexistent ProfileID surfaces as a wrapped
// FK-constraint driver error.
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
// Profile, ordered by ID (insertion order).
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
// s.ProfileID is intentionally not part of the UPDATE — reparenting a
// Service to a different Profile isn't supported. Returns a wrapped
// sql.ErrNoRows if s.ID doesn't exist.
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
// sibling services or the parent Profile. Returns a wrapped sql.ErrNoRows if
// id doesn't exist.
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
