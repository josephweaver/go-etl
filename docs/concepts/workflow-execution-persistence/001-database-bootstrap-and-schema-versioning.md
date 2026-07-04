# 001 Database Bootstrap and Schema Versioning

Status: proposed

## Objective

Introduce the first persistence store-opening boundary for the main database.
The slice creates a narrow `OpenStore` path that opens SQLite, enables required
database safety settings, creates schema metadata for an empty database, accepts
the current supported schema version, and fails closed on unsupported newer
schemas.

This slice does not move controller queue behavior onto the database yet.

## Required Context

Read these files first:

- `docs/concepts/workflow-execution-persistence/README.md`
- `internal/ledger/sqlite.go`
- `internal/ledger/sqlite_test.go`

Do not read unrelated controller files unless compile or test failures directly
require it.

## Allowed Production Files

- `internal/persistence/store.go`
- `internal/persistence/db_adapter_sqlite.go`

This slice needs new production files. If the active HCI mode does not include
`+newfile`, pause before implementation and ask for an updated budget.

## Allowed Test Files

- `internal/persistence/store_test.go`
- `internal/persistence/db_adapter_sqlite_test.go`

## Out Of Scope

- Moving `cmd/controller` startup to the new store.
- Replacing `internal/ledger` call sites.
- Creating workflow, stage, work-item, worker, placement, or terminal tables.
- Implementing project/workflow source-control persistence.
- Implementing GitHub source-control access.
- Implementing cache pinning or cleanup.
- Implementing UUIDv7 generation.
- Implementing canonical JSON helpers.
- Implementing retention policy.

## Acceptance Criteria

- `OpenStore` creates a valid empty SQLite database when the target file does
  not exist.
- Opening an existing database at the supported schema version succeeds.
- Opening a database with a schema version newer than the supported version
  fails before returning a usable store.
- Schema initialization is transactional: a failed initialization does not leave
  a partially initialized supported schema.
- SQLite foreign-key enforcement is enabled for opened connections.
- The controller-facing API does not expose raw migration steps; startup code
  should eventually treat schema setup as part of opening the store.
- SQLite-specific behavior is contained behind the persistence/database adapter
  boundary.

## Notes

- Start from the behavior already proven in `internal/ledger/sqlite.go`, but do
  not move existing ledger code in this slice.
- The first schema may contain only schema metadata. Workflow execution tables
  belong to slice 003.
- Prefer a concrete `Store` type over a broad interface until a second database
  implementation exists.
- Keep this slice small enough that a later controller integration slice can
  choose whether to adopt the new store directly or bridge from existing ledger
  behavior.
