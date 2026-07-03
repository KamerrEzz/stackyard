//go:build integration

// Integration test for mongodb.go: exercises EnsureMongoContainer (and the
// StartMongoEnvironment convenience wrapper) against a live local Docker
// Engine — no mocks. Requires Docker Desktop/dockerd running; run with:
//
//	go test -tags=integration ./internal/docker/...
//
// Uses profile/service ID 999003 and host port 27018: 999001 is
// compose_integration_test.go's and 999002 is lifecycle_integration_test.go's
// (both pre-existing), so 999003 is the next free ID rather than a value
// colliding with either — ServiceContainerName derives the container's name
// from this ID alone, engine-agnostic, so two tests sharing an ID would
// fight over the same container name. Port 27018 avoids Mongo's own default
// 27017 and this machine's unrelated zeew_* containers (verified via
// `docker ps -a`: none publish a 27017-range port). Everything created is
// torn down in t.Cleanup so the test is fully self-cleaning and safely
// re-runnable.
package docker

import (
	"context"
	"net"
	"strconv"
	"testing"
	"time"

	"stackyard/internal/storage"

	"github.com/docker/docker/api/types/container"
)

func TestIntegration_StartMongoEnvironment(t *testing.T) {
	c, err := NewClient()
	if err != nil {
		t.Fatalf("NewClient() failed: %v", err)
	}
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	if err := c.Ping(ctx); err != nil {
		t.Fatalf("Ping() failed to reach the local Docker Engine: %v", err)
	}

	const testProfileID int64 = 999005
	const testServiceID int64 = 999005
	const testHostPort = 27018

	username := "stackyard_test"
	password := "stackyard_test_pw"
	dbName := "stackyard_test_db"

	svc := storage.Service{
		ID:                testServiceID,
		ProfileID:         testProfileID,
		Engine:            storage.EngineMongoDB,
		ImageTag:          "mongo:7",
		HostPort:          testHostPort,
		Username:          &username,
		PasswordEncrypted: &password,
		DBName:            &dbName,
		VolumeName:        "stackyard-test-vol-2.2",
	}

	networkName := ProfileNetworkName(svc.ProfileID)
	containerName := ServiceContainerName(svc.ID)

	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()

		if inspect, err := c.cli.ContainerInspect(cleanupCtx, containerName); err == nil {
			timeout := 5
			_ = c.cli.ContainerStop(cleanupCtx, inspect.ID, container.StopOptions{Timeout: &timeout})
			removeMongoContainerRetryingRestartPolicyRace(t, cleanupCtx, c, inspect.ID, containerName)
		}

		removeMongoVolumeRetryingInUseRace(t, cleanupCtx, c, svc.VolumeName)

		if err := c.cli.NetworkRemove(cleanupCtx, networkName); err != nil {
			t.Logf("cleanup: failed to remove network %s: %v", networkName, err)
		} else {
			t.Logf("cleanup: removed network %s", networkName)
		}
	})

	if err := c.StartMongoEnvironment(ctx, svc); err != nil {
		t.Fatalf("StartMongoEnvironment() (first call, create path) failed: %v", err)
	}
	t.Logf("StartMongoEnvironment: network %q, volume %q, container %q created/started", networkName, svc.VolumeName, containerName)

	assertNetworkExists(t, ctx, c, networkName)
	assertVolumeExists(t, ctx, c, svc.VolumeName)
	assertContainerRunning(t, ctx, c, containerName)
	assertMongoReachable(t, testHostPort)

	if err := c.StartMongoEnvironment(ctx, svc); err != nil {
		t.Fatalf("StartMongoEnvironment() (second call, already-running reuse path) failed: %v", err)
	}
	assertContainerRunning(t, ctx, c, containerName)
	t.Logf("StartMongoEnvironment: second call against already-running environment succeeded (reuse path)")

	waitForMongoEntrypointInitToSettle()

	existing, err := c.cli.ContainerInspect(ctx, containerName)
	if err != nil {
		t.Fatalf("ContainerInspect before stop-and-restart check failed: %v", err)
	}
	originalID := existing.ID

	timeout := 5
	if err := c.cli.ContainerStop(ctx, originalID, container.StopOptions{Timeout: &timeout}); err != nil {
		t.Fatalf("ContainerStop (simulating external stop) failed: %v", err)
	}

	if err := c.StartMongoEnvironment(ctx, svc); err != nil {
		t.Fatalf("StartMongoEnvironment() (third call, exists-but-stopped path) failed: %v", err)
	}
	assertContainerRunning(t, ctx, c, containerName)

	afterRestart, err := c.cli.ContainerInspect(ctx, containerName)
	if err != nil {
		t.Fatalf("ContainerInspect after restart failed: %v", err)
	}
	if afterRestart.ID != originalID {
		t.Errorf("container was recreated (ID changed from %s to %s) instead of being restarted in place", originalID, afterRestart.ID)
	} else {
		t.Logf("container %s (id=%s) was started in place after external stop, not recreated", containerName, originalID)
	}
	assertMongoReachable(t, testHostPort)
}

// mongoEntrypointInitSettleDelay covers the official mongo image's own
// docker-entrypoint.sh startup dance on a cold volume: it briefly runs a
// no-auth mongod to execute MONGO_INITDB_* setup, shuts that instance down,
// then starts the real, auth-enabled mongod — all while the container
// itself keeps running throughout. The TCP port opens as soon as the
// temporary instance starts listening, before that dance completes. Sending
// an external stop while it's still in progress raced with the daemon on
// this Windows/Docker Desktop setup during a cold (freshly pulled image,
// freshly created volume) run, observed once as a spurious "No such
// container" on the following start-in-place call; a warm rerun against a
// cached image did not reproduce it. waitForMongoEntrypointInitToSettle
// gives the entrypoint script the same completion window a real user's
// "click Stop" would naturally have, rather than deliberately probing the
// narrowest possible race window.
const mongoEntrypointInitSettleDelay = 5 * time.Second

func waitForMongoEntrypointInitToSettle() {
	time.Sleep(mongoEntrypointInitSettleDelay)
}

// removeMongoContainerRetryingRestartPolicyRace retries ContainerRemove for a
// few seconds when the daemon reports removal is already in progress.
// buildMongoContainerSpec sets RestartPolicyUnlessStopped (matching
// buildPostgresContainerSpec's own container spec) — the Force-remove's kill
// signal was observed once, on this Windows/Docker Desktop setup, racing
// with that restart policy: the daemon treated the kill as an unexpected
// exit and began restarting the container in the same window the removal
// was still tearing it down, so the first ContainerRemove call returned
// "removal ... is already in progress" and the container briefly came back
// up before the daemon's own removal finished asynchronously moments later.
// Retrying here makes the test's cleanup deterministic instead of leaking a
// container for a few seconds past the test's own lifetime.
func removeMongoContainerRetryingRestartPolicyRace(t *testing.T, ctx context.Context, c *Client, id, name string) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		if err := c.cli.ContainerRemove(ctx, id, container.RemoveOptions{Force: true}); err != nil {
			lastErr = err
			time.Sleep(500 * time.Millisecond)
			continue
		}
		t.Logf("cleanup: removed container %s", name)
		return
	}
	t.Logf("cleanup: failed to remove container %s: %v", name, lastErr)
}

// removeMongoVolumeRetryingInUseRace mirrors
// removeMongoContainerRetryingRestartPolicyRace: while the restart-policy
// race above is unresolved, the volume briefly reports "in use" by the
// container that isn't supposed to exist anymore.
func removeMongoVolumeRetryingInUseRace(t *testing.T, ctx context.Context, c *Client, name string) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		if err := c.cli.VolumeRemove(ctx, name, true); err != nil {
			lastErr = err
			time.Sleep(500 * time.Millisecond)
			continue
		}
		t.Logf("cleanup: removed volume %s", name)
		return
	}
	t.Logf("cleanup: failed to remove volume %s: %v", name, lastErr)
}

func assertMongoReachable(t *testing.T, hostPort int) {
	t.Helper()
	addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(hostPort))
	deadline := time.Now().Add(30 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
		if err == nil {
			conn.Close()
			t.Logf("TCP dial to %s succeeded — MongoDB port is reachable", addr)
			return
		}
		lastErr = err
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatalf("could not reach MongoDB on %s within timeout: %v", addr, lastErr)
}
