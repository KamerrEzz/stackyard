import PingCheck from './PingCheck'

interface TopBarProps {
    subtitle: string
}

function TopBar({subtitle}: TopBarProps) {
    return (
        <header className="flex h-12 shrink-0 items-center justify-between border-b border-ink-800 bg-ink-900/60 px-4">
            <span className="font-mono text-sm uppercase tracking-widest text-ink-100">
                Stackyard
            </span>
            <div className="flex items-center gap-3">
                <span className="text-xs text-ink-400">{subtitle}</span>
                <PingCheck/>
            </div>
        </header>
    )
}

export default TopBar
