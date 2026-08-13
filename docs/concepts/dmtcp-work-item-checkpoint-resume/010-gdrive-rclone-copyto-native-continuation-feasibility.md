# 010 Google Drive Rclone Copyto Native-Continuation Feasibility

Status: implemented (full-restart resume passed; partial continuation unavailable)

## Objective

Determine the restart behavior of the exact `gdrive_rclone` acquisition used
by GOET—`rclone copyto <google-drive-remote>:<path> <local-temp-file>`—after
process termination. Distinguish partial-prefix continuation from idempotent
full restart, and record which validated resume contract a future adapter may
implement.

## Current State

`cmd/worker/gdrive_rclone_provider.go` invokes a configured external rclone
process with a structured `copyto` argument vector. A Google Drive object is
downloaded into a random local temporary path, hashed, and renamed into the
worker asset cache only after successful completion. On an ordinary command
error, GOET removes the temporary path.

`asset.materialize` executes synchronously through
`Worker.AssetMaterialize`; it does not implement `SupervisedExecution`, does
not expose the child rclone process to `ExecutionSupervisor`, and cannot
capture or consume a native resume artifact. The shared
`model.NativeResumePayload` can describe an operation, adapter version,
backend identity, and state-file paths, but no production native adapter uses
it.

The Strategic Concept assumes that a native rclone adapter may terminate its
owned process, retain a partial workspace, and relaunch through proven native
continuation semantics. That assumption is not yet evidence. Rclone's current
official copy documentation states that incomplete temporary transfers are
deleted after transfer failure and does not promise that a later `copyto`
process resumes a Google Drive download from existing partial bytes. GOET must
therefore distinguish continuation from a fresh retransmission before adapter
implementation.

## Target State

Add a repeatable Linux container/WSL feasibility harness under
`containers/rclone-continuation-feasibility/` that pins the tested rclone
version and records all non-secret inputs and evidence needed to classify one
Google Drive download.

The harness must:

1. require a caller-supplied rclone config path, remote name, immutable fixture
   path, expected size, and expected SHA-256 without copying credentials into
   the repository or evidence;
2. launch the same single-file `rclone copyto` download shape used by
   `gdriveRcloneProvider` into an owner-only durable workspace;
3. use a bounded bandwidth and fixture size that leave the first transfer
   incomplete long enough to inspect;
4. terminate the first rclone process and record the surviving partial files,
   their sizes and hashes, sanitized rclone logs, and source-request range or
   byte-offset evidence when rclone exposes it;
5. start a new rclone process with the exact same validated command inputs and
   workspace;
6. verify the completed destination size and SHA-256; and
7. classify whether the second process reused partial bytes from a nonzero
   offset or retransferred the object from byte zero.

A result must identify the exact rclone version, Google Drive backend,
command arguments, partial-state paths, termination method, relaunch behavior,
and evidence that bytes before the interruption were not downloaded again. If
that cannot be proven, the slice must not claim partial continuation. A full
retransfer is still a valid restart-on-resume result when it reproduces the
declared immutable size and SHA-256 without exposing a partial final object.
The distinction must remain explicit because full restart has no transfer-time
or bandwidth savings.

## Concept Decision

This slice adds a feasibility gate, not a production adapter. It narrows the
Strategic Concept's proposed native-tool path to the only rclone-backed worker
operation that currently exists: a Google Drive single-file download through
`copyto`.

The gate needs its own container harness because process termination, a pinned
rclone binary, durable partial files, and transfer-offset evidence are
independent of fake-command unit tests. Production changes to
`gdrive_rclone_provider.go`, `work_asset_materialize.go`, adapter registration,
or worker configuration are deferred until the gate demonstrates real native
continuation.

## Required Context

Read these files first:

- `docs/concepts/dmtcp-work-item-checkpoint-resume/README.md`
- `docs/concepts/data-assets-and-materialized-outputs/008-gdrive-rclone-data-provider.md`
- `cmd/worker/gdrive_rclone_provider.go`
- `cmd/worker/gdrive_rclone_provider_test.go`
- `cmd/worker/work_asset_materialize.go`
- `cmd/worker/checkpoint_adapter.go`
- `internal/model/resume_artifact.go`
- [official rclone `copyto` documentation](https://rclone.org/commands/rclone_copyto/)
- [official rclone global copy/partial-transfer documentation](https://rclone.org/docs/)

Do not read or modify unrelated controller, DMTCP, CRIU, workflow compiler,
publication, or manual-Go files unless a focused harness failure exposes a
direct dependency.

## Allowed Production Files

None. This is a feasibility slice.

## Allowed Test Files

- `containers/rclone-continuation-feasibility/Dockerfile` (new, pinned rclone
  test environment)
- `containers/rclone-continuation-feasibility/test` (new, build/version guard)
- `containers/rclone-continuation-feasibility/rclone-copyto-smoke` (new,
  termination/relaunch/evidence harness)

## Allowed Documentation Files

- `PROJECT_STATE.md`
- `docs/TEST_AND_SMOKE_STATUS.md`
- `docs/RUNTIME_RUNBOOK.md`
- `docs/concepts/README.md`
- `docs/concepts/dmtcp-work-item-checkpoint-resume/README.md`
- `docs/concepts/dmtcp-work-item-checkpoint-resume/010-gdrive-rclone-copyto-native-continuation-feasibility.md`

## Out Of Scope

- Implementing or registering a `PauseAdapter`.
- Changing `gdriveRcloneProvider`, `Worker.AssetMaterialize`, worker config, or
  the native resume-artifact model.
- Treating retry-from-byte-zero as partial-prefix continuation. It may qualify
  separately as restart-on-resume when exact final integrity is reproduced.
- Upload continuation, `copy`, `sync`, `move`, directory transfers, Google
  Docs export, VFS mount/cache behavior, or non-Google-Drive backends.
- Creating, modifying, deleting, or publishing remote Google Drive objects.
- Checking credentials, tokens, config contents, private remote paths, or
  customer filenames into Git or ordinary logs.
- Claiming support for every rclone version, command, backend, or remote
  configuration.
- Controller lifecycle, checkpoint persistence, Slurm signal forwarding,
  administrative drain, retention, or cross-allocation implementation.

## Acceptance Criteria

- The harness refuses to run without explicit fixture identity, expected size,
  expected SHA-256, remote name, config path, and evidence directory.
- The image or downloaded binary pins an exact rclone version and verifies its
  version before transfer.
- Secrets and config contents are absent from retained command, process, and
  summary evidence; the config is mounted read-only and referenced only by a
  redacted placeholder in summaries.
- The first `copyto` is terminated while incomplete, and evidence records
  whether any non-empty partial state survives.
- Relaunch uses the same source, destination, and continuation-relevant flags.
- The final file matches the declared size and SHA-256.
- `pass_native_continuation` requires direct evidence that the second process
  starts reading after byte zero and does not retransmit the completed prefix.
- `pass_restart` requires the replacement to retransmit the full expected byte
  count and reproduce the exact final size and SHA-256.
- Missing or ambiguous transfer evidence, incompatible relaunch behavior, or
  final-integrity failure produces `failed_resume` rather than pass.
- The summary uses a versioned JSON schema and records phase, classification,
  rclone version, backend, command shape, termination method, partial-state
  metadata, final integrity result, and non-secret evidence location.
- Documentation states whether a future adapter may claim partial continuation
  or only full restart, including the latter's cost tradeoff.
- Shell syntax validation passes, and default Go tests remain unchanged because
  this slice modifies no Go code.

## Notes

- Use an immutable ordinary binary file, not a Google Docs export, because
  exported content can change representation independently of the transfer.
- Prefer read-only Google Drive credentials for the fixture remote.
- Keep the first transfer slow through rclone's existing `--bwlimit` argument;
  do not add workflow-controlled arbitrary flags.
- Abrupt process termination may differ from a graceful command failure. Record
  the exact signal and test only the termination behavior that a future worker
  adapter can reliably issue and bound.
- If rclone cannot expose conclusive remote byte-range evidence, classify the
  result as ambiguous/blocked instead of inferring continuation from elapsed
  time or a surviving filename.
- A full-restart result may feed a restart-only adapter while a future transfer
  mechanism can still pursue partial-prefix continuation for efficiency.

## Implementation Progress

The first implementation increment completed on 2026-08-13. A new pinned
Linux-amd64 image downloads rclone 1.71.2 from the official archive, verifies
SHA-256
`ab9fa5877cee91c64fdfd61a27028a458cf618b39259e5c371dc2ec34a12e415`,
and fails its build unless the binary reports `rclone v1.71.2`. The local WSL
build passed and produced image ID
`sha256:902ee0bf8f98fdb7614a1d65ab4bd24035174cb2e15a35f2612d56c7100c76e4`.

`rclone-copyto-smoke` now validates every required external input before
starting Docker, mounts the caller's config read-only, uses a generic
owner-only destination workspace, interrupts the first `copyto` with
`SIGKILL`, inventories surviving state, and relaunches the same argument
vector. Raw subprocess logs live only in an owner-only temporary directory;
retained logs replace the source and config path with redacted labels. The
versioned summary can report pass only when final size/hash match, non-empty
partial state survived, and rclone's second-process stats report fewer than
the full expected bytes. A correct file produced by full retransmission is
classified `pass_restart`; it is not mislabeled as partial continuation.

Shell syntax, missing-input failure, official archive integrity, and the image
version guard pass. The mounted WSL account has no host rclone and no default
`~/.config/rclone/rclone.conf`; therefore no Google Drive request or credential
access occurred. The real fixture run and resulting pass/blocked
classification remain for the next increment. Production adapter code and
registration remain unchanged.

The second implementation increment completed on 2026-08-13 using rclone
1.71.2 against an ordinary immutable 33,554,432-byte object on the configured
Google Drive backend. The object had expected SHA-256
`83ee47245398adee79bd9c0a8bc57b821e92aba10f5f9ade8a5d1fae4d8c4302`.
The config file was tightened from mode `0644` to `0600` before access.
The user explicitly authorized creation of the generated fixture only beneath
the remote `Data/ETL/Test` folder; no other remote object was created or
modified, and the fixture remains there for repeatability.

The first `copyto`, limited to 1 MiB/s, was killed after the local partial
workspace exceeded 2 MiB. A single 5,468,160-byte
`destination.e84de822.partial` file survived. The replacement process used the
same source, generic destination, bandwidth limit, and config reference. It
transferred 33,554,432 bytes—the full source size—before producing a final file
with the expected size and SHA-256. Thus rclone retained a partial filename but
did not continue the Google Drive download from its completed prefix.

The versioned summary at
`/tmp/goetl-os010-gdrive-restart-20260813/summary.json` records
`pass_restart`, full final integrity, the surviving partial metadata, and the
full replacement transfer count. Retained evidence
contains neither the remote source path nor the host config path. This is a
decisive proof that partial-prefix continuation is unavailable for the tested
single-file Google Drive `copyto` contract, not an environment blocker. By the
accepted requirement, exact full retransmission is a valid restart-on-resume
strategy. The harness now reports that outcome as `pass_restart`; a future
adapter must advertise and document restart-only behavior rather than native
partial continuation.
