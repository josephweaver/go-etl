# GO-ETL Database Tables — EC Reference

This document describes the current SQLite-backed persistence model for GO-ETL on the `concept/dependency-aware-workflows` branch.

Important mental model:

```text
SQL ledger tables = durable execution/provenance facts
workflow_instances.submission_context_json = mutable submission context + dependency-aware workflow state
```

The dependency-aware entities (`WorkflowDependencyPlan`, `WorkflowDependencyStage`, `WorkflowDependencyStep`, and `WorkflowDependencyWorkItemMembership`) are not separate SQL tables yet. They are embedded as JSON inside `workflow_instances.submission_context_json`.

## Tables

### Table: completed_work

Stores terminal successful/skipped work attempt records.

| Column | Type | Nullable | PK | FK | Description | Lifecycle | Notes |
|---|---:|:---:|:---:|:---:|---|---|---|
| attempt_id | TEXT | No | Yes | work_item_attempts.attempt_id | Terminal attempt ID. | Created when a running attempt completes. | One completed row per completed attempt. |
| work_item_id | TEXT | No | No | work_items.work_item_id | Work item that completed. | Copied from running work attempt. | Used for run/stage completion counts. |
| skipped_parent_id | TEXT | Yes | No | completed_work.attempt_id | Prior completed attempt reused by skip logic. | Set only for reuse/skipped work. | Self-referential FK. |
| output_json | TEXT / JSON | No | No | No | Canonical logical worker output. | Written at completion. | Must be valid JSON. This is the durable attempt output ledger. |
| output_json_sha256 | TEXT | No | No | No | SHA-256 of canonical output JSON. | Written at completion. | Used for provenance/equality checks. |
| pre_state_sha256 | TEXT | No | No | No | Fingerprint before execution. | Written at completion. | Execution evidence. |
| post_state_sha256 | TEXT | No | No | No | Fingerprint after execution. | Written at completion. | Execution evidence. |
| queued_at | TEXT | No | No | No | Original queued timestamp. | Copied from queued/running state. | RFC3339-style string. |
| started_at | TEXT | No | No | No | Attempt start timestamp. | Copied from running state. |  |
| completed_at | TEXT | No | No | No | Completion timestamp. | Set by completion handler. |  |

### Table: failed_work

Stores terminal failed work attempt records.

| Column | Type | Nullable | PK | FK | Description | Lifecycle | Notes |
|---|---:|:---:|:---:|:---:|---|---|---|
| attempt_id | TEXT | No | Yes | work_item_attempts.attempt_id | Failed attempt ID. | Created when a running attempt fails. | One failed row per failed attempt. |
| work_item_id | TEXT | No | No | work_items.work_item_id | Work item that failed. | Copied from running work attempt. | Used for run/stage failed counts. |
| error | TEXT | No | No | No | Failure message. | Written by failure handler. | Human/operator diagnostic. |
| queued_at | TEXT | No | No | No | Original queued timestamp. | Copied from running state. |  |
| started_at | TEXT | No | No | No | Attempt start timestamp. | Copied from running state. |  |
| failed_at | TEXT | No | No | No | Failure timestamp. | Set by failure handler. |  |

### Table: projects

Stores admitted project source records.

| Column | Type | Nullable | PK | FK | Description | Lifecycle | Notes |
|---|---:|:---:|:---:|:---:|---|---|---|
| project_id | TEXT | No | Yes | No | Stable project identity. | Upserted during workflow admission. | Existing ID must match identical values. |
| project_name | TEXT | Yes | No | No | Project display name. | Written during admission. | May be empty/null depending source. |
| repository_identity | TEXT | No | No | No | Repository identity/provider source. | Written during admission. | Provenance anchor. |
| source_revision_id | TEXT | Yes | No | No | Immutable source revision. | Written during admission. | Commit/ref resolution evidence. |
| config_path | TEXT | No | No | No | Project config path in repository. | Written during admission. |  |
| source_object_id | TEXT | Yes | No | No | Provider object identifier. | Written during admission. | May be omitted. |
| config_sha256 | TEXT | No | No | No | Canonical project config hash. | Written during admission. | Provenance hash. |
| created_at | TEXT | No | No | No | Admission timestamp. | Written once. |  |

### Table: queued_work

Stores work items waiting to be claimed.

| Column | Type | Nullable | PK | FK | Description | Lifecycle | Notes |
|---|---:|:---:|:---:|:---:|---|---|---|
| work_item_id | TEXT | No | Yes | work_items.work_item_id | Queued work item. | Inserted when work becomes runnable; removed when claimed. | Represents queue membership. |
| queued_at | TEXT | No | No | No | Queue timestamp. | Set when enqueued. | Ordering input for claim. |

### Table: running_work

Stores currently claimed work.

| Column | Type | Nullable | PK | FK | Description | Lifecycle | Notes |
|---|---:|:---:|:---:|:---:|---|---|---|
| attempt_id | TEXT | No | Yes | work_item_attempts.attempt_id | Active attempt ID. | Inserted when worker/controller claims work; removed on terminal transition. |  |
| work_item_id | TEXT | No | No | work_items.work_item_id | Claimed work item. | Moved from queued_work. | Unique; one active attempt per work item. |
| worker_id | TEXT | Yes | No | workers.worker_id | Worker that claimed work. | Set at claim time. | Nullable for controller executor paths. |
| queued_at | TEXT | No | No | No | Original queue timestamp. | Copied from queued_work. |  |
| started_at | TEXT | No | No | No | Attempt start timestamp. | Set at claim time. |  |

### Table: work_item_attempts

Stores attempt metadata for work execution.

| Column | Type | Nullable | PK | FK | Description | Lifecycle | Notes |
|---|---:|:---:|:---:|:---:|---|---|---|
| attempt_id | TEXT | No | Yes | No | Attempt identity. | Created when work is claimed. | Parent for running/completed/failed attempt rows. |
| work_item_id | TEXT | No | No | work_items.work_item_id | Work item being attempted. | Set when attempt is created. |  |
| worker_id | TEXT | Yes | No | workers.worker_id | Worker identity. | Set when attempt is created. | Nullable for controller-executed attempts. |
| executor_type | TEXT | No | No | No | `worker` or `controller`. | Set when attempt is created. | Check-constrained. |
| started_at | TEXT | No | No | No | Attempt start timestamp. | Set when attempt is created. |  |

### Table: work_items

Stores concrete executable units.

| Column | Type | Nullable | PK | FK | Description | Lifecycle | Notes |
|---|---:|:---:|:---:|:---:|---|---|---|
| work_item_id | TEXT | No | Yes | No | Executable work unit ID. | Inserted when a stage is compiled. | Stable ID used by worker payload and dependency membership. |
| run_id | TEXT | No | No | workflow_stages.run_id | Owning workflow run. | Set at insertion. | Part of FK to workflow_stages. |
| stage_index | INTEGER | No | No | workflow_stages.stage_index | Owning stage. | Set at insertion. | Part of FK to workflow_stages. |
| work_item_index | INTEGER | No | No | No | Stable order within stage. | Set by compiler/admission. | Unique per `(run_id, stage_index, work_item_index)`. |
| worker_payload_json | TEXT / JSON | No | No | No | Full serialized `model.WorkItem` sent to worker. | Written at insertion. | Must be valid JSON. |
| resolved_inputs_sha256 | TEXT | No | No | No | Hash of resolved inputs/payload. | Written at insertion. | Used for provenance/reuse. |
| created_at | TEXT | No | No | No | Work item creation timestamp. | Written at insertion. |  |

### Table: workers

Stores worker registration/execution handles.

| Column | Type | Nullable | PK | FK | Description | Lifecycle | Notes |
|---|---:|:---:|:---:|:---:|---|---|---|
| worker_id | TEXT | No | Yes | No | Worker identity. | Created when worker is registered/started. |  |
| run_id | TEXT | Yes | No | workflow_instances.run_id | Associated workflow run (legacy launch context only). | Optional association. | Not used for queue ownership; scheduling is derived from `work_items.run_id`. |
| execution_handle | TEXT | Yes | No | No | Runtime/scheduler/container handle. | Written by worker start/registration path. | Backend-specific. |
| created_at | TEXT | No | No | No | Worker registration timestamp. | Written once. |  |

### Table: workflow_instances

Stores workflow run/submission records.

| Column | Type | Nullable | PK | FK | Description | Lifecycle | Notes |
|---|---:|:---:|:---:|:---:|---|---|---|
| run_id | TEXT | No | Yes | No | Workflow run / submission ID. | Created during workflow admission. | Client-facing `submission_id`. |
| project_id | TEXT | No | No | projects.project_id | Owning project. | Set at run creation. |  |
| workflow_id | TEXT | No | No | workflows.workflow_id | Owning workflow. | Set at run creation. |  |
| submission_context_json | TEXT / JSON | No | No | No | Submission context, admitted source metadata, run variables, and dependency-aware state. | Created during admission; updated when dependency plan changes. | Main mutable blob for dependency-aware workflows. Must be valid JSON. |
| created_at | TEXT | No | No | No | Run creation timestamp. | Written once. |  |

### Table: workflow_stages

Stores SQL-level stage ledger rows.

| Column | Type | Nullable | PK | FK | Description | Lifecycle | Notes |
|---|---:|:---:|:---:|:---:|---|---|---|
| run_id | TEXT | No | Yes* | workflow_instances.run_id | Owning workflow run. | Inserted during workflow admission. | Composite PK with `stage_index`. |
| stage_index | INTEGER | No | Yes* | No | Stage index. | Inserted during workflow admission. | Composite PK with `run_id`. |
| step_id | TEXT | No | No | No | Representative/logical step ID. | Inserted with stage plan. | Legacy naming: table is stage-based but carries step ID. |
| stage_source_reference | TEXT | No | No | No | Source reference for stage/workflow. | Inserted with stage plan. |  |
| state | TEXT | No | No | No | `ready`, `running`, `completed`, `failed`, `skipped`, or `blocked`. | Inserted with stage plan; updated by stage completion/failure paths. | SQL ledger state, separate from dependency JSON state. |
| created_at | TEXT | No | No | No | Stage creation timestamp. | Written at insertion. |  |
| ready_at | TEXT | Yes | No | No | Stage ready timestamp. | Written/updated when stage becomes ready. |  |
| started_at | TEXT | Yes | No | No | Stage start timestamp. | Written/updated when stage starts. |  |
| completed_at | TEXT | Yes | No | No | Stage completion timestamp. | Written when stage completes. |  |
| failed_at | TEXT | Yes | No | No | Stage failure timestamp. | Written when stage fails. |  |
| output_json | TEXT / JSON | Yes | No | No | Stage-level output JSON. | Written on SQL stage completion path. | Must be valid JSON if present. |
| output_json_sha256 | TEXT | Yes | No | No | Stage output hash. | Written with stage output. |  |

\* `run_id` and `stage_index` form a composite primary key.

### Table: workflows

Stores admitted workflow source records.

| Column | Type | Nullable | PK | FK | Description | Lifecycle | Notes |
|---|---:|:---:|:---:|:---:|---|---|---|
| workflow_id | TEXT | No | Yes | No | Stable workflow identity. | Upserted during workflow admission. | Existing ID must match identical values. |
| project_id | TEXT | No | No | projects.project_id | Owning project. | Set during admission. |  |
| workflow_name | TEXT | Yes | No | No | Workflow display name. | Written during admission. |  |
| repository_identity | TEXT | No | No | No | Source repository identity. | Written during admission. | Provenance anchor. |
| source_revision_id | TEXT | Yes | No | No | Immutable source revision. | Written during admission. |  |
| workflow_path | TEXT | No | No | No | Workflow file path. | Written during admission. |  |
| source_object_id | TEXT | Yes | No | No | Provider object identifier. | Written during admission. | May be omitted. |
| workflow_sha256 | TEXT | No | No | No | Canonical workflow JSON hash. | Written during admission. | Provenance hash. |
| created_at | TEXT | No | No | No | Admission timestamp. | Written once. |  |

## Embedded Dependency-Aware State

Stored inside:

```text
workflow_instances.submission_context_json.dependency_state
```

### Entity: WorkflowDependencyPlan

| Field | Type | Description | Lifecycle | Notes |
|---|---:|---|---|---|
| run_id | string | Owning workflow run. | Created after `workflow_instances` row exists. | Must match submission/run ID. |
| workflow_id | string | Owning workflow. | Created with plan. |  |
| state | string | `running`, `completed`, or `failed`. | Updated as stages become terminal. | Controller scheduling state. |
| stages | array | Dependency-aware stage list. | Created from normalized workflow stages. | Not SQL-normalized yet. |

### Entity: WorkflowDependencyStage

| Field | Type | Description | Lifecycle | Notes |
|---|---:|---|---|---|
| stage_index | int | Stage index. | Created during dependency plan initialization. | Unique within plan. |
| state | string | `blocked`, `ready`, `active`, `completed`, or `failed`. | Updated from step states. | Earliest stage starts ready; later stages start blocked. |
| parallel_with | string | Stage parallelism/dependency marker. | Copied from normalized workflow. |  |
| steps | array | Dependency-aware logical steps. | Created during plan initialization. |  |

### Entity: WorkflowDependencyStep

| Field | Type | Description | Lifecycle | Notes |
|---|---:|---|---|---|
| stage_index | int | Parent stage index. | Created during plan initialization. | Must match parent stage. |
| step_index | int | Global/logical step index. | Created during plan initialization. | Unique within plan. |
| step_id | string | Logical workflow step ID. | Created during plan initialization. | Required. |
| state | string | `blocked`, `ready`, `active`, `completed`, or `failed`. | Updated from work item memberships. |  |
| output_json | JSON string | Aggregated logical step output. | Written when all memberships are terminal and have outputs. | Pruned when workflow reaches terminal state. |
| output_json_sha256 | string | Hash of aggregated output. | Written with output. | Retained after pruning. |
| output_json_bytes | int | Size of output JSON. | Written with output / before pruning. | Retained after pruning. |
| output_json_pruned | bool | Whether output JSON was pruned. | Set when pruning occurs. | Controls blob growth. |
| work_items | array | Memberships linking concrete work items to this step. | Filled as stages are compiled. | Bridge to SQL `work_items`. |

### Entity: WorkflowDependencyWorkItemMembership

| Field | Type | Description | Lifecycle | Notes |
|---|---:|---|---|---|
| work_item_id | string | Concrete SQL work item ID. | Added when compiled work is persisted. | Must exist in `work_items` by intended lifecycle. |
| work_item_index | int | Stable order within step/stage. | Added with membership. | Used for deterministic aggregation. |
| state | string | `queued`, `running`, `completed`, `failed`, or `skipped`. | Updated by terminal-state/output handlers. | Dependency-level membership state. |
| output_json | JSON string | Canonical completed work output. | Written when completed work output is captured. | Pruned after step aggregation. |
| output_json_sha256 | string | Hash of work output. | Written with output. | Retained after pruning. |
| output_json_bytes | int | Size of output JSON. | Written with output / before pruning. | Retained after pruning. |
| output_json_pruned | bool | Whether work item output was pruned. | Set when pruning occurs. | Prevents `submission_context_json` growth. |

## Relationship Diagram

```text
projects.project_id
  └── workflows.project_id
        └── workflow_instances.workflow_id
              ├── workflow_stages(run_id, stage_index)
              │     └── work_items(run_id, stage_index)
              │           ├── queued_work.work_item_id
              │           ├── running_work.work_item_id
              │           ├── completed_work.work_item_id
              │           └── failed_work.work_item_id
              │
              ├── workers.run_id (legacy launch context only)
              │     └── work_item_attempts.worker_id
              │
              └── submission_context_json
                    └── dependency_state
                          └── stages[]
                                └── steps[]
                                      └── work_items[]
                                            └── work_item_id -> work_items.work_item_id
```

## System Tables

### Table: schema_version

| Column | Type | Nullable | PK | FK | Description | Lifecycle | Notes |
|---|---:|:---:|:---:|:---:|---|---|---|
| version | INTEGER | No | No | No | Supported schema version. | Inserted during schema initialization. | Expected to contain exactly one row. |

### Table: sqlite_stat1

SQLite-maintained query planner statistics table created/updated by `ANALYZE`.

### Table: sqlite_stat4

SQLite-maintained histogram statistics table when STAT4 support is available/enabled.

