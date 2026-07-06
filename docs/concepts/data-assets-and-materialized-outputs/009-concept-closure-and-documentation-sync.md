# 009 Concept Closure and Documentation Sync

Status: proposed

## Objective

Close the first phase of Data Assets and Materialized Outputs after artifact manifests, Python artifact output, data asset declarations, worker fixture materialization, fake-HPCC smoke, and CDL/Yan/Roy fixture pipeline slices are complete.

This is a documentation-only closure slice unless documentation reveals a concrete mismatch.

## Current State

The Strategic Concept starts as proposed. Earlier slices should have added:

- shared artifact manifest model and path validation;
- worker artifact staging/promotion;
- Python artifact output integration;
- controller compact artifact output recording;
- data asset declaration models;
- worker-side tiny asset materialization;
- fake-HPCC artifact smoke path;
- CDL/Yan/Roy fixture pipeline smoke.

The README, concept index, `PROJECT_STATE.md`, and smoke runbooks may still describe implemented behavior as planned behavior.

## Target State

Documentation accurately distinguishes:

- implemented first-phase GOET core behavior;
- fixture-only behavior;
- fake-HPCC verification;
- real CDL/Yan/Roy work intentionally deferred to data-product runbooks;
- real institutional HPCC configuration intentionally kept outside the repository;
- future work owned by other concepts.

Preferred status language:

```text
Status: Phase 1 implemented for filesystem-backed artifact manifests and fixture data assets
```

Avoid claiming that GOET has a general artifact storage service or full CDL/Yan/Roy processing unless those capabilities actually exist.

## Concept Decision

Close the concept phase conservatively. The completed capability is the artifact/data-asset orchestration boundary, not the full agricultural data product.

## Required Context

Read these files first:

- `AGENTS.md`
- `PROJECT_STATE.md`
- `TARGET_STATE.md`
- `docs/concepts/README.md`
- `docs/concepts/data-assets-and-materialized-outputs/README.md`
- all slice files in `docs/concepts/data-assets-and-materialized-outputs/`
- smoke/runbook documents created by this concept
- `docs/concepts/dependency-aware-workflows/README.md`
- `docs/concepts/python-workitem/README.md`
- `docs/concepts/source-control-resolution-and-cache/README.md`
- `docs/concepts/workflow-execution-persistence/README.md`
- `docs/concepts/resource-constraint/README.md`
- `docs/concepts/sensitive-variable-propagation/README.md`

Do not read unrelated runtime files unless documentation status cannot be reconciled without checking implementation.

## Allowed Production Files

- `docs/concepts/data-assets-and-materialized-outputs/README.md`
- slice files in `docs/concepts/data-assets-and-materialized-outputs/`
- smoke/runbook files in `docs/concepts/data-assets-and-materialized-outputs/`
- `docs/concepts/README.md`
- `PROJECT_STATE.md`
- `TARGET_STATE.md` only for a narrow current-target update if the product direction changed
- `README.md` only if it already has a concise current-capability section that should mention artifacts

## Allowed Test Files

None.

## Out Of Scope

- Go production code changes.
- Go test changes.
- New runtime behavior.
- New data product processing.
- Real HPCC configuration.
- Real CDL/Yan/Roy downloads.
- Artifact retention implementation.
- Sensitive credential propagation.

## Acceptance Criteria

- The Strategic Concept README states the implemented first-phase capability in current-state language.
- Proposed slice statuses are updated consistently.
- Fake-HPCC smoke status is accurately documented.
- CDL/Yan/Roy fixture status is accurately documented without implying real-data completion.
- `docs/concepts/README.md` indexes the concept in the right status section.
- `PROJECT_STATE.md` records the current artifact/data-asset capability without overstating storage, retention, or geospatial support.
- Deferred work is explicitly assigned to future concepts or data-product runbooks, including:
  - real CDL/Yan/Roy ingestion;
  - geospatial Python environment/image management;
  - artifact retention cleanup;
  - resource constraints for shared data and network downloads;
  - sensitive variables for credentialed data;
  - object-store or non-filesystem artifact backends.
- No runtime code is modified by this slice.

## Notes

- Be explicit that fake HPCC is a proof boundary, not production HPCC support.
- Be explicit that materialized artifact manifests are compact evidence, not an artifact-byte service.
- Be explicit that large data stays out of the source-control cache and SQLite.
