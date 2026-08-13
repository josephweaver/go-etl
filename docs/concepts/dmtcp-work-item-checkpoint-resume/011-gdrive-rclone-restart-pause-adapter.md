# 011 Google Drive Rclone Restart Pause Adapter

Status: proposed

## Objective

Implement a production `PauseAdapter` for one immutable, single-file
`gdrive_rclone` `asset.materialize` work item. On shutdown drain, terminate the
owned rclone process and publish a validated restart artifact; on replacement
assignment, discard partial transfer state, relaunch the full transfer, verify
the declared size and SHA-256, and emit ordinary `asset.materialize` evidence.

## Current State

OS-008 provides the adapter-neutral `ExecutionSupervisor`, native resume
manifest model, controller confirmation, and resume assignment. OS-009
registers the DMTCP direct-Python adapter through the singular explicit
`pause_adapter_profile` selector.

`Worker.AssetMaterialize` currently runs synchronously. It decodes one
`AssetMaterializeWorkItemPayload`, checks an existing deterministic materialized
destination, invokes `assetMaterializer.materialize`, promotes the result, and
returns a `goet/materialized-data-assets/v1` output. For `gdrive_rclone`,
`gdriveRcloneProvider.copyTo` starts `rclone copyto` through
`exec.CommandContext`, waits for completion, verifies the temporary local file,
and renames it into the worker cache. The child process is not exposed through
`SupervisedExecution` and cannot be terminated by `ExecutionSupervisor`.

OS-010 tested rclone 1.71.2 against an immutable 33,554,432-byte Google Drive
object. After `SIGKILL`, a 5,468,160-byte `.partial` file survived, but a new
process transferred all 33,554,432 bytes and reproduced the exact expected
SHA-256. The accepted requirement treats this idempotent full retransmission as
a valid restart-on-resume strategy, while explicitly not claiming
partial-prefix continuation.

The shared `model.NativeResumePayload` requires one or more immutable state
files. No production native adapter currently writes or consumes that payload.

## Target State

Add `cmd/worker/rclone_restart_adapter.go` with a concrete native adapter and
an injected rclone command/process boundary. The adapter supports only
`model.WorkItemTypeAssetMaterialize` items whose single bound asset:

- uses provider `gdrive_rclone`;
- is an ordinary single file with no archive extraction;
- uses immutable worker-cache materialization;
- declares nonzero expected size and lowercase SHA-256; and
- resolves through the worker's enabled rclone executable/config boundary.

`StartFresh` must validate that shape, create an owner-only attempt workspace,
and launch the exact structured `rclone copyto` operation into an
attempt-local temporary destination. The long-lived Go worker, supervisor,
controller client, and heartbeat remain outside rclone.

`CaptureForSuspend` must terminate and reap only the adapter-owned rclone
process, remove or exclude every partial data file from the resume artifact,
and write one canonical restart-state JSON file. That state records safe,
non-secret identities sufficient to prove that replacement work is the same
immutable operation: schema, work-item/input/source/code identities, asset and
materialization identities, expected size/SHA-256, adapter version, and exact
rclone/backend/command contract. The immutable native resume manifest is
written last and returned through `PreparedCheckpoint`.

`StartResume` must validate the assignment, exact stored manifest bytes and
reference digest, native strategy, operation, adapter/backend compatibility,
restart-state size/hash, work-item identities, and expected immutable output.
It must not reuse a `.partial` file or stored shell command. After validation,
it launches a new full `copyto` from the current validated work item into the
replacement attempt's workspace.

On successful fresh or resumed completion, the adapter verifies the downloaded
file against expected size/SHA-256, promotes it through the existing
deterministic asset-materialization destination and manifest boundaries, and
returns the same `WorkEvidence` shape as ordinary `Worker.AssetMaterialize`.
Process failure, missing output, integrity mismatch, or conflicting existing
cache evidence remains a normal execution failure.

Add an explicit `gdrive_rclone_restart_1_71_2` profile selector and complete
compatibility profile. Selecting it registers one native adapter for
`asset.materialize` with `PauseAdapterCapabilities{Shutdown: true}`. It must
not advertise periodic or yield: neither mode preserves transfer progress, and
yield would repeatedly restart at each quantum. Omission preserves the current
synchronous path and checked-in configurations remain disabled.

Add fake-rclone tests for launch, suspend artifact publication, replacement
restart, exact output evidence, partial-file exclusion, validation failures,
cleanup, registration, and ordinary-path preservation. Extend the OS-010
container harness or add a worker-level gated smoke that proves one real
supervisor shutdown capture and replacement full restart against the authorized
Google Drive test fixture without retaining credentials or remote paths in
evidence.

## Concept Decision

This slice adds a new restart-only native adapter concept. It belongs in
`cmd/worker/rclone_restart_adapter.go` because owned-process supervision,
restart-state publication, resume validation, and completion finalization have
an independent lifecycle and test surface from the synchronous provider.

The adapter reuses the existing provider's safe argument construction,
redaction, integrity verification, materialized-destination promotion, and
evidence formats. It does not preserve or trust rclone partial bytes. The
resume artifact is a validated restart recipe/identity, not a transfer-progress
checkpoint.

The existing singular `pause_adapter_profile` selector means one worker config
selects either the direct-Python DMTCP profile or this rclone restart profile in
this slice. Multi-adapter profile composition requires a later explicit design;
it must not be hidden behind automatic registration.

## Required Context

Read these files first:

- `docs/concepts/dmtcp-work-item-checkpoint-resume/README.md`
- `docs/concepts/dmtcp-work-item-checkpoint-resume/008-worker-drain-state-and-adapter-neutral-execution-supervisor.md`
- `docs/concepts/dmtcp-work-item-checkpoint-resume/010-gdrive-rclone-copyto-native-continuation-feasibility.md`
- `cmd/worker/checkpoint_adapter.go`
- `cmd/worker/execution_supervisor.go`
- `cmd/worker/gdrive_rclone_provider.go`
- `cmd/worker/work_asset_materialize.go`
- `cmd/worker/data_asset_materializer.go`
- `cmd/worker/config.go`
- `cmd/worker/worker.go`
- `internal/model/resume_artifact.go`

Do not read or modify unrelated controller persistence, DMTCP implementation,
CRIU, workflow compiler, publication, archive extraction, or manual-Go files
unless a focused compile/test failure exposes a direct dependency.

## Allowed Production Files

- `cmd/worker/rclone_restart_adapter.go` (new)
- `cmd/worker/gdrive_rclone_provider.go` (only to share structured command,
  process, or redaction boundaries)
- `cmd/worker/work_asset_materialize.go` (only to share validated
  payload/destination/evidence finalization)
- `cmd/worker/config.go` (only for the explicit restart profile)
- `cmd/worker/worker.go` (only for conditional construction/registration)

## Allowed Test Files

- `cmd/worker/rclone_restart_adapter_test.go` (new)
- `cmd/worker/gdrive_rclone_provider_test.go` (only for shared command boundary)
- `cmd/worker/work_asset_materialize_test.go` (only for shared finalization)
- `cmd/worker/config_test.go` (only for profile validation)
- `cmd/worker/worker_test.go` (only for registration and ordinary-path guards)
- `containers/rclone-continuation-feasibility/rclone-copyto-smoke` (only for a
  gated real supervisor/restart phase)

## Allowed Documentation Files

- `PROJECT_STATE.md`
- `docs/TEST_AND_SMOKE_STATUS.md`
- `docs/RUNTIME_RUNBOOK.md`
- `cmd/worker/README.md`
- `docs/concepts/README.md`
- `docs/concepts/dmtcp-work-item-checkpoint-resume/README.md`
- `docs/concepts/dmtcp-work-item-checkpoint-resume/011-gdrive-rclone-restart-pause-adapter.md`

## Out Of Scope

- Partial-prefix continuation or reuse of rclone `.partial` bytes.
- Periodic checkpoint capability or quantum/yield capability.
- DMTCP/CRIU wrapping of rclone or checkpointing the Go worker.
- HTTP, local-file, registered-location, upload, `copy`, `sync`, `move`,
  directory, archive, Google Docs export, VFS, or non-Google-Drive operations.
- Mutable or integrity-unspecified source objects.
- Automatically registering every enabled `gdrive_rclone` provider.
- Combining DMTCP and rclone profile selectors in one worker configuration.
- Controller transactions, retry policy, queue ordering, Slurm signal
  forwarding, administrative drain, retention, or cross-allocation operations.
- Persisting credentials, config contents, bearer tokens, remote paths, or
  partial data files in public manifests, logs, or ordinary output evidence.
- Deleting the authorized OS-010 remote fixture.

## Acceptance Criteria

- Fresh launch accepts only the exact immutable single-file Google Drive
  `copyto` shape and starts rclone with structured arguments.
- Invalid provider, archive use, mutable cache policy, absent size/SHA-256,
  unsafe values, disabled provider, incomplete profile, or unsupported mode
  starts no child process.
- The adapter owns and can terminate/reap only its rclone child; cleanup is
  bounded and idempotent.
- Shutdown capture excludes partial data, writes one canonical restart-state
  file and exact native manifest/reference last, and returns only after the
  original child is stopped.
- Restart state and manifest contain safe operation identities and integrity
  expectations but no credential/config contents or remote source path.
- Resume rejects manifest/reference/state tampering, incompatible rclone,
  backend, adapter, worker, image, OS/architecture/runtime identities, changed
  work/input/source/code/asset identities, or changed expected output before
  launch.
- Resume ignores/removes stale partial transfer files and launches a full new
  `copyto`; no argument is reconstructed from an unvalidated shell string.
- Successful replacement completion requires exact expected size/SHA-256,
  promotes through existing deterministic destination rules, and returns
  ordinary `asset.materialize` evidence.
- The registered adapter advertises shutdown only. Periodic and yield worker
  modes reject the selected profile rather than entering repeated restarts.
- Omitted profile preserves the existing synchronous `asset.materialize` path,
  and checked-in configs remain unchanged.
- Fake-command tests cover fresh launch, final capture, replacement restart,
  completion, partial exclusion, tampering, cleanup races, registration, and
  default-path preservation.
- A real gated smoke proves supervisor shutdown suspension and replacement
  restart with exact final output, or records a precise external blocker.
- Documentation states that full restart is correct but consumes full transfer
  time/bandwidth and that partial-prefix continuation remains unsupported.
- No file outside the allowed production, test, and documentation lists
  changes.

## Notes

- Use backend identity `rclone-1.71.2:gdrive:copyto-full-restart-v1` unless
  implementation evidence requires a more precise safe identity.
- Use a versioned restart-state schema such as
  `goetl/rclone-restart-state/v1`.
- Keep the artifact manifest native strategy because restart is owned by the
  external tool adapter, even though its state represents operation identity
  rather than byte progress.
- The first implementation prompt should add only the adapter's validated
  fresh-launch/process boundary and fake-command tests. Capture, resume,
  completion, config/registration, and real smoke should follow as separate
  EC-3 increments.
- This charter is intentionally proposed and requires human agreement before
  implementation or commit.

## Implementation Progress

The first implementation increment completed on 2026-08-13.
`cmd/worker/rclone_restart_adapter.go` adds the restart adapter's validated
fresh-launch and owned-process boundary. `StartFresh` accepts only
`asset.materialize`, rejects resume assignments, validates the ordinary work
item and safe attempt ID, decodes the existing materialization payload and its
single bound asset, and starts no child unless the asset is an immutable
single-file `gdrive_rclone` worker-cache acquisition with exact expected size
and SHA-256. The payload provider/location/integrity must match the bound asset;
archive extraction, file-ID access, disabled provider use, and missing rclone
configuration are rejected.

Each launch creates `TmpDir/rclone-restart/<attempt>/` with owner-only intent,
opens restrictive stdout/stderr logs, and builds the same structured
`--config ... copyto remote:path destination --bwlimit ...` argument vector as
the existing provider. An injected runner/process boundary keeps tests free of
real credentials and network access. The returned execution owns one
idempotent kill and waits for the process/reap boundary or caller cancellation.

Focused normal and race tests prove exact argv construction, attempt isolation,
pre-launch rejection, and idempotent termination. Successful transfer
finalization deliberately returns a not-implemented error at this increment;
capture/restart-state publication, `StartResume`, output promotion/evidence,
explicit profile registration, and the real supervisor smoke remain for later
increments.

The second implementation increment completed on 2026-08-13. The adapter now
accepts a complete internal compatibility profile and implements final
shutdown capture. `CaptureForSuspend` validates that the request identifies the
running attempt and uses final capture, marks the expected killed-process result
as nonterminal, kills and reaps the owned rclone child within the capture
context, and only then creates the immutable artifact directory.

The artifact contains one canonical `goetl/rclone-restart-state/v1` JSON file.
It records work/input/code identity, the asset/materialization keys, expected
size and SHA-256, adapter/backend identity, and the fixed full-restart command
contract. It does not contain the Google Drive remote path, rclone config path,
credentials, or partial data. The native `goet/resume-artifact/v1` manifest
declares only that state file, validates against the shared model, and is
written and synced last; exact bytes determine the returned reference digest.

Focused tests prove manifest-last ordering relative to child termination,
partial-file exclusion, sensitive operation-detail exclusion, exact prepared
checkpoint validation, rejection of non-final capture before kill, suppression
of the expected terminal race, and subsequent bounded supervisor cleanup.
Normal and race tests pass. `StartResume`, restart-state consumption, successful
download promotion/evidence, explicit config/registration, and the real
supervisor smoke remain unimplemented.

The third implementation increment completed on 2026-08-13. `StartResume`
requires the supplied assignment to equal the assignment embedded in the
replacement work item, validates the ordinary work item and immutable asset
shape, and consumes the restart artifact before creating a workspace or
starting a child. Validation covers the exact canonical manifest bytes on
shared storage, native operation and backend, every compatibility field, the
single canonical state file's declared size and SHA-256, strict canonical JSON,
work/input/source/code identity, asset/materialization identity, expected
size/SHA-256, and the fixed full-restart command contract.

After validation, the replacement launch removes only a stale `download` or
matching `download.*.partial` regular file from its own attempt workspace and
builds a new full `rclone copyto` argument vector from the current validated
work item. It does not use the producing attempt's workspace, partial bytes,
remote command text, or credentials from the resume artifact. Focused tests
prove the full replacement argv and stale-partial removal and prove assignment,
backend, stored-manifest, and state-file tampering start no process. Successful
download promotion/evidence, explicit config/registration, and the real
supervisor smoke remain for later increments.

The fourth implementation increment completed on 2026-08-13. A successful
rclone process exit now enters the existing asset finalization boundaries. The
adapter hashes the attempt-local download with the configured size limit,
requires the exact declared size and SHA-256, installs or validates the
immutable worker-cache source and cache manifest, promotes through the existing
pinned materialized-destination rules, and builds evidence with
`Worker.assetMaterializeEvidence`. The resulting output is the ordinary
`goet/materialized-data-assets/v1` document used by synchronous
`asset.materialize` work.

Missing downloads, integrity mismatches, cache conflicts, and destination
conflicts remain ordinary execution failures and cannot publish successful
evidence. After the child is reaped, the adapter removes attempt-local download
and `.partial` transfer files while retaining its logs. A focused replacement-
resume test proves exact output promotion and valid ordinary evidence; a
negative test proves mismatched bytes are removed and fail execution.
Configuration/registration and the real supervisor smoke remain for later
increments.

The fifth implementation increment completed on 2026-08-13. Worker config now
accepts the singular explicit `gdrive_rclone_restart_1_71_2` selector and a
complete `rclone_restart_profile`. The profile supplies the shared artifact
root and adapter, worker-contract, image, platform, runtime, and backend
identities. Validation additionally requires shutdown checkpoint mode, the
enabled `gdrive_rclone` provider, and configured rclone executable and config
path; DMTCP and rclone profile bodies cannot be combined under one selector.

The worker constructs exactly one native `asset.materialize` registration for
this profile with `PauseAdapterCapabilities{Shutdown: true}`. Periodic and yield
modes reject it rather than repeatedly restarting a transfer. Omitting the
selector still constructs an empty pause-adapter registry and preserves the
ordinary synchronous execution path. Focused tests prove profile validation,
registration identity, shutdown-only capability, mode rejection, and omission
behavior. The gated real supervisor smoke remains for the final increment.
