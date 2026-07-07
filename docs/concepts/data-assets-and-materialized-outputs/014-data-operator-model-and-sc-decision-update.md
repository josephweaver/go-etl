# 014 Data Operator Model And SC Decision Update

Status: Proposed

## Objective

Revise the Data Assets and Materialized Outputs Strategic Concept to introduce two first-class work-item operators:

```text
cache_data   # inbound materialization of declared input data
commit_data  # outbound publication of produced artifacts
```

This slice is documentation/modeling first. It updates the concept decision, vocabulary, target state, open questions, and proposed slice list. It must not implement provider logic, download throttling, or publish copying yet.

## Current State

The current concept models data movement as worker runtime phases around plugin execution:

```text
resolve input data bindings
acquire or reference source asset
verify integrity
extract selected archive members
run plugin work item
promote declared artifacts
publish selected artifacts
```

The concept also leaves standalone data movement work-item types as a later possibility.

That worked as a phase-1 simplification, but it creates an avoidable large-run ambiguity: when many compute work items use the same large file, each compute work item can appear responsible for acquiring or reusing that input.

## Target State

The Strategic Concept explicitly distinguishes three operator families:

```text
cache_data:
  inbound data movement
  external source or configured named location -> target-local materialized input

compute:
  local transformation
  materialized inputs -> attempt-local artifacts -> promoted artifacts

commit_data:
  outbound data movement
  promoted artifacts -> declared durable store / named publish location
```

The revised concept should say:

```text
Data assets are immutable logical inputs.
Materialized asset instances are target-scoped cached realizations of those inputs.
Published data assets are durable outbound realizations of workflow-produced artifacts.
cache_data and commit_data are first-class work-item operators when data movement crosses a shared target or storage boundary.
```

## Concept Decision

Add a revised decision section equivalent to:

```text
Revised decision: explicit data movement operators

For shared or fan-out workflows, ordinary compute work items must not implicitly
download common inputs or publish durable outputs.

The compiler/planner should produce cache_data work items for resolved input
data assets and commit_data work items for publish bindings. These work items
are scheduled by the same dependency and resource-admission machinery as
ordinary compute work.

Worker-internal materialization may remain as a compatibility path for small
single-step fixtures, but the target state for CDL/Yan/Roy-scale runs is explicit
cache_data -> compute -> commit_data DAG structure.
```

## Required Vocabulary Updates

Add or revise these definitions.

### `cache_data`

A `cache_data` work item materializes one resolved bound data asset for one target environment. It may:

```text
reference an already-mounted registered_location
copy from local_file into the target cache
download HTTP/HTTPS into the target cache
run rclone to copy from a configured Google Drive remote
extract selected archive members
verify expected and observed evidence
emit a materialized-data-assets manifest
```

It is an inbound operation.

### `commit_data`

A `commit_data` work item publishes a selected promoted artifact to a declared durable store location.

It may:

```text
copy to a registered named location
invoke rclone for an outbound Google Drive location if supported later
verify the published copy
enforce overwrite policy
emit published-asset evidence
```

It is an outbound operation.

### Internal cache promotion

Do not use `commit_data` for internal cache finalization. Internal cache finalization remains a phase of `cache_data`:

```text
acquire -> stage -> verify -> promote_cache -> emit manifest
```

## Required Target-State Update

The CDL/Yan/Roy shape should become:

```text
project:
  define cdl_zip(year), yanroy_release(tile/year), crop_lookup providers
  define field_cdl_composition_tile(year,tile) publish target

stage 0:
  cache_data(cdl_zip, year)
  cache_data(yanroy_release, year, tile)
  cache_data(crop_lookup)

stage 1:
  compute(field_cdl_composition, year, tile)
    depends on completed cache_data inputs

stage 2:
  compute(merge per-tile composition outputs)

stage 3:
  compute(dominant-crop assignment)

stage 4:
  compute(RCI transform)

stage 5:
  commit_data(selected published datasets)

stage 6:
  compute or commit_data validation/reporting as needed
```

## Required README Updates

Update the parent SC README:

1. Change the previous phase-1 language that discourages primary `get_data` / `put_data` work item types.
2. Clarify that `cache_data` and `commit_data` are not generic user-authored shell steps. They are GOET-owned data movement operators.
3. Add the added slices 014-018 to the Proposed Slices section.
4. Update Open Questions to remove or reframe the question about whether standalone `materialize_data_asset` and `publish_data_asset` types are worth adding.
5. Add a new open question only if needed:

```text
Whether small local-only workflows may keep implicit worker materialization as a compatibility path, or whether all data bindings should compile through cache_data immediately.
```

## Required Context

Read these files first:

```text
docs/concepts/data-assets-and-materialized-outputs/README.md
docs/concepts/complete/resource-constrained-work-admission/README.md
docs/concepts/complete/dependency-aware-workflow-execution/README.md
docs/concepts/complete/workflow-execution-persistence/README.md
docs/concepts/complete/python-workitem/README.md
```

If a path has moved, use the matching completed concept directory.

## Allowed Production Files

Documentation/modeling only in this slice.

```text
docs/concepts/data-assets-and-materialized-outputs/README.md
docs/concepts/data-assets-and-materialized-outputs/014-data-operator-model-and-sc-decision-update.md
docs/concepts/data-assets-and-materialized-outputs/README-data-operator-addendum.md
PROJECT_STATE.md
```

Do not edit Go production code in this slice.

## Allowed Test Files

None required.

## Out Of Scope

```text
new database tables
new work item operator enum
provider execution
rclone support
HTTP transfer throttling
artifact publication implementation
fake HPCC smoke changes
```

## Acceptance Criteria

- The parent SC explains `cache_data`, `compute`, and `commit_data`.
- The parent SC no longer implies that compute work items are responsible for shared remote acquisition.
- `commit_data` is clearly defined as outbound publication, not internal input-cache finalization.
- The parent SC states that internal cache finalization is part of `cache_data`.
- The added slices 014-018 are listed or referenced.
- Existing slice numbers 001-013 remain intact.
- No implementation files are changed in this slice.

## Notes

This slice intentionally reopens a decision from the current SC. The driver is CDL/Yan/Roy-scale execution, where shared assets and publish targets must be coordinated by the controller rather than hidden inside each compute worker.
