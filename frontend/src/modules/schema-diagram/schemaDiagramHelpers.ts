export const MIN_ZOOM = 0.25
export const MAX_ZOOM = 4
export const DEFAULT_ZOOM_STEP = 0.1

/**
 * Clamps `scale` to the [min, max] range the schema diagram's zoom controls
 * and wheel handler both funnel through, so neither can push the diagram
 * past a size that's either illegibly small or too large to usefully pan
 * around (tasks.md 4.5.3/4.5.5).
 */
export function clampZoom(scale: number, min: number = MIN_ZOOM, max: number = MAX_ZOOM): number {
    return Math.min(max, Math.max(min, scale))
}

export type ZoomDirection = 'in' | 'out'

/**
 * Returns the next zoom level after one "Zoom in"/"Zoom out" button click,
 * stepping by `step` and clamping to [min, max].
 */
export function zoomStep(
    current: number,
    direction: ZoomDirection,
    step: number = DEFAULT_ZOOM_STEP,
    min: number = MIN_ZOOM,
    max: number = MAX_ZOOM,
): number {
    const next = direction === 'in' ? current + step : current - step
    return clampZoom(next, min, max)
}

/**
 * Converts a wheel event's `deltaY` into a new zoom level: scrolling up
 * (negative deltaY) zooms in, scrolling down zooms out. The divisor keeps a
 * single mouse-wheel "tick" (commonly ±100 deltaY) close to one
 * `DEFAULT_ZOOM_STEP`, so wheel-zooming and button-zooming feel comparable
 * rather than the wheel jumping in much larger increments.
 */
export function wheelDeltaToZoom(current: number, deltaY: number, min: number = MIN_ZOOM, max: number = MAX_ZOOM): number {
    const next = current - deltaY / 1000
    return clampZoom(next, min, max)
}

/**
 * Turns `seed` into a string safe to use as a Mermaid `render()` call's DOM
 * element ID: Mermaid injects this ID directly into the DOM, which requires
 * a valid CSS identifier, but a schema name (the most natural seed for this
 * call) may contain characters — spaces, dots, non-ASCII letters — that
 * aren't valid there. Every disallowed character collapses to a single
 * hyphen, and the result is prefixed so it can never start with a digit
 * (also invalid for a CSS identifier).
 */
export function mermaidSafeId(seed: string): string {
    const collapsed = seed.replace(/[^A-Za-z0-9_-]+/g, '-').replace(/^-+|-+$/g, '')
    return `schema-diagram-${collapsed.length > 0 ? collapsed : 'default'}`
}

/**
 * Builds a deterministic export filename for the diagram's PNG/SVG/copy
 * exports (tasks.md 4.5.4), e.g. `schema-diagram-public-20260702-153000.svg`.
 * `now` is an explicit parameter (rather than `new Date()` called inside)
 * so this function stays pure and testable.
 */
export function exportFileName(schema: string, extension: string, now: Date): string {
    const stamp = [
        now.getFullYear(),
        String(now.getMonth() + 1).padStart(2, '0'),
        String(now.getDate()).padStart(2, '0'),
    ].join('') + '-' + [
        String(now.getHours()).padStart(2, '0'),
        String(now.getMinutes()).padStart(2, '0'),
        String(now.getSeconds()).padStart(2, '0'),
    ].join('')

    const safeSchema = schema.replace(/[^A-Za-z0-9_-]+/g, '-').replace(/^-+|-+$/g, '') || 'schema'
    return `schema-diagram-${safeSchema}-${stamp}.${extension}`
}
