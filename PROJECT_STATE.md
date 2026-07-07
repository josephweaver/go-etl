# Project State

Last updated: 2026-07-07

This is the concise current-state index for GOET. The pre-split root state file is preserved at [`docs/history/PROJECT_STATE_2026-07-07_pre-split.md`](docs/history/PROJECT_STATE_2026-07-07_pre-split.md).

## Current Pointers

- State map and ownership: [`docs/STATE_INDEX.md`](docs/STATE_INDEX.md)
- Current focus: [`docs/CURRENT_FOCUS.md`](docs/CURRENT_FOCUS.md)
- Implemented capabilities: [`docs/IMPLEMENTED_CAPABILITIES.md`](docs/IMPLEMENTED_CAPABILITIES.md)
- Architecture state: [`docs/ARCHITECTURE_STATE.md`](docs/ARCHITECTURE_STATE.md)
- Runtime runbook: [`docs/RUNTIME_RUNBOOK.md`](docs/RUNTIME_RUNBOOK.md)
- Test and smoke status: [`docs/TEST_AND_SMOKE_STATUS.md`](docs/TEST_AND_SMOKE_STATUS.md)
- Development governance: [`docs/DEVELOPMENT_GOVERNANCE.md`](docs/DEVELOPMENT_GOVERNANCE.md)

## Concept State

- Data assets and materialized outputs: [`docs/concepts/data-assets-and-materialized-outputs/STATE.md`](docs/concepts/data-assets-and-materialized-outputs/STATE.md)
- Geospatial worker plugins: [`docs/concepts/geospatial-worker-plugins/README.md`](docs/concepts/geospatial-worker-plugins/README.md). Operational slices `002-geospatial-operation-contract`, `003-raster-info-and-bounding-boxes`, and `004-reproject-and-align-raster` are implemented. The latest update adds `goet-geospatial` support for GDAL-backed `align_to_grid` and `reproject_crs` operations using explicit target grids or `like_raster`, default nearest-neighbor categorical resampling, unsafe-resampling opt-in validation, GeoTIFF and metadata artifact output, and WSL GDAL-tagged test coverage.
- Dependency-aware workflows: [`docs/concepts/dependency-aware-workflows/STATE.md`](docs/concepts/dependency-aware-workflows/STATE.md)
- Resource-constrained work admission: [`docs/concepts/resource-constrained-work-admission/STATE.md`](docs/concepts/resource-constrained-work-admission/STATE.md)
- Workflow execution persistence: [`docs/concepts/workflow-execution-persistence/STATE.md`](docs/concepts/workflow-execution-persistence/STATE.md)
- Operational observability: [`docs/concepts/operational-observability/STATE.md`](docs/concepts/operational-observability/STATE.md)
- Source-control resolution and cache: [`docs/concepts/source-control-resolution-and-cache/STATE.md`](docs/concepts/source-control-resolution-and-cache/STATE.md)
