# Database Design

SQLite schema notes for durable workflow execution.

This is a working design note. The active Strategic Concept README and numbered
Operational Slices under `docs/concepts/` are closer to implementation and take
precedence when they differ from this file.

## `projects`

```sql
CREATE TABLE projects (
    project_id    TEXT PRIMARY KEY,
    repo_ref      TEXT NOT NULL,
    config_path   TEXT NOT NULL,
    config_sha256 TEXT NOT NULL CHECK (length(config_sha256) = 64),
    created_at    TEXT NOT NULL
);
```

`repo_ref` identifies the repository. `config_path` is relative to its root.
`config_sha256` hashes the canonical project-config JSON.
Project rows are immutable. A materially different configuration receives a
new `project_id`, even when its repository and path are unchanged.

## Workflow Definitions

```sql
CREATE TABLE workflows (
    project_id      TEXT NOT NULL REFERENCES projects(project_id),
    workflow_id     TEXT NOT NULL,
    workflow_path   TEXT NOT NULL,
    workflow_sha256 TEXT NOT NULL CHECK (length(workflow_sha256) = 64),
    created_at      TEXT NOT NULL,
    PRIMARY KEY (project_id, workflow_id)
);
```

`workflow_path` is relative to the project repository root. `workflow_sha256`
hashes the canonical workflow JSON.
Workflow rows are immutable. A materially different workflow receives a new
`workflow_id`.

## `workflow_instances`

One row represents one submitted workflow run.

```sql
CREATE TABLE workflow_instances (
    run_id                  TEXT PRIMARY KEY,
    project_id              TEXT NOT NULL,
    workflow_id             TEXT NOT NULL,
    source_commit_sha       TEXT NOT NULL,
    submission_context_json TEXT NOT NULL
        CHECK (json_valid(submission_context_json)),
    submitted_at            TEXT NOT NULL,
    FOREIGN KEY (project_id, workflow_id)
        REFERENCES workflows(project_id, workflow_id)
);
```

`source_commit_sha` records the repository revision used for this submission.
The repository and commit must remain fetchable for restart and audit.
`submission_context_json` stores immutable resolver inputs captured at
submission, including generated variables, timezone, overrides, and versions.
Together with the repository commit and immutable definitions, it reconstructs
the variable stack used by later compilation.

## Submission Transaction

In one transaction, insert any new immutable project and workflow metadata,
then insert the workflow instance. Acknowledge the client with `run_id` only
after commit succeeds.

## `work_items`

One row stores one immutable compiled unit of work.

```sql
CREATE TABLE work_items (
    work_item_id   TEXT PRIMARY KEY,
    run_id         TEXT NOT NULL REFERENCES workflow_instances(run_id),
    stage_index    INTEGER NOT NULL CHECK (stage_index >= 0),
    work_item_index INTEGER NOT NULL CHECK (work_item_index >= 0),
    worker_payload_json TEXT NOT NULL CHECK (json_valid(worker_payload_json)),
    resolved_inputs_sha256 TEXT NOT NULL
        CHECK (length(resolved_inputs_sha256) = 64),
    created_at     TEXT NOT NULL,
    UNIQUE (run_id, stage_index, work_item_index)
);
```

The composite uniqueness constraint makes repeated stage compilation
idempotent. `worker_payload_json` contains the compiled worker payload that the
worker needs to execute the item. `resolved_inputs_sha256` supports prior-work
lookup without including unrelated submission context. Retries reuse both
values. The full resolved input document may be reconstructed from source
documents and resolver inputs; storing it directly in this table is not part of
the current epic design.

`stage_index` is the index of a logical block, commonly one step or a collection
of parallel steps. `work_item_index` is the ordinal within a fanout operation.
Every stage produces at least one work item; a no-op stage produces a skipped
work item.

## `workers`

```sql
CREATE TABLE workers (
    worker_id         TEXT PRIMARY KEY,
    worker_state_json TEXT NOT NULL CHECK (json_valid(worker_state_json)),
    created_at        TEXT NOT NULL
);
```

When a worker starts, it registers `worker_state_json` containing exactly the
execution-environment-specific identity the controller needs to terminate that
worker later. The document is a versioned cancellation handle, not a general
worker status document and not a place for credentials.

For a worker started as a Slurm job, the minimum state is conceptually:

```json
{
  "schema_version": 1,
  "scheduler": "slurm",
  "job_id": "12345"
}
```

The controller combines this state with the project/run execution-environment
definition to select the correct scheduler adapter and request cancellation.
Other execution environments provide their own minimal versioned state, such
as a container ID or process identity.

Worker registration must commit this cancellation state before the worker may
claim work. If the controller later declares the worker's attempt abandoned,
the persisted state remains available after controller restart for best-effort
cleanup of the old worker. Liveness, heartbeat timestamps, and worker
capabilities are separate concerns and are not stored in `worker_state_json`.

## `work_item_attempts`

```sql
CREATE TABLE work_item_attempts (
    attempt_id   TEXT PRIMARY KEY,
    work_item_id TEXT NOT NULL REFERENCES work_items(work_item_id),
    attempt_index INTEGER NOT NULL CHECK (attempt_index >= 0),
    worker_id    TEXT NOT NULL REFERENCES workers(worker_id),
    started_at   TEXT NOT NULL,
    UNIQUE (work_item_id, attempt_index),
    UNIQUE (attempt_id, work_item_id)
);
```

Retries increment `attempt_index` and create a new `attempt_id` for the same
`work_item_id`. `worker_id` identifies the worker assigned at claim time.

## `queued_work`

One row means a logical work item is available for assignment.

```sql
CREATE TABLE queued_work (
    work_item_id TEXT PRIMARY KEY
        REFERENCES work_items(work_item_id),
    queued_at    TEXT NOT NULL
);
```

Assignment order is `queued_at`, then `work_item_id` for a stable tie-break.

## `running_work`

One row means one attempt currently owns a logical work item.

```sql
CREATE TABLE running_work (
    attempt_id   TEXT PRIMARY KEY,
    work_item_id TEXT NOT NULL UNIQUE,
    queued_at    TEXT NOT NULL,
    FOREIGN KEY (attempt_id, work_item_id)
        REFERENCES work_item_attempts(attempt_id, work_item_id)
);
```

## `completed_work`

```sql
CREATE TABLE completed_work (
    attempt_id         TEXT PRIMARY KEY
        REFERENCES work_item_attempts(attempt_id),
    output_json_sha256 TEXT NOT NULL CHECK (length(output_json_sha256) = 64),
    output_json        TEXT NOT NULL CHECK (json_valid(output_json)),
    pre_state_sha256   TEXT NOT NULL CHECK (length(pre_state_sha256) = 64),
    post_state_sha256  TEXT NOT NULL CHECK (length(post_state_sha256) = 64),
    skipped            INTEGER NOT NULL DEFAULT 0
        CHECK (skipped IN (0, 1)),
    skip_reason        TEXT,
    queued_at          TEXT NOT NULL,
    started_at         TEXT NOT NULL,
    finished_at        TEXT NOT NULL,
    CHECK (
        (skipped = 0 AND skip_reason IS NULL) OR
        (skipped = 1 AND length(trim(skip_reason)) > 0)
    )
);
```

`output_json_sha256` verifies the canonical `output_json`. `pre_state_sha256`
records the plugin-defined external state observed before execution;
`post_state_sha256` records the same state domain after success. A later pre-state
matching a prior post-state indicates that the requested state already exists.

Skipped work is completed work. `skipped = 1` records that the logical work
completed successfully without performing its normal external operation.
`skip_reason` contains a stable reason code such as `empty_fanout` or
`reused_prior_result`; it is required only for skipped rows.

Skipped rows still contain a valid typed logical `output_json` for downstream
steps. An empty fan-out uses `[]`. A reused result restores the prior logical
output. Skipped work therefore satisfies normal stage-completion checks and
does not require a separate current-placement or terminal-outcome table.

## `failed_work`

```sql
CREATE TABLE failed_work (
    attempt_id   TEXT PRIMARY KEY
        REFERENCES work_item_attempts(attempt_id),
    error_json   TEXT NOT NULL CHECK (json_valid(error_json)),
    queued_at    TEXT NOT NULL,
    started_at   TEXT NOT NULL,
    finished_at  TEXT NOT NULL
);
```

`failed_work` durably records a failed or abandoned attempt. A retry also
returns the logical work item to `queued_work`; it does not remove prior failure
history.

## Invariants

- Attempt rows derive run, stage, and worker data from their parent records.
- Only `queued_work` and `running_work` represent current placement.
- `running_work.work_item_id` uniqueness prevents two attempts for one logical
  work item from running concurrently.
- Claiming work inserts its attempt and `running_work` row, then deletes its
  `queued_work` row in one transaction. It copies `queued_at` forward.
- Finishing work appends one attempt outcome and deletes its `running_work` row
  in one transaction, copying `queued_at` and `started_at`. A retry also inserts
  `queued_work` in that transaction.
- A skipped row in `completed_work` is a successful terminal outcome and counts
  toward stage completion.
- Retries reuse `work_item_id` and create a new `attempt_id`.
