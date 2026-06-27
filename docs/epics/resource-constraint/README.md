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
- Allow each named resource to have an arbitrary positive capacity.
- Allow the controller to enforce arbitrary positive concurrency capacity for
  that resource across workflow submissions.
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

Resource scope is expressed by the resource key rather than by a separate scope
field. A globally meaningful key can use a global namespace. A project-scoped
key can include the project identity, for example:

```text
${project_config.name}/python-env/<environment-fingerprint>
```

The controller tracks the work-item instance IDs holding each resource. Current
consumption is derived from the active holder list rather than maintained as an
independent integer. A holder consumes capacity for as long as its work-item
instance is running. Completion or failure removes that specific holder.

Constraint ordering follows the controller's pending-work FIFO order; resource
constraints do not maintain a separate waiter queue. Whether a blocked item at
the head of the queue prevents unrelated eligible work from being assigned
remains to be settled before implementation.

The current lifecycle assumption is that restarting the controller abandons
all compute started by that controller instance. Active resource holders are
therefore not restored after restart. Rejecting late reports from abandoned
compute requires a controller instance identity; that broader lifecycle is
deferred to the controller resilience epic.

## Proposed Slices

The slice sequence is not yet agreed. Candidate implementation areas are:

1. Define the work-item resource requirement and controller-owned holder state.
2. Gate work assignment on available resource capacity.
3. Release acquired capacity on work completion and failure.
4. Recover capacity from abandoned assignments.
5. Expose constraint state through controller diagnostics.

No numbered slice files should be created until the open questions below are
resolved and this epic is explicitly marked Ready.

## Open Questions

- Where is a resource's positive capacity declared, and how are conflicting
  declarations for the same key rejected?
- Does a work item consume one capacity unit, or may it request multiple units
  from the named resource?
- Does FIFO ordering block the entire pending queue when its first work item
  cannot acquire capacity, or may the controller assign later work that does
  not compete for that resource?
- Before a controller restart, does abandoned compute require active
  cancellation, or is ignoring reports bearing an old controller instance ID
  sufficient?
- Should status expose individual holders and waiters, aggregate counts, or
  both?

## Completion Criteria

- A work item can declare one named resource requirement.
- Each resource has an arbitrary positive capacity.
- The controller never assigns more concurrent holders than the configured
  capacity permits.
- Active consumption is attributable to work-item instance IDs.
- Independent workflow submissions requesting the same resource are
  coordinated by the controller.
- Capacity is released after normal completion and reported failure.
- Abandoned capacity has an agreed and tested recovery path.
- Constraint state is observable enough to diagnose waiting work.
- Existing unconstrained work continues to be assigned normally.
- The agreed implementation slices are complete and relevant tests pass.
