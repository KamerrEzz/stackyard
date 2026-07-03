// Package docker (this file, cleanup.go) exposes the remove half of
// container/volume/network lifecycle management, complementing lifecycle.go
// (stop/inspect) and compose.go (create/ensure). Task 2.6 ("Reset volume")
// is the first product feature expected to call RemoveVolume for real,
// stopping a single service, removing only its volume, and leaving it to be
// recreated fresh on next start; until then these three are exercised by
// integration tests that need to fully tear down what they create.
package docker

import (
	"context"
	"fmt"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/errdefs"
)

// RemoveContainer force-removes the named container, stopping it first if
// necessary. It is a deliberate no-op — not an error — when the container
// doesn't exist.
func (c *Client) RemoveContainer(ctx context.Context, name string) error {
	inspect, err := c.cli.ContainerInspect(ctx, name)
	if err != nil {
		if errdefs.IsNotFound(err) {
			return nil
		}
		return wrapContainerInspectErr(err)
	}

	if err := c.cli.ContainerRemove(ctx, inspect.ID, container.RemoveOptions{Force: true}); err != nil {
		return wrapContainerRemoveErr(err)
	}
	return nil
}

// RemoveVolume removes the named volume. It is a deliberate no-op — not an
// error — when the volume doesn't exist.
func (c *Client) RemoveVolume(ctx context.Context, name string) error {
	if err := c.cli.VolumeRemove(ctx, name, true); err != nil {
		if errdefs.IsNotFound(err) {
			return nil
		}
		return wrapVolumeRemoveErr(err)
	}
	return nil
}

// RemoveNetwork removes the named network. It is a deliberate no-op — not an
// error — when the network doesn't exist.
func (c *Client) RemoveNetwork(ctx context.Context, name string) error {
	if err := c.cli.NetworkRemove(ctx, name); err != nil {
		if errdefs.IsNotFound(err) {
			return nil
		}
		return wrapNetworkRemoveErr(err)
	}
	return nil
}

func wrapContainerRemoveErr(err error) error {
	return fmt.Errorf("docker: remove container: %w", err)
}

func wrapVolumeRemoveErr(err error) error {
	return fmt.Errorf("docker: remove volume: %w", err)
}

func wrapNetworkRemoveErr(err error) error {
	return fmt.Errorf("docker: remove network: %w", err)
}
