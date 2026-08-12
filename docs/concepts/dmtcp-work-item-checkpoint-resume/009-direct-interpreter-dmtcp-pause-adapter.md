# 009 Direct-Interpreter DMTCP Pause Adapter

Status: implementation in progress

## Objective

Implement the first production pause adapter for the direct-interpreter DMTCP
boundary proven by OS-003 and OS-004. The adapter must launch and supervise a
checkpointable interpreter process tree while the long-lived Go worker remains
outside DMTCP, and must produce the validated OS-008 checkpoint contract for
periodic, quantum, and final capture.

The initial registered work-item type is the existing `python_script` type.
The adapter must keep the interpreter launch contract generic enough for the
proven direct-R shape, but this slice does not invent an R work-item type or
enable R workflow execution before a later workflow-facing decision.

## Current State

OS-008 provides the worker-side `PauseAdapter` and `SupervisedExecution`
interfaces in `cmd/worker/checkpoint_adapter.go`, the serialized lifecycle in
`cmd/worker/execution_supervisor.go`, and controller confirmation through
`WorkerControllerClient`.

`Worker` accepts an explicit adapter registry, but no production adapter is
registered. The current `python_script` path in `cmd/worker/work_python.go`
launches CPython directly with `exec.Command`, waits synchronously, and
converts its output into ordinary `WorkEvidence`. It has no DMTCP coordinator,
checkpoint workspace, restart argument construction, or resume-artifact
generation.

OS-003 proved direct `Rscript` plus the tested R/BRMS process shapes under
normal-user institutional Singularity. OS-004 proved direct CPython 3.11 with
NumPy and one Python descendant under the same DMTCP 4.2.0/Singularity 4.1.2
boundary. Those feasibility slices do not provide a GOET adapter or a
work-item-facing input/output contract.

The shared resume model requires an immutable manifest written last, exact
artifact and manifest identities, a declared DMTCP compatibility record, and
one strategy-specific DMTCP payload. The adapter must use an attempt-scoped
workspace and must never attach to the default DMTCP coordinator or restore
through an unvalidated shell script.

## Target State

Add `cmd/worker/dmtcp_adapter.go` with a concrete adapter implementation that:

- launches one isolated DMTCP coordinator and one direct interpreter payload
  for a registered `python_script` work item;
- constructs argument vectors from validated work-item and resume inputs,
  without evaluating stored shell commands;
- keeps the Go worker, controller client, heartbeat, and supervisor outside the
  DMTCP computation;
- uses an attempt-scoped durable workspace beneath the configured shared
  temporary root with owner-only permissions;
- captures periodic, quantum, and final generations through the OS-008
  `PreparedCheckpoint` contract;
- waits for finalized DMTCP image files and validates that the coordinator and
  all expected clients are accounted for before returning a checkpoint;
- writes the GOET resume manifest last and returns exact manifest JSON plus a
  matching `ResumeArtifactReference`;
- resumes periodic captures before returning from `CapturePeriodic`, while a
  suspending capture uses DMTCP's checkpoint-and-kill command and suppresses
  the expected payload exit until the supervisor completes bounded cleanup;
- constructs an explicit `dmtcp_restart` argument vector from validated
  manifest payload paths for `StartResume`; and
- terminates only the adapter-owned payload and coordinator during bounded
  cleanup.

Register the adapter for `model.WorkItemTypePythonScript` only when the worker
configuration explicitly selects an available DMTCP runtime/profile. The
checked-in default remains unchanged, and no ordinary Python execution may be
silently converted to DMTCP execution.

Add focused adapter tests using injected command, clock, and filesystem
boundaries. They must prove launch isolation, argument validation, capture
quiescence, manifest-last ordering, exact reference identity, restart path
validation, cleanup, and failure behavior. Add one repeatable container-level
smoke that reuses the OS-004 direct-Python feasibility image or its pinned
runtime to prove the adapter can checkpoint, terminate, and restore the
existing `python_script` fixture through the OS-008 supervisor.

The adapter-level launch profile must preserve the direct-interpreter boundary
needed for the OS-003 R shape, but R is not registered as a GOET work-item type
by this slice. The result must explicitly record that distinction.

## Concept Decision

This slice adds a new production concept: a DMTCP-backed implementation of the
OS-008 pause-adapter contract. It belongs in a new focused
`cmd/worker/dmtcp_adapter.go` file because coordinator ownership, direct
interpreter launch, checkpoint workspace handling, and restart validation have
their own lifecycle and test surface.

The adapter should reuse existing model validation, worker staging, protected
value handling, and evidence structures where possible. It must not duplicate
the ordinary `runPythonScript` path or add DMTCP branches to the generic
supervisor.

## Required Context

Read these files first:

- `docs/concepts/dmtcp-work-item-checkpoint-resume/README.md`
- `docs/concepts/dmtcp-work-item-checkpoint-resume/003-r-tidyverse-brms-dmtcp-feasibility.md`
- `docs/concepts/dmtcp-work-item-checkpoint-resume/004-direct-python-dmtcp-feasibility.md`
- `docs/concepts/dmtcp-work-item-checkpoint-resume/005-resume-artifact-contract-model.md`
- `docs/concepts/dmtcp-work-item-checkpoint-resume/007-controller-checkpoint-confirmation-and-resume-assignment-transport.md`
- `docs/concepts/dmtcp-work-item-checkpoint-resume/008-worker-drain-state-and-adapter-neutral-execution-supervisor.md`
- `cmd/worker/checkpoint_adapter.go`
- `cmd/worker/execution_supervisor.go`
- `cmd/worker/worker.go`
- `cmd/worker/work_python.go`
- `cmd/worker/config.go`
- `cmd/worker/testdata/dmtcp-checkpoint/work-item.json`
- `containers/dmtcp-python-feasibility/Dockerfile`
- `containers/dmtcp-python-feasibility/dmtcp-smoke`
- `internal/model/resume_artifact.go`
- `internal/model/checkpoint_transport.go`

Do not read or modify unrelated controller, persistence, Slurm, rclone, or
manual-Go files unless a focused compile or smoke failure exposes a direct
dependency.

## Allowed Production Files

- `cmd/worker/dmtcp_adapter.go` (new)
- `cmd/worker/worker.go` (only for explicit adapter construction/registration)
- `cmd/worker/config.go` (only for DMTCP profile fields required by this slice)

## Allowed Test Files

- `cmd/worker/dmtcp_adapter_test.go` (new)
- `cmd/worker/worker_test.go` (only for registration and ordinary-path guards)
- `cmd/worker/testdata/dmtcp-checkpoint/source/main.py` (only if the fixture
  needs adapter-specific markers)
- `containers/dmtcp-python-feasibility/dmtcp-smoke` (only for the adapter
  integration smoke)
- `containers/dmtcp-python-feasibility/testdata/python-checkpoint/fixture.py`
  (only for adapter-specific input/output markers)

## Allowed Documentation Files

- `PROJECT_STATE.md`
- `docs/TEST_AND_SMOKE_STATUS.md`
- `docs/RUNTIME_RUNBOOK.md`
- `cmd/worker/README.md`
- `docs/concepts/README.md`
- `docs/concepts/dmtcp-work-item-checkpoint-resume/README.md`
- `docs/concepts/dmtcp-work-item-checkpoint-resume/009-direct-interpreter-dmtcp-pause-adapter.md`

## Out Of Scope

- Adding an R work-item type, R workflow schema, or R-specific production
  registration.
- Enabling DMTCP for checked-in production worker configuration by default.
- Putting the Go worker, controller HTTP client, heartbeat, or execution
  supervisor inside DMTCP.
- Reusing the ordinary `runPythonScript` synchronous path for a resume
  assignment or falling back from `dmtcp_restart` to fresh execution.
- Implementing rclone continuation, manual-Go state, CRIU, Slurm warning
  forwarding, or authenticated administrative drain delivery.
- Changing controller checkpoint transactions, persistence schema, queue
  order, assignment lineage, resume-attempt limits, or artifact retention.
- Copying or publishing arbitrary customer files beyond the adapter's declared
  checkpoint workspace and the existing validated result-envelope boundary.
- Claiming support for arbitrary R/Python packages, network resources, GPUs,
  MPI, multiple native threads, unvalidated descendants, or secrets embedded
  in checkpoint memory.
- Proving production-scale checkpoint size, ten-minute/five-minute scheduling
  margins, cross-worker filesystem performance, or cleanup after host failure.

## Acceptance Criteria

- The adapter constructs an isolated coordinator command and never uses the
  default DMTCP coordinator endpoint.
- Fresh launch starts the direct Python interpreter under DMTCP and returns a
  supervised execution without calling ordinary `Worker.Run`.
- Resume launch validates the consumed manifest's DMTCP strategy, adapter
  identity/version, build identity, storage scope, and checkpoint paths before
  starting `dmtcp_restart`; invalid input starts nothing.
- The adapter rejects shell metacharacters, path traversal, missing required
  interpreter/entrypoint values, and unsupported launch profiles.
- Periodic capture does not return until all expected DMTCP images and the
  immutable manifest/reference are complete; the payload resumes before the
  supervisor confirms the generation.
- Quantum and final capture use checkpoint-and-kill, preserve the validated
  generation, and do not expose the expected payload exit as an ordinary
  terminal result before bounded supervisor cleanup.
- A capture directory without finalized images, with temporary images, with an
  incomplete client set, or with a manifest/reference mismatch is rejected.
- Every generated manifest records the exact DMTCP build identity, direct
  interpreter identity, checkpoint paths, storage scope, producing attempt,
  execution lineage, and generation required by the shared model.
- Cleanup is idempotent, bounded, and limited to adapter-owned processes;
  ambiguous cleanup does not report a successful suspension.
- Existing empty-mode `python_script` execution remains unchanged, and no
  production adapter is active unless the explicit DMTCP profile is selected.
- Focused fake-command tests cover fresh launch, resume validation, periodic
  continuation, final suspension, incomplete-image rejection, manifest-last
  ordering, path injection, coordinator isolation, and cleanup races.
- The pinned direct-Python container smoke completes one checkpoint,
  original-process termination, fresh-invocation restore, and semantic output
  comparison through the adapter/supervisor boundary, or records a precise
  environment blocker without claiming adapter support.
- Documentation states that OS-009 proves only the registered direct-Python
  shape and preserves the separate direct-R feasibility evidence from OS-003.
- No file outside the allowed production, test, and documentation lists
  changes.

## Notes

- Use DMTCP `4.2.0`, commit
  `f8009ce7b4ad211311ca2f72a929b975e4aa1155`, and source SHA-256
  `3b240c78804bbf1e9354ee3da5c8760c3c952045f71773e9ed490846b15adce0`.
- Preserve exact manifest JSON bytes when calculating the controller reference
  digest.
- Generate restart arguments from validated artifact paths; never execute a
  generated restart shell script.
- Keep the first implementation prompt narrow: establish the DMTCP launch and
  workspace boundary with fake-command tests before wiring production
  registration or running the institutional smoke.
- This charter is intentionally proposed and requires human agreement before
  implementation or commit.

## Implementation Progress

The first implementation increment completed on 2026-08-12. The new
`cmd/worker/dmtcp_adapter.go` validates and stages a fresh `python_script`,
creates an owner-only attempt-scoped workspace, and constructs an isolated
`dmtcp_launch --new-coordinator` argument vector that directly launches the
configured Python interpreter. An injected command/process boundary lets
`cmd/worker/dmtcp_adapter_test.go` prove coordinator isolation, path and shell
metacharacter rejection before launch, workspace creation, and idempotent
termination without requiring DMTCP on the test host.

At the end of that first increment, resume argument construction, checkpoint
capture and image-set validation, manifest-last publication, normal result
finalization, explicit configuration/registration, and the container-level
adapter/supervisor smoke remained unimplemented. The adapter was not
registered, so checked-in worker behavior remained unchanged.

The second implementation increment completed on 2026-08-12. Periodic capture
now lists the isolated coordinator's running clients, requires the exact
profile client count, issues `dmtcp_command --bcheckpoint`, rejects temporary,
empty, or incomplete image sets, copies finalized images into a new immutable
generation directory, and writes/syncs the validated manifest last. Exact
manifest bytes produce the returned reference digest. Quantum/final capture
uses DMTCP's `--kcheckpoint` operation and suppresses the expected killed
payload result until bounded termination, avoiding a race with controller
confirmation.

The checkpoint-and-kill choice corrects the earlier proposed quiescence
wording. In pinned DMTCP 4.2.0, `--bcheckpoint` blocks the command caller only
until checkpoint completion; it resumes the clients. `--kcheckpoint` is the
available coordinator operation that checkpoints and then kills every enrolled
client. The current OS-008 supervisor does not invoke adapter continuation
after a suspending capture; it always performs bounded termination after
confirmation or fallback handling.

Resume launch, normal Python result finalization, explicit
configuration/registration, and the container-level adapter/supervisor smoke
remain to be implemented.

The third implementation increment completed on 2026-08-12. `StartResume`
now validates the controller assignment before creating a replacement-attempt
workspace. Adapter-specific validation requires the DMTCP strategy and build,
every adapter/worker/image/OS/architecture/runtime compatibility field, and the
assigned input/source/code identities to match the selected launch profile and
work item. It reads the canonical stored `manifest.json` and requires its exact
bytes to match the controller assignment. Each declared checkpoint image must
be a canonical `dmtcp/ckpt_*.dmtcp` path beneath the immutable artifact,
contain no symlink component, be a regular file, and match its declared size
and SHA-256.

Only after all checks pass does the adapter create the new attempt workspace
and invoke the configured `dmtcp_restart` executable with
`--new-coordinator`, new port/checkpoint/temporary paths, and the validated
absolute checkpoint-image arguments. It never executes the generated DMTCP
restart shell script. Focused tests prove the successful argument vector and
pre-launch rejection for runtime identity changes, stored manifest/image
tampering, noncanonical manifest references, and symlinked images.

Normal Python result finalization, explicit configuration/registration, and
the container-level adapter/supervisor smoke remain to be implemented.

The fourth implementation increment completed on 2026-08-12. Fresh supervised
launch now uses the existing Python environment-path, data-asset
materialization, argument-binding, protected-reference materialization, and
redaction boundaries. Both fresh and resumed execution retain the exact
staging, entrypoint, environment, argument, output, and log identities needed
to finalize a successful process. Completion closes and scrubs logs first,
requires `GOET_OUTPUT_JSON`, rejects materialized sensitive values, reuses
canonical output decoding and artifact promotion, atomically publishes the
logical output, calculates pre/post/log/input/output evidence, and returns the
same `WorkEvidence` shape as ordinary Python execution.

Process failure and successful exit without an output remain failures. Bounded
termination now waits for the adapter-owned launcher `Wait` boundary or context
cancellation, so log closure, redaction, and protected-value cleanup finish
before local termination is reported. Focused tests prove fresh and resumed
completion evidence, deterministic output publication, missing-output failure,
and protected-value log redaction.

Explicit configuration/registration and the container-level
adapter/supervisor smoke remain to be implemented.
