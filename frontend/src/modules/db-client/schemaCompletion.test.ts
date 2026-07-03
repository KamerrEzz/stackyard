import type * as monaco from 'monaco-editor'
import {describe, expect, it} from 'vitest'
import type {dbengine} from '../../../wailsjs/go/models'
import {
    buildSchemaSuggestions,
    filterSchemaSuggestions,
    registerModelSchemaProvider,
    schemaProviderForModel,
    unregisterModelSchemaProvider,
} from './schemaCompletion'

function table(name: string, columnNames: string[]): dbengine.TableInfo {
    return {
        Name: name,
        Columns: columnNames.map((columnName) => ({
            Name: columnName,
            DataType: 'text',
            Nullable: true,
            IsPrimaryKey: columnName === 'id',
        })),
    }
}

describe('buildSchemaSuggestions', () => {
    it('suggests every table name', () => {
        const suggestions = buildSchemaSuggestions([table('users', ['id']), table('orders', ['id'])])

        const tableSuggestions = suggestions.filter((s) => s.kind === 'table')
        expect(tableSuggestions.map((s) => s.label).sort()).toEqual(['orders', 'users'])
    })

    it('suggests every distinct column name across all tables', () => {
        const suggestions = buildSchemaSuggestions([table('users', ['id', 'email']), table('orders', ['id', 'total'])])

        const columnSuggestions = suggestions.filter((s) => s.kind === 'column')
        expect(columnSuggestions.map((s) => s.label).sort()).toEqual(['email', 'id', 'total'])
    })

    it('collapses a column name shared by multiple tables into one suggestion', () => {
        const suggestions = buildSchemaSuggestions([table('users', ['id']), table('orders', ['id'])])

        const idSuggestions = suggestions.filter((s) => s.kind === 'column' && s.label === 'id')
        expect(idSuggestions).toHaveLength(1)
        expect(idSuggestions[0].detail).toContain('users')
        expect(idSuggestions[0].detail).toContain('orders')
    })

    it('returns no suggestions for an empty schema', () => {
        expect(buildSchemaSuggestions([])).toEqual([])
    })

    it('tolerates a table with no columns', () => {
        const suggestions = buildSchemaSuggestions([{Name: 'empty_table', Columns: []}])
        expect(suggestions).toEqual([{label: 'empty_table', kind: 'table', detail: 'table', insertText: 'empty_table'}])
    })
})

describe('filterSchemaSuggestions', () => {
    const suggestions = buildSchemaSuggestions([table('users', ['id', 'email']), table('user_roles', ['id', 'role'])])

    it('returns everything for an empty prefix', () => {
        expect(filterSchemaSuggestions(suggestions, '')).toHaveLength(suggestions.length)
    })

    it('keeps only labels starting with the prefix, case-insensitively', () => {
        const filtered = filterSchemaSuggestions(suggestions, 'US')
        expect(filtered.map((s) => s.label).sort()).toEqual(['user_roles', 'users'])
    })

    it('returns nothing when no label matches', () => {
        expect(filterSchemaSuggestions(suggestions, 'zzz')).toEqual([])
    })
})

describe('model schema provider registry', () => {
    function fakeModel(): monaco.editor.ITextModel {
        return {} as monaco.editor.ITextModel
    }

    it('returns undefined for a model that was never registered', () => {
        expect(schemaProviderForModel(fakeModel())).toBeUndefined()
    })

    it('returns the provider registered for a given model instance', () => {
        const modelA = fakeModel()
        const modelB = fakeModel()
        const providerA = () => [table('a_only', ['id'])]
        const providerB = () => [table('b_only', ['id'])]

        registerModelSchemaProvider(modelA, providerA)
        registerModelSchemaProvider(modelB, providerB)

        expect(schemaProviderForModel(modelA)).toBe(providerA)
        expect(schemaProviderForModel(modelB)).toBe(providerB)

        unregisterModelSchemaProvider(modelA)
        unregisterModelSchemaProvider(modelB)
    })

    it('isolates two models from each other — this is the cross-tab-leak guarantee', () => {
        const tabA = fakeModel()
        const tabB = fakeModel()
        registerModelSchemaProvider(tabA, () => [table('tab_a_table', ['id'])])
        registerModelSchemaProvider(tabB, () => [table('tab_b_table', ['id'])])

        const tablesForA = schemaProviderForModel(tabA)?.() ?? []
        const tablesForB = schemaProviderForModel(tabB)?.() ?? []

        expect(tablesForA.map((t) => t.Name)).toEqual(['tab_a_table'])
        expect(tablesForB.map((t) => t.Name)).toEqual(['tab_b_table'])

        unregisterModelSchemaProvider(tabA)
        unregisterModelSchemaProvider(tabB)
    })

    it('forgets a model after it is unregistered', () => {
        const model = fakeModel()
        registerModelSchemaProvider(model, () => [])

        unregisterModelSchemaProvider(model)

        expect(schemaProviderForModel(model)).toBeUndefined()
    })
})
