import ts from 'typescript'
import {describe, expect, it} from 'vitest'

/**
 * Confirms a Drizzle schema.ts fixture is syntactically valid TypeScript,
 * the way Session 9's Mermaid diagram output validated itself via
 * `mermaid.parse()` at runtime. Unlike Mermaid, there is no equivalent
 * "ask the real generator to also validate its own output" hook reachable
 * here: the generator (internal/schemaexport.BuildDrizzleSchema) lives in
 * Go, not this frontend, so this test instead hardcodes the exact fixture
 * internal/schemaexport's own
 * TestBuildDrizzleSchema_ForeignKeyBetweenTwoTables Go test asserts
 * BuildDrizzleSchema produces for the same authors/books input — a
 * generator-output drift between the two would need updating in both
 * places, which is an acceptable, documented tradeoff for getting a real
 * TypeScript-syntax check on the generated output using this project's
 * existing `typescript` devDependency (no new dependency added).
 *
 * `ts.createSourceFile` only parses; it does not resolve `drizzle-orm/*`
 * imports or type-check anything, so this specifically proves the emitted
 * text is well-formed TypeScript syntax, not that it type-checks against a
 * real Drizzle installation (tasks.md 10.5's own scope note: Prisma has no
 * comparably lightweight syntax-only validator reachable without its own
 * toolchain, so schema.prisma output is exact-string tested in Go only).
 */
function parseDiagnosticCount(source: string): number {
    const sourceFile = ts.createSourceFile('schema.ts', source, ts.ScriptTarget.Latest, true, ts.ScriptKind.TS)
    const diagnostics = (sourceFile as unknown as {parseDiagnostics?: unknown[]}).parseDiagnostics
    return diagnostics?.length ?? 0
}

describe('generated Drizzle schema.ts syntax', () => {
    it('parses a simple single-table fixture with no syntax errors', () => {
        const fixture =
            'import { integer, numeric, pgTable, varchar } from "drizzle-orm/pg-core";\n' +
            '\n' +
            'export const widgets = pgTable("widgets", {\n' +
            '  id: integer("id").notNull().primaryKey(),\n' +
            '  name: varchar("name").notNull(),\n' +
            '  weight: numeric("weight"),\n' +
            '});\n'

        expect(parseDiagnosticCount(fixture)).toBe(0)
    })

    it('parses a two-table foreign-key fixture (matching BuildDrizzleSchema\'s own Go test) with no syntax errors', () => {
        const fixture =
            'import { integer, pgTable, text } from "drizzle-orm/pg-core";\n' +
            '\n' +
            'export const authors = pgTable("authors", {\n' +
            '  id: integer("id").notNull().primaryKey(),\n' +
            '  name: text("name").notNull(),\n' +
            '});\n' +
            '\n' +
            'export const books = pgTable("books", {\n' +
            '  id: integer("id").notNull().primaryKey(),\n' +
            '  title: text("title").notNull(),\n' +
            '  authorId: integer("author_id").notNull().references(() => authors.id),\n' +
            '});\n'

        expect(parseDiagnosticCount(fixture)).toBe(0)
    })

    it('parses a composite-primary-key fixture with no syntax errors', () => {
        const fixture =
            'import { integer, pgTable, primaryKey, text } from "drizzle-orm/pg-core";\n' +
            '\n' +
            'export const memberships = pgTable("memberships", {\n' +
            '  id: integer("id").notNull(),\n' +
            '  tenantId: integer("tenant_id").notNull(),\n' +
            '  role: text("role").notNull(),\n' +
            '}, (table) => ({\n' +
            '  pk: primaryKey({ columns: [table.id, table.tenantId] }),\n' +
            '}));\n'

        expect(parseDiagnosticCount(fixture)).toBe(0)
    })

    it('detects a genuinely malformed fixture as a sanity check on the assertion itself', () => {
        const malformed = 'export const widgets = pgTable("widgets", {{{ ;;; '
        expect(parseDiagnosticCount(malformed)).toBeGreaterThan(0)
    })
})
