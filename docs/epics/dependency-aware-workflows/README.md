# Dependency-Aware Workflow Execution Epic

Status: Proposed

## Purpose

Make workflow execution dependency-aware so the controller compiles and queues
only steps whose predecessors have completed successfully and whose outputs are
available.

The current controller eagerly compiles every step during workflow submission
and appends every generated work item to the same pending queue. Consequently,
a worker may receive work from a downstream step before an upstream step has
completed. The controller also does not retain the submitted workflow
definition and variable scopes needed to compile later steps just in time.

## Goals

- Treat workflow steps as sequential by default, with `parallel_with` as the
  explicit declaration that selected steps may run in the same dependency
  stage.
- Normalize step declarations into dependency edges and validate them before
  execution begins.
- Prevent downstream work from becoming assignable before all of its required
  predecessor steps have completed successfully.
- Retain each submitted workflow definition and its effective configuration
  scopes for the lifetime of the workflow instance.
- Compile initially runnable steps at submission time and compile newly ready
  steps when predecessor state changes.
- Treat a fan-out step as complete only after every work item belonging to that
  step reaches the required terminal state.
- Capture successful step outputs as typed values that downstream expressions
  can resolve through the variable system.
- Assemble a fresh resolver context at each compilation boundary from retained
  definition/configuration scopes, generated runtime variables, and completed
  predecessor outputs.
- Keep workflow-instance, step-instance, and work-item state distinct and
  attributable through their existing identities.
- Ensure worker scaling reacts when JIT compilation adds newly ready work.
- Define deterministic behavior for upstream failure, skipped/reused work, and
  controller restart.

## Non-Goals

- Implementing resource-capacity admission control; that remains in the
  resource-constraint epic.
- Moving workflow expression evaluation into workers.
- Allowing workers to decide which workflow steps are ready.
- Adding arbitrary conditional branching, loops, or a general workflow
  programming language in the first version.
- Implementing sub-workflow invocation unless it is explicitly added through
  later epic review.
- Redesigning scheduler-specific CPU, memory, GPU, or queue requests.
- Treating submission order alone as a safe substitute for tracked dependency
  state.

## Architectural Context

Dependency readiness is controller-owned orchestration state. Workers remain
simple: request one eligible work item, execute it, and report a terminal
result. A work item must enter the assignable pending queue only after its step
is ready.

The current submission flow in `cmd/controller/main.go` constructs temporary
workflow and submission scopes, creates a resolver, compiles every workflow
step, and discards the definition-level resolution context after the request.
The current completion flow records an attempt and removes the completed item
from the assigned map, but does not update step state or activate downstream
steps.

The target lifecycle is conceptually:

```text
submit workflow
      |
      v
validate dependency graph and retain workflow instance context
      |
      v
compile and queue dependency-ready steps only
      |
      v
workers complete every item in a step
      |
      v
capture typed step outputs and mark step complete
      |
      v
identify newly ready steps
      |
      v
assemble resolver context and compile those steps JIT
```

Workflow authoring remains sequential by default. A `parallel_with` tag is a
workflow-local group label, not a reference to a step name or position. A
contiguous run of steps carrying the same non-empty label forms one parallel
stage. An untagged step forms a sequential stage by itself. Each stage depends
on completion of every step in the preceding stage.

For example:

```text
step 1: parallel_with = "GroupA"
step 2: parallel_with = "GroupA"
step 3: untagged
step 4: parallel_with = "GroupB"
step 5: parallel_with = "GroupB"
```

normalizes to:

```text
[step 1, step 2] -> [step 3] -> [step 4, step 5]
```

Parallel groups must be contiguous and a closed label cannot be reopened. This
is invalid because `GroupA` ends before step 3 and is reused at step 4:

```text
step 1: parallel_with = "GroupA"
step 2: parallel_with = "GroupA"
step 3: untagged
step 4: parallel_with = "GroupA"
```

A step following a parallel group cannot become ready until every member of
that group has completed; waiting only for the textually last member would
recreate the same scheduling bug at a group boundary.

Submission normalizes this sequence into zero-based stages. An untagged step is
one stage; a contiguous `parallel_with` group is one stage containing multiple
steps. All stage definitions and indexes are recorded when the workflow run is
created. Only stage 0 is compiled initially. Successful completion of every
required work item in stage N activates JIT compilation of stage N+1.

If compiling a fan-out stage produces zero work items, the stage completes
successfully immediately with typed output `[]`. The controller then activates
the next stage through the same durable, idempotent stage-completion transition;
no synthetic work item or attempt is created for the empty fan-out.

Output aggregation preserves execution structure rather than flattening it. A
non-fan-out step produces one typed object using the variable module's existing
`TypeObject` and `ResolvedValue` tree. The object contains the step's named
logical outputs; no separate workflow-output value system is introduced. A
fan-out step produces an ordered list of its work-item outputs in deterministic
fan-out order. A parallel group produces an ordered list of its member-step
outputs in workflow-definition order. Consequently, a parallel group whose
members are fan-out steps produces a list of lists:

```text
parallel group output = [
  [fan-out step 1 item outputs],
  [fan-out step 2 item outputs]
]
```

No implicit flattening occurs. A downstream consumer that requires a flat list
must request an explicit transformation in a later expression capability.

Step outputs have stable implicit identities based on workflow-definition
position. The expression form is conceptually:

```text
workflow.step[0]
workflow.step[1]
```

`workflow.step` is a controller-generated, read-only generic list whose entries
appear in workflow-definition order. Each entry is the logical output of that
step: an object for a normal step or a list of objects for a fan-out step. The
controller exposes only outputs whose steps have completed successfully; an
unavailable current or future entry is a resolution error rather than an empty
placeholder.

Parallel group labels control concurrency and do not become a second output-
addressing scheme. The initial implementation uses explicit
`workflow.step[index]` references only; a convenience alias for the immediately
preceding stage is deferred to `FUTURE.md`.

For example, an SFTP download plugin may define its observable state as the
canonical combination of remote source metadata and local downloaded output
state. Before transfer it computes `pre_state_hash`. After successful transfer
it computes `post_state_hash` over the same state definition. On a later run,
matching the current pre-state hash to the prior successful post-state hash
proves, according to the plugin's declared observation guarantees, that the
downloaded state is already converged and the transfer may be skipped. A
changed remote file or missing/corrupt local output changes the observed state
and prevents the skip.

Reuse identity is composite. Its execution-relevant components include at
least:

```text
GOET Core source revision
selected GOET plugin source revision
project source revision
workflow definition ID within that project revision
step position within that workflow definition
bound work-item input fingerprint
```

The source revisions are immutable Git commit identities, not repository names
alone. Workflow and step definitions are cohesive project components checked
into the project repository under their IDs. The project commit is therefore
their version authority. A workflow ID such as `workflow-download-cdl` and a
position such as `step 3` identify components inside that immutable project
revision; they are not independent version sources. Normalized workflow and
step fingerprints may still be recorded for diagnostics and finer-grained
comparison, but correctness does not depend on locating separate workflow or
step repositories. A conservative first implementation may invalidate reuse
when any listed repository revision changes, even if the changed files did not
affect this step.

These components remain individually queryable typed variables in the attempt
record and also feed one deterministic composite execution fingerprint. The
worker uses that execution fingerprint to locate a prior successful candidate
and compares its current `pre_state_hash` to the candidate's
`post_state_hash`. Equality means the plugin-defined external state already
matches the prior successful final state.

Dependencies between complete workflows, including the `dependent_workflow`
tag and GitHub definition lookup, belong to the separate
`workflow-dependency-resolution` epic. This epic supplies the workflow-instance
state and readiness transitions that cross-workflow dependencies consume.

Retained workflow context must be isolated by workflow-instance ID. It must not
be implemented by accumulating workflow scopes in one global resolver. Each
JIT compilation should assemble the scopes belonging to that workflow
instance in deliberate precedence order.

The variable system remains the expression and precedence authority. A later
step should receive predecessor outputs through a defined read-only namespace
or equivalent typed scope rather than through an unrelated output lookup
mechanism. The controller should resolve downstream parameters before creating
worker assignments.

Workflow readiness and resource readiness are independent gates:

```text
dependency ready AND resource capacity available => assignable
```

A dependency-blocked item must not consume resource capacity. A resource-
blocked item must not cause a downstream step to be considered complete.

Any failed step fails its workflow instance and permanently prevents later
stages from becoming ready. Work items from the same parallel stage that were
already assigned may finish and report their terminal results, but their
completion cannot reactivate the failed workflow or trigger downstream
compilation. The first implementation does not require cancellation of those
already-running siblings.

When a completion transition makes a downstream stage ready, the controller
persists the newly compiled queued work and signals a reconciliation loop.
Completion handling does not directly launch worker processes. The reconciler
observes durable queued demand and applies the existing worker-scaling policy.

## Required State

The design is expected to require controller-owned records for at least:

- The workflow instance and retained submitted definition.
- Definition, project/configuration, submission override, and generated runtime
  variable scopes needed for later resolution.
- Each step's dependency set and lifecycle state.
- The work-item instances belonging to each compiled step instance.
- Terminal work-item results and the typed logical outputs derived from them,
  including ordered fan-out lists and nested parallel-group lists.
- Failure or skip decisions that affect downstream readiness.

Durable run/configuration snapshots, work-item and attempt identity, atomic
state transitions, restart reconstruction, and pre/post-state hashes are
owned by `workflow-execution-persistence`. Heartbeats, leases, fencing, and
abandoned-attempt recovery are owned by `attempt-liveness-recovery`. This epic
consumes those guarantees when deciding whether a stage has completed and when
the next stage may be compiled.

## Relationship To Other Epics

- `structured-variable-resolution` supplies recursive typed expressions,
  accessors, interpolation, and diagnostics used when compiling later steps.
- `resource-constraint` depends on this epic for the definition of workflow
  eligibility. Resource admission must never make a dependency-blocked step
  assignable.
- `controller-resilience` owns the broader controller-instance and abandoned-
  compute lifecycle. This epic must define which workflow state resilience
  would need to restore.
- `workflow-dependency-resolution` builds cross-workflow lookup and dependency
  behavior on top of the workflow-instance lifecycle defined here.
- `workflow-execution-persistence` is a prerequisite and owns database-backed
  run context, work-item/attempt records, outputs, and restart reconstruction.
- `attempt-liveness-recovery` owns heartbeat leases, caretaker reconciliation,
  fencing, and abandoned-attempt retry transitions.
- Attempt-ledger reuse must restore logical outputs before a reused or skipped
  predecessor can satisfy downstream dependencies.

## Proposed Slices

No implementation slices are agreed yet. Candidate implementation areas will
be decomposed after the remaining readiness and worker-scaling behavior is
resolved and prerequisite epic boundaries are reviewed.

## Open Questions

No unresolved epic-level behavior questions remain. Slice decomposition should
begin only after the prerequisite persistence and liveness boundaries are
reviewed.

## Completion Criteria

- Submission rejects invalid dependency graphs before queue mutation.
- Steps execute sequentially by default, while `parallel_with` steps may be
  assigned concurrently only when their shared predecessor requirements are
  satisfied.
- Submission rejects a `parallel_with` label that is reused after its
  contiguous group has closed.
- Only work items belonging to dependency-ready steps can enter the assignable
  pending queue.
- Two workers cannot execute dependent steps concurrently unless the declared
  dependency has already been satisfied.
- A fan-out predecessor is not complete until all of its required work items
  are terminal according to the agreed success policy.
- An empty fan-out completes successfully with typed output `[]` and advances
  without creating a work item or attempt.
- Fan-out output ordering follows deterministic fan-out order; parallel-group
  output ordering follows workflow-definition order, and nested lists remain
  nested.
- A non-fan-out step exposes one typed object represented by the variable
  module's existing resolved object model.
- Downstream steps compile JIT using the retained scopes and typed outputs of
  their own workflow instance.
- Workflow definitions, immutable resolver snapshots, step state, work items,
  and attempts are database-backed so a controller restart does not lose the
  run or cause later steps to use newer project configuration.
- Workflow configurations from different submissions never leak into one
  another.
- Upstream failure and skipped/reused work have explicit tested effects on
  downstream readiness.
- Any failed step fails the workflow and prevents compilation of all later
  stages; already-running parallel siblings may finish without changing that
  terminal workflow state.
- Newly compiled downstream work participates in worker scaling and normal
  assignment.
- Restart behavior matches the agreed persistence boundary and is not implied
  beyond what is implemented.
- Resource constraints can consume dependency readiness as a separate,
  authoritative eligibility gate.
- Relevant workflow, controller, variable, ledger, and integration tests pass.
