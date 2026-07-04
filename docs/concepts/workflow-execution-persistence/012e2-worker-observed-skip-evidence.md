# 012e2 Worker-observed Skip Evidence

Status: implemented

## Objective

Revise the 012e terminal-report contract so the worker can decide that a work
item is already satisfied from worker-local input/output state, report the
observed hashes, and record that outcome as a completed attempt with skip
metadata.

The controller remains the persistence authority. The worker decides whether
execution is necessary because it is the process that can observe the concrete
input and output state immediately before execution.

## Required Context

Read these files first:

- `docs/epics/workflow-execution-persistence/012e-persistence-backed-terminal-reports.md`
- `docs/epics/workflow-execution-persistence/008-attempt-terminal-transition-transaction.md`
- `docs/epics/workflow-execution-persistence/002-canonical-json-and-sha256-helpers.md`
- `internal/fingerprint/canonical_json.go`
- `internal/model/work_item.go`
- `cmd/controller/main.go`
- `cmd/worker/state.go`
- `cmd/worker/work_demo.go`
- `internal/persistence/store.go`

## Design Change

012e made the worker report JSON evidence and the controller compute canonical
hashes for storage. That remains useful, but it is incomplete for skip/reuse.

The worker should also report the hashes it observed from its plugin-defined
state domain. These hashes are not merely hashes of the transport JSON report.
They are worker-observed facts about inputs, outputs, and state around the
operation.

Initial completion report additions:

```go
type WorkCompletion struct {
    ID string `json:"id"`
    AttemptID string `json:"attempt_id,omitempty"`
    Skipped bool `json:"skipped,omitempty"`
    SkippedParentID string `json:"skipped_parent_id,omitempty"`
    SkipReason string `json:"skip_reason,omitempty"`
    InputSHA256 string `json:"input_sha256,omitempty"`
    OutputSHA256 string `json:"output_sha256,omitempty"`
    PreStateSHA256 string `json:"pre_state_sha256,omitempty"`
    PostStateSHA256 string `json:"post_state_sha256,omitempty"`
    OutputJSON string `json:"output_json,omitempty"`
    PreStateJSON string `json:"pre_state_json,omitempty"`
    PostStateJSON string `json:"post_state_json,omitempty"`
}
```

Naming is still open to implementation review. The key distinction is:

- `OutputJSON`, `PreStateJSON`, and `PostStateJSON` are explanatory evidence
  documents.
- `InputSHA256`, `OutputSHA256`, `PreStateSHA256`, and `PostStateSHA256` are
  worker-observed hashes the worker used or produced.
- Controller-computed hashes of the evidence documents may still be stored for
  transport integrity and deterministic database comparison.

## Worker-side Skip

A worker may mark a completion report as skipped when:

1. The assignment includes prior completed attempt candidates.
2. The worker observes its current pre-state and input/output domain.
3. The current observed hashes match one prior candidate under the same
   execution fingerprint.
4. The plugin can safely conclude that the requested operation is already
   satisfied.

The worker still reports `/work/complete`. A skipped completion is terminal
success, not a failed attempt and not untracked local behavior.

The report should include:

```json
{
  "id": "work-001",
  "attempt_id": "attempt-new",
  "skipped": true,
  "skipped_parent_id": "attempt-prior",
  "skip_reason": "matched_worker_observed_state",
  "input_sha256": "...",
  "output_sha256": "...",
  "pre_state_sha256": "...",
  "post_state_sha256": "...",
  "output_json": "{...}",
  "pre_state_json": "{...}",
  "post_state_json": "{...}"
}
```

For a skipped completion, `pre_state_sha256` and `post_state_sha256` may be the
same if the worker made no mutation. That is acceptable and is useful evidence.

## Assignment Candidate Contract

The controller should provide enough prior completed attempt data for the
worker to make a local skip decision.

Initial assignment addition:

```go
type WorkItem struct {
    ID string `json:"id"`
    AttemptID string `json:"attempt_id,omitempty"`
    ReuseCandidates []WorkReuseCandidate `json:"reuse_candidates,omitempty"`
}

type WorkReuseCandidate struct {
    AttemptID string `json:"attempt_id"`
    InputSHA256 string `json:"input_sha256,omitempty"`
    OutputSHA256 string `json:"output_sha256,omitempty"`
    PreStateSHA256 string `json:"pre_state_sha256,omitempty"`
    PostStateSHA256 string `json:"post_state_sha256,omitempty"`
    OutputJSONSHA256 string `json:"output_json_sha256,omitempty"`
}
```

The controller should select candidates using a composite execution fingerprint,
not by work-item ID alone.

Initial candidate key:

```text
resolved_input_sha256
controller_sha256
plugin_sha256
operation/plugin name
semantic plugin parameters hash
```

## Persistence Mapping

The existing `completed_work.skipped_parent_id` can represent the prior
completed attempt used for reuse.

The current schema already stores:

- `output_json`
- `output_json_sha256`
- `pre_state_sha256`
- `post_state_sha256`
- `skipped_parent_id`

Potential schema gap:

- There is no explicit `input_sha256` or `output_sha256` column on
  `completed_work`.
- There is no explicit `skipped` boolean; skipped status is currently implied
  by non-empty `skipped_parent_id`.
- There is no `skip_reason` column.

Implementation may either:

1. add columns for worker-observed `input_sha256`, `output_sha256`, and
   `skip_reason`; or
2. store those values in `output_json` for the first cut and defer schema
   expansion.

The stronger design is to add explicit columns before this becomes a production
contract, because those hashes will be query inputs for future reuse decisions.

## Acceptance Criteria

- `internal/fingerprint` remains the shared canonical JSON and SHA-256 helper
  package usable by both controller and worker code.
- Worker completion reports can include worker-observed input, output,
  pre-state, and post-state hashes.
- Worker completion reports can mark the attempt as skipped.
- Skipped completion reports can identify `skipped_parent_id`.
- Persisted completion maps skip metadata to completed terminal state, not
  failure state.
- The controller can include prior completed attempt candidates in work
  assignments.
- Reuse candidates are selected from a composite execution fingerprint including
  resolved inputs, controller identity, plugin identity, operation name, and
  semantic plugin parameters.
- The worker can compare observed hashes with candidate hashes before deciding
  whether to execute.
- Legacy non-skipped completion still works.

## Out Of Scope

- Full retry policy.
- Lease/fencing for stale workers.
- Final plugin state-observation schema.
- Large artifact manifest storage.
- Cross-workflow reuse search.
- Source-control implementation for controller/plugin SHA definitions.

## Ambiguity To Review

`controller_sha256` needs a precise definition. It may mean controller binary
revision, workflow compiler version, controller configuration hash, or a
composite of all execution-relevant controller code and configuration.

`plugin_sha256` also needs a precise definition. It may mean worker binary
revision, plugin source revision, plugin package hash, container image digest,
or plugin configuration hash.

`output_sha256` needs a plugin-owned meaning. For a single file it can be file
content SHA-256. For multiple files or external outputs it needs a canonical
manifest hash.

`input_sha256` likewise needs a plugin-owned state domain. Some operations may
have multiple files, remote resources, database rows, or no material input
file.

It is still open whether this slice should add explicit database columns or
first carry worker-observed hashes inside `output_json`. Explicit columns are
better for queries, but they require a schema slice.

## Implementation Notes

This slice implemented the no-schema-expansion path.

- `internal/fingerprint` remains the shared canonical JSON and SHA-256 helper.
- `model.WorkItem` now carries optional `reuse_candidates`.
- `model.WorkCompletion` now carries skip metadata and worker-observed
  input/output/pre-state/post-state hashes.
- Worker demo and summary operations compute observed hashes through the shared
  fingerprint helper after normalizing Go structs into JSON-shaped values.
- The demo operation can skip execution when the current pre-state and expected
  output match a supplied reuse candidate.
- The summary operation computes an input observation that includes the input
  file path, size, and file-content SHA-256.
- Persisted `/work/complete` maps `skipped_parent_id` to
  `CompleteAttemptRequest.SkippedParentID`.
- Persisted `/work/next` includes reuse candidates from prior completed
  attempts in the same run when the persisted `resolved_inputs_sha256` and
  `worker_payload_json` match.

The implementation intentionally does not add explicit `completed_work`
columns for `input_sha256`, `output_sha256`, or `skip_reason`. Those values are
currently carried in completion transport fields and embedded in canonical
`output_json`. A future schema slice should add explicit columns before broad
reuse queries depend on them.

The implementation also does not define final `controller_sha256` or
`plugin_sha256` semantics. Until those are formalized, persisted candidate
selection uses `resolved_inputs_sha256` plus exact `worker_payload_json` as a
conservative stand-in for the larger execution fingerprint.
