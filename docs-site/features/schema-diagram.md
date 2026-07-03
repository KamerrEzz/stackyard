# Schema Diagram

An entity-relationship diagram generated directly from live schema
introspection — no manual modeling, no external tool.

![Live ER diagram with a foreign-key relationship](/screenshots/schema-diagram.png)

## Relational (PostgreSQL, MySQL)

The diagram is built from live introspection of the active schema —
tables, columns, types, primary keys, and foreign keys — and rendered
inside the app using Mermaid's `erDiagram` syntax, so no external file is
required to view it. It supports zoom and pan for large schemas, staying
legible even projected on a classroom screen. A **Regenerate** button
refreshes the diagram on demand; it is intentionally not a live/
auto-updating view. Diagrams export as PNG, SVG, or raw copyable Mermaid
text, so the diagram can be pasted directly into notes or a README.

## MongoDB (inferred structure)

Since MongoDB collections don't have real foreign keys, the diagram
infers each collection's shape from a sample of documents (sample size
configurable, with a sensible default) and is explicitly labeled
**"inferred structure, not an enforced relationship"** — reinforcing the
real difference between relational and document modeling rather than
papering over it. It shares the same export capabilities as the
relational diagram.
