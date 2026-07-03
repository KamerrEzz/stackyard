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
// updated row. Returns a wrapped sql.ErrNoRows if id doesn't exist.
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

// DeleteProfile removes a Profile row by ID. Services belonging to the
// profile are removed as a consequence of the services table's ON DELETE
// CASCADE FK, but DeleteProfile itself does nothing Docker-related — volume
// teardown is a docker-layer concern. Returns a wrapped sql.ErrNoRows if id
// doesn't exist.
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

// DuplicateProfile copies the Profile identified by id — under a new,
// auto-generated name ("<original> (copy)", falling back to "(copy 2)",
// "(copy 3)", ... on further collisions, since profiles.name is UNIQUE) —
// along with every one of its Services. Each copied Service is inserted as
// a brand-new row (a new ID, never an alias of the original) with a freshly
// generated VolumeName so the duplicate never shares a Docker-managed
// volume with its source, but otherwise keeps the same Engine, ImageTag,
// HostPort, and credentials as the original. A duplicated Service's
// HostPort is therefore expected to collide with its source at start
// time — that collision is exactly what CheckProfilePortConflict/
// SuggestFreePort already surface to the user; DuplicateProfile does not
// attempt to resolve it itself. Returns a wrapped sql.ErrNoRows if id
// doesn't exist.
func DuplicateProfile(db *sql.DB, id int64) (*Profile, error) {
	original, err := GetProfile(db, id)
	if err != nil {
		return nil, fmt.Errorf("storage: duplicate profile %d: %w", id, err)
	}

	services, err := ListServicesByProfile(db, id)
	if err != nil {
		return nil, fmt.Errorf("storage: duplicate profile %d: %w", id, err)
	}

	name, err := nextDuplicateProfileName(db, original.Name)
	if err != nil {
		return nil, fmt.Errorf("storage: duplicate profile %d: %w", id, err)
	}

	copyProfile, err := CreateProfile(db, name)
	if err != nil {
		return nil, fmt.Errorf("storage: duplicate profile %d: create copy: %w", id, err)
	}

	for _, svc := range services {
		newSvc := svc
		newSvc.ID = 0
		newSvc.ProfileID = copyProfile.ID
		newSvc.VolumeName = fmt.Sprintf("stackyard-vol-profile-%d-%s", copyProfile.ID, svc.Engine)

		if _, err := CreateService(db, &newSvc); err != nil {
			return nil, fmt.Errorf("storage: duplicate profile %d: copy service %d: %w", id, svc.ID, err)
		}
	}

	return copyProfile, nil
}

func nextDuplicateProfileName(db *sql.DB, original string) (string, error) {
	candidate := fmt.Sprintf("%s (copy)", original)
	for n := 2; ; n++ {
		exists, err := profileNameExists(db, candidate)
		if err != nil {
			return "", err
		}
		if !exists {
			return candidate, nil
		}
		candidate = fmt.Sprintf("%s (copy %d)", original, n)
	}
}

func profileNameExists(db *sql.DB, name string) (bool, error) {
	var exists bool
	err := db.QueryRow(`SELECT EXISTS(SELECT 1 FROM profiles WHERE name = ?)`, name).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check profile name %q: %w", name, err)
	}
	return exists, nil
}

// ListProfiles returns every Profile, ordered by name.
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
