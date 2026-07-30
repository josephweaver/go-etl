# 001 Pinned DMTCP Image and Go-Process Feasibility

Status: blocked

## Objective

Pin DMTCP `v4.2.0` in the standard GOET worker image, build the existing
`goetl-worker` executable so DMTCP can inject its runtime, and add a repeatable
smoke that determines whether DMTCP can checkpoint, terminate, and restart the
actual `worker execute` Go process together with its Python descendant.

This slice is a feasibility gate. A successful result permits later
checkpoint-persistence and worker-supervisor slices to proceed. A reproducible
failure is also valid evidence, but it stops the current implementation path
and requires revision of the Strategic Concept before another implementation
slice is designed.

## Current State

`containers/goetl-worker/Dockerfile`:

- builds the standard worker from `golang:1.26.2-bookworm`;
- sets `CGO_ENABLED=0`;
- copies the resulting worker into `debian:bookworm-slim`;
- installs `ca-certificates` and `python3`; and
- does not install DMTCP.

The resulting standard worker is not built for DMTCP's normal dynamic-loader
injection path.

`containers/goetl-worker/test` currently:

- builds `goetl/worker:dev`;
- starts the image with a missing worker config; and
- verifies that the worker reports the missing config.

It does not inspect dynamic linking or exercise `dmtcp_launch`,
`dmtcp_command`, or `dmtcp_restart`.

`cmd/worker/direct.go` already provides:

```text
worker execute
  --config PATH
  --work-item PATH
  --source-bundle PATH
  --result PATH
```

The command runs the real `Worker.Run` dispatch in a one-shot GOET process,
uses no controller lifecycle API, and writes a
`gorc/worker-direct-result/v1` result. A Python work item launched through this
command gives the feasibility smoke both:

```text
the compiled Go process
    -> its Python descendant
```

without first adding the future production parent/child supervisor.

The repository fake-HPCC image verifies SingularityCE 4.1.2 on Rocky Linux 9.
The first institutional target is SingularityCE 4.1.2 on Ubuntu Jammy. There is
no recorded GOET DMTCP checkpoint/restart result for either environment.

## Target State

`containers/goetl-worker/Dockerfile`:

- downloads DMTCP release `v4.2.0` from an immutable upstream release URL;
- verifies a checked-in expected SHA-256 before building it;
- installs only the DMTCP runtime files needed by the final image;
- builds `goetl-worker` as a dynamically linked Linux `amd64` executable
  compatible with the pinned DMTCP runtime; and
- makes the exact DMTCP version observable through
  `dmtcp_launch --version`.

`containers/goetl-worker/test` retains its current missing-config assertion and
also invokes a focused DMTCP smoke.

A new checkpoint fixture runs the existing `worker execute` command under one
isolated DMTCP coordinator. The Python descendant:

1. writes and flushes one pre-checkpoint marker;
2. waits for a test-controlled continuation file;
3. writes a valid `GOET_OUTPUT_JSON`; and
4. writes one post-resume marker.

The smoke:

1. builds or uses the standard worker image;
2. verifies the worker executable is dynamically linked;
3. verifies DMTCP reports exactly `v4.2.0`;
4. launches the direct worker with an isolated coordinator and a mounted
   checkpoint/work directory;
5. waits for the pre-checkpoint marker;
6. requests one checkpoint and verifies that the checkpoint set covers the Go
   worker and Python descendant;
7. terminates the original DMTCP computation;
8. starts a new container invocation against the same mounted directory;
9. restarts the complete checkpoint set;
10. releases the fixture continuation condition; and
11. verifies one completed direct result, one pre-checkpoint marker, one
    post-resume marker, and the expected logical output.

The same built image or SIF is used for initial execution and restart. The
smoke invokes `dmtcp_restart` with an explicit checkpoint-file argument list;
it does not establish generated restart-shell execution as a production
contract.

The image is also exercised under SingularityCE 4.1.2 on the Ubuntu Jammy
target. Exact command, image identity, DMTCP version, success/failure, and
checkpoint/restart evidence are recorded in project test documentation.

No controller, worker-session, Slurm signal, pending-resume, or production
work-item subprocess behavior changes in this slice.

## Concept Decision

This slice updates the existing standard worker-image concept in
`containers/goetl-worker/Dockerfile`.

It adds one independent test concept: a cross-process DMTCP feasibility smoke.
That concept deserves its own executable test script and fixture directory
because it owns coordinator lifecycle, checkpoint creation, original-process
termination, restart, and continuation verification. Those responsibilities
are separate from the existing minimal image-start assertion.

The slice deliberately uses the existing development-only `worker execute`
boundary as a probe. It does not promote that command to the future production
worker-child protocol.

## Required Context

Read these files first:

- `docs/concepts/dmtcp-work-item-checkpoint-resume/README.md`
- `containers/goetl-worker/Dockerfile`
- `containers/goetl-worker/test`
- `cmd/worker/direct.go`
- `cmd/worker/testdata/direct-python/README.md`

Do not read unrelated controller, persistence, Slurm, GDAL, or work-item
implementation files unless the approved smoke exposes a direct build or test
failure that requires them.

## Allowed Production Files

- `containers/goetl-worker/Dockerfile`

## Allowed Test Files

- `containers/goetl-worker/test`
- `containers/goetl-worker/dmtcp-smoke`
- `cmd/worker/testdata/dmtcp-checkpoint/work-item.json`
- `cmd/worker/testdata/dmtcp-checkpoint/source/main.py`

## Allowed Documentation Files

- `containers/README.md`
- `PROJECT_STATE.md`
- `docs/TEST_AND_SMOKE_STATUS.md`
- `docs/concepts/dmtcp-work-item-checkpoint-resume/001-pinned-dmtcp-image-and-go-process-feasibility.md`

## Out Of Scope

- Modifying `containers/goetl-worker-gdal/Dockerfile` or its tests.
- Adding the production parent-worker/one-shot-child supervisor.
- Changing `cmd/worker/direct.go` or another Go production file.
- Adding worker drain signals, timers, process-group supervision, or
  administrative drain.
- Adding controller checkpoint APIs, persistence, pending-resume state,
  execution lineage, new-attempt resume claims, or resume limits.
- Generating Slurm `#SBATCH --signal` directives or testing Slurm signal
  propagation.
- Defining the final shared-temp UUID layout or checkpoint manifest.
- Claiming compatibility with every work-item type, rclone, HTTP transfer,
  archive extraction, GDAL, MPI, GPU, or a 128 GiB process.
- Selecting the production `--ckpt-open-files` policy from this one fixture
  alone.
- Executing a generated DMTCP restart shell script as a production mechanism.

## Acceptance Criteria

- The standard worker image build uses immutable DMTCP `v4.2.0` source and
  fails when its expected SHA-256 does not match.
- `dmtcp_launch --version` in the built image reports `v4.2.0`.
- The image test proves `/goetl/goetl-worker` is dynamically linked and can
  load the pinned DMTCP runtime.
- The existing missing-config container assertion remains green.
- The fixture enters the real `Worker.Run` Python dispatch through
  `worker execute`; no synthetic non-GOET executable replaces the Go process.
- One isolated DMTCP computation checkpoints the Go worker and its Python
  descendant after the pre-checkpoint marker is durable.
- The original computation is terminated before restart.
- A separate container invocation using the same image and mounted state
  restarts the complete checkpoint set.
- The restarted computation writes a completed
  `gorc/worker-direct-result/v1` document with non-empty completion evidence
  and the expected logical output.
- The pre-checkpoint and post-resume markers each occur exactly once, proving
  continuation rather than a fresh script start.
- The Docker smoke is repeatable from
  `containers/goetl-worker/test`.
- SingularityCE 4.1.2 on Ubuntu Jammy runs the same checkpoint/kill/restart
  proof, or the slice records a reproducible incompatibility with exact command
  output and stops the Strategic Concept implementation path.
- `PROJECT_STATE.md`, `containers/README.md`, and
  `docs/TEST_AND_SMOKE_STATUS.md` state the exact image, DMTCP, linking,
  container-runtime, and checkpoint/restart evidence without claiming later
  controller or all-work-item behavior.
- No production file outside the allowed file list changes.

## Notes

- Resolve and record the SHA-256 for the exact `v4.2.0` source archive during
  implementation. Do not use a floating branch, `latest` URL, or unverified
  download.
- Keep DMTCP build tools out of the final image unless a runtime component
  requires them.
- Use a new coordinator on a dynamically assigned private port. Do not join the
  default coordinator on port 7779.
- Keep checkpoint files, fixture controls, logs, and direct result under one
  mounted test root so a new container invocation can see them.
- Use bounded polling with diagnostic output. The smoke must not contain an
  unbounded wait for a marker, checkpoint, container exit, restart, or result.
- Verify that the original process is gone before restart so a passing result
  cannot come from the pre-checkpoint computation continuing in the
  background.
- Treat a reproducible Go/DMTCP restore failure as decision evidence, not as a
  reason to broaden this slice with another checkpoint backend or a worker
  architecture rewrite.

## Implementation Result

The feasibility gate failed reproducibly on 2026-07-28. Do not implement later
Strategic Concept slices until the architecture is revised.

The standard worker image successfully:

- built DMTCP `v4.2.0` from commit
  `f8009ce7b4ad211311ca2f72a929b975e4aa1155`;
- verified source-archive SHA-256
  `3b240c78804bbf1e9354ee3da5c8760c3c952045f71773e9ed490846b15adce0`;
- reported `dmtcp_launch (DMTCP) 4.2.0`;
- built `/goetl/goetl-worker` as a dynamically linked Linux `amd64`
  executable; and
- checkpointed a control `/bin/sleep` process into one 20,520,960-byte image
  using the same DMTCP image and Docker execution settings.

The real `goetl-worker execute` computation did not checkpoint:

- Docker and Singularity each reported exactly one coordinator client,
  `goetl-worker`;
- the running Python descendant wrote the durable pre-checkpoint marker but
  never appeared in the DMTCP client list;
- a blocking coordinator checkpoint request returned without creating a Go or
  Python checkpoint image; and
- DMTCP logged that the application tried to install a handler for its
  checkpoint signal. Selecting test-only signal `35` did not make the Go
  process checkpoint.

Docker additionally required `--pid host`; DMTCP failed during initialization
inside Docker's private PID namespace while opening `/proc/self/stat`.

The same Go-specific result occurred under:

```text
singularity-ce version 4.1.2-jammy
WSL host: Ubuntu 24.04.4 LTS
SIF SHA-256: fc593f8ed3d7c33e7ba6742f3f18cbd79e3596786e3e0b64ad63a9711ca7edf3
```

This is exact local runtime-package evidence, not evidence from the
institutional Ubuntu Jammy host. The institutional run was not attempted
because the generic Go/process-tree feasibility gate had already failed in
both local container runtimes.

The evidence indicates two independent incompatibilities with the proposed
transparent boundary:

1. the Go runtime's signal handling prevents the enrolled Go process from
   producing a checkpoint; and
2. a Python process created through Go's `os/exec` path does not automatically
   join the DMTCP computation.

`containers/goetl-worker/dmtcp-smoke` retains the full intended
checkpoint/kill/restart proof and exits nonzero at the missing checkpoint-set
assertion. This is an intentional blocked-gate result, not passing
checkpoint/resume support.
