import {useCallback, useEffect, useState} from 'react'
import {GetConnectionString, StartStatusWatcher, StopStatusWatcher} from '../../../wailsjs/go/main/App'
import {EventsOn} from '../../../wailsjs/runtime/runtime'

const STATUS_EVENT_NAME = 'environment:status'

export interface ServiceStatus {
    ServiceID: number
    ServiceName: string
    Engine: string
    EngineVersion: string
    State: string
    HostPort: number
    CPUPercent: number
    MemoryUsageBytes: number
    MemoryLimitBytes: number
    MemoryPercent: number
    StatsAvailable: boolean
}

export interface ProfileStatus {
    ProfileID: number
    ProfileName: string
    Services: ServiceStatus[]
}

export interface StatusSnapshot {
    Profiles: ProfileStatus[]
}

function engineLabel(engine: string): string {
    switch (engine) {
        case 'postgres':
            return 'PostgreSQL'
        case 'mysql':
            return 'MySQL'
        case 'mongodb':
            return 'MongoDB'
        case 'redis':
            return 'Redis'
        default:
            return engine
    }
}

function stateLabel(state: string): string {
    switch (state) {
        case 'running':
            return 'Running'
        case 'not_found':
            return 'Not found'
        case 'unknown':
            return 'Unknown'
        default:
            return state.charAt(0).toUpperCase() + state.slice(1)
    }
}

function stateColor(state: string): string {
    switch (state) {
        case 'running':
            return 'text-emerald-400'
        case 'exited':
        case 'not_found':
        case 'created':
            return 'text-ink-400'
        case 'paused':
        case 'restarting':
            return 'text-brass-400'
        case 'dead':
        case 'unknown':
            return 'text-red-400'
        default:
            return 'text-ink-400'
    }
}

function formatPercent(value: number, available: boolean): string {
    return available ? `${value.toFixed(1)}%` : '—'
}

function formatBytes(bytes: number): string {
    if (bytes <= 0) {
        return '0 MiB'
    }
    const mib = bytes / (1024 * 1024)
    if (mib >= 1024) {
        return `${(mib / 1024).toFixed(2)} GiB`
    }
    return `${mib.toFixed(1)} MiB`
}

function formatMemory(service: ServiceStatus): string {
    if (!service.StatsAvailable) {
        return '—'
    }
    return `${formatBytes(service.MemoryUsageBytes)} / ${formatBytes(service.MemoryLimitBytes)} (${service.MemoryPercent.toFixed(1)}%)`
}

type ConnectionStringState =
    | {status: 'loading'}
    | {status: 'loaded'; value: string}
    | {status: 'error'; message: string}

/**
 * Live dashboard of every managed profile/service across the whole app,
 * pushed by the Go backend over the `environment:status` Wails event
 * (started/stopped via StartStatusWatcher/StopStatusWatcher) instead of
 * frontend polling. Clicking a running service reveals its connection
 * string inline, in place, within that row.
 */
function StatusDashboard() {
    const [snapshot, setSnapshot] = useState<StatusSnapshot | null>(null)
    const [watcherError, setWatcherError] = useState<string | null>(null)
    const [expandedServiceId, setExpandedServiceId] = useState<number | null>(null)
    const [connectionStrings, setConnectionStrings] = useState<Record<number, ConnectionStringState>>({})

    useEffect(() => {
        const unsubscribe = EventsOn(STATUS_EVENT_NAME, (data: StatusSnapshot) => {
            setSnapshot(data)
        })

        StartStatusWatcher().catch((err) => setWatcherError(String(err)))

        return () => {
            unsubscribe()
            void StopStatusWatcher()
        }
    }, [])

    const toggleConnectionString = useCallback(
        (serviceId: number) => {
            setExpandedServiceId((current) => (current === serviceId ? null : serviceId))
            setConnectionStrings((prev) => {
                if (prev[serviceId]?.status === 'loaded') {
                    return prev
                }
                GetConnectionString(serviceId)
                    .then((value) => {
                        setConnectionStrings((latest) => ({...latest, [serviceId]: {status: 'loaded', value}}))
                    })
                    .catch((err) => {
                        setConnectionStrings((latest) => ({
                            ...latest,
                            [serviceId]: {status: 'error', message: String(err)},
                        }))
                    })
                return {...prev, [serviceId]: {status: 'loading'}}
            })
        },
        [],
    )

    const profiles = snapshot?.Profiles ?? []

    return (
        <div className="flex flex-col gap-6">
            <div>
                <h1 className="text-xl font-semibold text-ink-100">Status Dashboard</h1>
                <p className="text-sm text-ink-400">
                    Live state, port, CPU, and RAM for every managed service — pushed automatically, no manual
                    refresh needed. Click a running service to reveal its connection string.
                </p>
            </div>

            {watcherError && <p className="text-sm text-red-400">{watcherError}</p>}

            {!snapshot && !watcherError && <p className="text-sm text-ink-400">Waiting for status…</p>}

            {snapshot && profiles.length === 0 && (
                <p className="text-sm text-ink-400">No profiles yet — create one from Environments.</p>
            )}

            <div className="flex flex-col gap-4">
                {profiles.map((profile) => (
                    <div
                        key={profile.ProfileID}
                        className="flex flex-col gap-2 rounded border border-ink-800 bg-ink-900/40 p-4"
                    >
                        <h2 className="text-sm font-semibold text-ink-100">{profile.ProfileName}</h2>
                        <table className="w-full text-left text-sm">
                            <thead>
                                <tr className="text-xs uppercase tracking-widest text-ink-400">
                                    <th className="py-1 pr-3 font-medium">Service</th>
                                    <th className="py-1 pr-3 font-medium">Engine</th>
                                    <th className="py-1 pr-3 font-medium">State</th>
                                    <th className="py-1 pr-3 font-medium">Port</th>
                                    <th className="py-1 pr-3 font-medium">CPU</th>
                                    <th className="py-1 pr-3 font-medium">RAM</th>
                                </tr>
                            </thead>
                            <tbody>
                                {profile.Services.map((service) => (
                                    <ServiceRow
                                        key={service.ServiceID}
                                        service={service}
                                        expanded={expandedServiceId === service.ServiceID}
                                        connectionString={connectionStrings[service.ServiceID]}
                                        onToggle={() => toggleConnectionString(service.ServiceID)}
                                    />
                                ))}
                            </tbody>
                        </table>
                    </div>
                ))}
            </div>
        </div>
    )
}

interface ServiceRowProps {
    service: ServiceStatus
    expanded: boolean
    connectionString: ConnectionStringState | undefined
    onToggle: () => void
}

function ServiceRow({service, expanded, connectionString, onToggle}: ServiceRowProps) {
    const clickable = service.State === 'running'

    return (
        <>
            <tr
                onClick={clickable ? onToggle : undefined}
                className={`border-t border-ink-800 ${clickable ? 'cursor-pointer hover:bg-ink-800/50' : ''}`}
            >
                <td className="py-2 pr-3 font-mono text-xs text-ink-200">{service.ServiceName}</td>
                <td className="py-2 pr-3 text-ink-300" title={service.EngineVersion}>
                    {engineLabel(service.Engine)}
                </td>
                <td className={`py-2 pr-3 font-medium ${stateColor(service.State)}`}>{stateLabel(service.State)}</td>
                <td className="py-2 pr-3 font-mono text-ink-300">{service.HostPort}</td>
                <td className="py-2 pr-3 font-mono text-ink-300">
                    {formatPercent(service.CPUPercent, service.StatsAvailable)}
                </td>
                <td className="py-2 pr-3 font-mono text-ink-300">{formatMemory(service)}</td>
            </tr>
            {expanded && (
                <tr className="border-t border-ink-800/60 bg-ink-950/40">
                    <td colSpan={6} className="px-3 py-2">
                        <ConnectionStringReveal state={connectionString} />
                    </td>
                </tr>
            )}
        </>
    )
}

interface ConnectionStringRevealProps {
    state: ConnectionStringState | undefined
}

function ConnectionStringReveal({state}: ConnectionStringRevealProps) {
    if (!state || state.status === 'loading') {
        return <p className="text-xs text-ink-400">Loading connection string…</p>
    }
    if (state.status === 'error') {
        return <p className="text-xs text-red-400">{state.message}</p>
    }
    return <p className="break-all font-mono text-xs text-brass-400">{state.value}</p>
}

export default StatusDashboard
