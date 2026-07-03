package storage

import "testing"

func TestCreateAndGetQueryHistoryEntry_RoundTrip(t *testing.T) {
	db := openTestDB(t)

	conn, err := CreateConnection(db, &Connection{Name: "history-conn", Engine: EnginePostgres, Host: "localhost", Port: 5432})
	if err != nil {
		t.Fatalf("CreateConnection failed: %v", err)
	}

	errMsg := "syntax error near FROM"
	created, err := CreateQueryHistoryEntry(db, &QueryHistoryEntry{
		ConnectionID: conn.ID,
		QueryText:    "SELECT * FROM widgets",
		DurationMs:   42,
		Success:      false,
		RowsAffected: 0,
		ErrorMessage: &errMsg,
	})
	if err != nil {
		t.Fatalf("CreateQueryHistoryEntry failed: %v", err)
	}
	if created.ID == 0 {
		t.Fatal("expected CreateQueryHistoryEntry to populate a non-zero ID")
	}
	if created.ExecutedAt == "" {
		t.Fatal("expected CreateQueryHistoryEntry to populate ExecutedAt")
	}

	fetched, err := GetQueryHistoryEntry(db, created.ID)
	if err != nil {
		t.Fatalf("GetQueryHistoryEntry(%d) failed: %v", created.ID, err)
	}
	if fetched.ConnectionID != conn.ID {
		t.Errorf("ConnectionID mismatch: got %d, want %d", fetched.ConnectionID, conn.ID)
	}
	if fetched.QueryText != "SELECT * FROM widgets" {
		t.Errorf("QueryText mismatch: got %q", fetched.QueryText)
	}
	if fetched.DurationMs != 42 {
		t.Errorf("DurationMs mismatch: got %d", fetched.DurationMs)
	}
	if fetched.Success {
		t.Error("expected Success=false to round-trip as false")
	}
	if fetched.ErrorMessage == nil || *fetched.ErrorMessage != errMsg {
		t.Errorf("ErrorMessage mismatch: got %v", fetched.ErrorMessage)
	}
}

func TestCreateQueryHistoryEntry_SuccessfulExecutionHasNilErrorMessage(t *testing.T) {
	db := openTestDB(t)

	conn, err := CreateConnection(db, &Connection{Name: "success-conn", Engine: EnginePostgres, Host: "localhost", Port: 5432})
	if err != nil {
		t.Fatalf("CreateConnection failed: %v", err)
	}

	created, err := CreateQueryHistoryEntry(db, &QueryHistoryEntry{
		ConnectionID: conn.ID,
		QueryText:    "SELECT 1",
		DurationMs:   5,
		Success:      true,
		RowsAffected: 1,
	})
	if err != nil {
		t.Fatalf("CreateQueryHistoryEntry failed: %v", err)
	}
	if !created.Success {
		t.Error("expected Success=true to round-trip as true")
	}
	if created.ErrorMessage != nil {
		t.Errorf("expected ErrorMessage to be nil for a successful execution, got %v", *created.ErrorMessage)
	}
	if created.RowsAffected != 1 {
		t.Errorf("RowsAffected mismatch: got %d", created.RowsAffected)
	}
}

func TestCreateQueryHistoryEntry_RejectsUnknownConnection(t *testing.T) {
	db := openTestDB(t)

	if _, err := CreateQueryHistoryEntry(db, &QueryHistoryEntry{
		ConnectionID: 999999,
		QueryText:    "SELECT 1",
		DurationMs:   1,
		Success:      true,
	}); err == nil {
		t.Fatal("expected CreateQueryHistoryEntry with a non-existent connection_id to fail (NOT NULL FK)")
	}
}

func TestGetQueryHistoryEntry_NotFound(t *testing.T) {
	db := openTestDB(t)

	if _, err := GetQueryHistoryEntry(db, 999999); err == nil {
		t.Fatal("expected GetQueryHistoryEntry on a non-existent ID to return an error")
	}
}

func TestListQueryHistory_OrdersMostRecentFirst(t *testing.T) {
	db := openTestDB(t)

	conn, err := CreateConnection(db, &Connection{Name: "order-conn", Engine: EnginePostgres, Host: "localhost", Port: 5432})
	if err != nil {
		t.Fatalf("CreateConnection failed: %v", err)
	}

	first, err := CreateQueryHistoryEntry(db, &QueryHistoryEntry{ConnectionID: conn.ID, QueryText: "SELECT 1", DurationMs: 1, Success: true})
	if err != nil {
		t.Fatalf("CreateQueryHistoryEntry (first) failed: %v", err)
	}
	second, err := CreateQueryHistoryEntry(db, &QueryHistoryEntry{ConnectionID: conn.ID, QueryText: "SELECT 2", DurationMs: 1, Success: true})
	if err != nil {
		t.Fatalf("CreateQueryHistoryEntry (second) failed: %v", err)
	}

	entries, err := ListQueryHistory(db, QueryHistoryFilter{})
	if err != nil {
		t.Fatalf("ListQueryHistory failed: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0].ID != second.ID || entries[1].ID != first.ID {
		t.Errorf("expected most-recent-first order, got IDs %d, %d", entries[0].ID, entries[1].ID)
	}
}

func TestListQueryHistory_FiltersByConnectionID(t *testing.T) {
	db := openTestDB(t)

	connA, err := CreateConnection(db, &Connection{Name: "conn-a", Engine: EnginePostgres, Host: "localhost", Port: 5432})
	if err != nil {
		t.Fatalf("CreateConnection(conn-a) failed: %v", err)
	}
	connB, err := CreateConnection(db, &Connection{Name: "conn-b", Engine: EngineMySQL, Host: "localhost", Port: 3306})
	if err != nil {
		t.Fatalf("CreateConnection(conn-b) failed: %v", err)
	}

	if _, err := CreateQueryHistoryEntry(db, &QueryHistoryEntry{ConnectionID: connA.ID, QueryText: "SELECT 'a'", DurationMs: 1, Success: true}); err != nil {
		t.Fatalf("seed conn-a entry failed: %v", err)
	}
	if _, err := CreateQueryHistoryEntry(db, &QueryHistoryEntry{ConnectionID: connB.ID, QueryText: "SELECT 'b'", DurationMs: 1, Success: true}); err != nil {
		t.Fatalf("seed conn-b entry failed: %v", err)
	}

	entries, err := ListQueryHistory(db, QueryHistoryFilter{ConnectionID: connA.ID})
	if err != nil {
		t.Fatalf("ListQueryHistory failed: %v", err)
	}
	if len(entries) != 1 || entries[0].ConnectionID != connA.ID {
		t.Errorf("expected exactly 1 entry scoped to conn-a, got %+v", entries)
	}
}

func TestListQueryHistory_FiltersBySearchText(t *testing.T) {
	db := openTestDB(t)

	conn, err := CreateConnection(db, &Connection{Name: "search-conn", Engine: EnginePostgres, Host: "localhost", Port: 5432})
	if err != nil {
		t.Fatalf("CreateConnection failed: %v", err)
	}

	if _, err := CreateQueryHistoryEntry(db, &QueryHistoryEntry{ConnectionID: conn.ID, QueryText: "SELECT * FROM widgets", DurationMs: 1, Success: true}); err != nil {
		t.Fatalf("seed widgets entry failed: %v", err)
	}
	if _, err := CreateQueryHistoryEntry(db, &QueryHistoryEntry{ConnectionID: conn.ID, QueryText: "SELECT * FROM gadgets", DurationMs: 1, Success: true}); err != nil {
		t.Fatalf("seed gadgets entry failed: %v", err)
	}

	entries, err := ListQueryHistory(db, QueryHistoryFilter{SearchText: "widgets"})
	if err != nil {
		t.Fatalf("ListQueryHistory failed: %v", err)
	}
	if len(entries) != 1 || entries[0].QueryText != "SELECT * FROM widgets" {
		t.Errorf("expected exactly 1 entry matching %q, got %+v", "widgets", entries)
	}
}

func TestListQueryHistory_EmptyWhenNoneExist(t *testing.T) {
	db := openTestDB(t)

	entries, err := ListQueryHistory(db, QueryHistoryFilter{})
	if err != nil {
		t.Fatalf("ListQueryHistory failed: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected no entries, got %d", len(entries))
	}
}

func TestDeleteQueryHistoryEntry_RemovesRow(t *testing.T) {
	db := openTestDB(t)

	conn, err := CreateConnection(db, &Connection{Name: "delete-conn", Engine: EnginePostgres, Host: "localhost", Port: 5432})
	if err != nil {
		t.Fatalf("CreateConnection failed: %v", err)
	}
	created, err := CreateQueryHistoryEntry(db, &QueryHistoryEntry{ConnectionID: conn.ID, QueryText: "SELECT 1", DurationMs: 1, Success: true})
	if err != nil {
		t.Fatalf("CreateQueryHistoryEntry failed: %v", err)
	}

	if err := DeleteQueryHistoryEntry(db, created.ID); err != nil {
		t.Fatalf("DeleteQueryHistoryEntry failed: %v", err)
	}

	if _, err := GetQueryHistoryEntry(db, created.ID); err == nil {
		t.Fatal("expected GetQueryHistoryEntry on a deleted entry to return an error")
	}
}

func TestDeleteQueryHistoryEntry_NotFound(t *testing.T) {
	db := openTestDB(t)

	if err := DeleteQueryHistoryEntry(db, 999999); err == nil {
		t.Fatal("expected DeleteQueryHistoryEntry on a non-existent ID to return an error")
	}
}
