# Import / Export

Move data in and out of a connection without leaving Stackyard.

## Export

Export scope is explicit: a full table, or the result set of the
currently executed query.

- **CSV** and **JSON** exports preserve column types faithfully enough to
  round-trip — dates, numbers, and nulls stay distinguishable from an
  empty string.
- **SQL dump** export produces valid `CREATE TABLE` + `INSERT` statements,
  importable into a fresh instance of the same engine (PostgreSQL/MySQL).
- Schema-only exports are also available as a real `schema.prisma` file or
  a Drizzle `schema.ts` file, generated from the same table introspection
  used elsewhere in the DB Client.

## Import

CSV/JSON import validates the file against the target table's columns
**before** committing — mismatches are reported up front, and nothing is
written if validation fails.
