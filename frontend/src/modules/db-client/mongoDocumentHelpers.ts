export type MongoScalarKind = 'string' | 'number' | 'boolean' | 'null' | 'date' | 'objectid'
export type MongoValueKind = MongoScalarKind | 'object' | 'array'

/**
 * Matches the hex-encoded ObjectID string `convert.go`'s `sanitizeValue`
 * produces for every `primitive.ObjectID` (24 hex characters, no separators).
 * Post-conversion an ObjectID is indistinguishable on the wire from any other
 * 24-character hex string, so this is a display heuristic, not a guarantee —
 * documented here rather than silently assumed.
 */
export const OBJECT_ID_PATTERN = /^[0-9a-f]{24}$/i

/**
 * Matches the RFC3339Nano string `convert.go`'s `sanitizeValue` produces for
 * every `primitive.DateTime`/`time.Time`. Same caveat as `OBJECT_ID_PATTERN`:
 * a plain string that happens to look like a timestamp reads as a date here,
 * since the BSON-to-JSON conversion already discarded the original BSON type.
 */
export const ISO_DATE_PATTERN = /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d{1,9})?(Z|[+-]\d{2}:\d{2})$/

/**
 * Classifies a decoded document value for the tree's type indicator (tasks.md
 * 5.2, spec.md §4.4's "typed scalars"). Every document/array-valued BSON
 * field survives `convert.go`'s sanitization as a plain JS object/array, so
 * those two cases are unambiguous; `null` and JS primitives are equally
 * unambiguous. A JSON string additionally gets classified as `'objectid'` or
 * `'date'` when it matches the corresponding pattern above, purely for
 * display — `mongo.go`'s BSON→JSON conversion is one-way, so this is a best-
 * effort reconstruction of "what this string probably was in Mongo," not a
 * guarantee.
 */
export function classifyMongoValue(value: unknown): MongoValueKind {
    if (value === null || value === undefined) {
        return 'null'
    }
    if (Array.isArray(value)) {
        return 'array'
    }
    if (typeof value === 'object') {
        return 'object'
    }
    if (typeof value === 'boolean') {
        return 'boolean'
    }
    if (typeof value === 'number') {
        return 'number'
    }
    if (typeof value === 'string') {
        if (OBJECT_ID_PATTERN.test(value)) {
            return 'objectid'
        }
        if (ISO_DATE_PATTERN.test(value)) {
            return 'date'
        }
        return 'string'
    }
    return 'string'
}

/** Renders a scalar's display text — never called for `'object'`/`'array'`. */
export function describeScalarValue(value: unknown, kind: MongoValueKind): string {
    if (kind === 'null') {
        return 'null'
    }
    if (kind === 'boolean') {
        return String(value)
    }
    return String(value)
}

/** Collapsed-node label: `"{n keys}"` for an object, `"[n items]"` for an array. */
export function summarizeContainer(value: Record<string, unknown> | unknown[]): string {
    if (Array.isArray(value)) {
        return `[${value.length} item${value.length === 1 ? '' : 's'}]`
    }
    const keyCount = Object.keys(value).length
    return `{${keyCount} key${keyCount === 1 ? '' : 's'}}`
}

export type DocumentJSONValidation =
    | {ok: true; value: Record<string, unknown>}
    | {ok: false; error: string}

/**
 * Validates raw document-editor text before Save is allowed (tasks.md 5.3,
 * spec.md §4.4's "In-place edits validate JSON structure before allowing
 * save"). Two failure modes are distinguished: `JSON.parse` itself throwing
 * (malformed JSON — dangling commas, unmatched braces, ...) surfaces that
 * error's own message; well-formed JSON that isn't an object (an array or a
 * bare scalar) is rejected too, since a MongoDB document is always a single
 * top-level object — `{}` is the minimum valid shape, matching the "starts
 * from an empty {}" requirement (tasks.md 5.4).
 */
export function validateDocumentJSON(raw: string): DocumentJSONValidation {
    let parsed: unknown
    try {
        parsed = JSON.parse(raw)
    } catch (err) {
        return {ok: false, error: err instanceof Error ? err.message : String(err)}
    }
    if (parsed === null || Array.isArray(parsed) || typeof parsed !== 'object') {
        return {ok: false, error: 'A document must be a JSON object, e.g. {"field": "value"} — not an array or a bare value.'}
    }
    return {ok: true, value: parsed as Record<string, unknown>}
}

/**
 * Drops `_id` from `doc` for the "duplicate selected document" creation flow
 * (tasks.md 5.4): the duplicate must get its own freshly generated `_id` on
 * insert, not collide with the source document's, and `InsertDocument`
 * (mongo.go) only generates one when the caller-supplied document has none.
 */
export function stripId(doc: Record<string, unknown>): Record<string, unknown> {
    if (!('_id' in doc)) {
        return doc
    }
    const {_id: _omitted, ...rest} = doc
    return rest
}

/** Pretty-prints `doc` as the duplicate-creation flow's starting text. */
export function formatDocumentForDuplicate(doc: Record<string, unknown>): string {
    return JSON.stringify(stripId(doc), null, 2)
}

/** Pretty-prints `doc` as the in-place edit flow's starting text. */
export function formatDocumentForEdit(doc: Record<string, unknown>): string {
    return JSON.stringify(doc, null, 2)
}

export type MongoFilterInput =
    | {ok: true; filterJSON: string}
    | {ok: false; error: string}

/**
 * Validates and normalizes the collection browser's filter-bar text (tasks.md
 * 5.5) before it becomes `FindMongoDocuments`' `filterJSON` argument. Blank/
 * whitespace-only input maps to `''`, matching `mongo_session.go`'s
 * `decodeMongoJSONObject` convention where an empty filter means "match
 * everything" — sending `''` rather than `'{}'` keeps the frontend and
 * backend's notion of "no filter" as one representation instead of two.
 * Non-blank input reuses `validateDocumentJSON`'s object-shape check, since a
 * MongoDB filter, like a document, must be a single JSON object (e.g.
 * `{"status": "active"}`) — not an array or a bare scalar.
 */
export function parseFilterInput(raw: string): MongoFilterInput {
    const trimmed = raw.trim()
    if (trimmed === '') {
        return {ok: true, filterJSON: ''}
    }
    const validation = validateDocumentJSON(trimmed)
    if (!validation.ok) {
        return {ok: false, error: validation.error}
    }
    return {ok: true, filterJSON: JSON.stringify(validation.value)}
}

/**
 * Toggles `path`'s membership in an expanded-paths set, returning a new Set
 * (never mutating `expanded`) so React state updates detect the change. Used
 * per-document by `MongoDocumentTree` (tasks.md 5.2) to track which nested
 * object/array nodes are currently expanded; `path` is a dotted key/index
 * trail (e.g. `"address.tags.0"`) unique within one document's tree.
 */
export function toggleExpandedPath(expanded: ReadonlySet<string>, path: string): Set<string> {
    const next = new Set(expanded)
    if (next.has(path)) {
        next.delete(path)
    } else {
        next.add(path)
    }
    return next
}
