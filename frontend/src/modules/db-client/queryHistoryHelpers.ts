/**
 * Collapses whitespace/newlines into single spaces and truncates to
 * maxLength, appending an ellipsis when truncation actually happened
 * (tasks.md 4.5, spec.md §4.10) — the history panel shows one line per
 * entry, with the untruncated text available via the row's own expand
 * toggle/title attribute rather than wrapping onto multiple lines.
 */
export function truncateQueryText(text: string, maxLength = 80): string {
    const singleLine = text.replace(/\s+/g, ' ').trim()
    if (singleLine.length <= maxLength) {
        return singleLine
    }
    return `${singleLine.slice(0, Math.max(maxLength - 1, 0))}…`
}
