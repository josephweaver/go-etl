# 007 Collection Step Output Synthesis

Status: implemented

## Objective

Change logical output aggregation for collection `asset.materialize` steps so the controller preserves concrete member evidence but writes one compact collection descriptor instead of a normal list of member outputs.

## Current State

`cmd/controller/workflow_outputs.go` currently:

- returns the one work-item output object when a step has one item;
- returns a JSON list of output objects when a step has more than one item;
- stores the aggregate as the logical step output used by downstream workflow scope.

That behavior is natural for ordinary compute fan-out.

A collection materialization step, however, represents one indexed asset collection. Returning sixteen ordinary member manifests leaks the execution topology and increases output size.

Completed work-item output JSON and workflow step output facts are already durable and bounded.

## Target State

### Member evidence remains unchanged in purpose

Every concrete `asset.materialize` work item retains its terminal output and hashes. Controller restart and audit paths can inspect those records.

### Collection detection

Member outputs carry validated collection metadata:

```text
same collection fingerprint
same asset definition name
same materialization domain
same dimension order
same member count
unique member index
unique member bindings
same normalized relative path template
concrete destination and evidence
```

The aggregator detects a collection only through the versioned member metadata contract, not by guessing from filenames or work-item IDs.

### Completeness checks

Before writing the logical output, the controller verifies:

- expected member count is positive;
- exactly one member exists for every index;
- all members are completed or validly reused;
- collection fingerprints agree;
- domain/root facts agree;
- concrete paths conform to the declared template;
- dimension bindings reconstruct the declared finite domain;
- materialization identities are unique;
- content/member evidence is valid and canonically ordered.

### Compact logical output

The controller writes one object with schema:

```text
goet/materialized-asset-collection/v1
```

It contains:

```text
asset name
materialization domain
dimension order
dimension values
absolute deferred path template
required bindings
member count
ordered member-evidence SHA-256
collection fingerprint
```

It does not contain the normal list of member output objects.

### Step scope

The ordinary workflow step scope can expose:

```text
workflow.step[0].dimensions.year.values
workflow.step[0].path
workflow.step[0].member_count
```

The `path` value remains a collection descriptor field; downstream concrete path hydration is implemented in Operational Slice 008.

## Concept Decision

Special-case aggregation by a versioned output schema/collection-member contract, not by step ID, work-item count, or operation-name string alone.

Reuse existing `workflow_step_output_facts`; do not add a new persistence table unless implementation proves the current bounded fact cannot represent the descriptor.

The compact descriptor is an irreducible logical output. Concrete member outputs remain execution evidence and are not copied into the descriptor.

## Required Context

Read these files first:

- `AGENTS.md`
- `PROJECT_STATE.md`
- `docs/concepts/data-asset-collections-and-materialization/README.md`
- Operational Slices 002, 005, and 006
- `cmd/controller/workflow_outputs.go`
- `cmd/controller/workflow_outputs_test.go`
- `cmd/controller/main.go` only to trace the existing work-completion handler into step-output recording
- `cmd/controller/main_test.go` completion-path tests
- `cmd/controller/workflow_dependency_store.go`
- `internal/model/workflow_dependency.go`
- `internal/model/materialized_asset_collection.go`
- `internal/model/data_asset.go`
- `internal/persistence/store.go` methods for step output facts
- persistence tests for workflow step outputs

Do not read worker provider adapters, scheduler, transport, client, publication, or unrelated persistence schema code.

## Allowed Production Files

- `cmd/controller/workflow_outputs.go`
- `cmd/controller/main.go` only if the existing completion handler needs a narrow call-site change
- `cmd/controller/workflow_dependency_store.go` only if existing step-output recording needs a narrow collection call
- `internal/model/materialized_asset_collection.go`
- `internal/model/data_asset.go`
- `internal/model/workflow_dependency.go` only if output-policy metadata is strictly required

## Allowed Test Files

- `cmd/controller/workflow_outputs_test.go`
- `cmd/controller/main_test.go` only for the changed completion call path
- `cmd/controller/workflow_dependency_store_test.go` only if changed
- `internal/model/materialized_asset_collection_test.go`
- `internal/model/data_asset_test.go`
- narrow persistence tests only if existing step-output fact behavior changes

## Allowed Documentation Files

- `docs/concepts/data-asset-collections-and-materialization/007-collection-step-output-synthesis.md`
- `PROJECT_STATE.md` after implementation

## Out Of Scope

- Downstream member hydration.
- Generic path-template resolution.
- Per-member downstream pipelining.
- New collection persistence tables by default.
- Storing all member paths in the collection descriptor.
- Pruning concrete terminal evidence before dependent work is materialized.
- Artifact aggregation changes.
- Ordinary compute fan-out aggregation changes.
- Provider or worker behavior.
- Collection subset semantics.

## Acceptance Criteria

- A one-member scalar materialization step retains a valid object output.
- A three-member collection fixture produces one object, not a JSON list.
- A sixteen-member CDL-shaped fixture remains well below the logical step output size limit.
- The output schema is `goet/materialized-asset-collection/v1`.
- Dimension values are reconstructed deterministically and preserve declared order.
- `path` is one absolute deferred template with placeholders matching `required_bindings`.
- `member_count` equals the expected collection cardinality.
- `members_sha256` is computed from canonical ordered member evidence.
- Work-item completion outputs remain separately persisted.
- Missing member index fails aggregation.
- Duplicate member index fails aggregation.
- Mismatched collection fingerprint fails aggregation.
- Mismatched domain/root fails aggregation.
- A concrete member path that does not conform to the template fails aggregation.
- Different materialization identities resolving to one destination fail aggregation.
- A failed member prevents successful collection output.
- Controller restart can reconstruct the same collection descriptor from durable member facts.
- Ordinary multi-item compute steps still aggregate to lists exactly as before.
- Empty ordinary fan-out behavior remains unchanged.
- `go test ./cmd/controller ./internal/model ./internal/persistence` passes.

## Notes

- The aggregator already reads all member outputs to build an ordinary list; this slice changes the bounded logical result, not the existence of member evidence.
- Hash the ordered minimal evidence, not raw JSON formatting.
- Suggested HCI: `EC-3 / operational slice / files(6)+test+doc`.
