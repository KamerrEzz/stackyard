//go:build integration

// Integration test for tasks.md 2.6 ("Reset volume"): exercises
// App.ResetServiceVolume against a live local Docker Engine — no mocks. Its
// core purpose is proving spec.md §3.4's third acceptance criterion for
// real: resetting one service's volume must never affect a sibling service
// running in the same profile. Run with:
//
//	go test -tags=integration ./...
//
// Uses profile/service IDs 999008 (profile + target Redis service) and
// 999009 (sibling Postgres service) — the next free IDs per docs/STATE.md's
// registry (999001-999007 already taken). Host ports 15433 (Postgres) and
// 16380 (Redis) are distinct from every existing integration test's chosen
// ports. Reuses assertContainerReachesState/assertTCPReachable from
// profile_multiengine_integration_test.go (same package). Everything
// created (SQLite rows, network, volumes, containers) is torn down in
// t.Cleanup via internal/docker's cleanup.go helpers, so the test is fully
// self-cleaning and safely re-runnable.
package main

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"stackyard/internal/docker"
	"stackyard/internal/storage"
)

const (
	resetVolumeTestProfileID     int64 = 999008
	resetVolumeTestRedisSvcID    int64 = 999008
	resetVolumeTestPostgresSvcID int64 = 999009
	resetVolumeTestRedisPort           = 16380
	resetVolumeTestPostgresPort        = 15433
	resetVolumeTestMarkerKey           = "reset-volume-marker"
	resetVolumeTestMarkerValue         = "pre-reset-value"
)

func TestIntegration_ResetServiceVolume_LeavesSiblingRunning(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "stackyard-reset-volume-test.db")
	db, err := storage.OpenAt(dbPath)
	if err != nil {
		t.Fatalf("storage.OpenAt(%q) failed: %v", dbPath, err)
	}
	defer db.Close()

	dockerClient, err := docker.NewClient()
	if err != nil {
		t.Fatalf("docker.NewClient() failed: %v", err)
	}
	defer dockerClient.Close()

	pingCtx, pingCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer pingCancel()
	if err := dockerClient.Ping(pingCtx); err != nil {
		t.Fatalf("Ping() failed to reach the local Docker Engine: %v", err)
	}

	opCtx, opCancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer opCancel()
	a := &App{ctx: opCtx, db: db, docker: dockerClient}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := db.Exec(
		`INSERT INTO profiles (id, name, created_at) VALUES (?, ?, ?)`,
		resetVolumeTestProfileID, "reset-volume-integration-test", now,
	); err != nil {
		t.Fatalf("insert test profile failed: %v", err)
	}

	redisVolume := "stackyard-test-vol-2.6-redis"
	if _, err := db.Exec(
		`INSERT INTO services (id, profile_id, engine, image_tag, host_port, username, password_encrypted, db_name, volume_name)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		resetVolumeTestRedisSvcID, resetVolumeTestProfileID, storage.EngineRedis, "redis:7-alpine", resetVolumeTestRedisPort,
		nil, nil, nil, redisVolume,
	); err != nil {
		t.Fatalf("insert test redis service failed: %v", err)
	}

	pgUsername, pgPassword, pgDBName := "stackyard_test", "stackyard_test_pw", "stackyard_test_db"
	postgresVolume := "stackyard-test-vol-2.6-postgres"
	if _, err := db.Exec(
		`INSERT INTO services (id, profile_id, engine, image_tag, host_port, username, password_encrypted, db_name, volume_name)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		resetVolumeTestPostgresSvcID, resetVolumeTestProfileID, storage.EnginePostgres, "postgres:16-alpine", resetVolumeTestPostgresPort,
		pgUsername, pgPassword, pgDBName, postgresVolume,
	); err != nil {
		t.Fatalf("insert test postgres service failed: %v", err)
	}

	redisContainerName := docker.ServiceContainerName(resetVolumeTestRedisSvcID)
	postgresContainerName := docker.ServiceContainerName(resetVolumeTestPostgresSvcID)
	networkName := docker.ProfileNetworkName(resetVolumeTestProfileID)

	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()

		if err := dockerClient.RemoveContainer(cleanupCtx, redisContainerName); err != nil {
			t.Logf("cleanup: failed to remove container %s: %v", redisContainerName, err)
		} else {
			t.Logf("cleanup: removed container %s", redisContainerName)
		}
		if err := dockerClient.RemoveContainer(cleanupCtx, postgresContainerName); err != nil {
			t.Logf("cleanup: failed to remove container %s: %v", postgresContainerName, err)
		} else {
			t.Logf("cleanup: removed container %s", postgresContainerName)
		}
		if err := dockerClient.RemoveVolume(cleanupCtx, redisVolume); err != nil {
			t.Logf("cleanup: failed to remove volume %s: %v", redisVolume, err)
		} else {
			t.Logf("cleanup: removed volume %s", redisVolume)
		}
		if err := dockerClient.RemoveVolume(cleanupCtx, postgresVolume); err != nil {
			t.Logf("cleanup: failed to remove volume %s: %v", postgresVolume, err)
		} else {
			t.Logf("cleanup: removed volume %s", postgresVolume)
		}
		if err := dockerClient.RemoveNetwork(cleanupCtx, networkName); err != nil {
			t.Logf("cleanup: failed to remove network %s: %v", networkName, err)
		} else {
			t.Logf("cleanup: removed network %s", networkName)
		}
	})

	redisSvc, err := storage.GetService(db, resetVolumeTestRedisSvcID)
	if err != nil {
		t.Fatalf("GetService(redis) failed: %v", err)
	}
	if err := dockerClient.StartRedisEnvironment(opCtx, *redisSvc); err != nil {
		t.Fatalf("StartRedisEnvironment() (target service, first start) failed: %v", err)
	}
	assertContainerReachesState(t, a, redisContainerName, "running")
	assertTCPReachable(t, resetVolumeTestRedisPort)
	t.Logf("target Redis service is running on port %d before reset", resetVolumeTestRedisPort)

	writeRedisMarker(t, resetVolumeTestRedisPort, resetVolumeTestMarkerKey, resetVolumeTestMarkerValue)
	if got := readRedisMarker(t, resetVolumeTestRedisPort, resetVolumeTestMarkerKey); got != resetVolumeTestMarkerValue {
		t.Fatalf("marker readback before reset = %q, want %q", got, resetVolumeTestMarkerValue)
	}
	t.Logf("wrote and confirmed marker key %q in the target Redis volume before reset", resetVolumeTestMarkerKey)

	postgresSvc, err := storage.GetService(db, resetVolumeTestPostgresSvcID)
	if err != nil {
		t.Fatalf("GetService(postgres) failed: %v", err)
	}
	if err := dockerClient.StartPostgresEnvironment(opCtx, *postgresSvc); err != nil {
		t.Fatalf("StartPostgresEnvironment() (sibling service, first start) failed: %v", err)
	}
	assertContainerReachesState(t, a, postgresContainerName, "running")
	assertTCPReachable(t, resetVolumeTestPostgresPort)
	t.Logf("sibling Postgres service (same profile) is running on port %d before reset", resetVolumeTestPostgresPort)

	watchDone := make(chan struct{})
	var watchWG sync.WaitGroup
	var siblingSamplesMu sync.Mutex
	var siblingSamples []string
	watchWG.Add(1)
	go func() {
		defer watchWG.Done()
		ticker := time.NewTicker(150 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-watchDone:
				return
			case <-ticker.C:
				state, stateErr := a.docker.ContainerState(opCtx, postgresContainerName)
				siblingSamplesMu.Lock()
				if stateErr != nil {
					siblingSamples = append(siblingSamples, "error:"+stateErr.Error())
				} else {
					siblingSamples = append(siblingSamples, state)
				}
				siblingSamplesMu.Unlock()
			}
		}
	}()

	resetErr := a.ResetServiceVolume(resetVolumeTestRedisSvcID)
	close(watchDone)
	watchWG.Wait()
	if resetErr != nil {
		t.Fatalf("ResetServiceVolume() failed: %v", resetErr)
	}

	siblingSamplesMu.Lock()
	samplesCopy := append([]string(nil), siblingSamples...)
	siblingSamplesMu.Unlock()

	if len(samplesCopy) == 0 {
		t.Fatalf("sibling watcher recorded zero samples — the watch window was too short to prove anything")
	}
	for _, s := range samplesCopy {
		if s != "running" {
			t.Fatalf("sibling Postgres container observed in state %q during target's reset — it must stay %q throughout: samples=%v", s, "running", samplesCopy)
		}
	}
	t.Logf("sibling Postgres container stayed %q for all %d samples captured during ResetServiceVolume: %v", "running", len(samplesCopy), samplesCopy)

	assertContainerReachesState(t, a, postgresContainerName, "running")
	assertTCPReachable(t, resetVolumeTestPostgresPort)
	t.Logf("sibling Postgres service confirmed still running immediately after reset completed")

	assertContainerReachesState(t, a, redisContainerName, "running")
	assertTCPReachable(t, resetVolumeTestRedisPort)
	t.Logf("target Redis service is running again after reset (fresh container+volume)")

	if got := readRedisMarker(t, resetVolumeTestRedisPort, resetVolumeTestMarkerKey); got != "" {
		t.Fatalf("marker readback after reset = %q, want empty (nil) — the volume should have been recreated fresh", got)
	}
	t.Logf("marker key %q is gone after reset — the Redis volume was genuinely recreated fresh, not reused", resetVolumeTestMarkerKey)
}

func writeRedisMarker(t *testing.T, hostPort int, key, value string) {
	t.Helper()
	resp := redisCommand(t, hostPort, "SET", key, value)
	if !bytes.HasPrefix(resp, []byte("+OK")) {
		t.Fatalf("SET %s %s response = %q, want prefix %q", key, value, resp, "+OK")
	}
}

func readRedisMarker(t *testing.T, hostPort int, key string) string {
	t.Helper()
	resp := redisCommand(t, hostPort, "GET", key)
	if bytes.HasPrefix(resp, []byte("$-1")) {
		return ""
	}
	return parseRedisBulkString(t, resp)
}

func redisCommand(t *testing.T, hostPort int, args ...string) []byte {
	t.Helper()
	addr := fmt.Sprintf("127.0.0.1:%d", hostPort)

	deadline := time.Now().Add(15 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		resp, err := dialRedisCommand(addr, args)
		if err == nil {
			return resp
		}
		lastErr = err
		time.Sleep(300 * time.Millisecond)
	}
	t.Fatalf("RESP command %v against %s failed after retrying: %v", args, addr, lastErr)
	return nil
}

func dialRedisCommand(addr string, args []string) ([]byte, error) {
	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))

	var buf bytes.Buffer
	fmt.Fprintf(&buf, "*%d\r\n", len(args))
	for _, arg := range args {
		fmt.Fprintf(&buf, "$%d\r\n%s\r\n", len(arg), arg)
	}
	if _, err := conn.Write(buf.Bytes()); err != nil {
		return nil, err
	}

	resp := make([]byte, 512)
	n, err := conn.Read(resp)
	if err != nil {
		return nil, err
	}
	return resp[:n], nil
}

func parseRedisBulkString(t *testing.T, resp []byte) string {
	t.Helper()
	lines := bytes.SplitN(resp, []byte("\r\n"), 2)
	if len(lines) != 2 {
		t.Fatalf("malformed RESP bulk string reply: %q", resp)
	}
	return string(bytes.TrimSuffix(lines[1], []byte("\r\n")))
}
