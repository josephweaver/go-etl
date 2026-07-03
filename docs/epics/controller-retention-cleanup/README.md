# Controller Retention and Cleanup Epic

Status: Proposed

## Purpose

Add a controller-owned retention and cleanup process that bounds local disk and
database growth without deleting Git objects, artifacts, temporary work,
execution logs, or database records still required by active work, restart,
retry, audit, output restoration, or reuse.

Cleanup policy is supplied through the serialized controller variable document
and resolved during controller startup. Cleanup execution is a scheduled
controller responsibility, separate from worker heartbeat caretaking.

## Goals

- Apply configured age, capacity, and free-space policies to controller-owned
  storage.
- Clean abandoned controller temporary directories without touching live
  packaging operations.
- Evict unpinned published artifacts while preserving artifacts required by
  queued/running work, retry, or agreed audit/reuse retention.
- Evict unpinned Git repositories/objects while preserving commits required by
  active workflow runs.
- Retain, archive, or delete controller/workflow/attempt log files according to
  explicit policy.
- Clean database operational history according to explicit retention without
  deleting current placement, active-run state, required lineage, typed outputs,
  fingerprints, or provenance.
- Coordinate filesystem cleanup with cache/package operations so cleanup cannot
  delete content while another controller goroutine is reading or publishing
  it.
- Make database cleanup transactional and restart-safe.
- Respect configured minimum free-space reserves and fail new materialization
  clearly when pinned content prevents recovery.
- Support deterministic oldest-first eviction for eligible content.
- Expose cleanup runs, reclaimed bytes/rows, pinned content, failures, and
  storage pressure through logs, metrics, and status.
- Keep cleanup idempotent so interruption or controller restart can safely
  repeat it.

## Non-Goals

- Deleting customer source repositories or GitHub content.
- Deleting worker-produced customer outputs outside controller-owned artifact
  storage.
- Treating cleanup as evidence that an active worker or attempt is dead.
- Replacing the attempt-liveness caretaker.
- Defining database schema migrations or backup policy.
- Building a general storage abstraction before another storage backend is
  required.
- Guaranteeing that every configured target size can be reached when active
  pins exceed capacity.
- Implementing cleanup before retention, pinning, and audit requirements are
  agreed.

## Architectural Context

The controller currently has or plans five cleanup domains:

```text
controller temp staging
published artifact cache
semi-persistent Git cache
controller-owned logs
workflow execution database
```

Each domain has different correctness and retention rules. One generic
recursive directory deletion is not acceptable.

### Shared cleanup lifecycle

Conceptually, each scheduled cleanup pass performs:

```text
capture cleanup run identity and policy snapshot
        |
        v
discover domain-specific candidates
        |
        v
exclude active pins and retention-protected content
        |
        v
order eligible candidates deterministically
        |
        v
delete/archive through the owning domain boundary
        |
        v
record reclaimed capacity, failures, and remaining pressure
```

Candidates must be revalidated immediately before mutation because active work
may change between discovery and deletion.

### Temporary staging

The initial startup catalog defines:

```text
controller_temp_path
controller_temp_cleanup_age_secs
```

Every temp operation should have an operation identity and ownership marker.
Cleanup may remove an old directory only when no live packaging/materialization
operation owns it. Partially published artifacts remain temp content and are
safe to remove after abandonment is established.

### Artifact cache

The initial startup catalog defines:

```text
controller_artifact_cache_path
controller_artifact_cache_max_size_mb
controller_artifact_cache_retention_secs
controller_storage_min_free_mb
```

Artifacts use content-addressed immutable identities. Queued/running work and
any retained retry/audit contract pin their referenced artifacts. Cleanup
evicts only unpinned artifacts, oldest eligible first. Publication uses temp
staging followed by atomic promotion, so cleanup never sees a partially
published artifact as valid.

### Git cache

The initial startup catalog defines:

```text
controller_git_cache_path
controller_git_cache_max_size_mb
controller_git_cache_retention_secs
controller_storage_min_free_mb
```

Active workflow runs pin the exact repository/commit objects needed by their
dependency closures. Startup reconstructs those pins from durable run records
before cleanup begins. Cleanup coordinates with clone, fetch, object-read,
repack, and pin operations on a per-repository basis.

Terminal runs release strong pins but retain weak repository/commit/path/hash
lineage according to database retention. Later access may reload an evicted
commit from GitHub.

### Logs

The execution-observability epic defines controller-owned filesystem log sinks
for controller, workflow/run, and attempt scopes. This epic owns retention and
cleanup, not log observation transport or formatting.

Log cleanup must distinguish active writable files from closed files and must
not remove logs still required by an agreed run/attempt audit period. Log
retention variables have not yet been agreed.

### Database

The database is authoritative for workflow runs, work placement, attempts,
typed outputs, and restart state. Database cleanup cannot delete:

- queued or running placement;
- active workflow/run resolver context;
- attempt/work-item lineage required by current work;
- outputs required by downstream stages;
- fingerprints and state hashes required by retained reuse policy;
- records required to explain retained artifacts or logs.

Completed, failed, abandoned, worker, heartbeat-related, and other operational
history may be eligible after its retention period and dependency checks.
Deletion order must respect foreign keys and occur transactionally in bounded
batches. Database compaction/vacuum is separate from logical row eligibility
and must not block normal controller operation without explicit policy.

### Scheduling and ownership

Cleanup should use its own controller-configured schedule. It is not coupled to
`caretaker_interval_schedule_secs`: caretaker scheduling protects live work,
while cleanup scheduling manages retained storage.

Only one cleanup pass for a domain may mutate that domain at a time. The first
implementation assumes one active controller; future controller exclusivity or
leader election must preserve this ownership rule.

## Relationship to Other Epics

- `controller-startup-resolution` supplies resolved cleanup paths, limits,
  retention periods, free-space policy, and schedules.
- `workflow-execution-persistence` defines which database records and outputs
  are authoritative and which compact provenance must survive hot-row cleanup.
- `execution-observability` owns log production, routing, and filesystem sink
  layout; this epic owns retention and deletion.
- `attempt-liveness-recovery` determines abandoned attempts/workers. Cleanup
  may consume those outcomes but does not make liveness decisions.
- Dependency packaging defines artifact identities and pin relationships used
  by artifact cleanup.
- The controller Git cache defines repository/commit pins and per-repository
  coordination consumed by Git cleanup.

## Proposed Slices

No implementation slices are agreed yet. Slice decomposition should begin only
after domain-specific retention, pinning, deletion ordering, schedule, and
observability decisions are complete and the epic is explicitly moved to
`Ready`.

## Open Questions

1. What controller variables configure cleanup scheduling globally or per
   domain?
2. What retention periods apply to controller, workflow/run, and attempt logs?
3. Which artifact references remain pinned after work reaches a terminal state,
   and for how long?
4. Does Git retention apply at repository, commit, packfile, or whole-cache
   granularity in the first implementation?
5. Which database records form the compact provenance that must never be
   removed while a run remains retained?
6. What retention periods apply to completed, failed, abandoned, and worker
   records?
7. Is database cleanup deletion-only initially, or does the first version also
   schedule SQLite checkpoint/vacuum operations?
8. How large may one database cleanup transaction/batch be before it must yield
   to normal work?
9. How are filesystem access time/last-use values recorded reliably enough for
   oldest-first eviction?
10. Should low-free-space cleanup run immediately outside the normal schedule,
    and what hysteresis prevents repeated cleanup thrashing?
11. What status/API surface exposes pins, eligible bytes/rows, last cleanup,
    next cleanup, and cleanup failures?

## Completion Criteria

- Every cleanup domain has an agreed controller-owned policy and schedule.
- Active temp operations, artifacts, Git commits, log files, and database state
  are protected by explicit pins or eligibility queries.
- Cleanup revalidates eligibility before mutation and is safe under concurrent
  controller activity.
- Interrupted cleanup can repeat without corrupting cache indexes, artifact
  publication, Git repositories, logs, or database state.
- Capacity and retention cleanup is deterministic and respects the configured
  minimum free-space reserve.
- Pinned content is never deleted merely to satisfy a target size.
- Database cleanup preserves active execution, restart, downstream output,
  audit, reuse, and lineage requirements.
- Cleanup failures do not silently delete additional content and are visible in
  logs, metrics, and status.
- Relevant filesystem, Git, artifact, logging, database, concurrency, restart,
  and integration tests pass.
