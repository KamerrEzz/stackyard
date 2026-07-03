export type ViewKey = 'environments' | 'db-client'

interface NavItem {
    key: ViewKey
    label: string
}

const NAV_ITEMS: NavItem[] = [
    {key: 'environments', label: 'Environments'},
    {key: 'db-client', label: 'DB Client'},
]

interface SidebarProps {
    activeView: ViewKey
    onSelectView: (view: ViewKey) => void
}

function Sidebar({activeView, onSelectView}: SidebarProps) {
    return (
        <nav className="flex w-56 shrink-0 flex-col gap-1 border-r border-ink-800 bg-ink-900 px-3 py-4">
            <span className="mb-2 px-3 font-mono text-[11px] uppercase tracking-widest text-ink-400">
                Modules
            </span>
            {NAV_ITEMS.map((item) => {
                const isActive = item.key === activeView
                return (
                    <button
                        key={item.key}
                        type="button"
                        onClick={() => onSelectView(item.key)}
                        className={`rounded-sm border-l-2 px-3 py-2 text-left text-sm font-medium transition-colors ${
                            isActive
                                ? 'border-brass-500 bg-ink-800 text-brass-400'
                                : 'border-transparent text-ink-200 hover:bg-ink-800 hover:text-ink-100'
                        }`}
                    >
                        {item.label}
                    </button>
                )
            })}
        </nav>
    )
}

export default Sidebar
