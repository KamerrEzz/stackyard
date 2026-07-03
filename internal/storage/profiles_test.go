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

func TestDuplicateProfile_CopiesProfileAndServices(t *testing.T) {
	db := openTestDB(t)

	original, err := CreateProfile(db, "source")
	if err != nil {
		t.Fatalf("CreateProfile failed: %v", err)
	}

	svc1, err := CreateService(db, &Service{
		ProfileID:         original.ID,
		Engine:            EnginePostgres,
		ImageTag:          "postgres:16",
		HostPort:          5432,
		Username:          strPtr("appuser"),
		PasswordEncrypted: strPtr("enc:abc123"),
		DBName:            strPtr("appdb"),
		VolumeName:        "vol-source-postgres",
	})
	if err != nil {
		t.Fatalf("CreateService failed: %v", err)
	}
	svc2, err := CreateService(db, &Service{
		ProfileID:  original.ID,
		Engine:     EngineRedis,
		ImageTag:   "redis:7",
		HostPort:   6379,
		VolumeName: "vol-source-redis",
	})
	if err != nil {
		t.Fatalf("CreateService failed: %v", err)
	}

	dup, err := DuplicateProfile(db, original.ID)
	if err != nil {
		t.Fatalf("DuplicateProfile failed: %v", err)
	}

	if dup.ID == original.ID {
		t.Fatal("expected DuplicateProfile to create a new profile ID, not alias the original")
	}
	if dup.Name != "source (copy)" {
		t.Errorf("expected duplicate name %q, got %q", "source (copy)", dup.Name)
	}

	originalStillIntact, err := GetProfile(db, original.ID)
	if err != nil {
		t.Fatalf("expected original profile to remain after duplication, got error: %v", err)
	}
	if originalStillIntact.Name != "source" {
		t.Errorf("expected original profile's name to remain %q, got %q", "source", originalStillIntact.Name)
	}

	dupServices, err := ListServicesByProfile(db, dup.ID)
	if err != nil {
		t.Fatalf("ListServicesByProfile failed: %v", err)
	}
	if len(dupServices) != 2 {
		t.Fatalf("expected 2 duplicated services, got %d", len(dupServices))
	}

	for _, s := range dupServices {
		if s.ID == svc1.ID || s.ID == svc2.ID {
			t.Errorf("expected duplicated service to have a new ID, got original ID %d", s.ID)
		}
		if s.ProfileID != dup.ID {
			t.Errorf("expected duplicated service ProfileID %d, got %d", dup.ID, s.ProfileID)
		}
	}

	pgCopy := findServiceByEngine(dupServices, EnginePostgres)
	if pgCopy == nil {
		t.Fatal("expected a duplicated postgres service")
	}
	if pgCopy.ImageTag != "postgres:16" || pgCopy.HostPort != 5432 {
		t.Errorf("expected duplicated postgres service to keep ImageTag/HostPort, got %+v", pgCopy)
	}
	if pgCopy.Username == nil || *pgCopy.Username != "appuser" {
		t.Errorf("expected duplicated postgres service to keep Username, got %v", pgCopy.Username)
	}
	if pgCopy.VolumeName == "vol-source-postgres" {
		t.Error("expected duplicated service to get a freshly generated VolumeName, not reuse the source's")
	}

	redisCopy := findServiceByEngine(dupServices, EngineRedis)
	if redisCopy == nil {
		t.Fatal("expected a duplicated redis service")
	}
	if redisCopy.HostPort != 6379 {
		t.Errorf("expected duplicated redis service to keep HostPort 6379, got %d", redisCopy.HostPort)
	}

	originalServices, err := ListServicesByProfile(db, original.ID)
	if err != nil {
		t.Fatalf("ListServicesByProfile for original failed: %v", err)
	}
	if len(originalServices) != 2 {
		t.Errorf("expected original profile's services to remain untouched, got %d", len(originalServices))
	}
}

func findServiceByEngine(services []Service, engine Engine) *Service {
	for i := range services {
		if services[i].Engine == engine {
			return &services[i]
		}
	}
	return nil
}

func TestDuplicateProfile_NameCollisionIsNumbered(t *testing.T) {
	db := openTestDB(t)

	original, err := CreateProfile(db, "source")
	if err != nil {
		t.Fatalf("CreateProfile failed: %v", err)
	}
	if _, err := CreateProfile(db, "source (copy)"); err != nil {
		t.Fatalf("CreateProfile failed: %v", err)
	}

	dup, err := DuplicateProfile(db, original.ID)
	if err != nil {
		t.Fatalf("DuplicateProfile failed: %v", err)
	}
	if dup.Name != "source (copy 2)" {
		t.Errorf("expected duplicate name %q when %q is already taken, got %q", "source (copy 2)", "source (copy)", dup.Name)
	}
}

func TestDuplicateProfile_NotFound(t *testing.T) {
	db := openTestDB(t)

	if _, err := DuplicateProfile(db, 999999); err == nil {
		t.Fatal("expected DuplicateProfile on a non-existent ID to return an error")
	}
}

func TestDuplicateProfile_SourceWithNoServices(t *testing.T) {
	db := openTestDB(t)

	original, err := CreateProfile(db, "empty-profile")
	if err != nil {
		t.Fatalf("CreateProfile failed: %v", err)
	}

	dup, err := DuplicateProfile(db, original.ID)
	if err != nil {
		t.Fatalf("DuplicateProfile failed: %v", err)
	}

	services, err := ListServicesByProfile(db, dup.ID)
	if err != nil {
		t.Fatalf("ListServicesByProfile failed: %v", err)
	}
	if len(services) != 0 {
		t.Errorf("expected duplicate of a service-less profile to also have no services, got %d", len(services))
	}
}
