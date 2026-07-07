# Resource-Constrained Work Admission

Status: Complete

Implementation tracker:

| Slice | Status |
|---|---|
| `001-resource-constraint-model-and-schema.md` | Implemented |
| `002-resolve-resource-constraints-at-work-creation.md` | Implemented |
| `003-persist-constraints-with-work-items.md` | Implemented |
| `004-operator-evaluator-and-check-reader.md` | Implemented |
| `005-claim-next-resource-eligible-work.md` | Implemented |
| `006-resource-status-and-observability.md` | Implemented |
| `007-demo-fixtures-and-smoke-tests.md` | Implemented |
| `008-docs-project-state-and-cleanup.md` | Implemented |

Current State:

Slices 001-008 are now implemented. Resource-constrained admission is controller-owned and enforced during claim-time evaluation using persisted constraint facts plus Go operator evaluation.

## Purpose

Add controller-owned resource admission so a work item is assignable only when it is both dependency-ready and resource-admissible.

After this Strategic Concept is complete, GOET can safely express constraints such as:

```text
ctlr/python-env:torch: running + requested <= 1
target:local/memory-mib: running + requested <= 65536
target:local/gpu-count: running + requested <= 2
```

The same model supports all comparison operators:

```text
=, !=, <, >, <=, >=
```

The implementation deliberately keeps expression resolution and operator evaluation separate:

```text
workflow/controller expressions -> resolved persisted constraint facts -> SQL/view exposes candidate totals -> Go code evaluates the operator
```

## Strategic Decision

Resource readiness is a claim-time admission gate, not a dependency edge.

Dependency-aware workflow execution determines when work may enter the assignable queue. Resource-constrained work admission determines whether a queued work item may be claimed by a worker now.

```text
dependency ready AND resource predicate true => assignable
```

Workers must not evaluate resource constraints. Workers continue to ask for work, execute the assigned item, and report terminal completion or failure. The controller owns all resource admission decisions.

Resource constraint expressions must be resolved when the work item is compiled and persisted into queued/pending state. The `/work/next` claim path must not resolve `${...}` expressions. It should only read already-resolved persisted facts and apply deterministic Go comparison logic inside the claim transaction.

SQL should not encode the six comparison operators. SQL should expose candidate facts:

```text
resource_key
total_units
requested_units
operator
target_units
```

Go code then evaluates:

```text
candidate_total_units = total_units + requested_units
candidate_total_units <operator> target_units
```

This keeps the view simple, avoids a large SQL `CASE`/`OR` predicate for every operator, and gives the comparator a direct unit-test surface.

## Goals

- Persist resolved resource-admission predicates for work items.
- Support zero or more resource constraints per work item.
- Support all operators: `=`, `!=`, `<`, `>`, `<=`, `>=`.
- Resolve `resource_key`, `requested_units`, `operator`, and `target_units` before work enters `queued_work`.
- Store requested and target units as integers, not floats.
- Prefer normalized units such as `memory-mib` over ambiguous decimal `GB` values.
- Provide a SQL view that exposes one candidate-check row per queued work item resource constraint.
- Evaluate operator predicates in Go code, not inside the SQL view.
- Avoid head-of-line blocking: if the oldest queued work item is resource-blocked, the controller may assign the next queued eligible item.
- Keep work items with no resource constraints assignable under the existing dependency/queue rules.
- Preserve current worker contract: workers do not see or evaluate resource predicates unless later documentation explicitly permits exposing them as diagnostic metadata.
- Keep terminal completion/failure behavior unchanged: releasing a resource is the natural result of removing the attempt from `running_work`.
- Provide status/log visibility for resource-blocked work without exposing excessive scheduling internals.

## Non-Goals

- Worker-side resource checks.
- Worker-side expression evaluation.
- A distributed multi-controller scheduler.
- Preemption or cancellation of running work to satisfy a higher-priority item.
- Fair-share scheduling, priorities, reservations, quotas, or backfill optimization.
- Time-window rate limits such as requests per minute.
- Floating-point or decimal resource accounting.
- Automatic memory detection from the operating system.
- Inferring requested memory from Python code.
- Enforcing OS-level memory/GPU limits. This concept controls GOET admission; external runtimes may still need their own cgroup, Slurm, Docker, Singularity, or OS controls.
- Changing dependency-aware workflow semantics.

## Architectural Context

GOET already separates work-item identity/payload from queue and terminal state. `work_items` records the persisted work item, while `queued_work`, `running_work`, `completed_work`, and `failed_work` represent lifecycle state. Resource admission should fit beside those lifecycle facts instead of becoming a worker concern.

The dependency-aware workflow concept explicitly left resource-capacity admission as a later independent gate:

```text
resource readiness remains a later independent gate: dependency ready AND resource available => assignable
```

This concept implements that deferred gate.

## Target State

### Resource predicate model

Each work item may have zero or more resolved resource constraints:

```text
work_item_id
constraint_index
resource_key
requested_units
operator
target_units
created_at
```

The admission formula is:

```text
candidate_total_units = total_units + requested_units
candidate_total_units <operator> target_units
```

Where:

- `resource_key` is a stable string identifying the resource namespace.
- `total_units` is the sum of `requested_units` for currently running work items with the same `resource_key`.
- `requested_units` is the amount the candidate work item would add if admitted.
- `operator` is one of `=`, `!=`, `<`, `>`, `<=`, `>=`.
- `target_units` is the already-resolved comparison target.

A work item is resource-admissible only if all of its resource predicates evaluate true.

A work item with no resource constraints is resource-admissible by default.

### Naming guidance

Use resource keys that encode scope explicitly:

```text
ctlr/python-env:torch
run:<run_id>/python-env:torch
workflow:<workflow_id>/python-env:torch
target:<target_id>/memory-mib
target:<target_id>/gpu-count
provider:github-api/fetch
```

Recommended scope meanings:

| Prefix | Meaning |
|---|---|
| `ctlr/` | Shared by the controller process and its local caches/runtime state. |
| `run:<run_id>/` | Isolated to one workflow run/submission. |
| `workflow:<workflow_id>/` | Shared across runs of one workflow definition. |
| `target:<target_id>/` | Shared by a backend environment such as local, fake HPCC, or Slurm target. |
| `provider:<provider>/` | Shared by an external service/provider integration. |

### Example declarations

A mutex-like Python environment creation constraint:

```json
{
  "resource_constraints": [
    {
      "resource_key": "ctlr/python-env:torch",
      "requested_units": 1,
      "operator": "<=",
      "target_units": 1
    }
  ]
}
```

A memory capacity constraint using resolved integer units:

```json
{
  "resource_constraints": [
    {
      "resource_key": "target:local/memory-mib",
      "requested_units": "${step.memory_allocated_mib}",
      "operator": "<=",
      "target_units": "${controller_config.local_memory_limit_mib}"
    }
  ]
}
```

A combined constraint:

```json
{
  "resource_constraints": [
    {
      "resource_key": "target:local/memory-mib",
      "requested_units": 8192,
      "operator": "<=",
      "target_units": 65536
    },
    {
      "resource_key": "ctlr/python-env:torch",
      "requested_units": 1,
      "operator": "<=",
      "target_units": 1
    }
  ]
}
```

### View contract

The store should expose a view equivalent to:

```text
queued_resource_constraint_checks
  work_item_id
  queued_at
  constraint_index
  resource_key
  total_units
  requested_units
  operator
  target_units
```

The view should not decide whether the row passes. It only reports the current running total and the candidate predicate facts. Go code evaluates the predicate.

### Claim contract

The claim path should behave as follows:

```text
begin serialized claim section
begin database transaction
list queued candidates in deterministic order
for each candidate:
  read candidate resource-check rows from view
  if every predicate passes:
    insert work_item_attempts row
    insert running_work row
    delete queued_work row
    commit
    return claimed work
commit/rollback empty claim
return no work available
end serialized claim section
```

The implementation must not claim a resource-blocked work item and must not block the whole queue behind it when a later item is eligible.

### Operator semantics

The comparator must use integer arithmetic:

```text
candidate_total_units = total_units + requested_units
```

Then apply:

| Operator | Predicate |
|---|---|
| `=` | `candidate_total_units == target_units` |
| `!=` | `candidate_total_units != target_units` |
| `<` | `candidate_total_units < target_units` |
| `>` | `candidate_total_units > target_units` |
| `<=` | `candidate_total_units <= target_units` |
| `>=` | `candidate_total_units >= target_units` |

The first practical capacity constraints should mostly use `<=`. The other operators are supported for policy gates and future modeling needs, but they can easily create surprising scheduling behavior. Tests must cover all operators anyway.

## State Model

New persisted facts:

```text
WorkItemResourceConstraint
  work_item_id
  constraint_index
  resource_key
  requested_units
  operator
  target_units
  created_at
```

New derived/read model:

```text
QueuedResourceConstraintCheck
  work_item_id
  queued_at
  constraint_index
  resource_key
  total_units
  requested_units
  operator
  target_units
```

Suggested SQLite table:

```sql
CREATE TABLE work_item_resource_constraints (
    work_item_id TEXT NOT NULL,
    constraint_index INTEGER NOT NULL,
    resource_key TEXT NOT NULL,
    requested_units INTEGER NOT NULL,
    operator TEXT NOT NULL CHECK (operator IN ('=', '!=', '<', '>', '<=', '>=')),
    target_units INTEGER NOT NULL,
    created_at TEXT NOT NULL,

    PRIMARY KEY (work_item_id, constraint_index),
    UNIQUE (work_item_id, resource_key),
    FOREIGN KEY (work_item_id) REFERENCES work_items(work_item_id),

    CHECK (constraint_index >= 0),
    CHECK (resource_key <> ''),
    CHECK (requested_units > 0),
    CHECK (target_units >= 0)
);
```

Suggested view shape:

```sql
CREATE VIEW queued_resource_constraint_checks AS
SELECT
    q.work_item_id,
    q.queued_at,
    c.constraint_index,
    c.resource_key,
    COALESCE((
        SELECT SUM(r.requested_units)
        FROM running_work rw
        JOIN work_item_resource_constraints r
            ON r.work_item_id = rw.work_item_id
        WHERE r.resource_key = c.resource_key
    ), 0) AS total_units,
    c.requested_units,
    c.operator,
    c.target_units
FROM queued_work q
JOIN work_item_resource_constraints c
    ON c.work_item_id = q.work_item_id;
```

The view is intentionally not an eligibility view. Eligibility is Go code.

## Failure And Edge Cases

- Unknown operator: reject at validation/persistence time.
- Empty `resource_key`: reject.
- `requested_units <= 0`: reject.
- `target_units < 0`: reject.
- Duplicate `resource_key` for one work item: reject.
- Overflow while computing `total_units + requested_units`: return a scheduler error and do not claim the candidate.
- Constraint resolution failure during workflow submission or stage activation: fail/reject the admission operation before queue mutation.
- Constraint resolution failure during just-in-time stage activation: fail the workflow stage/workflow using the same terminal failure style as downstream compile failure.
- Resource-blocked item: remain queued.
- No eligible queued item: return no work, not an error.
- Work item with no resource constraints: eligible under resource admission.

## Operational Slices

| Slice | File | Purpose |
|---|---|---|
| 001 | `001-resource-constraint-model-and-schema.md` | Add resolved constraint model, operator enum, schema, and view. |
| 002 | `002-resolve-resource-constraints-at-work-creation.md` | Add workflow/raw declaration support and resolve expressions before queueing. |
| 003 | `003-persist-constraints-with-work-items.md` | Persist constraints atomically beside work-item insertion/queueing and JIT activation. |
| 004 | `004-operator-evaluator-and-check-reader.md` | Add Go comparator for all operators and store reader for view rows. |
| 005 | `005-claim-next-resource-eligible-work.md` | Replace oldest-only claim with resource-aware candidate selection. |
| 006 | `006-resource-status-and-observability.md` | Surface eligible/resource-blocked counts and concise logs. |
| 007 | `007-demo-fixtures-and-smoke-tests.md` | Add representative mutex, memory, and operator smoke coverage. |
| 008 | `008-docs-project-state-and-cleanup.md` | Final docs, project state, and cleanup. |

## Implementation Notes

- Keep this concept under `docs/concepts/complete/resource-constrained-work-admission/`.
- Reuse the existing persistence store owner. Do not create a second controller-local resource scheduler state layer.
- Keep claim-time code small and heavily tested.
- Prefer table/view facts over storing mutable resource counters.
- Do not add a persistent `resource_usage` table unless a later concept requires leases, reservations, or multi-controller coordination.
- In the current single-controller model, use controller-level claim serialization around resource-aware claim evaluation. If GOET later supports multiple active controllers over one database, this must be revisited with database-native locking/leases.
