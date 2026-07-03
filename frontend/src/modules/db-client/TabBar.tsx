interface TabBarTab {
    id: string
    label: string
}

interface TabBarProps {
    tabs: TabBarTab[]
    activeTabId: string | null
    onSelect: (id: string) => void
    onClose: (id: string) => void
    onNewTab: () => void
}

/**
 * Tab strip for the DB Client's multi-tab shell (tasks.md 3.8): one entry
 * per open connection/editor/results-pane tab, plus a trailing "+" that
 * clears the active selection so the always-visible connection form above
 * can be used to open another tab. Selecting or closing a tab never mounts
 * or unmounts anything itself — the caller keeps every open tab's
 * `QueryEditor` mounted and toggles visibility, this component only reports
 * clicks.
 */
function TabBar({tabs, activeTabId, onSelect, onClose, onNewTab}: TabBarProps) {
    return (
        <div className="flex items-center gap-1 overflow-x-auto border-b border-ink-800">
            {tabs.map((tab) => {
                const isActive = tab.id === activeTabId
                return (
                    <div
                        key={tab.id}
                        role="tab"
                        aria-selected={isActive}
                        onClick={() => onSelect(tab.id)}
                        className={`group flex max-w-[220px] shrink-0 cursor-pointer items-center gap-2 rounded-t border border-b-0 px-3 py-2 text-xs transition-colors ${
                            isActive
                                ? 'border-ink-700 bg-ink-900 text-brass-400'
                                : 'border-transparent text-ink-400 hover:bg-ink-900/60 hover:text-ink-200'
                        }`}
                    >
                        <span className="truncate font-mono">{tab.label}</span>
                        <button
                            type="button"
                            onClick={(e) => {
                                e.stopPropagation()
                                onClose(tab.id)
                            }}
                            aria-label={`Close tab ${tab.label}`}
                            className="rounded px-1 text-ink-500 hover:bg-ink-800 hover:text-red-400"
                        >
                            ×
                        </button>
                    </div>
                )
            })}
            <button
                type="button"
                onClick={onNewTab}
                aria-label="Open a new connection tab"
                className="shrink-0 rounded px-2.5 py-2 text-sm text-ink-500 hover:bg-ink-900/60 hover:text-brass-400"
            >
                +
            </button>
        </div>
    )
}

export default TabBar
