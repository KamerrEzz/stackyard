import {classifyMongoValue, describeScalarValue, summarizeContainer} from './mongoDocumentHelpers'

interface MongoDocumentTreeProps {
    document: Record<string, unknown>
    expandedPaths: ReadonlySet<string>
    onToggle: (path: string) => void
}

interface MongoTreeNodeProps {
    nodeKey: string
    value: unknown
    path: string
    expandedPaths: ReadonlySet<string>
    onToggle: (path: string) => void
    depth: number
}

const TYPE_BADGE_CLASS = 'rounded border border-ink-700 px-1 text-[10px] font-normal normal-case text-ink-500'

/**
 * One row of a document's tree (tasks.md 5.2): an object key or array index,
 * its type badge, and — for `'object'`/`'array'` values — a caret that
 * expands/collapses its children in place. Recurses for nested
 * objects/arrays; scalars render their value inline and never recurse.
 */
function MongoTreeNode({nodeKey, value, path, expandedPaths, onToggle, depth}: MongoTreeNodeProps) {
    const kind = classifyMongoValue(value)
    const isContainer = kind === 'object' || kind === 'array'
    const isExpanded = isContainer && expandedPaths.has(path)

    return (
        <div style={{paddingLeft: depth === 0 ? 0 : 14}}>
            <div className="flex items-center gap-1.5 py-0.5 text-xs">
                {isContainer ? (
                    <button
                        type="button"
                        onClick={() => onToggle(path)}
                        aria-label={isExpanded ? `Collapse ${nodeKey}` : `Expand ${nodeKey}`}
                        aria-expanded={isExpanded}
                        className="w-3 shrink-0 text-ink-500 hover:text-brass-400"
                    >
                        {isExpanded ? '▾' : '▸'}
                    </button>
                ) : (
                    <span className="w-3 shrink-0" />
                )}
                <span className="font-mono text-ink-300">{nodeKey}:</span>
                {isContainer ? (
                    <>
                        <span className={TYPE_BADGE_CLASS}>{kind}</span>
                        {!isExpanded && (
                            <span className="text-ink-500">
                                {summarizeContainer(value as Record<string, unknown> | unknown[])}
                            </span>
                        )}
                    </>
                ) : (
                    <>
                        <span className={kind === 'null' ? 'italic text-ink-600' : 'font-mono text-ink-100'}>
                            {describeScalarValue(value, kind)}
                        </span>
                        <span className={TYPE_BADGE_CLASS}>{kind}</span>
                    </>
                )}
            </div>

            {isContainer && isExpanded && (
                <div className="border-l border-ink-800 pl-1">
                    {Array.isArray(value)
                        ? value.map((item, index) => (
                              <MongoTreeNode
                                  key={index}
                                  nodeKey={String(index)}
                                  value={item}
                                  path={`${path}.${index}`}
                                  expandedPaths={expandedPaths}
                                  onToggle={onToggle}
                                  depth={depth + 1}
                              />
                          ))
                        : Object.entries(value as Record<string, unknown>).map(([key, childValue]) => (
                              <MongoTreeNode
                                  key={key}
                                  nodeKey={key}
                                  value={childValue}
                                  path={path === '' ? key : `${path}.${key}`}
                                  expandedPaths={expandedPaths}
                                  onToggle={onToggle}
                                  depth={depth + 1}
                              />
                          ))}
                </div>
            )}
        </div>
    )
}

/**
 * Renders one document as an expandable/collapsible tree matching its BSON
 * structure (tasks.md 5.2, spec.md §4.4): every top-level field is always
 * shown (the document itself is never collapsible — collapsing the whole
 * card is `MongoDocumentView`'s job, one level up), and every nested
 * object/array field starts collapsed until the user expands it via
 * `expandedPaths`/`onToggle`, both owned by the parent so expand state
 * survives a document being re-fetched into the same list position.
 */
function MongoDocumentTree({document, expandedPaths, onToggle}: MongoDocumentTreeProps) {
    return (
        <div className="flex flex-col">
            {Object.entries(document).map(([key, value]) => (
                <MongoTreeNode
                    key={key}
                    nodeKey={key}
                    value={value}
                    path={key}
                    expandedPaths={expandedPaths}
                    onToggle={onToggle}
                    depth={0}
                />
            ))}
        </div>
    )
}

export default MongoDocumentTree
