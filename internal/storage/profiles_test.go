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
