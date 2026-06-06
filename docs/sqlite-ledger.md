# SQLite Attempt Ledger

This document sketches the first local SQLite persistence boundary.

The ledger records completed work attempts and the typed variable values that
made those attempts correct. It is not a replacement for the variable system.
Common IDs and fingerprints may appear as indexed convenience columns, but they
must mirror stored typed variables.

## Goal

The first database slice should support one question:

```text
Was this exact work attempt already completed with the same resolved variables?
```

An output filename alone is not enough. A future skip decision must compare the
current resolved variable snapshot against a prior successful attempt snapshot.

## Initial Tables

### schema_version

Tracks the local database schema version.

```sql
CREATE TABLE schema_version (
    version INTEGER NOT NULL
);
```

The first implementation can insert one row with version `1`.

### attempts

Stores one row per concrete execution attempt.

```sql
CREATE TABLE attempts (
    attempt_id TEXT PRIMARY KEY,
    workflow_instance_id TEXT NOT NULL,
    step_instance_id TEXT NOT NULL,
    work_item_id TEXT NOT NULL,
    work_item_fingerprint TEXT NOT NULL,
    input_fingerprint TEXT NOT NULL,
    output_fingerprint TEXT NOT NULL,
    code_version TEXT NOT NULL,
    status TEXT NOT NULL,
    started_at TEXT NOT NULL,
    completed_at TEXT
);
```

The ID and fingerprint columns are convenience columns for indexing and lookup.
Each value should also be present in `attempt_variables` as a typed runtime
variable.

Early valid statuses:

```text
completed
failed
```

### attempt_variables

Stores the resolved typed variable snapshot for an attempt.

```sql
CREATE TABLE attempt_variables (
    attempt_id TEXT NOT NULL,
    namespace TEXT NOT NULL,
    name TEXT NOT NULL,
    type TEXT NOT NULL,
    value_json TEXT NOT NULL,
    source TEXT NOT NULL,
    lifecycle TEXT NOT NULL,

    PRIMARY KEY (attempt_id, namespace, name),
    FOREIGN KEY (attempt_id) REFERENCES attempts(attempt_id)
);
```

`value_json` stores the resolved value in a JSON-compatible representation even
for scalar values. The `type` column preserves the variable type, such as
`string`, `int`, `bool`, `datetime`, `path`, `object`, or `list[T]`.

Expected early lifecycles:

```text
workflow
step
work_item
attempt
```

## Important Runtime Variables

The first persisted snapshots should include these runtime variables when they
are available:

```text
runtime.workflow_definition_id
runtime.workflow_instance_id
runtime.workflow_fingerprint
runtime.step_definition_id
runtime.step_instance_id
runtime.step_fingerprint
runtime.work_item_id
runtime.work_item_fingerprint
runtime.attempt_id
runtime.code_version
runtime.input_fingerprint
runtime.output_fingerprint
runtime.completed_at
```

## First Go Implementation Boundary

The first Go database package should only do three things:

1. Open or create a SQLite database.
2. Create the version 1 schema.
3. Insert one completed attempt with its variable snapshot.

It should not yet change controller queue behavior, worker behavior, or skip
execution. Those are later slices after the ledger contract is tested.
