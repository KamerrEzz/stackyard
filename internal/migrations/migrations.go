// Package migrations manages schema migrations for the user's *target*
// database (Postgres or MySQL, spec.md §4.8) — not Stackyard's own local
// SQLite state, which internal/storage owns entirely on its own (see that
// package's doc comment for the explicit split).
//
// A migration is a pair of SQL files on disk, an "up" file and a "down"
// file, both named after the same <version>_<slug> stem:
//
//	20260703120000_create_users_table.up.sql
//	20260703120000_create_users_table.down.sql
//
// version is a UTC timestamp formatted as "20060102150405" (year, month,
// day, hour, minute, second — no separators), which is both a valid decimal
// integer and lexically sortable as a plain filename string, so "runs all
// pending migrations in order" (tasks.md 8.3, not this package's scope) can
// rely on either a numeric sort of Version or a plain string sort of the
// filenames themselves and get the same order. slug is name lowercased with
// every run of non-alphanumeric characters collapsed to a single
// underscore, leading/trailing underscores trimmed.
//
// This package only scaffolds and discovers migration files (CreateMigration,
// DiscoverMigrations) and bootstraps the schema_migrations tracking table
// inside the target database (BootstrapTrackingTable). It does not run
// migration SQL or track applied/pending state across runs — that is tasks.md
// 8.3 ("Apply") and 8.4 ("Rollback"), built on top of what this package
// exposes.
package migrations

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// versionLayout is the time.Format layout every migration version is
// generated from: a 14-digit UTC timestamp, second resolution.
const versionLayout = "20060102150405"

const (
	upFileSuffix   = ".up.sql"
	downFileSuffix = ".down.sql"
)

// migrationFileName matches "<14-digit version>_<slug>.<up|down>.sql" —
// the exact convention CreateMigration writes and DiscoverMigrations reads
// back. Any file in a migrations folder that doesn't match this pattern
// (including a non-.sql file, or a .sql file that doesn't start with a
// 14-digit version) is silently ignored by DiscoverMigrations rather than
// treated as an error, since a migrations folder is user-managed and may
// hold notes, READMEs, or other unrelated files.
var migrationFileName = regexp.MustCompile(`^(\d{14})_(.+)\.(up|down)\.sql$`)

// Migration is one migration file pair: a Version/slug identity plus the
// absolute paths of its up and down SQL files.
type Migration struct {
	Version  int64
	Slug     string
	UpPath   string
	DownPath string
}

// Name is Version and Slug joined the same way CreateMigration builds a
// migration's filename stem (without the .up.sql/.down.sql suffix or
// directory), e.g. "20260703120000_create_users_table".
func (m Migration) Name() string {
	return fmt.Sprintf("%d_%s", m.Version, m.Slug)
}

// CreateMigration scaffolds a new timestamped up/down file pair for name
// inside folder (tasks.md 8.1), creating folder if it doesn't already
// exist. The version is the current UTC time formatted per versionLayout;
// each file starts with a short templated SQL comment identifying which
// migration it belongs to, for the user to fill in — this package never
// generates migration SQL content itself. Returns an error without writing
// anything if name has no usable characters once slugified, or if a file
// at the target path already exists (most likely two calls landing within
// the same second — CreateMigration does not retry or disambiguate this
// case, so a caller hitting it should simply ask the user to try again).
func CreateMigration(folder, name string) (Migration, error) {
	slug := slugify(name)
	if slug == "" {
		return Migration{}, fmt.Errorf("migrations: create migration: name %q has no usable characters after slugifying", name)
	}

	if err := os.MkdirAll(folder, 0o755); err != nil {
		return Migration{}, fmt.Errorf("migrations: create migration: create folder %q: %w", folder, err)
	}

	versionStr := time.Now().UTC().Format(versionLayout)
	version, err := strconv.ParseInt(versionStr, 10, 64)
	if err != nil {
		return Migration{}, fmt.Errorf("migrations: create migration: parse generated version %q: %w", versionStr, err)
	}

	stem := versionStr + "_" + slug
	upPath := filepath.Join(folder, stem+upFileSuffix)
	downPath := filepath.Join(folder, stem+downFileSuffix)

	if _, err := os.Stat(upPath); err == nil {
		return Migration{}, fmt.Errorf("migrations: create migration: %q already exists", upPath)
	}

	upContent := fmt.Sprintf("-- Migration: %s\n-- Write the SQL that applies this migration below.\n", name)
	downContent := fmt.Sprintf("-- Migration: %s\n-- Write the SQL that reverses this migration below.\n", name)

	if err := os.WriteFile(upPath, []byte(upContent), 0o644); err != nil {
		return Migration{}, fmt.Errorf("migrations: create migration: write up file: %w", err)
	}
	if err := os.WriteFile(downPath, []byte(downContent), 0o644); err != nil {
		return Migration{}, fmt.Errorf("migrations: create migration: write down file: %w", err)
	}

	return Migration{Version: version, Slug: slug, UpPath: upPath, DownPath: downPath}, nil
}

// DiscoverMigrations reads folder and returns every migration file pair
// found, sorted by Version ascending (tasks.md 8.1) — the order tasks.md
// 8.3's "run all pending migrations in order" builds on top of. Files that
// don't match the "<version>_<slug>.<up|down>.sql" naming convention are
// ignored. A version+slug that has only an up file or only a down file (its
// pair is missing) is reported as an error, since a later Apply/Rollback
// against a half-formed migration should never be attempted silently.
// Sorting is purely by the parsed Version — DiscoverMigrations never
// consults filesystem modification times, so migrations created out of
// their filename-timestamp order still come back correctly ordered.
func DiscoverMigrations(folder string) ([]Migration, error) {
	entries, err := os.ReadDir(folder)
	if err != nil {
		return nil, fmt.Errorf("migrations: discover migrations: read folder %q: %w", folder, err)
	}

	type pairing struct {
		version  int64
		slug     string
		upPath   string
		downPath string
	}

	pairs := make(map[string]*pairing)
	var order []string

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		matches := migrationFileName.FindStringSubmatch(entry.Name())
		if matches == nil {
			continue
		}

		versionStr, slug, kind := matches[1], matches[2], matches[3]
		version, err := strconv.ParseInt(versionStr, 10, 64)
		if err != nil {
			continue
		}

		key := versionStr + "_" + slug
		p, ok := pairs[key]
		if !ok {
			p = &pairing{version: version, slug: slug}
			pairs[key] = p
			order = append(order, key)
		}

		fullPath := filepath.Join(folder, entry.Name())
		switch kind {
		case "up":
			p.upPath = fullPath
		case "down":
			p.downPath = fullPath
		}
	}

	result := make([]Migration, 0, len(pairs))
	for _, key := range order {
		p := pairs[key]
		if p.upPath == "" {
			return nil, fmt.Errorf("migrations: discover migrations: %q is missing its .up.sql file", key)
		}
		if p.downPath == "" {
			return nil, fmt.Errorf("migrations: discover migrations: %q is missing its .down.sql file", key)
		}
		result = append(result, Migration{
			Version:  p.version,
			Slug:     p.slug,
			UpPath:   p.upPath,
			DownPath: p.downPath,
		})
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Version < result[j].Version
	})

	return result, nil
}

// slugify lowercases name and collapses every run of characters outside
// [a-z0-9] into a single underscore, trimming any leading/trailing
// underscore left behind. "Create Users Table!" -> "create_users_table",
// "  " -> "".
func slugify(name string) string {
	var b strings.Builder

	for _, r := range strings.ToLower(name) {
		switch {
		case r >= 'a' && r <= 'z' || r >= '0' && r <= '9':
			b.WriteByte(byte(r))
		default:
			if b.Len() > 0 && b.String()[b.Len()-1] != '_' {
				b.WriteByte('_')
			}
		}
	}

	return strings.TrimRight(b.String(), "_")
}
