import type {storage} from '../../../wailsjs/go/models'

export type RunSnippetTarget = {kind: 'current-tab'; tabId: string} | {kind: 'new-tab'}

/**
 * Decides whether running a snippet should load its body into the
 * currently active tab or open a brand-new tab instead (tasks.md 4.7,
 * spec.md §4.7's third bullet: "loads it into the current tab's editor
 * without losing unsaved work in that tab"). The current tab is reused
 * only when one is actually active AND its editor is not dirty; a dirty
 * active tab, or no active tab at all, always opens a new one so nothing
 * already typed there is ever overwritten.
 */
export function resolveRunSnippetTarget(activeTabId: string | null, isActiveTabDirty: boolean): RunSnippetTarget {
    if (activeTabId !== null && !isActiveTabDirty) {
        return {kind: 'current-tab', tabId: activeTabId}
    }
    return {kind: 'new-tab'}
}

export type SnippetConnectionSource =
    | {kind: 'scoped'; connectionId: number}
    | {kind: 'reuse-active-tab'}
    | {kind: 'most-recent-compatible'; connectionId: number}
    | {kind: 'none'}

/**
 * Resolves which connection a brand-new tab opened for a snippet run
 * should use (tasks.md 4.7). Only consulted on the new-tab path — loading
 * a snippet into the already-active tab (resolveRunSnippetTarget's
 * 'current-tab' case) never changes that tab's connection, only its query
 * text. A connection-scoped snippet always dictates its own connection. A
 * global snippet prefers the currently active tab's connection (even a
 * dirty one — that dirtiness is exactly why a new tab is being opened) as
 * the most contextually relevant fallback; with no active tab at all, it
 * falls back to the most recently used saved connection of a compatible
 * engine; if neither exists, there is no reasonable connection to open the
 * new tab against.
 */
export function resolveSnippetConnectionSource(
    snippetConnectionId: number | null,
    hasActiveTab: boolean,
    mostRecentCompatibleConnectionId: number | null,
): SnippetConnectionSource {
    if (snippetConnectionId !== null) {
        return {kind: 'scoped', connectionId: snippetConnectionId}
    }
    if (hasActiveTab) {
        return {kind: 'reuse-active-tab'}
    }
    if (mostRecentCompatibleConnectionId !== null) {
        return {kind: 'most-recent-compatible', connectionId: mostRecentCompatibleConnectionId}
    }
    return {kind: 'none'}
}

/**
 * Finds the saved connection of the given engine most recently used
 * (storage.Connection.LastUsedAt — an RFC3339 string, directly sortable as
 * text per internal/storage's own convention), for
 * resolveSnippetConnectionSource's no-active-tab fallback. Returns null if
 * no saved connection matches the engine at all; a connection that has
 * never been used (empty LastUsedAt) sorts as older than any that has.
 */
export function findMostRecentCompatibleConnection(
    connections: readonly storage.Connection[],
    engine: string,
): storage.Connection | null {
    let best: storage.Connection | null = null
    for (const conn of connections) {
        if (conn.Engine !== engine) {
            continue
        }
        if (best === null || (conn.LastUsedAt ?? '') > (best.LastUsedAt ?? '')) {
            best = conn
        }
    }
    return best
}
