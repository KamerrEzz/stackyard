//go:build integration

// Integration test for tasks.md 10.4/10.5: proves internal/schemaexport's
// BuildPrismaSchema/BuildDrizzleSchema against real table/foreign-key
// metadata read from a live Postgres/MySQL container via this package's own
// gridSession-adjacent Engine.ListTables/ListForeignKeys calls — not
// against hand-written dbengine.TableInfo/ForeignKey literals the way
// internal/schemaexport's own unit tests do. This is also this task's
// substitute for a Wails-dev/Playwright manual verification pass (no
// Playwright tooling exists in this repo to drive one): the generated
// schema.prisma/schema.ts text is logged via t.Logf for a human to read
// directly in test output. Requires Docker Desktop/dockerd running; run
// with:
//
//	go test -tags=integration .
//
// Uses test/profile/service IDs 999033 (Postgres) and 999034 (MySQL) — the
// next free IDs per docs/STATE.md's running 9990\d\d registry as of this
// session (999001-999032 already taken; grepped the whole repo before
// picking these — note 999031/999032 are currently double-booked between
// createtable_integration_test.go and internal/snippettemplates/
// templates_integration_test.go, a collision between two parallel Phase 10
// work streams neither could see from the other; flagged here, not fixed,
// since both files are out of this task's scope). Host ports 15546
// (Postgres) and 13324 (MySQL), distinct from every other integration
// test's port in this repo. Everything created is torn down in t.Cleanup so
// the test is fully self-cleaning and safely re-runnable.
package main

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"stackyard/internal/dbengine"
	dbenginemysql "stackyard/internal/dbengine/mysql"
	dbenginepostgres "stackyard/internal/dbengine/postgres"
	"stackyard/internal/docker"
	"stackyard/internal/schemaexport"
	"stackyard/internal/storage"
)

func TestIntegration_SchemaExport_PostgresForeignKeyRoundTrip(t *testing.T) {
	const (
		profileID int64 = 999033
		serviceID int64 = 999033
		hostPort        = 15546
	)

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
		ID: serviceID, ProfileID: profileID, Engine: storage.EnginePostgres,
		ImageTag: "postgres:16-alpine", HostPort: hostPort,
		Username: &username, PasswordEncrypted: &password, DBName: &dbName,
		VolumeName: "stackyard-test-vol-schemaexport-pg",
	}

	networkName := docker.ProfileNetworkName(svc.ProfileID)
	containerName := docker.ServiceContainerName(svc.ID)

	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()
		_ = dockerClient.RemoveContainer(cleanupCtx, containerName)
		_ = dockerClient.RemoveVolume(cleanupCtx, svc.VolumeName)
		_ = dockerClient.RemoveNetwork(cleanupCtx, networkName)
	})

	if err := dockerClient.StartPostgresEnvironment(setupCtx, svc); err != nil {
		t.Fatalf("StartPostgresEnvironment() failed: %v", err)
	}

	engine := dbenginepostgres.New(docker.PostgresConnectionString(svc))
	if err := waitForPostgresConnect(engine, 90*time.Second); err != nil {
		t.Fatalf("Engine failed to become reachable: %v", err)
	}
	defer engine.Close()

	ctx := context.Background()

	if _, err := engine.Query(ctx, `CREATE TABLE authors (
		id SERIAL PRIMARY KEY,
		name TEXT NOT NULL
	)`); err != nil {
		t.Fatalf("CREATE TABLE authors failed: %v", err)
	}
	if _, err := engine.Query(ctx, `CREATE TABLE books (
		id SERIAL PRIMARY KEY,
		title TEXT NOT NULL,
		author_id INTEGER NOT NULL REFERENCES authors(id)
	)`); err != nil {
		t.Fatalf("CREATE TABLE books failed: %v", err)
	}

	tables, err := engine.ListTables(ctx, "public")
	if err != nil {
		t.Fatalf("ListTables() failed: %v", err)
	}
	foreignKeys, err := engine.ListForeignKeys(ctx, "public")
	if err != nil {
		t.Fatalf("ListForeignKeys() failed: %v", err)
	}
	if len(foreignKeys) != 1 {
		t.Fatalf("ListForeignKeys() returned %d entries, want 1", len(foreignKeys))
	}

	prismaSchema := schemaexport.BuildPrismaSchema(dbengine.DialectPostgres, tables, foreignKeys)
	t.Logf("generated schema.prisma:\n%s", prismaSchema)
	for _, want := range []string{
		"model authors {",
		"model books {",
		"id Int @id",
		"name String",
		"title String",
		"author_id Int",
		"author authors @relation(fields: [author_id], references: [id])",
		"author_books books[]",
	} {
		if !strings.Contains(prismaSchema, want) {
			t.Errorf("generated schema.prisma missing %q\nfull output:\n%s", want, prismaSchema)
		}
	}

	drizzleSchema := schemaexport.BuildDrizzleSchema(dbengine.DialectPostgres, tables, foreignKeys)
	t.Logf("generated schema.ts:\n%s", drizzleSchema)
	for _, want := range []string{
		`export const authors = pgTable("authors", {`,
		`export const books = pgTable("books", {`,
		`authorId: integer("author_id").notNull().references(() => authors.id)`,
	} {
		if !strings.Contains(drizzleSchema, want) {
			t.Errorf("generated schema.ts missing %q\nfull output:\n%s", want, drizzleSchema)
		}
	}
}

func TestIntegration_SchemaExport_MySQLForeignKeyRoundTrip(t *testing.T) {
	const (
		profileID int64 = 999034
		serviceID int64 = 999034
		hostPort        = 13324
	)

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
		ID: serviceID, ProfileID: profileID, Engine: storage.EngineMySQL,
		ImageTag: "mysql:8", HostPort: hostPort,
		Username: &username, PasswordEncrypted: &password, DBName: &dbName,
		VolumeName: "stackyard-test-vol-schemaexport-mysql",
	}

	networkName := docker.ProfileNetworkName(svc.ProfileID)
	containerName := docker.ServiceContainerName(svc.ID)

	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()
		_ = dockerClient.RemoveContainer(cleanupCtx, containerName)
		_ = dockerClient.RemoveVolume(cleanupCtx, svc.VolumeName)
		_ = dockerClient.RemoveNetwork(cleanupCtx, networkName)
	})

	if err := dockerClient.StartMySQLEnvironment(setupCtx, svc); err != nil {
		t.Fatalf("StartMySQLEnvironment() failed: %v", err)
	}

	dsn := fmt.Sprintf("%s:%s@tcp(127.0.0.1:%d)/%s?parseTime=true", username, password, hostPort, dbName)
	engine := dbenginemysql.New(dsn)
	if err := waitForMySQLConnect(engine, 120*time.Second); err != nil {
		t.Fatalf("Engine failed to become reachable: %v", err)
	}
	defer engine.Close()

	ctx := context.Background()

	if _, err := engine.Query(ctx, `CREATE TABLE authors (
		id INT AUTO_INCREMENT PRIMARY KEY,
		name VARCHAR(255) NOT NULL
	)`); err != nil {
		t.Fatalf("CREATE TABLE authors failed: %v", err)
	}
	if _, err := engine.Query(ctx, `CREATE TABLE books (
		id INT AUTO_INCREMENT PRIMARY KEY,
		title VARCHAR(255) NOT NULL,
		author_id INT NOT NULL,
		FOREIGN KEY (author_id) REFERENCES authors(id)
	)`); err != nil {
		t.Fatalf("CREATE TABLE books failed: %v", err)
	}

	tables, err := engine.ListTables(ctx, dbName)
	if err != nil {
		t.Fatalf("ListTables() failed: %v", err)
	}
	foreignKeys, err := engine.ListForeignKeys(ctx, dbName)
	if err != nil {
		t.Fatalf("ListForeignKeys() failed: %v", err)
	}
	if len(foreignKeys) != 1 {
		t.Fatalf("ListForeignKeys() returned %d entries, want 1", len(foreignKeys))
	}

	prismaSchema := schemaexport.BuildPrismaSchema(dbengine.DialectMySQL, tables, foreignKeys)
	t.Logf("generated schema.prisma:\n%s", prismaSchema)
	for _, want := range []string{
		"model authors {",
		"model books {",
		"id Int @id",
		"name String",
		"title String",
		"author_id Int",
		"author authors @relation(fields: [author_id], references: [id])",
		"author_books books[]",
	} {
		if !strings.Contains(prismaSchema, want) {
			t.Errorf("generated schema.prisma missing %q\nfull output:\n%s", want, prismaSchema)
		}
	}

	drizzleSchema := schemaexport.BuildDrizzleSchema(dbengine.DialectMySQL, tables, foreignKeys)
	t.Logf("generated schema.ts:\n%s", drizzleSchema)
	for _, want := range []string{
		`export const authors = mysqlTable("authors", {`,
		`export const books = mysqlTable("books", {`,
		`authorId: int("author_id").notNull().references(() => authors.id)`,
	} {
		if !strings.Contains(drizzleSchema, want) {
			t.Errorf("generated schema.ts missing %q\nfull output:\n%s", want, drizzleSchema)
		}
	}
}
