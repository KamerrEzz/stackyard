import type {dbengine} from '../../../wailsjs/go/models'

type ColumnInfo = dbengine.ColumnInfo

const NUMERIC_TYPE_PATTERN = /int|serial|numeric|decimal|float|double|real|money/i
const BOOLEAN_TYPE_PATTERN = /^bool/i

export function isNumericColumnType(dataType: string): boolean {
    return NUMERIC_TYPE_PATTERN.test(dataType)
}

export function isBooleanColumnType(dataType: string): boolean {
    return BOOLEAN_TYPE_PATTERN.test(dataType)
}

export function isTableEditable(columns: readonly ColumnInfo[]): boolean {
    return columns.some((column) => column.IsPrimaryKey)
}

export function primaryKeyColumnNames(columns: readonly ColumnInfo[]): string[] {
    return columns.filter((column) => column.IsPrimaryKey).map((column) => column.Name)
}

/**
 * Builds a primary-key value map for one grid row (tasks.md 4.1) by
 * matching `columns`' primary key names against `resultColumnNames`'
 * positions, independent of column ordering between the two. Returns null
 * whenever the row can't be reliably targeted by an UPDATE/DELETE: the
 * table has no primary key at all, a primary key column isn't present in
 * the result set, or a primary key value in the row itself is null/
 * undefined (a real primary key value is never absent in practice, but a
 * row like that has nothing safe to build a WHERE clause from).
 */
export function extractPkValues(
    columns: readonly ColumnInfo[],
    resultColumnNames: readonly string[],
    row: readonly unknown[],
): Record<string, unknown> | null {
    const pkNames = primaryKeyColumnNames(columns)
    if (pkNames.length === 0) {
        return null
    }

    const values: Record<string, unknown> = {}
    for (const name of pkNames) {
        const index = resultColumnNames.indexOf(name)
        if (index === -1) {
            return null
        }
        const value = row[index]
        if (value === null || value === undefined) {
            return null
        }
        values[name] = value
    }
    return values
}

/**
 * Type-aware default for one column of a brand-new blank row (tasks.md
 * 4.2): a nullable column always defaults to null regardless of its SQL
 * type, since null is always a valid starting point there. A non-nullable
 * column gets the smallest sensible value for its type instead — 0 for
 * numeric types, false for boolean types, an empty string otherwise (text/
 * date/time-like types) — so the row is immediately insertable without the
 * user having to touch every single cell first, while staying visually
 * distinct from NULL (see resultsGridHelpers.describeCell).
 */
export function defaultValueForColumn(column: ColumnInfo): unknown {
    if (column.Nullable) {
        return null
    }
    if (isBooleanColumnType(column.DataType)) {
        return false
    }
    if (isNumericColumnType(column.DataType)) {
        return 0
    }
    return ''
}

export function buildBlankRowValues(columns: readonly ColumnInfo[]): Record<string, unknown> {
    const values: Record<string, unknown> = {}
    for (const column of columns) {
        values[column.Name] = defaultValueForColumn(column)
    }
    return values
}

/**
 * Converts one cell's raw text-input value into the shape sent to
 * UpdateTableRow/InsertTableRow. Clearing a nullable cell to empty commits
 * SQL NULL; a non-nullable cell cleared to empty stays an empty string
 * (NULL isn't valid there anyway). Numeric-typed columns attempt a
 * `Number()` conversion; non-numeric-looking input for a numeric column is
 * passed through as the raw string rather than rejected client-side — the
 * database's own type-mismatch error surfaces inline instead (tasks.md
 * 4.4), rather than this helper guessing at validation rules the DB engine
 * already enforces authoritatively.
 */
export function coerceCellInput(column: ColumnInfo, raw: string): unknown {
    if (raw === '') {
        return column.Nullable ? null : ''
    }
    if (isBooleanColumnType(column.DataType)) {
        if (raw === 'true') return true
        if (raw === 'false') return false
        return raw
    }
    if (isNumericColumnType(column.DataType)) {
        const asNumber = Number(raw)
        return Number.isNaN(asNumber) ? raw : asNumber
    }
    return raw
}

/**
 * Builds the values map InsertTableRow receives for a blank row (tasks.md
 * 4.2). A primary key column the user never touched is omitted entirely so
 * the database can apply its own default (the common auto-increment/serial
 * case); a primary key column the user explicitly edited is included
 * as-is, supporting manual primary key assignment. Every non-primary-key
 * column is always included, using its current value (its type-aware
 * default if the user left it untouched).
 */
export function buildInsertPayload(
    columns: readonly ColumnInfo[],
    values: Record<string, unknown>,
    touchedColumns: ReadonlySet<string>,
): Record<string, unknown> {
    const payload: Record<string, unknown> = {}
    for (const column of columns) {
        if (column.IsPrimaryKey && !touchedColumns.has(column.Name)) {
            continue
        }
        payload[column.Name] = values[column.Name]
    }
    return payload
}

const PK_LESS_ERROR_SUBSTRING = 'read-only: table has no primary key'

/**
 * Detects grid.go's ErrTableHasNoPrimaryKey as it arrives on the frontend
 * (Wails rejects promises with an error whose message contains this
 * literal substring, per UpdateTableRow/DeleteTableRows' doc comments) —
 * the one write-failure case the grid should react to defensively even
 * though `isTableEditable` is already checked before the user can start
 * editing a table.
 */
export function isPkLessError(message: string): boolean {
    return message.includes(PK_LESS_ERROR_SUBSTRING)
}
