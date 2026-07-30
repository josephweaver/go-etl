# 004 Direct-Python DMTCP Feasibility

Status: implemented

## Objective

Add a pinned standalone Python feasibility image and a repeatable institutional
HPCC smoke that determines whether an ordinary Slurm user can use DMTCP under
SingularityCE 4.1.2 to checkpoint, terminate, and restore a directly launched
Python interpreter while it is executing NumPy native code and supervising one
Python descendant process.

This slice is the feasibility gate for selecting DMTCP as the `python_script`
pause adapter. It does not place Go inside DMTCP or implement the production
worker pause/resume lifecycle.

## Current State

OS-001 launched the Go worker as the DMTCP root:

```text
dmtcp_launch goetl-worker execute
    -> Go Worker.Run
        -> python3
```

That probe did not produce a checkpoint for the Go root, and its Python
descendant did not enroll in the DMTCP computation. It rejected DMTCP around
Go; it did not test Python as the direct DMTCP root.

OS-002 proved that privileged local CRIU can restore the Go-plus-Python tree,
but the normal institutional Slurm/Singularity environment lacks the
capabilities required to begin a CRIU dump.

OS-003 proved the narrower direct-interpreter boundary for R:

```text
Go worker outside DMTCP
    -> dmtcp_launch Rscript
        -> R native extensions
        -> optional enrolled Stan descendant
```

Both single-core RStan and CmdStanR passed checkpoint, original-computation
termination, fresh-Singularity restore, and exact baseline comparison on the
institutional target. That proof also found that native numerical thread pools
must be explicitly limited to one for the supported execution shape.

The current standard worker image contains Python 3 and pinned DMTCP 4.2.0, but
it does not pin a scientific Python package environment. The current
`python_script` handler in `cmd/worker/work_python.go` starts Python with
`exec.Command` from inside `Worker.Run`. The existing
`cmd/worker/testdata/dmtcp-checkpoint/source/main.py` fixture proves only that a
Python descendant ran during the failed Go-rooted OS-001 probe; it is not a
direct-Python checkpoint/restore proof.

No repository test currently proves:

- Python as the DMTCP root;
- active CPython state plus a loaded native extension;
- a Python-created descendant enrolled in the same DMTCP computation;
- open-file behavior across Python restore;
- fresh-container restore of that Python process tree; or
- deterministic output equivalence between uninterrupted and resumed Python
  execution.

## Target State

`containers/dmtcp-python-feasibility/Dockerfile` defines a dedicated
feasibility image that:

- pins an amd64 Python 3.11 Bookworm base image by immutable digest;
- builds DMTCP 4.2.0 from the same immutable commit and verified source SHA-256
  used by OS-001 and OS-003;
- installs one explicitly versioned and hash-verified NumPy wheel without
  resolving floating transitive dependencies;
- exposes the exact Python, NumPy, BLAS, compiler, libc, and DMTCP identities;
  and
- contains no GOET worker executable and requires no CRIU capability.

The exact Python patch version, base-image digest, NumPy version, and wheel
SHA-256 are resolved and recorded during implementation. A floating image tag,
unpinned `pip install`, or package upgrade during smoke execution cannot satisfy
this slice.

The image remains separate from `containers/goetl-worker/Dockerfile`. The slice
tests direct Python compatibility without enlarging or changing the production
worker image before the boundary is proven.

The intended process boundary is:

```text
future Go worker, outside DMTCP
    -> isolated DMTCP coordinator
        -> dmtcp_launch python3 fixture.py parent
            +-- deterministic NumPy/OpenBLAS computation
            `-- python3 fixture.py child
```

The fixture uses the current Python work-item environment shape where useful:
an immutable input JSON path, an output JSON path, an owner-only shared
workspace, stdout/stderr files, and deterministic arguments. It does not call
the controller.

The fixture has two ordered probes:

1. **Pure-Python control** - hold live Python state and an open regular file,
   write and flush a pre-checkpoint marker, checkpoint, terminate the original
   computation, restore in a fresh Singularity invocation, release it, and
   validate exactly one post-resume marker and deterministic output.
2. **NumPy plus descendant** - launch a Python parent that starts one Python
   child, loads NumPy, enters a bounded repeated native numerical workload, and
   writes flushed phase markers. After both processes are enrolled and native
   work has started, checkpoint the complete computation, terminate it, restore
   in a fresh Singularity invocation, and validate both parent and child output.

The second probe also runs an uninterrupted baseline from the same input,
arguments, package set, thread limits, and deterministic seed. The resumed
result must match the baseline exactly after excluding timestamps, process IDs,
elapsed time, and absolute runtime paths.

The supported execution shape explicitly sets common native numerical thread
pools to one, including OpenBLAS, OpenMP, MKL, NumExpr, and any NumPy-relevant
thread-control variables discovered during implementation. Multiple native
threads are not inferred from a passing one-core result.

All mutable state uses one owner-only shared directory mounted at the same
absolute destination in original and restore invocations. It contains input,
workspace state, regular-file progress, coordinator state, checkpoint images,
baseline output, resumed output, logs, environment facts, and the
machine-readable result. The smoke uses regular files, not FIFOs.

The smoke records:

- host OS, kernel, architecture, identity, Slurm job and node;
- Singularity version and flags, capability masks, `NoNewPrivs`, and seccomp;
- SIF SHA-256;
- DMTCP, Python, pip, NumPy, BLAS, compiler, and libc versions;
- sorted installed Python package/version output;
- DMTCP coordinator and client lists before checkpoint;
- checkpoint paths, sizes, and hashes;
- original and restore process termination status;
- fixture markers, stdout/stderr, and comparison output; and
- one machine-readable support decision.

The result classification is one of:

```text
pass
blocked_python_runtime
blocked_dmtcp_python_control
blocked_numpy_runtime
blocked_descendant_enrollment
blocked_python_checkpoint
blocked_restore
fixture_too_short
fixture_error
smoke_error
```

A passing institutional result changes OS-004 to `implemented` and permits the
common direct-interpreter DMTCP adapter slice to cover both the proven R and
Python shapes. A reproducible failure changes OS-004 to `blocked` and records
the exact CPython, native-extension, descendant, file-resource, DMTCP, or
container boundary that failed.

No GOET production process, worker protocol, controller state, Slurm signal
handling, resume-artifact persistence, or attempt lifecycle changes in this
slice.

## Concept Decision

This slice adds a standalone direct-Python feasibility concept. It needs its own
container directory because the pinned Python/NumPy runtime and its
checkpoint-specific test surface are independent of the current worker image.

The slice adds one executable smoke and one Python fixture. The smoke owns
container orchestration, bounded waits, DMTCP enrollment, checkpoint
finalization, original-computation termination, fresh-invocation restore,
evidence capture, baseline comparison, cleanup, and result classification. The
fixture owns deterministic Python/NumPy work, descendant creation, progress
markers, open-file state, output generation, and semantic comparison.

The Go worker remains outside DMTCP. A later adapter may prepare the Python
input/output contract and supervise this interpreter boundary, but no Go call
stack or worker session is part of the checkpoint.

## Required Context

Read these files first:

- `docs/concepts/dmtcp-work-item-checkpoint-resume/README.md`
- `docs/concepts/dmtcp-work-item-checkpoint-resume/001-pinned-dmtcp-image-and-go-process-feasibility.md`
- `docs/concepts/dmtcp-work-item-checkpoint-resume/003-r-tidyverse-brms-dmtcp-feasibility.md`
- `containers/dmtcp-r-brms-feasibility/Dockerfile`
- `containers/dmtcp-r-brms-feasibility/dmtcp-smoke`
- `containers/goetl-worker/Dockerfile`
- `cmd/worker/work_python.go`
- `cmd/worker/testdata/dmtcp-checkpoint/source/main.py`

Do not read unrelated controller, persistence, Slurm signal, rclone, manual-Go,
CRIU implementation, GDAL, workflow, provider, or environment-materialization
files unless the approved smoke exposes a direct build or execution failure that
requires them.

## Allowed Production Files

- `containers/dmtcp-python-feasibility/Dockerfile` (new)

## Allowed Test Files

- `containers/dmtcp-python-feasibility/test` (new)
- `containers/dmtcp-python-feasibility/dmtcp-smoke` (new)
- `containers/dmtcp-python-feasibility/testdata/python-checkpoint/fixture.py`
  (new)

## Allowed Documentation Files

- `containers/README.md`
- `PROJECT_STATE.md`
- `docs/TEST_AND_SMOKE_STATUS.md`
- `docs/concepts/README.md`
- `docs/concepts/dmtcp-work-item-checkpoint-resume/README.md`
- `docs/concepts/dmtcp-work-item-checkpoint-resume/004-direct-python-dmtcp-feasibility.md`

## Out Of Scope

- Modifying `goetl-worker`, `Worker.Run`, `work_python.go`, or any Go source or
  Go test.
- Implementing the production pause-adapter interface, interpreter supervisor,
  Slurm signal path, drain timer, controller resume-artifact model, persistence,
  attempt transition, resume limit, or cleanup policy.
- Placing a Go process inside DMTCP.
- Reusing the failed OS-001 `dmtcp_launch goetl-worker execute` boundary as
  evidence for or against direct Python.
- Adding packages to the standard, enhanced-Python, or GDAL worker images.
- Implementing Python environment Data Assets, `.venv` materialization,
  `python-create-env`, dependency installation at work-item execution time, or
  package-index policy.
- Claiming support for arbitrary Python packages, arbitrary native extensions,
  arbitrary child processes, asyncio event loops, sockets, active HTTP/database
  sessions, shared-memory services, GPUs, MPI, or distributed execution.
- Testing more than one Python descendant or more than one numerical thread.
- Testing rclone native continuation, archive tools, R, or manual Go pause
  state.
- Testing protected values or declaring DMTCP images safe for secrets beyond
  recording their required owner-only storage policy.
- Measuring a representative 128-GiB checkpoint or the final ten-minute/
  five-minute production timing budget.
- Publishing the image or SIF to a registry.

## Acceptance Criteria

- The feasibility image uses an immutable base-image digest and fails its build
  if the pinned DMTCP source or NumPy wheel hash does not match.
- The image asserts exact Python, NumPy, and DMTCP versions during build.
- The image and smoke record Python, pip, NumPy, BLAS, compiler, libc, DMTCP,
  platform, architecture, installed-package, image, and SIF identities needed
  to reproduce the run.
- The image test proves Python and NumPy import correctly and invokes the
  ordered DMTCP smoke with bounded waits and cleanup of surviving processes.
- The fixture performs real deterministic NumPy native numerical work. A
  sleep-only, standard-library-only, or synthetic replacement cannot satisfy
  the native-extension probe.
- The fixture launches exactly one Python descendant through Python. The smoke
  records the pre-checkpoint DMTCP client list and requires both parent and
  descendant to be enrolled and running.
- The NumPy checkpoint is requested only after flushed evidence shows that the
  native-work phase has started and before the parent or child completion
  marker exists. Premature completion is `fixture_too_short`, not `pass`.
- Each passing probe produces non-empty finalized checkpoint images for every
  enrolled process, with no remaining `.dmtcp.temp` files.
- The smoke proves the original DMTCP computation and coordinator are gone
  before restore.
- Restore uses `dmtcp_restart` from a separate Singularity invocation with the
  same immutable SIF and stable bind destinations.
- Restore continues the checkpointed parent and child instead of restarting
  the fixture entrypoint. Each passing probe has exactly one pre-checkpoint
  marker per process and one post-resume completion marker per process.
- Open regular-file content and offsets are reconciled without duplicated
  durable markers or output bytes.
- The resumed parent and child outputs are valid, complete, and semantically
  equal to the uninterrupted fixed-seed baseline after excluding explicitly
  named nondeterministic metadata.
- The smoke preserves complete coordinator, launch, checkpoint, termination,
  restart, parent, child, stdout, stderr, comparison, and environment logs.
- The institutional acceptance run uses the normal HPCC account in an actual
  Slurm allocation under SingularityCE 4.1.2 with no `sudo`, `--fakeroot`,
  capability grant, nested Slurm submission, or site security change.
- If the pure-Python control fails, the NumPy/descendant result is not reported
  as compatible.
- A passing institutional result changes OS-004 to `implemented` and permits
  design of the common direct-interpreter DMTCP adapter. It does not approve
  arbitrary Python environments or universal work-item checkpointing.
- A failing institutional result changes OS-004 to `blocked`, preserves
  reproducible evidence, and prevents production Python DMTCP enablement.
- Project and test documentation distinguish the failed Go-rooted OS-001 path,
  the passing direct-R OS-003 path, and this direct-Python result.
- No file outside the allowed production, test, and documentation lists
  changes.

## Notes

- Reuse DMTCP 4.2.0 commit
  `f8009ce7b4ad211311ca2f72a929b975e4aa1155` and source SHA-256
  `3b240c78804bbf1e9354ee3da5c8760c3c952045f71773e9ed490846b15adce0`.
- Resolve and record the immutable Python base digest and NumPy wheel identity
  during implementation rather than inventing them in the charter.
- Keep the Python version on the 3.11 Bookworm line so the feasibility runtime
  remains close to the current Debian Bookworm worker's system Python.
- Use NumPy because it gives the first bounded CPython/native-extension/BLAS
  proof. It does not stand in for later GDAL, PyArrow, PyTorch, or other
  package-specific evidence.
- Use fixed inputs, a fixed random seed, and one native thread so baseline
  comparison is deterministic.
- Make the native operation long enough to checkpoint on the development
  partition but bounded enough for a short Slurm job.
- The child process must be created before the checkpoint trigger and remain
  live until after restore.
- Use DMTCP open-file checkpointing and explicit overwrite behavior only after
  the smoke proves the exact regular-file semantics.
- Wait for finalized checkpoint files; a successful checkpoint-command return
  alone is not proof.
- Construct an explicit `dmtcp_restart` argument vector from discovered
  checkpoint images. Do not execute an unvalidated generated restart script.
- Run the pure-Python control before the expensive native-extension probe and
  retain the first failing phase.
- Local WSL/Docker and local Singularity runs are useful smoke-orchestration
  evidence, but only the normal-account institutional Slurm/Singularity run
  decides the gate.

## Implementation Result

OS-004 passed on 2026-07-30. The implemented feasibility runtime pins:

```text
Python base: python:3.11.15-slim-bookworm
amd64 base manifest: sha256:28255a3ace7eb4c48bc1b57b90af29e1bc82b4fd6c60614a8e3dce61b87ff941
Python: 3.11.15
NumPy: 2.4.6
NumPy wheel SHA-256: 89cd468399cfd2504718f0ba50e410dca55a170b61a02ad92bb18c8a65186e93
DMTCP: 4.2.0
DMTCP commit: f8009ce7b4ad211311ca2f72a929b975e4aa1155
DMTCP source SHA-256: 3b240c78804bbf1e9354ee3da5c8760c3c952045f71773e9ed490846b15adce0
SIF SHA-256: 48c58538c53e6cd5c007362f87f4659783ef1ff675320a050775032d3402a6d7
```

The local WSL Docker proof passed the pure-Python control and the complete
NumPy-plus-descendant test. The parent and child were both present in the
DMTCP client list, the original computation was terminated, a new Docker
invocation restored the two images, and the resumed outputs matched the
uninterrupted baseline exactly. The local normal-user SingularityCE
4.1.2-jammy run then passed the same complete smoke with the exact SIF.

The decisive normal-account institutional run was:

```text
Slurm job: 13966757
Partition: general-short
State/exit: COMPLETED / 0:0
Elapsed: 00:00:30
Compute node: skl-132
CPUs/memory: 2 / 8G
SingularityCE: 4.1.2-jammy
```

Before checkpoint, the coordinator listed two running `python3.11` clients:
the directly launched parent and its Python-created child. The checkpoint set
contained two finalized images with no temporary image remaining:

```text
parent: 251256862 bytes
child:   37035748 bytes
```

The smoke killed the original DMTCP computation, restored both images from a
separate Singularity invocation using the same SIF and bind destination, and
completed all 12 NumPy native-work repetitions. The comparison result was:

```json
{
  "actual_run": "resumed",
  "expected_run": "baseline",
  "match": true,
  "native_repetitions": 12,
  "schema": "goetl/dmtcp-python-compare/v1"
}
```

The baseline and resumed parent files had the same SHA-256, and the baseline
and resumed child files had the same SHA-256. Complete institutional evidence
is retained at:

```text
/mnt/scratch/weave151/etl/runtime/os004-dmtcp-python/evidence/goetl-os004-python.cTJp2Y
```

This result approves DMTCP feasibility for the tested direct CPython 3.11,
NumPy 2.4.6, one-native-thread, one-Python-descendant execution shape. It
permits design of a common externally supervised R/Python DMTCP pause adapter
with Go outside the checkpoint. It does not approve arbitrary Python
environments, arbitrary native extensions or descendants, multiple native
threads, network resources, GPUs, MPI, or universal work-item checkpointing.
No production worker pause adapter, controller resume lifecycle, or Slurm
warning path was implemented by this slice.
