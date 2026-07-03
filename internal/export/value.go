// Package export is the shared formatting layer for tasks.md 7.1-7.3
// (spec.md §4.9): it converts an already-fetched (column names, rows) pair
// into CSV text, JSON text, or a SQL dump's CREATE TABLE + INSERT text. It
// has no database dependency of its own — both export scopes spec.md §4.9
// requires ("full table" and "the current query result") converge on this
// package once their data has been fetched by whichever caller owns that
// concern (see export.go at the repo root for both entry points), so the
// same NULL-vs-empty-string and date/numeric formatting rules apply
// regardless of where the rows came from.
package export

import (
	"database/sql/driver"
	"fmt"
	"strconv"
	"strings"
	"time"

	"stackyard/internal/dbengine"
)

// formatCSVValue renders v as CSV cell text, reporting isNull separately so
// the caller can distinguish a SQL NULL from an empty string — the exact
// values a CSV writer needs to implement this package's NULL convention (see
// ToCSV's doc comment). time.Time is formatted as RFC3339Nano (ISO-8601, not
// Go's default time.Time.String() layout) so a re-imported date/timestamp
// column round-trips unambiguously across both Postgres (which returns
// time.Time for a temporal column via pgx) and MySQL (which returns
// time.Time when its DSN carries parseTime=true — see internal/dbengine/mysql's
// own doc comment on that requirement; app.go's OpenConnection always forces
// it). A driver.Valuer value (e.g. pgx's pgtype.Numeric for a Postgres
// NUMERIC/DECIMAL column, which pgx's Values() scans into rather than a plain
// float64 to avoid silently losing precision) is resolved through its own
// Value() method and re-formatted from the result, rather than being
// stringified via reflection, so a NUMERIC column's text is exactly the
// value's own decimal representation. Every other type (already-JSON-native
// values arriving from the "current query result" export path, having
// already round-tripped through Wails' own JSON bridge once — see
// export.go's doc comment on ExportQueryResultAsCSV) falls back to
// fmt.Sprintf("%v", v), which is exactly right for bool/int/float/string.
func formatCSVValue(v any) (text string, isNull bool) {
	if v == nil {
		return "", true
	}
	switch val := v.(type) {
	case time.Time:
		return val.Format(time.RFC3339Nano), false
	case []byte:
		return string(val), false
	case driver.Valuer:
		resolved, err := val.Value()
		if err != nil {
			return fmt.Sprintf("%v", v), false
		}
		return formatCSVValue(resolved)
	default:
		return fmt.Sprintf("%v", v), false
	}
}

// normalizeJSONValue adjusts v just enough for encoding/json to produce the
// output this package wants, then lets json.Marshal do the rest natively:
// time.Time already implements json.Marshaler (RFC3339Nano, the same layout
// formatCSVValue uses, kept as encoding/json's default rather than
// overridden — it's already ISO-8601 and already round-trips), and
// pgtype.Numeric already implements json.Marshaler too (a bare JSON number),
// so neither needs special handling here. Only []byte is overridden: by
// default encoding/json base64-encodes a []byte, which would silently break
// this package's "human-readable, re-importable text" goal for any driver
// that surfaces a textual/decimal column as raw bytes; converting it to a
// plain string first keeps its output consistent with formatCSVValue's own
// []byte handling.
func normalizeJSONValue(v any) any {
	if b, ok := v.([]byte); ok {
		return string(b)
	}
	return v
}

// formatSQLLiteral renders v as a literal suitable for direct embedding in a
// generated SQL dump's INSERT statement (tasks.md 7.3) — never used for
// query parameters bound through Engine.Exec, which is how every other
// grid/query code path in this codebase (internal/dbengine/gridsql.go,
// BuildUpdateRow/BuildInsertRow/BuildDeleteRow) avoids building executable
// SQL text from untrusted values. A SQL dump has no equivalent bound-
// parameter path — it is a standalone .sql file meant to be replayed later,
// possibly by a different tool entirely — so this function is the one place
// in the codebase that must escape a value for literal embedding correctly,
// per dialect (see quoteSQLString for exactly what differs between Postgres
// and MySQL). bool is rendered as the literal TRUE/FALSE (both dialects
// accept it — MySQL treats TRUE/FALSE as synonyms for 1/0). time.Time uses a
// space-separated "YYYY-MM-DD HH:MM:SS[.fraction]" layout (Go's ".999999999"
// directive drops trailing zero fractional digits, including the whole
// fractional part when it's exactly zero) rather than RFC3339's "T"
// separator, since that space-separated form is the one both Postgres and
// MySQL accept unambiguously as a timestamp literal. A driver.Valuer (pgx's
// pgtype.Numeric) is resolved via formatSQLNumericLiteral, since it always
// represents a numeric column in this codebase's usage — never a
// string/date-like value — and is therefore rendered as a bare, unquoted
// numeric token, matching how a plain Go int/float value is rendered below.
func formatSQLLiteral(dialect dbengine.Dialect, v any) string {
	if v == nil {
		return "NULL"
	}
	switch val := v.(type) {
	case bool:
		if val {
			return "TRUE"
		}
		return "FALSE"
	case time.Time:
		return quoteSQLString(dialect, val.Format("2006-01-02 15:04:05.999999999"))
	case []byte:
		return quoteSQLString(dialect, string(val))
	case driver.Valuer:
		resolved, err := val.Value()
		if err != nil {
			return quoteSQLString(dialect, fmt.Sprintf("%v", v))
		}
		if resolved == nil {
			return "NULL"
		}
		return formatSQLNumericLiteral(resolved)
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64:
		return fmt.Sprintf("%v", val)
	case string:
		return quoteSQLString(dialect, val)
	default:
		return quoteSQLString(dialect, fmt.Sprintf("%v", val))
	}
}

// formatSQLNumericLiteral renders a driver.Value already known to represent
// a numeric column (see formatSQLLiteral's driver.Valuer case) as a bare,
// unquoted SQL token.
func formatSQLNumericLiteral(v driver.Value) string {
	switch val := v.(type) {
	case string:
		return val
	case int64:
		return strconv.FormatInt(val, 10)
	case float64:
		return strconv.FormatFloat(val, 'f', -1, 64)
	default:
		return fmt.Sprintf("%v", val)
	}
}

// quoteSQLString wraps s in single quotes, escaped per dialect's own string-
// literal rules: both Postgres and MySQL require doubling an embedded single
// quote, but MySQL additionally treats a bare backslash as an escape
// character inside a string literal (its default sql_mode does not set
// NO_BACKSLASH_ESCAPES) while Postgres's standard_conforming_strings default
// (on since Postgres 9.1) treats backslash as a literal character. Skipping
// the backslash escape on MySQL would let a value containing a trailing
// backslash swallow the closing quote that follows it (e.g. a value ending
// in a literal "\" immediately before the closing "'" reads as an escaped
// quote, not a string terminator) — exactly the SQL-injection-shaped bug
// this function exists to avoid in a hand-built dump file. Backslashes are
// escaped before quotes so the two replacements can't interact.
func quoteSQLString(dialect dbengine.Dialect, s string) string {
	escaped := s
	if dialect == dbengine.DialectMySQL {
		escaped = strings.ReplaceAll(escaped, `\`, `\\`)
	}
	escaped = strings.ReplaceAll(escaped, `'`, `''`)
	return "'" + escaped + "'"
}
