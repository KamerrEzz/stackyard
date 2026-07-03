import mermaid from 'mermaid'
import {useCallback, useEffect, useRef, useState} from 'react'
import {clampZoom, exportFileName, mermaidSafeId, wheelDeltaToZoom, zoomStep} from './schemaDiagramHelpers'

/**
 * Minimum font size (in px) Mermaid renders entity/attribute text at
 * (tasks.md 4.5.5). Mermaid's own erDiagram default is 12px, which reads as
 * genuinely small print once the app window is itself scaled up for a
 * classroom projector; 16px was chosen as the floor because it matches the
 * commonly cited minimum body-text size for comfortable at-a-distance
 * reading, and was checked (not just assumed) by rendering a multi-table
 * diagram and inspecting it at 2x browser zoom — simulating a projector's
 * further physical magnification — where the 12px default started to blur
 * and the 16px value stayed crisp and legible.
 */
export const MIN_LEGIBLE_FONT_SIZE = 16

mermaid.initialize({
    startOnLoad: false,
    theme: 'dark',
    securityLevel: 'strict',
    er: {fontSize: MIN_LEGIBLE_FONT_SIZE},
})

interface MermaidDiagramProps {
    source: string
    schemaName: string
    badge?: string
}

type RenderState = 'idle' | 'rendering' | 'ready' | 'error'
type CopyState = 'idle' | 'copied' | 'error'

function downloadBlob(blob: Blob, filename: string): void {
    const url = URL.createObjectURL(blob)
    const anchor = document.createElement('a')
    anchor.href = url
    anchor.download = filename
    anchor.click()
    URL.revokeObjectURL(url)
}

/**
 * Renders Mermaid diagram text (an `erDiagram` for the relational schema
 * diagram, tasks.md 4.5.2 — or, later, MongoDB's inferred-structure diagram,
 * tasks.md 5.6, which is why `badge` exists as an optional prop rather than
 * being relational-diagram-specific) inside a zoom/pan viewport, with PNG/
 * SVG export and "copy Mermaid text" actions (tasks.md 4.5.3/4.5.4).
 *
 * Zoom/pan is a lightweight, dependency-free CSS transform: `scale` and
 * `pan` state drive a single `transform` on the wrapper around the injected
 * SVG, updated by a wheel handler (zoom) and mousedown/mousemove/mouseup
 * drag handlers (pan) — no pan/zoom library was added for this, matching
 * plan.md's note that `mermaid` is this feature's only new dependency.
 */
function MermaidDiagram({source, schemaName, badge}: MermaidDiagramProps) {
    const [renderState, setRenderState] = useState<RenderState>('idle')
    const [renderError, setRenderError] = useState<string | null>(null)
    const [svgMarkup, setSvgMarkup] = useState<string>('')
    const [copyState, setCopyState] = useState<CopyState>('idle')

    const [scale, setScale] = useState(1)
    const [pan, setPan] = useState({x: 0, y: 0})
    const dragRef = useRef<{startX: number; startY: number; originX: number; originY: number} | null>(null)

    const viewportRef = useRef<HTMLDivElement | null>(null)
    const copyTimeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null)

    useEffect(() => {
        return () => {
            if (copyTimeoutRef.current) {
                clearTimeout(copyTimeoutRef.current)
            }
        }
    }, [])

    useEffect(() => {
        let cancelled = false

        setRenderState('rendering')
        setRenderError(null)
        setScale(1)
        setPan({x: 0, y: 0})

        mermaid
            .render(mermaidSafeId(schemaName), source)
            .then(({svg}) => {
                if (cancelled) {
                    return
                }
                setSvgMarkup(svg)
                setRenderState('ready')
            })
            .catch((err: unknown) => {
                if (cancelled) {
                    return
                }
                setRenderError(String(err))
                setRenderState('error')
            })

        return () => {
            cancelled = true
        }
    }, [source, schemaName])

    const handleWheel = useCallback((event: React.WheelEvent<HTMLDivElement>) => {
        event.preventDefault()
        setScale((current) => wheelDeltaToZoom(current, event.deltaY))
    }, [])

    const handleMouseDown = useCallback(
        (event: React.MouseEvent<HTMLDivElement>) => {
            dragRef.current = {startX: event.clientX, startY: event.clientY, originX: pan.x, originY: pan.y}
        },
        [pan.x, pan.y],
    )

    const handleMouseMove = useCallback((event: React.MouseEvent<HTMLDivElement>) => {
        const drag = dragRef.current
        if (!drag) {
            return
        }
        setPan({x: drag.originX + (event.clientX - drag.startX), y: drag.originY + (event.clientY - drag.startY)})
    }, [])

    const handleMouseUp = useCallback(() => {
        dragRef.current = null
    }, [])

    const handleZoomIn = useCallback(() => setScale((current) => zoomStep(current, 'in')), [])
    const handleZoomOut = useCallback(() => setScale((current) => zoomStep(current, 'out')), [])
    const handleResetView = useCallback(() => {
        setScale(1)
        setPan({x: 0, y: 0})
    }, [])

    const handleCopyMermaidText = useCallback(async () => {
        try {
            await navigator.clipboard.writeText(source)
            setCopyState('copied')
        } catch {
            setCopyState('error')
        } finally {
            if (copyTimeoutRef.current) {
                clearTimeout(copyTimeoutRef.current)
            }
            copyTimeoutRef.current = setTimeout(() => setCopyState('idle'), 2000)
        }
    }, [source])

    const handleExportSVG = useCallback(() => {
        const svgElement = viewportRef.current?.querySelector('svg')
        if (!svgElement) {
            return
        }
        const serialized = new XMLSerializer().serializeToString(svgElement)
        const blob = new Blob([serialized], {type: 'image/svg+xml'})
        downloadBlob(blob, exportFileName(schemaName, 'svg', new Date()))
    }, [schemaName])

    const handleExportPNG = useCallback(() => {
        const svgElement = viewportRef.current?.querySelector('svg')
        if (!svgElement) {
            return
        }

        const serialized = new XMLSerializer().serializeToString(svgElement)
        const svgBlob = new Blob([serialized], {type: 'image/svg+xml'})
        const svgUrl = URL.createObjectURL(svgBlob)

        const exportScale = 2
        const width = svgElement.viewBox.baseVal.width || svgElement.clientWidth || 800
        const height = svgElement.viewBox.baseVal.height || svgElement.clientHeight || 600

        const image = new Image()
        image.onload = () => {
            const canvas = document.createElement('canvas')
            canvas.width = width * exportScale
            canvas.height = height * exportScale
            const context = canvas.getContext('2d')
            if (context) {
                context.fillStyle = '#0a0e17'
                context.fillRect(0, 0, canvas.width, canvas.height)
                context.drawImage(image, 0, 0, canvas.width, canvas.height)
            }
            URL.revokeObjectURL(svgUrl)
            canvas.toBlob((blob) => {
                if (blob) {
                    downloadBlob(blob, exportFileName(schemaName, 'png', new Date()))
                }
            }, 'image/png')
        }
        image.src = svgUrl
    }, [schemaName])

    return (
        <div className="flex flex-col gap-3">
            <div className="flex flex-wrap items-center gap-2">
                <button
                    type="button"
                    onClick={handleZoomOut}
                    className="rounded border border-ink-700 px-3 py-1 text-sm text-ink-200 hover:border-brass-500 hover:text-brass-400"
                >
                    Zoom out
                </button>
                <button
                    type="button"
                    onClick={handleZoomIn}
                    className="rounded border border-ink-700 px-3 py-1 text-sm text-ink-200 hover:border-brass-500 hover:text-brass-400"
                >
                    Zoom in
                </button>
                <button
                    type="button"
                    onClick={handleResetView}
                    className="rounded border border-ink-700 px-3 py-1 text-sm text-ink-200 hover:border-brass-500 hover:text-brass-400"
                >
                    Reset view
                </button>
                <span className="text-xs text-ink-500">{Math.round(scale * 100)}%</span>

                <div className="ml-auto flex flex-wrap items-center gap-2">
                    <button
                        type="button"
                        onClick={() => void handleCopyMermaidText()}
                        className={`rounded border px-3 py-1 text-sm transition-colors ${
                            copyState === 'copied'
                                ? 'border-emerald-600 text-emerald-400'
                                : copyState === 'error'
                                  ? 'border-red-600 text-red-400'
                                  : 'border-ink-700 text-ink-200 hover:border-brass-500 hover:text-brass-400'
                        }`}
                    >
                        {copyState === 'copied' ? 'Copied!' : copyState === 'error' ? 'Copy failed' : 'Copy Mermaid text'}
                    </button>
                    <button
                        type="button"
                        onClick={handleExportSVG}
                        disabled={renderState !== 'ready'}
                        className="rounded border border-ink-700 px-3 py-1 text-sm text-ink-200 hover:border-brass-500 hover:text-brass-400 disabled:cursor-not-allowed disabled:opacity-40"
                    >
                        Export SVG
                    </button>
                    <button
                        type="button"
                        onClick={handleExportPNG}
                        disabled={renderState !== 'ready'}
                        className="rounded border border-ink-700 px-3 py-1 text-sm text-ink-200 hover:border-brass-500 hover:text-brass-400 disabled:cursor-not-allowed disabled:opacity-40"
                    >
                        Export PNG
                    </button>
                </div>
            </div>

            {badge && (
                <p className="w-fit rounded border border-brass-500 bg-brass-500/10 px-3 py-1 text-xs font-medium uppercase tracking-widest text-brass-400">
                    {badge}
                </p>
            )}

            {renderState === 'error' && renderError && (
                <p className="text-sm text-red-400">Failed to render diagram: {renderError}</p>
            )}

            <div
                ref={viewportRef}
                onWheel={handleWheel}
                onMouseDown={handleMouseDown}
                onMouseMove={handleMouseMove}
                onMouseUp={handleMouseUp}
                onMouseLeave={handleMouseUp}
                className="h-[70vh] cursor-grab overflow-hidden rounded border border-ink-800 bg-ink-950/60 active:cursor-grabbing"
            >
                <div
                    style={{
                        transform: `translate(${pan.x}px, ${pan.y}px) scale(${scale})`,
                        transformOrigin: '0 0',
                    }}
                    dangerouslySetInnerHTML={{__html: svgMarkup}}
                />
            </div>
        </div>
    )
}

export default MermaidDiagram
