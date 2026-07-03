// Package docker (this file, connstring.go) turns a storage.Service into the
// connection URL a user pastes into a client/app to reach it.
//
// This lives in the docker package rather than internal/storage for the same
// reason buildPostgresContainerSpec (compose.go) does: it is a pure,
// Docker-topology-aware transform of a storage.Service — the host/port pair
// it produces is specifically "how to reach the container Stackyard itself
// just started," not a generic URL formatter. Phase 3's urlparse.go
// (internal/dbengine) is the inverse direction (parse a user-supplied string
// into fields for an arbitrary, possibly-remote connection) and is a
// deliberately separate concern.
package docker

import (
	"fmt"
	"net/url"

	"stackyard/internal/storage"
)

// localhost is the only host Phase 1 MVP ever generates a connection string
// for — these are always Stackyard-managed containers reachable via their
// published HostPort on the machine running Stackyard.
const localhost = "localhost"

const (
	defaultPostgresConnUser = "postgres"
	defaultPostgresConnDB   = "postgres"
)

// PostgresConnectionString builds the canonical
// "postgres://user:pass@host:port/db" URL for svc.
//
// svc.Username, svc.PasswordEncrypted, and svc.DBName are nullable on
// storage.Service. This function handles nil gracefully rather than
// panicking:
//
//   - nil/empty Username falls back to "postgres", the official image's own
//     default superuser.
//   - nil PasswordEncrypted omits the password segment entirely, producing
//     "postgres://user@host:port/db" rather than a bogus placeholder.
//     PasswordEncrypted is treated as already usable as the literal
//     password here — no decryption step exists yet (see compose.go).
//   - nil/empty DBName falls back to "postgres", the official image's own
//     default database.
//
// The string is always derived fresh from svc's current fields — nothing is
// cached. Username/password/db name are passed through net/url so any
// character that isn't URL-safe is percent-encoded rather than corrupting
// the URL's structure.
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
