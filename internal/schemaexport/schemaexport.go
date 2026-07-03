package schemaexport

import "stackyard/internal/dbengine"

// tableColumnKey identifies one column by its owning table, used as a map
// key by both prisma.go and drizzle.go to look up a column's foreign key
// (if any) without an O(n*m) scan per column — mirrors
// internal/diagram/relational.go's own tableColumn helper, duplicated here
// rather than shared since that type is unexported in its own package too
// (see internal/export/sqldump.go's qualifiedDumpTableName doc comment for
// this codebase's established precedent on small intentional per-package
// duplication over introducing a new shared/exported dependency for it).
type tableColumnKey struct {
	table  string
	column string
}

// primaryKeyColumnNames returns table's primary key column names in the
// same order ListTables reported its columns — unlike grid.go's own
// primaryKeyColumns, which alphabetizes them for a deterministic SQL WHERE
// clause, both Prisma's `@@id([...])` and Drizzle's `primaryKey({ columns:
// [...] })` are read more naturally in the table's own declared column
// order, and neither has grid.go's determinism requirement (there is no
// query being built from this ordering).
func primaryKeyColumnNames(table dbengine.TableInfo) []string {
	var names []string
	for _, col := range table.Columns {
		if col.IsPrimaryKey {
			names = append(names, col.Name)
		}
	}
	return names
}
