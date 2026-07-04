# 003 Core Execution Schema

Status: proposed

## Objective

Extend the persistence schema from metadata-only bootstrap to the first
workflow-execution table set. The slice adds tables for immutable source
identity, workflow runs, stage plans, compiled work items, workers, attempts,
current placement, and terminal outcomes.

This slice creates schema only. It does not add repository methods for inserting
or transitioning execution records.

## Required Context

Read these files first:

- `docs/epics/workflow-execution-persistence/README.md`
- `docs/epics/workflow-execution-persistence/001-database-bootstrap-and-schema-versioning.md`
- `internal/persistence/store.go`
- `internal/persistence/db_adapter_sqlite.go`
- `internal/persistence/db_adapter_sqlite_test.go`

Do not read controller files unless compile or test failures directly require
it.

## Allowed Production Files

- `internal/persistence/db_adapter_sqlite.go`

## Allowed Test Files

- `internal/persistence/db_adapter_sqlite_test.go`

## Out Of Scope

- Repository methods such as `CreateWorkflowRun`, `InsertWorkItems`, or
  `ClaimNextWork`.
- Controller startup wiring.
- Migrating existing `internal/ledger` data.
- Source-control cache implementation.
- GitHub API or Git cache behavior.
- UUIDv7 generation.
- Canonical JSON hashing integration.
- Retention cleanup behavior.
- Work claiming, completion, failure, retry, or stage publication logic.

## Acceptance Criteria

- Opening a new SQLite store creates the core workflow-execution tables:
  `projects`, `workflows`, `workflow_instances`, `workflow_stages`,
  `work_items`, `workers`, `work_item_attempts`, `queued_work`,
  `running_work`, `completed_work`, and `failed_work`.
- `workflow_instances` includes `submission_context_json`.
- `work_items` includes fields for compiled worker payload JSON and
  `resolved_inputs_sha256`.
- `completed_work` includes `skipped_parent_id`.
- Current placement tables represent current work location separately from
  terminal outcome tables.
- Foreign-key relationships prevent orphan rows for the table relationships
  introduced in this slice.
- Uniqueness constraints preserve the first idempotency boundaries, including
  stage identity by `(run_id, stage_index)` and work-item identity by
  `(run_id, stage_index, work_item_index)`.
- Existing supported metadata-only databases can be opened and upgraded or
  initialized to the new schema version through the store-opening path.
- Unsupported newer schema versions still fail closed.

## Notes

- This slice may increase `SupportedSchemaVersion` from `1` to `2` if the
  implementation treats the metadata-only schema from slice 001 as version 1.
- Use SQLite `CHECK (json_valid(...))` constraints for JSON columns where
  practical.
- Keep DDL in the SQLite adapter for now. A later PostgreSQL adapter can define
  equivalent DDL behind the same store boundary.
- Prefer conservative columns that support the epic's documented identity and
  lifecycle model; do not add scheduling policy fields that belong to later
  epics.
