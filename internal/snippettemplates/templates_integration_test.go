//go:build integration

// Integration test for the built-in template gallery (tasks.md 10.3):
// proves every Template's SQL genuinely runs, not just "looks right", by
// executing it against a real Postgres AND a real MySQL container started
// through internal/docker's own Start<Engine>Environment (no mocks), then
// inserting and reading back one representative row per created table.
//
// Requires Docker Desktop/dockerd running; run with:
//
//	go test -tags=integration ./...
//
// Uses test/profile/service IDs 999033 (Postgres) and 999034 (MySQL) —
// 999001-999032 are already taken across this repo's other integration
// tests (grepped for every 9990\d\d literal in the repo before picking
// these, per docs/STATE.md's running convention; 999031/999032 were
// re-grepped and found freshly claimed by a concurrently-developed parallel
// task's own integration test after this file's first draft, so this file
// moved to 999033/999034 instead of colliding with it) — and host ports
// 15545 (Postgres) and 13323 (MySQL), distinct from every other integration
// test's port in this repo (grepped for HostPort\s*=\s*[0-9]{4,5} across the
// repo before picking these too, for the same reason). Everything created is
// torn down in t.Cleanup so each test is fully self-cleaning and safely
// re-runnable.
package snippettemplates

import (
	"context"
	"fmt"
	"testing"
	"time"

	"stackyard/internal/dbengine"
	"stackyard/internal/dbengine/mysql"
	"stackyard/internal/dbengine/postgres"
	"stackyard/internal/docker"
	"stackyard/internal/storage"
)

const (
	templatesIntegrationPostgresProfileID int64 = 999033
	templatesIntegrationPostgresServiceID int64 = 999033
	templatesIntegrationPostgresHostPort        = 15545

	templatesIntegrationMySQLProfileID int64 = 999034
	templatesIntegrationMySQLServiceID int64 = 999034
	templatesIntegrationMySQLHostPort        = 13323
)

func TestIntegration_SnippetTemplates_Postgres(t *testing.T) {
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
	dbName := "stackyard_test_db"

	svc := storage.Service{
		ID:                templatesIntegrationPostgresServiceID,
		ProfileID:         templatesIntegrationPostgresProfileID,
		Engine:            storage.EnginePostgres,
		ImageTag:          "postgres:16-alpine",
		HostPort:          templatesIntegrationPostgresHostPort,
		Username:          &username,
		PasswordEncrypted: &password,
		DBName:            &dbName,
		VolumeName:        "stackyard-test-vol-snippettemplates-postgres",
	}

	networkName := docker.ProfileNetworkName(svc.ProfileID)
	containerName := docker.ServiceContainerName(svc.ID)
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()
		if err := dockerClient.RemoveContainer(cleanupCtx, containerName); err != nil {
			t.Logf("cleanup: failed to remove container %s: %v", containerName, err)
		}
		if err := dockerClient.RemoveVolume(cleanupCtx, svc.VolumeName); err != nil {
			t.Logf("cleanup: failed to remove volume %s: %v", svc.VolumeName, err)
		}
		if err := dockerClient.RemoveNetwork(cleanupCtx, networkName); err != nil {
			t.Logf("cleanup: failed to remove network %s: %v", networkName, err)
		}
	})

	if err := dockerClient.StartPostgresEnvironment(setupCtx, svc); err != nil {
		t.Fatalf("StartPostgresEnvironment() failed: %v", err)
	}
	t.Logf("StartPostgresEnvironment: network %q, volume %q, container %q created/started", networkName, svc.VolumeName, containerName)

	connString := docker.PostgresConnectionString(svc)
	engine := postgres.New(connString)
	if err := waitForConnect(t, engine, 90*time.Second); err != nil {
		t.Fatalf("Engine failed to become reachable within timeout: %v", err)
	}
	defer engine.Close()

	ctx := context.Background()

	for _, tmpl := range List() {
		sql, ok := tmpl.SQL[storage.EnginePostgres]
		if !ok {
			t.Fatalf("template %q has no Postgres variant", tmpl.ID)
		}
		results := dbengine.ExecuteMultiStatementText(ctx, engine, sql)
		for i, result := range results {
			if !result.Success {
				t.Fatalf("template %q statement %d failed against Postgres: %s\nSQL: %s", tmpl.ID, i, result.ErrorMessage, result.Statement)
			}
		}
		t.Logf("template %q: %d statement(s) ran cleanly against Postgres", tmpl.ID, len(results))
	}

	if _, err := engine.Exec(ctx, `INSERT INTO users (email, password_hash) VALUES ($1, $2)`, "ada@example.com", "hash"); err != nil {
		t.Fatalf("INSERT INTO users failed: %v", err)
	}
	if _, err := engine.Exec(ctx, `INSERT INTO refresh_tokens (user_id, token_hash, expires_at) VALUES (1, $1, now() + interval '1 day')`, "tokenhash"); err != nil {
		t.Fatalf("INSERT INTO refresh_tokens failed: %v", err)
	}
	usersResult, err := engine.Query(ctx, `SELECT u.email, t.token_hash FROM users u JOIN refresh_tokens t ON t.user_id = u.id`)
	if err != nil {
		t.Fatalf("SELECT users JOIN refresh_tokens failed: %v", err)
	}
	if len(usersResult.Rows) != 1 {
		t.Fatalf("SELECT users JOIN refresh_tokens returned %d rows, want 1", len(usersResult.Rows))
	}
	t.Logf("auth-users-sessions round trip succeeded: %v", usersResult.Rows[0])

	if _, err := engine.Exec(ctx, `INSERT INTO audit_log (actor, action, target_type, target_id) VALUES ($1, $2, $3, $4)`, "ada", "create", "user", "1"); err != nil {
		t.Fatalf("INSERT INTO audit_log failed: %v", err)
	}
	auditResult, err := engine.Query(ctx, `SELECT actor, action FROM audit_log`)
	if err != nil {
		t.Fatalf("SELECT audit_log failed: %v", err)
	}
	if len(auditResult.Rows) != 1 {
		t.Fatalf("SELECT audit_log returned %d rows, want 1", len(auditResult.Rows))
	}
	t.Logf("audit-log round trip succeeded: %v", auditResult.Rows[0])

	if _, err := engine.Exec(ctx, `INSERT INTO settings (key, value) VALUES ($1, $2)`, "theme", "dark"); err != nil {
		t.Fatalf("INSERT INTO settings failed: %v", err)
	}
	settingsResult, err := engine.Query(ctx, `SELECT key, value FROM settings`)
	if err != nil {
		t.Fatalf("SELECT settings failed: %v", err)
	}
	if len(settingsResult.Rows) != 1 {
		t.Fatalf("SELECT settings returned %d rows, want 1", len(settingsResult.Rows))
	}
	t.Logf("settings-kv round trip succeeded: %v", settingsResult.Rows[0])
}

func TestIntegration_SnippetTemplates_MySQL(t *testing.T) {
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
	dbName := "stackyard_test_db"

	svc := storage.Service{
		ID:                templatesIntegrationMySQLServiceID,
		ProfileID:         templatesIntegrationMySQLProfileID,
		Engine:            storage.EngineMySQL,
		ImageTag:          "mysql:8",
		HostPort:          templatesIntegrationMySQLHostPort,
		Username:          &username,
		PasswordEncrypted: &password,
		DBName:            &dbName,
		VolumeName:        "stackyard-test-vol-snippettemplates-mysql",
	}

	networkName := docker.ProfileNetworkName(svc.ProfileID)
	containerName := docker.ServiceContainerName(svc.ID)
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()
		if err := dockerClient.RemoveContainer(cleanupCtx, containerName); err != nil {
			t.Logf("cleanup: failed to remove container %s: %v", containerName, err)
		}
		if err := dockerClient.RemoveVolume(cleanupCtx, svc.VolumeName); err != nil {
			t.Logf("cleanup: failed to remove volume %s: %v", svc.VolumeName, err)
		}
		if err := dockerClient.RemoveNetwork(cleanupCtx, networkName); err != nil {
			t.Logf("cleanup: failed to remove network %s: %v", networkName, err)
		}
	})

	if err := dockerClient.StartMySQLEnvironment(setupCtx, svc); err != nil {
		t.Fatalf("StartMySQLEnvironment() failed: %v", err)
	}
	t.Logf("StartMySQLEnvironment: network %q, volume %q, container %q created/started", networkName, svc.VolumeName, containerName)

	// mysql.New expects go-sql-driver/mysql's own DSN format, not the
	// "mysql://" URL docker.MySQLConnectionString returns (that helper's
	// consumer is urlparse.go's ParseConnectionURL, a separate concern) —
	// built directly here, the same way internal/dbengine/mysql's own
	// integration test does.
	dsn := fmt.Sprintf("%s:%s@tcp(127.0.0.1:%d)/%s?parseTime=true", username, password, svc.HostPort, dbName)
	engine := mysql.New(dsn)
	if err := waitForConnect(t, engine, 90*time.Second); err != nil {
		t.Fatalf("Engine failed to become reachable within timeout: %v", err)
	}
	defer engine.Close()

	ctx := context.Background()

	for _, tmpl := range List() {
		sql, ok := tmpl.SQL[storage.EngineMySQL]
		if !ok {
			t.Fatalf("template %q has no MySQL variant", tmpl.ID)
		}
		results := dbengine.ExecuteMultiStatementText(ctx, engine, sql)
		for i, result := range results {
			if !result.Success {
				t.Fatalf("template %q statement %d failed against MySQL: %s\nSQL: %s", tmpl.ID, i, result.ErrorMessage, result.Statement)
			}
		}
		t.Logf("template %q: %d statement(s) ran cleanly against MySQL", tmpl.ID, len(results))
	}

	if _, err := engine.Exec(ctx, `INSERT INTO users (email, password_hash) VALUES (?, ?)`, "ada@example.com", "hash"); err != nil {
		t.Fatalf("INSERT INTO users failed: %v", err)
	}
	if _, err := engine.Exec(ctx, `INSERT INTO refresh_tokens (user_id, token_hash, expires_at) VALUES (1, ?, DATE_ADD(NOW(), INTERVAL 1 DAY))`, "tokenhash"); err != nil {
		t.Fatalf("INSERT INTO refresh_tokens failed: %v", err)
	}
	usersResult, err := engine.Query(ctx, `SELECT u.email, t.token_hash FROM users u JOIN refresh_tokens t ON t.user_id = u.id`)
	if err != nil {
		t.Fatalf("SELECT users JOIN refresh_tokens failed: %v", err)
	}
	if len(usersResult.Rows) != 1 {
		t.Fatalf("SELECT users JOIN refresh_tokens returned %d rows, want 1", len(usersResult.Rows))
	}
	t.Logf("auth-users-sessions round trip succeeded: %v", usersResult.Rows[0])

	if _, err := engine.Exec(ctx, `INSERT INTO audit_log (actor, action, target_type, target_id) VALUES (?, ?, ?, ?)`, "ada", "create", "user", "1"); err != nil {
		t.Fatalf("INSERT INTO audit_log failed: %v", err)
	}
	auditResult, err := engine.Query(ctx, `SELECT actor, action FROM audit_log`)
	if err != nil {
		t.Fatalf("SELECT audit_log failed: %v", err)
	}
	if len(auditResult.Rows) != 1 {
		t.Fatalf("SELECT audit_log returned %d rows, want 1", len(auditResult.Rows))
	}
	t.Logf("audit-log round trip succeeded: %v", auditResult.Rows[0])

	if _, err := engine.Exec(ctx, "INSERT INTO settings (`key`, value) VALUES (?, ?)", "theme", "dark"); err != nil {
		t.Fatalf("INSERT INTO settings failed: %v", err)
	}
	settingsResult, err := engine.Query(ctx, "SELECT `key`, value FROM settings")
	if err != nil {
		t.Fatalf("SELECT settings failed: %v", err)
	}
	if len(settingsResult.Rows) != 1 {
		t.Fatalf("SELECT settings returned %d rows, want 1", len(settingsResult.Rows))
	}
	t.Logf("settings-kv round trip succeeded: %v", settingsResult.Rows[0])
}

func waitForConnect(t *testing.T, engine dbengine.Engine, timeout time.Duration) error {
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
