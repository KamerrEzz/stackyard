package schemaexport

import (
	"fmt"
	"strings"

	"stackyard/internal/dbengine"
)

// BuildPrismaSchema renders tables and foreignKeys as a valid Prisma schema
// (tasks.md 10.4): a datasource/generator header naming dialect, then one
// `model` block per table.
//
// Model and field names are kept verbatim from the table/column names
// ListTables reports — no PascalCase/camelCase transform, no `@map`/`@@map`
// — since Prisma has no separate "DB name vs. field name" requirement this
// generator needs to satisfy, and adding one would need extra `@map`
// bookkeeping this task's scope doesn't call for. Compare this to
// BuildDrizzleSchema, which does camelCase its JS symbol/property names,
// specifically because Drizzle's own column builder syntax already carries
// the original DB name as its own argument at no extra cost.
//
// A foreign key becomes a Prisma relation only when it is the sole
// referencing column ListForeignKeys reports for its (table, column) pair —
// see writePrismaModel's own doc comment for exactly how the relation and
// its column names are derived. Column defaults/autoincrement are never
// emitted: dbengine.ColumnInfo carries no default-value metadata at all, so
// there is nothing here to translate into `@default(...)`.
func BuildPrismaSchema(dialect dbengine.Dialect, tables []dbengine.TableInfo, foreignKeys []dbengine.ForeignKey) string {
	relationFieldNames := computeRelationFieldNames(tables, foreignKeys)

	fkByTable := make(map[string][]dbengine.ForeignKey)
	backRelationsByTable := make(map[string][]string)
	for _, fk := range foreignKeys {
		fkByTable[fk.TableName] = append(fkByTable[fk.TableName], fk)
		relationName := relationFieldNames[tableColumnKey{table: fk.TableName, column: fk.ColumnName}]
		backRelationsByTable[fk.ReferencedTable] = append(
			backRelationsByTable[fk.ReferencedTable],
			fmt.Sprintf("%s_%s %s[]", relationName, fk.TableName, fk.TableName),
		)
	}

	var b strings.Builder
	b.WriteString(prismaHeader(dialect))
	for i, table := range tables {
		if i > 0 {
			b.WriteString("\n")
		}
		writePrismaModel(&b, table, fkByTable[table.Name], backRelationsByTable[table.Name], relationFieldNames, dialect)
	}
	return b.String()
}

// prismaHeader renders the datasource/generator boilerplate every Prisma
// schema needs, pointing DATABASE_URL at an env var (the standard Prisma
// convention) rather than embedding the real connection string, since this
// generated file is meant to be committed to the user's own project.
func prismaHeader(dialect dbengine.Dialect) string {
	provider := "postgresql"
	if dialect == dbengine.DialectMySQL {
		provider = "mysql"
	}
	return fmt.Sprintf(
		"datasource db {\n  provider = %q\n  url      = env(\"DATABASE_URL\")\n}\n\ngenerator client {\n  provider = \"prisma-client-js\"\n}\n\n",
		provider,
	)
}

// writePrismaModel appends one `model` block for table. A single primary-key
// column gets an inline `@id`; two or more get a trailing `@@id([...])`
// block attribute instead, since Prisma does not allow `@id` on more than
// one field at once. Relation fields (one per entry in ownForeignKeys) and
// back-relation array fields (one per entry in backRelations, already fully
// formatted by BuildPrismaSchema) are appended after the table's own scalar
// columns, in that order, so a model reads as "real columns, then
// relationships" regardless of which side of the relationship it's on.
func writePrismaModel(
	b *strings.Builder,
	table dbengine.TableInfo,
	ownForeignKeys []dbengine.ForeignKey,
	backRelations []string,
	relationFieldNames map[tableColumnKey]string,
	dialect dbengine.Dialect,
) {
	fmt.Fprintf(b, "model %s {\n", table.Name)

	pkColumns := primaryKeyColumnNames(table)
	singlePK := len(pkColumns) == 1

	for _, col := range table.Columns {
		scalar := prismaScalarFor(dialect, col.DataType)
		optional := ""
		if col.Nullable {
			optional = "?"
		}
		idAttr := ""
		if singlePK && col.IsPrimaryKey {
			idAttr = " @id"
		}
		fmt.Fprintf(b, "  %s %s%s%s\n", col.Name, scalar, optional, idAttr)
	}

	for _, fk := range ownForeignKeys {
		relationField := relationFieldNames[tableColumnKey{table: fk.TableName, column: fk.ColumnName}]
		fmt.Fprintf(b, "  %s %s @relation(fields: [%s], references: [%s])\n", relationField, fk.ReferencedTable, fk.ColumnName, fk.ReferencedColumn)
	}

	for _, line := range backRelations {
		fmt.Fprintf(b, "  %s\n", line)
	}

	if len(pkColumns) > 1 {
		fmt.Fprintf(b, "\n  @@id([%s])\n", strings.Join(pkColumns, ", "))
	}

	b.WriteString("}\n")
}

// computeRelationFieldNames assigns every foreign key in foreignKeys the
// name its Prisma relation scalar field should use on the referencing
// model, keyed by (table, column) so writePrismaModel/BuildPrismaSchema can
// look each one up directly.
//
// The base name strips a trailing "_id" (case-insensitive) from the FK
// column ("author_id" -> "author"); a column with no such suffix falls back
// to "<column>Ref" instead of reusing the raw column name outright, since
// the column itself already exists as a scalar field with that exact name.
// If the resulting name collides with an existing column on the same table,
// or with a relation name already assigned to an earlier foreign key on
// that same table, "Ref" is appended repeatedly until it doesn't — this
// keeps two foreign keys from the same table to the same referenced table
// (e.g. `books.author_id` and `books.editor_id`, both referencing
// `authors`) from producing colliding field names, without needing to
// detect and special-case that scenario directly.
//
// A composite (multi-column) foreign key — reported by ListForeignKeys as
// multiple entries sharing the same (table, referencedTable) pair, one per
// column, per dbengine.ForeignKey's own doc comment — is not detected or
// treated specially here: every entry still gets its own independent
// relation field, exactly as internal/diagram/relational.go's Mermaid
// generator already treats every foreign key as its own relationship line
// without a dedicated composite representation. A genuinely composite key
// would therefore render as two separate Prisma relations rather than one
// multi-field one — a known, documented limitation shared with the
// generator this package's own precedent already followed.
func computeRelationFieldNames(tables []dbengine.TableInfo, foreignKeys []dbengine.ForeignKey) map[tableColumnKey]string {
	existingColumns := make(map[string]map[string]bool, len(tables))
	for _, table := range tables {
		set := make(map[string]bool, len(table.Columns))
		for _, col := range table.Columns {
			set[col.Name] = true
		}
		existingColumns[table.Name] = set
	}

	usedNames := make(map[string]map[string]bool)
	result := make(map[tableColumnKey]string, len(foreignKeys))

	for _, fk := range foreignKeys {
		base := stripIDSuffix(fk.ColumnName)
		if base == "" {
			base = fk.ColumnName + "Ref"
		}
		if usedNames[fk.TableName] == nil {
			usedNames[fk.TableName] = make(map[string]bool)
		}
		name := base
		for existingColumns[fk.TableName][name] || usedNames[fk.TableName][name] {
			name += "Ref"
		}
		usedNames[fk.TableName][name] = true
		result[tableColumnKey{table: fk.TableName, column: fk.ColumnName}] = name
	}
	return result
}

// stripIDSuffix returns columnName with a trailing "_id" removed
// (case-insensitive), or "" if columnName doesn't end that way. Only the
// underscore-delimited "_id" suffix is recognized — a bare "id" suffix
// without the underscore is deliberately not stripped (a column named
// "valid" must not become "val").
func stripIDSuffix(columnName string) string {
	if len(columnName) > 3 && strings.EqualFold(columnName[len(columnName)-3:], "_id") {
		return columnName[:len(columnName)-3]
	}
	return ""
}
