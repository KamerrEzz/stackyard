import {describe, expect, it} from 'vitest'
import {closeTab, openTab} from './tabState'

interface Tab {
    id: string
    label: string
}

function tab(id: string): Tab {
    return {id, label: id}
}

describe('openTab', () => {
    it('appends the new tab and activates it when the list is empty', () => {
        const result = openTab([], tab('a'))
        expect(result.tabs).toEqual([tab('a')])
        expect(result.activeTabId).toBe('a')
    })

    it('appends after existing tabs without disturbing them', () => {
        const result = openTab([tab('a'), tab('b')], tab('c'))
        expect(result.tabs).toEqual([tab('a'), tab('b'), tab('c')])
        expect(result.activeTabId).toBe('c')
    })
})

describe('closeTab', () => {
    it('closing a non-active tab leaves activeTabId untouched', () => {
        const tabs = [tab('a'), tab('b'), tab('c')]
        const result = closeTab(tabs, 'a', 'c')
        expect(result.tabs).toEqual([tab('a'), tab('b')])
        expect(result.activeTabId).toBe('a')
    })

    it('closing the active middle tab selects the tab that took its place', () => {
        const tabs = [tab('a'), tab('b'), tab('c')]
        const result = closeTab(tabs, 'b', 'b')
        expect(result.tabs).toEqual([tab('a'), tab('c')])
        expect(result.activeTabId).toBe('c')
    })

    it('closing the active rightmost tab falls back to the new last tab', () => {
        const tabs = [tab('a'), tab('b'), tab('c')]
        const result = closeTab(tabs, 'c', 'c')
        expect(result.tabs).toEqual([tab('a'), tab('b')])
        expect(result.activeTabId).toBe('b')
    })

    it('closing the only remaining active tab leaves no active tab', () => {
        const result = closeTab([tab('a')], 'a', 'a')
        expect(result.tabs).toEqual([])
        expect(result.activeTabId).toBeNull()
    })

    it('closing an unknown tab id is a no-op', () => {
        const tabs = [tab('a'), tab('b')]
        const result = closeTab(tabs, 'a', 'does-not-exist')
        expect(result.tabs).toEqual(tabs)
        expect(result.activeTabId).toBe('a')
    })

    it('closing the active first tab selects the new first tab', () => {
        const tabs = [tab('a'), tab('b'), tab('c')]
        const result = closeTab(tabs, 'a', 'a')
        expect(result.tabs).toEqual([tab('b'), tab('c')])
        expect(result.activeTabId).toBe('b')
    })
})
