// Package docker (this file, redis.go) extends compose.go's create-vs-
// reuse-vs-start orchestration to Redis. It reuses EnsureNetwork,
// EnsureVolume, ensureImage, ProfileNetworkName, and ServiceContainerName
// from compose.go unchanged — only the container spec and the connection-
// string format are Redis-specific.
//
// # Engine-specific decisions
//
//   - Redis has no username concept: unlike Postgres/MySQL/MongoDB,
//     svc.Username is never consulted by this file.
//   - Redis has no upfront database-name concept either: its numbered
//     logical databases (SELECT 0..15) are chosen at connection time, not
//     provisioned ahead of time, so svc.DBName is never consulted by this
//     file. See RedisConnectionString's doc comment for how this affects
//     the generated connection string specifically.
//   - The official redis image does not support a REDIS_PASSWORD-style
//     environment variable the way other official images use env vars for
//     credentials. Authentication is instead configured by overriding the
//     container's command to run `redis-server --requirepass <password>`.
//   - When svc.PasswordEncrypted is nil, the container runs with the
//     image's default command and therefore no authentication at all. This
//     is a deliberate judgment call for a local dev tool (favoring
//     zero-friction "just start it" ergonomics over forcing every profile
//     to carry a password), not an oversight — flagged here for review
//     rather than silently assumed.
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

const redisContainerPort = "6379/tcp"

const redisDataDir = "/data"

// EnsureRedisContainer makes sure a Redis container for svc exists and is
// running, creating (and pulling the image, if needed) or starting it as
// necessary. It mirrors EnsurePostgresContainer's create-vs-reuse-vs-start
// decision tree exactly (see compose.go's package doc comment). Returns the
// container ID either way.
//
// EnsureRedisContainer does NOT create the network or volume itself — call
// EnsureNetwork/EnsureVolume first (StartRedisEnvironment does this for you
// in the right order).
func (c *Client) EnsureRedisContainer(ctx context.Context, svc storage.Service) (string, error) {
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
	cfg, hostCfg := buildRedisContainerSpec(svc, networkName)

	resp, err := c.cli.ContainerCreate(ctx, cfg, hostCfg, nil, nil, name)
	if err != nil {
		return "", wrapContainerCreateErr(err)
	}
	if err := c.cli.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		return "", wrapContainerStartErr(err)
	}
	return resp.ID, nil
}

// StartRedisEnvironment is the entrypoint the "Start" binding calls for a
// Redis service: it ensures the profile's network, the service's volume,
// and the service's container all exist and the container ends up running,
// creating only what's missing.
func (c *Client) StartRedisEnvironment(ctx context.Context, svc storage.Service) error {
	if svc.Engine != storage.EngineRedis {
		return fmt.Errorf("docker: StartRedisEnvironment only supports engine %q, got %q", storage.EngineRedis, svc.Engine)
	}

	if _, err := c.EnsureNetwork(ctx, svc.ProfileID); err != nil {
		return err
	}
	if _, err := c.EnsureVolume(ctx, svc.VolumeName); err != nil {
		return err
	}
	if _, err := c.EnsureRedisContainer(ctx, svc); err != nil {
		return err
	}
	return nil
}

func buildRedisContainerSpec(svc storage.Service, networkName string) (*container.Config, *container.HostConfig) {
	port := nat.Port(redisContainerPort)

	var cmd []string
	if svc.PasswordEncrypted != nil {
		cmd = []string{"redis-server", "--requirepass", *svc.PasswordEncrypted}
	}

	cfg := &container.Config{
		Image:        svc.ImageTag,
		Cmd:          cmd,
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
				Target: redisDataDir,
			},
		},
		RestartPolicy: container.RestartPolicy{Name: container.RestartPolicyUnlessStopped},
	}

	return cfg, hostCfg
}

// RedisConnectionString builds the "redis://[:pass@]host:port" URL for svc
// (spec.md §3.3).
//
// Redis has no username concept, so unlike PostgresConnectionString this
// never consults svc.Username. When svc.PasswordEncrypted is set, it is
// encoded as a password-only userinfo segment (":pass@"); when nil, the
// userinfo segment is omitted entirely, producing a bare "redis://host:port"
// for a no-auth instance. PasswordEncrypted is treated as already usable as
// the literal password here — no decryption step exists yet (see
// compose.go's package doc comment for the same caveat on Postgres).
//
// svc.DBName is never consulted either: Redis has no database created
// upfront the way Postgres/MySQL/MongoDB do — its numbered logical
// databases are selected per-connection (e.g. a client's own SELECT/db
// option), not provisioned as part of starting the container. This function
// deliberately omits the optional trailing "/db" segment entirely rather
// than defaulting to "/0", so the generated string never implies a specific
// database selection that Stackyard didn't actually make.
func RedisConnectionString(svc storage.Service) string {
	var userInfo *url.Userinfo
	if svc.PasswordEncrypted != nil {
		userInfo = url.UserPassword("", *svc.PasswordEncrypted)
	}

	u := &url.URL{
		Scheme: "redis",
		User:   userInfo,
		Host:   fmt.Sprintf("%s:%d", localhost, svc.HostPort),
	}

	return u.String()
}
