# Migrations

Minimal, explicit schema versioning for PostgreSQL and MySQL — no
auto-magic, no bulk rollback.

- **Create migration** scaffolds a timestamped up/down file pair tied to a
  connection profile.
- **Apply** runs all pending migrations in order and records applied state
  in a tracking table (`schema_migrations`) inside the target database. A
  mid-run failure leaves the tracking table accurate — the failed
  migration is not marked applied — and surfaces the underlying database
  error.
- **Rollback** reverts exactly one migration step at a time; there is no
  bulk rollback by design.

A dedicated migrations panel lists pending and applied migrations per
connection, with apply/rollback actions available directly from the UI.
