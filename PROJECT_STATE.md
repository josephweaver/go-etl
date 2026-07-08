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
- Geospatial worker plugins: [`docs/concepts/geospatial-worker-plugins/README.md`](docs/concepts/geospatial-worker-plugins/README.md). Operational slices `002-geospatial-operation-contract`, `003-raster-info-and-bounding-boxes`, `004-reproject-and-align-raster`, `005-stack-aligned-rasters`, `006-raster-pair-value-counts`, `007-crop-by-polygons`, `008-polygonize-raster`, `009-aggregate-by-polygons`, and `010-cdl-yanroy-fixture-workflow-and-docs` are implemented. `010` adds a GDAL worker smoke fixture workflow (`containers/goetl-worker-gdal/test`) that runs raster metadata, grid validation, align-to-grid, pair counting, and summary CSV generation.
- Dependency-aware workflows: [`docs/concepts/dependency-aware-workflows/STATE.md`](docs/concepts/dependency-aware-workflows/STATE.md)
- Resource-constrained work admission: [`docs/concepts/resource-constrained-work-admission/STATE.md`](docs/concepts/resource-constrained-work-admission/STATE.md)
- Workflow execution persistence: [`docs/concepts/workflow-execution-persistence/STATE.md`](docs/concepts/workflow-execution-persistence/STATE.md)
- Operational observability: [`docs/concepts/operational-observability/STATE.md`](docs/concepts/operational-observability/STATE.md)
- Source-control resolution and cache: [`docs/concepts/source-control-resolution-and-cache/STATE.md`](docs/concepts/source-control-resolution-and-cache/STATE.md)
