# Database Design

SQLite schema notes for durable workflow execution.

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
