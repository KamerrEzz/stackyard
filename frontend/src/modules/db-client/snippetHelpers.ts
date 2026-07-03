export function parseTagsInput(input: string): string[] {
    const seen = new Set<string>()
    const tags: string[] = []
    for (const raw of input.split(',')) {
        const tag = raw.trim()
        if (tag && !seen.has(tag)) {
            seen.add(tag)
            tags.push(tag)
        }
    }
    return tags
}

export function tagsToInput(tags: string[] | undefined): string {
    return (tags ?? []).join(', ')
}

export function parseTagsJSON(raw: string | undefined): string[] {
    if (!raw) {
        return []
    }
    try {
        const parsed = JSON.parse(raw)
        if (!Array.isArray(parsed)) {
            return []
        }
        return parsed.filter((tag): tag is string => typeof tag === 'string')
    } catch {
        return []
    }
}
