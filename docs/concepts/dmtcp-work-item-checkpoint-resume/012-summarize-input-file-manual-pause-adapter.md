# 012 Summarize Input File Manual Pause Adapter

Status: proposed

## Objective

Add a production manual-state `PauseAdapter` for integrity-pinned
`summarize_input_file` work. The adapter incrementally hashes the input at
cooperative chunk boundaries, publishes resumable progress for periodic,
shutdown, or yield capture, and completes with the existing summary output and
ordinary `WorkEvidence`.

## Current State

`Worker.summarizeInputFile` in `cmd/worker/work_summary.go` runs synchronously.
It resolves the `input_path` parameter, stats and hashes the entire file through
`fileSHA256`, computes reuse and pre-state evidence, writes a two-line summary
to the temporary directory, promotes it into `DataDir`, and returns ordinary
output evidence. The worker cannot pause this in-process call while hashing a
large input.

OS-005 defines `model.ManualResumePayload`, which identifies one handler,
handler version, state schema, and declared state file. OS-008 provides the
adapter-neutral supervisor and supports shutdown, periodic, and yield capture.
OS-009 and OS-011 implement DMTCP and native-tool adapters, respectively. No
production adapter currently writes or consumes a manual resume payload, and no
in-process Go handler exposes a cooperative safe boundary.

The existing work item does not declare immutable input content separately from
its path. A replacement cannot safely continue a serialized hash computation
unless the assigned work item pins both the expected input size and SHA-256.

## Target State

Add `cmd/worker/manual_summary_adapter.go` with one concrete adapter for
`model.WorkItemTypeSummarizeInputFile`. The supervised shape requires the
existing non-empty `input_path` plus `input_size_bytes` and `input_sha256`
parameters. The size must be positive and the digest lowercase SHA-256. The
adapter rejects missing fingerprints, unsafe attempts, directories, symlinks,
and an initial file size that differs from the declaration before starting
work.

`StartFresh` opens the current work item's input and hashes it in bounded
chunks. After each chunk, one owned execution goroutine publishes a safe state
containing the byte offset, SHA-256 implementation state, and prefix digest.
No output file is open or promoted until the complete input digest equals the
declared digest. The final summary bytes, reuse decision, temporary-output
promotion, logging, and `WorkEvidence` remain identical to
`Worker.summarizeInputFile`.

Periodic capture snapshots one completed chunk boundary into an immutable
manual artifact and then lets the same execution continue. Shutdown or yield
capture requests a safe boundary, prevents further input reads, writes the
same artifact shape, and returns only after the execution goroutine has
stopped. Capture and termination are bounded, mutually exclusive with terminal
publication, race-safe, and idempotent.

The canonical state JSON records safe work/input/source/code identity, expected
size and digest, completed offset, prefix digest, encoded SHA-256 state, handler
identity/version, state schema, and hash-state contract. It does not record the
input path or file contents. The manifest uses `PauseStrategyManual`, declares
exactly `manual/summary-state.json`, and is synced last.

`StartResume` validates the controller assignment, exact stored manifest and
state bytes, complete compatibility profile, handler/state/hash contracts,
work identity, expected input identity, and canonical non-symlink paths before
starting a goroutine. It reopens the current assigned `input_path`, verifies the
declared size, rehashes the completed prefix to validate the stored prefix
digest, restores the encoded SHA-256 state, seeks to the stored offset, and
continues from the next byte. A changed prefix, suffix, declaration, work item,
runtime identity, or serialized state fails before successful output.

Add a singular explicit `summarize_input_file_manual_v1` profile selector and
complete compatibility profile. It registers one manual adapter for
`summarize_input_file` with shutdown, periodic, and yield capabilities.
Omission preserves the existing synchronous handler and checked-in
configurations remain disabled.

## Concept Decision

This slice adds the first cooperative manual-Go execution concept. It needs its
own `cmd/worker/manual_summary_adapter.go` because chunk ownership, capture
coordination, immutable state publication, resume validation, and terminal
result arbitration form an independent lifecycle from the synchronous summary
function.

The adapter is concrete rather than a generic manual-handler framework. One
passing handler is needed before extracting common abstractions. Existing
summary calculation and evidence helpers should be shared narrowly where doing
so preserves byte-for-byte ordinary behavior.

The serialized standard-library SHA-256 state is accepted only under an exact
adapter, worker, image, OS/architecture/runtime, Go toolchain, and hash-state
contract identity. Resume also rehashes and verifies the stored prefix before
using that state; opaque hash bytes alone are not trusted.

## Required Context

Read these files first:

- `docs/concepts/dmtcp-work-item-checkpoint-resume/README.md`
- `docs/concepts/dmtcp-work-item-checkpoint-resume/005-resume-artifact-contract-model.md`
- `docs/concepts/dmtcp-work-item-checkpoint-resume/008-worker-drain-state-and-adapter-neutral-execution-supervisor.md`
- `cmd/worker/checkpoint_adapter.go`
- `cmd/worker/execution_supervisor.go`
- `cmd/worker/work_summary.go`
- `cmd/worker/work_summary_test.go`
- `cmd/worker/config.go`
- `cmd/worker/worker.go`
- `internal/model/resume_artifact.go`

Do not read or modify DMTCP, rclone, controller persistence, remote
publication, workflow compilation, or unrelated work handlers unless a focused
compile/test failure exposes a direct dependency.

## Allowed Production Files

- `cmd/worker/manual_summary_adapter.go` (new)
- `cmd/worker/work_summary.go` (only to share exact summary finalization and
  evidence behavior)
- `cmd/worker/config.go` (only for the explicit manual-summary profile)
- `cmd/worker/worker.go` (only for conditional construction/registration)

## Allowed Test Files

- `cmd/worker/manual_summary_adapter_test.go` (new)
- `cmd/worker/work_summary_test.go` (only for ordinary/supervised equivalence)
- `cmd/worker/config_test.go` (only for profile validation)
- `cmd/worker/worker_test.go` (only for registration and omission behavior)

## Allowed Documentation Files

- `PROJECT_STATE.md`
- `docs/TEST_AND_SMOKE_STATUS.md`
- `docs/RUNTIME_RUNBOOK.md`
- `cmd/worker/README.md`
- `docs/concepts/README.md`
- `docs/concepts/dmtcp-work-item-checkpoint-resume/README.md`
- `docs/concepts/dmtcp-work-item-checkpoint-resume/012-summarize-input-file-manual-pause-adapter.md`

## Out Of Scope

- Generic manual-handler registration or public plugin APIs.
- Adoption by `write_demo_output`, `asset.materialize`, `commit_data`, Python,
  R, archive extraction, publication, or remote providers.
- Persisting input bytes, input paths, output files, credentials, or arbitrary
  Go memory in a resume artifact.
- Resuming an unpinned, mutable, directory, symlink, device, pipe, or socket
  input.
- Treating modification time, inode, path, or size alone as content identity.
- Stable hash-state compatibility across undeclared Go versions or runtime
  profiles.
- Changing the existing summary text, output promotion, reuse contract, or
  evidence schema.
- Controller transactions, resume limits, queue policy, Slurm forwarding,
  administrative-drain transport, artifact retention, or cross-host shared
  input provisioning.
- Combining multiple pause-adapter profiles in one worker configuration.

## Acceptance Criteria

- Fresh supervised execution accepts only a regular integrity-pinned
  `summarize_input_file` input and produces the same summary bytes and ordinary
  evidence as the synchronous handler.
- One execution goroutine owns the input stream and mutable hash state; capture
  observes only completed chunk boundaries.
- Periodic capture writes and validates an immutable manual artifact while the
  same execution continues to completion.
- Shutdown and yield capture stop at a safe boundary, write the artifact after
  input activity stops, suppress the expected nonterminal race, and permit
  bounded idempotent cleanup.
- The manual artifact declares exactly one canonical state file and excludes
  the input path and input contents.
- Resume validates exact manifest/state bytes, file declarations and digests,
  compatibility, handler/state/hash contracts, work identity, and expected
  input identity before continuing.
- Resume rehashes the completed prefix, rejects changed prefix bytes, restores
  the declared SHA-256 state, and continues reading at the stored offset.
- A suffix change is rejected by the declared final size/SHA-256 before output
  promotion or successful evidence.
- Manifest, state, assignment, profile, input, offset, prefix digest, or hash
  state tampering starts no resumable work or produces no successful output.
- Completion/capture/termination races publish exactly one terminal or
  suspended outcome and pass Go race testing.
- The explicit profile registers one manual `summarize_input_file` adapter with
  shutdown, periodic, and yield capabilities; omission preserves the ordinary
  synchronous path.
- Deterministic local supervisor tests prove fresh completion, periodic
  continue, shutdown suspension and replacement resume, yield suspension and
  replacement resume, repeated capture generations, tamper rejection, and
  ordinary/supervised output equivalence.
- Checked-in configurations remain disabled and no file outside the allowed
  production, test, and documentation lists changes.

## Notes

- Use a versioned state schema such as
  `goetl/summarize-input-file-manual-state/v1`.
- Use a handler identity such as `summarize-input-file` and version `1`.
- Record the exact Go toolchain/hash-state encoding identity in the profile;
  do not infer compatibility from handler version alone.
- Keep the chunk size fixed by the adapter contract for deterministic capture
  boundaries. Tests may inject a boundary hook or reader only inside the
  adapter's testable process/state boundary.
- Writing the final output remains a post-hash atomic action. No partially
  written summary belongs in manual state.
- Implement under repeated `EC-3 / Operational Slice / file(1)+test+doc`
  prompts. The first implementation prompt should add only the fresh
  incremental execution and exact ordinary completion boundary. Capture,
  resume, configuration/registration, and supervisor smoke should follow as
  separate increments.
- This charter is proposed. Do not implement or commit it until the human
  approves the scope.
