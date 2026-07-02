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
    workitem_id   TEXT PRIMARY KEY,
    run_id        TEXT NOT NULL REFERENCES workflow_instances(run_id),
    stage_index   INTEGER NOT NULL CHECK (stage_index >= 0),
    workitem_index INTEGER NOT NULL CHECK (workitem_index >= 0),
    workitem_json TEXT NOT NULL CHECK (json_valid(workitem_json)),
    created_at    TEXT NOT NULL,
    UNIQUE (run_id, stage_index, workitem_index)
);
```

The composite uniqueness constraint makes repeated stage compilation
idempotent. `workitem_json` contains the compiled worker input.

## Assumed Parent Tables

- `attempts(attempt_id, workitem_id)` stores immutable execution attempts.

## `queued_work`

One row means a logical work item is available for assignment.

```sql
CREATE TABLE queued_work (
    workitem_id TEXT PRIMARY KEY
        REFERENCES work_items(workitem_id),
    run_id      TEXT NOT NULL,
    stage_index INTEGER NOT NULL CHECK (stage_index >= 0),
    queued_at   TEXT NOT NULL
);
```

Assignment order is `queued_at`, then `workitem_id` for a stable tie-break.

## `running_work`

One row means one attempt currently owns a logical work item.

```sql
CREATE TABLE running_work (
    workitem_id TEXT PRIMARY KEY
        REFERENCES work_items(workitem_id),
    attempt_id  TEXT NOT NULL UNIQUE
        REFERENCES attempts(attempt_id),
    run_id      TEXT NOT NULL,
    stage_index INTEGER NOT NULL CHECK (stage_index >= 0),
    started_at  TEXT NOT NULL
);
```

## Invariants

- `run_id` and `stage_index` do not change between placement tables.
- A `workitem_id` occupies only one placement table after commit.
- Claiming work inserts its attempt and `running_work` row, then deletes its
  `queued_work` row in one transaction.
- Retries reuse `workitem_id` and create a new `attempt_id`.
