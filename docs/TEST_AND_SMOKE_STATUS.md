# Test And Smoke Status

Last updated: 2026-07-30

This file preserves the moved test coverage and smoke-test status section from the pre-split root state file.

## Tests

The project uses Go's standard `testing` package. Run all tests from the repository root:

```powershell
go test ./...
```

Current coverage includes:

- Shared work-item validation.
- Variable type validation, including scalar, object, and list types.
- Variable literal parsing for scalar, object, and list types.
- Variable object field, list index, and fan-out accessors.
- Variable precedence merging and reference lookup.
- Recursive variable resolution, scalar structured access, fan-out expression resolution, and max-depth failure behavior.
- Local workflow fan-out compilation into validated draft work items.
- Local client workflow submission HTTP behavior.
- JSON config loading and validation.
- Runtime directory validation.
- Demo temporary-output promotion, deterministic overwrite, and logging.
- Worker dispatch validation.
- Worker HTTP fetch, completion, and failure clients.
- Empty-queue handling.
- Worker looping across multiple items.
- Worker failure reporting.
- Controller assignment, completion, and failure endpoints.
- Controller raw work submission and status endpoint behavior.
- Controller submission status endpoint behavior.
- Controller source-bundle endpoint behavior for admitted Python source files,
  including missing-run, missing-source-context, unsafe-path, and cache
  miss/corruption errors.
- Controller workflow submission into the pending queue.
- Controller worker-start hook selection from submitted variables.
- Controller local worker command resolution.
- Controller worker-scaling decision state.
- Controller shutdown endpoint behavior.
- Controller rejection of invalid methods and payloads.
- Controller config loading and namespace normalization.
- Controller default config loading when no config path is supplied.
- Controller execution-environment config validation and construction.
- Controller startup assembly coverage for precedence, recovery mode, qualified lookup protection, and fail-closed startup.
- Docker transport command construction for `exec` and `cp` behavior.
- SSH transport config validation, key loading, host-key checking, connect/close behavior, command execution, copy/list behavior, filesystem helpers, reconnect behavior, and end-to-end in-process SSH/SFTP fixture coverage.
- Fake HPCC SSH controller config construction.
- Client SSH setup key generation, existing-key config generation, and required host-key confirmation behavior.
- Bash shell dialect newline, quoting, path localization, copy command, and remove command behavior.
- Slurm scheduler script writing, copy, and submit behavior.
- WorkerRuntime path derivation, remote directory preparation, worker config upload, and optional worker artifact upload.
- Optional `Preparer` helper behavior for components that need setup hooks.
- Controller workflow submission using `Controller.env` to prepare the runtime and submit scheduled worker jobs.
- Required controller SQLite initialization from the qualified main-database driver and connection-string variables.
- SQLite schema creation, strict version-1 validation, parent-directory creation, and attempt snapshot insertion.
- Controller-owned attempt recording adapter.
- Controller completion handling that records full completion metadata when present and still accepts legacy `id`-only completions.
- Explicit data-operator fixture smoke coverage for `asset.materialize -> compute -> commit_data`, including materialized input manifest hydration into compute, terminal records for all three operator families, source-transfer resource serialization, and publish-location write serialization.
- Worker use of controller-provided `materialized_data_assets` manifests without reacquiring provider data.
- Direct worker development execution for source-free and Python work, including
  runtime-only config, local source ZIP staging, generated bookkeeping,
  subprocess environment, logical output, artifact promotion, retained
  stdout/stderr, failure results, and zero controller HTTP requests.

Norton antivirus may briefly lock Go's temporary test executables after tests finish. If that happens, assertions still report `PASS`, but Go may print a cleanup error. Re-running the command usually succeeds.

## DMTCP Work-Item Feasibility Gate

Recorded on 2026-07-28 on branch
`concept/dmtcp-work-item-checkpoint-resume`.

Pinned build identity:

```text
DMTCP release: v4.2.0
DMTCP commit: f8009ce7b4ad211311ca2f72a929b975e4aa1155
Source archive SHA-256: 3b240c78804bbf1e9354ee3da5c8760c3c952045f71773e9ed490846b15adce0
Version output: dmtcp_launch (DMTCP) 4.2.0
Worker linking: dynamic, libc.so.6
Local Docker image ID: sha256:2ed1d0dd3042e8255fbeda596b222361f2b8e9be422e2806f28755ad34c18608
```

The image build succeeded and its existing missing-config assertion remained
green:

```text
invalid config: read config file /missing-worker-config.json:
open /missing-worker-config.json: no such file or directory
```

The full Docker test command was:

```bash
containers/goetl-worker/test
```

The image build and missing-config assertion passed. The DMTCP phase failed at
the required checkpoint-set assertion:

```text
DMTCP smoke failed: expected checkpoint images for the Go worker and Python descendant

Client List:
1, goetl-worker[40000:<real-pid>]@goetl-dmtcp-smoke, ..., WorkerState::RUNNING
```

The fixture's Python process had already written the durable
`pre-checkpoint.log`, proving that it was running, but it was absent from the
coordinator client list. A blocking `dmtcp_command --bcheckpoint` returned and
the checkpoint directory remained empty. The DMTCP worker log also reported:

```text
Application trying to use DMTCP's signal for it's own use.
```

Using DMTCP-internal test signal `35` instead of its default `SIGUSR2` did not
change the result.

A control process under the same image and Docker runtime settings succeeded:

```text
Client List:
1, sleep[40000:<real-pid>]@<container>, ..., WorkerState::RUNNING

ckpt_sleep_<identity>.dmtcp 20520960 bytes
```

Docker required `--pid host`. Without it, DMTCP exited during initialization
with:

```text
Unable to open /proc/self/stat
```

The image was exported and exercised through the installed Singularity
runtime:

```text
singularity-ce version 4.1.2-jammy
WSL host: Ubuntu 24.04.4 LTS
SIF SHA-256: fc593f8ed3d7c33e7ba6742f3f18cbd79e3596786e3e0b64ad63a9711ca7edf3
```

Command:

```bash
GOETL_DMTCP_RUNTIME=singularity \
GOETL_WORKER_SIF=/tmp/goetl-worker-dmtcp-os1.sif \
  bash containers/goetl-worker/dmtcp-smoke
```

Singularity produced the same single-client list, signal-handler warning, empty
checkpoint directory, and nonzero smoke result. This proves the local
`4.1.2-jammy` package behavior, but the WSL host is Ubuntu 24.04 rather than the
institutional Ubuntu Jammy host.

Result: the feasibility gate is blocked. No checkpoint restart or completed
direct result could be tested because no Go/Python checkpoint set existed.
Later DMTCP supervisor, signaling, persistence, and resume slices were not
started.

## CRIU Work-Item Feasibility Gate

Local and institutional validation was recorded on 2026-07-29 on branch
`concept/dmtcp-work-item-checkpoint-resume`.

Pinned build identity:

```text
CRIU release: v4.2.1
CRIU commit: f3e4ef5389601ed0893820d5eef1a769a5eee901
Source archive SHA-256: fbe32da7dec8d8443f162b81ff28dae1e75195fd78ca502d94c478504798e5fe
Version output: Version: 4.2.1
Local Docker image ID: sha256:22ce410d4e4f584ae4b4a8ff0fdcf70da5669dad1883691d5a9921fdfb1f193b
```

The complete standard worker-image test passed:

```bash
containers/goetl-worker/test
```

It retained the missing-config assertion, verified the exact CRIU version, and
ran the control and actual GOET probes with explicit local Docker test
privilege. The probe environment used:

```text
WSL kernel: 6.18.33.2-microsoft-standard-WSL2
Docker: --privileged --pid host --security-opt seccomp=unconfined
CapEff: 000001ffffffffff
NoNewPrivs: 0
Seccomp: 0
ptrace_scope: 1
scope: local_only
```

The control process completed dump, original-process termination,
fresh-container restore, and one post-resume marker. The actual
`goetl-worker execute` root and Python descendant also produced non-empty CRIU
images, terminated before restore, resumed in a fresh container invocation,
and completed the validated `gorc/worker-direct-result/v1` document. The
durable fixture markers were exactly:

```text
before-checkpoint
after-resume
```

`criu check --all` returned status `1` in this privileged local environment
even though both actual dump/restore probes passed. This confirms that
`criu check` is retained as diagnostic evidence rather than used as the gate.

The same image was converted through a Docker archive and exercised without
privilege elevation under the installed local Singularity package:

```text
singularity-ce version 4.1.2-jammy
SIF SHA-256: 06c0fa4e6180ed9661aac41c009abf346f7be1789baadb6ec8b7d3b6e729de1e
UID/GID: 1000/1000
CapPrm/CapEff/CapBnd/CapAmb: all zero
NoNewPrivs: 1
ptrace_scope: 1
scope: local_only
```

The normal-user Singularity run stopped at the control dump and emitted:

```json
{
  "schema": "goetl/criu-smoke-result/v1",
  "result": "blocked_host_capability",
  "phase": "control_dump",
  "exit_status": 1,
  "runtime": "singularity",
  "scope": "local_only"
}
```

CRIU reported that effective capability 40 (`CAP_CHECKPOINT_RESTORE`) and
capability 21 (`CAP_SYS_ADMIN`) were both missing. The GOET probe correctly did
not run after this control failure. That WSL result did not decide the gate.

The SIF and exact smoke files were staged on institutional shared scratch and
their hashes verified. A normal-account development-node run on `dev-amd20`
reproduced `blocked_host_capability` at the control dump. The required Slurm
run then executed with no `sudo`, `--fakeroot`, capability grant, or test-only
security override:

```text
Slurm job: 13875979
Partition: general-short
State/exit: FAILED / 1:0
Elapsed: 00:00:08
Compute node: skl-010
Host OS: Ubuntu 22.04.5 LTS (Jammy)
Kernel: 5.15.0-173-generic
Cgroup filesystem: cgroup2fs
SingularityCE: 4.1.2-jammy
CRIU: 4.2.1
SIF SHA-256: 06c0fa4e6180ed9661aac41c009abf346f7be1789baadb6ec8b7d3b6e729de1e
Runtime UID/GID: 6123447/2024
CapInh/CapPrm/CapEff/CapBnd/CapAmb: all zero
NoNewPrivs: 1
Seccomp: 0
CRIU check status: 1
```

The institutional result document was:

```json
{
  "schema": "goetl/criu-smoke-result/v1",
  "result": "blocked_host_capability",
  "phase": "control_dump",
  "command": "criu dump --tree CONTROL_PID",
  "exit_status": 1,
  "runtime": "singularity",
  "scope": "institutional_hpcc"
}
```

The control process wrote its durable pre-checkpoint marker. CRIU then reported
that effective capabilities 40 and 21 were missing and produced no checkpoint
image. The GOET probe did not run, as required by the ordered gate.

Complete institutional evidence remains at:

```text
/mnt/scratch/weave151/etl/runtime/os002-criu/evidence/slurm-13875979
```

Result: OS-002 is blocked by the current institutional normal-user capability
policy before CRIU reaches a Go-specific operation. Privileged local evidence
shows the exact Go-plus-Python tree can restore, but later CRIU runner,
signaling, persistence, and resume slices must not proceed without a new
backend decision or an explicitly approved institutional capability change.

## Direct-R DMTCP BRMS Feasibility Gate

Local and institutional validation was recorded on 2026-07-30 on branch
`concept/dmtcp-work-item-checkpoint-resume`.

Pinned runtime identity:

```text
Base image digest: sha256:d6684038a67fc65864c958151d76162a8f005e87f3bc861153e3d3c905f7dbdb
R: 4.4.3
Posit package snapshot: 2026-05-20
tidyverse: 2.0.0
brms: 2.23.0
rstan: 2.32.7
cmdstanr: 0.9.0
CmdStan: 2.39.0
DMTCP: 4.2.0
SIF SHA-256: 9de8dee8678b98de0524d8a55b9814ef1d3ce882bd6f83e19fb46447215a236a
```

The smoke also records a sorted `environment/r-packages.tsv`; the exact SIF
produced 218 installed-package records plus the header.

The full local Docker `cmdstanr` smoke passed under WSL. It checkpointed the R
parent and live compiled Stan descendant, terminated the original computation,
restored in a fresh container invocation, and compared 2,000 resumed posterior
draws with the uninterrupted baseline:

```json
{
  "backend": "cmdstanr",
  "draws": 2000,
  "parameters": 27,
  "max_absolute_difference": 0,
  "match": true
}
```

Local normal-user SingularityCE 4.1.2 also passed the direct-R control
checkpoint using the exact SIF. A complete local RStan checkpoint run was not
repeated because the WSL environment had about 4 GiB of RAM; model preparation
and uninterrupted RStan sampling were verified locally before the full
institutional proof.

Two normal-account jobs then ran in actual institutional Slurm allocations
without `sudo`, `--fakeroot`, a capability grant, nested Slurm submission, or a
site security change:

| Backend | Slurm job | Node | State/exit | Elapsed | Peak RSS |
| --- | --- | --- | --- | --- | --- |
| `cmdstanr` | `13946899` | `skl-032` | `COMPLETED / 0:0` | `00:01:49` | `764252K` |
| `rstan` | `13947170` | `skl-001` | `COMPLETED / 0:0` | `00:02:41` | `3609488K` |

The institutional host/runtime facts included:

```text
Host kernel: Ubuntu Jammy 5.15.0-173-generic
SingularityCE: 4.1.2-jammy
Runtime UID/GID: normal institutional account
CapInh/CapPrm/CapEff/CapBnd/CapAmb: all zero
NoNewPrivs: 1
Seccomp: 0
```

RStan enrolled one R client and produced one 722,812,229-byte checkpoint image.
CmdStanR enrolled both the R client and the live compiled Stan executable and
produced a 614,485,317-byte R image plus a 24,588,288-byte Stan image. Both
restores used a fresh Singularity invocation and the same stable shared bind
destination. Both completed BRMS fits and reported:

```json
{
  "draws": 2000,
  "parameters": 27,
  "max_absolute_difference": 0,
  "match": true
}
```

Complete evidence remains at:

```text
/mnt/scratch/weave151/etl/runtime/os003-dmtcp-r-brms/evidence/slurm-13946899-cmdstanr
/mnt/scratch/weave151/etl/runtime/os003-dmtcp-r-brms/evidence/rstan-institutional-20260730
/mnt/scratch/weave151/etl/runtime/os003-dmtcp-r-brms/evidence/package-manifest-control-20260730
```

The probe found one required runtime constraint: R/OpenBLAS startup under DMTCP
must explicitly limit native thread pools to one for this execution shape. The
smoke sets OpenBLAS, OpenMP, MKL, NumExpr, RcppParallel, and TBB thread limits.
It also waits for finalized checkpoint images rather than trusting only the
checkpoint-command return, and restores checkpointed regular files with
explicit overwrite permission.

Result: OS-003 passes for direct, single-chain, single-core RStan and CmdStanR
under the institutional runtime. This permits an R-specific external-supervisor
Operational Slice while leaving Go outside DMTCP. It does not approve universal
work-item checkpointing, parallel chains, threaded Stan, or arbitrary R
packages.

## Direct-Python DMTCP Feasibility Gate

Local and institutional validation was recorded on 2026-07-30 on branch
`concept/dmtcp-work-item-checkpoint-resume`.

Pinned runtime identity:

```text
Base image: python:3.11.15-slim-bookworm
amd64 manifest: sha256:28255a3ace7eb4c48bc1b57b90af29e1bc82b4fd6c60614a8e3dce61b87ff941
Python: 3.11.15
NumPy: 2.4.6
NumPy wheel SHA-256: 89cd468399cfd2504718f0ba50e410dca55a170b61a02ad92bb18c8a65186e93
DMTCP: 4.2.0
SIF SHA-256: 48c58538c53e6cd5c007362f87f4659783ef1ff675320a050775032d3402a6d7
```

The full local WSL Docker smoke passed. Its ordered pure-Python control
checkpointed live interpreter state and an open regular file, terminated the
original computation, restored in a new container invocation, and produced
singular before-checkpoint and after-resume markers. The complete probe then:

- ran an uninterrupted fixed-input baseline;
- launched CPython directly under DMTCP;
- started one Python child from the parent;
- entered repeated NumPy matrix multiplication with native thread pools
  limited to one;
- required both processes in the DMTCP client list while native work was
  active;
- produced two finalized checkpoint images;
- killed the original DMTCP computation;
- restored both images from a fresh container invocation; and
- required exact parent and child JSON equality with the baseline.

The same complete smoke passed locally as an ordinary WSL user under
SingularityCE 4.1.2-jammy with the exact SIF.

The decisive normal-account institutional allocation used no `sudo`,
`--fakeroot`, capability grant, nested Slurm command, or site security change:

```text
Slurm job: 13966757
Partition: general-short
State/exit: COMPLETED / 0:0
Elapsed: 00:00:30
Compute node: skl-132
Allocated CPUs: 2
Requested memory: 8G
SingularityCE: 4.1.2-jammy
```

The institutional DMTCP coordinator listed the direct parent and its child as
two running `python3.11` clients. The parent image was 251,256,862 bytes and
the child image was 37,035,748 bytes. The restore used a new Singularity
invocation with the same SIF and bind destination. Both baseline/resumed pairs
were byte-identical, and the semantic comparison reported:

```json
{
  "actual_run": "resumed",
  "expected_run": "baseline",
  "match": true,
  "native_repetitions": 12,
  "schema": "goetl/dmtcp-python-compare/v1"
}
```

Complete evidence remains at:

```text
/mnt/scratch/weave151/etl/runtime/os004-dmtcp-python/evidence/goetl-os004-python.cTJp2Y
```

Result: OS-004 passes for the tested direct CPython 3.11, NumPy 2.4.6,
one-native-thread, one-Python-descendant execution shape. It permits design of
the common external R/Python DMTCP supervisor while the Go worker remains
outside the checkpoint. It does not approve arbitrary Python environments,
native extensions, descendants, parallel numerical execution, or universal
work-item checkpointing.

## Resume-Artifact Contract Model

OS-005 focused verification recorded on 2026-07-30:

```powershell
go test ./internal/model -count=1
```

Result:

```text
ok  	goetl/internal/model
```

The tests validate JSON round trips for DMTCP, native-tool, and manual-Go
resume manifests and reject unsupported schema/strategy values, missing
identity or compatibility facts, invalid timestamps and generations, unsafe
relative paths, malformed or uppercase SHA-256 values, duplicate or missing
files, ambiguous strategy payloads, and invalid controller-facing references.

This is model-level evidence only. It does not prove filesystem manifest
writing, controller acceptance/persistence, worker adapter dispatch, Slurm
drain behavior, or actual resume execution.

## Checkpoint-Generation SQLite Persistence Lifecycle

OS-006 schema, migration, and store-lifecycle verification recorded on
2026-07-30:

```powershell
go test ./internal/persistence -count=1
```

Result:

```text
ok  	goetl/internal/persistence
```

SQLite schema version 7 adds immutable checkpoint generations, quantum or
shutdown suspended-attempt history, pending-resume references, attempt lineage
and resume-attempt fields, and generation/resume lookup indexes. The focused
migration fixture proves that one transactional version 6 to version 7
migration preserves representative queued, running, abandoned, completed, and
failed records, initializes new nullable resume fields, reopens idempotently,
and rejects an incomplete version 6 database without partial migration.

The store tests prove exact manifest/reference persistence, immutable
generation sequencing and replay, periodic confirmation without ownership
loss, atomic quantum/final suspension, fallback suspension from the latest
accepted generation, and rollback when the queue transition fails. Resume
claims create a new attempt with predecessor, artifact, lineage, and
per-artifact attempt-count metadata. Worker stop and expiry recover from the
latest accepted checkpoint, while causal failure remains terminal. Invalid
bundles, stale owners, and claims over the configured resume-attempt limit fail
without mutating pending work.

This is persistence-boundary evidence only. No controller HTTP transport,
worker timer/adapter, filesystem checkpoint bundle creation and byte
verification, or runtime resume path exists yet.

## Direct Worker Development Execution Evidence

Recorded on 2026-07-11 on branch
`concept/gorc-worker-direct-execution`.

Focused command:

```powershell
go test ./cmd/worker -run 'TestRunDirectPythonTargetFixture' -count=1 -v
```

Observed result on Windows with Python 3.10.9:

```text
PASS
TestRunDirectPythonTargetFixture/sentinel_controller
TestRunDirectPythonTargetFixture/no_controller_URL
TestRunDirectPythonTargetFixtureFailure
```

The fixture builds a source ZIP at test time and invokes `runDirectCommand`.
Assertions cover generated attempt/source bookkeeping, source extraction,
required `GOET_*` environment variables, input/output JSON, a promoted file
artifact, result evidence, stdout/stderr retention, and failed-process
diagnostics. The sentinel server counts every path and observed zero total HTTP
requests during successful and failed direct Python execution.

Fixture sources:

```text
cmd/worker/testdata/direct-python/source/main.py
cmd/worker/testdata/direct-python/work-item.json
```

## Secure Network Exposure OS-009 Evidence

Recorded on 2026-07-10 against branch
`concept/secure-network-exposure-gorc-controller-api` at commit
`bf67915`.

### Automated security tests

Command:

```powershell
go test ./internal/controllerauth ./internal/controllerhttp ./cmd/controller ./cmd/worker
```

Result: pass.

Evidence added in this slice:

- `internal/controllerauth` has an explicit route-role matrix for every phase-1
  route and role.
- `internal/controllerhttp` has HTTPS fixture coverage, untrusted certificate
  rejection, same-origin redirect handling, and cross-origin credential
  forwarding rejection.

### Production-like VM HTTPS smoke

Target:

```text
Dedicated Linux VM with temporary wildcard DNS
Temporary DNS: <temporary-controller-host> -> <dedicated-vm-public-ip>
Ingress: Caddy v2.11.4
Controller listener: 127.0.0.1:8080
```

The VM controller was rebuilt from the concept branch and installed at:

```text
<controller-install-root>/bin/gorc-controller
<controller-install-root>/bin/gorc-worker
```

The controller config used bearer credentials from restrictive service-owned
token files and isolated OS-009 state under service-owned controller data and
log roots:

```text
<controller-state-root>
<controller-log-root>
```

External endpoint smoke command:

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File scripts\network\smoke-controller-endpoint.ps1 `
  -ControllerUrl https://<temporary-controller-host> `
  -HttpUrl http://<temporary-controller-host> `
  -TokenFile .run\os009-secrets\controller-client-token `
  -SkipLocalLoopbackCheck
```

Result:

```text
GET /healthz over HTTPS -> HTTP 204
GET /status without token over HTTPS -> HTTP 401
GET /status with token over HTTPS -> HTTP 200
GET /healthz over HTTP -> HTTP 308 redirect to HTTPS
Controller endpoint smoke passed.
```

External TCP reachability from the development machine:

```text
<dedicated-vm-public-ip>:80    open
<dedicated-vm-public-ip>:443   open
<dedicated-vm-public-ip>:8080  closed
```

VM loopback verification:

```text
127.0.0.1:8080 listened by gorc-controller
GET http://127.0.0.1:8080/healthz -> HTTP 204
GET http://127.0.0.1:8080/status without token -> HTTP 401
```

### External worker callback smoke

The development machine acted as the external worker host. The worker read its
token from `.run/os009-secrets/controller-worker-token` and used:

```text
https://<temporary-controller-host>
```

Work item submitted over HTTPS:

```json
{
  "id": "os009-external-worker-001",
  "type": "write_demo_output",
  "output_filename": "os009-external-worker-001.txt"
}
```

Worker command:

```powershell
go run ./cmd/worker .run\os009-worker\worker.json
```

Result:

```text
worker starting
log dir: C:\Joe Local Only\College\Research\go-etl\.run\os009-worker\logs
no work available
```

Output evidence:

```text
.run/os009-worker/data/os009-external-worker-001.txt
completed os009-external-worker-001
```

Controller status after the worker run:

```json
{"pending":0,"assigned":0,"failed":0,"pending_reuse_candidates":0,"attempts":0,"attempt_variables":0}
```

### Singularity worker image build and staging

The worker container image was built from `containers/goetl-worker/Dockerfile`
in WSL with Docker and SingularityCE 4.1.2 available.

Build and verification commands:

```bash
containers/goetl-worker/test
docker save -o .run/os009-bin/goetl-worker-dev.tar goetl/worker:dev
singularity build .run/os009-bin/goetl-worker.sif docker-archive:.run/os009-bin/goetl-worker-dev.tar
singularity exec .run/os009-bin/goetl-worker.sif /goetl/goetl-worker /missing-worker-config.json
```

Result:

```text
invalid config: read config file /missing-worker-config.json: open /missing-worker-config.json: no such file or directory
```

Artifact:

```text
.run/os009-bin/goetl-worker.sif
size: 48 MiB
sha256: 5f32cbe58ca7ed11981a4efdacc17c8d216001d465d4fba6894ede6fe1898e29
```

The SIF was staged on the dedicated controller VM for later transfer to the
execution host:

```text
<controller-install-root>/images/goetl-worker.sif
sha256: 5f32cbe58ca7ed11981a4efdacc17c8d216001d465d4fba6894ede6fe1898e29
```

The controller `singularity_worker` runtime now defaults an omitted `bind`
setting to `<runtime root>:<runtime root>`, so the generated worker config,
token file, logs, temp directory, data directory, and cache roots can live under
one HPCC runtime root mounted at the same absolute path inside the container.

### Actual HPCC Slurm worker scheduling smoke

The dedicated VM controller was reconfigured to schedule workers on an HPCC
through SSH transport, Slurm, and the staged Singularity worker image. The
controller service uses service-owned SSH material outside user home
directories.

Runtime paths used by the smoke:

```text
HPCC runtime root: <hpcc-runtime-root>
Worker image: <hpcc-runtime-root>/images/goetl-worker.sif
Worker token file: <hpcc-runtime-root>/secrets/controller-worker-token
Generated worker config: <hpcc-runtime-root>/config/worker.json
Generated Slurm script: <hpcc-runtime-root>/scripts/worker.slurm
```

Submitted work item:

```json
{
  "id": "os009-hpcc-worker-001",
  "type": "write_demo_output",
  "output_filename": "os009-hpcc-worker-001.txt"
}
```

Controller evidence:

```text
worker_start_requested start_count=1 reason=active_capacity_below_claimable_work
worker_start_confirmed_by_claim reservation_id=worker-start-1
persisted work item completed: os009-hpcc-worker-001 attempt-47ab8d033630ef101bb7303b8e9379f6
```

HPCC Slurm evidence:

```text
Slurm output: <hpcc-runtime-root>/logs/goetl-worker-<job-id>.out
Slurm error: <hpcc-runtime-root>/logs/goetl-worker-<job-id>.err
Worker log: <hpcc-runtime-root>/logs/worker.log
```

The generated Slurm script ran:

```bash
/usr/bin/singularity exec \
  --bind <hpcc-runtime-root>:<hpcc-runtime-root> \
  <hpcc-runtime-root>/images/goetl-worker.sif \
  /goetl/goetl-worker \
  <hpcc-runtime-root>/config/worker.json
```

HPCC output evidence:

```text
<hpcc-runtime-root>/data/os009-hpcc-worker-001.txt
completed os009-hpcc-worker-001
```

Controller status after the HPCC worker run:

```json
{"pending":0,"assigned":0,"failed":0,"pending_reuse_candidates":0,"attempts":0,"attempt_variables":0}
```

### Sentinel scan

Sentinel:

```text
goet-controller-auth-sentinel-009-do-not-persist
```

The exact sentinel was absent from:

- `.run/os009-worker`;
- `.run/os009-deploy`;
- service-owned controller logs;
- service-owned controller OS-009 state;
- service-owned controller config;
- `journalctl -u gorc-controller -u caddy`;
- `<hpcc-runtime-root>` on HPCC, excluding the intentional worker token file.

The exact sentinel is intentionally present only in explicitly provisioned
credential fixture files.

### Remaining OS-009 evidence gap

The production-like HTTPS VM smoke, external worker callback, and actual
HPCC/Slurm worker scheduling smoke are complete. No OS-009 external smoke
evidence gap remains open.

## Local Controller SSH Reverse Callback Evidence

Recorded on 2026-07-10 against branch
`concept/secure-network-exposure-gorc-controller-api`.

This smoke verified the local/no-domain controller mode that uses SSH reverse
callback transport instead of public HTTPS. The local controller listened only on
`127.0.0.1:8080`; an SSH reverse callback listener on an HPCC dev node forwarded
worker HTTP callbacks back to the laptop controller.

Execution shape:

```text
local client -> local controller -> SSH transport -> HPCC dev-node process
HPCC dev-node worker -> ssh_reverse loopback callback -> local controller
```

The worker was launched through the `remote_process` scheduler, not Slurm. This
uses an HPCC dev-node process and is therefore suitable only for short smoke
tests or sites that explicitly permit that process model.

Evidence:

```text
controller /healthz: 200 OK
submission: completed
initial work items: 2
completed work items: 2
remote output files: cdl-demo-2024.txt, cdl-demo-2025.txt
remote process stderr: empty
```

Output contents:

```text
completed write-demo-2024
completed write-demo-2025
```

The direct non-loopback SSH reverse bind remained unavailable: requesting a
non-loopback reverse bind on the HPCC dev node still produced a loopback-only
listener. A dev-node relay was added for the Slurm path below.

### Local Controller SSH Reverse Relay Slurm Evidence

Recorded on 2026-07-10 against branch
`concept/secure-network-exposure-gorc-controller-api`.

This smoke verified local-controller HPCC orchestration without DNS, a public VM,
or managed HTTPS ingress. The local controller listened on `127.0.0.1:8080`.
The controller opened an SSH reverse listener on the HPCC dev node, then started
a dev-node relay bound to a worker-visible interface. Slurm compute workers used
the relay URL and the relay forwarded callbacks through the SSH reverse tunnel
to the laptop controller.

Execution shape:

```text
local client -> local controller -> SSH transport -> HPCC Slurm
HPCC compute worker -> dev-node relay -> ssh_reverse loopback listener -> local controller
```

The worker config generated for this smoke included the explicit opt-in flag:

```json
{
  "controller_url": "http://<hpcc-dev-node>:<relay-port>",
  "controller_token_file": "<hpcc-runtime-root>/secrets/controller-worker-token",
  "controller_insecure_external_http_allowed": true
}
```

The worker SIF was rebuilt from the current source and staged to the HPCC image
path used by the smoke:

```text
sha256: f76788783e0d0ea0355cc10f714989ed63744f7e20d6196f4166eace7bda5f72
```

CLI result:

```text
Submission: run-c13a4d12909e63a80081fcaeda6df94c
Workflow: cdl-demo
Initial work items: 2
Status: completed
Known work items: 2
Queued: 0
Running: 0
Completed: 2
Failed: 0
Skipped: 0
Stage 0: completed steps=1 assignable_pending=0 blocked_future=0 active=0 completed=2 failed=0 skipped=0
```

Controller evidence:

```text
worker_start_requested start_count=1 reason=active_capacity_below_claimable_work
worker_start_confirmed_by_claim reservation_id=worker-start-1
persisted work item completed: write-demo-2024 attempt-29c686cd2d5c0bef303086e2534efb2d
worker_start_confirmed_by_claim reservation_id=worker-start-2
persisted work item completed: write-demo-2025 attempt-9581947532efcebcb12b2bbe9010ab11
```

HPCC evidence:

```text
Slurm jobs: goetl-worker-12222949, goetl-worker-12222950
Compute nodes: skl-035, skl-083
Output files:
<hpcc-runtime-root>/data/cdl-demo-2024.txt
<hpcc-runtime-root>/data/cdl-demo-2025.txt
```

Post-shutdown check: neither the reverse-listener port nor the relay port
remained listening on the HPCC dev node.

Residual issue: one late extra worker printed a `connection refused` fetch error
after the workflow had already completed and the controller had shut down. Slurm
still marked that job `COMPLETED` because the current worker main logs errors
and returns without a non-zero process exit. Track that as a worker process exit
semantics follow-up; it did not prevent the workflow from completing.
