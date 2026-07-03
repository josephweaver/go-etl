# 004 Project and Workflow Persistence Methods

Status: proposed

## Objective

Add the first task-oriented persistence methods for immutable project and
workflow source identities. The methods should let the controller upsert and
retrieve project/workflow records by their generated IDs and safely delete only
unused records.

This slice introduces data access behavior only for `projects` and `workflows`.

## Required Context

Read these files first:

- `docs/epics/workflow-execution-persistence/README.md`
- `docs/epics/workflow-execution-persistence/003-core-execution-schema.md`
- `internal/persistence/store.go`
- `internal/persistence/db_adapter_sqlite.go`
- `internal/persistence/db_adapter_sqlite_test.go`

Do not read controller files unless compile or test failures directly require
it.

## Allowed Production Files

- `internal/persistence/store.go`

## Allowed Test Files

- `internal/persistence/store_test.go`

## Out Of Scope

- Workflow run creation.
- Stage, work-item, worker, attempt, placement, or terminal methods.
- Source-control cache or GitHub behavior.
- UUIDv7 generation.
- Canonical JSON computation.
- Controller startup integration.
- Deleting records used by workflow instances or other lifecycle state.
- Retention policy.

## Acceptance Criteria

- `Store` exposes a method to upsert one project source identity.
- `Store` exposes a method to retrieve one project source identity by
  `project_id`.
- `Store` exposes a method to delete a project only when no workflow references
  it.
- `Store` exposes a method to upsert one workflow source identity under an
  existing project.
- `Store` exposes a method to retrieve one workflow source identity by
  `workflow_id`.
- `Store` exposes a method to delete a workflow only when no workflow instance
  references it.
- Upsert methods are idempotent for identical values.
- Upsert methods reject conflicting values for an existing generated ID rather
  than silently rewriting immutable identity.
- Missing records return a distinguishable not-found result rather than an
  ambiguous zero-value record.
- Safe delete methods report whether a row was deleted.

## Notes

- Keep the API concrete on `Store`; do not introduce repository interfaces yet.
- Use explicit structs for project and workflow records so later controller code
  does not pass loosely shaped maps.
- This slice should not compute `config_sha256` or `workflow_sha256`; callers
  supply already computed canonical hashes.
- Prefer transaction boundaries inside each method where multiple statements are
  needed.
