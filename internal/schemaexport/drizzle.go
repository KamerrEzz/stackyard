package schemaexport

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"stackyard/internal/dbengine"
)

// BuildDrizzleSchema renders tables and foreignKeys as a valid Drizzle ORM
// schema.ts (tasks.md 10.5): an `import { ... } from "drizzle-orm/pg-core"`
// (or mysql-core for MySQL) naming only the builders this output actually
// uses, then one `export const <table> = pgTable(...)`/`mysqlTable(...)`
// call per table.
//
// Table/column JS identifiers (the exported const name and each object
// property key) are camelCased from the real table/column name — e.g.
// `books.author_id` becomes the property `authorId` — while the original DB
// name is always still passed as the builder's own first string argument
// (`authorId: integer("author_id")`), so this is a naming-convention
// translation, not a lossy one. Compare BuildPrismaSchema, which keeps
// model/field names verbatim instead, since Prisma has no equally cheap
// place to carry the original DB name separately from the field name.
//
// Every foreign key becomes an inline `.references(() => otherTable.column)`
// on its own column — Drizzle's reference syntax needs no separate
// back-relation field the way Prisma's relations do, since `.references`
// resolves lazily via its arrow function regardless of declaration order.
// A composite (multi-column) foreign key still renders one independent
// `.references()` per column, the same documented limitation
// BuildPrismaSchema's own relation handling has (see its doc comment) — a
// genuinely composite FK does not get a single combined reference.
func BuildDrizzleSchema(dialect dbengine.Dialect, tables []dbengine.TableInfo, foreignKeys []dbengine.ForeignKey) string {
	fkByTableColumn := make(map[tableColumnKey]dbengine.ForeignKey, len(foreignKeys))
	for _, fk := range foreignKeys {
		fkByTableColumn[tableColumnKey{table: fk.TableName, column: fk.ColumnName}] = fk
	}

	tableVarByName := make(map[string]string, len(tables))
	for _, table := range tables {
		tableVarByName[table.Name] = camelCase(table.Name)
	}

	usedBuilders := make(map[string]bool)
	usesPrimaryKeyHelper := false

	var body strings.Builder
	for i, table := range tables {
		if i > 0 {
			body.WriteString("\n")
		}
		writeDrizzleTable(&body, dialect, table, fkByTableColumn, tableVarByName, usedBuilders, &usesPrimaryKeyHelper)
	}

	return drizzleHeader(dialect, usedBuilders, usesPrimaryKeyHelper) + "\n" + body.String()
}

// writeDrizzleTable appends one `export const ... = pgTable/mysqlTable(...)`
// call for table. A single primary-key column gets an inline
// `.primaryKey()`; two or more instead get a trailing
// `(table) => ({ pk: primaryKey({ columns: [...] }) })` config callback,
// since Drizzle (like Prisma) has no inline way to mark more than one
// column as part of the same primary key.
func writeDrizzleTable(
	b *strings.Builder,
	dialect dbengine.Dialect,
	table dbengine.TableInfo,
	fkByTableColumn map[tableColumnKey]dbengine.ForeignKey,
	tableVarByName map[string]string,
	usedBuilders map[string]bool,
	usesPrimaryKeyHelper *bool,
) {
	tableFn := "pgTable"
	if dialect == dbengine.DialectMySQL {
		tableFn = "mysqlTable"
	}
	tableVar := tableVarByName[table.Name]

	pkColumns := primaryKeyColumnNames(table)
	singlePK := len(pkColumns) == 1
	compositePK := len(pkColumns) > 1

	fmt.Fprintf(b, "export const %s = %s(%q, {\n", tableVar, tableFn, table.Name)

	for _, col := range table.Columns {
		fnName, expr := drizzleColumnCall(dialect, col.DataType, col.Name)
		usedBuilders[fnName] = true

		var chain strings.Builder
		chain.WriteString(expr)
		if !col.Nullable {
			chain.WriteString(".notNull()")
		}
		if singlePK && col.IsPrimaryKey {
			chain.WriteString(".primaryKey()")
		}
		if fk, ok := fkByTableColumn[tableColumnKey{table: table.Name, column: col.Name}]; ok {
			refVar := tableVarByName[fk.ReferencedTable]
			if refVar == "" {
				refVar = camelCase(fk.ReferencedTable)
			}
			fmt.Fprintf(&chain, ".references(() => %s.%s)", refVar, camelCase(fk.ReferencedColumn))
		}

		fmt.Fprintf(b, "  %s: %s,\n", camelCase(col.Name), chain.String())
	}

	if compositePK {
		*usesPrimaryKeyHelper = true
		quoted := make([]string, len(pkColumns))
		for i, name := range pkColumns {
			quoted[i] = "table." + camelCase(name)
		}
		fmt.Fprintf(b, "}, (table) => ({\n  pk: primaryKey({ columns: [%s] }),\n}));\n", strings.Join(quoted, ", "))
		return
	}
	b.WriteString("});\n")
}

// drizzleColumnCall returns the bare builder function name used (for the
// caller's import-collection pass) and the full call expression for
// dbColumnName/dataType under dialect — handling the two builder shapes
// that need more than a single string argument: MySQL's varchar/char, which
// require a `{ length }` config (see drizzleDefaultVarcharLength's own doc
// comment for why the length is always this fixed default), and bigint
// (both dialects), which requires a `{ mode }` config. `{ mode: "number" }`
// is used rather than `{ mode: "bigint" }` — a judgment call favoring plain
// JS numbers for the common case, at the cost of precision loss for values
// beyond Number.MAX_SAFE_INTEGER, documented in this task's final report.
func drizzleColumnCall(dialect dbengine.Dialect, dataType, dbColumnName string) (fnName, expr string) {
	fnName = drizzleBuilderFor(dialect, dataType)
	quotedName := strconv.Quote(dbColumnName)

	switch {
	case fnName == "bigint":
		expr = fmt.Sprintf("bigint(%s, { mode: \"number\" })", quotedName)
	case fnName == "varchar" && dialect == dbengine.DialectMySQL:
		expr = fmt.Sprintf("varchar(%s, { length: %d })", quotedName, drizzleDefaultVarcharLength)
	case fnName == "char" && dialect == dbengine.DialectMySQL:
		expr = fmt.Sprintf("char(%s, { length: %d })", quotedName, drizzleDefaultVarcharLength)
	default:
		expr = fmt.Sprintf("%s(%s)", fnName, quotedName)
	}
	return fnName, expr
}

// drizzleHeader renders the import statement naming exactly the builders
// usedBuilders collected (plus the table-creation function and, if
// usesPrimaryKeyHelper, the composite-key helper), alphabetically sorted so
// output is deterministic regardless of column iteration order.
func drizzleHeader(dialect dbengine.Dialect, usedBuilders map[string]bool, usesPrimaryKeyHelper bool) string {
	corePackage := "drizzle-orm/pg-core"
	tableFn := "pgTable"
	if dialect == dbengine.DialectMySQL {
		corePackage = "drizzle-orm/mysql-core"
		tableFn = "mysqlTable"
	}

	names := make(map[string]bool, len(usedBuilders)+2)
	names[tableFn] = true
	if usesPrimaryKeyHelper {
		names["primaryKey"] = true
	}
	for name := range usedBuilders {
		names[name] = true
	}

	sorted := make([]string, 0, len(names))
	for name := range names {
		sorted = append(sorted, name)
	}
	sort.Strings(sorted)

	return fmt.Sprintf("import { %s } from %q;\n", strings.Join(sorted, ", "), corePackage)
}
