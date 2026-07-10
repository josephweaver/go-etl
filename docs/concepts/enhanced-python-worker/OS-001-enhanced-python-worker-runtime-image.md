# OS-001: Enhanced Python Worker Runtime Image

Status: Proposed

## Purpose

Define and build the first enhanced Python worker runtime that can execute
Python work items with common scientific, geospatial, and archive dependencies.

## Scope

This slice should produce a repeatable local image or runtime build path. It
does not need to publish a production registry artifact.

## Requirements

The runtime must include at least:

```text
goet worker executable
python3
python3 venv support
pip or the selected first package installer
numpy
GDAL command-line tools
Python osgeo.gdal bindings
7z/7za/7zr
basic POSIX shell utilities used by worker scripts
```

## Configuration Boundary

The image should not bake workflow-specific source files or private credentials.
The environment root is mounted at runtime and is not part of the image.

## Validation

Add a smoke command that runs inside the image and prints versions for:

```text
python3
pip or installer
numpy import
osgeo.gdal import
gdalinfo
7z/7za/7zr
goet worker
```

## Stop Conditions

- Python GDAL bindings cannot be installed consistently with the GDAL CLI.
- The image requires credentials or site-private paths to build.
- The runtime cannot execute the existing no-env `python_script` smoke.

## Completion Criteria

- A documented build command exists.
- The image passes the dependency smoke.
- Existing Python work-item behavior still works when no environment selection
  is requested.
