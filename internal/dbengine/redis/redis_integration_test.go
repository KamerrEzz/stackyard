//go:build integration

// Integration test for redis.go: exercises Engine against a real Redis
// container started through internal/docker's own StartRedisEnvironment (no
// bespoke container-launch code, no mocks) — the same pattern
// mongo_integration_test.go already established for Mongo. Requires Docker
// Desktop/dockerd running; run with:
//
//	go test -tags=integration ./internal/dbengine/...
//
// Uses test/profile/service ID 999022 (999001-999021 are already taken
// across internal/docker's and the repo-root's integration tests — grepped
// for every 9990\d\d literal in the repo before picking this one) and host
// port 6380, distinct from internal/docker's own Redis integration test
// (redis_integration_test.go's own 6379-adjacent port) and every other
// integration test's port in this repo. Everything created is torn down in
// t.Cleanup via internal/docker/cleanup.go's RemoveContainer/RemoveVolume/
// RemoveNetwork so the test is fully self-cleaning and safely re-runnable.
package redis

import (
	"context"
	"errors"
	"testing"
	"time"

	"stackyard/internal/docker"
	"stackyard/internal/storage"
)

const (
	redisIntegrationTestProfileID int64 = 999022
	redisIntegrationTestServiceID int64 = 999022
	redisIntegrationTestHostPort        = 6380
)

func TestIntegration_RedisEngine(t *testing.T) {
	dockerClient, err := docker.NewClient()
	if err != nil {
		t.Fatalf("docker.NewClient() failed: %v", err)
	}
	defer dockerClient.Close()

	setupCtx, setupCancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer setupCancel()

	if err := dockerClient.Ping(setupCtx); err != nil {
		t.Fatalf("Ping() failed to reach the local Docker Engine: %v", err)
	}

	password := "stackyard_test_pw"

	svc := storage.Service{
		ID:                redisIntegrationTestServiceID,
		ProfileID:         redisIntegrationTestProfileID,
		Engine:            storage.EngineRedis,
		ImageTag:          "redis:7-alpine",
		HostPort:          redisIntegrationTestHostPort,
		PasswordEncrypted: &password,
		VolumeName:        "stackyard-test-vol-dbengine-redis",
	}

	networkName := docker.ProfileNetworkName(svc.ProfileID)
	containerName := docker.ServiceContainerName(svc.ID)

	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()

		if err := dockerClient.RemoveContainer(cleanupCtx, containerName); err != nil {
			t.Logf("cleanup: failed to remove container %s: %v", containerName, err)
		} else {
			t.Logf("cleanup: removed container %s", containerName)
		}
		if err := dockerClient.RemoveVolume(cleanupCtx, svc.VolumeName); err != nil {
			t.Logf("cleanup: failed to remove volume %s: %v", svc.VolumeName, err)
		} else {
			t.Logf("cleanup: removed volume %s", svc.VolumeName)
		}
		if err := dockerClient.RemoveNetwork(cleanupCtx, networkName); err != nil {
			t.Logf("cleanup: failed to remove network %s: %v", networkName, err)
		} else {
			t.Logf("cleanup: removed network %s", networkName)
		}
	})

	if err := dockerClient.StartRedisEnvironment(setupCtx, svc); err != nil {
		t.Fatalf("StartRedisEnvironment() failed: %v", err)
	}
	t.Logf("StartRedisEnvironment: network %q, volume %q, container %q created/started", networkName, svc.VolumeName, containerName)

	connString := docker.RedisConnectionString(svc)
	engine, err := NewFromURL(connString)
	if err != nil {
		t.Fatalf("NewFromURL(%q) failed: %v", connString, err)
	}

	if err := waitForConnect(t, engine, 90*time.Second); err != nil {
		t.Fatalf("Engine failed to become reachable within timeout: %v", err)
	}
	defer engine.Close()

	ctx := context.Background()

	if err := engine.Ping(ctx); err != nil {
		t.Fatalf("Ping() failed: %v", err)
	}
	t.Log("Ping() succeeded against the live container")

	matchingKeys := []string{"session:1", "session:2", "session:3"}
	otherKeys := []string{"cache:1", "cache:2"}
	for _, key := range matchingKeys {
		if err := engine.SetString(ctx, key, "value"); err != nil {
			t.Fatalf("SetString(%q) (seed) failed: %v", key, err)
		}
	}
	for _, key := range otherKeys {
		if err := engine.SetString(ctx, key, "value"); err != nil {
			t.Fatalf("SetString(%q) (seed) failed: %v", key, err)
		}
	}
	t.Cleanup(func() {
		_ = engine.DeleteKeys(context.Background(), append(append([]string{}, matchingKeys...), otherKeys...)...)
	})

	found := map[string]bool{}
	var scanCursor uint64
	for {
		keys, nextCursor, err := engine.ScanKeys(ctx, "session:*", scanCursor, 10)
		if err != nil {
			t.Fatalf("ScanKeys() failed: %v", err)
		}
		for _, k := range keys {
			found[k] = true
		}
		scanCursor = nextCursor
		if scanCursor == 0 {
			break
		}
	}
	for _, key := range matchingKeys {
		if !found[key] {
			t.Errorf("ScanKeys(\"session:*\") did not return %q", key)
		}
	}
	for _, key := range otherKeys {
		if found[key] {
			t.Errorf("ScanKeys(\"session:*\") unexpectedly returned non-matching key %q", key)
		}
	}
	t.Logf("ScanKeys() succeeded: %v", found)

	if err := engine.SetString(ctx, "greeting", "hello"); err != nil {
		t.Fatalf("SetString() failed: %v", err)
	}
	gotString, err := engine.GetString(ctx, "greeting")
	if err != nil {
		t.Fatalf("GetString() failed: %v", err)
	}
	if gotString != "hello" {
		t.Errorf("GetString() = %q, want %q", gotString, "hello")
	}
	keyType, err := engine.KeyType(ctx, "greeting")
	if err != nil {
		t.Fatalf("KeyType() failed: %v", err)
	}
	if keyType != "string" {
		t.Errorf("KeyType(\"greeting\") = %q, want \"string\"", keyType)
	}
	t.Log("string set/get round-trip succeeded")

	hashFields := map[string]string{"name": "bolt", "material": "steel"}
	if err := engine.SetHash(ctx, "widget:1", hashFields); err != nil {
		t.Fatalf("SetHash() failed: %v", err)
	}
	gotHash, err := engine.GetHash(ctx, "widget:1")
	if err != nil {
		t.Fatalf("GetHash() failed: %v", err)
	}
	if gotHash["name"] != "bolt" || gotHash["material"] != "steel" {
		t.Errorf("GetHash() = %v, want %v", gotHash, hashFields)
	}
	t.Log("hash set/get round-trip succeeded")

	if err := engine.PushList(ctx, "queue:jobs", "job1", "job2", "job3", "job4", "job5"); err != nil {
		t.Fatalf("PushList() failed: %v", err)
	}
	firstPage, err := engine.GetList(ctx, "queue:jobs", 0, 2)
	if err != nil {
		t.Fatalf("GetList() (first page) failed: %v", err)
	}
	if len(firstPage) != 3 || firstPage[0] != "job1" || firstPage[2] != "job3" {
		t.Errorf("GetList(0,2) = %v, want [job1 job2 job3]", firstPage)
	}
	secondPage, err := engine.GetList(ctx, "queue:jobs", 3, 4)
	if err != nil {
		t.Fatalf("GetList() (second page) failed: %v", err)
	}
	if len(secondPage) != 2 || secondPage[0] != "job4" || secondPage[1] != "job5" {
		t.Errorf("GetList(3,4) = %v, want [job4 job5]", secondPage)
	}
	if err := engine.SetListElement(ctx, "queue:jobs", 0, "job1-updated"); err != nil {
		t.Fatalf("SetListElement() failed: %v", err)
	}
	updatedFirst, err := engine.GetList(ctx, "queue:jobs", 0, 0)
	if err != nil {
		t.Fatalf("GetList() after SetListElement failed: %v", err)
	}
	if len(updatedFirst) != 1 || updatedFirst[0] != "job1-updated" {
		t.Errorf("GetList(0,0) after SetListElement = %v, want [job1-updated]", updatedFirst)
	}
	t.Log("list push/get/paginate/set-element round-trip succeeded")

	setMembers := []string{"alpha", "beta", "gamma"}
	if err := engine.AddToSet(ctx, "tags:widget1", setMembers...); err != nil {
		t.Fatalf("AddToSet() failed: %v", err)
	}
	gotSetMembers := map[string]bool{}
	var setCursor uint64
	for {
		members, nextCursor, err := engine.GetSet(ctx, "tags:widget1", setCursor, 10)
		if err != nil {
			t.Fatalf("GetSet() failed: %v", err)
		}
		for _, m := range members {
			gotSetMembers[m] = true
		}
		setCursor = nextCursor
		if setCursor == 0 {
			break
		}
	}
	for _, m := range setMembers {
		if !gotSetMembers[m] {
			t.Errorf("GetSet() did not return member %q", m)
		}
	}
	t.Log("set add/get round-trip succeeded")

	sortedMembers := []SortedSetMember{
		{Member: "bronze", Score: 10},
		{Member: "silver", Score: 20},
		{Member: "gold", Score: 30},
	}
	if err := engine.AddToSortedSet(ctx, "leaderboard", sortedMembers...); err != nil {
		t.Fatalf("AddToSortedSet() failed: %v", err)
	}
	gotSortedSet, err := engine.GetSortedSet(ctx, "leaderboard", 0, -1)
	if err != nil {
		t.Fatalf("GetSortedSet() failed: %v", err)
	}
	if len(gotSortedSet) != 3 {
		t.Fatalf("GetSortedSet() returned %d members, want 3", len(gotSortedSet))
	}
	if gotSortedSet[0].Member != "bronze" || gotSortedSet[1].Member != "silver" || gotSortedSet[2].Member != "gold" {
		t.Errorf("GetSortedSet() order = %v, want ascending by score [bronze silver gold]", gotSortedSet)
	}
	if gotSortedSet[0].Score != 10 || gotSortedSet[2].Score != 30 {
		t.Errorf("GetSortedSet() scores = %v, want [10 .. 30]", gotSortedSet)
	}
	t.Log("sorted set add/get round-trip with ordering succeeded")

	if err := engine.SetTTL(ctx, "greeting", 5*time.Minute); err != nil {
		t.Fatalf("SetTTL() failed: %v", err)
	}
	gotTTL, err := engine.TTL(ctx, "greeting")
	if err != nil {
		t.Fatalf("TTL() after SetTTL failed: %v", err)
	}
	if gotTTL <= 0 || gotTTL > 5*time.Minute {
		t.Errorf("TTL() after SetTTL = %v, want a positive duration <= 5m", gotTTL)
	}
	if err := engine.PersistKey(ctx, "greeting"); err != nil {
		t.Fatalf("PersistKey() failed: %v", err)
	}
	persistedTTL, err := engine.TTL(ctx, "greeting")
	if err != nil {
		t.Fatalf("TTL() after PersistKey failed: %v", err)
	}
	if persistedTTL != -1 {
		t.Errorf("TTL() after PersistKey = %v, want -1 (no TTL)", persistedTTL)
	}
	_, err = engine.TTL(ctx, "does-not-exist-key")
	if !errors.Is(err, ErrKeyNotFound) {
		t.Errorf("TTL() on a nonexistent key error = %v, want ErrKeyNotFound", err)
	}
	t.Log("TTL set/check/persist round-trip succeeded")

	if err := engine.SetString(ctx, "rename:source", "value"); err != nil {
		t.Fatalf("SetString() (rename source) failed: %v", err)
	}
	if err := engine.SetString(ctx, "rename:target-exists", "value"); err != nil {
		t.Fatalf("SetString() (rename target) failed: %v", err)
	}
	if err := engine.RenameKey(ctx, "rename:source", "rename:target-exists"); !errors.Is(err, ErrKeyExists) {
		t.Errorf("RenameKey() onto an existing key error = %v, want ErrKeyExists", err)
	}
	if err := engine.RenameKey(ctx, "rename:source", "rename:target-new"); err != nil {
		t.Fatalf("RenameKey() onto a fresh key failed: %v", err)
	}
	if _, err := engine.GetString(ctx, "rename:target-new"); err != nil {
		t.Errorf("GetString() after rename failed: %v", err)
	}
	t.Log("rename (guarded overwrite + successful rename) succeeded")

	if err := engine.DeleteKeys(ctx, "widget:1", "queue:jobs", "tags:widget1"); err != nil {
		t.Fatalf("DeleteKeys() (multi-key) failed: %v", err)
	}
	if _, err := engine.GetHash(ctx, "widget:1"); err != nil {
		t.Errorf("GetHash() after delete failed unexpectedly: %v", err)
	}
	remainingType, err := engine.KeyType(ctx, "widget:1")
	if err != nil {
		t.Fatalf("KeyType() after delete failed: %v", err)
	}
	if remainingType != "none" {
		t.Errorf("KeyType(\"widget:1\") after DeleteKeys = %q, want \"none\"", remainingType)
	}
	t.Log("DeleteKeys() (multi-key) succeeded")

	if err := engine.Close(); err != nil {
		t.Errorf("Close() failed: %v", err)
	}
	if err := engine.Ping(context.Background()); err == nil {
		t.Error("Ping() after Close() should fail")
	}
	t.Log("Close() succeeded; Ping() after Close() correctly fails")
}

func waitForConnect(t *testing.T, engine *Engine, timeout time.Duration) error {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		connectCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		err := engine.Connect(connectCtx)
		cancel()
		if err == nil {
			return nil
		}
		lastErr = err
		time.Sleep(1 * time.Second)
	}
	return lastErr
}
