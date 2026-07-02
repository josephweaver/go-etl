# Database Design

SQLite schema notes for durable workflow execution.

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

## Submission Transaction

In one transaction, upsert the project and workflow metadata, then insert the
workflow instance. Acknowledge the client with `run_id` only after commit
succeeds.

## `work_items`

One row stores one immutable compiled unit of work.

```sql
CREATE TABLE work_items (
    work_item_id   TEXT PRIMARY KEY,
    run_id         TEXT NOT NULL REFERENCES workflow_instances(run_id),
    stage_index    INTEGER NOT NULL CHECK (stage_index >= 0),
    work_item_index INTEGER NOT NULL CHECK (work_item_index >= 0),
    work_item_json  TEXT NOT NULL CHECK (json_valid(work_item_json)),
    created_at     TEXT NOT NULL,
    UNIQUE (run_id, stage_index, work_item_index)
);
```

The composite uniqueness constraint makes repeated stage compilation
idempotent. `work_item_json` contains the compiled worker input.

`stage_index` is the index of a logical block, commonly one step or a collection
of parallel steps. `work_item_index` is the ordinal within a fanout operation.

## `work_item_attempts`

```sql
CREATE TABLE work_item_attempts (
    attempt_id   TEXT PRIMARY KEY,
    work_item_id TEXT NOT NULL REFERENCES work_items(work_item_id),
    attempt_index INTEGER NOT NULL CHECK (attempt_index >= 0),
    worker_id    TEXT NOT NULL,
    created_at   TEXT NOT NULL,
    UNIQUE (work_item_id, attempt_index)
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
    run_id      TEXT NOT NULL,
    stage_index INTEGER NOT NULL CHECK (stage_index >= 0),
    queued_at   TEXT NOT NULL
);
```

Assignment order is `queued_at`, then `work_item_id` for a stable tie-break.

## `running_work`

One row means one attempt currently owns a logical work item.

```sql
CREATE TABLE running_work (
    work_item_id TEXT PRIMARY KEY
        REFERENCES work_items(work_item_id),
    attempt_id  TEXT NOT NULL UNIQUE
        REFERENCES work_item_attempts(attempt_id),
    run_id      TEXT NOT NULL,
    stage_index INTEGER NOT NULL CHECK (stage_index >= 0),
    started_at  TEXT NOT NULL
);
```

## Invariants

- `run_id` and `stage_index` do not change between placement tables.
- A `work_item_id` occupies only one placement table after commit.
- Claiming work inserts its attempt and `running_work` row, then deletes its
  `queued_work` row in one transaction.
- Retries reuse `work_item_id` and create a new `attempt_id`.
