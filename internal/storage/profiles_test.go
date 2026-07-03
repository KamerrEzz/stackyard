package storage

import "testing"

func TestCreateAndGetProfile_RoundTrip(t *testing.T) {
	db := openTestDB(t)

	created, err := CreateProfile(db, "local-dev")
	if err != nil {
		t.Fatalf("CreateProfile failed: %v", err)
	}
	if created.ID == 0 {
		t.Fatal("expected CreateProfile to populate a non-zero ID")
	}
	if created.CreatedAt == "" {
		t.Fatal("expected CreateProfile to populate CreatedAt")
	}

	fetched, err := GetProfile(db, created.ID)
	if err != nil {
		t.Fatalf("GetProfile(%d) failed: %v", created.ID, err)
	}

	if fetched.ID != created.ID {
		t.Errorf("ID mismatch: created=%d fetched=%d", created.ID, fetched.ID)
	}
	if fetched.Name != "local-dev" {
		t.Errorf("Name mismatch: want %q, got %q", "local-dev", fetched.Name)
	}
	if fetched.CreatedAt != created.CreatedAt {
		t.Errorf("CreatedAt mismatch: created=%q fetched=%q", created.CreatedAt, fetched.CreatedAt)
	}
}

func TestCreateProfile_DuplicateNameRejected(t *testing.T) {
	db := openTestDB(t)

	if _, err := CreateProfile(db, "duplicate-name"); err != nil {
		t.Fatalf("first CreateProfile failed: %v", err)
	}

	if _, err := CreateProfile(db, "duplicate-name"); err == nil {
		t.Fatal("expected the UNIQUE constraint on profiles.name to reject a duplicate, but it succeeded")
	}
}

func TestGetProfile_NotFound(t *testing.T) {
	db := openTestDB(t)

	if _, err := GetProfile(db, 12345); err == nil {
		t.Fatal("expected GetProfile on a non-existent ID to return an error")
	}
}

func TestUpdateProfile_RenamePersists(t *testing.T) {
	db := openTestDB(t)

	created, err := CreateProfile(db, "before-rename")
	if err != nil {
		t.Fatalf("CreateProfile failed: %v", err)
	}

	updated, err := UpdateProfile(db, created.ID, "after-rename")
	if err != nil {
		t.Fatalf("UpdateProfile failed: %v", err)
	}
	if updated.Name != "after-rename" {
		t.Errorf("expected returned Name %q, got %q", "after-rename", updated.Name)
	}

	fetched, err := GetProfile(db, created.ID)
	if err != nil {
		t.Fatalf("GetProfile after update failed: %v", err)
	}
	if fetched.Name != "after-rename" {
		t.Errorf("expected rename to persist, got Name=%q", fetched.Name)
	}
	if fetched.CreatedAt != created.CreatedAt {
		t.Errorf("expected CreatedAt to stay unchanged across a rename: created=%q fetched=%q", created.CreatedAt, fetched.CreatedAt)
	}
}

func TestUpdateProfile_NotFound(t *testing.T) {
	db := openTestDB(t)

	if _, err := UpdateProfile(db, 999999, "ghost"); err == nil {
		t.Fatal("expected UpdateProfile on a non-existent ID to return an error")
	}
}

func TestUpdateProfile_DuplicateNameRejected(t *testing.T) {
	db := openTestDB(t)

	if _, err := CreateProfile(db, "taken-name"); err != nil {
		t.Fatalf("CreateProfile failed: %v", err)
	}
	toRename, err := CreateProfile(db, "renameable")
	if err != nil {
		t.Fatalf("CreateProfile failed: %v", err)
	}

	if _, err := UpdateProfile(db, toRename.ID, "taken-name"); err == nil {
		t.Fatal("expected renaming into an already-taken name to be rejected by the UNIQUE constraint")
	}
}

func TestDeleteProfile_RemovesRow(t *testing.T) {
	db := openTestDB(t)

	created, err := CreateProfile(db, "to-delete")
	if err != nil {
		t.Fatalf("CreateProfile failed: %v", err)
	}

	if err := DeleteProfile(db, created.ID); err != nil {
		t.Fatalf("DeleteProfile failed: %v", err)
	}

	if _, err := GetProfile(db, created.ID); err == nil {
		t.Fatal("expected GetProfile on a deleted profile to return an error")
	}
}

func TestDeleteProfile_NotFound(t *testing.T) {
	db := openTestDB(t)

	if err := DeleteProfile(db, 999999); err == nil {
		t.Fatal("expected DeleteProfile on a non-existent ID to return an error")
	}
}

func TestDeleteProfile_CascadesToServices(t *testing.T) {
	db := openTestDB(t)

	profile, err := CreateProfile(db, "cascade-check")
	if err != nil {
		t.Fatalf("CreateProfile failed: %v", err)
	}

	svc, err := CreateService(db, &Service{
		ProfileID:  profile.ID,
		Engine:     EnginePostgres,
		ImageTag:   "postgres:16",
		HostPort:   5432,
		VolumeName: "vol-cascade",
	})
	if err != nil {
		t.Fatalf("CreateService failed: %v", err)
	}

	if err := DeleteProfile(db, profile.ID); err != nil {
		t.Fatalf("DeleteProfile failed: %v", err)
	}

	if _, err := GetService(db, svc.ID); err == nil {
		t.Fatal("expected the service to be gone after its profile was deleted (ON DELETE CASCADE)")
	}
}

func TestListProfiles_ReturnsAllOrderedByName(t *testing.T) {
	db := openTestDB(t)

	for _, name := range []string{"zebra", "apple", "mango"} {
		if _, err := CreateProfile(db, name); err != nil {
			t.Fatalf("CreateProfile(%q) failed: %v", name, err)
		}
	}

	profiles, err := ListProfiles(db)
	if err != nil {
		t.Fatalf("ListProfiles failed: %v", err)
	}

	if len(profiles) != 3 {
		t.Fatalf("expected 3 profiles, got %d", len(profiles))
	}

	want := []string{"apple", "mango", "zebra"}
	for i, p := range profiles {
		if p.Name != want[i] {
			t.Errorf("index %d: expected name %q, got %q", i, want[i], p.Name)
		}
	}
}

func TestListProfiles_EmptyWhenNoneExist(t *testing.T) {
	db := openTestDB(t)

	profiles, err := ListProfiles(db)
	if err != nil {
		t.Fatalf("ListProfiles failed: %v", err)
	}
	if len(profiles) != 0 {
		t.Errorf("expected no profiles, got %d", len(profiles))
	}
}
