# 008 Downstream Member Hydration and Data Projection

Status: implemented

## Objective

Let a downstream step fan out over a completed collection domain, bind one concrete collection member through the existing asset-binding shape, and receive the matching concrete materialized path in the step-local `data` namespace.

## Current State

The current controller hydration path matches completed materialization manifests to compute requirements and injects `materialized_data_assets`.

`internal/model/materialized_projection.go` and `cmd/worker/data_scope.go` convert a concrete manifest into:

```text
data.<alias>.asset_key
data.<alias>.materialization_domain_id
data.<alias>.path[]
data.<alias>.files.<role>.path
```

The current system does not have a compact collection descriptor or a member lookup keyed by collection dimension bindings.

Ordinary variable resolution requires complete values and should continue to do so.

## Target State

### Fan-out domain

A completed collection descriptor is available in workflow step scope:

```yaml
fan_out:
  over: "${workflow.step[0].dimensions.year.values[*]}"
  as: year
  id: "${fanout.year}"
```

### Concrete data binding

The downstream step uses the normal asset definition:

```yaml
data:
  inputs:
    cdl:
      asset: cdl
      with:
        year: "${fanout.year}"
```

The compiler resolves this to the concrete source asset key and expected materialization identity for that year.

### Durable lookup

The controller finds the completed member evidence using semantic facts:

```text
source asset key
materialization domain
destination/materialization identity
collection fingerprint where applicable
```

Do not match only by human-readable work-item ID or alias.

### Concrete worker projection

The hydrated compute assignment receives one concrete `MaterializedDataAssetManifest`.

The worker's existing data-scope projection produces:

```text
${data.cdl.path[0]}
```

as an ordinary concrete path.

No unresolved `${year}` placeholder reaches compute execution.

### Template field behavior

`workflow.step[0].path` remains useful as inspectable logical metadata. Canonical execution binds through `data.inputs`; it does not require nested interpolation or a generic second pass over arbitrary strings.

## Concept Decision

Reuse the existing data-binding and materialized-data projection contracts.

The collection descriptor is the logical index/domain contract. Concrete member manifests are the execution contract.

Do not add `TypePathTemplate` to `internal/variable` in this phase. Collection template binding belongs in asset hydration, where the controller has the asset definition, dimension bindings, semantic identity, and completed member evidence.

## Required Context

Read these files first:

- `AGENTS.md`
- `docs/concepts/data-asset-collections-and-materialization/README.md`
- Operational Slices 003, 005, 006, and 007
- `cmd/controller/asset_materialize_hydration.go`
- controller hydration tests
- `cmd/controller/workflow_stage_activation.go`
- `cmd/controller/workflow_stage_queue.go`
- relevant stage activation/queue tests
- `cmd/controller/workflow_outputs.go`
- `internal/workflow/data_instance.go`
- `internal/workflow/data_collection.go`
- `internal/workflow/compile_stage.go`
- `internal/model/materialized_projection.go`
- `internal/model/materialized_projection_test.go`
- `cmd/worker/data_scope.go`
- `cmd/worker/data_scope_test.go`
- `internal/model/data_asset.go`

Do not read provider subprocess, scheduler, transport, publication, client UI, or unrelated variable-function code.

## Allowed Production Files

- `cmd/controller/asset_materialize_hydration.go`
- `cmd/controller/workflow_stage_activation.go` only for collection-aware readiness/hydration
- `cmd/controller/workflow_stage_queue.go` only for hydrated payload insertion
- `internal/workflow/data_instance.go`
- `internal/workflow/data_collection.go`
- `internal/workflow/compile_stage.go` only for concrete member requirement metadata
- `internal/model/materialized_projection.go`
- `cmd/worker/data_scope.go`
- `internal/model/data_asset.go` only for lookup/projection facts

## Allowed Test Files

- controller asset-materialization hydration tests
- `cmd/controller/workflow_stage_activation_test.go`
- `cmd/controller/workflow_stage_queue_test.go`
- `internal/workflow/data_instance_test.go`
- `internal/workflow/data_collection_test.go`
- `internal/workflow/compile_stage_test.go`
- `internal/model/materialized_projection_test.go`
- `cmd/worker/data_scope_test.go`
- `internal/model/data_asset_test.go`

## Allowed Documentation Files

- `docs/concepts/data-asset-collections-and-materialization/008-downstream-member-hydration-and-data-projection.md`
- `PROJECT_STATE.md` after implementation

## Out Of Scope

- Direct generic evaluation of `workflow.step[0].path` as a concrete path without bindings.
- A new variable kind.
- Nested interpolation.
- Per-member pipelining before the complete materialization stage finishes.
- Invocation-time subset materialization.
- Provider discovery.
- Multi-role collection members.
- Worker-local scope.
- Changes to Python or plugin-specific argument syntax beyond existing data-path projection.
- A global materialization catalog.

## Acceptance Criteria

- A downstream step can fan out over `workflow.step[0].dimensions.year.values[*]`.
- Fan-out values retain integer type.
- For each year, `data.inputs.cdl.with.year` resolves to the correct concrete asset instance.
- The controller selects the matching completed member by semantic identity and domain.
- A step alias can differ from the materialization-step alias without changing physical matching.
- The hydrated assignment contains one concrete materialized asset manifest for the requested year.
- `data.cdl.path[0]` is the deterministic absolute path for that year.
- No unresolved collection placeholder reaches the worker compute operation.
- Requesting a year outside the declared domain fails before worker execution.
- Missing completed member evidence fails before worker execution.
- Wrong materialization domain evidence cannot satisfy the binding.
- Wrong selection/archive member cannot satisfy the binding.
- Conflicting destination evidence cannot satisfy the binding.
- Controller restart can hydrate the same concrete member from durable facts.
- Ordinary scalar asset hydration remains unchanged.
- Existing named file-role projections remain unchanged for scalar assets.
- Generic variable resolver tests remain unchanged; no partial-resolution mode is introduced.
- `go test ./internal/workflow ./cmd/controller ./cmd/worker ./internal/model` passes.

## Notes

- The logical path template is useful for inspection and deterministic naming, but semantic asset identity remains the authoritative lookup contract.
- Keep the concrete worker-facing projection identical to current plugin expectations.
- Suggested HCI: `EC-3 / operational slice / files(9)+test+doc+cleanup`.
