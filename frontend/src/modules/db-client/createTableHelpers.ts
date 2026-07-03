import {dbengine} from '../../../wailsjs/go/models'

/**
 * The curated, non-overwhelming column type list the "Create table" form
 * (tasks.md 10.2) exposes — mirrors internal/dbengine/createtable.go's
 * ColumnType constants exactly; keep both in sync if the set ever changes.
 * `serial`/`bigserial` are the auto-increment-primary-key types: the form
 * only allows either to be picked while its row's primary key checkbox is
 * also checked (see validateCreateTableForm), matching
 * BuildCreateTableDDL's own requirement.
 */
export const COLUMN_TYPE_OPTIONS: ReadonlyArray<{value: string; label: string}> = [
    {value: 'text', label: 'Text'},
    {value: 'varchar', label: 'Varchar (255)'},
    {value: 'integer', label: 'Integer'},
    {value: 'bigint', label: 'Big integer'},
    {value: 'serial', label: 'Serial (auto-increment, primary key only)'},
    {value: 'bigserial', label: 'Big serial (auto-increment, primary key only)'},
    {value: 'boolean', label: 'Boolean'},
    {value: 'timestamp', label: 'Timestamp'},
    {value: 'numeric', label: 'Numeric / decimal'},
]

const AUTO_INCREMENT_TYPES = new Set(['serial', 'bigserial'])

export function isAutoIncrementType(type: string): boolean {
    return AUTO_INCREMENT_TYPES.has(type)
}

/**
 * One column row's editable form state. A plain object rather than
 * `dbengine.ColumnDefinition` itself: `defaultValue` is always a string here
 * (an empty text input, never undefined) so a controlled `<input>` never
 * flips between controlled/uncontrolled, and `isPrimaryKey` is enforced
 * single-valued across a whole row list by setPrimaryKey below rather than
 * left as an independent per-row boolean the way `dbengine.ColumnDefinition`
 * itself allows (composite primary keys are a valid DDL shape, but this
 * form deliberately curates down to "zero or one primary key column," the
 * same "keep it non-overwhelming" scope every other curated choice in this
 * feature follows — see internal/dbengine/createtable.go's own doc
 * comments for the matching examples on the Go side).
 */
export interface ColumnFormRow {
    name: string
    type: string
    nullable: boolean
    isPrimaryKey: boolean
    defaultValue: string
}

export function createBlankColumnRow(): ColumnFormRow {
    return {name: '', type: 'text', nullable: true, isPrimaryKey: false, defaultValue: ''}
}

/**
 * Sets rows[index]'s primary key flag, clearing every other row's flag when
 * isPrimaryKey is true — radio-button behavior across the whole column list,
 * so "at most one primary key column" (see ColumnFormRow's doc comment) is
 * enforced the moment the user checks a box, not just at submit time.
 */
export function setPrimaryKey(rows: readonly ColumnFormRow[], index: number, isPrimaryKey: boolean): ColumnFormRow[] {
    return rows.map((row, i) => {
        if (i === index) {
            return {...row, isPrimaryKey}
        }
        return isPrimaryKey ? {...row, isPrimaryKey: false} : row
    })
}

/**
 * Validates a "Create table" form's current state (tasks.md 10.2) before
 * CreateTable is called, returning the first problem found or null when the
 * form is submittable: at least one column, a non-blank table name, every
 * column named and unique (case-sensitive, matching
 * BuildCreateTableDDL's own exact-match duplicate check), at most one
 * primary key column, and every auto-increment-typed column (serial/
 * bigserial) marked as that one primary key — mirroring
 * BuildCreateTableDDL's own validation so the user sees the same problem
 * client-side instead of only after a round trip to the backend fails.
 */
export function validateCreateTableForm(tableName: string, rows: readonly ColumnFormRow[]): string | null {
    if (tableName.trim() === '') {
        return 'Table name is required.'
    }
    if (rows.length === 0) {
        return 'At least one column is required.'
    }

    const seenNames = new Set<string>()
    for (const row of rows) {
        if (row.name.trim() === '') {
            return 'Every column needs a name.'
        }
        if (seenNames.has(row.name)) {
            return `Duplicate column name: "${row.name}".`
        }
        seenNames.add(row.name)
    }

    const primaryKeyCount = rows.filter((row) => row.isPrimaryKey).length
    if (primaryKeyCount > 1) {
        return 'Only one primary key column is supported.'
    }

    for (const row of rows) {
        if (isAutoIncrementType(row.type) && !row.isPrimaryKey) {
            return `Column "${row.name}" is auto-increment and must be the primary key.`
        }
    }

    return null
}

/**
 * Converts the form's rows into the payload CreateTable expects. Assumes
 * validateCreateTableForm already returned null for rows — this performs no
 * validation of its own, matching gridEditHelpers.buildInsertPayload's own
 * "helpers build payloads, callers validate first" split. A blank default
 * (after trimming) becomes undefined so the generated JSON carries no
 * Default key at all, which internal/dbengine.ColumnDefinition.Default
 * (a Go `*string`) decodes as nil — "no DEFAULT clause" — rather than a
 * present-but-empty string.
 */
export function buildColumnDefinitions(rows: readonly ColumnFormRow[]): dbengine.ColumnDefinition[] {
    return rows.map(
        (row) =>
            new dbengine.ColumnDefinition({
                Name: row.name.trim(),
                Type: row.type,
                Nullable: row.nullable,
                IsPrimaryKey: row.isPrimaryKey,
                Default: row.defaultValue.trim() === '' ? undefined : row.defaultValue.trim(),
            }),
    )
}
