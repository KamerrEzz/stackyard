package dbengine

import (
	"fmt"
	"strings"
)

// ColumnType is one entry from the curated, non-overwhelming set of column
// types the "Create table" UI (tasks.md 10.2) exposes — not an exhaustive
// mirror of every Postgres/MySQL type, deliberately: text/varchar for
// strings, integer/bigint for whole numbers, serial/bigserial for
// auto-increment primary keys, boolean, timestamp, and numeric for
// arbitrary-precision decimals covers what someone starting a new project
// reaches for first, without the form becoming a full SQL-type picker.
type ColumnType string

const (
	ColumnTypeText      ColumnType = "text"
	ColumnTypeVarchar   ColumnType = "varchar"
	ColumnTypeInteger   ColumnType = "integer"
	ColumnTypeBigInt    ColumnType = "bigint"
	ColumnTypeSerial    ColumnType = "serial"
	ColumnTypeBigSerial ColumnType = "bigserial"
	ColumnTypeBoolean   ColumnType = "boolean"
	ColumnTypeTimestamp ColumnType = "timestamp"
	ColumnTypeNumeric   ColumnType = "numeric"
)

// createTableVarcharLength is the fixed length used for every
// ColumnTypeVarchar column. MySQL requires VARCHAR to carry an explicit
// length (unlike Postgres, which accepts a bare VARCHAR); rather than adding
// a length input to the "Create table" form — one more field for a UI this
// task deliberately keeps to a curated, non-overwhelming set of choices — a
// single reasonable default is used for both dialects. A user who needs a
// different length can always adjust the column afterward with a
// hand-written ALTER TABLE via the query editor, the same "form is a
// convenience over raw SQL" framing this whole feature is scoped under.
const createTableVarcharLength = 255

// ColumnDefinition is one user-authored column for BuildCreateTableDDL,
// the shape the "Create table" form (tasks.md 10.2) lets the user construct
// directly, one row per column. Default is a raw SQL expression (e.g. "0",
// "'active'", "now()") appended verbatim after the DDL's DEFAULT keyword
// rather than a typed Go value quoted per-type: the query editor already
// trusts the user to type valid SQL directly (spec.md §4.6), and this form
// is a convenience over writing CREATE TABLE by hand, not a sandboxed
// input — so a default expression gets the same trust level as everything
// else the user can already run. A nil or blank (after trimming) Default
// means "no DEFAULT clause".
type ColumnDefinition struct {
	Name         string
	Type         ColumnType
	Nullable     bool
	IsPrimaryKey bool
	Default      *string
}

// BuildCreateTableDDL renders a single CREATE TABLE statement (no trailing
// semicolon, matching internal/export.BuildCreateTable's own convention) for
// table from columns, per dialect. Every identifier is quoted via
// QuoteIdentifier and every primary-key column collected into a trailing
// PRIMARY KEY clause, the same shape internal/export.BuildCreateTable
// produces — but this function is not a thin wrapper around it: columns
// here carries a curated ColumnType key that still needs resolving into a
// concrete, dialect-valid SQL type name (and, for ColumnTypeSerial/
// ColumnTypeBigSerial, an auto-increment mechanism that differs entirely
// between Postgres and MySQL), whereas internal/export.ColumnDumpInfo.SQLType
// is already a complete, dialect-valid type string by the time it reaches
// that package (re-serialized from an existing table's own introspected
// columns). That type-resolution step — plus the auto-increment-specific
// validation below — is genuinely new logic with no equivalent in the
// export path, so it lives here as its own function rather than
// reshaping columns into ColumnDumpInfo and calling into internal/export.
//
// Returns an error, before generating any SQL, when: table is empty,
// columns is empty, any column has an empty/blank name, two columns share
// the same name, a column names an unrecognized ColumnType, or a
// ColumnTypeSerial/ColumnTypeBigSerial column is not marked IsPrimaryKey or
// carries an explicit Default (auto-increment columns generate their own
// value; an explicit default alongside one is either redundant or, on
// MySQL, invalid DDL). Every other per-engine validation rule (e.g. MySQL
// only allowing one AUTO_INCREMENT column per table) is left to the
// database's own error at execution time, the same "let the real engine
// report it" philosophy gridEditHelpers.coerceCellInput's doc comment
// already documents for the editable grid.
func BuildCreateTableDDL(dialect Dialect, schema, table string, columns []ColumnDefinition) (string, error) {
	if strings.TrimSpace(table) == "" {
		return "", fmt.Errorf("build create table: table name is required")
	}
	if len(columns) == 0 {
		return "", fmt.Errorf("build create table: at least one column is required")
	}

	seenNames := make(map[string]bool, len(columns))
	lines := make([]string, 0, len(columns)+1)
	var pkColumns []string

	for _, col := range columns {
		if strings.TrimSpace(col.Name) == "" {
			return "", fmt.Errorf("build create table: column name is required")
		}
		if seenNames[col.Name] {
			return "", fmt.Errorf("build create table: duplicate column name %q", col.Name)
		}
		seenNames[col.Name] = true

		sqlType, autoIncrement, err := columnSQLType(dialect, col.Type)
		if err != nil {
			return "", fmt.Errorf("build create table: column %q: %w", col.Name, err)
		}

		hasDefault := col.Default != nil && strings.TrimSpace(*col.Default) != ""
		if autoIncrement {
			if !col.IsPrimaryKey {
				return "", fmt.Errorf("build create table: column %q: %s columns must be the primary key", col.Name, col.Type)
			}
			if hasDefault {
				return "", fmt.Errorf("build create table: column %q: an auto-increment column cannot have an explicit default", col.Name)
			}
		}

		line := fmt.Sprintf("  %s %s", QuoteIdentifier(dialect, col.Name), sqlType)
		if !col.Nullable || autoIncrement {
			line += " NOT NULL"
		}
		if dialect == DialectMySQL && autoIncrement {
			line += " AUTO_INCREMENT"
		}
		if hasDefault {
			line += " DEFAULT " + strings.TrimSpace(*col.Default)
		}
		lines = append(lines, line)

		if col.IsPrimaryKey {
			pkColumns = append(pkColumns, col.Name)
		}
	}

	if len(pkColumns) > 0 {
		quoted := make([]string, len(pkColumns))
		for i, name := range pkColumns {
			quoted[i] = QuoteIdentifier(dialect, name)
		}
		lines = append(lines, fmt.Sprintf("  PRIMARY KEY (%s)", strings.Join(quoted, ", ")))
	}

	return fmt.Sprintf("CREATE TABLE %s (\n%s\n)", qualifiedTableName(dialect, schema, table), strings.Join(lines, ",\n")), nil
}

// columnSQLType resolves colType into dialect's own concrete SQL type name,
// plus whether that type is an auto-increment mechanism. Postgres's
// SERIAL/BIGSERIAL pseudo-types already are the auto-increment mechanism
// (a NOT NULL column backed by an owned sequence), so no separate keyword
// is added for them; MySQL has no such pseudo-type, so ColumnTypeSerial/
// ColumnTypeBigSerial resolve to a plain INT/BIGINT here and
// BuildCreateTableDDL appends the literal AUTO_INCREMENT keyword itself.
//
// ColumnTypeTimestamp deliberately resolves to MySQL's DATETIME rather than
// its own TIMESTAMP type: MySQL's TIMESTAMP has historically carried
// implicit "auto-initialize/auto-update to CURRENT_TIMESTAMP" behavior for
// whichever column defines it first unless the server's
// explicit_defaults_for_timestamp setting is on (the default since MySQL
// 5.7.8, but not guaranteed for every server this app might connect to) —
// DATETIME never carries that surprise regardless of server configuration,
// which matters here since this form lets the user create a plain,
// unadorned nullable/non-nullable column with no way to inspect or opt out
// of that implicit behavior if TIMESTAMP silently added it. Both store an
// equivalent date+time value for this feature's purposes.
//
// ColumnTypeNumeric resolves to a bare NUMERIC (Postgres) / DECIMAL (MySQL)
// with no explicit precision/scale — both engines accept the bare form
// (Postgres: arbitrary precision; MySQL: defaults to DECIMAL(10,0)) — rather
// than adding precision/scale inputs to the form, the same "keep the type
// list curated" reasoning createTableVarcharLength's doc comment explains
// for VARCHAR.
func columnSQLType(dialect Dialect, colType ColumnType) (sqlType string, autoIncrement bool, err error) {
	switch colType {
	case ColumnTypeText:
		return "TEXT", false, nil
	case ColumnTypeVarchar:
		return fmt.Sprintf("VARCHAR(%d)", createTableVarcharLength), false, nil
	case ColumnTypeInteger:
		if dialect == DialectMySQL {
			return "INT", false, nil
		}
		return "INTEGER", false, nil
	case ColumnTypeBigInt:
		return "BIGINT", false, nil
	case ColumnTypeSerial:
		if dialect == DialectMySQL {
			return "INT", true, nil
		}
		return "SERIAL", true, nil
	case ColumnTypeBigSerial:
		if dialect == DialectMySQL {
			return "BIGINT", true, nil
		}
		return "BIGSERIAL", true, nil
	case ColumnTypeBoolean:
		return "BOOLEAN", false, nil
	case ColumnTypeTimestamp:
		if dialect == DialectMySQL {
			return "DATETIME", false, nil
		}
		return "TIMESTAMP", false, nil
	case ColumnTypeNumeric:
		if dialect == DialectMySQL {
			return "DECIMAL", false, nil
		}
		return "NUMERIC", false, nil
	default:
		return "", false, fmt.Errorf("unsupported column type %q", colType)
	}
}
