# 018 Data Operator Integration Smoke And Documentation Sync

Status: Proposed

## Objective

Prove the explicit data-operator path end-to-end with fixture-sized data and update documentation/project state.

The smoke shape is:

```text
cache_data fixture input(s)
  -> compute fixture artifact
  -> commit_data fixture published output
```

It must exercise dependency ordering, resource constraints, data manifests, artifact manifests, and published-asset evidence without real CDL, Google Drive, 7z, or HPCC data.

## Current State

The Data Assets concept already plans local and fake-HPCC smokes for data binding, provider materialization, archive extraction, artifact production, and publication.

After slices 014-017, that smoke path must be updated so data movement is explicit:

```text
cache_data -> compute -> commit_data
```

rather than hidden inside compute.

## Target State

A fixture workflow can be submitted and observed with the following behavior:

```text
1. cache_data downloads or references a tiny fixture input.
2. cache_data emits goet/materialized-data-assets/v1.
3. compute waits for cache_data.
4. compute receives materialized input local paths.
5. compute writes a tiny artifact under GOET_ARTIFACT_DIR.
6. compute emits artifact manifest.
7. commit_data waits for compute.
8. commit_data copies selected artifact to a fixture published-data location.
9. commit_data emits goet/published-data-assets/v1.
10. controller status exposes all three work items and compact evidence.
```

## Required Fixture Workflow

Use tiny text/CSV/matrix fixtures, not real rasters.

Example logical shape:

```text
cache_data(crop_lookup_fixture)
cache_data(field_tile_fixture)
cache_data(cdl_tile_fixture)

compute(field_cdl_composition_fixture)
  consumes the three materialized fixture inputs
  emits field_cdl_composition.csv

commit_data(field_cdl_composition_fixture)
  publishes field_cdl_composition.csv to published_data fixture root
```

The compute script may be deliberately simple:

```text
read small field_id matrix
read small crop_code matrix
count crop_code by field_id
write CSV
```

It does not need GDAL.

## Required Resource Smoke

Add a resource-constrained variant:

```text
provider:fixture-http/download target_units=1
target:local/published-data-write:published_data target_units=1
```

The smoke should demonstrate that multiple queued `cache_data` items sharing the same source resource run sequentially when configured.

Do not implement true request-per-minute limits. Sequential/capacity admission is enough for this slice.

## Required Fake HPCC Smoke

If fake HPCC support is already available for the parent concept, add one fake HPCC path after local smoke passes.

The fake HPCC smoke must use:

```text
fixture input root
fixture asset cache root
fixture artifact root
fixture published-data root
container/Singularity worker runtime if already supported
```

It must not use:

```text
real MSU HPCC paths
real Slurm partitions
real Google Drive
real NASS CDL
real Yan/Roy data
private credentials
```

## Documentation Updates

Update:

```text
docs/concepts/data-assets-and-materialized-outputs/README.md
PROJECT_STATE.md
```

The SC README should state:

```text
The preferred large-run shape is explicit cache_data -> compute -> commit_data.
```

The docs should also record:

```text
cache_data is inbound.
commit_data is outbound.
Internal cache promotion is a cache_data phase, not commit_data.
Resource constraints bound concurrent transfers/writes.
Provider transfer limits bound one admitted transfer when supported.
```

## Required Context

Read these files first:

```text
docs/concepts/data-assets-and-materialized-outputs/README.md
docs/concepts/resource-constrained-work-admission/README.md
docs/concepts/dependency-aware-workflows/README.md
docs/concepts/python-workitem/README.md
PROJECT_STATE.md
```

Also read existing demo/smoke/test fixture conventions.

## Allowed Production Files

```text
docs/concepts/data-assets-and-materialized-outputs/README.md
PROJECT_STATE.md
examples/**
demos/**
cmd/**
internal/demo/**
internal/worker/**
internal/workflow/**
```

Only touch production code needed to register/run the fixture smoke path. Prefer docs/examples/test fixtures over broad runtime changes.

## Allowed Test Files

```text
internal/**/*data*_test.go
internal/**/*artifact*_test.go
internal/**/*publish*_test.go
internal/**/*workflow*_test.go
testdata/**
examples/**
demos/**
```

## Out Of Scope

```text
real CDL/Yan/Roy vertical slice
GDAL/rasterio/geospatial container work
real rclone credentials
real Google Drive
real HPCC
cache eviction
data catalog registration
token-bucket rate limits
```

## Acceptance Criteria

- A local fixture workflow exercises `cache_data -> compute -> commit_data`.
- The controller records terminal states for all three operator types.
- The compute work item does not perform remote acquisition or durable publication.
- The `cache_data` output manifest is passed to compute or rendered into compute data-path args.
- The `commit_data` output manifest records published target evidence.
- A constrained fixture shows that source download capacity can serialize multiple `cache_data` items.
- A constrained fixture shows that publish-location write capacity can serialize `commit_data` items.
- Existing tests still pass.
- Documentation and `PROJECT_STATE.md` describe the new data operator path.
- No default test downloads external data or requires credentials.

## Notes

This slice is the concept closure for the explicit operator revision. The real CDL/Yan/Roy runbook should remain a later manual, bounded, site-configured run after the fixture and fake-HPCC paths prove the mechanics.
