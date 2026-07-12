# 010 Collection Materialization Smoke and Concept Closure

Status: proposed

## Objective

Add a fixture-sized end-to-end proof of collection `asset.materialize`, compact logical output, downstream member hydration, reuse, and conflict failure; then update current-state documentation and close the Strategic Concept after implementation review.

## Current State

Prior slices establish the models, operation rename, compiler expansion, deterministic worker destinations, controller collection aggregation, downstream hydration, and legacy cleanup.

Unit tests prove individual packages, but the concept is not complete until one repeatable path proves the full boundary with synthetic data.

Real CDL archives are intentionally unsuitable for default tests and local smoke automation.

## Target State

### Fixture

Use three tiny fixture years, for example:

```text
2008
2009
2010
```

Each source is a tiny ZIP containing one deterministic text or CSV member named like:

```text
2008_30m_cdls.tif
```

The bytes need not be a real GeoTIFF. The fixture proves orchestration and filesystem semantics, not raster science.

### Workflow

The fixture project declares:

```text
finite year domain
HTTP or local fixture provider template
ZIP member template
source cache key template
shared destination template cdl/${asset.year}.tif
```

The workflow declares:

```text
step 0: one authored asset.materialize collection step
step 1: fan out over step 0 dimension values
step 1: bind cdl with current year
step 1: read or summarize the concrete data path
```

### Smoke assertions

The smoke verifies:

- three concrete member work items are persisted;
- three deterministic destination files exist;
- each destination has valid pinned evidence;
- step 0 logical output is one object, not a list;
- the output path is one template ending in `cdl/${year}.tif`;
- member count and domain values are correct;
- step 1 receives the correct path for each year;
- a second equivalent run reuses valid source/destination evidence;
- a conflicting pre-existing destination fails safely;
- controller reaches the expected terminal state;
- no external network or large data is required.

### Closure

After the smoke passes:

- update `PROJECT_STATE.md`;
- update `docs/concepts/README.md`;
- update affected canonical document-model status/current-state text;
- mark implemented slice files consistently;
- run implementation review using the shared procedure;
- mark this Strategic Concept `Implemented` only after human acceptance.

## Concept Decision

Use a synthetic fixture that preserves the CDL naming/archive shape without requiring geospatial bytes.

Prefer an automated Go integration test plus one shell/PowerShell smoke script only when the current repository's smoke conventions justify both. Do not build a new general smoke framework.

The closure slice may correct narrowly discovered documentation mismatches. It must not add unplanned runtime behavior.

## Required Context

Read these files first:

- `AGENTS.md`
- `PROJECT_STATE.md`
- `docs/concepts/README.md`
- `docs/concepts/data-asset-collections-and-materialization/README.md`
- all prior slice files in this concept
- `docs/concepts/canonical-workflow-data-document-model/README.md`
- `docs/concepts/data-assets-and-materialized-outputs/README.md`
- `internal/workflow/data_operator_integration_smoke_test.go`
- current data-asset smoke tests and scripts
- current fake-HPCC data-asset runbook and scripts if the smoke reuses them
- sibling demo-project fixture layout only if the existing smoke convention requires it
- `../epistemic-control/procedures/implementation-review.md`
- `../epistemic-control/procedures/ec-scoring.md`

Do not read unrelated concepts or production packages unless the smoke exposes a concrete mismatch.

## Allowed Production Files

None by default.

A runtime change is not part of this closure slice. If the smoke exposes a production defect, stop and create a focused corrective Operational Slice rather than expanding this one.

## Allowed Test Files

- `internal/workflow/data_operator_integration_smoke_test.go`
- focused controller/worker integration test files that already own the explicit data-operator path
- tiny fixture files under existing repository testdata directories
- sibling demo-project fixture files only when the existing test path depends on that repository
- one new focused integration test file if no existing file is an appropriate owner

## Allowed Script and Configuration Files

- `scripts/asset-materialize-collection-smoke.sh` (new, if practical)
- `scripts/asset-materialize-collection-smoke.ps1` (new, if practical)
- fixture project/workflow/submission JSON or YAML
- fake/local controller and worker config only for paths required by the fixture
- no private hostnames, accounts, partitions, credentials, or institutional paths

## Allowed Documentation Files

- `docs/concepts/data-asset-collections-and-materialization/README.md`
- all slice files in this concept for final status notes
- `docs/concepts/data-asset-collections-and-materialization/collection-materialization-smoke.md` (new)
- `docs/concepts/README.md`
- `docs/concepts/canonical-workflow-data-document-model/README.md`
- narrowly affected canonical document-model slice status files
- `docs/concepts/data-assets-and-materialized-outputs/README.md`
- `PROJECT_STATE.md`
- `TARGET_STATE.md` only if the implemented target direction differs from its current text

## Out Of Scope

- Real CDL downloads.
- Real Yan/Roy data.
- GDAL/rasterio/geospatial processing.
- Production HPCC credentials or paths.
- New provider implementation.
- New runtime behavior.
- Invocation-time subsets.
- Per-member pipelining.
- Worker-scope materialization.
- Artifact publication changes.
- Broad documentation cleanup.
- Marking the concept Implemented before review and human acceptance.

## Acceptance Criteria

- A three-year fixture runs without external network access.
- One authored `asset.materialize` step creates three concrete member work items.
- Deterministic destinations exist under the configured fixture materialization root.
- The destinations contain the expected fixture bytes.
- Pinned destination evidence validates.
- Step 0 logical output is a single `goet/materialized-asset-collection/v1` object.
- The logical output contains ordered years 2008, 2009, and 2010.
- The logical output contains one path template with `${year}`.
- The logical output does not contain an ordinary list of three work-item outputs.
- Step 1 fans out over the output domain.
- Each step-1 item receives the matching concrete `data.cdl.path[0]`.
- Re-running the equivalent workflow demonstrates valid reuse without corrupting output.
- A conflicting destination fixture fails without overwrite.
- Restart/recovery coverage proves the collection output and member lookup survive controller reconstruction.
- Current active docs contain no public `cache_data` examples.
- `PROJECT_STATE.md` states the implemented capability without claiming real CDL ingestion.
- The concept index reflects the correct status.
- Narrow tests pass.
- `go test ./...` passes.
- Implementation review records no unresolved blocker.
- The human accepts the implementation before the README status becomes `Implemented`.

## Notes

- Keep fixture contents obviously synthetic.
- Report exact commands and paths used by the smoke.
- The smoke should prove the orchestration contract, not scientific validity.
- Suggested HCI: `EC-3 / operational slice / files(0)+test+doc+config+newfile`.
