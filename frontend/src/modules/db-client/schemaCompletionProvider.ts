import * as monaco from 'monaco-editor'
import {buildSchemaSuggestions, filterSchemaSuggestions, schemaProviderForModel, type SchemaSuggestion} from './schemaCompletion'

function completionRangeForWord(position: monaco.Position, word: monaco.editor.IWordAtPosition): monaco.IRange {
    return {
        startLineNumber: position.lineNumber,
        endLineNumber: position.lineNumber,
        startColumn: word.startColumn,
        endColumn: word.endColumn,
    }
}

function monacoKindFor(kind: SchemaSuggestion['kind']): monaco.languages.CompletionItemKind {
    return kind === 'table' ? monaco.languages.CompletionItemKind.Struct : monaco.languages.CompletionItemKind.Field
}

/**
 * Builds the single, global `languages.CompletionItemProvider` registered
 * for the `'sql'` language (see ensureSqlCompletionProviderRegistered).
 * Monaco invokes provideCompletionItems with the specific model/position the
 * user is typing in; looking that model up in schemaProviderForModel is what
 * resolves "which tab's schema applies" without leaking suggestions across
 * tabs (tasks.md 3.8, 4.8).
 */
export function createSqlCompletionProvider(): monaco.languages.CompletionItemProvider {
    return {
        provideCompletionItems(model, position) {
            const provider = schemaProviderForModel(model)
            if (!provider) {
                return {suggestions: []}
            }

            const word = model.getWordUntilPosition(position)
            const range = completionRangeForWord(position, word)
            const suggestions = filterSchemaSuggestions(buildSchemaSuggestions(provider()), word.word)

            return {
                suggestions: suggestions.map((suggestion) => ({
                    label: suggestion.label,
                    kind: monacoKindFor(suggestion.kind),
                    detail: suggestion.detail,
                    insertText: suggestion.insertText,
                    range,
                })),
            }
        },
    }
}

let sqlCompletionProviderRegistered = false

/**
 * Registers the SQL completion provider exactly once for the process's
 * lifetime, no matter how many QueryEditor instances mount — Monaco's
 * registerCompletionItemProvider has no built-in idempotency guard, and
 * registering it once per mounted tab would return duplicate suggestions
 * for every keystroke (tasks.md 3.8's multi-tab shell can mount several
 * QueryEditor instances at once).
 */
export function ensureSqlCompletionProviderRegistered(): void {
    if (sqlCompletionProviderRegistered) {
        return
    }
    sqlCompletionProviderRegistered = true
    monaco.languages.registerCompletionItemProvider('sql', createSqlCompletionProvider())
}
