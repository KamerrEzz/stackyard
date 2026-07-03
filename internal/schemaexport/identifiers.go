// Package schemaexport renders schema metadata already discovered via
// dbengine.Engine (tables, columns, foreign keys — the same introspection
// internal/diagram already consumes for the schema-diagram feature) into two
// read-only ORM schema targets: a Prisma schema.prisma file and a Drizzle
// ORM schema.ts file (tasks.md 10.4/10.5). It owns no database access of its
// own — every function here is a pure string builder over the structs
// dbengine.Engine.ListTables/ListForeignKeys already return.
package schemaexport

import (
	"strings"
	"unicode"
)

// camelCase converts a SQL identifier such as "author_id" into the camelCase
// form idiomatic Drizzle schema property/table names use ("authorId"),
// splitting on any run of non-alphanumeric characters. A name with no such
// separators (the common case for already-short table/column names like
// "widgets" or "id") comes back with only its first rune lowercased. The
// original identifier is never lost by this transform — every Drizzle
// column/table builder call still carries it verbatim as the builder's own
// string argument (e.g. `authorId: integer("author_id")`), so camelCasing
// here is purely a JS-side naming convention, not a lossy rename.
func camelCase(name string) string {
	words := splitIdentifierWords(name)
	if len(words) == 0 {
		return name
	}
	var b strings.Builder
	for i, word := range words {
		if word == "" {
			continue
		}
		runes := []rune(word)
		if i == 0 {
			b.WriteRune(unicode.ToLower(runes[0]))
			b.WriteString(strings.ToLower(string(runes[1:])))
			continue
		}
		b.WriteRune(unicode.ToUpper(runes[0]))
		b.WriteString(strings.ToLower(string(runes[1:])))
	}
	return b.String()
}

func splitIdentifierWords(name string) []string {
	return strings.FieldsFunc(name, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
}
