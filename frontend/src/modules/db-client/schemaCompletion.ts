import type * as monaco from 'monaco-editor'
import type {dbengine} from '../../../wailsjs/go/models'

export interface SchemaSuggestion {
    label: string
    kind: 'table' | 'column'
    detail: string
    insertText: string
}

/**
 * Flattens a session's tables into a deduplicated suggestion list: one entry
 * per table name, plus one entry per distinct column name across every
 * table (tasks.md 4.8). Column name collisions across tables collapse into a
 * single suggestion whose `detail` lists every table that has a column with
 * that name, rather than one suggestion per table/column pair — Monaco's
 * completion list reads better without `id` (users) and `id` (orders)
 * showing up as two indistinguishable "id" entries.
 *
 * This is a flat, non-context-aware list (no "after FROM prefer tables,
 * after SELECT prefer columns" detection) — an acceptable scope reduction
 * for this task per tasks.md 4.8; ranking suggestions by SQL clause context
 * is a reasonable follow-up, not implemented here.
 */
export function buildSchemaSuggestions(tables: readonly dbengine.TableInfo[]): SchemaSuggestion[] {
    const suggestions: SchemaSuggestion[] = []
    const tablesByColumn = new Map<string, Set<string>>()

    for (const table of tables) {
        suggestions.push({label: table.Name, kind: 'table', detail: 'table', insertText: table.Name})

        for (const column of table.Columns ?? []) {
            const owners = tablesByColumn.get(column.Name) ?? new Set<string>()
            owners.add(table.Name)
            tablesByColumn.set(column.Name, owners)
        }
    }

    for (const [columnName, owners] of tablesByColumn) {
        suggestions.push({
            label: columnName,
            kind: 'column',
            detail: `column · ${Array.from(owners).join(', ')}`,
            insertText: columnName,
        })
    }

    return suggestions
}

/** Keeps only suggestions whose label starts with prefix, case-insensitively. An empty prefix keeps everything. */
export function filterSchemaSuggestions(suggestions: readonly SchemaSuggestion[], prefix: string): SchemaSuggestion[] {
    if (!prefix) {
        return [...suggestions]
    }
    const lowerPrefix = prefix.toLowerCase()
    return suggestions.filter((suggestion) => suggestion.label.toLowerCase().startsWith(lowerPrefix))
}

type SchemaProvider = () => readonly dbengine.TableInfo[]

/**
 * Associates a live Monaco text model with the function that returns its
 * owning tab's current schema snapshot. A single completion provider is
 * registered once for the whole app (see schemaCompletionProvider.ts)
 * because Monaco has no per-editor-instance registration API for language
 * services — this map is what lets that one global provider resolve "which
 * tab's tables/columns apply" from the model it was invoked against,
 * keeping autocomplete suggestions from one DB Client tab out of another's
 * editor (tasks.md 3.8's per-tab independence, applied to 4.8).
 *
 * This file only imports monaco-editor's types (`import type`), never the
 * package itself: the actual runtime import lives in
 * schemaCompletionProvider.ts, kept separate so this module (and its
 * Vitest suite) never triggers monaco-editor's browser-only side effects,
 * which crash under Vitest's node test environment.
 */
const modelSchemaProviders = new Map<monaco.editor.ITextModel, SchemaProvider>()

export function registerModelSchemaProvider(model: monaco.editor.ITextModel, provider: SchemaProvider): void {
    modelSchemaProviders.set(model, provider)
}

export function unregisterModelSchemaProvider(model: monaco.editor.ITextModel): void {
    modelSchemaProviders.delete(model)
}

export function schemaProviderForModel(model: monaco.editor.ITextModel): SchemaProvider | undefined {
    return modelSchemaProviders.get(model)
}
