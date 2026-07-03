// Package docker (this file, compose.go) turns a storage.Service into
// running Docker resources — network, volume, container — entirely through
// the Engine API. No docker-compose.yml or any file is ever written to disk;
// every step below is the SDK equivalent of a manual `docker network create`
// / `docker volume create` / `docker run` invocation.
//
// # Naming / scoping decisions
//
//   - Network: one network PER PROFILE, not per service, named
//     "stackyard-profile-<profileID>" — services within the same profile
//     need to reach each other by container name, which requires a shared
//     network.
//   - Volume: uses storage.Service.VolumeName as-is.
//   - Container: named "stackyard-service-<serviceID>", deterministic from
//     the Service's own primary key so repeated "Start" calls find the same
//     container again.
//
// # Idempotency / create-vs-reuse-vs-start logic
//
// EnsureNetwork and EnsureVolume both inspect-then-create: if the named
// resource already exists, it's reused unchanged.
//
// EnsurePostgresContainer decides what "start" means for a container that
// may already exist in three shapes:
//
//  1. No container with this name exists yet -> create it (pulling the image
//     first if needed), then start it.
//  2. A container with this name exists and is already running -> no-op,
//     return its ID.
//  3. A container with this name exists but is NOT running -> start it in
//     place rather than removing and recreating it, preserving the existing
//     volume mount and container identity across stop/start cycles.
//
// What this deliberately does NOT do: if an existing container's
// configuration (image tag, port mapping, env vars) no longer matches the
// Service row, this function does not detect or reconcile that drift — it
// just starts the stale container as-is.
//
// # Known gap
//
// storage.Service.PasswordEncrypted is documented (plan.md §4) as holding an
// *encrypted* value at rest. This file treats it as already usable as the
// literal POSTGRES_PASSWORD env var value — no decryption step exists yet
// anywhere in the codebase.
package docker

import (
	"context"
	"fmt"
	"io"
	"strconv"

	"stackyard/internal/storage"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/volume"
	"github.com/docker/docker/errdefs"
	"github.com/docker/go-connections/nat"
)

const postgresContainerPort = "5432/tcp"

const postgresDataDir = "/var/lib/postgresql/data"

// managedLabel marks every network/volume/container Stackyard creates, so a
// future cleanup routine can filter on it without relying on name-prefix
// parsing alone.
const managedLabel = "stackyard.managed"

// ProfileNetworkName returns the deterministic network name for a profile.
func ProfileNetworkName(profileID int64) string {
	return fmt.Sprintf("stackyard-profile-%d", profileID)
}

// ServiceContainerName returns the deterministic container name for a
// service, used both to create the container and to look it up again on a
// later "Start" call.
func ServiceContainerName(serviceID int64) string {
	return fmt.Sprintf("stackyard-service-%d", serviceID)
}

// EnsureNetwork creates the profile-scoped bridge network if it doesn't
// already exist, or returns the existing network's ID unchanged. Safe to
// call repeatedly.
func (c *Client) EnsureNetwork(ctx context.Context, profileID int64) (string, error) {
	name := ProfileNetworkName(profileID)

	existing, err := c.cli.NetworkInspect(ctx, name, network.InspectOptions{})
	if err == nil {
		return existing.ID, nil
	}
	if !errdefs.IsNotFound(err) {
		return "", wrapNetworkInspectErr(err)
	}

	resp, err := c.cli.NetworkCreate(ctx, name, network.CreateOptions{
		Driver: "bridge",
		Labels: map[string]string{
			managedLabel:           "true",
			"stackyard.profile_id": strconv.FormatInt(profileID, 10),
		},
	})
	if err != nil {
		return "", wrapNetworkCreateErr(err)
	}
	return resp.ID, nil
}

// EnsureVolume creates the named volume if it doesn't already exist, or
// returns the existing volume's name unchanged. Safe to call repeatedly.
func (c *Client) EnsureVolume(ctx context.Context, volumeName string) (string, error) {
	existing, err := c.cli.VolumeInspect(ctx, volumeName)
	if err == nil {
		return existing.Name, nil
	}
	if !errdefs.IsNotFound(err) {
		return "", wrapVolumeInspectErr(err)
	}

	created, err := c.cli.VolumeCreate(ctx, volume.CreateOptions{
		Name: volumeName,
		Labels: map[string]string{
			managedLabel: "true",
		},
	})
	if err != nil {
		return "", wrapVolumeCreateErr(err)
	}
	return created.Name, nil
}

// EnsurePostgresContainer makes sure a Postgres container for svc exists and
// is running, creating (and pulling the image, if needed) or starting it as
// necessary. See the package doc comment above for the full create-vs-
// reuse-vs-start decision tree. Returns the container ID either way.
//
// EnsurePostgresContainer does NOT create the network or volume itself —
// call EnsureNetwork/EnsureVolume first (StartPostgresEnvironment does this
// for you in the right order).
func (c *Client) EnsurePostgresContainer(ctx context.Context, svc storage.Service) (string, error) {
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
	cfg, hostCfg := buildPostgresContainerSpec(svc, networkName)

	resp, err := c.cli.ContainerCreate(ctx, cfg, hostCfg, nil, nil, name)
	if err != nil {
		return "", wrapContainerCreateErr(err)
	}
	if err := c.cli.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		return "", wrapContainerStartErr(err)
	}
	return resp.ID, nil
}

// StartPostgresEnvironment is the entrypoint the "Start" binding calls for a
// Postgres service: it ensures the profile's network, the service's volume,
// and the service's container all exist and the container ends up running,
// creating only what's missing.
func (c *Client) StartPostgresEnvironment(ctx context.Context, svc storage.Service) error {
	if svc.Engine != storage.EnginePostgres {
		return fmt.Errorf("docker: StartPostgresEnvironment only supports engine %q, got %q", storage.EnginePostgres, svc.Engine)
	}

	if _, err := c.EnsureNetwork(ctx, svc.ProfileID); err != nil {
		return err
	}
	if _, err := c.EnsureVolume(ctx, svc.VolumeName); err != nil {
		return err
	}
	if _, err := c.EnsurePostgresContainer(ctx, svc); err != nil {
		return err
	}
	return nil
}

func (c *Client) ensureImage(ctx context.Context, imageTag string) error {
	images, err := c.cli.ImageList(ctx, image.ListOptions{
		Filters: filters.NewArgs(filters.Arg("reference", imageTag)),
	})
	if err != nil {
		return wrapImageListErr(err)
	}
	if len(images) > 0 {
		return nil
	}

	rc, err := c.cli.ImagePull(ctx, imageTag, image.PullOptions{})
	if err != nil {
		return wrapImagePullErr(err)
	}
	defer rc.Close()
	if _, err := io.Copy(io.Discard, rc); err != nil {
		return wrapImagePullErr(err)
	}
	return nil
}

func buildPostgresContainerSpec(svc storage.Service, networkName string) (*container.Config, *container.HostConfig) {
	port := nat.Port(postgresContainerPort)

	var env []string
	if svc.Username != nil {
		env = append(env, "POSTGRES_USER="+*svc.Username)
	}
	if svc.PasswordEncrypted != nil {
		env = append(env, "POSTGRES_PASSWORD="+*svc.PasswordEncrypted)
	}
	if svc.DBName != nil {
		env = append(env, "POSTGRES_DB="+*svc.DBName)
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
				Target: postgresDataDir,
			},
		},
		RestartPolicy: container.RestartPolicy{Name: container.RestartPolicyUnlessStopped},
	}

	return cfg, hostCfg
}

func wrapNetworkInspectErr(err error) error {
	return fmt.Errorf("docker: inspect network: %w", err)
}

func wrapNetworkCreateErr(err error) error {
	return fmt.Errorf("docker: create network: %w", err)
}

func wrapVolumeInspectErr(err error) error {
	return fmt.Errorf("docker: inspect volume: %w", err)
}

func wrapVolumeCreateErr(err error) error {
	return fmt.Errorf("docker: create volume: %w", err)
}

func wrapContainerInspectErr(err error) error {
	return fmt.Errorf("docker: inspect container: %w", err)
}

func wrapContainerCreateErr(err error) error {
	return fmt.Errorf("docker: create container: %w", err)
}

func wrapContainerStartErr(err error) error {
	return fmt.Errorf("docker: start container: %w", err)
}

func wrapImageListErr(err error) error {
	return fmt.Errorf("docker: list images: %w", err)
}

func wrapImagePullErr(err error) error {
	return fmt.Errorf("docker: pull image: %w", err)
}
