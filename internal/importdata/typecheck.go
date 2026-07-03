package importdata

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// columnCategory groups a target column's dbengine.ColumnInfo.DataType into
// the handful of plausibility rules isValuePlausible checks a cell's value
// against — see categorizeDataType for the exact mapping.
type columnCategory int

const (
	categoryUnknown columnCategory = iota
	categoryInteger
	categoryNumeric
	categoryBoolean
	categoryDateTime
	categoryText
)

// categorizeDataType maps one dbengine.ColumnInfo.DataType string, exactly
// as reported by Postgres's or MySQL's information_schema.columns.data_type
// (see internal/dbengine/postgres/postgres.go and
// internal/dbengine/mysql/mysql.go), to the category isValuePlausible checks
// a cell's value against. Matching is done against the finite, known
// vocabulary these two engines actually report (a case-insensitive exact
// match, not a substring search — a substring search would, for example,
// wrongly match Postgres's "point"/"interval" types against the "int"
// integer rule) rather than an exhaustive type system. Any DataType string
// not in this list — Postgres's uuid/json/jsonb/bytea/point/interval/array/
// USER-DEFINED types, MySQL's enum/set/blob/bit/binary/json — falls back to
// categoryUnknown, which isValuePlausible always accepts: this validator is
// documented as reasonable rather than exhaustive (see the package doc), and
// silently letting an unrecognized type through is safer than guessing wrong
// and rejecting a genuinely valid import.
//
// MySQL's BOOLEAN column type is a bare alias for TINYINT(1); its
// information_schema.columns.data_type reports "tinyint" for both, with no
// way to tell them apart at this layer (MySQL's own column_type value would
// carry "tinyint(1)", but ColumnInfo does not expose it — see
// internal/dbengine/mysql/mysql.go's ListTables). Such a column is therefore
// validated as categoryInteger, not categoryBoolean: a MySQL boolean column
// fed the literal strings "true"/"false" is flagged as implausible under
// this rule set — only 0/1 pass. This is a known, documented, MySQL-only
// limitation. Postgres's own "boolean" data_type has no such ambiguity.
func categorizeDataType(dataType string) columnCategory {
	switch strings.ToLower(strings.TrimSpace(dataType)) {
	case "boolean", "bool":
		return categoryBoolean
	case "integer", "int", "int2", "int4", "int8", "bigint", "smallint", "tinyint", "mediumint",
		"serial", "bigserial", "smallserial", "year":
		return categoryInteger
	case "numeric", "decimal", "real", "double precision", "double", "float", "money":
		return categoryNumeric
	case "date", "datetime", "timestamp", "timestamp without time zone", "timestamp with time zone",
		"time", "time without time zone", "time with time zone":
		return categoryDateTime
	case "character varying", "varchar", "character", "char", "text",
		"tinytext", "mediumtext", "longtext", "citext":
		return categoryText
	default:
		return categoryUnknown
	}
}

// dateTimeLayouts is the fixed set of layouts looksDateTime tries in order,
// covering both engines' common textual date/time representations
// (RFC3339-ish timestamps, a bare date, a space- or "T"-separated date and
// time, and a bare time). This is a documented, reasonable set, not an
// exhaustive list of every locale/format a spreadsheet export might produce.
var dateTimeLayouts = []string{
	time.RFC3339,
	time.RFC3339Nano,
	"2006-01-02",
	"2006-01-02 15:04:05",
	"2006-01-02T15:04:05",
	"2006-01-02 15:04:05.999999999",
	"2006/01/02",
	"01/02/2006",
	"15:04:05",
}

func looksDateTime(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	for _, layout := range dateTimeLayouts {
		if _, err := time.Parse(layout, s); err == nil {
			return true
		}
	}
	return false
}

func looksInteger(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	_, err := strconv.ParseInt(s, 10, 64)
	return err == nil
}

func looksNumeric(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	_, err := strconv.ParseFloat(s, 64)
	return err == nil
}

// booleanLiterals is the fixed set of case-insensitive strings looksBoolean
// accepts, covering Postgres's own accepted boolean literal spellings
// ("true"/"false"/"t"/"f"/"yes"/"no"/"y"/"n"/"1"/"0") plus the equivalent
// spellings a CSV/JSON export of a boolean column commonly produces.
var booleanLiterals = map[string]bool{
	"true": true, "false": true,
	"t": true, "f": true,
	"yes": true, "no": true,
	"y": true, "n": true,
	"1": true, "0": true,
}

func looksBoolean(s string) bool {
	return booleanLiterals[strings.ToLower(strings.TrimSpace(s))]
}

// stringifyValue renders value (a CSV cell, always a Go string, or a
// JSON-decoded string/float64/bool/nil/map/slice) as text for the looks*
// literal-format checks above.
func stringifyValue(value any) string {
	if s, ok := value.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", value)
}

// isValuePlausible checks value — a cell already known to be non-null and
// present for a column, see Validate for how nullness is handled separately
// — against category's plausibility rule (tasks.md 7.4's "basic type-
// plausibility check"). This is a best-effort format check, not a guarantee
// the database will accept the value: a plausible-looking integer can still
// violate a CHECK constraint or exceed a column's precision, for example.
// categoryText and categoryUnknown always accept any value, per this task's
// explicit "anything is plausible" rule for text columns and the documented
// safe fallback for a DataType this package does not recognize.
func isValuePlausible(value any, category columnCategory) bool {
	switch category {
	case categoryText, categoryUnknown:
		return true
	case categoryBoolean:
		if _, ok := value.(bool); ok {
			return true
		}
		return looksBoolean(stringifyValue(value))
	case categoryInteger:
		switch n := value.(type) {
		case float64:
			return n == float64(int64(n))
		case int, int32, int64:
			return true
		}
		return looksInteger(stringifyValue(value))
	case categoryNumeric:
		switch value.(type) {
		case float64, int, int32, int64:
			return true
		}
		return looksNumeric(stringifyValue(value))
	case categoryDateTime:
		return looksDateTime(stringifyValue(value))
	default:
		return true
	}
}
