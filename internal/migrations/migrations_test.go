package migrations

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestSlugify(t *testing.T) {
	cases := []struct {
		name string
		want string
	}{
		{"Create Users Table", "create_users_table"},
		{"create_users_table", "create_users_table"},
		{"  leading and trailing  ", "leading_and_trailing"},
		{"Add!!!Index---To***Orders", "add_index_to_orders"},
		{"   ", ""},
		{"already-slug-like", "already_slug_like"},
		{"MixedCASE123", "mixedcase123"},
	}

	for _, c := range cases {
		if got := slugify(c.name); got != c.want {
			t.Errorf("slugify(%q) = %q, want %q", c.name, got, c.want)
		}
	}
}

func writeMigrationFile(t *testing.T, folder, name string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(folder, name), []byte("-- test\n"), 0o644); err != nil {
		t.Fatalf("write %q: %v", name, err)
	}
}

func TestCreateMigration_ScaffoldsUpAndDownFiles(t *testing.T) {
	folder := t.TempDir()

	m, err := CreateMigration(folder, "Create Users Table")
	if err != nil {
		t.Fatalf("CreateMigration failed: %v", err)
	}

	if m.Slug != "create_users_table" {
		t.Errorf("expected slug %q, got %q", "create_users_table", m.Slug)
	}
	if m.Version == 0 {
		t.Error("expected a non-zero Version")
	}

	wantStem := fmt.Sprintf("%014d_create_users_table", m.Version)
	if filepath.Base(m.UpPath) != wantStem+".up.sql" {
		t.Errorf("UpPath = %q, want basename %q", m.UpPath, wantStem+".up.sql")
	}
	if filepath.Base(m.DownPath) != wantStem+".down.sql" {
		t.Errorf("DownPath = %q, want basename %q", m.DownPath, wantStem+".down.sql")
	}

	upContent, err := os.ReadFile(m.UpPath)
	if err != nil {
		t.Fatalf("read up file: %v", err)
	}
	if len(upContent) == 0 {
		t.Error("expected the up file to have starter content, got empty file")
	}

	downContent, err := os.ReadFile(m.DownPath)
	if err != nil {
		t.Fatalf("read down file: %v", err)
	}
	if len(downContent) == 0 {
		t.Error("expected the down file to have starter content, got empty file")
	}
}

func TestCreateMigration_CreatesFolderIfMissing(t *testing.T) {
	folder := filepath.Join(t.TempDir(), "nested", "migrations")

	if _, err := CreateMigration(folder, "init"); err != nil {
		t.Fatalf("CreateMigration failed: %v", err)
	}

	info, err := os.Stat(folder)
	if err != nil || !info.IsDir() {
		t.Fatalf("expected folder %q to have been created, err=%v", folder, err)
	}
}

func TestCreateMigration_RejectsUnusableName(t *testing.T) {
	folder := t.TempDir()

	if _, err := CreateMigration(folder, "   "); err == nil {
		t.Fatal("expected an error for a name with no usable characters, got nil")
	}
}

func TestDiscoverMigrations_SortsByVersionNotFileTime(t *testing.T) {
	folder := t.TempDir()

	// Written in reverse chronological order (by mtime) but with
	// filename-version timestamps in ascending order, to prove sorting
	// reads the parsed version, not filesystem metadata.
	writeMigrationFile(t, folder, "20260703120500_third.up.sql")
	writeMigrationFile(t, folder, "20260703120500_third.down.sql")
	writeMigrationFile(t, folder, "20260703120300_first.up.sql")
	writeMigrationFile(t, folder, "20260703120300_first.down.sql")
	writeMigrationFile(t, folder, "20260703120400_second.up.sql")
	writeMigrationFile(t, folder, "20260703120400_second.down.sql")

	found, err := DiscoverMigrations(folder)
	if err != nil {
		t.Fatalf("DiscoverMigrations failed: %v", err)
	}

	if len(found) != 3 {
		t.Fatalf("expected 3 migrations, got %d", len(found))
	}

	wantOrder := []string{"first", "second", "third"}
	for i, want := range wantOrder {
		if found[i].Slug != want {
			t.Errorf("index %d: expected slug %q, got %q", i, want, found[i].Slug)
		}
	}
	if !(found[0].Version < found[1].Version && found[1].Version < found[2].Version) {
		t.Errorf("expected strictly increasing versions, got %v", found)
	}
}

func TestDiscoverMigrations_IgnoresStrayNonMigrationFiles(t *testing.T) {
	folder := t.TempDir()

	writeMigrationFile(t, folder, "20260703120000_only_one.up.sql")
	writeMigrationFile(t, folder, "20260703120000_only_one.down.sql")
	writeMigrationFile(t, folder, "README.md")
	writeMigrationFile(t, folder, "notes.txt")
	writeMigrationFile(t, folder, "not_a_migration.sql")

	found, err := DiscoverMigrations(folder)
	if err != nil {
		t.Fatalf("DiscoverMigrations failed: %v", err)
	}
	if len(found) != 1 {
		t.Fatalf("expected 1 migration (stray files ignored), got %d: %+v", len(found), found)
	}
	if found[0].Slug != "only_one" {
		t.Errorf("expected slug %q, got %q", "only_one", found[0].Slug)
	}
}

func TestDiscoverMigrations_ErrorsOnMissingUpFile(t *testing.T) {
	folder := t.TempDir()
	writeMigrationFile(t, folder, "20260703120000_incomplete.down.sql")

	if _, err := DiscoverMigrations(folder); err == nil {
		t.Fatal("expected an error when a migration's .up.sql file is missing, got nil")
	}
}

func TestDiscoverMigrations_ErrorsOnMissingDownFile(t *testing.T) {
	folder := t.TempDir()
	writeMigrationFile(t, folder, "20260703120000_incomplete.up.sql")

	if _, err := DiscoverMigrations(folder); err == nil {
		t.Fatal("expected an error when a migration's .down.sql file is missing, got nil")
	}
}

func TestDiscoverMigrations_EmptyFolderReturnsNoMigrations(t *testing.T) {
	folder := t.TempDir()

	found, err := DiscoverMigrations(folder)
	if err != nil {
		t.Fatalf("DiscoverMigrations on an empty folder failed: %v", err)
	}
	if len(found) != 0 {
		t.Errorf("expected no migrations, got %d", len(found))
	}
}

func TestMigration_Name(t *testing.T) {
	m := Migration{Version: 20260703120000, Slug: "create_users_table"}
	want := "20260703120000_create_users_table"
	if got := m.Name(); got != want {
		t.Errorf("Name() = %q, want %q", got, want)
	}
}
