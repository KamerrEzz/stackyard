package docker

import (
	"testing"

	"stackyard/internal/storage"
)

func TestPostgresConnectionString_AllFieldsSet(t *testing.T) {
	svc := storage.Service{
		ID:                1,
		ProfileID:         2,
		Engine:            storage.EnginePostgres,
		ImageTag:          "postgres:16-alpine",
		HostPort:          5432,
		Username:          strPtr("appuser"),
		PasswordEncrypted: strPtr("s3cret"),
		DBName:            strPtr("appdb"),
		VolumeName:        "stackyard-vol-1",
	}

	got := PostgresConnectionString(svc)
	want := "postgres://appuser:s3cret@localhost:5432/appdb"
	if got != want {
		t.Errorf("PostgresConnectionString() = %q, want %q", got, want)
	}
}

// TestPostgresConnectionString_NilPassword covers the edge case nil-handling
// creates: a Service with no password set should omit the password segment
// entirely (no bare trailing ":") rather than embed a literal "nil"/empty
// placeholder that could be mistaken for a real, blank password.
func TestPostgresConnectionString_NilPassword(t *testing.T) {
	svc := storage.Service{
		ID:         2,
		ProfileID:  2,
		Engine:     storage.EnginePostgres,
		HostPort:   5433,
		Username:   strPtr("appuser"),
		DBName:     strPtr("appdb"),
		VolumeName: "stackyard-vol-2",
		// PasswordEncrypted is nil.
	}

	got := PostgresConnectionString(svc)
	want := "postgres://appuser@localhost:5433/appdb"
	if got != want {
		t.Errorf("PostgresConnectionString() = %q, want %q", got, want)
	}
}

// TestPostgresConnectionString_AllNilFallback covers a Service with
// Username/PasswordEncrypted/DBName all nil (shouldn't happen for a Postgres
// service created via App.CreateProfile's defaults, but must not panic):
// falls back to the official postgres image's own implicit defaults.
func TestPostgresConnectionString_AllNilFallback(t *testing.T) {
	svc := storage.Service{
		ID:         3,
		ProfileID:  2,
		Engine:     storage.EnginePostgres,
		HostPort:   5434,
		VolumeName: "stackyard-vol-3",
	}

	got := PostgresConnectionString(svc)
	want := "postgres://postgres@localhost:5434/postgres"
	if got != want {
		t.Errorf("PostgresConnectionString() = %q, want %q", got, want)
	}
}

// TestPostgresConnectionString_SpecialCharactersEscaped confirms a password
// containing URL-reserved characters is percent-encoded rather than
// corrupting the URL's structure (e.g. an "@" in the password must not be
// mistaken for the userinfo/host separator).
func TestPostgresConnectionString_SpecialCharactersEscaped(t *testing.T) {
	svc := storage.Service{
		ID:                4,
		ProfileID:         2,
		Engine:            storage.EnginePostgres,
		HostPort:          5435,
		Username:          strPtr("app user"),
		PasswordEncrypted: strPtr("p@ss:word/1"),
		DBName:            strPtr("appdb"),
		VolumeName:        "stackyard-vol-4",
	}

	got := PostgresConnectionString(svc)
	want := "postgres://app%20user:p%40ss%3Aword%2F1@localhost:5435/appdb"
	if got != want {
		t.Errorf("PostgresConnectionString() = %q, want %q", got, want)
	}
}
