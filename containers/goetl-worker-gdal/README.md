# GDAL Worker Image

This folder defines a dedicated GDAL-enabled worker image used for geospatial
operations.

## Image Layout

- Build stage:
  - compiles `goetl-worker` with `-tags gdal` and CGO enabled,
  - installs `gdal-dev` for future `github.com/airbusgeo/godal` builds.
- Runtime stage:
  - contains the compiled `/goetl/goetl-worker` entrypoint,
  - includes `gdal-bin` so plugin binaries can run GDAL CLI tools.

## Base Images

- Build: `golang:1.26.2-bookworm`
- Runtime: `debian:bookworm-slim`

## Build

```bash
docker build -t goetl/worker-gdal:dev -f containers/goetl-worker-gdal/Dockerfile .
```

## Test

```bash
containers/goetl-worker-gdal/test
```

The test verifies:

- `/goetl/goetl-worker` is present and fails with `invalid config` when no config
  exists.
- `gdalinfo --version` succeeds and prints a GDAL version string.
- `ogrinfo --version` succeeds and prints a version string.
- `goet-geospatial` fixture path runs `raster_info`, `align_to_grid`,
  `raster_pair_value_counts`, and a downstream summary CSV check on tiny local
  rasters.
