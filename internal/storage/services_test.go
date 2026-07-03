package storage

import "testing"

func strPtr(s string) *string { return &s }

func TestCreateAndGetService_RoundTrip(t *testing.T) {
	db := openTestDB(t)

	profile, err := CreateProfile(db, "svc-round-trip")
	if err != nil {
		t.Fatalf("CreateProfile failed: %v", err)
	}

	created, err := CreateService(db, &Service{
		ProfileID:         profile.ID,
		Engine:            EnginePostgres,
		ImageTag:          "postgres:16",
		HostPort:          5432,
		Username:          strPtr("appuser"),
		PasswordEncrypted: strPtr("enc:abc123"),
		DBName:            strPtr("appdb"),
		VolumeName:        "vol-round-trip",
	})
	if err != nil {
		t.Fatalf("CreateService failed: %v", err)
	}
	if created.ID == 0 {
		t.Fatal("expected CreateService to populate a non-zero ID")
	}

	fetched, err := GetService(db, created.ID)
	if err != nil {
		t.Fatalf("GetService(%d) failed: %v", created.ID, err)
	}

	if fetched.ProfileID != profile.ID {
		t.Errorf("ProfileID mismatch: want %d, got %d", profile.ID, fetched.ProfileID)
	}
	if fetched.Engine != EnginePostgres {
		t.Errorf("Engine mismatch: want %q, got %q", EnginePostgres, fetched.Engine)
	}
	if fetched.ImageTag != "postgres:16" {
		t.Errorf("ImageTag mismatch: got %q", fetched.ImageTag)
	}
	if fetched.HostPort != 5432 {
		t.Errorf("HostPort mismatch: got %d", fetched.HostPort)
	}
	if fetched.Username == nil || *fetched.Username != "appuser" {
		t.Errorf("Username mismatch: got %v", fetched.Username)
	}
	if fetched.PasswordEncrypted == nil || *fetched.PasswordEncrypted != "enc:abc123" {
		t.Errorf("PasswordEncrypted mismatch: got %v", fetched.PasswordEncrypted)
	}
	if fetched.DBName == nil || *fetched.DBName != "appdb" {
		t.Errorf("DBName mismatch: got %v", fetched.DBName)
	}
	if fetched.VolumeName != "vol-round-trip" {
		t.Errorf("VolumeName mismatch: got %q", fetched.VolumeName)
	}
}

func TestCreateService_NullableFieldsForRedis(t *testing.T) {
	db := openTestDB(t)

	profile, err := CreateProfile(db, "redis-nullable")
	if err != nil {
		t.Fatalf("CreateProfile failed: %v", err)
	}

	created, err := CreateService(db, &Service{
		ProfileID:  profile.ID,
		Engine:     EngineRedis,
		ImageTag:   "redis:7",
		HostPort:   6379,
		VolumeName: "vol-redis",
	})
	if err != nil {
		t.Fatalf("CreateService failed for a Redis service with nil username/password/db_name: %v", err)
	}

	fetched, err := GetService(db, created.ID)
	if err != nil {
		t.Fatalf("GetService failed: %v", err)
	}
	if fetched.Username != nil || fetched.PasswordEncrypted != nil || fetched.DBName != nil {
		t.Errorf("expected nullable fields to stay nil for a Redis service, got Username=%v PasswordEncrypted=%v DBName=%v",
			fetched.Username, fetched.PasswordEncrypted, fetched.DBName)
	}
}

func TestCreateService_RejectsUnknownProfile(t *testing.T) {
	db := openTestDB(t)

	_, err := CreateService(db, &Service{
		ProfileID:  999999,
		Engine:     EnginePostgres,
		ImageTag:   "postgres:16",
		HostPort:   5432,
		VolumeName: "vol-orphan",
	})
	if err == nil {
		t.Fatal("expected CreateService with a non-existent ProfileID to fail the FK constraint")
	}
}

func TestGetService_NotFound(t *testing.T) {
	db := openTestDB(t)

	if _, err := GetService(db, 999999); err == nil {
		t.Fatal("expected GetService on a non-existent ID to return an error")
	}
}

func TestListServicesByProfile_ReturnsOnlyThatProfilesServices(t *testing.T) {
	db := openTestDB(t)

	profileA, err := CreateProfile(db, "profile-a")
	if err != nil {
		t.Fatalf("CreateProfile failed: %v", err)
	}
	profileB, err := CreateProfile(db, "profile-b")
	if err != nil {
		t.Fatalf("CreateProfile failed: %v", err)
	}

	svcA1, err := CreateService(db, &Service{ProfileID: profileA.ID, Engine: EnginePostgres, ImageTag: "postgres:16", HostPort: 5432, VolumeName: "vol-a1"})
	if err != nil {
		t.Fatalf("CreateService failed: %v", err)
	}
	svcA2, err := CreateService(db, &Service{ProfileID: profileA.ID, Engine: EngineRedis, ImageTag: "redis:7", HostPort: 6379, VolumeName: "vol-a2"})
	if err != nil {
		t.Fatalf("CreateService failed: %v", err)
	}
	if _, err := CreateService(db, &Service{ProfileID: profileB.ID, Engine: EngineMySQL, ImageTag: "mysql:8", HostPort: 3306, VolumeName: "vol-b1"}); err != nil {
		t.Fatalf("CreateService failed: %v", err)
	}

	services, err := ListServicesByProfile(db, profileA.ID)
	if err != nil {
		t.Fatalf("ListServicesByProfile failed: %v", err)
	}

	if len(services) != 2 {
		t.Fatalf("expected 2 services for profile A, got %d", len(services))
	}
	if services[0].ID != svcA1.ID || services[1].ID != svcA2.ID {
		t.Errorf("expected services ordered by insertion id [%d, %d], got [%d, %d]",
			svcA1.ID, svcA2.ID, services[0].ID, services[1].ID)
	}
}

func TestListServicesByProfile_EmptyWhenProfileHasNone(t *testing.T) {
	db := openTestDB(t)

	profile, err := CreateProfile(db, "no-services")
	if err != nil {
		t.Fatalf("CreateProfile failed: %v", err)
	}

	services, err := ListServicesByProfile(db, profile.ID)
	if err != nil {
		t.Fatalf("ListServicesByProfile failed: %v", err)
	}
	if len(services) != 0 {
		t.Errorf("expected no services, got %d", len(services))
	}
}
