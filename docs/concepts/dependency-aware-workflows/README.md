# Dependency-Aware Workflow Execution

Status: Ready

## Purpose

Make workflow execution dependency-aware so the controller only makes dependency-ready work assignable.

After this Strategic Concept is complete, GOET workflows execute in dependency stages instead of submitting every generated work item to the queue at once. Sequential workflow steps run sequentially by default. Contiguous steps with the same `parallel_with` label run in the same stage and may be assigned concurrently after their shared predecessor stage completes.

This concept assumes these previous Strategic Concepts have already been completed and merged:

- `submission-cli-status`: workflow submission returns a stable `submission_id`; `goet status`, `goet submit ... --wait`, and JSON output exist.
- `execution-observability`: controller-owned log observations exist; submission-scoped logs are readable through the controller and CLI.

Those assumptions let this concept focus on dependency readiness instead of creating another public run handle or another observation surface.

## Strategic Decision

Dependency readiness is controller-owned orchestration state.

Workers do not decide which workflow step is ready. Workers continue to request one assignable work item, execute it, and report a terminal result. A work item must enter the assignable queue only after the controller has proven that the step's predecessor stage completed successfully.

The first dependency-aware workflow model is stage-based, not a general graph language:

```text
workflow definition order + contiguous parallel_with groups => ordered dependency stages
```

This keeps the user-facing workflow format simple while fixing the current scheduling bug: downstream work cannot race ahead of upstream work merely because all steps were compiled during submission.

## Goals

- Treat workflow steps as sequential by default.
- Treat `parallel_with` as a workflow-local label that groups only a contiguous run of adjacent steps into one parallel stage.
- Reject non-contiguous reuse of a `parallel_with` label before any queue mutation.
- Normalize every submitted workflow into ordered, zero-based stages.
- Persist enough workflow, stage, step, and work-item state for the controller to evaluate readiness after terminal work results.
- Compile and queue only stage 0 during submission.
- Compile later stages just in time after the previous stage completes successfully.
- Treat a step as complete only after all required work items belonging to that step have reached a success-equivalent terminal state.
- Treat a stage as complete only after every step in that stage completes successfully.
- Capture typed logical step outputs so downstream expressions can resolve `workflow.step[index]` through the existing variable resolver.
- Preserve deterministic output order for fan-out steps by using the original fan-out item order, not completion order or work-item ID sorting.
- Fail the workflow instance when any step fails, and prevent later stages from being compiled after that failure.
- Surface dependency state through the already-existing submission status and observability surfaces.

## Non-Goals

- Arbitrary DAG declarations such as `depends_on`.
- Conditional branches, loops, dynamic step creation, or a general workflow programming language.
- Cross-workflow dependencies or the `dependent_workflow` tag.
- Resource-capacity admission control. Resource readiness remains a later independent gate: `dependency ready AND resource available => assignable`.
- Worker-side workflow expression evaluation.
- Worker-side dependency readiness decisions.
- Cancellation of already-running sibling work items in a failed parallel stage.
- New public identifiers separate from `submission_id` and the existing internal workflow/run IDs.
- A new observability system separate from the completed Execution Observability concept.
- Attempt liveness recovery, abandoned-attempt recovery, or lease fencing beyond what already exists in the repository when this concept begins.
- Attempt reuse or skip correctness beyond the existing terminal work-result contract. A skipped/reused item may satisfy a dependency only when it records the same logical output shape needed by downstream resolution.

## Architectural Context

GOET's controller owns orchestration, queueing, source admission, workflow compilation, and direct SQLite/database access. Workers interact through HTTP, execute assigned work, and report completion or failure.

Before this concept, workflow submission compiles every step immediately and queues every generated work item. That makes queue order the only accidental barrier between upstream and downstream work. Queue order is not a dependency model. With multiple workers, a downstream item can be assigned before the upstream stage has completed.

After this concept, the controller owns a workflow-instance state machine:

```text
submit workflow
  -> normalize workflow stages
  -> persist workflow/stage/step context
  -> compile and queue stage 0 only
  -> workers execute ready work
  -> terminal work result updates work-item and step state
  -> completed stage captures typed outputs
  -> next stage becomes ready
  -> controller compiles next stage just in time
  -> repeat until workflow completed or failed
```

This concept builds on the existing variable package. Downstream step parameters are still resolved by the controller before creating worker assignments. Predecessor outputs are exposed as a generated, read-only workflow variable rather than through a separate expression mechanism.

## Current State

This current state is the expected state after the previous two Strategic Concepts have landed.

- `goet submit` submits a workflow and returns a `submission_id`.
- `goet status <submission_id>` can read workflow/submission status from the controller.
- `goet submit ... --wait` can wait for a terminal submission state.
- `goet logs <submission_id>` can read controller-owned execution observations.
- Python work items can execute admitted source and return a canonical logical `output.json` through the worker completion path.
- The controller can compile a workflow into work items through `internal/workflow`.
- The workflow compiler currently treats the workflow as a flat list of steps and compiles all steps during submission.
- The current `workflow.Step` shape does not yet encode stage readiness or dependency grouping.
- The queue is assignable-work oriented; it does not represent future blocked steps as assignable work.

## Target State

Workflow submission creates a dependency-aware workflow instance.

The controller persists or otherwise records the normalized stage plan, step instance states, queued work-item membership, retained workflow/configuration context, and terminal outputs needed to compile later stages. Only dependency-ready work items enter the assignable queue.

A simple sequential workflow:

```text
step 0
step 1
step 2
```

normalizes to:

```text
stage 0: step 0
stage 1: step 1
stage 2: step 2
```

A workflow with contiguous `parallel_with` groups:

```text
step 0: parallel_with = "A"
step 1: parallel_with = "A"
step 2: untagged
step 3: parallel_with = "B"
step 4: parallel_with = "B"
```

normalizes to:

```text
stage 0: step 0, step 1
stage 1: step 2
stage 2: step 3, step 4
```

This is invalid because label `A` is reused after the first `A` group closed:

```text
step 0: parallel_with = "A"
step 1: parallel_with = "A"
step 2: untagged
step 3: parallel_with = "A"
```

A later stage becomes eligible only after every step in the previous stage has completed successfully. A step with fan-out is complete only after every generated fan-out work item has completed successfully or has been accepted as a success-equivalent skip by the existing terminal-result contract.

## Output Contract

The public generated output namespace is:

```text
workflow.step[index]
```

`workflow.step` is a controller-generated, read-only list in workflow-definition order. It is not authored by the workflow document.

Each list entry is the logical output of one workflow step:

- a non-fan-out step produces one object;
- a fan-out step produces a list of item-output objects in deterministic fan-out order;
- a future, failed, or unavailable step has no output and causes a resolution error when referenced.

`parallel_with` labels do not create output names. They only control concurrency. Downstream expressions should use explicit step indexes in the first implementation.

The controller must not flatten fan-out output lists implicitly. A downstream consumer that needs a flattened shape requires a later explicit transformation capability.

## State Model

The implementation may name these records differently, but it needs equivalent concepts:

```text
WorkflowInstance
  submission_id
  workflow_instance_id / run_id
  workflow_definition_id
  state: submitted | running | completed | failed
  retained submitted workflow document
  retained variable/configuration scopes needed for JIT compilation

WorkflowStageInstance
  submission_id
  stage_index
  state: blocked | ready | active | completed | failed
  step_indexes in workflow-definition order

WorkflowStepInstance
  submission_id
  stage_index
  step_index
  step_definition_id
  state: blocked | ready | active | completed | failed
  expected_work_item_count
  completed_work_item_count
  failed_work_item_count
  output value when completed

WorkflowWorkItemMembership
  submission_id
  stage_index
  step_index
  work_item_id
  work_item_index within the step/fan-out result
  terminal state and terminal evidence
```

The controller should transition this state idempotently. Replaying or retrying a terminal work-result handler must not double-complete a stage or double-queue the next stage.

## Failure Contract

Any failed work item fails its step. Any failed step fails its stage. Any failed stage fails the workflow instance/submission.

When a step fails inside a parallel stage, sibling work items that were already assigned may still finish and report terminal results. Their later completion must not reactivate the failed workflow or compile downstream stages. The first implementation does not need to cancel running siblings.

## Submission Status And Observability

The completed Submission CLI Status concept owns the public submission handle and status command. This concept should extend those existing surfaces, not replace them.

Status should be able to tell a user whether a submission is blocked, active, completed, or failed because of dependency state. It does not need a rich UI, but it should expose enough structured data for `goet status --json` to show the current stage and step states.

The completed Execution Observability concept owns log observations. This concept should emit observations for meaningful dependency transitions, such as:

```text
workflow stages normalized
stage 0 queued
stage N completed
stage N+1 activated
workflow failed because step X failed
```

## Relationship To Later Concepts

- `resource-constraint` consumes dependency readiness as an eligibility gate. It must not make dependency-blocked work assignable.
- Python environment management benefits from dependency-aware execution because environment setup failures can stop downstream stages deterministically.
- Python/R SDKs can show stage-aware status through the existing submission/status API.
- Cross-workflow dependencies can later build on the workflow-instance lifecycle introduced here.

## Proposed Slices

1. `001-normalize-workflow-stages.md` — add `parallel_with` to workflow steps and normalize definitions into ordered stages.
2. `002-compile-single-workflow-stage.md` — let the workflow compiler compile one normalized stage without compiling future stages.
3. `003-persist-workflow-stage-state.md` — add controller-owned workflow/stage/step/work-item dependency state records.
4. `004-stamp-work-items-with-step-instance-metadata.md` — attach workflow, stage, step, and item-order metadata to compiled work items before queue insertion.
5. `005-submit-only-initial-ready-stage.md` — make workflow submission persist the plan and queue only stage 0.
6. `006-record-terminal-work-item-state.md` — update dependency state when work completion or failure is reported.
7. `007-capture-typed-step-outputs.md` — convert successful terminal outputs into typed step outputs and expose `workflow.step[index]` scope construction.
8. `008-compile-next-ready-stage.md` — activate and JIT-compile the next stage after a predecessor stage completes.
9. `009-handle-empty-fanout-and-auto-advance.md` — complete zero-work-item steps/stages without synthetic attempts and continue activation.
10. `010-propagate-step-and-workflow-failure.md` — make failure transitions terminal and prevent downstream activation.
11. `011-surface-dependency-state-in-status-and-logs.md` — extend submission status and observations with stage/step dependency state.
12. `012-update-dependency-workflow-docs-and-smoke.md` — update docs and add repeatable smoke coverage for sequential and parallel-stage workflows.

## Completion Criteria

- Invalid non-contiguous `parallel_with` labels are rejected before queue mutation.
- Workflow steps execute sequentially by default.
- Adjacent steps with the same `parallel_with` label may become assignable concurrently after their predecessor stage completes.
- Only stage 0 work items are queued during initial submission.
- Later-stage work items are absent from the assignable queue until their predecessor stage completes successfully.
- A stage following a parallel group waits for every step in that parallel group, not only the textually last step.
- Fan-out predecessor steps are not complete until every generated fan-out work item reaches an accepted terminal success state.
- Empty fan-out steps complete successfully with output `[]` and advance the workflow without creating a synthetic work item or attempt.
- Downstream stage compilation uses retained workflow/configuration scopes and generated `workflow.step[index]` outputs from the same submission only.
- Fan-out output ordering follows fan-out generation order, not completion order.
- Workflow configuration, output state, and resolver scopes from one submission never leak into another submission.
- A work-item failure fails the owning step/stage/workflow and permanently prevents downstream stage compilation.
- Already-running sibling work items in a failed parallel stage can report terminal results without changing the failed workflow state.
- `goet status <submission_id>` reflects dependency-aware state.
- `goet logs <submission_id>` includes useful dependency transition observations when observability is enabled.
- Relevant workflow, controller, status, and smoke tests pass.
