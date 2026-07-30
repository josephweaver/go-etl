# 003 R Tidyverse and BRMS DMTCP Feasibility

Status: implemented

## Objective

Add a pinned, standalone R feasibility image and a repeatable institutional
HPCC smoke that determines whether an ordinary Slurm user can use DMTCP under
SingularityCE 4.1.2 to checkpoint, terminate, and restore an `Rscript`
computation that performs tidyverse preprocessing and active BRMS sampling.

Test BRMS with one chain and one core under both supported execution shapes:
the `rstan` backend, whose sampler remains in the R process for this
configuration, and the `cmdstanr` backend, which starts a compiled Stan
executable as a descendant process.

This slice is a feasibility gate. A passing backend permits a later
Operational Slice to design the Go worker as an external supervisor for that
R execution shape. A reproducible failure is also valid evidence, but it must
identify the exact R, Stan, DMTCP, Singularity, filesystem, or process-resource
boundary that failed.

## Current State

OS-001 established that pinned DMTCP `v4.2.0` can checkpoint a native control
process, but cannot checkpoint the existing Go-rooted execution shape:

```text
dmtcp_launch goetl-worker execute
    -> Go Worker.Run
        -> python3
```

The Go root did not produce a checkpoint image and its Python descendant did
not enroll in the DMTCP computation. That result rejects DMTCP around the Go
worker; it does not test an interpreter launched directly by DMTCP.

OS-002 established that privileged local CRIU can restore the Go-plus-Python
tree, but that the institutional normal-user Slurm/Singularity environment
lacks the capabilities required to begin a CRIU dump. DMTCP remains attractive
for a narrower process boundary because it does not require the CRIU
capabilities denied by the site.

The repository currently has:

- pinned DMTCP `v4.2.0` build and control-process evidence in
  `containers/goetl-worker/Dockerfile` and
  `containers/goetl-worker/dmtcp-smoke`;
- no R work-item type;
- no R worker image;
- no checked-in tidyverse or BRMS runtime; and
- no evidence for checkpointing R, R native extensions, Stan sampling, or a
  CmdStan descendant under the institutional runtime.

The intended experimental process boundary is:

```text
future Go worker, outside the checkpoint
    -> DMTCP coordinator, outside the checkpointed R process
        -> dmtcp_launch Rscript fixture.R
            -> tidyverse preprocessing
            -> BRMS
                +-- rstan: sampler in the single R process
                `-- cmdstanr: external compiled Stan descendant
```

The Go worker, controller communication, Slurm warning handling, and any future
runner service are not part of the DMTCP computation in this slice.

## Target State

`containers/dmtcp-r-brms-feasibility/Dockerfile` defines a dedicated
feasibility image that:

- pins its Linux/R base image by immutable digest;
- builds the same immutable DMTCP `v4.2.0` source used by OS-001 and verifies
  its expected source SHA-256;
- installs R, tidyverse, BRMS, RStan, CmdStanR, CmdStan, and the native
  build/runtime libraries needed by the fixture;
- uses an immutable package-repository snapshot and explicitly selected
  tidyverse, BRMS, RStan, CmdStanR, and CmdStan versions;
- makes the exact R, package, Stan, compiler, libc, and DMTCP versions
  observable; and
- contains no GOET worker executable and requires no CRIU capability.

The image is intentionally separate from `containers/goetl-worker/Dockerfile`.
The slice tests R checkpoint compatibility without enlarging the production
worker image or adding an R plugin before the backend is proven.

The fixture performs deterministic tidyverse preprocessing, fits a small
BRMS model with a fixed seed, and writes canonical posterior draws plus a
bounded result summary to regular files on a shared mounted path. Model
compilation and cache preparation occur before the checkpoint proof. The
checkpoint is requested only after evidence shows that Stan sampling has
started and before the expected draws are complete.

The smoke runs these ordered probes:

1. **R/tidyverse control** - launch `Rscript` directly with `dmtcp_launch`,
   transform deterministic data, checkpoint after a flushed marker, terminate
   the original process, restore with `dmtcp_restart` in a fresh Singularity
   invocation, and verify continuation and output.
2. **BRMS with `rstan`** - fit the prepared model with one chain, one core, and
   no Stan threading; checkpoint during active sampling, terminate the
   original DMTCP computation, restore it in a fresh Singularity invocation,
   and verify the completed fit and canonical draws.
3. **BRMS with `cmdstanr`** - repeat the proof with one chain and one core,
   additionally proving that the live compiled Stan executable is enrolled in
   the same DMTCP computation and restored with its R parent.

Each BRMS probe also runs an uninterrupted baseline from the same prepared
model, input, seed, and package set. A passing result compares canonical
posterior draw values from the resumed run with that baseline, excluding
timestamps, elapsed-time comments, absolute cache paths, and other
non-algorithmic metadata.

All mutable state uses one owner-only shared directory mounted at the same
absolute destination in the original and restore invocations. That directory
contains the fixture input, prepared model artifacts, logs, DMTCP coordinator
state, checkpoint images, baseline output, resumed output, and the
machine-readable summary. The smoke uses regular files, not FIFOs.

The institutional run:

- executes as the normal HPCC account in an actual Slurm allocation;
- uses SingularityCE 4.1.2 with no `sudo`, `--fakeroot`, capability grant, or
  host configuration change;
- records the Slurm job/node, host OS/kernel/architecture, Singularity version
  and flags, SIF SHA-256, DMTCP version, R/package/Stan/compiler/libc versions,
  effective identity, capability masks, and `NoNewPrivs`; and
- preserves full coordinator, launch, checkpoint, restart, R, and Stan logs.

The smoke emits one machine-readable result with an overall decision and
separate results for the control, `rstan`, and `cmdstanr` probes:

```text
pass
blocked_r_runtime
blocked_dmtcp_r_control
blocked_rstan_checkpoint
blocked_cmdstanr_enrollment
blocked_cmdstanr_checkpoint
blocked_restore
fixture_too_short
fixture_error
smoke_error
```

At least one BRMS backend must pass the full institutional proof for OS-003 to
be implemented. The summary records the backend-specific support matrix; a
passing backend does not imply that the other backend, parallel chains, Stan
threading, or arbitrary R packages are supported.

No GOET production process, worker protocol, work-item type, controller state,
Slurm signal handling, or checkpoint persistence behavior changes in this
slice.

## Concept Decision

This slice adds a standalone R/BRMS checkpoint-feasibility concept. It needs
its own container directory because its R, Stan, compiler, and package-lock
surface is independent of the current Go worker image and is too large to add
to that image before feasibility is established.

The slice adds one executable smoke and one R fixture. The smoke owns container
orchestration, bounded waits, DMTCP process enrollment, checkpoint/termination,
fresh-invocation restore, evidence capture, and result classification. The R
fixture owns deterministic tidyverse preprocessing, backend selection, BRMS
sampling, progress markers, canonical draws, and result validation.

This slice narrows the earlier universal checkpoint boundary. DMTCP owns only
the direct `Rscript` computation and its enrolled descendants. A future Go
worker may supervise that computation, but the Go process itself must remain
outside DMTCP unless later evidence reverses OS-001.

Testing both BRMS backends is deliberate. They have materially different
process trees, and choosing one without evidence would hide the principal
checkpointing risk.

## Required Context

Read these files first:

- `docs/concepts/dmtcp-work-item-checkpoint-resume/README.md`
- `docs/concepts/dmtcp-work-item-checkpoint-resume/001-pinned-dmtcp-image-and-go-process-feasibility.md`
- `docs/concepts/dmtcp-work-item-checkpoint-resume/002-hpcc-criu-capability-and-go-process-tree-feasibility.md`
- `containers/goetl-worker/Dockerfile`
- `containers/goetl-worker/dmtcp-smoke`
- `containers/README.md`

Do not read unrelated controller, persistence, workflow, provider, CRIU,
signal, or GDAL implementation files unless the approved smoke exposes a
direct build or execution failure that requires them.

## Allowed Production Files

- `containers/dmtcp-r-brms-feasibility/Dockerfile` (new)

## Allowed Test Files

- `containers/dmtcp-r-brms-feasibility/test` (new)
- `containers/dmtcp-r-brms-feasibility/dmtcp-smoke` (new)
- `containers/dmtcp-r-brms-feasibility/testdata/brms-checkpoint/fixture.R`
  (new)

## Allowed Documentation Files

- `containers/README.md`
- `PROJECT_STATE.md`
- `docs/TEST_AND_SMOKE_STATUS.md`
- `docs/concepts/README.md`
- `docs/concepts/dmtcp-work-item-checkpoint-resume/README.md`
- `docs/concepts/dmtcp-work-item-checkpoint-resume/003-r-tidyverse-brms-dmtcp-feasibility.md`

## Out Of Scope

- Adding an R work-item type, R plugin, or general R execution contract.
- Modifying `goetl-worker`, `Worker.Run`, or any Go source or Go test.
- Placing a Go process inside the DMTCP computation.
- Implementing the lightweight-worker/reusable-work-item-container topology or
  worker-to-runner communication.
- Adding Slurm warning signals, drain timers, checkpoint manifests, controller
  APIs, attempt lineage, pending-resume state, or resume limits.
- Checkpointing model compilation, C++ compilation, or linking.
- Testing more than one chain, more than one core, Stan threading, MPI, GPU
  state, fork clusters, `future`, or nested parallelism.
- Claiming support for arbitrary tidyverse packages, BRMS models, Stan custom
  code, R packages, native extensions, open network connections, or external
  data services.
- Testing live database, HTTP, rclone, controller, or package-repository
  connections.
- Measuring representative 128-GiB checkpoint size or the production
  ten-minute/five-minute deadline budget.
- Modifying the standard or GDAL worker images.
- Removing CRIU or DMTCP evidence from OS-001 or OS-002.
- Selecting the production checkpoint manifest, security, retention, or
  compatibility policy.

## Acceptance Criteria

- The feasibility image is based on an immutable image digest and fails its
  build if the pinned DMTCP source hash does not match.
- The Dockerfile uses an immutable package-repository snapshot plus explicitly
  selected tidyverse, BRMS, RStan, CmdStanR, and CmdStan versions. The test
  records the complete resolved R package/version manifest and SIF SHA-256.
- The image reports DMTCP `4.2.0` and records the exact R, Stan, compiler,
  libc, image, and package facts needed to reproduce the run.
- The container test proves that all required R packages load, the prepared
  BRMS model can run with each backend, and the smoke has bounded waits and
  cleans up its own surviving processes on success or failure.
- The R fixture performs a real tidyverse transformation and a real
  `brms::brm()` fit; a sleep-only or synthetic non-BRMS replacement cannot
  satisfy the BRMS probes.
- Compilation completes before each checkpoint proof begins.
- Each BRMS checkpoint is requested only after backend-specific evidence shows
  active sampling and incomplete expected output. A fixture that completes
  before checkpoint is classified `fixture_too_short`, not `pass`.
- Before each checkpoint, the smoke records the live DMTCP client set. The
  `rstan` probe includes the launched R process; the `cmdstanr` probe includes
  both the R parent and its compiled Stan descendant.
- Each passing probe produces non-empty checkpoint images, proves the original
  DMTCP computation is gone, and restores through `dmtcp_restart` from a
  separate Singularity invocation using the same SIF and stable mount
  destinations.
- Restore continues the checkpointed computation rather than starting
  `fixture.R` from its beginning. Exactly one pre-checkpoint marker and one
  post-restore completion marker exist for each passing probe.
- Each passing BRMS probe produces a valid `brmsfit`, the expected formula,
  chain/iteration counts, finite parameter summaries, and the expected number
  of posterior draws.
- Canonical posterior draw values from each passing resumed fit match its
  uninterrupted fixed-seed baseline. Timing and path metadata are not part of
  this comparison.
- The institutional acceptance run uses the normal HPCC account in an actual
  Slurm allocation under SingularityCE 4.1.2 with no privilege elevation or
  site security change.
- The institutional evidence includes every environment and version fact
  named in the target state plus complete DMTCP, R, and Stan logs.
- If the R/tidyverse control fails, neither BRMS result is reported as
  compatible. If one BRMS backend fails, the other backend may still pass but
  the support matrix preserves the failure classification.
- A passing institutional result for at least one BRMS backend changes OS-003
  to `implemented` and permits design of an R-specific worker-supervisor
  slice. It does not approve universal work-item checkpointing.
- If neither backend passes, OS-003 changes to `blocked` with reproducible
  evidence and no R-specific supervisor slice proceeds.
- Project and test documentation distinguish the failed Go-rooted DMTCP path,
  the blocked institutional CRIU path, and this direct-R DMTCP result.
- No file outside the allowed production, test, and documentation lists
  changes.

## Implementation Result

OS-003 passed on 2026-07-30. DMTCP can checkpoint and restore the scoped
single-chain, single-core tidyverse-plus-BRMS workload when `Rscript` is the
DMTCP root and the Go worker remains outside the DMTCP computation.

The feasibility runtime is defined by:

- `containers/dmtcp-r-brms-feasibility/Dockerfile`;
- `containers/dmtcp-r-brms-feasibility/test`;
- `containers/dmtcp-r-brms-feasibility/dmtcp-smoke`; and
- `containers/dmtcp-r-brms-feasibility/testdata/brms-checkpoint/fixture.R`.

The tested image pins:

```text
Base image: rocker/tidyverse@sha256:d6684038a67fc65864c958151d76162a8f005e87f3bc861153e3d3c905f7dbdb
R: 4.4.3
Posit package snapshot: 2026-05-20
tidyverse: 2.0.0
brms: 2.23.0
rstan: 2.32.7
cmdstanr: 0.9.0
CmdStan: 2.39.0
DMTCP: 4.2.0 at f8009ce7b4ad211311ca2f72a929b975e4aa1155
```

The exact institutional SIF SHA-256 was:

```text
9de8dee8678b98de0524d8a55b9814ef1d3ce882bd6f83e19fb46447215a236a
```

The smoke also writes a sorted `environment/r-packages.tsv` containing all
resolved R package names, versions, and build versions. The exact SIF produced
a 219-line manifest including its header.

### Institutional support matrix

| Backend | Slurm evidence | DMTCP clients at checkpoint | Checkpoint images | Result |
| --- | --- | --- | --- | --- |
| `rstan` | job `13947170`, node `skl-001`, exit `0`, elapsed `00:02:41`, peak RSS `3609488K` | one R process | one R image, 722,812,229 bytes | pass |
| `cmdstanr` | job `13946899`, node `skl-032`, exit `0`, elapsed `00:01:49`, peak RSS `764252K` | R parent plus compiled Stan descendant | R image, 614,485,317 bytes; Stan image, 24,588,288 bytes | pass |

Both runs used the normal institutional account, the `general-short`
partition, Ubuntu Jammy kernel `5.15.0-173-generic`, and SingularityCE
`4.1.2-jammy`. Inside Singularity, all capability masks were zero and
`NoNewPrivs` was `1`. No `sudo`, `--fakeroot`, capability grant, nested Slurm
submission, or site policy change was used. This distinguishes DMTCP's
user-space direct-R path from the CRIU capability failure in OS-002.

For each backend, the fixture completed a real tidyverse transform and
`brms::brm()` fit, checkpointed during active sampling, terminated the original
DMTCP computation, restored through `dmtcp_restart` in a fresh Singularity
invocation, and produced a valid resumed fit. Each resumed result contained
2,000 posterior draws over 27 parameters. Both comparisons reported
`max_absolute_difference: 0` and `match: true` against their uninterrupted
fixed-seed baselines.

Complete institutional evidence remains under:

```text
/mnt/scratch/weave151/etl/runtime/os003-dmtcp-r-brms/evidence/slurm-13946899-cmdstanr
/mnt/scratch/weave151/etl/runtime/os003-dmtcp-r-brms/evidence/rstan-institutional-20260730
/mnt/scratch/weave151/etl/runtime/os003-dmtcp-r-brms/evidence/package-manifest-control-20260730
```

Local WSL/Docker evidence also passed the full `cmdstanr` checkpoint path, with
2,000 draws matching exactly. The same SIF passed the direct-R control
checkpoint under local normal-user SingularityCE 4.1.2. The WSL host did not
have enough memory to justify duplicating the full RStan checkpoint run after
the institutional RStan proof passed.

### Constraints discovered

- R's OpenBLAS/native-thread initialization segfaulted under DMTCP until the
  scoped one-core runtime explicitly set `OPENBLAS_NUM_THREADS=1`,
  `OMP_NUM_THREADS=1`, and the corresponding MKL, NumExpr, RcppParallel, and
  TBB thread limits. Parallel chains and threaded Stan remain unsupported.
- Model compilation must complete before the checkpointed sampling process
  starts.
- The smoke must wait until DMTCP has renamed all `.dmtcp.temp` files to final
  checkpoint images; a successful `dmtcp_command --bcheckpoint` return alone
  is not sufficient evidence.
- Open regular files require DMTCP open-file checkpointing and explicit
  overwrite permission during restore. All mutable files and prepared model
  artifacts must remain at the same bind destination.
- The WSL Docker-to-SIF conversion needed a Docker archive because
  SingularityCE 4.1.2 did not import the local `docker-daemon` image correctly.
  SIF construction also required temporary extra WSL swap because the local
  environment had only about 4 GiB of RAM. Neither issue occurred during the
  institutional runtime proof.

### Decision

The feasibility gate approves design of an R-specific external supervisor for
these two execution shapes. It does not approve checkpointing the Go worker,
existing Python work items, arbitrary R packages, multiple chains, threaded
Stan, external network sessions, or universal work-item checkpointing.

The later 2026-07-30 Strategic Concept decision selects the same direct-
interpreter DMTCP boundary for Python, subject to its own institutional
feasibility gate. That Python gate precedes the common interpreter-supervisor
slice. Worker/controller communication remains outside DMTCP.

## Notes

- Reuse the exact DMTCP release, commit, and source SHA-256 already recorded by
  OS-001. Do not use a floating branch, `latest` tag, or unverified download.
- Resolve and record exact immutable base-image and R dependency identities
  during implementation; this proposed charter intentionally does not invent
  them.
- Use a small deterministic Gaussian BRMS model with enough iterations or data
  to keep single-chain sampling active through the checkpoint trigger while
  remaining bounded on a development Slurm partition.
- Use fixed seeds and one execution thread so the normalized baseline
  comparison tests checkpoint continuation without parallel scheduling noise.
- Prepared model and CmdStan executable paths must live under the stable
  shared mount or at identical immutable paths in both container invocations.
- Use regular files for progress, output, logs, checkpoint evidence, and
  control conditions. Do not introduce a FIFO.
- Do not use DMTCP to launch the Go worker. The acceptance root is
  `dmtcp_launch Rscript`.
- Do not treat successful R startup, package loading, model compilation, or a
  DMTCP checkpoint command alone as proof. Sampling must resume and complete.
- Run the control probe before the expensive BRMS probes, and retain logs from
  the first failing phase.
- A later slice should test representative multiple-chain and threading modes
  before production claims include them.
