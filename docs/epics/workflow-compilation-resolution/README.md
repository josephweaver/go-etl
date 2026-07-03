# Workflow Compilation Resolution Epic

Status: Proposed

## Purpose

Define how the controller compiles workflow work using short-lived, stateless
variable resolvers while retaining the durable source data, resolver recipes,
resolved snapshots, and compilation outputs needed for later stages, retries,
audit, and controller restart.

This epic owns the compilation-resolution boundary for submitted workflows and
later ready steps. It turns workflow definitions, project/configuration scopes,
submission overrides, generated runtime values, predecessor outputs, step
bindings, and work-item bindings into concrete compiled work artifacts.

The key rule is:

> Persist the inputs and outputs of resolution, not the resolver.

Case 2 workflow submission and Case 3 ready-step compilation should be
remarkably similar. They should share resolver recipe construction, scope
assembly, validation, provenance capture, fingerprinting, and compiled artifact
publication wherever their lifecycle inputs overlap.

## Decisions

- The epic directory is `workflow-compilation-resolution`. The earlier
  `workflow-complilation-resolution` spelling was a typo and should not be used
  in new references.
- A resolver recipe is a reconstructable controller contract. It is not a
  persisted object, serialized document, standalone database entity, or
  long-lived runtime object. The controller reconstructs the recipe from
  authoritative state immediately before building a short-lived resolver.
  Persistence owns the durable records; workflow compilation owns recipe
  reconstruction; the variable subsystem owns expression evaluation.
- The authoritative inputs required to reconstruct a resolver are case-specific.
  Case 1 startup uses `defaults.json`, the controller environment,
  `controller.json`, and caller/client overrides. Case 2 workflow submission uses
  the Case 1 exportable inputs plus `workflow.json`, `project.json`, and
  generated runtime records. Case 3 ready-step compilation uses the Case 2 run
  basis plus worker-produced step `output_json` records. Given the recorded
  source identities and runtime records for the relevant case, variable
  resolution must be deterministic.
- Persistence stores exactly the durable references and records needed to
  reconstruct each resolver case. For Case 2 this includes `workflow.json` and
  `project.json` source identity, including GitHub repository, commit SHA, and
  repository-relative path, plus runtime records. For Case 3 this additionally
  includes completed step `output_json` records. Client environment and client
  configuration may be added later, but are currently empty inputs.
- The minimum first implementation should compile and persist only the initially
  ready stage, while preserving the run basis needed for later Case 3
  compilations. On `submit workflow`, the controller receives project/workflow
  references, resolves them through the configured GitHub cache/source-of-truth,
  loads `project.json` and `workflow.json`, assembles the Case 2 resolver from
  persisted startup configuration plus project/workflow/runtime inputs, and
  compiles stage 0. Any unresolved variable in a compiled work item is an error.
  Once stage 0 work items are persisted, the resolver is discarded.
- A compiled work item is the persisted self-contained execution payload for a
  worker. It must contain concrete resolved inputs needed for execution and must
  not require the worker to reread controller, project, or workflow
  configuration. The workflow instance remains the durable continuation record:
  it keeps the project/workflow GitHub repository, commit SHA, path, runtime
  records, and later step outputs needed to rebuild a resolver for stage 1 and
  later Case 3 compilations.

## Goals

- Define a controller-owned workflow compilation boundary that uses
  `internal/variable.Resolver` as a create-use-discard evaluator.
- Treat Case 2 workflow submission and Case 3 ready-step compilation as two
  lifecycle events using the same compilation model with different available
  scopes.
- Reuse code between Case 2 and Case 3 wherever practical, especially for
  resolver recipe construction, scope assembly, required-variable access,
  provenance capture, fingerprint construction, and compiled-work publication.
- Retain immutable workflow-run context rather than retaining a long-running
  resolver or accumulating workflow scopes in global controller state.
- Persist enough resolver input data to reconstruct later compilation decisions
  after predecessor completion or controller restart.
- Compile only the work that is ready for the current lifecycle boundary.
- Resolve worker-facing parameters before publishing work items whenever the
  required values are known at compilation time.
- Capture resolved input snapshots and provenance for compiled work so errors,
  status output, fingerprints, and audit can explain which inputs were used.
- Keep variable expression evaluation inside the variable subsystem while
  keeping workflow lifecycle, database transactions, and persistence inside the
  controller.
- Support dependency-aware workflow execution by giving it a clean operation to
  call when a stage becomes ready.
- Preserve workflow-instance, step-instance, work-item, and runtime identities
  through compiled artifacts and attempt metadata.

## Non-Goals

- Implementing cross-workflow dependency lookup from GitHub. That belongs to
  `workflow-dependency-resolution`.
- Defining dependencies between complete workflows.
- Letting workers evaluate workflow expressions independently.
- Making `variable.Resolver` aware of databases, workflows, controller state,
  step lifecycle, or workers.
- Designing arbitrary conditional branching, loops, or a general workflow
  programming language.
- Implementing resource-capacity admission control.
- Implementing heartbeat leases, caretaker recovery, or abandoned-attempt
  fencing.
- Defining a final database schema for all durable workflow execution state.
- Implementing every persistence and restart guarantee in the first slice.
- Designing plugin package management, Python environments, or worker artifact
  packaging except where compiled work must reference already-agreed artifacts.

## Architectural Context

The current workflow submission path accepts a workflow and optional submitted
variables, builds temporary workflow and submission scopes, compiles the workflow
through `internal/workflow`, and appends every generated work item to the pending
queue. That creates two related problems:

1. Downstream steps can become assignable before predecessor steps complete.
2. The controller discards the definition and resolver context needed to compile
   later steps just in time.

The target model separates durable lifecycle state from ephemeral resolution. A
resolver is built for one named compilation event, resolves the required values,
produces validated outputs, and is discarded. The controller persists or retains
the source scopes, generated runtime scopes, resolved snapshots, provenance,
fingerprints, and compiled artifacts needed for later decisions.

Conceptually:

```text
durable records / immutable cached definitions
              |
              v
       assemble lifecycle scopes
              |
              v
 add generated runtime values and event bindings
              |
              v
 reconstruct resolver recipe
              |
              v
        construct resolver
              |
              v
 resolve required values and capture provenance
              |
              v
 validate and persist compiled artifact
              |
              v
        discard resolver
```

The variable package remains responsible for typed expression evaluation,
precedence lookup, recursion limits, cycle detection, interpolation, and
accessors. The controller remains responsible for deciding which scopes apply,
when a lifecycle event occurs, which values must be resolved, what transaction
publishes the compiled work, and which durable records are required for restart.

## Authoritative Resolver Reconstruction Inputs

Resolver reconstruction is case-specific. The controller must assemble the
required sources for the lifecycle case, construct a short-lived resolver,
resolve the required values, and discard the resolver.

| Case | Purpose | Authoritative inputs |
|---|---|---|
| Case 1 | Controller startup | `defaults.json`, controller environment, `controller.json`, and caller/client overrides. |
| Case 2 | Workflow submission compilation | Case 1 exportable inputs plus `workflow.json`, `project.json`, and generated runtime records. |
| Case 3 | Ready-step compilation | Case 2 run basis plus completed worker-produced step `output_json` records. |

For Case 2, persistence must record the workflow and project source identities:
GitHub repository, resolved commit SHA, repository-relative path, and any
fingerprint/hash required by the persistence design. It must also record the
runtime records needed to recreate the run basis.

For Case 3, persistence additionally records the completed step output JSONs that
become typed read-only inputs to downstream resolution.

Client environment and client configuration may later join these inputs. They are
currently treated as empty sources until their capture and authorization model is
designed.

## Minimum First Implementation

The first implementation should prove the compilation boundary without requiring
the final database schema, full restart recovery, full provenance, or final
fingerprinting design.

When a caller submits a workflow, the API provides project and workflow
references. The controller resolves those references through the configured
GitHub repository cache/source-of-truth, loads `project.json` and
`workflow.json`, and records the source identities used for the run. The source
identity includes at least repository identity, resolved commit SHA, and
repository-relative path.

The controller then assembles the Case 2 resolver from:

- persisted controller startup configuration and allowed exportable startup
  inputs;
- `project.json`;
- `workflow.json`;
- submitted overrides;
- generated runtime records for the new workflow run.

Using that resolver, the controller compiles only stage 0 / step 0 ready work.
Each compiled work item must have all worker-facing variables resolved. An
unresolved variable in a compiled work item is a submission/compilation error,
and no partially resolved work should be published.

After the stage 0 work items are inserted, the resolver is discarded. The work
item is now the persisted outcome of compilation and must be a self-contained
unit of execution. A worker executing that item should not need to read
`defaults.json`, `controller.json`, `project.json`, or `workflow.json`.

The workflow instance still retains the source references and runtime context
needed for continuation. Later, when stage 1 or higher becomes ready, Case 3 uses
that workflow-instance run basis plus completed step `output_json` records to
rebuild a resolver and compile the next stage.

This creates the intended split:

```text
compiled work item = self-contained execution payload
workflow instance  = retained run basis for future compilation
resolver           = discarded evaluation object
```

## Lifecycle Model

### Case 2: Workflow Submission Compilation

Purpose: accept a submitted workflow run, snapshot its immutable configuration
and source context, and compile only the initially ready work.

Typical inputs:

- selected controller-exportable configuration subset;
- project definition and project configuration;
- workflow definition and workflow-local variables;
- captured client, controller, and configured worker environment values required
  by declared expressions;
- validated submission overrides;
- generated workflow-run runtime values;
- initial stage, step, and work-item bindings available at submission time.

Typical outputs:

- workflow-run identity and immutable run snapshot;
- authoritative state needed to reconstruct later resolver recipes;
- normalized stage/step plan;
- initially ready compiled work items;
- resolved input snapshots, provenance, and fingerprints for published work;
- idempotent markers preventing duplicate publication of the same initial stage.

### Case 3: Ready-Step Compilation

Purpose: after predecessor state changes, reconstruct the accepted run context,
add completed predecessor outputs and new step/work-item bindings, and compile
one newly ready stage.

Typical inputs:

- the existing workflow-run snapshot from Case 2;
- exact project/workflow definition references retained by the run;
- captured environment/configuration/override scopes from the run snapshot;
- generated workflow-run runtime values from the original run;
- completed predecessor outputs as typed read-only values;
- generated stage, step, work-item, and evaluation-time runtime values for this
  compilation event.

Typical outputs:

- new step-instance or stage-instance state;
- newly compiled immutable work items;
- ordered fan-out bindings when fan-out exists;
- resolved input snapshots, provenance, and fingerprints;
- idempotent markers preventing the same ready stage from compiling twice.

Case 3 must not read mutable current workflow definitions, mutable project
configuration, a current Git branch, the current client process environment, or
unbounded controller runtime metrics to determine workflow semantics. It uses
the run's durable recipe inputs and captured values, adding only lifecycle inputs
that are legitimately newly available, such as predecessor outputs and generated
step/work-item identities.

## Shared Compilation Operation

Case 2 and Case 3 should be implemented as two callers of one shared conceptual
operation rather than as unrelated compilation paths.

A candidate shape is:

```text
BuildCompilationContext(event, run_basis, stage_basis, bindings)
        |
        v
ReconstructResolverRecipe(context)
        |
        v
BuildResolver(recipe)
        |
        v
CompileReadyStage(context, resolver)
        |
        v
PersistCompilationResult(result)
```

Names are provisional. The important design point is that the shared operation
receives explicit lifecycle inputs and returns explicit compilation outputs. It
does not retain a resolver and does not hide workflow state inside a
controller-global mutable object.

## Relationship To Other Epics

- `dependency-aware-workflows` should consume this epic. It decides when a stage
  is dependency-ready and calls the compilation boundary to publish work for that
  stage.
- `workflow-execution-persistence` owns the durable database-backed run, step,
  work-item, attempt, output, configuration-snapshot, and restart state needed to
  make this model authoritative across restart.
- `attempt-liveness-recovery` owns worker heartbeat leases, fencing, abandoned
  attempts, and retry/requeue behavior after work has been published.
- `resource-constraint` remains an independent assignment gate after dependency
  readiness and compilation have produced assignable work.
- `workflow-dependency-resolution` should build cross-workflow invocation and
  GitHub workflow lookup on top of the workflow-instance lifecycle and typed
  output model supplied by dependency-aware execution and this compilation
  boundary.
- `sensitive-variable-propagation` must define how sensitive values, protected
  references, redaction, provenance, and persistence interact with resolved
  snapshots.
- `execution-observability` may expose compilation events, resolver diagnostics,
  source identity, and readiness state without owning compilation state.

## Candidate Slices

No implementation slices are agreed yet. Candidate slices below are planning
ideas only and should be revised after the open questions are answered.

### 001 Document Compilation Lifecycle Boundaries

Clarify Case 2 and Case 3 terminology, shared invariants, lifecycle inputs,
required outputs, and the create-use-discard resolver rule.

### 002 Define Resolver Recipe Reconstruction Contract

Define what authoritative state must exist to reconstruct a resolver recipe for
a compilation event. The recipe is a contract assembled by the controller, not a
persisted object.

### 003 Extract Shared Compilation Context Builder

Introduce an internal controller helper for assembling scopes and constructing a
resolver from explicit lifecycle inputs. The helper should not talk directly to
the database or mutate workflow state.

### 004 Compile Initial Stage Through Shared Boundary

Retool workflow submission so it uses the shared compilation boundary for the
initial ready stage instead of compiling all steps eagerly.

### 005 Compile Ready Stage Through Shared Boundary

Retool downstream activation so a newly ready stage uses the same compilation
boundary with added predecessor-output and step/work-item binding scopes.

### 006 Capture Resolved Input Snapshots and Provenance

Persist or otherwise expose resolved values consumed by compiled work, including
redacted provenance sufficient for debugging, status, audit, and fingerprint
explanation.

### 007 Add Idempotent Compilation Markers

Define and test the marker that prevents the same stage from being compiled and
published twice after retry, duplicate completion handling, or controller
restart.

### 008 Align Existing Dependency-Aware Workflow Epic

Update `dependency-aware-workflows/README.md` so it delegates compilation details
to this epic and focuses on dependency graph normalization, readiness, step
state, outputs, and downstream activation.

## Open Questions

1. Does Case 2 persist source expressions for every later stage, or does it
   retain the entire normalized workflow definition and reconstruct future
   expressions from that definition?
2. Should resolved provenance be stored for every consumed value, only for
   non-sensitive values, only in debug mode, or only in failure diagnostics?
3. What exact provenance shape is needed to explain precedence without leaking
   sensitive values?
4. Which resolved values participate in workflow, step, work-item, input,
   output, and code-version fingerprints?
5. Should compilation produce work items directly, or should it first produce an
   intermediate compiled-stage representation that persistence later publishes?
6. Does fan-out expansion belong inside the shared compilation operation, or is
   fan-out a separate compiler phase that consumes a resolver?
7. What is the exact typed output shape made available from predecessor steps?
8. What namespace exposes predecessor outputs to downstream expressions?
9. Are predecessor outputs read as `workflow.step[index]`, a generated runtime
   scope, a workflow read-only scope, or another typed scope?
10. What happens when a downstream expression references a future or unavailable
    step output?
11. Should empty fan-out create no work item and immediately complete the stage,
    or create a deterministic skipped/no-op work item?
12. Which stage-level or step-level values may be recomputed during Case 3, and
    which must come only from the Case 2 run snapshot?
13. How should protected sensitive values captured at Case 2 be materialized for
    Case 3 without persisting plaintext?
14. Should compilation be allowed to read current controller operational metrics,
    or must those remain outside workflow semantics unless explicitly captured?
15. What is the boundary between compilation and database transaction ownership?
16. Can the shared compilation context builder be pure/in-memory, with callers
    responsible for loading and persisting state?
17. How should compilation failures be represented: submission rejection, blocked
    run, failed stage, or retryable controller error?
18. Can a ready-stage compilation be retried safely after a crash before its
    transaction commits?
19. What idempotency key uniquely identifies an already-compiled stage?
20. How should compilation versioning and schema evolution be recorded so older
    runs remain explainable?
21. Which pieces of compiled work are immutable after publication?
22. Can any compilation artifacts be cached, and if so what is the cache key and
    eviction policy?
23. How much of the existing `internal/workflow` compiler should be reused versus
    wrapped by a controller-side compilation boundary?
24. What is the eventual boundary for plugin-provided compile-time behavior, if
    plugins need to contribute declared outputs, package references, or parameter
    schemas?
25. How should this epic divide responsibilities with
    `workflow-execution-persistence` so neither document owns the same durable
    state twice?
26. Should `dependency-aware-workflows` be revised immediately after this epic is
    accepted, or after the first implementation slice proves the boundary?

## Completion Criteria

- The controller has an agreed workflow compilation-resolution boundary for
  Case 2 and Case 3.
- The design explicitly states that resolvers are short-lived and never retained
  as workflow execution state.
- A resolver recipe is treated as a reconstructable controller contract, not a
  persisted object.
- The authoritative resolver reconstruction inputs are defined for Case 1, Case
  2, and Case 3.
- The minimum first implementation publishes only initially ready work and keeps
  the workflow instance source references needed for later Case 3 compilation.
- Case 2 and Case 3 share the same conceptual resolver recipe and compilation
  path wherever their inputs overlap.
- The design identifies which lifecycle-specific inputs distinguish submission
  compilation from ready-step compilation.
- The controller can retain or reconstruct the inputs required to compile later
  ready stages without using a global resolver.
- Initially ready work is compiled from a submission resolver context, not by
  eagerly compiling every downstream step.
- Downstream ready work can be compiled from the retained run context plus
  predecessor outputs and new lifecycle bindings.
- Compiled work contains concrete resolved parameters whenever values are known
  at compilation time.
- A compiled work item is self-contained for worker execution and does not depend
  on configuration files at worker runtime.
- The workflow instance retains project/workflow source references and runtime
  context for later ready-step compilation.
- Resolved snapshots and provenance are captured according to the agreed
  redaction and persistence policy.
- Compilation publication is idempotent for a workflow instance and stage.
- Workflow-instance, step-instance, work-item, and runtime identities remain
  attributable in compiled artifacts and attempts.
- Sensitive values are not leaked through diagnostics, provenance, fingerprints,
  status, logs, or persisted resolved snapshots.
- `dependency-aware-workflows` can delegate compilation details to this epic
  instead of owning resolver construction itself.
- Relevant controller, workflow, variable, persistence-boundary, and integration
  tests pass for the agreed slice scope.
