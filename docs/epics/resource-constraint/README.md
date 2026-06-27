# Resource Constraint Epic

Status: Proposed

## Purpose

Add controller-owned resource constraints so GOET can limit how many work items
may concurrently use the same named resource.

A resource constraint represents orchestration admission control, not a mutex in
worker process memory. The controller remains authoritative for deciding which
work item may acquire a constrained resource and when that resource becomes
available again.

The motivating case is Python environment creation. Two workflow submissions
may both need the same immutable environment, but only one should build and
publish it while the other waits and then reuses the result.

## Goals

- Allow a work item to declare that it requires a named resource.
- Allow the controller to enforce a concurrency capacity for that resource
  across workflow submissions.
- Keep constrained work pending until its required capacity is available.
- Associate granted capacity with the assigned work item that holds it.
- Release capacity when the work item completes or fails.
- Define recovery behavior for capacity held by a worker that stops reporting.
- Make constraint state observable through controller status or diagnostics.
- Keep the concept generic enough for environment builds, artifact publication,
  datasets, licenses, and other shared resources.

## Non-Goals

- Building or executing Python environments.
- Implementing a general distributed lock service for customer applications.
- Coordinating resource constraints across multiple independent controllers.
- Replacing scheduler-specific resource requests such as CPU, memory, or GPU
  allocation.
- Using worker-local process mutexes as orchestration state.
- Defining priority, fairness, or preemption policy beyond what is required to
  avoid permanently blocked work in the first implementation.
- Supporting a work item that atomically acquires multiple named constraints in
  the first version.

## Architectural Context

The resource constraint belongs to GOET Core because it affects assignment and
concurrency decisions across workflows. The controller owns the constraint
state, grants capacity before assigning work, and releases or recovers capacity
as work-item state changes. Workers only declare or consume the granted work;
they do not coordinate directly with one another.

This follows the controller-authoritative model in
`docs/ARCHITECTURE_OVERVIEW.md`. It is separate from scheduler resource
allocation and from the worker-plugin responsibilities described in
`docs/PLUGIN_CONTRACT.md`.

The initial motivating flow is:

```text
workflow submission A requests python-env/<fingerprint>
workflow submission B requests python-env/<fingerprint>
                         |
                         v
            controller grants one holder
                         |
                         v
              environment is published
                         |
                         v
          capacity is released for reuse
```

The resource key identifies the shared constraint. For environment creation,
the key should be derived from an immutable environment fingerprint rather than
from a mutable project alias such as `torch`.

## Proposed Slices

The slice sequence is not yet agreed. Candidate implementation areas are:

1. Define the work-item resource requirement and controller-owned constraint
   state.
2. Gate work assignment on available resource capacity.
3. Release acquired capacity on work completion and failure.
4. Recover capacity from abandoned assignments.
5. Expose constraint state through controller diagnostics.

No numbered slice files should be created until the open questions below are
resolved and this epic is explicitly marked Ready.

## Open Questions

- Is a resource key scoped to one controller, one project, or another explicit
  namespace?
- Does the first version support only exclusive capacity of one, or arbitrary
  positive capacities?
- How does the controller detect an abandoned assignment: worker heartbeat,
  assignment timeout, explicit cancellation, or another mechanism?
- Must active constraint state survive a controller restart?
- What ordering guarantee, if any, should waiting work receive?
- Should status expose individual holders and waiters, aggregate counts, or
  both?

## Completion Criteria

- A work item can declare one named resource requirement.
- The controller never assigns more concurrent holders than the configured
  capacity permits.
- Independent workflow submissions requesting the same resource are
  coordinated by the controller.
- Capacity is released after normal completion and reported failure.
- Abandoned capacity has an agreed and tested recovery path.
- Constraint state is observable enough to diagnose waiting work.
- Existing unconstrained work continues to be assigned normally.
- The agreed implementation slices are complete and relevant tests pass.
