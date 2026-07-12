# 009 Remove Legacy Hidden Materialization and Migrate Fixtures

Status: proposed

## Objective

Remove the planner behavior that discovers data assets inside compute parameters and generates materialization work implicitly, then migrate active workflows, fixtures, tests, and canonical documentation to explicit `asset.materialize` steps and collection definitions.

## Current State

The explicit materialization compiler exists, but the current stage planner still retains a legacy path derived from `cache_data_plan.go`: non-explicit compute items may be scanned for data-asset parameters and receive generated materialization dependencies.

The canonical document concept intends visible materialization work, but current repository fixtures and historical implementation tests include older parameter shapes and `cache_data` names.

After Operational Slice 004, the legacy code may have been mechanically renamed, but its hidden-generation behavior still exists until this slice.

## Target State

### Compiler behavior

The compiler only creates materialization members when the workflow authors:

```yaml
work:
  type: asset.materialize
```

A compute step that declares `data.inputs` but has no completed explicit materializer for the required shared member fails with a clear diagnostic.

No planner scans:

```text
parameters.data_assets
parameters.publish
```

to append hidden inbound or outbound operator work.

`commit_data` hidden-generation cleanup remains governed by the canonical document-model concept; this slice changes only inbound materialization unless the same already-approved migration fixture requires a mechanical current-shape update.

### Repository migration

Active examples use:

```text
asset.materialize
```

Collection examples declare finite dimensions and omit workflow fan-out on the materialization step.

All current tests distinguish:

```text
explicit authored member expansion
ordinary compute fan-out
compact collection logical output
```

### Compatibility

`cache_data` is not accepted as a work-item type or canonical workflow type.

Legacy compute-parameter shapes fail with migration guidance rather than silently generating work.

## Concept Decision

Delete the hidden planning path rather than keeping two authoring systems.

Preserve low-level functions that compute source asset keys, materialization keys, resource constraints, and payloads when they are used by the explicit compiler. Move those helpers into clearly named explicit/materialization files if deleting the planner file would otherwise discard reusable mechanics.

Do not remove tests merely to make the migration pass; convert them to the new explicit contract or replace them with a precise rejection test.

## Required Context

Read these files first:

- `AGENTS.md`
- `PROJECT_STATE.md`
- `docs/concepts/data-asset-collections-and-materialization/README.md`
- all implemented prior slices in this concept
- `docs/concepts/canonical-workflow-data-document-model/README.md`
- affected canonical document-model OS files
- `internal/workflow/asset_materialize_plan.go`
- `internal/workflow/asset_materialize_plan_test.go`
- `internal/workflow/explicit_asset_materialize.go`
- `internal/workflow/explicit_asset_materialize_test.go`
- `internal/workflow/document_adapter.go`
- `internal/workflow/document_adapter_test.go`
- `internal/workflow/data_operator_integration_smoke_test.go`
- current repository workflow/config fixtures containing `data_assets`, `cache_data`, or generated materialization assumptions
- current scripts/runbooks that invoke data-asset smokes

Search the repository for:

```text
cache_data
CacheData
parameters.data_assets
"data_assets"
PlanAssetMaterializeWorkItems
generated materialization
```

Classify each result before editing.

## Allowed Production Files

- `internal/workflow/asset_materialize_plan.go`
- `internal/workflow/explicit_asset_materialize.go`
- `internal/workflow/document_adapter.go`
- `internal/workflow/compile_stage.go`
- narrow controller call sites only if they still invoke the legacy planner
- delete obsolete production files when all reusable helpers have moved

## Allowed Test Files

- `internal/workflow/asset_materialize_plan_test.go`
- `internal/workflow/explicit_asset_materialize_test.go`
- `internal/workflow/document_adapter_test.go`
- `internal/workflow/compile_stage_test.go`
- `internal/workflow/data_operator_integration_smoke_test.go`
- active controller/worker tests that serialize legacy shapes
- repository fixture JSON/YAML files
- smoke scripts whose only required change is the new authored contract

## Allowed Documentation Files

- `docs/concepts/data-asset-collections-and-materialization/009-remove-legacy-hidden-materialization-and-migrate-fixtures.md`
- `docs/concepts/canonical-workflow-data-document-model/README.md`
- `docs/concepts/canonical-workflow-data-document-model/009-explicit-cache-data-step.md`
- `docs/concepts/canonical-workflow-data-document-model/010-shared-materialization-hydration.md`
- `docs/concepts/canonical-workflow-data-document-model/011-step-data-projections-and-worker-resolution.md`
- `docs/concepts/canonical-workflow-data-document-model/013-workflow-migration-and-equivalence-smoke.md`
- `docs/concepts/data-assets-and-materialized-outputs/README.md` only for a concise supersession note
- affected smoke/runbook documentation
- `PROJECT_STATE.md`

## Out Of Scope

- New provider types.
- New collection-domain sources.
- Invocation-time subsets.
- `commit_data` redesign.
- Worker-scope materialization.
- Per-member pipelining.
- Real data downloads.
- Broad documentation rewrites unrelated to materialization.
- Permanent legacy compatibility mode.
- Removing terminal evidence or current recovery facts.
- Customer-specific workflow logic.

## Acceptance Criteria

- No compute-parameter scanner creates inbound materialization work.
- The explicit compiler retains all required asset-key, payload, transfer, and resource-constraint helpers.
- A compute step requiring a shared asset without prior explicit materialization fails before worker execution.
- Active canonical workflows use `asset.materialize`.
- Active collection materialization steps do not repeat the collection domain with workflow `fan_out`.
- Current fixture behavior is preserved under explicit authored steps.
- JSON and YAML reference fixtures remain semantically equivalent.
- `cache_data` is rejected in current workflow and work-item tests.
- Legacy `parameters.data_assets` authoring is rejected with migration guidance.
- Tests formerly proving hidden generation are converted into explicit-materialization or rejection tests.
- No tests are deleted solely to avoid migration work.
- Production code has no active `Plan*` function whose purpose is discovering materialization from compute parameters.
- `git grep` finds `cache_data` only in deliberate historical/supersession prose, if any.
- Current canonical docs use `asset.materialize` and describe compact collection output.
- `go test ./...` passes.

## Notes

- Keep reusable identity/resource helper functions; remove only hidden authoring behavior and obsolete names.
- This is the breaking migration gate. Review the final repository-wide search results explicitly.
- Suggested HCI: `EC-3 / operational slice / files(5)+test+doc+config+cleanup`.
