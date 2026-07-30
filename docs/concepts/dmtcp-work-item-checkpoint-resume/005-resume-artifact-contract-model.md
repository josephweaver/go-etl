# 005 Resume-Artifact Contract Model

Status: implemented

## Objective

Add a shared, versioned, validated Go model for an immutable resume-artifact
manifest and its controller-facing reference. The model establishes the data
contract that future DMTCP, native-tool, and manual-Go pause adapters must
produce without implementing an adapter, persistence, or worker lifecycle
behavior.

## Current State

`internal/model` owns role-neutral JSON transport models shared by the
controller, worker, clients, and tests. `internal/model/work_item.go` defines
work assignment, completion, and failure documents. `internal/model/
artifact_manifest.go` defines completed workflow artifacts and validates their
relative paths, sizes, and schema.

The controller persistence layer currently records queued, running, abandoned,
completed, and failed work. Every claim creates a distinct `attempt_id`, and
running/terminal transitions are fenced by worker and worker-session identity.
There is no suspended attempt, pending-resume state, predecessor link,
execution-lineage record, or resume-attempt count yet.

OS-003 and OS-004 proved that directly launched R and Python execution can
produce usable DMTCP checkpoint generations when the Go worker remains outside
DMTCP. The Strategic Concept also selects native continuation for tools such as
`rclone` and manual application state for in-process Go handlers.

No shared Go type currently represents:

- the common identity, lineage, storage, compatibility, integrity, and
  retention facts for one accepted resume artifact;
- the exact strategy selected for that artifact;
- the DMTCP, native-tool, or manual-Go strategy payload;
- the complete inventory of required files; or
- the small reference that a worker can report and a pending work record can
  retain without moving checkpoint bytes through the controller API.

Without that contract, later worker and controller slices would each invent
their own artifact shape.

## Target State

`internal/model/resume_artifact.go` defines the role-neutral
`goet/resume-artifact/v1` contract.

The production file contains:

- `PauseStrategy` with only `dmtcp`, `native`, and `manual`;
- `ResumeArtifactManifest`, the immutable manifest written last after all
  strategy state is durable;
- `ResumeArtifactReference`, the small storage reference and manifest digest
  later controller requests and pending records can carry;
- `ResumeArtifactCompatibility`, containing the adapter, worker execution
  contract, worker build, container image, operating system, architecture, and
  container-runtime identities required to fail closed before resume;
- `ResumeArtifactFile`, containing a clean relative path, byte size, and
  lowercase SHA-256 for every required regular file;
- `DMTCPResumePayload`, containing the exact DMTCP build identity and ordered
  checkpoint image paths;
- `NativeResumePayload`, containing the operation, adapter/backend identities,
  and typed state-file references needed by a later native adapter; and
- `ManualResumePayload`, containing the handler identity/version, manual-state
  schema, and state-file reference needed by a later Go handler.

The common manifest fields are:

```text
schema
resume_artifact_id
resume_generation
pause_strategy
work_item_id
work_item_type
producing_attempt_id
execution_lineage_id
input_fingerprint
source_version
code_version
created_at
storage_scope
storage_relative_path
retention_policy
compatibility
files
exactly one strategy payload
```

The initial storage and retention values are:

```text
storage_scope: shared_tmp
retention_policy: while_referenced
```

`storage_relative_path` is relative to the configured protected shared
temporary root. The model never stores a node-local path, an absolute host
path, a shell command, or checkpoint bytes. The future storage layer joins the
validated relative path to its configured root.

`ResumeArtifactManifest.Validate` fails closed when:

- the schema, strategy, storage scope, or retention policy is unsupported;
- required identity, lineage, fingerprint, version, timestamp, or
  compatibility fields are absent or contain leading/trailing whitespace;
- the artifact identity is not safe as one storage path segment;
- generation is less than one;
- a file path is absolute, unclean, duplicated, or uses backslashes or parent
  traversal;
- a file size is negative or a SHA-256 is not exactly 64 lowercase
  hexadecimal characters;
- no required file is declared;
- zero, two, or three strategy payloads are present;
- the selected strategy does not match the one present payload;
- a DMTCP checkpoint, native state file, or manual state file is absent from
  the common file inventory; or
- strategy-specific required identities or path references are absent.

`ResumeArtifactReference.Validate` requires the artifact identity, exact
schema, `shared_tmp` scope, clean manifest-relative path, and lowercase
manifest SHA-256. It carries no strategy state and cannot substitute for
validating the referenced manifest.

`internal/model/resume_artifact_test.go` proves stable JSON field names,
round-trip behavior, valid examples for all three strategies, and table-driven
rejection of invalid common, file, reference, compatibility, and
strategy-specific states.

This model is the common value boundary used by later slices. It does not add a
Go `PauseAdapter` interface. That behavioral interface belongs with the worker
supervisor after the immutable artifact contract and controller lifecycle are
known; it will consume and produce these shared model values rather than define
a second manifest shape.

## Concept Decision

This slice adds the resume-artifact document as a new shared concept. It needs
its own `internal/model/resume_artifact.go` file because it has an independent
schema, validation invariants, strategy-specific types, and test surface.
Adding it to `work_item.go` would mix immutable resume state with ordinary work
assignment and terminal-report documents.

The common manifest uses one tagged union rather than an untyped
`map[string]any`. Exactly one typed strategy payload is present, and it must
match `pause_strategy`. This keeps controller validation independent of worker
implementation while preventing arbitrary shell fragments or silently ignored
strategy fields.

Completed workflow artifacts and resume artifacts remain separate concepts.
`ArtifactManifest` describes publishable or workflow-visible completed output.
`ResumeArtifactManifest` describes sensitive internal continuation state and
must never be promoted as an ordinary workflow artifact.

## Required Context

Read these files first:

- `docs/concepts/dmtcp-work-item-checkpoint-resume/README.md`
- `docs/concepts/dmtcp-work-item-checkpoint-resume/003-r-tidyverse-brms-dmtcp-feasibility.md`
- `docs/concepts/dmtcp-work-item-checkpoint-resume/004-direct-python-dmtcp-feasibility.md`
- `internal/model/README.md`
- `internal/model/artifact_manifest.go`
- `internal/model/artifact_path.go`
- `internal/model/work_item.go`
- `internal/model/artifact_manifest_test.go`

Do not read worker dispatch, controller handlers, SQLite schema, Slurm
generation, signal handling, DMTCP orchestration, rclone providers, manual Go
handlers, or unrelated workflow/model files unless a focused model test exposes
a direct compile dependency.

## Allowed Production Files

- `internal/model/resume_artifact.go` (new)

## Allowed Test Files

- `internal/model/resume_artifact_test.go` (new)

## Allowed Documentation Files

- `internal/model/README.md`
- `PROJECT_STATE.md`
- `docs/TEST_AND_SMOKE_STATUS.md`
- `docs/concepts/README.md`
- `docs/concepts/dmtcp-work-item-checkpoint-resume/README.md`
- `docs/concepts/dmtcp-work-item-checkpoint-resume/005-resume-artifact-contract-model.md`

## Out Of Scope

- Adding a worker `PauseAdapter` Go interface, adapter registry, adapter
  selection table, or operation dispatch.
- Modifying `model.WorkItem`, `WorkCompletion`, `WorkFailure`, worker
  registration, heartbeat, stop, or controller status models.
- Adding a pause-report or resume-claim HTTP endpoint.
- Changing SQLite schema, persistence records, queue transitions, attempt
  states, ownership fencing, CareTaker behavior, or resume-attempt limits.
- Writing manifests or state files, joining them to the configured shared
  temporary root, checking filesystem permissions, reading file bytes, or
  recomputing hashes from storage.
- Implementing DMTCP launch/checkpoint/restart, rclone termination/relaunch, or
  manual Go safe-boundary behavior.
- Adding Slurm warnings, signal propagation, drain state, pause timers,
  administrative drain, or completion/pause arbitration.
- Encrypting resume state, scanning process images for secrets, exposing
  resume artifacts as workflow outputs, or implementing retention cleanup.
- Claiming compatibility for any execution shape beyond the evidence recorded
  by OS-003 and OS-004.

## Acceptance Criteria

- `internal/model/resume_artifact.go` is the only production file added or
  modified.
- The schema constant is exactly `goet/resume-artifact/v1`.
- JSON field names match the Target State contract and remain role-neutral;
  they contain no controller database or worker process-handle type.
- `dmtcp`, `native`, and `manual` manifests each validate and survive JSON
  marshal/unmarshal without losing common or strategy-specific values.
- Unknown strategies, storage scopes, retention policies, and schemas fail
  validation.
- A manifest requires one and only one typed strategy payload matching
  `pause_strategy`.
- The common file inventory is non-empty, has unique clean relative paths,
  non-negative sizes, and exact lowercase SHA-256 values.
- Every strategy payload references only paths present in the common file
  inventory.
- DMTCP validation requires an exact build identity and at least one ordered
  checkpoint image.
- Native validation requires operation, adapter version, backend identity, and
  at least one typed state-file reference; it cannot store a shell command.
- Manual validation requires handler identity/version, state schema, and one
  state-file reference.
- Compatibility validation requires non-empty worker-contract, worker-build,
  image, operating-system, architecture, and container-runtime identities.
- Artifact identity and every stored path reject absolute paths, drive
  prefixes, backslashes, empty/dot/parent segments, leading/trailing
  whitespace, and unsafe artifact path segments.
- `ResumeArtifactReference` validates its manifest path and digest separately
  from the referenced manifest.
- Tests cover empty required fields, invalid timestamp, generation zero,
  malformed/uppercase hashes, duplicate files, missing referenced files,
  multiple payloads, mismatched strategy/payload, and invalid reference paths.
- `go test ./internal/model -count=1` passes.
- Project and model documentation state that the model is implemented but no
  controller persistence, worker adapter, or pause/resume lifecycle exists.
- No file outside the allowed production, test, and documentation lists
  changes.

## Notes

- Reuse `ValidateArtifactRelativePath` for every relative file or manifest
  path. Do not create a weaker resume-only path validator.
- Keep timestamp validation structural and deterministic by requiring RFC 3339;
  do not compare it with the local clock.
- Keep SHA-256 values lowercase and unprefixed in file/reference fields so
  callers cannot represent the same digest in multiple ways.
- `input_fingerprint`, source version, and code version are compatibility
  identities, not authorization tokens.
- The manifest may name sensitive files but must not contain protected values,
  environment contents, arbitrary command strings, or file bytes.
- The manifest-last storage protocol is documented here but implemented by a
  later storage/supervisor slice.
- A future controller lifecycle slice may persist the manifest JSON, its
  reference, or normalized fields. This slice does not decide database
  normalization.
- A future worker-supervisor slice should define the behavioral adapter
  interface in `cmd/worker` using these model types after pause acceptance and
  resume-claim behavior are concrete.

## Implementation Result

OS-005 was implemented on 2026-07-30.

`internal/model/resume_artifact.go` now defines:

- schema `goet/resume-artifact/v1`;
- the `dmtcp`, `native`, and `manual` `PauseStrategy` values;
- the common `ResumeArtifactManifest` and `ResumeArtifactReference`;
- required compatibility, storage, retention, file-integrity, attempt, and
  execution-lineage fields;
- typed DMTCP, native-tool, and manual-Go payloads; and
- fail-closed validation for the common manifest, reference, compatibility,
  file inventory, and strategy-specific paths.

The native payload carries its adapter version as typed strategy state and
must match the common compatibility adapter version. Every strategy-referenced
file must also exist in the common inventory. Paths reuse
`ValidateArtifactRelativePath` and additionally reject control characters.

`internal/model/resume_artifact_test.go` proves valid JSON round trips for all
three strategies and rejects unsupported schemas/strategies, unsafe identities
and paths, missing common and compatibility fields, invalid timestamps,
generation zero, malformed or uppercase hashes, duplicate files, missing
referenced files, multiple or mismatched payloads, and invalid artifact
references.

Focused verification:

```text
go test ./internal/model -count=1
ok  	goetl/internal/model
```

No controller persistence, HTTP route, work assignment field, worker adapter,
runtime storage writer, Slurm signal behavior, or pause/resume execution was
added. Those remain later Operational Slices.
