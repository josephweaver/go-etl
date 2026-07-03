# 010 Main Database Contract

Status: proposed

## Objective

Replace the optional `ledger_db_path` startup path with a required main-database
contract that resolves the qualified controller driver and connection string,
opens the selected database, and initializes or strictly validates its schema
before later controller services or the HTTP listener are constructed.

## Required Context

Read these files first:

- `docs/epics/controller-startup-resolution/README.md`
- `docs/epics/controller-startup-resolution/009-startup-resolver-assembly.md`
- `docs/controller.internal.datamodel.md`
- `cmd/controller/main.go`
- `cmd/controller/config_test.go`
- `cmd/controller/main_test.go`
- `internal/ledger/sqlite.go`
- `internal/ledger/sqlite_test.go`

Do not read unrelated files unless a targeted test failure directly requires
it.

## Allowed Production Files

- `cmd/controller/main.go`
- `internal/ledger/sqlite.go`

## Allowed Test Files

- `cmd/controller/config_test.go`
- `cmd/controller/main_test.go`
- `internal/ledger/sqlite_test.go`

## Out Of Scope

- Supporting database drivers other than the currently registered SQLite driver
- Inferring the database driver from the connection string
- Allowing `override` declarations to replace either required database variable
- Database pool, connection-lifetime, timeout, TLS, or migration-policy variables
- Adding a generic database abstraction or aggregate runtime-config object
- Database ownership locks, leases, leader election, or stale-owner policy
- Sensitivity propagation, protected secret storage, or general-purpose error sanitization
- Filesystem, logging, caretaker, HTTP-setting, recovery-mode, or readiness contracts
- Changing the ledger schema beyond validation needed to enforce schema version 1
- Retaining the startup resolver or resolved connection string on `Controller`

## Acceptance Criteria

- The database consumer resolves
  `controller_config.main_database_driver` and
  `controller_config.main_database_connection_string` as required, non-empty
  strings through a caller-supplied bounded startup resolver.
- Both lookups are qualified to `controller_config`, so declarations in
  `override` cannot replace either database value.
- A missing driver and a missing connection string return distinct errors that
  identify the database consumer and the missing qualified variable.
- A declared connection string whose expression has a missing dependency
  returns resolution context identifying
  `controller_config.main_database_connection_string` and the missing
  dependency without including the materialized connection string.
- The only accepted initial driver value is `sqlite`; an unsupported driver is
  rejected before a database handle is returned.
- For `sqlite`, the resolved connection string is passed to the existing SQLite
  opening boundary as its path or DSN; the driver is not inferred from that
  value.
- Opening or schema initialization failure closes any newly opened database and
  returns an error with main-database startup context.
- A database with no schema is initialized at schema version 1.
- An existing database is accepted only when `schema_version` contains exactly
  one valid row whose version equals the controller-supported version.
- A missing version row, multiple version rows, an unreadable or malformed
  version, or a version newer or older than the supported version fails startup
  without rewriting the recorded version.
- Existing schema initialization remains idempotent for a valid version-1
  database.
- Live controller startup builds the bounded startup resolver and constructs
  the required database before constructing the execution environment or
  binding the HTTP listener.
- Live startup no longer treats the main database as optional and no longer
  reads `ledger_db_path`.
- The bounded resolver and resolved driver and connection-string values remain
  local to database construction; the returned database handle may remain on
  `Controller` for its normal lifetime.
- Targeted controller database-contract and SQLite schema-version tests pass.

## Notes

- In this slice, `main_database_driver = "sqlite"` selects the existing
  `modernc.org/sqlite` implementation. The connection string may therefore be
  a filesystem path, `:memory:`, or another DSN already accepted by that
  driver.
- Strict schema validation is intentionally separate from future migration
  policy. Version 1 may be created for an uninitialized database, but an
  existing non-version-1 database is not migrated or downgraded by this slice.
- Schema creation and version inspection should occur through one ledger-owned
  operation so controller startup does not acquire SQLite schema knowledge.
- `database/sql.Open` may defer physical connection work. Schema initialization
  and validation provide the required connectivity check for this SQLite path.
- Preserve consumer context when wrapping errors, but do not render resolved
  variable values. Full sensitivity propagation and sink sanitization remain
  owned by the `sensitive-variable-propagation` epic.
