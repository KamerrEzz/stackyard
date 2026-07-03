// Package docker (this file, connstring.go) turns a storage.Service into the
// connection URL a user pastes into a client/app to reach it (spec.md §3.3,
// tasks.md 1.6).
//
// This lives in the docker package rather than internal/storage or a new
// package for the same reason buildPostgresContainerSpec (compose.go) does:
// it is a pure, Docker-topology-aware transform of a storage.Service — the
// host/port pair it produces is specifically "how to reach the container
// Stackyard itself just started," not a generic URL formatter. Phase 3's
// urlparse.go (internal/dbengine) is the inverse direction (parse a
// user-supplied string into fields for an arbitrary, possibly-remote
// connection) and is deliberately a separate concern; this file only builds
// strings for Stackyard-managed local containers.
package docker

import (
	"fmt"
	"net/url"

	"stackyard/internal/storage"
)

// localhost is the only host Phase 1 MVP ever generates a connection string
// for — these are always Stackyard-managed containers reachable via their
// published HostPort on the machine running Stackyard. Connecting to an
// external/remote database is Module 2's concern (task 4.1's connect-by-URL
// form), not this one.
const localhost = "localhost"

// defaultPostgresConnUser/DB mirror the official postgres image's own
// implicit defaults (the same ones app.go's defaultPostgres* constants use
// when creating a service), so a nil Username/DBName falls back to what the
// container would actually accept rather than an arbitrary placeholder.
const (
	defaultPostgresConnUser = "postgres"
	defaultPostgresConnDB   = "postgres"
)

// PostgresConnectionString builds the canonical
// "postgres://user:pass@host:port/db" URL (spec.md §3.3) for svc.
//
// svc.Username, svc.PasswordEncrypted, and svc.DBName are nullable on
// storage.Service (models.go) — Redis has no equivalent, so the struct-wide
// nullability is a schema-level given, not something a Postgres Service is
// expected to hit in practice (App.CreateProfile always sets all three, see
// app.go's defaultPostgres* constants). This function still handles nil
// gracefully rather than panicking:
//
//   - nil/empty Username falls back to "postgres", the official image's own
//     default superuser.
//   - nil PasswordEncrypted omits the password segment entirely, producing
//     "postgres://user@host:port/db" (no trailing ":") rather than a bogus
//     placeholder a user might paste and mistake for a real credential.
//     Note: PasswordEncrypted is treated as already usable as the literal
//     password here, matching compose.go's buildPostgresContainerSpec's
//     documented "known gap" — no decryption step exists yet anywhere in
//     the codebase for Phase 1 MVP scope.
//   - nil/empty DBName falls back to "postgres", the official image's own
//     default database.
//
// The string is *always* derived fresh from svc's current fields — nothing
// here is cached/memoized, so a caller that re-fetches svc after the user
// edits credentials/port and restarts the service gets an up-to-date string
// for free (spec.md §3.3's third acceptance criterion).
//
// Username/password/db name are passed through net/url so any character
// that isn't URL-safe (e.g. "@", ":", "/" in a password) is percent-encoded
// rather than corrupting the URL's structure.
func PostgresConnectionString(svc storage.Service) string {
	username := defaultPostgresConnUser
	if svc.Username != nil && *svc.Username != "" {
		username = *svc.Username
	}

	dbName := defaultPostgresConnDB
	if svc.DBName != nil && *svc.DBName != "" {
		dbName = *svc.DBName
	}

	var userInfo *url.Userinfo
	if svc.PasswordEncrypted != nil {
		userInfo = url.UserPassword(username, *svc.PasswordEncrypted)
	} else {
		userInfo = url.User(username)
	}

	u := &url.URL{
		Scheme: "postgres",
		User:   userInfo,
		Host:   fmt.Sprintf("%s:%d", localhost, svc.HostPort),
		Path:   "/" + dbName,
	}

	return u.String()
}
