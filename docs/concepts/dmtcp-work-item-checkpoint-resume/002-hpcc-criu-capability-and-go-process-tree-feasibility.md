# 002 HPCC CRIU Capability and Go-Process-Tree Feasibility

Status: blocked

## Objective

Pin CRIU in the standard GOET worker image and add a repeatable institutional
HPCC smoke that determines whether an ordinary Slurm user can use CRIU under
SingularityCE 4.1.2 to checkpoint, terminate, and restore the actual
`worker execute` Go process together with its Python descendant.

This slice is a feasibility gate. A successful result permits design of the
reusable work-item-container and CRIU-supervisor slices. A reproducible failure
is also valid evidence, but it must identify the exact site capability, kernel
feature, namespace behavior, security policy, or CRIU incompatibility that
blocks the path.

## Current State

OS-001 established that:

- the standard worker image can build and identify pinned DMTCP `v4.2.0`;
- the image contains a dynamically linked `goetl-worker`;
- that DMTCP build can checkpoint a control `/bin/sleep` process;
- the real `worker execute` Go process does not produce a DMTCP checkpoint;
- its Python descendant does not join the DMTCP computation; and
- the failure is reproducible under Docker and
  `singularity-ce version 4.1.2-jammy`.

The DMTCP result blocks the original checkpoint backend. It does not determine
whether the institutional HPCC permits CRIU.

The current standard worker image does not contain CRIU. GOET has no repeatable
probe that records the HPCC kernel, capabilities, `ptrace` policy, namespaces,
seccomp state, CRIU feature checks, or dump/restore logs.

`cmd/worker/direct.go` already provides the bounded probe command:

```text
worker execute
  --config PATH
  --work-item PATH
  --source-bundle PATH
  --result PATH
```

The existing controlled Python fixture gives the probe this real process tree:

```text
goetl-worker execute
    -> Worker.Run
        -> python3
```

The agreed provisional deployment shape starts a lightweight worker container
and one reusable work-item container from the original Slurm batch script.
There is no nested `srun` or `sbatch`. The worker handles controller
communication outside the checkpoint. A runner in the reusable work-item
container owns one one-shot work-item child at a time. This slice does not
implement that topology; it tests the CRIU-sensitive child boundary using the
existing direct command.

SingularityCE 4.1.2 can start persistent instances and can pause a live OCI
container, but it exposes no durable checkpoint/restore command. A successful
GOET path therefore requires CRIU to dump and restore the work-item process
tree inside a reproducibly recreated Singularity environment.

## Target State

`containers/goetl-worker/Dockerfile`:

- builds CRIU `v4.2.1` from an immutable upstream release source;
- verifies a checked-in expected SHA-256 before building;
- installs the CRIU executable and required runtime libraries in the final
  image;
- makes the exact CRIU version observable through `criu --version`; and
- retains the existing OS-001 evidence until a later cleanup slice decides
  whether to remove DMTCP and its dynamic-linking changes.

A focused CRIU smoke first validates its orchestration locally, then runs under
the real institutional Slurm allocation as the normal HPCC user. The
institutional run is the acceptance gate; WSL, Docker, fake Slurm, or a
privileged container cannot substitute for it.

The HPCC smoke records, without exposing credentials:

- Slurm job and node identity;
- host distribution, kernel release, CPU architecture, and cgroup version;
- SingularityCE version and exact invocation flags;
- SIF SHA-256;
- effective UID and GID;
- effective, permitted, bounding, and ambient Linux capability masks;
- `NoNewPrivs`, seccomp mode, and seccomp-filter count;
- Yama `ptrace_scope` when exposed;
- user, PID, mount, and cgroup namespace identities visible to the probe;
- CRIU version;
- `criu check` and relevant feature-check output; and
- complete dump and restore logs.

The smoke runs without `sudo`, `--fakeroot`, a host configuration change, or a
test-only capability grant. It performs two ordered probes.

### Probe 1: control process tree

The smoke launches a deterministic non-GOET process tree, checkpoints it by
root PID, verifies that CRIU produced a non-empty image set, verifies that the
original tree is gone, and restores it in a fresh Singularity invocation using
the same SIF and stable mounted paths.

If this probe fails, the smoke records the exact failure and stops before
claiming anything about Go compatibility.

### Probe 2: actual GOET process tree

The smoke launches the existing `worker execute` Python fixture. The Python
descendant:

1. writes and flushes one pre-checkpoint marker;
2. waits for a test-controlled continuation file;
3. writes a valid `GOET_OUTPUT_JSON`; and
4. writes one post-resume marker.

The smoke:

1. confirms the root PID is the actual `goetl-worker execute` process;
2. confirms the live Python PID is a descendant before dumping;
3. invokes `criu dump --tree <pid>` into an owner-only shared directory;
4. verifies CRIU captured the process tree and that the original tree no
   longer exists;
5. starts a fresh Singularity invocation with the same SIF, mount destinations,
   working directory, and checkpoint directory;
6. invokes CRIU restore without restoring the old controller-facing worker;
7. releases the fixture continuation condition; and
8. verifies one completed direct result, one pre-checkpoint marker, one
   post-resume marker, and the expected logical output.

The smoke emits one machine-readable summary that classifies the gate as:

```text
pass
blocked_host_capability
blocked_kernel_feature
blocked_security_policy
blocked_singularity_namespace
blocked_criu_process_resource
blocked_go_process_restore
smoke_error
```

A blocked result includes the failing phase, exact command, exit status, log
paths, and non-secret environment facts needed to reproduce it. It does not
automatically request or apply an HPCC administrator change.

No controller, worker-session, Slurm warning, sibling-container, reusable
runner, pending-resume, or production checkpoint behavior changes in this
slice.

## Concept Decision

This slice updates the existing standard worker-image concept in
`containers/goetl-worker/Dockerfile`.

It adds one independent test concept: an unprivileged HPCC CRIU capability and
process-tree smoke. That concept needs its own executable test script because
it owns environment evidence collection, process-tree discovery, dump,
original-process verification, fresh-container restore, and result
classification.

The slice uses the current development-only `worker execute` boundary as an
exact probe. It does not promote that command to the future production runner
protocol.

The slice does not claim native Singularity container checkpointing. A passing
result proves that GOET can recreate the same immutable container environment
and use CRIU to restore the active work-item process tree within it.

## Required Context

Read these files first:

- `docs/concepts/dmtcp-work-item-checkpoint-resume/README.md`
- `docs/concepts/dmtcp-work-item-checkpoint-resume/001-pinned-dmtcp-image-and-go-process-feasibility.md`
- `containers/goetl-worker/Dockerfile`
- `containers/goetl-worker/test`
- `containers/goetl-worker/dmtcp-smoke`
- `cmd/worker/direct.go`
- `cmd/worker/testdata/dmtcp-checkpoint/work-item.json`
- `cmd/worker/testdata/dmtcp-checkpoint/source/main.py`

Do not read unrelated controller, persistence, signal, GDAL, workflow, or
provider implementation files unless the approved smoke exposes a direct build
or execution failure that requires them.

## Allowed Production Files

- `containers/goetl-worker/Dockerfile`

## Allowed Test Files

- `containers/goetl-worker/test`
- `containers/goetl-worker/criu-smoke`

The existing fixture files under
`cmd/worker/testdata/dmtcp-checkpoint/` may be read and mounted unchanged. If
they cannot express the CRIU proof without modification, stop and revise this
slice rather than silently broadening the test-file boundary.

## Allowed Documentation Files

- `containers/README.md`
- `PROJECT_STATE.md`
- `docs/TEST_AND_SMOKE_STATUS.md`
- `docs/concepts/README.md`
- `docs/concepts/dmtcp-work-item-checkpoint-resume/README.md`
- `docs/concepts/dmtcp-work-item-checkpoint-resume/002-hpcc-criu-capability-and-go-process-tree-feasibility.md`

## Out Of Scope

- Implementing the lightweight worker-container and reusable
  work-item-container topology.
- Adding a production runner service or worker-to-runner IPC.
- Running `srun` or `sbatch` from inside a worker.
- Adding worker drain signals, checkpoint timers, or Slurm warning directives.
- Adding controller checkpoint APIs, persistence, attempt lineage,
  pending-resume state, or resume limits.
- Modifying the GDAL worker image.
- Removing DMTCP or reverting OS-001 image changes.
- Treating Singularity `pause`/`resume` or a persistent overlay as a durable
  process checkpoint.
- Granting capabilities, changing `ptrace_scope`, changing seccomp policy, or
  modifying another HPCC host setting.
- Claiming cross-node or cross-allocation compatibility from a fresh container
  invocation in one allocation.
- Checkpointing active HTTP, rclone, database, or controller connections.
- Testing archive extraction, GDAL, MPI, GPU state, or representative
  128-GiB-memory checkpoint timing.
- Selecting final checkpoint manifest, retention, or security policy.

## Acceptance Criteria

- The standard worker image build uses immutable CRIU `v4.2.1` source and
  fails when its expected SHA-256 does not match.
- `criu --version` in the built image reports `4.2.1`.
- The existing missing-config image assertion remains green.
- The CRIU smoke has bounded waits and cleans up its own surviving test
  processes and Singularity invocations on success or failure.
- A local run proves that the smoke itself can detect processes, enforce
  marker ordering, and classify a CRIU failure; local privilege is not reported
  as HPCC compatibility.
- The institutional run executes as the normal HPCC account inside an actual
  Slurm allocation using SingularityCE 4.1.2 and no test-only privilege
  elevation.
- The institutional evidence records the exact kernel, capabilities,
  namespace, seccomp, `ptrace`, SIF, Singularity, and CRIU facts listed in the
  target state.
- The control probe either completes checkpoint/kill/fresh-invocation restore
  or produces a reproducible classified blocker with complete CRIU logs.
- The GOET probe runs only after the control probe passes.
- The GOET probe enters the real `Worker.Run` Python dispatch through
  `worker execute`; no synthetic executable replaces the Go root process.
- Before dump, the smoke proves the Python fixture is a descendant of the
  actual Go root PID.
- A passing GOET result proves a non-empty CRIU checkpoint, removal of the
  original tree, restore in a separate Singularity invocation, exactly one
  pre-checkpoint marker, exactly one post-resume marker, and one completed
  `gorc/worker-direct-result/v1` document containing the expected logical
  output.
- A failed control or GOET probe leaves OS-002 `blocked`, records the exact
  failure classification, and prevents later CRIU architecture slices from
  proceeding.
- A passing institutional GOET probe changes OS-002 to `implemented` and
  permits design of the reusable work-item-container slice; it does not claim
  that cross-allocation restore, external resources, or controller lifecycle
  are implemented.
- Project and test documentation distinguish DMTCP failure evidence, local
  CRIU evidence, and institutional HPCC CRIU evidence.
- No production file outside the allowed production-file list changes.

## Notes

- Resolve and record the SHA-256 for the exact CRIU `v4.2.1` release source
  during implementation. Do not use a floating branch, `latest` URL, or
  unverified download.
- The official CRIU `v4.2.1` release is:
  <https://github.com/checkpoint-restore/criu/releases/tag/v4.2.1>.
- `criu check` is diagnostic evidence, not the acceptance proof. The actual
  dump and restore must run.
- Run CRIU from outside the checkpointed root tree. In the future topology,
  the reusable runner can own both the active work-item child and a separate
  CRIU helper process.
- Use regular files for request, result, logs, markers, and checkpoint
  evidence. Do not introduce a FIFO.
- Do not preserve a live worker-to-runner socket in the checkpoint. This slice
  has no such connection.
- Keep every mutable fixture file under one owner-only mounted directory whose
  destination path is identical in the initial and restore invocations.
- Do not use `--leave-running` for the acceptance proof. Verify the original
  process tree is gone before restore.
- Preserve full CRIU logs even when the summary can classify the first error.
  CRIU failures often report a useful root cause earlier than their final
  message.

## Implementation Result

Implementation and institutional validation completed on 2026-07-29. The slice
is `blocked` at its control-process capability gate.

The standard worker image now builds:

```text
CRIU release: v4.2.1
CRIU commit: f3e4ef5389601ed0893820d5eef1a769a5eee901
Source archive SHA-256: fbe32da7dec8d8443f162b81ff28dae1e75195fd78ca502d94c478504798e5fe
Version output: Version: 4.2.1
```

`containers/goetl-worker/criu-smoke` implements both ordered probes, bounded
waits, process cleanup, environment evidence, full CRIU logs, and the
`goetl/criu-smoke-result/v1` classification document. The existing Python
fixture is mounted unchanged; the smoke creates a test-local work-item copy
only to redirect its control and marker paths beneath the owner-only evidence
directory.

Explicitly privileged local WSL/Docker passed the control probe and the actual
`goetl-worker execute` plus Python probe. Both original trees terminated,
restored in fresh container invocations, and continued rather than restarting.
The GOET result and single before/after markers validated successfully. This
run is recorded as `scope=local_only` and cannot satisfy the institutional
criterion.

The same image was converted to a SIF and run under
`singularity-ce version 4.1.2-jammy` as local UID/GID `1000/1000` without
privilege elevation. The probe recorded zero permitted, effective, bounding,
and ambient capabilities plus `NoNewPrivs: 1`. It stopped at the control dump
with:

```text
result: blocked_host_capability
missing effective capability 40: CAP_CHECKPOINT_RESTORE
missing effective capability 21: CAP_SYS_ADMIN
```

That WSL result was reproduced first on institutional development node
`dev-amd20`, then inside the required Slurm allocation.

```text
Slurm job: 13875979
Partition: general-short
Compute node: skl-010
Host OS: Ubuntu 22.04.5 LTS (Jammy)
Kernel: 5.15.0-173-generic
SingularityCE: 4.1.2-jammy
CRIU: 4.2.1
SIF SHA-256: 06c0fa4e6180ed9661aac41c009abf346f7be1789baadb6ec8b7d3b6e729de1e
Runtime UID/GID: 6123447/2024
CapInh/CapPrm/CapEff/CapBnd/CapAmb: all zero
NoNewPrivs: 1
Seccomp: 0
Result: blocked_host_capability
Phase: control_dump
```

The normal-account allocation used no `sudo`, `--fakeroot`, capability grant,
or test-only runtime security flag. CRIU reported missing effective
`CAP_CHECKPOINT_RESTORE` and `CAP_SYS_ADMIN` before producing any checkpoint
image. The durable pre-checkpoint control marker proves the target process was
running. The smoke correctly stopped before the GOET probe.

Complete evidence remains under:

```text
/mnt/scratch/weave151/etl/runtime/os002-criu/evidence/slurm-13875979
```

This result satisfies the slice's reproducible-blocker acceptance path. It
does not establish that CRIU and Go are incompatible; privileged local
Docker proved the exact Go-plus-Python tree can dump and restore. It
establishes that the current normal-user Slurm/Singularity environment cannot
start the CRIU dump. Do not proceed to a CRIU runner or later checkpoint
architecture slices without a new human-approved backend or an explicit,
institutionally approved capability-policy change.
