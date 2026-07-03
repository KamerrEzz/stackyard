export interface SnippetFilterScope {
    connectionId: number
    engine: string
}

export interface ActiveConnectionContext {
    savedConnectionId: number
    engine: string
}

/**
 * Decides the {connectionId, engine} pair SnippetsPanel's ListSnippets
 * filter should use, given whichever DB Client tab is currently active
 * (tasks.md 4.7, spec.md §4.7's "usable from any connection of a compatible
 * engine" scoping). No active tab returns the unscoped {0, ''} pair, which
 * shows every snippet regardless of scope — there is no connection context
 * to narrow by. An active tab always contributes its own engine, whether it
 * is a saved connection (a real, positive savedConnectionId) or an ad-hoc
 * one opened via Test connection/a pasted URL (savedConnectionId 0):
 * storage.ListSnippets' scoping condition (connection_id = ? OR
 * (connection_id IS NULL AND engine = ?)) never matches a real scoped
 * snippet against connectionId 0, so an ad-hoc tab correctly sees only
 * compatible-engine global snippets, never another connection's scoped
 * ones.
 */
export function resolveSnippetFilterScope(activeTab: ActiveConnectionContext | null): SnippetFilterScope {
    if (activeTab === null) {
        return {connectionId: 0, engine: ''}
    }
    return {connectionId: activeTab.savedConnectionId, engine: activeTab.engine}
}
