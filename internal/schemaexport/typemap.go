package schemaexport

import "stackyard/internal/dbengine"

// prismaFallbackScalar is used for any Postgres/MySQL data_type this
// package does not recognize (an exotic/extension type, e.g. Postgres's
// "tsvector" or "geometry" from a PostGIS install, or a MySQL "geometry"
// spatial column) — String is Prisma's most permissive scalar and keeps the
// generated schema valid rather than omitting the column, at the cost of
// losing the original type's real shape. Every mapping below that resolves
// through this fallback, or that is itself already a narrowing judgment
// call, is documented in this task's final report.
const prismaFallbackScalar = "String"

// postgresToPrismaScalar maps Postgres's information_schema.columns.data_type
// (as ColumnInfo.DataType already carries it verbatim via ListTables) to a
// Prisma scalar. Array columns are not mapped here: Postgres reports every
// array column's data_type as the bare literal "ARRAY" regardless of
// element type, with the actual element type only recoverable from a
// separate udt_name/element_type catalog lookup this task's scope
// explicitly excludes ("no new DB queries") — an "ARRAY" column falls
// through to prismaFallbackScalar like any other unrecognized type.
var postgresToPrismaScalar = map[string]string{
	"character varying": "String",
	"character":         "String",
	"text":              "String",
	"citext":            "String",
	"uuid":              "String",
	"xml":               "String",
	"inet":              "String",
	"cidr":              "String",
	"macaddr":           "String",
	"integer":           "Int",
	"smallint":          "Int",
	"bigint":            "BigInt",
	"boolean":           "Boolean",
	"numeric":           "Decimal",
	"real":              "Float",
	"double precision":  "Float",

	"timestamp without time zone": "DateTime",
	"timestamp with time zone":    "DateTime",
	"date":                        "DateTime",
	"time without time zone":      "DateTime",
	"time with time zone":         "DateTime",

	"json":  "Json",
	"jsonb": "Json",
	"bytea": "Bytes",
}

// mysqlToPrismaScalar maps MySQL's information_schema.columns.DATA_TYPE
// (the bare type keyword, e.g. "varchar", "int" — not the fuller COLUMN_TYPE
// string with length/precision export.go's SQL dump path separately queries)
// to a Prisma scalar.
//
// "tinyint" always maps to Int, never Boolean: MySQL's conventional
// tinyint(1)-as-boolean idiom is only visible in COLUMN_TYPE's display width
// ("tinyint(1)" vs "tinyint(4)"), which this task's scope does not query —
// DATA_TYPE alone reports "tinyint" for both, so every tinyint column,
// including genuine booleans, exports as Int. This is a documented, lossy
// judgment call. "boolean"/"bool" entries exist defensively in case some
// driver/version ever reports DATA_TYPE that way, but real MySQL
// information_schema output does not use them — BOOLEAN is a TINYINT(1)
// alias resolved to "tinyint" before this map ever sees it.
var mysqlToPrismaScalar = map[string]string{
	"varchar":    "String",
	"char":       "String",
	"text":       "String",
	"tinytext":   "String",
	"mediumtext": "String",
	"longtext":   "String",
	"enum":       "String",
	"set":        "String",

	"int":       "Int",
	"smallint":  "Int",
	"mediumint": "Int",
	"tinyint":   "Int",
	"year":      "Int",
	"bigint":    "BigInt",

	"decimal": "Decimal",
	"float":   "Float",
	"double":  "Float",

	"boolean": "Boolean",
	"bool":    "Boolean",

	"date":      "DateTime",
	"datetime":  "DateTime",
	"timestamp": "DateTime",
	"time":      "DateTime",

	"json": "Json",

	"blob":       "Bytes",
	"tinyblob":   "Bytes",
	"mediumblob": "Bytes",
	"longblob":   "Bytes",
	"binary":     "Bytes",
	"varbinary":  "Bytes",
}

// prismaScalarFor resolves dataType (as reported by ListTables for dialect)
// to the Prisma scalar its column should use, falling back to
// prismaFallbackScalar for anything unrecognized.
func prismaScalarFor(dialect dbengine.Dialect, dataType string) string {
	table := postgresToPrismaScalar
	if dialect == dbengine.DialectMySQL {
		table = mysqlToPrismaScalar
	}
	if scalar, ok := table[dataType]; ok {
		return scalar
	}
	return prismaFallbackScalar
}

// drizzleFallbackBuilder mirrors prismaFallbackScalar's role for Drizzle:
// `text` is the most permissive column builder available in both
// drizzle-orm/pg-core and drizzle-orm/mysql-core.
const drizzleFallbackBuilder = "text"

// postgresToDrizzleBuilder maps a Postgres data_type to the
// drizzle-orm/pg-core column builder function name that should render it.
// "serial" is deliberately never produced: ColumnInfo carries no
// autoincrement/default-sequence signal, so every Postgres integer column —
// including primary keys — renders as `integer(...)`, not `serial(...)`.
// This is a plain, valid, non-lossy (if more verbose) choice, not a lossy
// judgment call: a manually-sequenced integer primary key still round-trips
// a structurally correct schema; the only difference is that this generated
// file, if ever applied as real Drizzle migrations (explicitly out of this
// task's scope — this is a read-only export), would not also recreate the
// identity/sequence DDL a `serial` column implies. "bytea" maps to "text":
// drizzle-orm/pg-core has no dedicated binary/bytea column builder as of
// this generator's target version, so "text" is the closest always-valid
// stand-in — documented as lossy.
var postgresToDrizzleBuilder = map[string]string{
	"character varying": "varchar",
	"character":         "char",
	"text":              "text",
	"citext":            "text",
	"uuid":              "uuid",
	"xml":               "text",
	"inet":              "text",
	"cidr":              "text",
	"macaddr":           "text",
	"integer":           "integer",
	"smallint":          "smallint",
	"bigint":            "bigint",
	"boolean":           "boolean",
	"numeric":           "numeric",
	"real":              "real",
	"double precision":  "doublePrecision",

	"timestamp without time zone": "timestamp",
	"timestamp with time zone":    "timestamp",
	"date":                        "date",
	"time without time zone":      "time",
	"time with time zone":         "time",

	"json":  "json",
	"jsonb": "jsonb",

	"bytea": "text",
}

// mysqlToDrizzleBuilder maps a MySQL DATA_TYPE to the
// drizzle-orm/mysql-core column builder function name that should render
// it. "year" maps to "int" (mysql-core's `year` builder support is not
// reliable across versions this generator can't pin down without a real
// toolchain check, so the safe, always-valid choice is a plain integer,
// documented as lossy). Binary/blob types map to "text" for the same reason
// bytea does in the Postgres table above.
var mysqlToDrizzleBuilder = map[string]string{
	"varchar":    "varchar",
	"char":       "char",
	"text":       "text",
	"tinytext":   "text",
	"mediumtext": "text",
	"longtext":   "text",
	"enum":       "text",
	"set":        "text",

	"int":       "int",
	"smallint":  "smallint",
	"mediumint": "mediumint",
	"tinyint":   "tinyint",
	"year":      "int",
	"bigint":    "bigint",

	"decimal": "decimal",
	"float":   "float",
	"double":  "double",

	"boolean": "boolean",
	"bool":    "boolean",

	"date":      "date",
	"datetime":  "datetime",
	"timestamp": "timestamp",
	"time":      "time",

	"json": "json",

	"blob":       "text",
	"tinyblob":   "text",
	"mediumblob": "text",
	"longblob":   "text",
	"binary":     "text",
	"varbinary":  "text",
}

// drizzleDefaultVarcharLength is the length drizzle-orm/mysql-core's
// varchar/char builders render when a length isn't otherwise known — MySQL
// requires a length for both. This generator has no length/precision
// available (ColumnInfo.DataType carries only the bare type keyword; the
// real length lives in COLUMN_TYPE, a lookup this task's "no new DB
// queries" scope excludes), so every MySQL varchar/char column renders with
// this fixed default rather than the column's real declared length — a
// documented, lossy judgment call.
const drizzleDefaultVarcharLength = 255

// drizzleBuilderFor resolves dataType (as reported by ListTables for
// dialect) to the bare drizzle-orm column builder function name, falling
// back to drizzleFallbackBuilder for anything unrecognized. Callers needing
// the full call expression (with per-builder argument shape, e.g. MySQL's
// mandatory varchar length or bigint's mandatory mode) use
// drizzleColumnCall instead.
func drizzleBuilderFor(dialect dbengine.Dialect, dataType string) string {
	table := postgresToDrizzleBuilder
	if dialect == dbengine.DialectMySQL {
		table = mysqlToDrizzleBuilder
	}
	if builder, ok := table[dataType]; ok {
		return builder
	}
	return drizzleFallbackBuilder
}
