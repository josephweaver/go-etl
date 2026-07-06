# 007 Fake HPCC Artifact Smoke Path

Status: proposed

## Objective

Add a repeatable fake-HPCC smoke path that proves materialized artifacts through the configured execution environment: transport, Linux dialect, Slurm scheduler, Singularity worker runtime, worker pull, artifact promotion, and controller completion.

This slice uses tiny fixture files only. It does not use real CDL, Yan/Roy tiles, MSU HPCC, or private configuration.

## Current State

The repository contains fake-HPCC and worker container assets. The container documentation describes a Dockerized Slurm cluster, a Slurm job script, a SingularityCE worker runtime, and a worker that pulls work from the controller.

The controller has execution-environment components for Docker container transport, SSH transport, Bash/Linux dialect behavior, Slurm scheduling, and worker runtime preparation. There are local and fake-HPCC configuration fixtures.

Artifact promotion is proven locally by earlier slices, but not through the fake HPCC boundary.

## Target State

A developer can run one smoke script or runbook from a shell with the required local container tooling. The smoke path should:

1. verify fake-HPCC container prerequisites;
2. build or locate the worker runtime image/SIF expected by the fake environment;
3. start the controller with fake-HPCC execution-environment configuration;
4. submit a tiny source-reference Python workflow that writes one small artifact;
5. let the controller launch worker capacity through the fake Slurm/Singularity path;
6. wait for workflow completion;
7. verify the controller recorded an artifact manifest;
8. verify the promoted artifact exists in the expected fake shared data root;
9. collect controller, worker, Slurm, and smoke logs for debugging;
10. shut down local resources when practical.

## Concept Decision

Use fake HPCC now, but only as a proof of GOET core execution boundaries. Do not use it to prove geospatial throughput yet.

The fake smoke is the bridge between local unit tests and real institutional HPCC. It should stay generic and must not encode real HPCC hostnames, users, queues, partitions, module names, or private filesystem paths.

## Required Context

Read these files first:

- `AGENTS.md`
- `PROJECT_STATE.md`
- `TARGET_STATE.md`
- `docs/concepts/data-assets-and-materialized-outputs/README.md`
- `containers/README.md`
- `containers/goetl-worker/Dockerfile`
- `containers/goetl-worker/test`
- `containers/fake-hpcc-slurm-singularity/test`
- `cmd/controller/controller-default-config.json`
- `cmd/controller/fake-hpcc-ssh-config.json`
- `cmd/controller/local-singularity-config.json`
- `cmd/controller/slurm_scheduler.go`
- `cmd/controller/slurm_worker_script.go`
- `cmd/controller/runtime.go`
- `cmd/controller/ssh_transport.go`
- `cmd/controller/docker_transport.go`
- `scripts/local-singularity/run-demo` if present
- sibling demo project files only if the smoke uses the demo project fixture

Do not read unrelated concept documents unless the smoke path exposes an implementation mismatch.

## Allowed Production Files

- `scripts/fake-hpcc-artifact-smoke.ps1`
- `scripts/fake-hpcc-artifact-smoke.sh`
- `docs/concepts/data-assets-and-materialized-outputs/fake-hpcc-artifact-smoke.md`
- tiny demo-project fixture files in the sibling demo project if that repository is available and intended for workflow fixtures
- `cmd/controller/*config*.json` only for non-private fake/local fixture adjustments
- `containers/*` files only for generic fake-HPCC or worker image fixes exposed by the smoke
- `PROJECT_STATE.md` for a concise current-state note after validation

## Allowed Test Files

None by default. This is a smoke/runbook slice. Add Go tests only if the smoke exposes a narrowly scoped bug in existing execution-environment code.

## Out Of Scope

- Real MSU HPCC configuration.
- Private hostnames, usernames, partitions, account names, module names, or filesystem paths.
- Real CDL downloads.
- Real Yan/Roy data.
- GDAL/rasterio/pyarrow image work.
- Rewriting scheduler, transport, or runtime abstractions.
- General user-facing backend setup UX.

## Acceptance Criteria

- A fake-HPCC artifact smoke runbook exists.
- A script exists when practical for the current development shell environment.
- The smoke uses a tiny artifact-producing workflow.
- The controller launches worker capacity through fake HPCC execution-environment components, not by manually starting a local worker process.
- The worker runs inside the fake Slurm/Singularity path.
- The work item completes and reports an artifact manifest.
- The promoted artifact exists under the fake shared data root.
- The smoke records or prints paths to controller, worker, Slurm, and smoke logs.
- The smoke states exact prerequisites and exact unsupported cases.
- No private institutional configuration is committed.
- `PROJECT_STATE.md` records the validated fake-HPCC artifact path after the smoke is proven.

## Notes

- Keep the fixture boring: a Python script that writes a CSV or text file is enough.
- This slice proves orchestration plumbing, not raster science.
- If the fake environment is too heavy for a developer's local machine, the runbook should say so and document the minimum local checks that still run without it.
