# Attempt Liveness And Recovery Epic

Status: Proposed

## Purpose

Detect lost worker attempts through a heartbeat API and controller caretaker
loop, then safely return abandoned logical work items to runnable state without
allowing late reports to corrupt newer attempts.

## Goals

- Add an outbound worker heartbeat API for active attempts.
- Store renewable attempt leases in the workflow execution database.
- Run a controller-owned caretaker/reconciliation loop that scans questionable
  and expired leases.
- Optionally confirm execution state through a reachable worker or configured
  scheduler backend.
- Distinguish temporarily uncertain attempts from confirmed abandoned attempts.
- Make an abandoned work item eligible for a new attempt according to retry
  policy.
- Fence late heartbeats and terminal reports from superseded attempts.
- Expose liveness, lease, and recovery state through controller diagnostics.
- Keep external side-effect safety an explicit plugin/runtime responsibility.

## Non-Goals

- Defining workflow step dependencies or JIT compilation.
- Defining the complete workflow persistence schema.
- Guaranteeing exactly-once external side effects through leases alone.
- Requiring workers to accept inbound connections.
- Building a separate caretaker service in the first implementation.
- Defining scheduler-specific job cancellation for every backend.

## Architectural Context

Workers may run behind NAT, inside containers, or on HPC compute nodes that the
controller cannot contact directly. Therefore outbound worker heartbeats are
the authoritative liveness signal. Each heartbeat identifies at least:

```text
worker identity
workitem_id
attempt_id
lease/fencing token
```

The controller renews only the currently active matching attempt. A
controller-owned caretaker periodically scans persisted running attempts. When
the execution environment exposes scheduler or worker status, the caretaker
may use it as supporting evidence before declaring an expired attempt lost.

An expired lease enters an uncertain state for a configured grace period. Once
the abandonment policy is satisfied, the controller terminates that attempt as
abandoned and makes its logical work item runnable. A retry creates a new
`attempt_id` and fencing token. Reports carrying an older token cannot renew or
complete the newer attempt.

Leases limit duplicate execution but do not make external operations
exactly-once. Plugins should use atomic publication, idempotent operations, or
content-addressed destinations where duplicate execution would be unsafe.

## Relationship To Other Epics

- `workflow-execution-persistence` is a prerequisite and owns durable attempts,
  claims, and work-item state.
- `dependency-aware-workflows` consumes terminal attempt/work-item state but
  does not own liveness detection.
- `controller-resilience` owns controller-instance identity and broader
  abandoned-compute policy.
- Scheduler and transport implementations may provide optional backend probes
  without owning the recovery decision.

## Proposed Slices

No implementation slices are agreed yet. They will be drafted after heartbeat
identity, lease timing, caretaker ownership, backend probing, fencing, and
retry transitions are agreed.

## Open Questions

1. What heartbeat interval, lease duration, and expiry grace period are safe
   defaults, and which are configurable?
2. Does a restarted controller honor the prior lease until expiry or
   immediately mark active attempts uncertain?
3. Which scheduler state is sufficient to extend grace when heartbeats stop?
4. Is a heartbeat accepted only from the worker that claimed the attempt, and
   how is worker identity authenticated?
5. What retry limit and backoff apply after abandonment?
6. Can an abandoned attempt's late success ever be reconciled, or is it always
   rejected once a replacement attempt exists?
7. Which status fields expose healthy, uncertain, expired, abandoned, and
   retried attempts?

## Completion Criteria

- Active workers renew only their own matching attempt leases.
- Missing heartbeats become visible as uncertain or expired state.
- The caretaker applies the configured grace and optional backend confirmation
  policy deterministically.
- Confirmed abandoned attempts release their logical work item for retry.
- Retries receive new attempt identities and fencing tokens.
- Late reports cannot overwrite or complete a superseding attempt.
- Controller restart preserves lease and recovery state through the database.
- Existing healthy attempts continue without duplicate assignment.
- Recovery state is observable and relevant API, caretaker, persistence, and
  integration tests pass.
