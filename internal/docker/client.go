// Package docker wraps the Docker Engine API client (docker/docker/client)
// with a small, Stackyard-owned surface: connecting to the local engine and
// confirming that connection, plus container/network/volume orchestration
// (compose.go) and resource-usage polling (stats.go, Phase 2).
package docker

import (
	"context"
	"fmt"

	"github.com/docker/docker/api/types/container"
	dockerclient "github.com/docker/docker/client"
)

// Client is Stackyard's thin wrapper around *dockerclient.Client. Call sites
// in the rest of the codebase depend on this type instead of importing
// docker/docker/client directly, so a future transport/library swap stays
// contained to this package.
type Client struct {
	cli *dockerclient.Client
}

// NewClient connects to the local Docker Engine using the same
// environment-based configuration Docker's own CLI uses (DOCKER_HOST,
// DOCKER_API_VERSION, DOCKER_CERT_PATH, DOCKER_TLS_VERIFY via
// dockerclient.FromEnv), with API version negotiation enabled.
//
// NewClient does not itself dial the engine — constructing the underlying
// *dockerclient.Client only builds configuration. Use Ping to confirm the
// engine is actually reachable.
func NewClient() (*Client, error) {
	cli, err := dockerclient.NewClientWithOpts(
		dockerclient.FromEnv,
		dockerclient.WithAPIVersionNegotiation(),
	)
	if err != nil {
		return nil, wrapConnectErr(err)
	}
	return &Client{cli: cli}, nil
}

// Close releases the underlying transport (named-pipe/Unix-socket connection
// pool). Safe to call on a nil *Client or a Client with no underlying client.
func (c *Client) Close() error {
	if c == nil || c.cli == nil {
		return nil
	}
	return c.cli.Close()
}

// Ping confirms connectivity to the Docker Engine daemon. A successful Ping
// proves the named-pipe (Windows) or Unix-socket (Linux/macOS) transport is
// reachable and speaking the Docker Engine API.
func (c *Client) Ping(ctx context.Context) error {
	if _, err := c.cli.Ping(ctx); err != nil {
		return wrapPingErr(err)
	}
	return nil
}

// ListContainers returns a summary of containers known to the engine. When
// all is false, only running containers are returned (matching `docker ps`'s
// default); when true, stopped containers are included too (matching
// `docker ps -a`).
func (c *Client) ListContainers(ctx context.Context, all bool) ([]container.Summary, error) {
	containers, err := c.cli.ContainerList(ctx, container.ListOptions{All: all})
	if err != nil {
		return nil, wrapListErr(err)
	}
	return containers, nil
}

func wrapConnectErr(err error) error {
	return fmt.Errorf("docker: connect to engine: %w", err)
}

func wrapPingErr(err error) error {
	return fmt.Errorf("docker: ping engine (is Docker Desktop/the Docker Engine running?): %w", err)
}

func wrapListErr(err error) error {
	return fmt.Errorf("docker: list containers: %w", err)
}
