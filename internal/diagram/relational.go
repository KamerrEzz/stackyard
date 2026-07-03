// Package diagram translates live schema metadata already discovered via
// dbengine.Engine (tables, columns, foreign keys, and — for MongoDB,
// tasks.md 5.6 — sampled document shape) into Mermaid diagram text for the
// schema-diagram feature (spec.md §4.11). It owns no rendering or layout of
// its own: every value this package returns is plain Mermaid syntax, and
// Mermaid itself is the only thing that ever lays it out or draws it,
// matching this project's "never hand-rolled SVG/canvas layout" rule.
package diagram

import (
	"fmt"
	"strings"
	"unicode"

	"stackyard/internal/dbengine"
)

// BuildRelationalERDiagram renders tables and foreignKeys as a Mermaid
// erDiagram (spec.md §4.11): one entity block per table (columns annotated
// PK/FK where applicable) followed by one relationship line per foreign key.
//
// Cardinality is inferred the same way for every foreign key, without
// inspecting whether the referencing column itself carries a UNIQUE
// constraint: a foreign key column is read as "many rows on the referencing
// side may point at one row on the referenced side" — Mermaid's "exactly
// one" / "zero-or-more" pair, `||--o{`, read from the referenced table
// toward the referencing table. This is the standard relational-modeling
// default (a FK is a many-to-one from the child's perspective) and is
// deliberately not upgraded to a one-to-one `||--||` even when the FK column
// happens to be unique, since detecting that would require a second,
// separate metadata query this function does not have — TableInfo/
// ForeignKey carry no uniqueness signal today. A self-referencing foreign
// key (a table whose FK points at its own primary key) renders as a
// relationship line from the table to itself, which is valid Mermaid syntax
// and requires no special-casing here.
//
// Every table is always given its own entity block, including one with no
// foreign keys pointing to or from it, so it still renders as a standalone
// box rather than being silently omitted for lacking a relationship.
func BuildRelationalERDiagram(tables []dbengine.TableInfo, foreignKeys []dbengine.ForeignKey) string {
	fkColumns := foreignKeyColumnSet(foreignKeys)

	var b strings.Builder
	b.WriteString("erDiagram\n")

	for _, table := range tables {
		writeEntityBlock(&b, table, fkColumns)
	}
	for _, fk := range foreignKeys {
		writeRelationshipLine(&b, fk)
	}

	return b.String()
}

// tableColumn identifies one column by its owning table, used only as a map
// key to answer "is this column part of some foreign key" while writing
// entity blocks.
type tableColumn struct {
	table  string
	column string
}

// foreignKeyColumnSet indexes foreignKeys by (table, column) so
// writeEntityBlock can annotate a column as FK without an O(n*m) scan per
// column.
func foreignKeyColumnSet(foreignKeys []dbengine.ForeignKey) map[tableColumn]bool {
	set := make(map[tableColumn]bool, len(foreignKeys))
	for _, fk := range foreignKeys {
		set[tableColumn{table: fk.TableName, column: fk.ColumnName}] = true
	}
	return set
}

// writeEntityBlock appends one Mermaid erDiagram entity block for table,
// annotating each column with its Mermaid-safe type and, where applicable,
// a trailing PK/FK/"PK, FK" key marker.
func writeEntityBlock(b *strings.Builder, table dbengine.TableInfo, fkColumns map[tableColumn]bool) {
	fmt.Fprintf(b, "    %s {\n", mermaidToken(table.Name))
	for _, column := range table.Columns {
		isForeignKey := fkColumns[tableColumn{table: table.Name, column: column.Name}]
		fmt.Fprintf(
			b,
			"        %s %s%s\n",
			mermaidToken(column.DataType),
			mermaidToken(column.Name),
			keyAnnotation(column.IsPrimaryKey, isForeignKey),
		)
	}
	b.WriteString("    }\n")
}

// keyAnnotation returns the trailing " PK", " FK", " PK, FK", or "" a
// column's Mermaid attribute line ends with, matching Mermaid erDiagram's
// own comma-separated key-list syntax for a column that is both a primary
// and a foreign key.
func keyAnnotation(isPrimaryKey, isForeignKey bool) string {
	switch {
	case isPrimaryKey && isForeignKey:
		return " PK, FK"
	case isPrimaryKey:
		return " PK"
	case isForeignKey:
		return " FK"
	default:
		return ""
	}
}

// writeRelationshipLine appends one Mermaid erDiagram relationship line for
// fk, labeled with the referencing column's name so a viewer can tell which
// column drives the relationship without cross-referencing the entity
// blocks above it.
func writeRelationshipLine(b *strings.Builder, fk dbengine.ForeignKey) {
	fmt.Fprintf(
		b,
		"    %s ||--o{ %s : \"via %s\"\n",
		mermaidToken(fk.ReferencedTable),
		mermaidToken(fk.TableName),
		mermaidToken(fk.ColumnName),
	)
}

// mermaidToken makes s safe to use as a Mermaid erDiagram identifier or
// attribute type token: every run of characters that isn't a letter, digit,
// or underscore collapses to a single underscore, and any leading/trailing
// underscore left by that collapsing is trimmed. This matters most for
// database type names, which routinely contain spaces and punctuation
// Mermaid's grammar cannot parse as a single token (Postgres's
// "character varying" or "timestamp without time zone", MySQL's
// "varchar(255)") — table and column names are typically already valid
// identifiers and pass through unchanged, but are run through the same
// function defensively rather than assumed safe. An input that sanitizes to
// the empty string (e.g. one made entirely of punctuation) falls back to
// "col" so a Mermaid attribute line is never left with a missing token.
func mermaidToken(s string) string {
	var b strings.Builder
	lastWasUnderscore := false
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' {
			b.WriteRune(r)
			lastWasUnderscore = false
			continue
		}
		if !lastWasUnderscore {
			b.WriteRune('_')
			lastWasUnderscore = true
		}
	}
	token := strings.Trim(b.String(), "_")
	if token == "" {
		return "col"
	}
	return token
}
