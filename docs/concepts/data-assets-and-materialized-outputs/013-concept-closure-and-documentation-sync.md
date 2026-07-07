# 013 Concept Closure and Documentation Sync

Status: proposed

## Objective

Close the first phase of Data Assets and Materialized Outputs after artifact manifests, Python artifact output, data provider/binding declarations, worker fixture materialization, archive extraction, gdrive-rclone adapter, Python data argument binding, published-data copying, fake-HPCC smoke, and CDL/Yan/Roy fixture pipeline slices are complete.

This is a documentation-only closure slice unless documentation reveals a concrete mismatch.

## Current State

The Strategic Concept starts as proposed. Earlier slices should have added:

- shared artifact manifest model and path validation;
- worker artifact staging/promotion;
- Python artifact output integration;
- controller compact artifact output recording;
- named data-location, provider-template, data-binding, archive-selection, cache-policy, integrity, and publish-target models;
- worker-side tiny `local_file`, `http`, and `registered_location` asset materialization/reference;
- immutable cache behavior and integrity verification;
- ZIP archive extraction and selected-member exposure;
- reserved 7z/seven_zip behavior with clear missing-extractor failure;
- `gdrive_rclone` provider using fake-rclone tests;
- Python data path argument binding;
- worker-side published-asset copying to named locations;
- fake-HPCC data/artifact/publication smoke path;
- CDL/Yan/Roy fixture field-composition pipeline smoke.

The README, concept index, `PROJECT_STATE.md`, and smoke runbooks may still describe implemented behavior as planned behavior.

## Target State

Documentation accurately distinguishes:

- implemented first-phase GOET core behavior;
- fixture-only behavior;
- fake-HPCC verification;
- provider support by provider type;
- ZIP extraction support versus reserved/external 7z support;
- `gdrive_rclone` support through a configured executable versus native Google Drive API support;
- materialized attempt artifacts versus published data assets;
- copy-to-named-location publication versus catalog registration;
- real CDL/Yan/Roy work intentionally deferred to data-product runbooks;
- real Google Drive credentials and real institutional HPCC configuration intentionally kept outside the repository;
- future work owned by other concepts.

Preferred status language:

```text
Status: Phase 1 implemented for filesystem-backed artifact manifests, provider-backed data bindings, immutable cache fixtures, ZIP-selected input assets, and named-location publication fixtures
```

Avoid claiming that GOET has a general artifact storage service, a data catalog, arbitrary object-store support, native Google Drive support, full Yan/Roy extraction, or full CDL/Yan/Roy processing unless those capabilities actually exist.

## Concept Decision

Close the concept phase conservatively. The completed capability is the artifact/data-asset orchestration boundary, not the full agricultural data product and not a general data registry.

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
- `README.md` only if it already has a concise current-capability section that should mention artifacts/data assets

## Allowed Test Files

None.

## Out Of Scope

- Go production code changes.
- Go test changes.
- New runtime behavior.
- New data product processing.
- Real HPCC configuration.
- Real CDL/Yan/Roy downloads.
- Real Google Drive credentials or rclone setup.
- Artifact/cache/asset retention implementation.
- Sensitive credential propagation.
- Data catalog implementation.

## Acceptance Criteria

- The Strategic Concept README states the implemented first-phase capability in current-state language.
- Proposed slice statuses are updated consistently.
- Data provider/binding behavior is documented separately from source file admission.
- `http`, `local_file`, `registered_location`, and `gdrive_rclone` support status is documented accurately.
- Cache immutability and integrity behavior is documented accurately.
- ZIP archive extraction support status is documented accurately.
- 7z/seven_zip support is documented accurately as implemented or reserved/external, without overclaiming.
- Published data assets are documented as copy-to-named-location evidence, not automatic registry entries.
- Fake-HPCC smoke status is accurately documented.
- CDL/Yan/Roy fixture status is accurately documented without implying real-data completion.
- The CDL/Yan/Roy product direction is documented as field/CDL composition first, dominant-crop and RCI downstream.
- `docs/concepts/README.md` indexes the concept in the right status section.
- `PROJECT_STATE.md` records the current artifact/data-asset capability without overstating storage, retention, catalog, Google Drive, 7z, or geospatial support.
- Deferred work is explicitly assigned to future concepts or data-product runbooks, including:
  - real CDL/Yan/Roy ingestion;
  - real Google Drive/rclone operational setup and credential handling;
  - geospatial Python environment/image management;
  - artifact/cache/published-data retention cleanup;
  - resource constraints for shared data, network downloads, archive extraction, and cache pressure;
  - sensitive variables for credentialed data;
  - object-store or non-filesystem artifact/publish backends;
  - data catalog or asset registry indexing.
- No runtime code is modified by this slice.

## Notes

- Be explicit that fake HPCC is a proof boundary, not production HPCC support.
- Be explicit that materialized artifact manifests are compact evidence, not an artifact-byte service.
- Be explicit that large data stays out of the source-control cache and SQLite.
- Be explicit that publication copies bytes to a predefined named location; registry/catalog semantics remain future work.
- Be explicit that real rclone credentials live outside GOET workflow files.
