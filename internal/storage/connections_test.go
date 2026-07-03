package storage

import (
	"database/sql"
	"testing"
)

func TestCreateAndGetConnection_RoundTrip(t *testing.T) {
	db := openTestDB(t)

	created, err := CreateConnection(db, &Connection{
		Name:              "prod-readonly",
		Engine:            EnginePostgres,
		Host:              "db.example.com",
		Port:              5432,
		Username:          strPtr("appuser"),
		PasswordEncrypted: strPtr("s3cret"),
		Database:          strPtr("appdb"),
		ParamsJSON:        `{"sslmode":"require"}`,
	})
	if err != nil {
		t.Fatalf("CreateConnection failed: %v", err)
	}
	if created.ID == 0 {
		t.Fatal("expected CreateConnection to populate a non-zero ID")
	}

	fetched, err := GetConnection(db, created.ID)
	if err != nil {
		t.Fatalf("GetConnection(%d) failed: %v", created.ID, err)
	}

	if fetched.Name != "prod-readonly" {
		t.Errorf("Name mismatch: got %q", fetched.Name)
	}
	if fetched.Engine != EnginePostgres {
		t.Errorf("Engine mismatch: got %q", fetched.Engine)
	}
	if fetched.Host != "db.example.com" {
		t.Errorf("Host mismatch: got %q", fetched.Host)
	}
	if fetched.Port != 5432 {
		t.Errorf("Port mismatch: got %d", fetched.Port)
	}
	if fetched.Username == nil || *fetched.Username != "appuser" {
		t.Errorf("Username mismatch: got %v", fetched.Username)
	}
	if fetched.PasswordEncrypted == nil || *fetched.PasswordEncrypted != "s3cret" {
		t.Errorf("PasswordEncrypted mismatch: got %v", fetched.PasswordEncrypted)
	}
	if fetched.Database == nil || *fetched.Database != "appdb" {
		t.Errorf("Database mismatch: got %v", fetched.Database)
	}
	if fetched.ParamsJSON != `{"sslmode":"require"}` {
		t.Errorf("ParamsJSON mismatch: got %q", fetched.ParamsJSON)
	}
	if fetched.LastUsedAt != nil {
		t.Errorf("expected LastUsedAt to be nil for a freshly created connection, got %v", fetched.LastUsedAt)
	}
}

func TestCreateConnection_DefaultsParamsJSONWhenEmpty(t *testing.T) {
	db := openTestDB(t)

	created, err := CreateConnection(db, &Connection{
		Name:   "no-params",
		Engine: EngineRedis,
		Host:   "localhost",
		Port:   6379,
	})
	if err != nil {
		t.Fatalf("CreateConnection failed: %v", err)
	}
	if created.ParamsJSON != "{}" {
		t.Errorf("expected empty ParamsJSON to default to %q, got %q", "{}", created.ParamsJSON)
	}
	if created.Username != nil || created.PasswordEncrypted != nil || created.Database != nil {
		t.Errorf("expected nullable fields to stay nil for a Redis-style connection, got Username=%v PasswordEncrypted=%v Database=%v",
			created.Username, created.PasswordEncrypted, created.Database)
	}
}

func TestGetConnection_NotFound(t *testing.T) {
	db := openTestDB(t)

	if _, err := GetConnection(db, 999999); err == nil {
		t.Fatal("expected GetConnection on a non-existent ID to return an error")
	}
}

func TestListConnections_ReturnsAllOrderedByName(t *testing.T) {
	db := openTestDB(t)

	for _, name := range []string{"zebra-db", "apple-db", "mango-db"} {
		if _, err := CreateConnection(db, &Connection{Name: name, Engine: EnginePostgres, Host: "localhost", Port: 5432}); err != nil {
			t.Fatalf("CreateConnection(%q) failed: %v", name, err)
		}
	}

	connections, err := ListConnections(db)
	if err != nil {
		t.Fatalf("ListConnections failed: %v", err)
	}

	if len(connections) != 3 {
		t.Fatalf("expected 3 connections, got %d", len(connections))
	}

	want := []string{"apple-db", "mango-db", "zebra-db"}
	for i, c := range connections {
		if c.Name != want[i] {
			t.Errorf("index %d: expected name %q, got %q", i, want[i], c.Name)
		}
	}
}

func TestListConnections_EmptyWhenNoneExist(t *testing.T) {
	db := openTestDB(t)

	connections, err := ListConnections(db)
	if err != nil {
		t.Fatalf("ListConnections failed: %v", err)
	}
	if len(connections) != 0 {
		t.Errorf("expected no connections, got %d", len(connections))
	}
}

func TestTouchConnectionLastUsed_SetsTimestamp(t *testing.T) {
	db := openTestDB(t)

	created, err := CreateConnection(db, &Connection{Name: "touch-me", Engine: EnginePostgres, Host: "localhost", Port: 5432})
	if err != nil {
		t.Fatalf("CreateConnection failed: %v", err)
	}
	if created.LastUsedAt != nil {
		t.Fatal("expected a freshly created connection to have a nil LastUsedAt")
	}

	touched, err := TouchConnectionLastUsed(db, created.ID)
	if err != nil {
		t.Fatalf("TouchConnectionLastUsed failed: %v", err)
	}
	if touched.LastUsedAt == nil || *touched.LastUsedAt == "" {
		t.Fatal("expected TouchConnectionLastUsed to populate LastUsedAt")
	}

	fetched, err := GetConnection(db, created.ID)
	if err != nil {
		t.Fatalf("GetConnection after touch failed: %v", err)
	}
	if fetched.LastUsedAt == nil || *fetched.LastUsedAt != *touched.LastUsedAt {
		t.Errorf("expected the touched LastUsedAt to persist, got %v", fetched.LastUsedAt)
	}
}

func TestTouchConnectionLastUsed_NotFound(t *testing.T) {
	db := openTestDB(t)

	if _, err := TouchConnectionLastUsed(db, 999999); err == nil {
		t.Fatal("expected TouchConnectionLastUsed on a non-existent ID to return an error")
	}
}

func TestSetConnectionMigrationsFolder_SetsAndPersists(t *testing.T) {
	db := openTestDB(t)

	created, err := CreateConnection(db, &Connection{Name: "with-migrations", Engine: EnginePostgres, Host: "localhost", Port: 5432})
	if err != nil {
		t.Fatalf("CreateConnection failed: %v", err)
	}
	if created.MigrationsFolder != nil {
		t.Fatal("expected a freshly created connection to have a nil MigrationsFolder")
	}

	updated, err := SetConnectionMigrationsFolder(db, created.ID, `C:\migrations\prod`)
	if err != nil {
		t.Fatalf("SetConnectionMigrationsFolder failed: %v", err)
	}
	if updated.MigrationsFolder == nil || *updated.MigrationsFolder != `C:\migrations\prod` {
		t.Fatalf("expected MigrationsFolder to be set, got %v", updated.MigrationsFolder)
	}

	fetched, err := GetConnection(db, created.ID)
	if err != nil {
		t.Fatalf("GetConnection after SetConnectionMigrationsFolder failed: %v", err)
	}
	if fetched.MigrationsFolder == nil || *fetched.MigrationsFolder != `C:\migrations\prod` {
		t.Errorf("expected the migrations folder to persist, got %v", fetched.MigrationsFolder)
	}
}

func TestSetConnectionMigrationsFolder_NotFound(t *testing.T) {
	db := openTestDB(t)

	if _, err := SetConnectionMigrationsFolder(db, 999999, `/tmp/migrations`); err == nil {
		t.Fatal("expected SetConnectionMigrationsFolder on a non-existent ID to return an error")
	}
}

func TestDeleteConnection_RemovesRow(t *testing.T) {
	db := openTestDB(t)

	created, err := CreateConnection(db, &Connection{Name: "to-delete", Engine: EnginePostgres, Host: "localhost", Port: 5432})
	if err != nil {
		t.Fatalf("CreateConnection failed: %v", err)
	}

	if err := DeleteConnection(db, created.ID); err != nil {
		t.Fatalf("DeleteConnection failed: %v", err)
	}

	if _, err := GetConnection(db, created.ID); err == nil {
		t.Fatal("expected GetConnection on a deleted connection to return an error")
	}
}

func TestDeleteConnection_NotFound(t *testing.T) {
	db := openTestDB(t)

	if err := DeleteConnection(db, 999999); err == nil {
		t.Fatal("expected DeleteConnection on a non-existent ID to return an error")
	}
}

func TestDeleteConnection_DemotesScopedSnippetsToGlobal(t *testing.T) {
	db := openTestDB(t)

	conn, err := CreateConnection(db, &Connection{Name: "snippet-owner", Engine: EnginePostgres, Host: "localhost", Port: 5432})
	if err != nil {
		t.Fatalf("CreateConnection failed: %v", err)
	}

	res, err := db.Exec(
		`INSERT INTO snippets (connection_id, engine, name, body) VALUES (?, ?, ?, ?)`,
		conn.ID, EnginePostgres, "scoped-snippet", "SELECT 1",
	)
	if err != nil {
		t.Fatalf("seed snippet insert failed: %v", err)
	}
	snippetID, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("read snippet id: %v", err)
	}

	if err := DeleteConnection(db, conn.ID); err != nil {
		t.Fatalf("DeleteConnection failed: %v", err)
	}

	var connectionID sql.NullInt64
	if err := db.QueryRow(`SELECT connection_id FROM snippets WHERE id = ?`, snippetID).Scan(&connectionID); err != nil {
		t.Fatalf("expected the snippet to still exist after its connection was deleted, got error: %v", err)
	}
	if connectionID.Valid {
		t.Errorf("expected the snippet's connection_id to be demoted to NULL, got %v", connectionID.Int64)
	}
}

func TestDeleteConnection_CascadesQueryHistory(t *testing.T) {
	db := openTestDB(t)

	conn, err := CreateConnection(db, &Connection{Name: "history-owner", Engine: EnginePostgres, Host: "localhost", Port: 5432})
	if err != nil {
		t.Fatalf("CreateConnection failed: %v", err)
	}

	if _, err := db.Exec(
		`INSERT INTO query_history (connection_id, query_text, duration_ms, success) VALUES (?, ?, ?, ?)`,
		conn.ID, "SELECT 1", 5, 1,
	); err != nil {
		t.Fatalf("seed query_history insert failed: %v", err)
	}

	if err := DeleteConnection(db, conn.ID); err != nil {
		t.Fatalf("DeleteConnection failed: %v", err)
	}

	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM query_history WHERE connection_id = ?`, conn.ID).Scan(&count); err != nil {
		t.Fatalf("count query_history rows: %v", err)
	}
	if count != 0 {
		t.Errorf("expected query_history rows to cascade-delete with their connection, got %d remaining", count)
	}
}
