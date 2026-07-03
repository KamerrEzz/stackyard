// Package docker (this file, mongodb.go) extends compose.go's container
// orchestration to MongoDB. EnsureNetwork, EnsureVolume, and ensureImage are
// already engine-agnostic and are reused as-is; only the container spec
// (port, data directory, env vars) and the connection-string format are
// MongoDB-specific.
//
// # MONGO_INITDB_DATABASE and a nil/empty Service.DBName
//
// Unlike Postgres (POSTGRES_DB) or MySQL, the official mongo image does not
// require a database name upfront: MONGO_INITDB_DATABASE only selects which
// database is active while running init scripts under
// /docker-entrypoint-initdb.d, and MongoDB creates databases lazily on first
// write regardless of whether this variable is set. So when svc.DBName is
// nil or empty, MONGO_INITDB_DATABASE is omitted from the container's env
// entirely rather than defaulted to a placeholder — there is no Mongo
// equivalent of Postgres's image-default "postgres" database to fall back
// to, since Mongo ships with no pre-created non-admin database at all.
package docker

import (
	"context"
	"fmt"
	"net/url"
	"strconv"

	"stackyard/internal/storage"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/errdefs"
	"github.com/docker/go-connections/nat"
)

const mongoContainerPort = "27017/tcp"

const mongoDataDir = "/data/db"

const (
	defaultMongoConnUser = "root"
	defaultMongoConnDB   = "admin"
)

// EnsureMongoContainer makes sure a MongoDB container for svc exists and is
// running, creating (and pulling the image, if needed) or starting it as
// necessary. It follows the same create-vs-reuse-vs-start decision tree as
// EnsurePostgresContainer (see compose.go's package doc comment). Returns the
// container ID either way.
//
// EnsureMongoContainer does NOT create the network or volume itself — call
// EnsureNetwork/EnsureVolume first (StartMongoEnvironment does this for you
// in the right order).
func (c *Client) EnsureMongoContainer(ctx context.Context, svc storage.Service) (string, error) {
	name := ServiceContainerName(svc.ID)

	existing, err := c.cli.ContainerInspect(ctx, name)
	switch {
	case err == nil:
		if existing.State != nil && existing.State.Running {
			return existing.ID, nil
		}
		if startErr := c.cli.ContainerStart(ctx, existing.ID, container.StartOptions{}); startErr != nil {
			return "", wrapContainerStartErr(startErr)
		}
		return existing.ID, nil
	case errdefs.IsNotFound(err):
	default:
		return "", wrapContainerInspectErr(err)
	}

	if err := c.ensureImage(ctx, svc.ImageTag); err != nil {
		return "", err
	}

	networkName := ProfileNetworkName(svc.ProfileID)
	cfg, hostCfg := buildMongoContainerSpec(svc, networkName)

	resp, err := c.cli.ContainerCreate(ctx, cfg, hostCfg, nil, nil, name)
	if err != nil {
		return "", wrapContainerCreateErr(err)
	}
	if err := c.cli.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		return "", wrapContainerStartErr(err)
	}
	return resp.ID, nil
}

// StartMongoEnvironment is the entrypoint the "Start" binding calls for a
// MongoDB service: it ensures the profile's network, the service's volume,
// and the service's container all exist and the container ends up running,
// creating only what's missing.
func (c *Client) StartMongoEnvironment(ctx context.Context, svc storage.Service) error {
	if svc.Engine != storage.EngineMongoDB {
		return fmt.Errorf("docker: StartMongoEnvironment only supports engine %q, got %q", storage.EngineMongoDB, svc.Engine)
	}

	if _, err := c.EnsureNetwork(ctx, svc.ProfileID); err != nil {
		return err
	}
	if _, err := c.EnsureVolume(ctx, svc.VolumeName); err != nil {
		return err
	}
	if _, err := c.EnsureMongoContainer(ctx, svc); err != nil {
		return err
	}
	return nil
}

func buildMongoContainerSpec(svc storage.Service, networkName string) (*container.Config, *container.HostConfig) {
	port := nat.Port(mongoContainerPort)

	var env []string
	if svc.Username != nil {
		env = append(env, "MONGO_INITDB_ROOT_USERNAME="+*svc.Username)
	}
	if svc.PasswordEncrypted != nil {
		env = append(env, "MONGO_INITDB_ROOT_PASSWORD="+*svc.PasswordEncrypted)
	}
	if svc.DBName != nil && *svc.DBName != "" {
		env = append(env, "MONGO_INITDB_DATABASE="+*svc.DBName)
	}

	cfg := &container.Config{
		Image:        svc.ImageTag,
		Env:          env,
		ExposedPorts: nat.PortSet{port: struct{}{}},
		Labels: map[string]string{
			managedLabel:           "true",
			"stackyard.service_id": strconv.FormatInt(svc.ID, 10),
			"stackyard.profile_id": strconv.FormatInt(svc.ProfileID, 10),
		},
	}

	hostCfg := &container.HostConfig{
		NetworkMode: container.NetworkMode(networkName),
		PortBindings: nat.PortMap{
			port: []nat.PortBinding{
				{HostIP: "0.0.0.0", HostPort: strconv.Itoa(svc.HostPort)},
			},
		},
		Mounts: []mount.Mount{
			{
				Type:   mount.TypeVolume,
				Source: svc.VolumeName,
				Target: mongoDataDir,
			},
		},
		RestartPolicy: container.RestartPolicy{Name: container.RestartPolicyUnlessStopped},
	}

	return cfg, hostCfg
}

// MongoConnectionString builds the canonical "mongodb://user:pass@host:port/db"
// URL for svc.
//
// svc.Username, svc.PasswordEncrypted, and svc.DBName are nullable on
// storage.Service, handled with the same nil-safe philosophy as
// PostgresConnectionString (connstring.go), with one Mongo-specific
// difference in the fallback database name:
//
//   - nil/empty Username falls back to "root", matching the container
//     spec's own MONGO_INITDB_ROOT_USERNAME expectation for a service
//     Stackyard just created.
//   - nil PasswordEncrypted omits the password segment entirely, producing
//     "mongodb://user@host:port/db" rather than a bogus placeholder.
//     PasswordEncrypted is treated as already usable as the literal
//     password here — no decryption step exists yet (see compose.go).
//   - nil/empty DBName falls back to "admin". This is NOT a Mongo-native
//     "default database" — Mongo has none, see this file's package doc
//     comment on MONGO_INITDB_DATABASE. "admin" is used instead because
//     it's the database the root user (MONGO_INITDB_ROOT_USERNAME) actually
//     authenticates against, so the generated string works for login out of
//     the box rather than merely looking consistent with Postgres's own
//     fallback.
//
// The string is always derived fresh from svc's current fields — nothing is
// cached. Username/password/db name are passed through net/url so any
// character that isn't URL-safe is percent-encoded rather than corrupting
// the URL's structure.
func MongoConnectionString(svc storage.Service) string {
	username := defaultMongoConnUser
	if svc.Username != nil && *svc.Username != "" {
		username = *svc.Username
	}

	dbName := defaultMongoConnDB
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
		Scheme: "mongodb",
		User:   userInfo,
		Host:   fmt.Sprintf("%s:%d", localhost, svc.HostPort),
		Path:   "/" + dbName,
	}

	return u.String()
}
