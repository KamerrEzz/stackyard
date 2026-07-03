//go:build integration

// Integration test for mongo.go: exercises Engine against a real MongoDB
// container started through internal/docker's own StartMongoEnvironment (no
// bespoke container-launch code, no mocks) — the same pattern
// postgres_integration_test.go/mysql_integration_test.go already established
// for the other two engines. Requires Docker Desktop/dockerd running; run
// with:
//
//	go test -tags=integration ./internal/dbengine/...
//
// Uses test/profile/service ID 999021 (999001-999020 are already taken
// across internal/docker's and the repo-root's integration tests — grepped
// for every 9990\d\d literal in the repo before picking this one, per
// docs/STATE.md's running convention) and host port 27019, distinct from
// internal/docker's own Mongo integration test (27018) and every other
// integration test's port in this repo. Everything created is torn down in
// t.Cleanup via internal/docker/cleanup.go's RemoveContainer/RemoveVolume/
// RemoveNetwork so the test is fully self-cleaning and safely re-runnable.
package mongo

import (
	"context"
	"errors"
	"testing"
	"time"

	"stackyard/internal/docker"
	"stackyard/internal/storage"
)

const (
	mongoIntegrationTestProfileID int64 = 999021
	mongoIntegrationTestServiceID int64 = 999021
	mongoIntegrationTestHostPort        = 27019
)

func TestIntegration_MongoEngine(t *testing.T) {
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

	username := "stackyard_test"
	password := "stackyard_test_pw"

	// DBName is deliberately left nil: MONGO_INITDB_ROOT_USERNAME/PASSWORD
	// (below) creates the root user in the "admin" database regardless, and
	// MongoConnectionString's DB path segment doubles as the driver's SCRAM
	// authSource — setting a non-admin DBName here would generate a
	// connection string that authenticates against the wrong database and
	// fail (empirically confirmed while writing this test: the root user
	// exists only in "admin", so authenticating against a fresh
	// "stackyard_test_db" path fails with "Authentication failed"). Leaving
	// it nil makes MongoConnectionString fall back to "admin" (see
	// mongodb.go's own doc comment), matching where the root user actually
	// lives; the document operations below still target an arbitrary named
	// database ("stackyard_test_db") directly via FindDocuments/
	// InsertDocument's own database parameter — MongoDB creates databases
	// lazily on first write, independent of the connection URI's default
	// database/authSource.
	svc := storage.Service{
		ID:                mongoIntegrationTestServiceID,
		ProfileID:         mongoIntegrationTestProfileID,
		Engine:            storage.EngineMongoDB,
		ImageTag:          "mongo:7",
		HostPort:          mongoIntegrationTestHostPort,
		Username:          &username,
		PasswordEncrypted: &password,
		VolumeName:        "stackyard-test-vol-dbengine-mongo",
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

	if err := dockerClient.StartMongoEnvironment(setupCtx, svc); err != nil {
		t.Fatalf("StartMongoEnvironment() failed: %v", err)
	}
	t.Logf("StartMongoEnvironment: network %q, volume %q, container %q created/started", networkName, svc.VolumeName, containerName)

	connString := docker.MongoConnectionString(svc)
	engine := New(connString)

	if err := waitForConnect(t, engine, 90*time.Second); err != nil {
		t.Fatalf("Engine failed to become reachable within timeout: %v", err)
	}
	defer engine.Close()

	ctx := context.Background()

	if err := engine.Ping(ctx); err != nil {
		t.Fatalf("Ping() failed: %v", err)
	}
	t.Log("Ping() succeeded against the live container")

	const database = "stackyard_test_db"
	const collection = "widgets"

	databases, err := engine.ListDatabases(ctx)
	if err != nil {
		t.Fatalf("ListDatabases() failed: %v", err)
	}
	if !containsString(databases, "admin") {
		t.Errorf("ListDatabases() = %v, want it to include the built-in \"admin\" database", databases)
	}
	t.Logf("ListDatabases() succeeded: %v", databases)

	inserted, err := engine.InsertDocument(ctx, database, collection, map[string]any{
		"name":   "bolt",
		"weight": 5,
		"specs": map[string]any{
			"material": "steel",
			"sizes":    []any{"M8", "M10", "M12"},
		},
		"tags": []any{"hardware", "fastener"},
	})
	if err != nil {
		t.Fatalf("InsertDocument() failed: %v", err)
	}
	t.Logf("InsertDocument() succeeded: %+v", inserted)

	insertedID, ok := inserted["_id"].(string)
	if !ok || insertedID == "" {
		t.Fatalf("InsertDocument() result[\"_id\"] = %#v, want a non-empty hex string", inserted["_id"])
	}
	if _, err := hexObjectIDLength(insertedID); err != nil {
		t.Errorf("InsertDocument() result _id %q does not look like a hex ObjectID: %v", insertedID, err)
	}

	names, err := engine.ListCollections(ctx, database)
	if err != nil {
		t.Fatalf("ListCollections() failed: %v", err)
	}
	if !containsString(names, collection) {
		t.Errorf("ListCollections() = %v, want it to include %q", names, collection)
	}
	t.Logf("ListCollections() succeeded: %v", names)

	found, err := engine.FindDocuments(ctx, database, collection, nil, 10, 0)
	if err != nil {
		t.Fatalf("FindDocuments() failed: %v", err)
	}
	if len(found) != 1 {
		t.Fatalf("FindDocuments() returned %d documents, want 1", len(found))
	}
	assertBoltDocumentRoundTrips(t, found[0], insertedID)
	t.Logf("FindDocuments() round-trip succeeded: %+v", found[0])

	count, err := engine.CountDocuments(ctx, database, collection, nil)
	if err != nil {
		t.Fatalf("CountDocuments() failed: %v", err)
	}
	if count != 1 {
		t.Errorf("CountDocuments() = %d, want 1", count)
	}
	t.Logf("CountDocuments() succeeded: %d", count)

	if err := engine.UpdateDocument(ctx, database, collection, insertedID, map[string]any{
		"name":   "bolt",
		"weight": 6,
		"specs": map[string]any{
			"material": "stainless steel",
			"sizes":    []any{"M8", "M10"},
		},
		"tags": []any{"hardware"},
	}); err != nil {
		t.Fatalf("UpdateDocument() failed: %v", err)
	}

	updated, err := engine.FindDocuments(ctx, database, collection, map[string]any{"name": "bolt"}, 10, 0)
	if err != nil {
		t.Fatalf("FindDocuments() after update failed: %v", err)
	}
	if len(updated) != 1 {
		t.Fatalf("FindDocuments() after update returned %d documents, want 1", len(updated))
	}
	if updated[0]["weight"] != int32(6) {
		t.Errorf("FindDocuments() after update: weight = %#v, want 6", updated[0]["weight"])
	}
	specs, ok := updated[0]["specs"].(map[string]any)
	if !ok || specs["material"] != "stainless steel" {
		t.Errorf("FindDocuments() after update: specs = %#v, want material \"stainless steel\"", updated[0]["specs"])
	}
	t.Logf("UpdateDocument() round-trip succeeded: %+v", updated[0])

	secondID, err := engine.InsertDocument(ctx, database, collection, map[string]any{"name": "nut", "weight": 2})
	if err != nil {
		t.Fatalf("InsertDocument() (second document) failed: %v", err)
	}
	secondHexID := secondID["_id"].(string)

	sampled, err := engine.SampleDocuments(ctx, database, collection, 2)
	if err != nil {
		t.Fatalf("SampleDocuments() failed: %v", err)
	}
	if len(sampled) != 2 {
		t.Fatalf("SampleDocuments(n=2) returned %d documents, want 2", len(sampled))
	}
	t.Logf("SampleDocuments() succeeded: %+v", sampled)

	countBeforeDelete, err := engine.CountDocuments(ctx, database, collection, nil)
	if err != nil {
		t.Fatalf("CountDocuments() before delete failed: %v", err)
	}
	if countBeforeDelete != 2 {
		t.Fatalf("CountDocuments() before delete = %d, want 2", countBeforeDelete)
	}

	if err := engine.DeleteDocuments(ctx, database, collection, []string{insertedID, secondHexID}); err != nil {
		t.Fatalf("DeleteDocuments() (multi-document) failed: %v", err)
	}

	countAfterDelete, err := engine.CountDocuments(ctx, database, collection, nil)
	if err != nil {
		t.Fatalf("CountDocuments() after delete failed: %v", err)
	}
	if countAfterDelete != 0 {
		t.Errorf("CountDocuments() after deleting both documents = %d, want 0", countAfterDelete)
	}
	t.Log("DeleteDocuments() (multi-document) succeeded")

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

func containsString(values []string, target string) bool {
	for _, v := range values {
		if v == target {
			return true
		}
	}
	return false
}

// hexObjectIDLength is a light sanity check that id looks like a 24-char hex
// ObjectID string, without importing primitive here just for this one
// assertion.
func hexObjectIDLength(id string) (int, error) {
	if len(id) != 24 {
		return len(id), errors.New("expected a 24-character hex string")
	}
	return len(id), nil
}

// assertBoltDocumentRoundTrips confirms the nested structure and typed
// scalars inserted for the "bolt" document survive the
// InsertDocument -> FindDocuments round trip, including that the returned
// _id is the same sanitized hex string InsertDocument itself returned.
func assertBoltDocumentRoundTrips(t *testing.T, doc map[string]any, wantID string) {
	t.Helper()

	if doc["_id"] != wantID {
		t.Errorf("FindDocuments() _id = %v, want %q (matching InsertDocument's own returned _id)", doc["_id"], wantID)
	}
	if doc["name"] != "bolt" {
		t.Errorf("FindDocuments() name = %v, want \"bolt\"", doc["name"])
	}

	specs, ok := doc["specs"].(map[string]any)
	if !ok {
		t.Fatalf("FindDocuments() specs = %#v (%T), want map[string]any (nested object preserved)", doc["specs"], doc["specs"])
	}
	if specs["material"] != "steel" {
		t.Errorf("FindDocuments() specs.material = %v, want \"steel\"", specs["material"])
	}

	sizes, ok := specs["sizes"].([]any)
	if !ok || len(sizes) != 3 {
		t.Fatalf("FindDocuments() specs.sizes = %#v, want a 3-element []any (nested array preserved)", specs["sizes"])
	}

	tags, ok := doc["tags"].([]any)
	if !ok || len(tags) != 2 {
		t.Fatalf("FindDocuments() tags = %#v, want a 2-element []any", doc["tags"])
	}
}
