# GORC Install Instructions

These steps build the Go client, controller, and worker from the `go-etl` repo.

## Prerequisites

- Go `1.26.2` or newer, matching [`go.mod`](./go.mod).
- Git.
- A shell that can run the Go toolchain.
- Optional for container builds:
  - Docker.
  - For Singularity-based runs, SingularityCE and a Linux or WSL environment.

## Local Build

From the repository root:

```bash
go build -o .run/goet.exe ./cmd/demo-client
go build -o .run/goetl-controller.exe ./cmd/controller
go build -o .run/goetl-worker.exe ./cmd/worker
```

If you are building on Linux or macOS, you can drop the `.exe` suffix.

This produces:

- `goet` client
- `goetl-controller`
- `goetl-worker`

## Container Build

This repository currently provides container assets for the worker runtime and
the fake-HPCC support image. The controller is still built as a Go binary.

If you want to run the worker in a container, build the worker image:

```bash
docker build -t goetl/worker:dev -f containers/goetl-worker/Dockerfile .
```

If you need the GDAL-enabled worker image:

```bash
docker build -t goetl/worker-gdal:dev -f containers/goetl-worker-gdal/Dockerfile .
```

If you want the local fake-HPCC Slurm plus Singularity image:

```bash
docker build -t goetl/fake-hpcc-slurm-singularity:dev -f containers/fake-hpcc-slurm-singularity/Dockerfile .
```

## Quick Notes

- The worker image entrypoint is `/goetl/goetl-worker`.
- The worker expects a config file at runtime, for example:

```bash
/goetl/goetl-worker /data/goetl/config/worker.json
```

- Container smoke tests are available under `containers/goetl-worker/test` and `containers/fake-hpcc-slurm-singularity/test`.
