package main

import (
	"context"
	"testing"

	"stackyard/internal/storage"
)

func TestApp_ListAppliedMigrationVersions_NoOpenSession(t *testing.T) {
	a := &App{ctx: context.Background()}

	if _, err := a.ListAppliedMigrationVersions("does-not-exist"); err == nil {
		t.Fatal("ListAppliedMigrationVersions() with no open session returned nil error, want an error")
	}
}

func TestApp_ListAppliedMigrationVersions_RejectsNonRelationalEngine(t *testing.T) {
	a := &App{ctx: context.Background()}
	a.putQuerySession("mongo-session", &querySession{
		engine:     &fakeMigrationsBoundEngine{},
		engineType: storage.EngineMongoDB,
	})

	if _, err := a.ListAppliedMigrationVersions("mongo-session"); err == nil {
		t.Fatal("ListAppliedMigrationVersions() against a Mongo session returned nil error, want an error")
	}
}

func TestApp_ListAppliedMigrationVersions_ReturnsSortedVersions(t *testing.T) {
	a := &App{ctx: context.Background()}
	engine := &fakeMigrationsBoundEngine{
		appliedVersions: [][]any{
			{int64(20260101000002), "20260101000002_second"},
			{int64(20260101000001), "20260101000001_first"},
		},
	}
	a.putQuerySession("s1", &querySession{
		engine:     engine,
		engineType: storage.EnginePostgres,
	})

	versions, err := a.ListAppliedMigrationVersions("s1")
	if err != nil {
		t.Fatalf("ListAppliedMigrationVersions() failed: %v", err)
	}
	if len(versions) != 2 || versions[0] != 20260101000001 || versions[1] != 20260101000002 {
		t.Fatalf("ListAppliedMigrationVersions() = %v, want [20260101000001, 20260101000002] ascending", versions)
	}
}

func TestApp_ListAppliedMigrationVersions_EmptyTrackingTable(t *testing.T) {
	a := &App{ctx: context.Background()}
	engine := &fakeMigrationsBoundEngine{}
	a.putQuerySession("s1", &querySession{
		engine:     engine,
		engineType: storage.EngineMySQL,
	})

	versions, err := a.ListAppliedMigrationVersions("s1")
	if err != nil {
		t.Fatalf("ListAppliedMigrationVersions() failed: %v", err)
	}
	if len(versions) != 0 {
		t.Fatalf("ListAppliedMigrationVersions() = %v, want empty", versions)
	}
}
