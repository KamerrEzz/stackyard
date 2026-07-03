import {describe, expect, it} from 'vitest'
import {clampZoom, exportFileName, mermaidSafeId, wheelDeltaToZoom, zoomStep} from './schemaDiagramHelpers'

describe('clampZoom', () => {
    it('leaves an in-range value untouched', () => {
        expect(clampZoom(1)).toBe(1)
    })

    it('clamps below the minimum', () => {
        expect(clampZoom(0.01)).toBe(0.25)
    })

    it('clamps above the maximum', () => {
        expect(clampZoom(10)).toBe(4)
    })

    it('honors custom min/max bounds', () => {
        expect(clampZoom(5, 1, 2)).toBe(2)
        expect(clampZoom(0, 1, 2)).toBe(1)
    })
})

describe('zoomStep', () => {
    it('steps in by the default step', () => {
        expect(zoomStep(1, 'in')).toBeCloseTo(1.1)
    })

    it('steps out by the default step', () => {
        expect(zoomStep(1, 'out')).toBeCloseTo(0.9)
    })

    it('clamps at the minimum when stepping out from the floor', () => {
        expect(zoomStep(0.25, 'out')).toBe(0.25)
    })

    it('clamps at the maximum when stepping in from the ceiling', () => {
        expect(zoomStep(4, 'in')).toBe(4)
    })
})

describe('wheelDeltaToZoom', () => {
    it('zooms in on a negative deltaY (scroll up)', () => {
        expect(wheelDeltaToZoom(1, -100)).toBeCloseTo(1.1)
    })

    it('zooms out on a positive deltaY (scroll down)', () => {
        expect(wheelDeltaToZoom(1, 100)).toBeCloseTo(0.9)
    })

    it('clamps the result', () => {
        expect(wheelDeltaToZoom(4, -1000)).toBe(4)
        expect(wheelDeltaToZoom(0.25, 1000)).toBe(0.25)
    })
})

describe('mermaidSafeId', () => {
    it('passes an already-safe name through unchanged apart from the prefix', () => {
        expect(mermaidSafeId('public')).toBe('schema-diagram-public')
    })

    it('collapses spaces and punctuation into a single hyphen', () => {
        expect(mermaidSafeId('My Schema!!')).toBe('schema-diagram-My-Schema')
    })

    it('falls back to "default" when nothing safe remains', () => {
        expect(mermaidSafeId('***')).toBe('schema-diagram-default')
    })

    it('falls back to "default" for an empty string', () => {
        expect(mermaidSafeId('')).toBe('schema-diagram-default')
    })
})

describe('exportFileName', () => {
    it('formats schema and timestamp into a deterministic filename', () => {
        const now = new Date(2026, 6, 2, 15, 30, 5)
        expect(exportFileName('public', 'svg', now)).toBe('schema-diagram-public-20260702-153005.svg')
    })

    it('sanitizes an unsafe schema name', () => {
        const now = new Date(2026, 0, 1, 0, 0, 0)
        expect(exportFileName('my schema!', 'png', now)).toBe('schema-diagram-my-schema-20260101-000000.png')
    })

    it('falls back to "schema" when the schema name sanitizes to nothing', () => {
        const now = new Date(2026, 0, 1, 0, 0, 0)
        expect(exportFileName('***', 'png', now)).toBe('schema-diagram-schema-20260101-000000.png')
    })
})
