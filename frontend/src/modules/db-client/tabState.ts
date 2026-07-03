export interface IdentifiedTab {
    id: string
}

export interface TabListState<T extends IdentifiedTab> {
    tabs: T[]
    activeTabId: string | null
}

/**
 * Appends `newTab` to `tabs` and makes it the active tab — the transition
 * every successful "Test connection" or "Load" from a saved connection goes
 * through (tasks.md 3.8): each one always opens an additional tab, it never
 * replaces an existing one.
 */
export function openTab<T extends IdentifiedTab>(tabs: readonly T[], newTab: T): TabListState<T> {
    return {tabs: [...tabs, newTab], activeTabId: newTab.id}
}

/**
 * Removes the tab identified by `idToClose` from `tabs`. Closing a tab that
 * isn't the active one leaves `activeTabId` completely untouched (spec.md
 * §4.2's independence requirement). Closing the active tab selects the tab
 * that visually took its place — the one now at the same index, or the new
 * last tab if the closed tab was the rightmost — falling back to `null`
 * (no active tab) once the last tab closes. `idToClose` not being present in
 * `tabs` is a no-op, returned unchanged.
 */
export function closeTab<T extends IdentifiedTab>(
    tabs: readonly T[],
    activeTabId: string | null,
    idToClose: string,
): TabListState<T> {
    const closingIndex = tabs.findIndex((tab) => tab.id === idToClose)
    if (closingIndex === -1) {
        return {tabs: [...tabs], activeTabId}
    }

    const remaining = tabs.filter((tab) => tab.id !== idToClose)

    if (activeTabId !== idToClose) {
        return {tabs: remaining, activeTabId}
    }
    if (remaining.length === 0) {
        return {tabs: remaining, activeTabId: null}
    }

    const nextIndex = Math.min(closingIndex, remaining.length - 1)
    return {tabs: remaining, activeTabId: remaining[nextIndex].id}
}
