package storage

import (
	"testing"
)

func TestCreateAndGetSnippet_RoundTrip_Global(t *testing.T) {
	db := openTestDB(t)

	created, err := CreateSnippet(db, &Snippet{
		Engine:   EnginePostgres,
		Name:     "top-10-users",
		Body:     "SELECT * FROM users ORDER BY created_at DESC LIMIT 10",
		TagsJSON: `["reporting","users"]`,
	})
	if err != nil {
		t.Fatalf("CreateSnippet failed: %v", err)
	}
	if created.ID == 0 {
		t.Fatal("expected CreateSnippet to populate a non-zero ID")
	}
	if created.ConnectionID != nil {
		t.Errorf("expected a global snippet's ConnectionID to stay nil, got %v", created.ConnectionID)
	}
	if created.CreatedAt == "" || created.UpdatedAt == "" {
		t.Error("expected CreatedAt/UpdatedAt to be populated by the schema default")
	}

	fetched, err := GetSnippet(db, created.ID)
	if err != nil {
		t.Fatalf("GetSnippet(%d) failed: %v", created.ID, err)
	}
	if fetched.Name != "top-10-users" {
		t.Errorf("Name mismatch: got %q", fetched.Name)
	}
	if fetched.Engine != EnginePostgres {
		t.Errorf("Engine mismatch: got %q", fetched.Engine)
	}
	if fetched.TagsJSON != `["reporting","users"]` {
		t.Errorf("TagsJSON mismatch: got %q", fetched.TagsJSON)
	}
}

func TestCreateAndGetSnippet_RoundTrip_ConnectionScoped(t *testing.T) {
	db := openTestDB(t)

	conn, err := CreateConnection(db, &Connection{Name: "scoped-owner", Engine: EngineMySQL, Host: "localhost", Port: 3306})
	if err != nil {
		t.Fatalf("CreateConnection failed: %v", err)
	}

	created, err := CreateSnippet(db, &Snippet{
		ConnectionID: &conn.ID,
		Engine:       EngineMySQL,
		Name:         "scoped-snippet",
		Body:         "SELECT 1",
	})
	if err != nil {
		t.Fatalf("CreateSnippet failed: %v", err)
	}
	if created.ConnectionID == nil || *created.ConnectionID != conn.ID {
		t.Errorf("expected ConnectionID to be %d, got %v", conn.ID, created.ConnectionID)
	}
}

func TestCreateSnippet_DefaultsTagsJSONWhenEmpty(t *testing.T) {
	db := openTestDB(t)

	created, err := CreateSnippet(db, &Snippet{Engine: EnginePostgres, Name: "no-tags", Body: "SELECT 1"})
	if err != nil {
		t.Fatalf("CreateSnippet failed: %v", err)
	}
	if created.TagsJSON != "[]" {
		t.Errorf("expected empty TagsJSON to default to %q, got %q", "[]", created.TagsJSON)
	}
}

func TestGetSnippet_NotFound(t *testing.T) {
	db := openTestDB(t)

	if _, err := GetSnippet(db, 999999); err == nil {
		t.Fatal("expected GetSnippet on a non-existent ID to return an error")
	}
}

func TestListSnippets_ReturnsAllOrderedByName(t *testing.T) {
	db := openTestDB(t)

	for _, name := range []string{"zebra-query", "apple-query", "mango-query"} {
		if _, err := CreateSnippet(db, &Snippet{Engine: EnginePostgres, Name: name, Body: "SELECT 1"}); err != nil {
			t.Fatalf("CreateSnippet(%q) failed: %v", name, err)
		}
	}

	snippets, err := ListSnippets(db, SnippetFilter{})
	if err != nil {
		t.Fatalf("ListSnippets failed: %v", err)
	}
	if len(snippets) != 3 {
		t.Fatalf("expected 3 snippets, got %d", len(snippets))
	}

	want := []string{"apple-query", "mango-query", "zebra-query"}
	for i, s := range snippets {
		if s.Name != want[i] {
			t.Errorf("index %d: expected name %q, got %q", i, want[i], s.Name)
		}
	}
}

func TestListSnippets_EmptyWhenNoneExist(t *testing.T) {
	db := openTestDB(t)

	snippets, err := ListSnippets(db, SnippetFilter{})
	if err != nil {
		t.Fatalf("ListSnippets failed: %v", err)
	}
	if len(snippets) != 0 {
		t.Errorf("expected no snippets, got %d", len(snippets))
	}
}

func TestListSnippets_SearchMatchesName(t *testing.T) {
	db := openTestDB(t)

	if _, err := CreateSnippet(db, &Snippet{Engine: EnginePostgres, Name: "monthly-revenue-report", Body: "SELECT 1"}); err != nil {
		t.Fatalf("CreateSnippet failed: %v", err)
	}
	if _, err := CreateSnippet(db, &Snippet{Engine: EnginePostgres, Name: "user-audit", Body: "SELECT 1"}); err != nil {
		t.Fatalf("CreateSnippet failed: %v", err)
	}

	snippets, err := ListSnippets(db, SnippetFilter{SearchText: "revenue"})
	if err != nil {
		t.Fatalf("ListSnippets failed: %v", err)
	}
	if len(snippets) != 1 || snippets[0].Name != "monthly-revenue-report" {
		t.Fatalf("expected exactly the revenue snippet to match, got %+v", snippets)
	}
}

func TestListSnippets_SearchMatchesTag(t *testing.T) {
	db := openTestDB(t)

	if _, err := CreateSnippet(db, &Snippet{Engine: EnginePostgres, Name: "a", Body: "SELECT 1", TagsJSON: `["billing","finance"]`}); err != nil {
		t.Fatalf("CreateSnippet failed: %v", err)
	}
	if _, err := CreateSnippet(db, &Snippet{Engine: EnginePostgres, Name: "b", Body: "SELECT 1", TagsJSON: `["users"]`}); err != nil {
		t.Fatalf("CreateSnippet failed: %v", err)
	}

	snippets, err := ListSnippets(db, SnippetFilter{SearchText: "finance"})
	if err != nil {
		t.Fatalf("ListSnippets failed: %v", err)
	}
	if len(snippets) != 1 || snippets[0].Name != "a" {
		t.Fatalf("expected exactly the finance-tagged snippet to match, got %+v", snippets)
	}
}

func TestListSnippets_SearchIsCaseInsensitiveAndLiteral(t *testing.T) {
	db := openTestDB(t)

	if _, err := CreateSnippet(db, &Snippet{Engine: EnginePostgres, Name: "Weekly_Report", Body: "SELECT 1"}); err != nil {
		t.Fatalf("CreateSnippet failed: %v", err)
	}
	if _, err := CreateSnippet(db, &Snippet{Engine: EnginePostgres, Name: "unrelated", Body: "SELECT 1"}); err != nil {
		t.Fatalf("CreateSnippet failed: %v", err)
	}

	snippets, err := ListSnippets(db, SnippetFilter{SearchText: "weekly_report"})
	if err != nil {
		t.Fatalf("ListSnippets failed: %v", err)
	}
	if len(snippets) != 1 || snippets[0].Name != "Weekly_Report" {
		t.Fatalf("expected the case-insensitive literal-underscore match to find exactly one snippet, got %+v", snippets)
	}

	noMatch, err := ListSnippets(db, SnippetFilter{SearchText: "weeklyxreport"})
	if err != nil {
		t.Fatalf("ListSnippets failed: %v", err)
	}
	if len(noMatch) != 0 {
		t.Fatalf("expected '_' in the search text to be treated literally (not as a SQL LIKE wildcard), got %+v", noMatch)
	}
}

func TestListSnippetsForConnection_ScopedSnippetOnlyForOwnConnection(t *testing.T) {
	db := openTestDB(t)

	connA, err := CreateConnection(db, &Connection{Name: "conn-a", Engine: EnginePostgres, Host: "localhost", Port: 5432})
	if err != nil {
		t.Fatalf("CreateConnection failed: %v", err)
	}
	connB, err := CreateConnection(db, &Connection{Name: "conn-b", Engine: EnginePostgres, Host: "localhost", Port: 5433})
	if err != nil {
		t.Fatalf("CreateConnection failed: %v", err)
	}

	if _, err := CreateSnippet(db, &Snippet{ConnectionID: &connA.ID, Engine: EnginePostgres, Name: "a-only", Body: "SELECT 1"}); err != nil {
		t.Fatalf("CreateSnippet failed: %v", err)
	}

	resultsForA, err := ListSnippetsForConnection(db, connA.ID, EnginePostgres)
	if err != nil {
		t.Fatalf("ListSnippetsForConnection(A) failed: %v", err)
	}
	if len(resultsForA) != 1 || resultsForA[0].Name != "a-only" {
		t.Fatalf("expected connection A to see its own scoped snippet, got %+v", resultsForA)
	}

	resultsForB, err := ListSnippetsForConnection(db, connB.ID, EnginePostgres)
	if err != nil {
		t.Fatalf("ListSnippetsForConnection(B) failed: %v", err)
	}
	if len(resultsForB) != 0 {
		t.Fatalf("expected connection B to NOT see connection A's scoped snippet, got %+v", resultsForB)
	}
}

func TestListSnippetsForConnection_GlobalSnippetRequiresCompatibleEngine(t *testing.T) {
	db := openTestDB(t)

	pgConn, err := CreateConnection(db, &Connection{Name: "pg-conn", Engine: EnginePostgres, Host: "localhost", Port: 5432})
	if err != nil {
		t.Fatalf("CreateConnection failed: %v", err)
	}
	mysqlConn, err := CreateConnection(db, &Connection{Name: "mysql-conn", Engine: EngineMySQL, Host: "localhost", Port: 3306})
	if err != nil {
		t.Fatalf("CreateConnection failed: %v", err)
	}

	if _, err := CreateSnippet(db, &Snippet{Engine: EnginePostgres, Name: "global-pg-snippet", Body: "SELECT 1"}); err != nil {
		t.Fatalf("CreateSnippet failed: %v", err)
	}

	forPostgres, err := ListSnippetsForConnection(db, pgConn.ID, EnginePostgres)
	if err != nil {
		t.Fatalf("ListSnippetsForConnection(postgres) failed: %v", err)
	}
	if len(forPostgres) != 1 || forPostgres[0].Name != "global-pg-snippet" {
		t.Fatalf("expected the global Postgres snippet to be usable from a Postgres connection, got %+v", forPostgres)
	}

	forMySQL, err := ListSnippetsForConnection(db, mysqlConn.ID, EngineMySQL)
	if err != nil {
		t.Fatalf("ListSnippetsForConnection(mysql) failed: %v", err)
	}
	if len(forMySQL) != 0 {
		t.Fatalf("expected a global Postgres snippet to NOT appear for a MySQL connection, got %+v", forMySQL)
	}
}

func TestUpdateSnippet_ChangesPersist(t *testing.T) {
	db := openTestDB(t)

	created, err := CreateSnippet(db, &Snippet{Engine: EnginePostgres, Name: "before-update", Body: "SELECT 1", TagsJSON: `["a"]`})
	if err != nil {
		t.Fatalf("CreateSnippet failed: %v", err)
	}

	created.Name = "after-update"
	created.Body = "SELECT 2"
	created.TagsJSON = `["a","b"]`

	updated, err := UpdateSnippet(db, created)
	if err != nil {
		t.Fatalf("UpdateSnippet failed: %v", err)
	}
	if updated.Name != "after-update" || updated.Body != "SELECT 2" || updated.TagsJSON != `["a","b"]` {
		t.Errorf("expected updated fields to be returned, got %+v", updated)
	}
	if updated.UpdatedAt == created.CreatedAt {
		t.Error("expected UpdatedAt to change on update")
	}

	fetched, err := GetSnippet(db, created.ID)
	if err != nil {
		t.Fatalf("GetSnippet after update failed: %v", err)
	}
	if fetched.Name != "after-update" {
		t.Errorf("expected Name update to persist, got %q", fetched.Name)
	}
}

func TestUpdateSnippet_CanRescopeToConnection(t *testing.T) {
	db := openTestDB(t)

	conn, err := CreateConnection(db, &Connection{Name: "rescope-target", Engine: EnginePostgres, Host: "localhost", Port: 5432})
	if err != nil {
		t.Fatalf("CreateConnection failed: %v", err)
	}

	created, err := CreateSnippet(db, &Snippet{Engine: EnginePostgres, Name: "was-global", Body: "SELECT 1"})
	if err != nil {
		t.Fatalf("CreateSnippet failed: %v", err)
	}

	created.ConnectionID = &conn.ID
	updated, err := UpdateSnippet(db, created)
	if err != nil {
		t.Fatalf("UpdateSnippet failed: %v", err)
	}
	if updated.ConnectionID == nil || *updated.ConnectionID != conn.ID {
		t.Errorf("expected snippet to be rescoped to connection %d, got %v", conn.ID, updated.ConnectionID)
	}
}

func TestUpdateSnippet_NotFound(t *testing.T) {
	db := openTestDB(t)

	ghost := &Snippet{ID: 999999, Engine: EnginePostgres, Name: "ghost", Body: "SELECT 1"}
	if _, err := UpdateSnippet(db, ghost); err == nil {
		t.Fatal("expected UpdateSnippet on a non-existent ID to return an error")
	}
}

func TestDeleteSnippet_RemovesRow(t *testing.T) {
	db := openTestDB(t)

	created, err := CreateSnippet(db, &Snippet{Engine: EnginePostgres, Name: "to-delete", Body: "SELECT 1"})
	if err != nil {
		t.Fatalf("CreateSnippet failed: %v", err)
	}

	if err := DeleteSnippet(db, created.ID); err != nil {
		t.Fatalf("DeleteSnippet failed: %v", err)
	}

	if _, err := GetSnippet(db, created.ID); err == nil {
		t.Fatal("expected GetSnippet on a deleted snippet to return an error")
	}
}

func TestDeleteSnippet_NotFound(t *testing.T) {
	db := openTestDB(t)

	if err := DeleteSnippet(db, 999999); err == nil {
		t.Fatal("expected DeleteSnippet on a non-existent ID to return an error")
	}
}
