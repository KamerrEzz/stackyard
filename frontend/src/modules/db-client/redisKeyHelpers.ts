export const NO_EXPIRY_LABEL = 'No expiry'

/**
 * Formats a `GetRedisTTL` result for display (tasks.md 6.3, spec.md §4.5's
 * "TTL is visible per key"). `ttlNanoseconds` is the raw value Wails hands
 * back for a Go `time.Duration` (see `redis_session.go`'s `SetRedisTTL` doc
 * comment: the generated TS binding represents `time.Duration` as its
 * underlying int64 nanosecond count, not a richer object). Any negative value
 * is treated as "no expiry" — `redis.Engine.TTL`'s own contract only ever
 * produces `-1` for that case, but this reads `< 0` rather than `=== -1`
 * specifically so a future sentinel change on the Go side degrades to the
 * same safe label instead of a raw negative number leaking into the UI.
 */
export function formatTTL(ttlNanoseconds: number): string {
    if (ttlNanoseconds < 0) {
        return NO_EXPIRY_LABEL
    }
    const totalSeconds = Math.max(0, Math.round(ttlNanoseconds / 1_000_000_000))
    const hours = Math.floor(totalSeconds / 3600)
    const minutes = Math.floor((totalSeconds % 3600) / 60)
    const seconds = totalSeconds % 60

    const parts: string[] = []
    if (hours > 0) {
        parts.push(`${hours}h`)
    }
    if (hours > 0 || minutes > 0) {
        parts.push(`${minutes}m`)
    }
    parts.push(`${seconds}s`)
    return parts.join(' ')
}

export interface CursorPageState {
    items: string[]
    cursor: number
    hasScanned: boolean
}

export const INITIAL_CURSOR_PAGE_STATE: CursorPageState = {items: [], cursor: 0, hasScanned: false}

/**
 * Folds one `ScanRedisKeys`/`GetRedisSet` page into `state` — shared cursor
 * shape between the key browser's pattern scan and the set-value viewer's
 * member pagination (tasks.md 6.2/6.3), since both are SCAN-family calls with
 * an identical "0 cursor means done" contract (see `redis.go`'s `ScanKeys`/
 * `GetSet` doc comments). `mode: 'replace'` starts a fresh scan (a new
 * pattern, or the first page); `'append'` is "Load more," keeping every
 * already-fetched item.
 */
export function applyCursorPage(
    state: CursorPageState,
    page: {items: string[]; nextCursor: number},
    mode: 'replace' | 'append',
): CursorPageState {
    return {
        items: mode === 'replace' ? page.items : [...state.items, ...page.items],
        cursor: page.nextCursor,
        hasScanned: true,
    }
}

/**
 * True once a page has been fetched and the last-returned cursor is nonzero
 * — i.e. there is a "Load more" page still to fetch. Before the first scan
 * (`hasScanned` false) this is false even though `cursor` also reads `0`,
 * disambiguating "haven't scanned yet" from "scan complete," both of which
 * share the same `cursor === 0` value.
 */
export function canLoadMore(state: CursorPageState): boolean {
    return state.hasScanned && state.cursor !== 0
}

export type HashJSONValidation = {ok: true; value: Record<string, string>} | {ok: false; error: string}

/**
 * Validates the hash editor's bulk JSON text (tasks.md 6.2) before `Save`
 * calls `SetRedisHash`. Mirrors `mongoDocumentHelpers.validateDocumentJSON`'s
 * two failure modes (malformed JSON vs. wrong shape), plus one Redis-specific
 * check: every field's value must itself be a JSON string, since a Redis hash
 * field is always a string — a JSON number/bool/object/array/null value has
 * no lossless representation as a hash field and is rejected with the
 * offending field's name rather than silently coerced.
 */
export function validateHashJSON(raw: string): HashJSONValidation {
    let parsed: unknown
    try {
        parsed = JSON.parse(raw)
    } catch (err) {
        return {ok: false, error: err instanceof Error ? err.message : String(err)}
    }
    if (parsed === null || Array.isArray(parsed) || typeof parsed !== 'object') {
        return {ok: false, error: 'A hash must be a JSON object of field/value pairs, e.g. {"field": "value"} — not an array or a bare value.'}
    }
    const value: Record<string, string> = {}
    for (const [field, fieldValue] of Object.entries(parsed as Record<string, unknown>)) {
        if (typeof fieldValue !== 'string') {
            return {ok: false, error: `Field "${field}" must be a JSON string — Redis hash values are always strings.`}
        }
        value[field] = fieldValue
    }
    return {ok: true, value}
}

/**
 * Splits the list-append/set-add textarea's raw text into individual values,
 * one per line, dropping blank lines so pressing Enter on an empty textarea
 * doesn't push/add an empty-string element by accident. Leading/trailing
 * whitespace on a non-blank line is preserved — a Redis value legitimately
 * may need it.
 */
export function parseLineValues(raw: string): string[] {
    return raw.split('\n').filter((line) => line.trim().length > 0)
}

export type ScoreInputValidation = {ok: true; score: number} | {ok: false; error: string}

/** Parses the sorted-set editor's score input, rejecting non-finite input. */
export function parseScoreInput(raw: string): ScoreInputValidation {
    const score = Number(raw)
    if (raw.trim() === '' || !Number.isFinite(score)) {
        return {ok: false, error: 'Score must be a number.'}
    }
    return {ok: true, score}
}
