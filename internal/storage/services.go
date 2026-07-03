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
