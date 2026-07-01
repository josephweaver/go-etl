# Database Design

SQLite schema notes for durable workflow execution.

## `projects`

```sql
CREATE TABLE projects (
    project_id TEXT PRIMARY KEY,
    repo_ref   TEXT NOT NULL,
    commit_sha TEXT NOT NULL,
    created_at TEXT NOT NULL
);

CREATE TABLE project_variables (
    project_id TEXT NOT NULL REFERENCES projects(project_id),
    name       TEXT NOT NULL,
    value_json TEXT NOT NULL CHECK (json_valid(value_json)),
    PRIMARY KEY (project_id, name)
);
```

`repo_ref` identifies the repository. `commit_sha` records the immutable,
resolved Git revision used by the submission.

`value_json` preserves the variable's JSON type.

## Workflow Definitions

```sql
CREATE TABLE workflows (
    project_id  TEXT NOT NULL REFERENCES projects(project_id),
    workflow_id TEXT NOT NULL,
    created_at  TEXT NOT NULL,
    PRIMARY KEY (project_id, workflow_id)
);

CREATE TABLE workflow_variables (
    project_id  TEXT NOT NULL,
    workflow_id TEXT NOT NULL,
    name        TEXT NOT NULL,
    value_json  TEXT NOT NULL CHECK (json_valid(value_json)),
    PRIMARY KEY (project_id, workflow_id, name),
    FOREIGN KEY (project_id, workflow_id)
        REFERENCES workflows(project_id, workflow_id)
);

CREATE TABLE workflow_steps (
    project_id  TEXT NOT NULL,
    workflow_id TEXT NOT NULL,
    step_index  INTEGER NOT NULL CHECK (step_index >= 0),
    step_json   TEXT NOT NULL CHECK (json_valid(step_json)),
    PRIMARY KEY (project_id, workflow_id, step_index),
    FOREIGN KEY (project_id, workflow_id)
        REFERENCES workflows(project_id, workflow_id)
);
```

`step_index` preserves client-submitted order. `step_json` is the canonical,
schema-versioned step definition.

## `workflow_instances`

One row represents one submitted workflow run.

```sql
CREATE TABLE workflow_instances (
    run_id                  TEXT PRIMARY KEY,
    project_id              TEXT NOT NULL,
    workflow_id             TEXT NOT NULL,
    project_snapshot_json   TEXT NOT NULL
        CHECK (json_valid(project_snapshot_json)),
    workflow_snapshot_json  TEXT NOT NULL
        CHECK (json_valid(workflow_snapshot_json)),
    submitted_at            TEXT NOT NULL,
    FOREIGN KEY (project_id, workflow_id)
        REFERENCES workflows(project_id, workflow_id)
);
```

Snapshots are immutable and include their document schema version. They keep a
run reproducible if the project or workflow definition later changes.

## Submission Transaction

In one transaction, upsert the project and workflow definition, insert their
variables and ordered steps, then insert the workflow instance. Acknowledge the
client with `run_id` only after commit succeeds.

## Assumed Parent Tables

- `work_items(workitem_id)` stores immutable logical work.
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
