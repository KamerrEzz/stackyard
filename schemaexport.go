package main

import (
	"context"
	"fmt"

	"stackyard/internal/schemaexport"
)

// ExportSchemaAsPrisma exports schema's tables and foreign keys (an existing
// connection's already-created schema, tasks.md 10.4) as a real
// schema.prisma file, prompting for a save location and writing the file
// itself (see saveExportFile). This is a read-only introspection export —
// it never applies the generated schema back to the database — built
// entirely on the same ListTables/ListForeignKeys the schema-diagram
// feature already uses (see internal/schemaexport.BuildPrismaSchema's own
// doc comment for the exact type-mapping/relation rules).
func (a *App) ExportSchemaAsPrisma(sessionID, schema string) (string, error) {
	session, dialect, err := a.gridSession(sessionID)
	if err != nil {
		return "", fmt.Errorf("export schema as prisma: %w", err)
	}

	ctx, cancel := context.WithTimeout(a.ctx, schemaIntrospectionTimeout)
	defer cancel()

	tables, err := session.engine.ListTables(ctx, schema)
	if err != nil {
		return "", fmt.Errorf("export schema as prisma: %w", err)
	}
	foreignKeys, err := session.engine.ListForeignKeys(ctx, schema)
	if err != nil {
		return "", fmt.Errorf("export schema as prisma: %w", err)
	}

	content := schemaexport.BuildPrismaSchema(dialect, tables, foreignKeys)
	return a.saveExportFile("schema.prisma", content)
}

// ExportSchemaAsDrizzle exports schema's tables and foreign keys as a real
// Drizzle ORM schema.ts file (tasks.md 10.5) — the Drizzle counterpart of
// ExportSchemaAsPrisma, see its doc comment for the shared read-only-export
// contract and internal/schemaexport.BuildDrizzleSchema's own doc comment
// for the exact type-mapping/reference rules.
func (a *App) ExportSchemaAsDrizzle(sessionID, schema string) (string, error) {
	session, dialect, err := a.gridSession(sessionID)
	if err != nil {
		return "", fmt.Errorf("export schema as drizzle: %w", err)
	}

	ctx, cancel := context.WithTimeout(a.ctx, schemaIntrospectionTimeout)
	defer cancel()

	tables, err := session.engine.ListTables(ctx, schema)
	if err != nil {
		return "", fmt.Errorf("export schema as drizzle: %w", err)
	}
	foreignKeys, err := session.engine.ListForeignKeys(ctx, schema)
	if err != nil {
		return "", fmt.Errorf("export schema as drizzle: %w", err)
	}

	content := schemaexport.BuildDrizzleSchema(dialect, tables, foreignKeys)
	return a.saveExportFile("schema.ts", content)
}
