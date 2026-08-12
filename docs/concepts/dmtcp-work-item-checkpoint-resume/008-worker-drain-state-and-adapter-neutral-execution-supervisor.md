# 008 Worker Drain State and Adapter-Neutral Execution Supervisor

Status: implemented - accepted with recorded unrelated full-suite exception

## Objective

Add the worker-side lifecycle that stops new claims after `SIGUSR1`, supervises
one active work-item execution, serializes completion against periodic,
quantum-yield, and shutdown checkpoint transitions, and reports validated
checkpoint generations through the OS-007 worker client.

Define the adapter boundary used by later DMTCP, native-tool, and manual-Go
slices, but do not implement or enable any production pause adapter in this
slice. A resume assignment without a matching registered adapter must never run
through the existing fresh-execution path.

## Current State

`cmd/worker/main.go` owns a synchronous pull-execute-report loop:

```text
register -> start heartbeat -> fetch -> Worker.Run -> report -> fetch
```

The loop has no signal subscription or drain state. It continues claiming work
until the controller returns no work, a configured idle timeout expires, a
heartbeat self-fences the worker, or execution/reporting fails. While
`Worker.Run` is blocked, `runWorkerLoop` cannot arbitrate a lifecycle event
against work completion.

`cmd/worker/worker.go` validates and synchronously dispatches each work item.
`python_script` launches CPython directly, while the other current operations
execute through Go handlers and may launch their own subprocesses. There is no
active-execution handle, pause-adapter registry, checkpoint capability
declaration, or separate fresh-versus-resume launch path.

OS-007 added `model.WorkItem.Resume` and validates its exact manifest/reference,
predecessor attempt, execution lineage, and resume-attempt number. The current
worker dispatch ignores that field after validation. If a resume assignment
were produced today, `Worker.Run` would incorrectly start the operation through
its fresh path.

OS-007 also added callable
`WorkerControllerClient.ConfirmCheckpoint` and
`WorkerControllerClient.SuspendLatestCheckpoint` methods in
`cmd/worker/checkpoint_client.go`. Nothing in the worker calls them.

`cmd/worker/config.go` has idle polling and timeout settings, but no checkpoint
mode, interval, execution quantum, drain-escalation delay, capture/report
deadline, or termination grace. `WorkerLifecycleClock` supplies only `Now` and
`NewTicker`, which is sufficient for heartbeats but not a resettable,
deterministically tested supervisor timer.

The generated worker configuration in `cmd/controller/runtime.go` and
`cmd/controller/execution_environment.go` does not carry pause/resume policy.
The worker process does not subscribe to `SIGUSR1`. There is also no
authenticated administrative-drain transport; OS-007 exposes checkpoint
reporting only.

## Target State

### Worker checkpoint policy

Extend `Config` with JSON fields equivalent to:

```text
checkpoint_mode                         "" | shutdown | periodic | yield
checkpoint_interval_seconds             required only for periodic
work_item_execution_quantum_seconds     required only for yield
drain_pause_delay_seconds               delay from SIGUSR1 to final pause
checkpoint_capture_timeout_seconds
checkpoint_report_timeout_seconds
execution_termination_grace_seconds
```

An empty mode preserves the existing non-checkpointing execution path.
`shutdown`, `periodic`, and `yield` are the only enabled values. Durations use
positive whole seconds when required. Validation rejects irrelevant or
contradictory values and requires:

```text
checkpoint_capture_timeout
    + checkpoint_report_timeout
    + execution_termination_grace
    < drain_pause_delay
```

`periodic` requires a positive interval and no execution quantum. `yield`
requires a positive quantum and no periodic interval in this slice.
`shutdown` has neither an interval nor a quantum. The initial rendered values
are 300 seconds for the checkpoint interval and drain-to-pause delay when the
corresponding mode is selected; empty mode remains the checked-in default
until a production adapter slice deliberately enables it.

Controller worker-configuration rendering carries the fields unchanged. This
slice does not add Slurm command-line flags or calculate the original Slurm
warning lead time; it consumes the worker-local delay after a delivered drain
event.

### Adapter-neutral execution contract

Add a focused `cmd/worker/checkpoint_adapter.go` concept with interfaces and
records equivalent to:

```text
PauseAdapterRegistry
PauseAdapter
SupervisedExecution
ExecutionResult
CheckpointCaptureRequest
PreparedCheckpoint
PauseAdapterCapabilities
```

The registry is an explicit dependency of `Worker`, not package-global state.
It selects at most one adapter by `model.WorkItemType`. Each registration
declares its exact `model.PauseStrategy`, adapter identity/version, and whether
it permits repeatable periodic and quantum captures.

`PauseAdapter` has distinct fresh-start and resume-start operations. Resume
start receives the already validated `model.WorkItemResumeAssignment`; the
adapter must validate its own compatibility and live artifact bytes before it
starts restored work. There is no fallback from resume start to fresh start.

`SupervisedExecution` exposes one terminal-result channel plus separate
operations equivalent to:

- periodic capture, which returns only after a complete validated immutable
  generation is durable and the payload has resumed;
- suspending capture, which returns a complete validated generation while the
  payload remains quiesced;
- continuation after a definitively rejected suspending capture; and
- bounded termination after suspension or unresolved ownership.

Each successful capture returns exact `manifest_json` and a matching
`ResumeArtifactReference`. The supervisor revalidates the shared model,
assignment identity, strategy, lineage, and expected generation before calling
the controller. The adapter contract owns the strategy-specific quiesce and
complete state/output bundle; OS-009 through OS-011 implement those mechanics.

No production work-item type is registered in OS-008. Tests use deterministic
fake adapters and executions. Enabled checkpoint policy with no registered
adapters fails worker validation before the first claim. A claimed resume
assignment without a matching adapter produces a non-causal resume-launch
rejection: the worker does not call `Worker.Run`, does not report
`WorkFailure`, and stops its owned session so existing abandonment/requeue and
the OS-007 per-artifact attempt limit remain authoritative.

### Serialized execution supervisor

Add `cmd/worker/execution_supervisor.go`. One supervisor owns one claimed
attempt and is the only component allowed to choose its local terminal result.
Its observable outcomes distinguish:

```text
completed
failed
suspended
abandon_without_terminal_report
```

The supervisor starts a fresh execution or resumes the assignment through the
selected adapter, then serializes these events:

- execution completion or causal failure;
- periodic active-execution interval expiry;
- execution-quantum expiry;
- first drain request and its final-pause timer;
- checkpoint capture completion or failure;
- controller acknowledgement or report timeout; and
- heartbeat/session cancellation.

At most one capture or confirmation is active. Timers measure active payload
time: time spent capturing, quiesced, confirming, or restoring does not consume
the next periodic interval or yield quantum. A periodic tick arriving while a
capture/report is unresolved coalesces into at most one overdue request.

The next generation is `1` for a fresh lineage or one greater than the resume
assignment's generation. It advances locally only after a matching controller
acknowledgement. The exact confirmation request is retained and replayed while
its outcome is ambiguous; a different generation is not allocated during that
period.

Periodic capture uses `periodic/continue`. Because its adapter operation has
already resumed the payload, a failed or timed-out confirmation leaves the
attempt running and does not replace the latest accepted acknowledgement.

Quantum expiry uses `quantum/suspend`. First drain is monotonic and arms one
`drain_pause_delay_seconds` timer; its expiry uses `final/suspend`. A successful
suspend acknowledgement is followed by bounded execution termination and a
`suspended` outcome. The supervisor never also returns ordinary completion or
failure for that attempt.

If quantum/final capture fails and this attempt has a controller-accepted
checkpoint, the supervisor calls `SuspendLatestCheckpoint` with a stable
request. A resumed attempt begins with its consumed artifact as its latest
accepted recovery point. If no accepted checkpoint exists, the outcome is
`abandon_without_terminal_report` so worker-session recovery uses the existing
fresh path.

If suspend confirmation remains ambiguous through its report deadline, the
supervisor terminates the quiesced execution and returns
`abandon_without_terminal_report`; it never resumes work that the controller
may already have released. A definitively rejected suspend may continue only
when ownership is still known to be held and the active policy permits it.

### Drain source and worker-loop behavior

Add a small drain-request abstraction and platform-specific signal source.
Linux builds subscribe to `SIGUSR1` through `os/signal`; non-Linux builds retain
the same injectable channel contract without referring to an unavailable
signal constant. Signal callbacks only enqueue a drain request. They never
perform checkpoint or controller I/O.

`runWorkerLoop` consumes the drain source:

- drain while idle stops heartbeats, reports worker stop reason
  `slurm_drain_idle`, and makes no later claim;
- drain while an item is active stops later claims but leaves the supervisor
  to arbitrate completion versus final pause;
- ordinary completion or causal failure that wins after drain is reported
  through the existing endpoint, followed by worker stop reason
  `slurm_drain_finished`;
- accepted suspension stops the worker session with reason
  `checkpoint_suspended` after the controller has already released assignment
  ownership; and
- abandon/resume-launch-rejection stops the session without reporting a causal
  work failure, allowing existing controller recovery to select the accepted
  artifact or fresh path.

Heartbeat self-fencing cancels the supervisor. No completion, failure, or
checkpoint report is sent after the worker knows its session is no longer
active.

When checkpoint mode is empty and no resume assignment is present, existing
work items continue through `Worker.Run`. The drain source still prevents a
new claim and permits an active ordinary execution to finish, but there is no
checkpoint escalation for that unconfigured execution.

### Administrative drain boundary

The supervisor accepts injected drain requests without depending on their
source, so a later authenticated administrative-control transport can feed the
same state machine. This slice wires only the local `SIGUSR1` source. Adding a
controller administrative endpoint, durable command delivery, heartbeat
response command, or polling protocol is deferred because none exists in the
current controller/worker lifecycle contract.

## Concept Decision

This slice updates the existing worker configuration, main-loop, lifecycle,
and controller worker-config rendering concepts.

The adapter contract is a new concept with its own interfaces, capability
rules, and independent fake-based tests, so it belongs in new
`cmd/worker/checkpoint_adapter.go`.

The execution supervisor is a separate state machine with independent timing,
race, and retry tests, so it belongs in new
`cmd/worker/execution_supervisor.go`. Do not place it in `worker.go` or
`main.go` merely to reduce file count.

Drain request representation and platform signal subscription have separate
responsibilities. Keep the common drain contract in a focused new file and use
small build-tagged signal files so the worker continues to compile on Windows
while the production Linux build subscribes to `SIGUSR1`.

The selected `file(1)+test+doc+newfile` budget applies per implementation
prompt, not to the whole Operational Slice. OS-008 is intentionally a
multi-prompt implementation. Each prompt changes at most one production file,
may change its focused test file and allowed documentation, reports the
remaining target, and stops for human review before the next pass.

## Required Context

Read these files first:

- `docs/concepts/dmtcp-work-item-checkpoint-resume/README.md`
- `docs/concepts/dmtcp-work-item-checkpoint-resume/007-controller-checkpoint-confirmation-and-resume-assignment-transport.md`
- `internal/model/checkpoint_transport.go`
- `internal/model/resume_artifact.go`
- `cmd/worker/config.go`
- `cmd/worker/lifecycle.go`
- `cmd/worker/checkpoint_client.go`
- `cmd/worker/main.go`
- `cmd/worker/worker.go`
- `cmd/controller/runtime.go`
- `cmd/controller/execution_environment.go`

Do not read adapter implementation candidates, Slurm generation, container
builds, operation handlers, or unrelated controller/persistence code unless a
focused compile or test failure exposes a direct dependency.

## Allowed Production Files

- `cmd/worker/config.go`
- `cmd/worker/checkpoint_adapter.go` (new)
- `cmd/worker/execution_supervisor.go` (new)
- `cmd/worker/drain.go` (new)
- `cmd/worker/drain_signal_linux.go` (new)
- `cmd/worker/drain_signal_other.go` (new)
- `cmd/worker/lifecycle.go`
- `cmd/worker/worker.go`
- `cmd/worker/main.go`
- `cmd/controller/runtime.go`
- `cmd/controller/execution_environment.go`

## Allowed Test Files

- `cmd/worker/config_test.go`
- `cmd/worker/checkpoint_adapter_test.go` (new)
- `cmd/worker/execution_supervisor_test.go` (new)
- `cmd/worker/drain_test.go` (new)
- `cmd/worker/lifecycle_test.go`
- `cmd/worker/worker_test.go`
- `cmd/worker/main_test.go`
- `cmd/controller/runtime_test.go`
- `cmd/controller/execution_environment_test.go`

## Allowed Documentation Files

- `PROJECT_STATE.md`
- `TARGET_STATE.md`
- `docs/TEST_AND_SMOKE_STATUS.md`
- `docs/RUNTIME_RUNBOOK.md`
- `docs/concepts/README.md`
- `docs/concepts/dmtcp-work-item-checkpoint-resume/README.md`
- `docs/concepts/dmtcp-work-item-checkpoint-resume/008-worker-drain-state-and-adapter-neutral-execution-supervisor.md`
- `cmd/worker/README.md`

## Out Of Scope

- Implementing or registering the R/Python DMTCP adapter, invoking
  `dmtcp_launch`, `dmtcp_command`, or `dmtcp_restart`, or managing a DMTCP
  coordinator.
- Implementing rclone continuation or changing any rclone command/backend
  behavior.
- Adding manual state or cooperative pause points to an in-process Go handler.
- Reading, hashing, copying, reconciling, or deleting real checkpoint/state or
  registered-output files; fake adapter results exercise the supervisor
  contract only.
- Changing `goet/resume-artifact/v1`, checkpoint HTTP transport, SQLite schema,
  queue order, attempt lineage, resume-attempt counting, or controller
  checkpoint transactions.
- Reporting resume incompatibility as a new durable controller status or
  adding an operator reset/retry decision.
- Adding authenticated administrative-drain endpoints or command delivery.
- Generating or forwarding the Slurm warning in batch scripts, changing
  Singularity invocation, invoking `srun`/`sbatch`/`scontrol`, or requeueing a
  Slurm allocation.
- Proving process-group termination, DMTCP quiescence, output-copy consistency,
  filesystem permissions, retention cleanup, or cross-allocation restore.
- Enabling checkpoint mode in checked-in production configuration before a
  production adapter is implemented and accepted.
- Changing existing work-operation implementations or direct development mode.

## Acceptance Criteria

- Every implementation prompt changes at most one production file, plus its
  focused tests and allowed documentation, and stops for review with OS-008
  still marked incomplete until every pass is accepted.
- Worker policy validation accepts empty, `shutdown`, `periodic`, and `yield`
  modes with their exact required fields and rejects unsupported modes,
  nonpositive required durations, irrelevant interval/quantum values, and an
  unsafe capture/report/termination budget.
- Controller-generated worker JSON carries the validated policy without
  enabling it by default.
- The adapter registry has no global mutable state, rejects duplicate work-item
  registrations and invalid capability/strategy declarations, and performs
  distinct fresh and resume selection.
- A resume assignment never reaches `Worker.Run` or a fresh adapter launch.
  Missing, mismatched, or incompatible resume adapters cause session
  stop/recovery without `WorkFailure`.
- Checkpoint-enabled startup fails before registration/claim when no production
  adapter can satisfy the configured mode.
- The supervisor accepts at most one local terminal outcome and never reports
  both completion/failure and suspension for one attempt.
- Periodic capture confirms `periodic/continue`, resumes locally before
  controller confirmation, keeps the prior accepted generation on failure,
  and does not overlap generations.
- Generation numbering starts at one for fresh execution, continues from a
  consumed resume artifact, and advances only after a matching
  acknowledgement.
- Quantum expiry confirms `quantum/suspend`; drain escalation confirms
  `final/suspend`; both terminate only the supervised execution after accepted
  suspension.
- Final/quantum capture or report failure uses the latest acknowledged
  generation when present. With none, it produces
  `abandon_without_terminal_report` and does not manufacture a resume
  reference.
- Ambiguous suspend reporting replays the identical request until its deadline,
  never starts a new generation, and never resumes the quiesced execution when
  ownership may have been released.
- Periodic and quantum timers exclude capture, confirmation, and restore time;
  overlapping ticks coalesce without concurrent capture.
- First drain is monotonic and idempotent. Idle drain makes no later claim;
  active drain allows ordinary completion before escalation; completion and
  escalation races yield exactly one controller transition attempt.
- The Linux worker subscribes to `SIGUSR1`; non-Linux builds compile with
  the same injectable drain contract.
- Heartbeats continue during capture and controller reporting, and session
  self-fencing suppresses later terminal/checkpoint reports.
- Existing behavior and JSON shape remain unchanged when checkpoint mode is
  empty and the assignment has no `resume` field.
- Focused fake-clock/fake-adapter tests cover periodic success/failure,
  coalescing, yield, drain-before-completion, completion-before-escalation,
  final fallback, no-checkpoint abandonment, ambiguous confirmation,
  termination timeout, resume selection, and stale-session cancellation.
- The package verification passes:

  ```text
  go test ./cmd/worker ./cmd/controller -count=1
  ```

- Project, worker, runtime, and test-status documentation states that the
  adapter-neutral lifecycle is implemented but no production pause adapter is
  enabled.
- No production or test file outside the allowed lists changes.

## Implementation Sequence

Implement OS-008 across separately reviewed prompts. Each numbered pass changes
only the named production file; its focused test and allowed documentation may
change in the same prompt.

1. `cmd/worker/config.go`: add and validate the worker checkpoint policy.
2. `cmd/controller/runtime.go`: add the rendered worker-policy JSON fields.
3. `cmd/controller/execution_environment.go`: resolve and populate those
   fields without enabling checkpoint mode by default.
4. `cmd/worker/lifecycle.go`: extend the lifecycle clock/timer boundary needed
   by the supervisor without changing heartbeat semantics.
5. `cmd/worker/checkpoint_adapter.go` (new): define the registry, adapter,
   execution, capture, result, and capability contracts.
6. `cmd/worker/drain.go` (new): define the monotonic injectable drain source
   and common drain reasons.
7. `cmd/worker/execution_supervisor.go` (new): implement the serialized
   fake-clock-tested checkpoint/completion state machine.
8. `cmd/worker/drain_signal_linux.go` (new): subscribe Linux workers to
   `SIGUSR1`.
9. `cmd/worker/drain_signal_other.go` (new): preserve non-Linux compilation and
   dependency injection.
10. `cmd/worker/worker.go`: own the explicit adapter registry, validate policy
    capabilities, and expose distinct supervised fresh/resume launch.
11. `cmd/worker/main.go`: wire drain, supervision, checkpoint reporting,
    outcome-specific terminal reporting, and no-later-claim behavior into the
    controller loop.
12. Documentation-only closeout: record final focused/full test evidence and
    the exact remaining adapter, Slurm-forwarding, administrative-control, and
    operations gaps.

If an earlier pass proves that a later production-file change is unnecessary,
remove that pass during review rather than touching the file for symmetry. If
a pass cannot satisfy its acceptance criteria within one production file,
stop and revise this charter before broadening the implementation boundary.

## Implementation Progress

2026-08-04 pass 1 updated `cmd/worker/config.go`. `Config` now carries typed
checkpoint modes `shutdown`, `periodic`, and `yield` plus interval, quantum,
drain-to-pause, capture, report, and termination settings. Empty mode requires
all checkpoint-policy fields to remain zero. Enabled modes require positive
common deadlines, enforce their mode-specific interval/quantum shape, and
reject a capture/report/termination budget that is not strictly smaller than
the drain-to-pause delay.

Focused and complete worker-package verification passes:

```text
go test ./cmd/worker -run 'TestLoadConfig|TestConfigValidate' -count=1
go test ./cmd/worker -count=1
```

2026-08-04 pass 2 updated `cmd/controller/runtime.go`. `WorkerRuntime` and its
serialized `WorkerConfig` now carry the same seven checkpoint-policy fields,
and `writeWorkerConfig` preserves them exactly. The zero-value runtime omits
all checkpoint keys, so this pass does not enable checkpointing in generated
worker configuration.

Focused verification passes:

```text
go test ./cmd/controller -run 'TestWorkerRuntimePrepareWritesWorkerConfig' -count=1
```

The complete `go test ./cmd/controller -count=1` run remains red. It reaches
the previously recorded `TestNewStartupRuntimeScope` timestamp-precision
mismatch and a reproducible Windows-only fake-`sbatch` exit-126 failure in
`TestGeneratedSlurmWorkerScriptRunsThroughFakeSbatch`. Neither test exercises
the worker-config fields changed by this pass.

2026-08-04 pass 3 updated `cmd/controller/execution_environment.go`.
`workerRuntimeFromSettings` now resolves all checkpoint-policy settings,
validates the same disabled/shutdown/periodic/yield shapes before runtime
preparation, and populates `WorkerRuntime`. Omitted enabled-mode
`drain_pause_delay_seconds` defaults to `300`; omitted periodic
`checkpoint_interval_seconds` defaults to `300`. Explicit zero for either
required value is rejected. Capture, report, termination, and yield-quantum
values remain explicit because no defaults are approved for them.

Focused and broader environment/runtime verification passes:

```text
go test ./cmd/controller -run 'TestNewExecutionEnvironmentSupportsLocalDirectProcess|TestWorkerRuntimeFromSettingsCheckpointPolicy|TestWorkerRuntimePrepareWritesWorkerConfig' -count=1
go test ./cmd/controller -run 'TestNewExecutionEnvironment|TestWorkerRuntime' -count=1
```

The known unrelated full-controller failures recorded in pass 2 were not
rerun. OS-008 remains incomplete. Pass 4 must extend the worker lifecycle
clock with the resettable timer boundary required by the future supervisor,
without changing heartbeat behavior.

2026-08-04 pass 4 updated `cmd/worker/lifecycle.go`.
`WorkerLifecycleClock` now creates resettable one-shot
`WorkerLifecycleTimer` values in addition to heartbeat tickers. The timer
exposes the same channel, stop result, and reset result needed to model active
execution deadlines without sleeping in tests. `realWorkerLifecycleClock`
delegates that contract to `time.Timer`; heartbeat continues to use the
unchanged ticker path.

Focused and complete worker verification passes:

```text
go test ./cmd/worker -run 'TestRunHeartbeat|TestRealWorkerLifecycleTimer|TestFakeWorkerLifecycleTimer' -count=1
go test ./cmd/worker -count=1
```

OS-008 remains incomplete. Pass 5 must add the new adapter registry,
capability, execution, capture, and result contracts in
`cmd/worker/checkpoint_adapter.go`; no production adapter will be registered.

2026-08-04 pass 5 added `cmd/worker/checkpoint_adapter.go` as the independent
adapter-contract concept approved by this charter. It defines the explicit
work-item-keyed registry, shutdown/periodic/yield capability declarations,
distinct fresh and resume starts, the supervised execution lifecycle,
checkpoint capture requests, terminal execution results, and prepared
checkpoint results. No package-global registry or production registration was
added.

`PreparedCheckpoint.Validate` decodes and validates the exact common manifest,
recomputes its reference digest, checks manifest-path containment, and matches
work-item, attempt, lineage, generation, artifact, storage, strategy, adapter
ID, and adapter version against the capture contract. Tests use only fake
adapters and fake executions.

Focused and complete worker verification passes:

```text
go test ./cmd/worker -run 'TestPauseAdapter|TestCheckpointCaptureRequest|TestPreparedCheckpoint|TestFakeSupervisedExecution' -count=1
go test ./cmd/worker -count=1
```

OS-008 remains incomplete. Pass 6 must add the monotonic injectable drain
request/source contract and common drain reasons in `cmd/worker/drain.go`.

2026-08-04 pass 6 added `cmd/worker/drain.go`. `WorkerDrainRequests` validates
the shared `slurm_sigusr1` and future `administrative` reasons, normalizes the
accepted timestamp to UTC, and latches the first valid request under concurrent
callers. It exposes that request through a single buffered event and through a
read-only current-state query. Invalid requests do not change state, later
valid requests are ignored, and the zero value is usable. This pass adds no
goroutine, signal subscription, controller I/O, or process-wide state.

Focused and complete worker verification passes:

```text
go test ./cmd/worker -run 'TestWorkerDrain' -count=1
go test ./cmd/worker -count=1
```

OS-008 remains incomplete. Pass 7 must implement the serialized,
fake-clock-tested checkpoint/completion state machine in
`cmd/worker/execution_supervisor.go`.

2026-08-11 pass 7 added `cmd/worker/execution_supervisor.go`. The supervisor
starts the selected adapter through its distinct fresh or resume path, owns one
attempt's local terminal decision, and serializes execution completion against
periodic, quantum, and final checkpoint events. It validates prepared
checkpoints before reporting, retries the exact request after ambiguous
transport failures, advances lineage generation only after a matching
acknowledgement, retains unresolved periodic confirmations for exact later
replay, uses the latest accepted generation when suspending capture fails, and
requires bounded execution termination before returning `suspended`.

Focused fake-clock, fake-adapter, and fake-controller-client tests cover
completion/failure, periodic confirmation and retry, generation behavior,
resume lineage continuation, yield, drain escalation and fallback, ambiguous
suspend reporting, periodic ownership-conflict abandonment, no-fallback
abandonment, termination timeout, and cancellation. Focused and complete
worker verification passes:

```text
go test ./cmd/worker -run '^TestExecutionSupervisor' -count=1
go test ./cmd/worker -count=1
```

OS-008 remains incomplete. Pass 8 must add the Linux `SIGUSR1` subscription in
`cmd/worker/drain_signal_linux.go`. It must enqueue only the shared drain
request and must not perform checkpoint or controller work in the signal
callback.

2026-08-11 pass 8 added `cmd/worker/drain_signal_linux.go` behind the `linux`
build constraint. `newPlatformWorkerDrainSource` registers a buffered
`os/signal` channel for `syscall.SIGUSR1`, translates delivery into the common
UTC-normalized `WorkerDrainRequest`, and performs no checkpoint or controller
I/O. Its idempotent stop function unregisters the signal and waits for the
translation goroutine to exit.

Linux cross-compilation passes:

```text
GOOS=linux GOARCH=amd64 go test -c ./cmd/worker
```

The temporary cross-compiled test binary was removed. This is compile-level
evidence; live Linux signal delivery remains untested, and shared worker code
does not call the platform source yet.

OS-008 remains incomplete. Pass 9 must add the matching non-Linux injectable
source in `cmd/worker/drain_signal_other.go` so later shared-code wiring can
compile on Windows without referring to `SIGUSR1`.

2026-08-11 pass 9 added `cmd/worker/drain_signal_other.go` behind the `!linux`
build constraint. It returns the same `WorkerDrainSource` plus stop-function
contract without importing `os/signal`, starting a goroutine, or naming an
unavailable signal constant. A shared platform-source test proves callers can
inject a drain request through the returned `WorkerDrainRequests` and can call
the stop function repeatedly. Focused and complete Windows worker tests pass,
and the complete worker test binary cross-compiles for Linux:

```text
go test ./cmd/worker -run 'TestWorkerDrain|TestPlatformWorkerDrainSource' -count=1
go test ./cmd/worker -count=1
GOOS=linux GOARCH=amd64 go test -c ./cmd/worker
wsl.exe -d Ubuntu-24.04 --cd <repository> -- go test ./cmd/worker -run TestPlatformWorkerDrainSource -count=1 -v
```

The temporary cross-compiled binary was removed. WSL Ubuntu 24.04 already has
Go 1.26.2, and both focused Linux platform tests pass, including delivery of
real `SIGUSR1` to the Go test process and observation of the expected drain
request. The complete WSL worker-package run reaches four unrelated failures
in heartbeat timing, one expected error string, and two Python-subprocess log
tests; the complete Windows worker package remains green.

OS-008 remains incomplete. Pass 10 must update `cmd/worker/worker.go` to own
the explicit adapter registry, reject unsupported enabled policies before a
claim, and expose distinct supervised fresh/resume launch without enabling a
production adapter.

2026-08-11 pass 10 updated `cmd/worker/worker.go`. `Worker` now owns a
`PauseAdapterRegistry`. Startup validation keeps disabled mode compatible with
an empty registry, but enabled modes require at least one registration and
require every registered adapter to support the configured mode. Ordinary
`Worker.Run` rejects every resume assignment before logging or dispatch.

`Worker.RunSupervised` selects the work-item registration and delegates to the
single-attempt supervisor, whose `StartFresh` versus `StartResume` boundary is
therefore the only adapter-backed launch path. Before resume launch, the worker
matches the validated manifest's pause strategy, adapter ID, and adapter
version to the selected registration. Missing or mismatched registrations
return a non-causal setup error without starting either adapter path; pass 11
must map that error to session stop/recovery rather than `WorkFailure`.

Focused tests pass on Windows and WSL/Linux, and the complete Windows worker
package remains green:

```text
go test ./cmd/worker -run 'TestWorkerValidateRequiresAdapters|TestWorkerRunRejectsResume|TestWorkerRunSupervised' -count=1
go test ./cmd/worker -count=1
```

OS-008 remains incomplete. Pass 11 must update `cmd/worker/main.go` to create
and stop the platform drain source, select ordinary versus supervised
execution, preserve heartbeat self-fencing, map supervisor outcomes to the
correct controller/worker-stop transitions, and prevent claims after drain.

2026-08-11 pass 11 updated `cmd/worker/main.go`. The controller-mode loop now
validates checkpoint capability before registration, creates and stops the
platform drain source, and refuses to claim after observing a drain request.
Disabled fresh assignments retain ordinary `Worker.Run`; enabled checkpoint
modes and resume assignments use `Worker.RunSupervised`. During supervised
execution, the supervisor is the sole drain-channel consumer, while the loop
observes the monotonic latch after termination.

The loop preserves heartbeat self-fencing by cancelling supervised execution
and waiting for its termination before returning a rejected heartbeat. A final
synchronous ownership heartbeat fences completion or failure reporting against
a background-heartbeat race. The loop maps ordinary or supervised completion
and causal failure to the existing terminal reports, maps suspension and
abandonment to worker-stop transitions without a duplicate terminal report,
and treats adapter setup or resume rejection as a session stop/recovery
condition rather than a causal `WorkFailure`. Idle drain stops without a claim;
a successful claim already in flight when drain arrives is finished; and active
drain permits ordinary completion or supervisor checkpoint policy to settle
before the worker stops.

Focused and complete Windows worker verification, focused race verification,
and focused WSL/Linux worker-loop verification pass:

```text
go test ./cmd/worker -run '^TestRunWorkerLoop' -count=1
go test ./cmd/worker -count=1
go test -race ./cmd/worker -run '^TestRunWorkerLoop' -count=1
go vet ./cmd/worker
wsl.exe -d Ubuntu-24.04 --cd <repository> -- go test ./cmd/worker -run TestRunWorkerLoop -count=1 -v
wsl.exe -d Ubuntu-24.04 --cd <repository> -- go vet ./cmd/worker
```

The complete WSL worker-package run now passes every worker-loop test and
retains three unrelated failures in one data-binding assertion and two Python
subprocess log tests. The complete Windows worker package is green.

2026-08-11 pass 12 completed the documentation-only closure and final evidence
review. The focused OS-008 worker and controller suites pass. The exact charter
package command does not pass because the complete controller package retains
the previously recorded `TestNewStartupRuntimeScope` timestamp-precision
failure. One concurrent combined run also encountered a transient Windows file
lock in `TestPromoteArtifactsPromotesDirectoryArtifact`; that test passes when
rerun in isolation. Neither failure exercises checkpoint policy, drain,
adapter, supervisor, resume selection, or worker-loop behavior, and this
closure does not claim that the combined command is green.

Final focused evidence:

```text
go test ./cmd/worker -run 'TestConfigValidate|TestPauseAdapter|TestCheckpointCaptureRequest|TestPreparedCheckpoint|TestWorkerDrain|TestPlatformWorkerDrainSource|TestExecutionSupervisor|TestWorkerValidateRequiresAdapters|TestWorkerRunRejectsResume|TestWorkerRunSupervised|TestRunWorkerLoop' -count=1
go test ./cmd/controller -run 'TestWorkerRuntimePrepareWritesWorkerConfig|TestWorkerRuntimeFromSettingsCheckpointPolicy|TestNewExecutionEnvironmentSupportsLocalDirectProcess' -count=1
go test ./cmd/worker -run '^TestPromoteArtifactsPromotesDirectoryArtifact$' -count=1
```

The exact remaining product gaps are deliberately outside OS-008:

- OS-009 must implement and prove the scoped R/Python DMTCP adapter before any
  production work-item registration or checkpoint mode is enabled.
- OS-010 and OS-011 must add their native rclone and manual-Go continuation
  contracts and per-work-item adoption evidence.
- A later scheduler/control slice must arrange Slurm warning-signal forwarding
  and authenticated administrative drain delivery into the existing injected
  drain boundary.
- OS-012 must provide cross-worker restore, security, retention, cleanup,
  timeout/fallback status, and operator evidence.

OS-008 is implemented. It supplies the adapter-neutral worker lifecycle and
keeps every production pause adapter disabled.

## Notes

- The Go concept introduced by the supervisor is ownership through an
  interface: the supervisor owns policy and event ordering, while an adapter
  owns one execution and its strategy-specific capture mechanics.
- Keep timestamps and timers injectable. Tests must not sleep for real
  five-minute intervals or depend on process-wide signal delivery.
- Preserve exact manifest JSON bytes from adapter output through the OS-007
  client so idempotent confirmation hashes remain stable.
- Do not log a manifest, file inventory, environment, command line, or
  protected value while reporting checkpoint errors.
- Do not use `context.CancelFunc` alone as proof that a process or operation is
  stopped. The adapter's bounded termination result is the evidence required
  for a suspended outcome.
- The consumed resume artifact is an accepted recovery point even before the
  resumed attempt creates a newer generation.
- The controller remains the durable ownership authority. Local serialization
  prevents duplicate worker actions; controller session/attempt fencing
  remains the final race defense.
- Administrative drain should reuse the injected drain-request boundary in a
  later slice rather than introduce a second supervisor path.
