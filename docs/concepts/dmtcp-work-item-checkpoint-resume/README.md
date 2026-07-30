# Work-Item Pause and Resume

Status: Active - per-work-item pause adapter strategy selected

> Architecture revision notice, 2026-07-30: OS-001 disproved DMTCP for the
> required Go work-item boundary. OS-002 proved that privileged CRIU can
> restore the required local Go-plus-Python tree, but the normal institutional
> Slurm/Singularity environment supplies neither
> `CAP_CHECKPOINT_RESTORE` nor `CAP_SYS_ADMIN`. OS-003 passed a narrower
> feasibility gate for the target tidyverse-plus-BRMS use case:
> `dmtcp_launch Rscript`, with the Go worker outside the checkpoint. The
> original universal DMTCP target material below records historical design and
> is not authority for implementation. DMTCP is approved only for the tested
> single-chain, single-core direct-R execution shapes. OS-004 passed the
> corresponding direct-Python boundary with CPython 3.11, NumPy native work,
> and one Python descendant. Native and manual adapters still require separate
> implementation slices.

## Post-OS-001 Architecture Direction

The provisional allocation topology is:

```text
original Slurm batch script
    +-- lightweight worker container
    |      +-- controller communication and drain ownership
    |
    +-- reusable work-item container
           +-- fresh runner service for the allocation
                  +-- DMTCP-owned R/Python interpreter; or
                  +-- native tool process such as rclone; or
                  `-- in-process Go handler with manual pause state
```

The original batch script starts both containers. The worker does not invoke
`srun` or `sbatch` after it starts. One worker owns at most one active work item;
parallel work uses multiple independently allocated workers.

The lightweight worker container and runner service remain outside DMTCP. A
replacement allocation recreates the compatible work-item container and stable
bind destinations, starts a new runner service, and invokes the selected
resume adapter against shared durable state.

SingularityCE 4.1.2 supplies container and instance lifecycle, but not durable
checkpoint/restore. DMTCP is rejected for this boundary because OS-001 found
Go signal-handler interference and failure to enroll the Python descendant.
CRIU is not selected as the production backend. OS-002 found that the exact
unprivileged institutional HPCC path cannot start a CRIU control dump.

OS-003 establishes a separate, narrower boundary:

```text
lightweight Go worker, outside the checkpoint
    -> DMTCP coordinator
        -> dmtcp_launch Rscript
            -> tidyverse preprocessing
            -> BRMS/Stan computation and enrolled descendants
```

This boundary does not checkpoint Go or claim compatibility for existing Go,
Python, rclone, archive, or GDAL work items. Institutional Slurm jobs
`13947170` and `13946899` proved the actual target R workload with
single-chain, single-core `rstan` and `cmdstanr` process shapes. Both jobs
checkpointed during active BRMS sampling, terminated the original computation,
restored in a fresh Singularity invocation, and reproduced 2,000 posterior
draws exactly. The `cmdstanr` checkpoint enrolled both the R parent and compiled
Stan descendant.

OS-004 establishes the corresponding direct-Python boundary:

```text
lightweight Go worker, outside the checkpoint
    -> DMTCP coordinator
        -> dmtcp_launch python3
            -> NumPy/OpenBLAS native computation
            -> enrolled Python descendant
```

Normal-account institutional Slurm job `13966757` checkpointed both Python
processes, terminated the original computation, restored both images in a
fresh Singularity invocation, and reproduced the uninterrupted parent and
child outputs exactly. The proven shape uses CPython 3.11, NumPy 2.4.6, one
native numerical thread, and one Python descendant.

## Authoritative Pause/Resume Strategy

Decision recorded 2026-07-30: GOET has one controller-level pause/resume
lifecycle and multiple work-item-owned pause adapters. A pause adapter converts
one running attempt into a validated durable resume artifact. The controller
accepts that artifact, records the producing attempt as suspended history,
returns the logical work item to pending state, and creates a new linked
`attempt_id` when another worker claims it.

The selected adapter depends on the operation:

| Work-item execution shape | Pause adapter | Resume behavior | Evidence state |
| --- | --- | --- | --- |
| Direct R interpreter and enrolled Stan descendants | DMTCP | Validate the DMTCP generation, terminate it, then use `dmtcp_restart` from the compatible image and stable mounts | RStan and CmdStanR single-chain/single-core passed OS-003 |
| Direct Python interpreter with NumPy native work and one Python descendant | DMTCP | Same external-supervisor boundary as R | scoped CPython 3.11/NumPy shape passed OS-004 |
| `rclone` transfer launched by a Go handler | native continuation | Stop `rclone` after preserving its durable partial-transfer workspace, then relaunch through the operation adapter using compatible rclone continuation behavior | selected design; exact command/backend semantics still require proof |
| In-process Go operation | manual continuation | The handler writes explicit operation progress at a safe boundary, returns a paused result, and a later handler invocation resumes from that state | selected design; each handler requires its own state contract and tests |
| Other external or mixed operations | explicitly selected per type | DMTCP, native continuation, or manual continuation only after compatibility evidence | undecided until the work-item type is reviewed |

`rclone` is not itself a Go runtime checkpoint. It is an external process
started by Go. The Go adapter owns termination, durable partial state,
validation, and relaunch. Likewise, manual Go pause does not serialize Go
memory; it serializes application-level progress such as completed members,
cursors, phase identity, and verified temporary outputs.

All adapters produce the same controller-facing resume-artifact envelope with
common work-item, attempt, lineage, strategy, compatibility, storage, integrity,
and retention facts. The strategy-specific payload may reference DMTCP images,
a native tool workspace, or manual Go state. The controller does not implement
separate queues or attempt models for those mechanisms.

The long-lived Go worker always remains outside DMTCP. For R and Python, it
owns the DMTCP coordinator and interpreter process. For native and manual
adapters, it owns the operation's pause request and waits for a validated paused
result. If an adapter cannot produce and report a valid artifact before its
deadline, existing lost-work recovery remains authoritative.

Any historical section below that says every work item or one-shot Go child is
inside DMTCP is superseded by this strategy.

Local OS-002 evidence distinguishes two results. Explicitly privileged
WSL/Docker completed checkpoint, termination, fresh-container restore, and
continuation for both a control process and the actual Go-plus-Python tree.
Normal-user local SingularityCE 4.1.2 stopped at the control dump because its
effective capability set contained neither `CAP_CHECKPOINT_RESTORE` nor
`CAP_SYS_ADMIN`. Slurm job `13875979` reproduced that result on institutional
Ubuntu Jammy node `skl-010`: every capability mask visible inside
SingularityCE 4.1.2 was zero and `NoNewPrivs` was enabled. The ordered smoke
therefore stopped before the GOET probe, as designed.

## Purpose

Allow a GOET worker running under Slurm and Apptainer/Singularity to react to a
two-stage time-limit warning policy by:

1. entering graceful drain ten minutes before the allocation ends so it claims
   no more work while allowing the active work item to finish normally;
2. escalating five minutes before allocation end if the work item is still
   running;
3. asking the active work item's selected adapter to create a durable resume
   artifact;
4. stopping the active execution after that artifact is validated;
5. reporting the durable resume artifact to the controller; and
6. placing the logical work item back into pending state with a reference to
   that artifact so a later claim creates a new attempt that resumes it.

Every work item declares a worker-owned pause adapter. The common boundary is
the controller lifecycle and resume-artifact contract, not one universal
process-checkpoint mechanism.

In this Strategic Concept, **pause** means "write and validate durable
strategy-specific resume state, then stop the active execution." It does not
mean leaving a process alive after the Slurm allocation ends.

## Goals

- Make pause/resume a worker-owned work-item capability rather than
  checkpointing the whole long-lived worker or controller.
- Give every work-item type an explicit DMTCP, native-continuation, or
  manual-continuation adapter.
- Configure a ten-minute Slurm warning and propagate it through the batch shell
  and foreground container process to the Go worker.
- Put the worker into a one-way graceful-drain state at the ten-minute warning.
- Escalate to the selected pause adapter at the five-minute threshold when the
  active work item has not completed.
- Give each DMTCP attempt its own coordinator and checkpoint generation while
  keeping DMTCP's internal signal separate from the Slurm drain signal.
- Persist enough strategy-specific resume metadata for another compatible
  worker allocation to validate and resume the work.
- Preserve execution lineage while giving every resumed claim a new
  `attempt_id` and retaining the resume-artifact-producing attempt as history.
- Make pause creation, controller acknowledgement, resume, fallback, and
  cleanup observable.
- Prove each current work-item adapter, including direct Python DMTCP, rclone
  native continuation, and manual Go state.
- Require every future work-item type to declare and prove its pause/resume
  strategy before it is enabled in a pause-required runtime.
- Preserve current completion evidence, artifact promotion, assignment fencing,
  heartbeat, and CareTaker ownership rules.
- Treat checkpoint images as sensitive runtime state because they may contain
  credentials, protected values, input data, and process memory.

## Non-Goals

- Checkpointing the controller process.
- Checkpointing the entire Go worker process.
- Promising transparent restart for arbitrary network services, GPU state, MPI,
  open remote sessions, or external side effects.
- Rewinding an external API, object store, Google Drive, or other remote system
  to the state it had at checkpoint time.
- Replacing operation-level idempotency, atomic output promotion, or content
  verification.
- Using Slurm's job requeue as a second orchestration authority in the first
  implementation.
- Automatically resuming a checkpoint under a different DMTCP build,
  incompatible container image, architecture, or runtime contract.
- Publishing checkpoint images as workflow outputs or data assets.
- Writing implementation or test code while this Strategic Concept remains in
  design mode.

## Terminology

### Drain signal

A signal reserved for GOET's ten-minute Slurm time-limit warning path. The
initial design uses `SIGUSR1`.

DMTCP uses `SIGUSR2` as its default internal checkpoint signal. GOET must not
use DMTCP's internal signal as its drain signal or send that internal signal
directly to the payload.

### Graceful-drain threshold

The first warning stage, initially ten minutes before allocation end. The worker
stops claiming work, but an active work item continues running and may complete
normally.

### Pause threshold

The second warning stage, initially five minutes before allocation end. If the
active work item still exists, its supervisor invokes that work-item type's
pause adapter, validates the resume artifact, stops the active execution, and
reports the artifact to the controller.

The two thresholds are distinct worker lifecycle events. The portable initial
design requests one Slurm `SIGUSR1` warning at 600 seconds and arms a
worker-local five-minute escalation timer when that signal is received. This
avoids depending on multiple `--signal` values, which the Slurm `sbatch`
interface does not document as a portable multi-warning facility.

Because Slurm documents that its warning can arrive early, the local escalation
may also occur early. That increases the final pause/report safety margin; it
must never delay pausing past the expected five-minute threshold.

### Work-item execution

The active operation supervised by the long-lived worker for one work item.
Direct R and Python interpreters run as external DMTCP roots. Native tools such
as `rclone` run as owned subprocesses with operation-native continuation.
In-process Go handlers cooperate with a pause request and write manual
application-level state at safe boundaries.

The Go worker performs controller communication, worker-session heartbeats,
claim ownership, drain timing, pause reporting, and terminal reporting. It is
never part of a DMTCP computation.

### Resume artifact

One complete, immutable, validated set of state needed to continue an attempt's
logical execution. Its common manifest identifies the strategy and carries
strategy-specific references:

- DMTCP checkpoint images and compatibility facts for R or Python;
- a durable native-tool workspace and relaunch parameters for tools such as
  `rclone`; or
- explicit serialized operation state for an in-process Go handler.

### DMTCP checkpoint generation

The DMTCP-specific form of a resume artifact. A later pause after a resume
creates a new generation. The controller may select only a complete, validated
generation.

### Suspended attempt

A terminal historical attempt that produced a controller-accepted resume
artifact.
The logical work item returns to pending state with a reference to that
artifact.

### Resume attempt

A new attempt created when a worker claims pending work that carries a
resume-artifact reference. It records `resumed_from_attempt_id`,
`resume_artifact_id`, the strategy, and a stable execution-lineage ID.

A DMTCP-restored interpreter may retain the producing attempt ID in process
memory or values read before pause. Native and manual adapters may also carry
the producing attempt identity in their durable workspace. The new worker owns
the new attempt ID used for controller fencing and reporting. Evidence preserves
both identities rather than pretending resumed execution started fresh.

### Execution lineage

A stable identity shared by the initial attempt and all later resume attempts
that continue the same logical execution. Attempt IDs identify
controller scheduling/ownership segments; the execution lineage identifies the
continued work and its stable shared workspace.

### Fresh retry

A new attempt that starts the operation from the beginning. This is the
existing abandonment/requeue behavior and is distinct from resuming a suspended
attempt.

## Architectural Context

The relevant implemented path is:

```text
Slurm batch script
    -> singularity/apptainer exec
        -> Go worker
            -> Worker.Run
                -> operation-specific Go handler
                    -> optional child process
```

Concrete current artifacts include:

- `cmd/controller/slurm_worker_script.go`, which generates the batch script but
  does not request or trap a warning signal;
- `cmd/controller/runtime.go`, where `SingularityWorkerRuntime.WorkerScript`
  places the Go worker inside `singularity exec`;
- `cmd/worker/main.go`, where `runWorkerLoop` registers a worker session,
  heartbeats, claims work, runs it synchronously, and reports a terminal result;
- `cmd/worker/direct.go`, where the development-only `worker execute` command
  already runs one resolved work item and writes a local result without
  controller lifecycle calls;
- `cmd/worker/worker.go`, where `Worker.runWorkItem` dispatches by work-item
  type;
- `cmd/worker/work_python.go`, which directly starts and waits for a Python
  subprocess;
- `cmd/worker/work_asset_materialize.go`, which coordinates materialization in
  Go;
- `cmd/worker/data_asset_materializer.go`,
  `cmd/worker/gdrive_rclone_provider.go`, and
  `cmd/worker/archive_extractor.go`, which may use HTTP, `rclone`, or `7z`
  during parts of materialization; and
- the implemented controller CareTaker and worker-session persistence, which
  already fence stale owners and atomically abandon and freshly requeue work
  after a session dies.

GOET currently has no general plugin process interface. "Plugin" in this
concept means a work-item operation selected by the dispatch in
`Worker.runWorkItem`.

### External behavior that informs this design

- Slurm `--signal=[B:]<signal>@<seconds>` can send a warning before job end.
  `B:` sends it only to the batch shell, so the generated shell script must trap
  and forward it explicitly.
- DMTCP launches an application under `dmtcp_launch`, uses one coordinator per
  computation, creates `ckpt_*.dmtcp` files and a restart script, and resumes
  with `dmtcp_restart` or the generated restart script.
- DMTCP documents `SIGUSR2` as its default internal checkpoint signal; it is
  not an application control signal.
- DMTCP checkpoints enrolled descendants in the same computation. GOET uses
  that behavior only when an interpreter such as R or Python is the DMTCP root.
- DMTCP does not promise Go-runtime compatibility, and OS-001 rejected the
  one-shot Go-root boundary. In-process Go work therefore uses explicit
  application-level pause state.
- Apptainer documents a signal-forwarding init shim when PID isolation is used.
  GOET still requires an integration test for the exact site runtime flags and
  must not assume signal delivery from documentation alone.

Official references:

- [Slurm `sbatch` signal and requeue options](https://slurm.schedmd.com/sbatch.html)
- [DMTCP source and documented support scope](https://github.com/dmtcp/dmtcp)
- [DMTCP v4.2.0 release](https://github.com/dmtcp/dmtcp/releases/tag/v4.2.0)
- [DMTCP launch documentation](https://dmtcp.github.io/docs/)
- [DMTCP command, coordinator, and restart overview](https://dmtcp.github.io/manpages/dmtcp.html)
- [DMTCP restart documentation](https://dmtcp.github.io/manpages/dmtcp_restart.html)
- [DMTCP FAQ](https://dmtcp.github.io/FAQ.html)
- [Apptainer container init shim and signal propagation](https://apptainer.org/docs/user/latest/docker_and_oci.html#init-shim-process)

## Current State

### Strategic level

The controller owns durable work state, assignment ownership, worker-session
liveness, abandonment, and fresh requeue. A worker owns execution of one
claimed work item at a time.

The system can recover from a dead worker by fencing the old assignment and
starting the logical work again. It cannot preserve in-memory computation
progress across the loss of a Slurm allocation.

### Operational level

- Generated Slurm scripts may contain `#SBATCH --time`, but they do not contain
  `#SBATCH --signal`, a drain-signal trap, or explicit child PID forwarding.
- The batch script invokes the foreground worker/container command directly.
- `runWorkerLoop` has no operating-system signal subscription or drain state.
- `Worker.Run` is synchronous and has no context or execution-supervisor
  contract through which the loop can request a checkpoint.
- Production claims call `Worker.Run` inside the long-lived worker process.
  The existing `worker execute` one-shot mode is development-only and does not
  provide production source, lifecycle, heartbeat, or terminal-report behavior.
- The Python handler launches `python` directly with `exec.Command` and waits
  for it.
- `asset.materialize` is a Go handler. Wrapping an individual `rclone` or `7z`
  child would not preserve the Go call stack, provider phase, validation state,
  or promotion state.
- A worker session is either active, stopped, or dead. An assignment is
  currently queued, running, completed, failed, or abandoned/requeued; there is
  no suspended checkpoint state.
- Worker heartbeats continue during long-running work, and the controller
  rejects terminal reports from stale session owners.
- A dead-session recovery creates a fresh retry with a new attempt. There is no
  controller contract for putting pending work back in the queue with a
  checkpoint reference and creating a new resume attempt from it.
- Runtime configuration has no DMTCP executable, checkpoint root, checkpoint
  deadline, compatibility, or enablement settings.

## Target State

### Strategic level

GOET has a controller-recognized suspended-attempt and pending-resume lifecycle.
A worker can convert its running attempt into a durable resume artifact before
its Slurm allocation ends. The controller returns the logical work item to
pending state with that artifact reference. A compatible worker claim creates a
new attempt that resumes through the work item's declared adapter and completes
through the existing evidence and terminal-report path.

Every work item declares a DMTCP, native-continuation, or manual-continuation
strategy. Pause/resume compatibility is part of the runtime contract for every
current work-item type and for each future type before it is enabled in a
pause-required Slurm environment.

The controller remains the only authority that decides whether work is ready
for resume. Slurm supplies allocation lifecycle and the warning signal; it does
not independently duplicate GOET queue state.

### Operational level

#### Signal path

The generated Slurm script has behavior equivalent to:

```text
#SBATCH --signal=B:USR1@600
batch shell starts singularity/apptainer as a tracked foreground child
batch shell traps SIGUSR1
trap forwards SIGUSR1 to the tracked container/worker process
Go worker receives SIGUSR1 and enters graceful drain
worker arms a five-minute monotonic escalation timer
active work finishes before timer -> normal completion and worker exit
timer expires while work remains -> strategy-specific pause transition
```

The exact shell implementation must preserve the worker command's exit status,
forward ordinary termination signals, reap the child, and avoid a race between
child exit and trap execution.

`SIGUSR1` is the GOET graceful-drain signal. The pause threshold is an
internal lifecycle event, not `SIGUSR2`; DMTCP's internal checkpoint signal
remains reserved for DMTCP-backed adapters.

#### Worker drain behavior

The worker has one monotonic state machine:

```text
accepting
    |
    | ten-minute drain signal
    v
graceful draining
    |
    +-- idle --------------------------> graceful stop
    |
    +-- active work completes ----------> report completion, graceful stop
    |
    +-- five-minute threshold ----------> pausing
                                              |
                           +------------------+------------------+
                           |                                     |
                           v                                     v
                  controller accepts                        pause/report
                    resume artifact                         fails or times out
                           |                                     |
                           v                                     v
                  suspended and exit                   fallback and eventual
                                                        current dead-session
                                                        recovery
```

After entering graceful drain, the worker never claims another work item. Its
heartbeat continues until it reports normal completion, the controller accepts
the resume artifact, or the worker exits.

Signal handling does not run pause commands directly inside a Go signal
callback. It cancels or notifies the active execution supervisor, which
serializes completion, failure, and pause transitions.

#### Work-item execution supervisor

The worker owns one execution supervisor for the active item. Its conceptual
contract separates:

```text
prepare immutable execution request
    -> select the work-item pause adapter
    -> launch or resume the operation
    -> wait for completion, failure, or a validated paused result
    -> report result to controller
```

The supervisor owns:

- the active handler or payload process and any process group;
- the selected pause adapter;
- any per-attempt DMTCP coordinator or native-tool process;
- the durable resume-artifact workspace;
- stdout and stderr files;
- the current execution phase;
- completion/pause mutual exclusion;
- pause and termination deadlines; and
- production of the common resume-artifact manifest.

DMTCP-backed attempts use an isolated computation. They must not accidentally
join the default coordinator on port 7779 or another work item. Native and
manual adapters do not start a DMTCP coordinator.

The process boundary is conceptually:

```text
current:
long-lived worker -> Worker.Run -> Go operation handler -> optional descendants

target direct R/Python:
long-lived worker -> DMTCP coordinator -> dmtcp_launch interpreter
replacement worker -> validate manifest -> dmtcp_restart interpreter tree

target native tool:
long-lived worker -> Go adapter -> rclone or another owned tool
replacement worker -> validate workspace -> Go adapter relaunches tool

target in-process Go:
long-lived worker -> Go handler writes explicit safe-boundary state
replacement worker -> Go handler loads that state and continues
```

The existing `worker execute` path remains useful as a development harness, but
the production pause contract is an operation interface rather than a universal
one-shot Go child.

The long-lived worker remains outside every DMTCP computation so its controller
HTTP connection, worker session, heartbeat state, drain timer, and assignment
owner are not restored from stale process memory. Only a directly launched R or
Python interpreter and its enrolled descendants are inside DMTCP.

Before launch, the worker writes immutable, attempt-scoped inputs that an
external adapter or resumed operation can consume without a controller
connection:

```text
resolved work item
runtime configuration snapshot
source bundle or source handoff evidence when required
result-envelope path
resume-artifact/workspace identity
```

The operation writes or returns a versioned result envelope atomically. It
contains ordinary `WorkEvidence`, a structured execution error, or a validated
paused result. Only the worker uses that result to call controller APIs.

#### Pause creation

At the five-minute pause threshold, when work is still running, the supervisor:

1. stops accepting an ordinary terminal transition;
2. invokes the work item's selected pause adapter;
3. waits for the adapter to reach a safe boundary and write its complete
   strategy-specific state;
4. validates the DMTCP generation, native workspace, or manual state;
5. writes an immutable GOET resume-artifact manifest last;
6. terminates any process that the adapter has not already stopped;
7. reports the manifest to the controller using the current assignment owner;
   and
8. exits only after controller acknowledgement or after its bounded reporting
   policy is exhausted.

For DMTCP, the supervisor requests a checkpoint, waits for finalized images,
validates the complete client set, and explicitly terminates the computation
and coordinator. For rclone, the adapter stops the process only at a defined
durable partial-transfer boundary and records the validated workspace and
relaunch inputs. For a Go handler, the handler returns only after atomically
writing a complete manual state record.

The manifest-last rule makes a directory without a valid manifest incomplete
and ineligible for resume.

#### Resume-artifact persistence and security

Resume state lives in a configured durable root visible to both the original
and replacement allocation, such as a protected shared filesystem. Large state
does not travel through the controller API.

The agreed initial storage root is the worker's shared temporary mount, exposed
at the same absolute path inside every worker container. GOET owns subdirectories
under that mount:

```text
<shared_tmp>/goetl/work/<execution-lineage-id>/
<shared_tmp>/goetl/resume/<resume-artifact-uuid>/
```

`resume-artifact-uuid` is controller- or worker-generated random identity
recorded in the manifest. Resume and work paths must not depend on a node-local
`/tmp`. DMTCP-backed execution preserves the same Singularity bind destination
so restored process memory and open file descriptors refer to valid locations.

`asset.materialize` performs acquisition, partial download, archive extraction,
and other incomplete work under the shared execution-lineage workspace. It
promotes only verified completed content to the existing materialization/cache
destination. A replacement allocation therefore sees the accepted resume
artifact and its attempt-local partial files.

A versioned manifest records at least:

```text
schema
resume_artifact_id
resume_generation
pause_strategy
work_item_id
producing_attempt_id
work_item_type
input_fingerprint
source/code version
created_at
worker version
container image identity
operating-system/architecture compatibility facts
storage scope
strategy-specific relative paths and compatibility facts
size and SHA-256 for every required file
execution workspace identity
execution_lineage_id
```

For DMTCP, strategy-specific facts include the exact DMTCP build, complete
checkpoint set, and restart inputs. For rclone, they include the operation,
remote/backend compatibility identity, partial workspace, and validated
relaunch parameters. For Go, they include the manual state schema and handler
version.

The manifest contains only safe metadata. Resume files may be opaque sensitive
blobs, use restrictive permissions, are never emitted into ordinary logs, and
are never exposed as public workflow artifacts. Pause-required execution is
rejected when no storage root satisfying the configured protection policy is
available.

Protected references are allowed only when the shared temporary mount provides
owner-only access. GOET resume directories use mode `0700` and resume files use
mode `0600`, subject to the HPCC filesystem's ACL policy. DMTCP images can
contain plaintext secrets from process memory, environments, or materialized
secret files; native and manual state may also contain protected data. Hashing
or hiding the manifest does not remove that content.

The DMTCP adapter constructs an explicit `dmtcp_restart` argument vector from
validated manifest entries. It does not execute a generated restart shell
script as an unvalidated command. Native and manual adapters likewise construct
typed relaunch/resume inputs rather than evaluating stored shell commands.

Retention keeps every controller-accepted generation referenced by pending
resume work or an active resume attempt. Incomplete, superseded, completed,
failed, or abandoned resume generations become cleanup candidates only after
durable controller state no longer references them.

#### Runtime and image baseline

The first institutional target is:

```text
SingularityCE 4.1.2 on Ubuntu Jammy
```

The repository fake-HPCC image also verifies SingularityCE 4.1.2, but currently
does so on a Rocky Linux 9 Slurm base. That smoke is useful but does not replace
an early test against the Jammy target.

Pin DMTCP `v4.2.0` in interpreter images that enable the DMTCP adapter. The
image build records the release source checksum and verifies the installed
version. A DMTCP resume manifest records the exact build identity; resume
requires an exact match unless a later compatibility policy has direct evidence
for another build.

The standard Go worker and GDAL images do not need to run Go inside DMTCP.
OS-001 remains evidence that this boundary is unsupported. The long-lived Go
worker supervises only the directly launched R or Python DMTCP computation.

#### Open-file restart policy

Each pause adapter must reconcile stdout/stderr, partial outputs, and work-item
files at its safe boundary. For DMTCP, filesystem bytes live outside process
memory: an interpreter can write bytes after the checkpoint snapshot and before
GOET terminates it, then write those bytes again after restart from the earlier
file offset. Native and manual adapters instead record their own verified
durable boundaries.

The initial policy is:

- all mutable work-item files remain under the shared execution-lineage
  workspace until verified promotion;
- the resume manifest records the path, size, and hash or safe length
  boundary of mutable regular files at checkpoint completion;
- resume reconciles only attempt-local mutable files according to the selected
  adapter before restoration or relaunch;
- completed/promoted outputs and external destinations are never truncated as a
  generic restart action;
- the child result envelope is an atomic write performed only after execution
  completes; and
- each DMTCP interpreter slice tests `--ckpt-open-files`, inherited
  stdout/stderr offsets, and the duplicate-write boundary before production
  enablement.

Operation-specific quiesce may still be required when a file cannot be safely
reconciled from generic metadata.

#### Controller attempt lifecycle

The target lifecycle is:

```text
queued
  |
  | fresh claim creates attempt A
  v
running attempt A, owner session 1
  |
  | accepted pause report
  v
suspended attempt A (historical)
pending work item -> resume artifact N, execution lineage L
  |
  | resume claim
  v
running attempt B, owner session 2
resumed_from_attempt_id=A, resume artifact=N, execution lineage=L
  |
  +-- complete --> completed attempt B
  |
  +-- fail with cause --> failed attempt B, no automatic causal retry
  |
  +-- pause again --> suspended attempt B
                      pending work -> resume artifact N+1, lineage L
                      later claim creates attempt C
```

Each claim gets a new attempt ID and uses the existing session-bound ownership
fence. Completion, failure, heartbeat-associated ownership, and pause
reports require:

```text
attempt_id
worker_id
worker_session_id
```

Attempt A becomes historical before attempt B is created, so its former owner
cannot report against B. The resume-artifact record links the attempts without
weakening the existing session fence.

Resumed execution can still contain attempt A's original environment,
workspace, or serialized values. Its stable workspace and result evidence
therefore use execution lineage L. The worker for attempt B reports
`attempt_id=B`, `resumed_from_attempt_id=A`, and lineage L to the controller.

The controller transaction that accepts a pause:

1. verifies current assignment ownership;
2. validates manifest identity and compatibility metadata shape;
3. records the resume-artifact generation and strategy;
4. records the producing attempt as suspended history;
5. removes its running worker-session owner;
6. inserts the logical work item into pending state with the accepted
   resume-artifact reference and execution lineage; and
7. signals the CareTaker after commit.

Pending resume work contributes to ordinary worker demand. It receives no
priority over fresh pending work; both use normal FIFO ordering. A claim creates
a new attempt, records its predecessor/artifact/lineage, and returns the
resume-artifact manifest with the work item.

The first implementation does not call `scontrol requeue`. Avoiding simultaneous
Slurm requeue and GOET resume prevents two schedulers from independently
creating replacement execution.

#### Resume compatibility

A worker fails closed before invoking the selected adapter unless the resume
artifact matches the required:

- work item, producing attempt, new attempt predecessor link, and
  execution lineage;
- operation adapter;
- source and input fingerprints;
- exact DMTCP build for DMTCP artifacts or exact native/manual adapter version;
- worker execution contract version;
- container image identity;
- CPU architecture and required runtime features; and
- complete file list and hashes.

Path layout must be stable or deliberately rewritten by a tested adapter.
DMTCP restores process memory and original environment; a new worker must not
assume that ordinary environment replacement changes values already read by the
payload.

If an artifact is incompatible or corrupt, the controller records why resume
was rejected and the causal failure is terminal. An operator may explicitly
request a fresh attempt, but the controller does not automatically retry work
that failed for a known cause and must not present a fresh start as a successful
resume.

#### Retry policy

GOET keeps its existing distinction:

- a work-item failure with a reported cause is terminal and is not
  automatically retried;
- loss of a worker/session remains lost-work recovery and may requeue work;
- failure while creating a pause artifact falls back to existing lost-work
  handling when the allocation ends; and
- resume-launch/restart failures may retry the same accepted artifact only up
  to `resume_attempt_limit`.

The initial default for `resume_attempt_limit` is `3`, configurable
per submitted worker job. Each new attempt that consumes the same artifact
counts toward the limit. At exhaustion, the logical work fails with
`resume_attempt_limit_exhausted`; it does not silently start from the beginning.

#### Status and administrative drain

Durable state distinguishes:

```text
suspended attempt history
pending_resume work
running resume attempt
completed or failed attempt
```

Status/events may expose:

```text
draining
pausing
suspended
resuming
resume_rejected
```

`suspended` is a user-facing summary for pending work with an accepted resume
artifact, not an attempt ID that will later be re-owned.

Include an authenticated administrative drain request for testing and
operations. Because workers use outbound controller communication, the
controller records the request and returns a drain directive on the worker's
next heartbeat or control poll. The worker feeds that directive into the same
idempotent state machine as `SIGUSR1`; this is not a second pause
implementation.

#### Per-work-item adapter compatibility

There is one GOET controller lifecycle for pause/resume, but no universal
process boundary. Each registered work-item type declares one adapter and
receives focused pause/resume tests.

Direct R and Python use DMTCP around the interpreter, never around Go. R has
passing evidence; Python still requires a direct-interpreter proof under the
institutional runtime.

`asset.materialize` uses native or manual continuation according to its active
phase. Its tests separately cover:

- local copy;
- HTTP download;
- `gdrive_rclone` acquisition;
- archive listing/extraction; and
- final verified destination promotion.

Rclone continuation is owned by the rclone operation adapter. The adapter
terminates the process, retains and validates its partial-transfer workspace,
and relaunches through the exact native continuation semantics proven for the
configured rclone command and remote. This does not imply that every rclone
backend or command resumes identically.

In-process Go work serializes logical progress rather than process memory. Each
handler defines safe pause points and a versioned state schema. Atomic verified
promotion remains handler-owned and cannot be replayed from an unsafe midpoint.

There is one GOET resume lifecycle: claim accepted resume state and invoke the
declared adapter. DMTCP restore, rclone relaunch, and manual Go continuation do
not create separate controller queues, attempt models, or independent resumers.

A pause-required runtime
profile is not considered complete until all work-item types enabled in that
profile have passing compatibility evidence. A newly added type cannot bypass
this rule.

## Core Invariants

### Invariant 1: One selected pause adapter per active attempt

An attempt has exactly one declared adapter and one isolated resume workspace.
DMTCP attempts also have isolated coordinators and checkpoint directories.

### Invariant 2: Every work-item type has a versioned pause contract

The contract names the strategy, safe pause boundary, resume-artifact schema,
compatibility checks, and fallback behavior. Manual Go handlers may run in the
worker process; interpreter and native-tool adapters own their subprocesses.

### Invariant 3: The Go worker remains outside DMTCP

Only a directly launched interpreter and its enrolled descendants enter a
DMTCP computation. Controller connection, heartbeat, signal subscription, and
worker-session identity belong to the current worker on every allocation.

### Invariant 4: Drain and pause escalation are monotonic

After receiving the ten-minute drain signal, a worker never returns to accepting
work and never claims another item. After crossing the five-minute pause
threshold, it never returns to ordinary execution for that attempt.

### Invariant 5: One winning transition

For one active attempt, exactly one of completion, failure, pause, or
controller abandonment may become authoritative. All later reports are fenced.

### Invariant 6: Resume state is not usable before acknowledgement

Strategy-specific files without a complete manifest and controller acceptance are
orphaned evidence, not queued resume work.

### Invariant 7: Resume creates a new attempt and preserves execution lineage

A resume claim creates a new attempt linked to the resume-artifact-producing
attempt. The continued work preserves its execution-lineage identity. A fresh
retry is also a new attempt, but it has no resume-artifact reference and never
claims to have resumed.

### Invariant 8: Compatibility is verified before process creation

The worker validates manifest identity, hashes, strategy and handler versions,
container/runtime identity when applicable, architecture, and storage
accessibility before invoking the selected adapter.

### Invariant 9: Resume artifacts are sensitive

Resume artifacts receive access controls and retention handling at least as
strict as the protected values and data they may contain.

### Invariant 10: External effects remain operation-owned

DMTCP restoration, native relaunch, and manual continuation do not prove that
an external service can be safely replayed. Each operation/provider documents
and tests its pause-safe boundary.

### Invariant 11: Controller state remains authoritative

Slurm signals allocation pressure, and adapters create resume artifacts. Only
the controller decides whether an attempt is running, suspended, completed,
failed, or abandoned and whether its logical work is pending fresh execution or
pending resume from an accepted artifact.

## Failure and Race Behavior

### Signal while idle

The worker stops heartbeats in the normal order, reports a graceful
`slurm_drain_idle` stop, and exits without claiming work.

### Ten-minute warning while work is active

The worker stops claiming work and allows the active operation to continue. If
the operation completes before escalation, the worker reports the ordinary
completion or failure and exits gracefully.

### Five-minute threshold before operation launch

Attempt preparation must be deliberately small and bounded. If no active
operation exists at the pause threshold, the worker records the diagnostic,
exits, and current dead-session abandonment creates a fresh retry.

### Completion races with pause threshold

The execution supervisor serializes the local transition. If a valid result
wins, the worker reports it normally. If pausing wins, the worker does not also
report ordinary completion from that attempt.

The controller independently verifies ownership so only one durable transition
wins.

### Pause succeeds but controller report fails

The worker retries the report within the remaining drain budget while
heartbeating if possible. If the controller never accepts it, the resume
artifact is orphaned and must not be resumed automatically. Session expiry then
uses the existing abandonment/fresh-retry path.

### Controller accepts pause but worker does not exit

The accepted transaction removes the old assignment owner. All later outcome
reports from that owner are fenced. The batch script and Slurm time limit remain
the final process cleanup boundary.

### Resume launch fails

The new resume attempt records a resume-launch failure. If the artifact's
configured resume-attempt limit is not exhausted, the logical work returns to
pending with the same accepted artifact and the next claim creates another
new attempt. At the limit, the work fails with
`resume_attempt_limit_exhausted`. It does not silently convert to fresh work.

### Resume artifact corrupt or incompatible

The worker does not invoke the selected adapter. The controller records the
rejected compatibility reason. An operator or explicit policy may request a
fresh retry.

### Repeated drain after resume

The resumed attempt may create a new immutable generation. The controller
switches its resume reference atomically only after validating and accepting
the newer manifest.

## Configuration Direction

Names are provisional until Operational Slice design. Configuration needs to
represent:

```text
Slurm warning signal
Slurm graceful-drain lead time (initial target: 600 seconds)
worker pause-before limit (initial target: 300 seconds before job end)
work-item pause strategy and enablement
DMTCP launch/command/restart paths for interpreter adapters
native-tool relaunch configuration
manual-state schema/version configuration
shared temporary mount
pause creation timeout
controller-report reserve within the warning window
termination grace period
compatibility policy
resume-artifact retention policy
resume-attempt limit (initial default: 3)
pause-adapter protocol version
```

Expose the two lifecycle thresholds through command-line/startup overrides with
names equivalent to:

```text
--worker-drain-before=10m
--worker-pause-before=5m
```

The first value generates Slurm's warning lead time. The worker derives its
local escalation delay from the difference. Validation requires:

```text
drain_before > pause_before > report_reserve + termination_grace
```

The ten-minute warning begins graceful drain. The first five minutes are the
normal-finish opportunity. The remaining time is a pause budget, not five full
minutes for writing state: validation, controller reporting, and process
termination require reserved time. Configuration validation rejects a pause
timeout that consumes the entire remaining allocation window.

Target HPCC nodes have 128 GiB of memory, so GOET must not assume every
DMTCP image or other resume artifact fits within the default time budget. If
pause creation fails or times out, the worker records the diagnostic and
existing lost-work recovery remains authoritative. Users can increase both
lifecycle thresholds for memory-heavy jobs through command-line/startup
overrides.

## Slice History And Candidates

Implemented and blocked slices retain their evidence here. Later entries remain
planning candidates until they receive an approved Operational Slice charter.

1. **Pinned DMTCP image and Go-process feasibility gate**
   - Future production artifacts:
     `containers/goetl-worker/Dockerfile` and
     `containers/goetl-worker-gdal/Dockerfile`.
   - Pin DMTCP `v4.2.0`, produce the required dynamically linked one-shot GOET
     child, and verify the exact versions in both image tests.
   - Future test artifact: an early Linux/Singularity fixture that checkpoints
     and restarts the actual compiled one-shot child during controlled
     in-process Go work, including open files and the completed direct result
     envelope.
   - Run against SingularityCE 4.1.2, with the Ubuntu Jammy institutional target
     as required evidence. Stop for an architecture decision if the pinned
     DMTCP build cannot safely restore the Go runtime.

2. **HPCC CRIU capability and Go-process-tree feasibility gate**
   - Operational Slice charter:
     `002-hpcc-criu-capability-and-go-process-tree-feasibility.md`.
   - Pin CRIU `v4.2.1` in the standard worker image and exercise it as the
     normal institutional HPCC user under SingularityCE 4.1.2.
   - Status: blocked. Local privileged control-tree and actual Go-plus-Python
     restore passed, but institutional Slurm job `13875979` failed at the
     control dump because the normal Singularity process had neither required
     effective capability.
   - Require an actual dump, original-tree termination, and restore of the
     existing `worker execute` Go process plus its Python descendant in a fresh
     Singularity invocation.
   - Treat a reproducible failure as valid gate evidence only when it
     classifies the exact host capability, kernel feature, security policy,
     namespace behavior, process resource, or Go restore incompatibility.
   - Do not implement later CRIU slices unless this gate passes under an
     institutionally approved capability model.

3. **R tidyverse and BRMS DMTCP feasibility gate**
   - Operational Slice charter:
     `003-r-tidyverse-brms-dmtcp-feasibility.md`.
   - Status: implemented. Institutional jobs `13947170` (`rstan`) and
     `13946899` (`cmdstanr`) both passed under normal-user SingularityCE 4.1.2.
   - Launch `Rscript` directly under pinned DMTCP `v4.2.0`, leaving the Go
     worker outside the checkpoint.
   - Run a real tidyverse transform and active single-chain BRMS sampling under
     both `rstan` and `cmdstanr`.
   - Require checkpoint, original-computation termination, fresh-Singularity
     restore, completed fit validation, and normalized comparison with an
     uninterrupted fixed-seed baseline on the institutional Slurm target.
   - Permit an R-specific supervisor design only for backends that pass; do not
     infer universal work-item checkpoint compatibility.

4. **Direct-Python DMTCP feasibility gate**
   - Operational Slice charter:
     `004-direct-python-dmtcp-feasibility.md`.
   - Status: implemented. Institutional job `13966757` passed under
     normal-user SingularityCE 4.1.2.
   - Launch the Python interpreter directly under the pinned DMTCP runtime,
     leaving Go outside the computation.
   - Prove pure Python plus NumPy native work and one Python descendant under
     normal-user institutional Singularity.
   - Require checkpoint, original-computation termination, fresh-invocation
     restore, and exact deterministic output comparison.

5. **Resume-artifact contract model**
   - Operational Slice charter:
     `005-resume-artifact-contract-model.md`.
   - Status: implemented.
   - Define the shared immutable manifest and controller-facing reference with
     common identity, lineage, compatibility, storage, integrity, and retention
     fields.
   - Use typed DMTCP, native-tool, and manual-Go payloads with exactly one
     payload matching the selected strategy.
   - Defer the behavioral worker adapter interface until the worker-supervisor
     slice, where it will consume and produce this shared model.

6. **Controller suspended-attempt and pending-resume lifecycle**
   - Persist accepted resume artifacts, suspended attempt history, execution
     lineage, predecessor links, and configurable resume-attempt limits.
   - Create a new attempt for every resume claim while retaining one FIFO work
     queue and one controller state machine.

7. **Worker drain state and adapter supervisor**
   - Add `SIGUSR1` drain handling, the worker-owned five-minute pause timer,
     completion/pause mutual exclusion, bounded reporting, and administrative
     drain control.
   - Keep the worker outside DMTCP and dispatch pause/resume through the selected
     work-item adapter.

8. **R and Python DMTCP adapter**
   - Launch supported interpreters under isolated DMTCP coordinators and restore
     validated generations from compatible images and stable mounts.
   - Enable only interpreter/package/process shapes with passing evidence.

9. **Rclone native-continuation adapter**
   - Terminate the owned rclone process at pause, validate its durable partial
     workspace, and relaunch it through the exact continuation contract proven
     for each enabled command/backend.
   - Preserve provider idempotency and final verified promotion.

10. **Manual Go continuation contract and work-item adoption**
    - Add cooperative pause points and versioned manual state for in-process Go
      handlers without serializing Go memory.
    - Adopt work-item families one focused slice at a time, starting with a
      deterministic long-running reference handler.

11. **Cross-worker compatibility, security, retention, and operations**
    - Prove each enabled adapter across allocations and replacement workers.
    - Complete sensitive-state protection, cleanup, status, timeout, fallback,
      and operator documentation.

### Superseded Universal-DMTCP Decomposition

The following original candidates are retained as design history. They are not
current implementation authority.

1. **Universal production one-shot work-item boundary (historical)**
   - Future production artifacts in `cmd/worker/main.go`,
     `cmd/worker/direct.go`, `cmd/worker/worker.go`, and a focused child
     request/result protocol file.
   - Move every work-item dispatch into a supervised child invocation of the
     GOET binary while keeping controller communication and heartbeats in the
     parent.
   - Preserve ordinary completion/failure behavior for every current type
     before adding the selected checkpoint backend.

2. **Checkpoint manifest and pending-resume attempt model (historical)**
   - Future production artifacts under `internal/model` and
     `internal/persistence`.
   - Define the versioned manifest, checkpointed attempt history, pending
     checkpoint reference, execution lineage, predecessor-attempt link,
     transactions, indexes, and compatibility facts before checkpoint
     execution is wired.

3. **Worker drain state and execution supervisor (historical)**
   - Future production artifacts in `cmd/worker/main.go`,
     `cmd/worker/worker.go`, and a focused new execution-supervisor file.
   - Add signal subscription, ten-minute graceful drain, five-minute
     escalation, process-group ownership, and mutually exclusive result /
     checkpoint transitions.
   - Include the authenticated controller drain request and outbound
     heartbeat/control delivery so tests and operators invoke the same state
     machine without a Slurm deadline.

4. **Checkpoint adapter and isolated per-attempt process tree (historical)**
   - Future production artifact: a focused checkpoint adapter owned by the
     reusable work-item runner.
   - If OS-002 passes, use CRIU to launch or adopt, checkpoint, validate,
     terminate, and restore only the one-shot work-item process tree.

5. **Controller checkpoint acceptance and resume claim (historical)**
   - Future production artifacts in controller handlers, CareTaker demand, and
     persistence.
   - Atomically checkpoint an owned attempt, return its work item to normal
     pending order with a checkpoint reference, and create a new linked attempt
     on resume claim.
   - Keep normal FIFO ordering and enforce the configurable resume-attempt
     limit without adding causal work retries.

6. **All-work-item checkpoint/resume compatibility (historical)**
   - Future test artifacts cover every registered work-item type through the
     common child boundary.
   - Include descendant-process coverage for Python, `rclone`, and `7z`;
     in-process Go coverage; result-envelope correctness; atomic promotion; log
     behavior; and provider-specific reconnect or quiesce behavior.
   - Retain one GOET resume lifecycle; provider-native continuation remains an
     internal hook rather than a separate controller resumer.
   - A checkpoint-required runtime profile cannot enable a work-item type
     without this evidence.

7. **Cross-allocation all-work-item smoke (historical)**
   - Future test/documentation artifacts include deterministic long-running
     fixtures for each work-item family plus fake/real Slurm evidence.
   - Prove graceful completion between ten and five minutes, checkpoint at the
     second threshold, controller acknowledgement, old-worker exit,
     replacement allocation, resume, completion, and stale-owner fencing.

8. **Checkpoint security, retention, status, and operator documentation
    (historical)**
   - Future production artifacts in status APIs and cleanup policy.
   - Future documentation artifacts in `PROJECT_STATE.md`,
     `docs/RUNTIME_RUNBOOK.md`, `docs/TEST_AND_SMOKE_STATUS.md`, and the
     relevant deployment/container documentation.

## Delivery Cadence

Selected cadence:

```text
C(SI)x
```

After the Strategic Concept is explicitly approved as `Ready`, design and
implement one Operational Slice at a time. Stop for human review and acceptance
before creating the next slice. OS-001 blocked the Go-rooted DMTCP path, and
OS-002 blocked the CRIU path under the current institutional capability policy.
OS-003 is implemented and approves DMTCP feasibility only for its direct,
single-chain, single-core RStan and CmdStanR shapes. OS-004 is implemented and
approves the tested direct CPython 3.11, NumPy 2.4.6, one-native-thread,
one-descendant shape. OS-005 is implemented and defines the shared immutable
resume-artifact manifest/reference model with typed DMTCP, native, and manual
payloads. No pause adapter is wired into the production worker yet. The next
candidate is the controller suspended-attempt and pending-resume lifecycle; it
requires a separately reviewed charter before implementation.

The selected implementation HCI specification is:

```text
EC-3 / Operational Slice / file(1)+test+doc+newfile
```

## Agreed Decisions

1. Every resume claim creates a new `attempt_id`. The pending work record
   references the accepted resume artifact, producing attempt, and stable
   execution lineage.
2. Resume artifacts and mutable work-item state use GOET-owned UUID subdirectories
   under the shared temporary mount exposed to worker containers.
3. The first institutional runtime target is SingularityCE 4.1.2 on Ubuntu
   Jammy.
4. Slurm sends one `SIGUSR1` warning at ten minutes. The worker owns the
   five-minute graceful-drain timer and pause escalation.
5. OS-001 rejects DMTCP as the production backend for the required Go
   work-item process tree. Its pinned build and failure evidence remain until a
   later cleanup slice.
6. OS-002 rejects CRIU under the current institutional normal-user policy.
   Privileged local Go-plus-Python restore passed, but institutional
   Singularity exposes neither `CAP_CHECKPOINT_RESTORE` nor `CAP_SYS_ADMIN`;
   the ordered smoke is blocked at its control dump.
7. Target nodes have 128 GiB of memory. The ten-minute and five-minute
   thresholds have command-line/startup overrides. Failed or timed-out
   pause creation relies on existing lost-work recovery.
8. Pending resume work receives no special priority; it uses the normal
   FIFO queue with fresh pending work.
9. Known work-item failures are not automatically retried. Resume attempts that
   consume an accepted artifact have a configurable limit, initially `3`.
10. Work using protected references may be paused only into an owner-only
    shared temp directory because its resume state can contain plaintext
    process memory, environment values, or secret files.
11. `asset.materialize` writes incomplete acquisition and extraction state to
    its shared execution-lineage temp workspace and promotes only verified
    completed content.
12. GOET has one controller-level resume lifecycle with DMTCP, native-tool, and
    manual-Go adapters. There is no separate controller queue or resumer for
    rclone.
13. Status/events may expose `draining`, `pausing`, `suspended`,
    `resuming`, and `resume_rejected`, backed by precise durable attempt and
    pending-work state.
14. An authenticated administrative drain request is included so the same
    state machine can be tested or invoked operationally without waiting for a
    Slurm deadline.
15. Open files use the shared workspace and manifest-recorded safe boundaries.
    CRIU resource handling for the controlled fixture is empirical evidence in
    OS-002; active external sessions remain outside that gate.
16. The original Slurm batch script starts a lightweight worker container and
    one reusable work-item container. Neither container invokes `srun` or
    `sbatch` after startup.
17. One worker owns at most one active work item. The reusable work-item
    container runs one active operation at a time; multiple workers provide
    parallelism.
18. SingularityCE supplies container lifecycle, not durable checkpointing.
    OS-002's provisional CRIU boundary is blocked under the institutional
    capability policy. For supported R and future proven Python paths, the
    worker and any fresh runner service remain outside DMTCP.
19. OS-003 passed DMTCP around directly launched R/tidyverse/BRMS work. The Go
    worker remains outside the checkpoint, model compilation happens before
    checkpointing, native thread pools are limited to one, and both `rstan` and
    `cmdstanr` have separate passing institutional evidence.
20. Direct R and Python use DMTCP within the exact execution shapes proven by
    OS-003 and OS-004. Rclone uses operation-native continuation: GOET
    terminates the owned process, retains validated partial state, and
    relaunches it. In-process Go operations implement manual application-level
    pause/resume state rather than process-memory checkpointing.

## Remaining Validation Questions

1. Which additional Python package, native-extension, or descendant shapes
   must pass their own compatibility evidence before they can use the
   direct-Python DMTCP adapter?
2. Will the institution support a bounded CRIU capability model that supplies
   `CAP_CHECKPOINT_RESTORE` or `CAP_SYS_ADMIN` inside the work-item container,
   if universal non-R process-tree checkpointing is revisited?
3. What minimal pause-adapter protocol keeps controller communication outside
   DMTCP while supporting DMTCP, native-tool, and manual-Go strategies?
4. Which SingularityCE 4.1.2 PID/init flags on the Ubuntu Jammy target provide
   the required `SIGUSR1` propagation to the lightweight worker container?
5. What measured pause time, validation reserve, and termination reserve
   are safe for representative memory sizes up to the 128 GiB node limit?
6. Does the target shared temporary filesystem enforce the required owner-only
   modes/ACLs and preserve stable absolute paths across allocations?
7. Which exact rclone commands and configured remotes preserve partial state
   across termination, and what files or flags prove native continuation rather
   than a fresh transfer?
8. Does the interpreter supervisor preserve or safely reconcile inherited
   stdout/stderr, partial regular files, and file offsets without duplicate
   writes after restore?
9. What versioned safe-boundary state is required for each in-process Go
   operation, beginning with the first deterministic long-running reference
   handler?
10. Which representative BRMS models, multiple-chain modes, or Stan threading
   configurations can be added without violating the one-core OS-003 support
   boundary?

## Current Completion Criteria

- The generated Slurm script requests and forwards the ten-minute warning; the
  worker enters monotonic drain and invokes its five-minute pause timer.
- The controller persists one common resume-artifact model, suspended attempt
  history, pending-resume state, execution lineage, predecessor links, and the
  configurable resume-attempt limit.
- Every resume claim creates a new fenced attempt and invokes exactly the
  strategy declared by the work-item type.
- Direct R and Python run under isolated DMTCP coordinators with the Go worker
  outside the computation. Each enabled interpreter/package/process shape has
  target-runtime checkpoint, termination, cross-worker restore, and completed
  output evidence.
- Rclone-backed operations prove termination and native continuation for every
  enabled command/remote combination without duplicating remote effects or
  replaying final promotion.
- Each enabled in-process Go operation defines versioned manual state, atomic
  safe pause boundaries, compatibility validation, and cross-worker
  continuation tests.
- Other work-item types cannot enter a pause-required runtime until they declare
  and prove DMTCP, native, or manual continuation.
- All adapters use the same controller queue and attempt lifecycle, but retain
  strategy-specific compatibility and integrity facts.
- Failed or timed-out pause creation falls back to existing lost-work recovery;
  known causal work failures do not gain automatic retry.
- Resume artifacts are owner-only, omitted from ordinary logs/public artifacts,
  retained while referenced, and safely cleaned when no durable state needs
  them.
- Administrative drain and real Slurm warning tests cover completion/pause
  races, stale-owner fencing, replacement workers, repeated pause after resume,
  corrupt/incompatible artifacts, and controller restart.
- Project, runtime, container, status, security, and operator documentation
  names the supported adapter and compatibility matrix.
- All approved Operational Slices are implemented and accepted, and the human
  explicitly approves moving the Strategic Concept to `Implemented`.

## Historical Universal Completion Criteria

The criteria below belong to the original all-work-item DMTCP design. OS-001
made its Go-rooted DMTCP requirement unattainable, so this list is retained for
design history and is not a current implementation checklist. OS-003 passed
the narrow direct-R gate; subsequent slices use the per-work-item adapter
strategy rather than the universal Go-rooted assumptions below.

- A generated Slurm script requests the ten-minute warning and reliably
  forwards it through the actual Apptainer/Singularity runtime to the parent Go
  worker.
- The worker stops claiming work at the ten-minute warning, allows the active
  child to finish normally for five minutes, and exits normally when it does.
- An active child that remains at the five-minute threshold enters checkpoint
  exactly once, leaving reserved time for validation, controller reporting, and
  termination.
- Every registered work-item type runs in the common production one-shot child;
  the parent never invokes `Worker.Run` directly for a claimed item.
- The actual compiled Go one-shot child passes DMTCP checkpoint/restart in the
  pinned DMTCP `v4.2.0` worker container under SingularityCE 4.1.2 before
  pending-resume persistence is considered ready.
- Every active attempt uses an isolated DMTCP computation and private
  checkpoint directory containing the one-shot child and all descendants.
- The worker can checkpoint and terminate deterministic fixtures for every
  registered work-item family, produce a complete hashed manifest, and report
  it before the allocation ends.
- The controller atomically records the owned running attempt as checkpointed
  history, returns its logical work to pending FIFO order with the checkpoint
  reference, and rejects all later terminal reports from the prior owner.
- A compatible replacement worker claim creates a new attempt linked to the
  producing attempt and execution lineage, validates the checkpoint/runtime
  identity, resumes it with DMTCP, and reports ordinary completion evidence
  under the new attempt.
- A second drain after resume creates and can resume a newer checkpoint
  generation.
- Missing, partial, corrupt, incompatible, or unacknowledged checkpoints never
  start automatically.
- `python_script`, `asset.materialize`, `commit_data`, and every in-process Go
  operation enabled by the worker have focused checkpoint/resume evidence.
- Local, HTTP, `gdrive_rclone`, and archive materialization paths either pass
  cross-allocation resume or use a tested quiesce/provider-native resume hook;
  none bypasses the common child boundary.
- Adding a future work-item type requires ordinary subprocess-boundary tests and
  checkpoint/resume compatibility evidence.
- The ten-minute and five-minute thresholds are command-line/startup
  configurable, and the resume-attempt limit is enforced without retrying
  causal work failures.
- The administrative drain request reaches the worker through its outbound
  control path and triggers the same behavior as `SIGUSR1`.
- Checkpoint blobs are access-controlled, omitted from logs and public
  artifacts, and removed only when no durable controller state needs them.
- Open-file tests prove safe stdout/stderr offsets, attempt-local file
  reconciliation, atomic result creation, and no generic truncation of promoted
  or external outputs.
- Completion, checkpoint, abandonment, resume, and late-report races have
  focused tests.
- Controller restart reconstructs pending-resume work, attempt lineage, resume
  counts, and accepted checkpoint references from durable state.
- Current worker-session heartbeat and CareTaker tests remain green.
- Runtime documentation names the supported DMTCP, container, architecture,
  filesystem, Slurm, operation, and provider matrix.
- All approved Operational Slices are implemented and accepted.
- The human explicitly approves moving this Strategic Concept from `Proposed`
  to `Ready`, and later from `Ready` to `Implemented`.
