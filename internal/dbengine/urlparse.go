package dbengine

import (
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"stackyard/internal/storage"
)

// ConnectionFields is the form-field breakdown produced by parsing one of
// the four canonical connection-string formats (spec.md §3.3): the
// Connect-by-URL feature (spec.md §4.1) autofills a connection form from
// these fields instead of requiring the user to type each one by hand.
type ConnectionFields struct {
	Engine   storage.Engine
	Host     string
	Port     int
	Username string
	Password string
	Database string
	Params   url.Values
}

var schemeToEngine = map[string]storage.Engine{
	"postgres": storage.EnginePostgres,
	"mysql":    storage.EngineMySQL,
	"mongodb":  storage.EngineMongoDB,
	"redis":    storage.EngineRedis,
}

const supportedSchemesList = "postgres, mysql, mongodb, redis"

var invalidPortPattern = regexp.MustCompile(`invalid port "?:?([^"]*)"? after host`)

// ParseConnectionString parses one of the four canonical connection-string
// formats (mysql://, postgres://, mongodb://, redis://) into
// ConnectionFields, delegating the scheme/userinfo/host/port/path/query
// breakdown to net/url and layering engine-specific validation on top.
//
// Every rejection names the offending part of the input (spec.md §4.1's
// "not a generic failure" requirement) instead of returning a bare
// "invalid URL" error.
func ParseConnectionString(raw string) (*ConnectionFields, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, fmt.Errorf("empty connection string")
	}

	if !strings.Contains(trimmed, "://") {
		return nil, fmt.Errorf("missing \"://\" scheme separator in connection string: expected one of postgres://, mysql://, mongodb://, redis://")
	}

	parsed, err := url.Parse(trimmed)
	if err != nil {
		return nil, translateURLError(trimmed, err)
	}

	if parsed.Scheme == "" {
		return nil, fmt.Errorf("missing scheme in connection string: expected one of %s", supportedSchemesList)
	}

	engine, ok := schemeToEngine[parsed.Scheme]
	if !ok {
		return nil, fmt.Errorf("unsupported scheme %q: expected one of %s", parsed.Scheme, supportedSchemesList)
	}

	hostname := parsed.Hostname()
	if hostname == "" {
		return nil, fmt.Errorf("missing host in connection string")
	}

	if strings.HasSuffix(parsed.Host, ":") {
		return nil, fmt.Errorf("missing port number after \":\" in connection string")
	}

	port := 0
	if portStr := parsed.Port(); portStr != "" {
		port, err = strconv.Atoi(portStr)
		if err != nil {
			return nil, fmt.Errorf("invalid port %q: not a valid port number", portStr)
		}
		if port < 1 || port > 65535 {
			return nil, fmt.Errorf("invalid port %d: must be between 1 and 65535", port)
		}
	}

	var username, password string
	if parsed.User != nil {
		username = parsed.User.Username()
		password, _ = parsed.User.Password()
	}

	if engine == storage.EngineRedis && username != "" {
		return nil, fmt.Errorf("invalid userinfo %q: redis connection strings have no username, use redis://:password@host:port", username)
	}

	database := strings.TrimPrefix(parsed.Path, "/")
	if strings.Contains(database, "/") {
		return nil, fmt.Errorf("invalid database segment %q: expected a single path segment", parsed.Path)
	}

	if (engine == storage.EnginePostgres || engine == storage.EngineMySQL) && database == "" {
		return nil, fmt.Errorf("missing database name in connection string: required for %s", engine)
	}

	return &ConnectionFields{
		Engine:   engine,
		Host:     hostname,
		Port:     port,
		Username: username,
		Password: password,
		Database: database,
		Params:   parsed.Query(),
	}, nil
}

// translateURLError turns a net/url parse error, which names the offending
// token but wraps it in url.Error's generic "parse %q: ..." framing, into a
// message that leads with the specific problem so it reads well as an
// inline form error.
func translateURLError(raw string, err error) error {
	msg := err.Error()

	if m := invalidPortPattern.FindStringSubmatch(msg); m != nil {
		return fmt.Errorf("invalid port %q: not a valid port number", m[1])
	}
	if strings.Contains(msg, "missing protocol scheme") {
		return fmt.Errorf("missing scheme in connection string: expected one of %s", supportedSchemesList)
	}
	if strings.Contains(msg, "invalid userinfo") {
		return fmt.Errorf("invalid userinfo in connection string: username/password contain characters that must be percent-encoded")
	}
	if strings.Contains(msg, "invalid character") && strings.Contains(msg, "host") {
		return fmt.Errorf("invalid host in connection string: %s", raw)
	}

	return fmt.Errorf("malformed connection string: %w", err)
}
