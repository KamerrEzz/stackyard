// Package docker (this file, mysql.go) is the MySQL counterpart to
// compose.go: it turns a storage.Service with Engine == storage.EngineMySQL
// into running Docker resources using the official mysql image, following
// the exact same create-vs-reuse-vs-start-in-place semantics
// EnsurePostgresContainer establishes. Network naming (ProfileNetworkName),
// container naming (ServiceContainerName), the managed label, and
// ensureImage are all shared unchanged from compose.go — only the
// image-specific container spec (data dir, container port, env vars) and
// the connection-string format differ.
//
// # MySQL-specific container spec
//
//   - Container port: 3306/tcp (vs. Postgres's 5432/tcp).
//   - Data directory: /var/lib/mysql (vs. Postgres's
//     /var/lib/postgresql/data) — this is where the official mysql image
//     expects its volume mounted.
//
// # Credential mapping (svc.Username / svc.PasswordEncrypted / svc.DBName)
//
// storage.Service has exactly one username/password slot, shared across all
// 4 engines. The official mysql image distinguishes a root account (whose
// password is mandatory for the container to initialize at all) from an
// optional additional regular user. This file maps svc's single
// username/password pair onto that distinction as follows:
//
//   - If svc.Username is nil, empty, or exactly "root": svc is treated as
//     connecting as the root account. Only MYSQL_ROOT_PASSWORD (from
//     svc.PasswordEncrypted) and MYSQL_DATABASE (from svc.DBName) are set;
//     no MYSQL_USER is set, since the mysql image rejects MYSQL_USER=root
//     outright.
//   - Otherwise, svc.Username/svc.PasswordEncrypted are mapped to
//     MYSQL_USER/MYSQL_PASSWORD (a regular user scoped to svc.DBName by the
//     image's own entrypoint), and svc.PasswordEncrypted is *also* reused as
//     MYSQL_ROOT_PASSWORD, since the image requires a root password
//     regardless of whether a regular user exists and storage.Service has no
//     separate field to hold a distinct root password.
//
// Same known gap as compose.go: svc.PasswordEncrypted is treated as already
// usable as a literal env var value — no decryption step exists yet.
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

const mysqlContainerPort = "3306/tcp"

const mysqlDataDir = "/var/lib/mysql"

const mysqlRootUser = "root"

const (
	defaultMySQLConnUser = "root"
	defaultMySQLConnDB   = "mysql"
)

// EnsureMySQLContainer makes sure a MySQL container for svc exists and is
// running, creating (and pulling the image, if needed) or starting it as
// necessary. It follows the same create-vs-reuse-vs-start decision tree as
// EnsurePostgresContainer (compose.go). Returns the container ID either way.
//
// EnsureMySQLContainer does NOT create the network or volume itself — call
// EnsureNetwork/EnsureVolume first (StartMySQLEnvironment does this for you
// in the right order).
func (c *Client) EnsureMySQLContainer(ctx context.Context, svc storage.Service) (string, error) {
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
	cfg, hostCfg := buildMySQLContainerSpec(svc, networkName)

	resp, err := c.cli.ContainerCreate(ctx, cfg, hostCfg, nil, nil, name)
	if err != nil {
		return "", wrapContainerCreateErr(err)
	}
	if err := c.cli.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		return "", wrapContainerStartErr(err)
	}
	return resp.ID, nil
}

// StartMySQLEnvironment is the entrypoint the "Start" binding calls for a
// MySQL service: it ensures the profile's network, the service's volume, and
// the service's container all exist and the container ends up running,
// creating only what's missing.
func (c *Client) StartMySQLEnvironment(ctx context.Context, svc storage.Service) error {
	if svc.Engine != storage.EngineMySQL {
		return fmt.Errorf("docker: StartMySQLEnvironment only supports engine %q, got %q", storage.EngineMySQL, svc.Engine)
	}

	if _, err := c.EnsureNetwork(ctx, svc.ProfileID); err != nil {
		return err
	}
	if _, err := c.EnsureVolume(ctx, svc.VolumeName); err != nil {
		return err
	}
	if _, err := c.EnsureMySQLContainer(ctx, svc); err != nil {
		return err
	}
	return nil
}

func buildMySQLContainerSpec(svc storage.Service, networkName string) (*container.Config, *container.HostConfig) {
	port := nat.Port(mysqlContainerPort)

	regularUsername := ""
	if svc.Username != nil && *svc.Username != "" && *svc.Username != mysqlRootUser {
		regularUsername = *svc.Username
	}

	var env []string
	if regularUsername != "" {
		env = append(env, "MYSQL_USER="+regularUsername)
		if svc.PasswordEncrypted != nil {
			env = append(env, "MYSQL_PASSWORD="+*svc.PasswordEncrypted)
		}
	}
	if svc.PasswordEncrypted != nil {
		env = append(env, "MYSQL_ROOT_PASSWORD="+*svc.PasswordEncrypted)
	}
	if svc.DBName != nil {
		env = append(env, "MYSQL_DATABASE="+*svc.DBName)
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
				Target: mysqlDataDir,
			},
		},
		RestartPolicy: container.RestartPolicy{Name: container.RestartPolicyUnlessStopped},
	}

	return cfg, hostCfg
}

// MySQLConnectionString builds the canonical "mysql://user:pass@host:port/db"
// URL for svc.
//
// svc.Username, svc.PasswordEncrypted, and svc.DBName are nullable on
// storage.Service. This function handles nil the same way
// PostgresConnectionString (connstring.go) does:
//
//   - nil/empty Username falls back to "root", matching the credential
//     mapping buildMySQLContainerSpec uses (nil/empty/"root" Username all
//     mean "connect as root").
//   - nil PasswordEncrypted omits the password segment entirely, producing
//     "mysql://user@host:port/db" rather than a bogus placeholder.
//     PasswordEncrypted is treated as already usable as the literal password
//     here — no decryption step exists yet (see the package doc comment
//     above).
//   - nil/empty DBName falls back to "mysql", the schema always present in
//     a MySQL instance regardless of whether MYSQL_DATABASE was set.
//
// The string is always derived fresh from svc's current fields — nothing is
// cached. Username/password/db name are passed through net/url so any
// character that isn't URL-safe is percent-encoded rather than corrupting
// the URL's structure.
func MySQLConnectionString(svc storage.Service) string {
	username := defaultMySQLConnUser
	if svc.Username != nil && *svc.Username != "" {
		username = *svc.Username
	}

	dbName := defaultMySQLConnDB
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
		Scheme: "mysql",
		User:   userInfo,
		Host:   fmt.Sprintf("%s:%d", localhost, svc.HostPort),
		Path:   "/" + dbName,
	}

	return u.String()
}
