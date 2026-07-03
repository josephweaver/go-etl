# Workflow Compilation Resolution Epic

Status: Proposed

## Purpose

Define how the controller compiles workflow work and finalizes assignment-time
work using short-lived, stateless variable resolvers while retaining the durable
source data, resolver recipes, resolved snapshots, and compilation outputs needed
for later stages, retries, audit, heterogeneous workers, and controller restart.

This epic owns the compilation-resolution boundary for submitted workflows,
later ready steps, and assignment-scoped finalization. It turns workflow
definitions, project/configuration scopes, submission overrides, generated
runtime values, predecessor outputs, step bindings, work-item bindings,
assignment bindings, worker capabilities, and approved secret references into
concrete compiled work and assignment artifacts.

The key rule is:

> Persist the inputs and outputs of resolution, not the resolver.

Case 2 workflow submission and Case 3 ready-step compilation should be
remarkably similar. They should share resolver recipe construction, scope
assembly, validation, fingerprinting, and compiled artifact publication wherever
their lifecycle inputs overlap.

Case 2.5 assignment-time resolution adapts a compiled logical work item to a
specific worker assignment. It may resolve assignment-scoped values such as
worker-local paths, worker capabilities, lease metadata, and approved secrets.
Plaintext secret material is never persisted in the workflow database.

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
  generated runtime records. Case 2.5 assignment-time resolution uses the
  compiled work item, current assignment/attempt identity, selected worker
  identity/capabilities, controller secret source, and approved secret
  references. Case 3 ready-step compilation uses the Case 2 run basis plus
  worker-produced step `output_json` records. Given the recorded source
  identities and runtime records for the relevant case, variable resolution must
  be deterministic within that lifecycle boundary.
- Persistence stores exactly the durable references and records needed to
  reconstruct each resolver case. For Case 2 this includes `workflow.json` and
  `project.json` source identity, including GitHub repository, commit SHA, and
  repository-relative path, plus runtime records. For Case 2.5 this includes the
  compiled work item, assignment/attempt identity, allowed secret references, and
  any durable worker identity or capability records required for assignment
  finalization; it does not include plaintext secrets. For Case 3 this
  additionally includes completed step `output_json` records. Client environment
  and client configuration may be added later, but are currently empty inputs.
- The minimum first implementation should compile and persist only the initially
  ready stage, while preserving the run basis needed for later Case 3
  compilations. On `submit workflow`, the controller receives project/workflow
  references, resolves them through the configured GitHub cache/source-of-truth,
  loads `project.json` and `workflow.json`, assembles the Case 2 resolver from
  persisted startup configuration plus project/workflow/runtime inputs, and
  compiles stage 0. Any unresolved non-secret logical variable in a compiled work
  item is an error. Once stage 0 work items are persisted, the resolver is
  discarded.
- A compiled work item is the persisted self-contained logical execution payload
  for a worker. It must contain concrete resolved non-secret inputs needed for
  execution and must not require the worker to reread controller, project, or
  workflow configuration. Secret-bearing inputs are represented as approved
  secret references or required-secret declarations, not plaintext values. The
  workflow instance remains the durable continuation record: it keeps the
  project/workflow GitHub repository, commit SHA, path, runtime records, and
  later step outputs needed to rebuild a resolver for stage 1 and later Case 3
  compilations.
- Case 2 does not persist extracted source expressions for future stages as a
  separate expression store. The workflow instance retains the authoritative
  workflow definition identity, including GitHub repository, commit SHA, and
  repository-relative path. Case 3 reconstructs future expressions from the
  authoritative workflow definition, optionally through a normalized workflow
  representation derived from that definition. The normalized representation is
  derived compiler state, not a second expression source of truth.
- Assignment-time secret access is assignment-scoped. A worker may request a
  declared secret through an endpoint such as
  `/assignments/{assignment_id}/secrets/{name}`. The controller must authorize
  that the requested secret name belongs to the assigned work item and that the
  assignment/attempt is current. Secret access closes when the assignment,
  attempt, or lease is no longer valid. Secret access metadata may be logged, but
  secret values must not be persisted, returned through status, or included in
  diagnostics.
- Case 2.5 should be broader than secrets. It is the lifecycle boundary that
  binds logical work to a concrete worker assignment. This matters for
  heterogeneous workers: assignment-time resolution may depend on selected worker
  capabilities, mounted paths, scratch directories, GPUs, local environment,
  lease/attempt identity, and other values unavailable at workflow compilation.
- Provenance is not a persisted primary artifact. The canonical documents and
  runtime records already contain enough information to reconstruct provenance:
  defaults, controller environment/configuration, worker environment/configuration,
  client environment/configuration, project configuration, workflow definition,
  generated runtime records, work items, assignments, and step outputs. When
  provenance is needed, the controller rebuilds the appropriate resolver from
  canonical inputs and computes provenance just in time. Future debug logging may
  record redacted provenance, but normal durable workflow state does not store a
  separate provenance document.
- Fingerprints are content-addressed semantic identities. GitHub repository,
  commit SHA, and path are source locators used to retrieve or audit known-valid
  copies of canonical documents; they are not the primary semantic identity of
  those documents. The project fingerprint is the canonical hash of
  `project.json`; the workflow fingerprint is the canonical hash of
  `workflow.json`. Many Git commits may contain the same valid file content; the
  content hash defines what was used, while the recorded commit/path identifies
  one known source location where that content was obtained.
- Work-item and result fingerprints compose semantic hashes. A work-item
  fingerprint should include the controller version, plugin version, project
  configuration hash, workflow configuration hash, resolved input-variable hash,
  declared or resolved output-variable hash where applicable, and the relevant
  pre-state hash for external file/system state. A completion/result fingerprint
  should include the work-item fingerprint plus output-variable hash and
  post-state hash. Controller and plugin versions may themselves be represented
  by Git commit SHAs or other immutable code-version identifiers.

## Goals

- Define a controller-owned workflow compilation boundary that uses
  `internal/variable.Resolver` as a create-use-discard evaluator.
- Treat Case 2 workflow submission and Case 3 ready-step compilation as two
  lifecycle events using the same compilation model with different available
  scopes.
- Define Case 2.5 assignment-time resolution for adapting compiled logical work
  to a specific worker assignment.
- Reuse code between Case 2, Case 2.5, and Case 3 wherever practical, especially
  for resolver recipe construction, scope assembly, required-variable access,
  fingerprint construction, and artifact publication.
- Retain immutable workflow-run context rather than retaining a long-running
  resolver or accumulating workflow scopes in global controller state.
- Persist enough canonical input data to reconstruct later compilation,
  assignment, and provenance calculations after predecessor completion, worker
  assignment, or controller restart.
- Compile only the work that is ready for the current lifecycle boundary.
- Resolve worker-facing non-secret logical parameters before publishing work
  items whenever the required values are known at compilation time.
- Preserve approved secret references during compilation and materialize
  plaintext secrets only through an assignment-scoped boundary.
- Support heterogeneous workers by allowing assignment-time values to depend on
  the selected worker, worker capabilities, worker-local paths, and assignment
  identity.
- Compute provenance just in time from canonical resolver inputs when diagnostics
  or debugging require it, without persisting provenance as normal durable state.
- Use content hashes, not source locations, as semantic fingerprints for project,
  workflow, work-item, and result identity.
- Keep variable expression evaluation inside the variable subsystem while
  keeping workflow lifecycle, assignment lifecycle, database transactions, and
  persistence inside the controller.
- Support dependency-aware workflow execution by giving it a clean operation to
  call when a stage becomes ready.
- Preserve workflow-instance, step-instance, work-item, attempt, assignment, and
  runtime identities through compiled artifacts and assignment metadata.

## Non-Goals

- Implementing cross-workflow dependency lookup from GitHub. That belongs to
  `workflow-dependency-resolution`.
- Defining dependencies between complete workflows.
- Letting workers evaluate workflow expressions independently.
- Making `variable.Resolver` aware of databases, workflows, controller state,
  step lifecycle, assignments, secrets, or workers.
- Designing arbitrary conditional branching, loops, or a general workflow
  programming language.
- Implementing resource-capacity admission control.
- Implementing heartbeat leases, caretaker recovery, or abandoned-attempt
  fencing.
- Defining a final database schema for all durable workflow execution state.
- Implementing every persistence and restart guarantee in the first slice.
- Designing plugin package management, Python environments, or worker artifact
  packaging except where compiled work must reference already-agreed artifacts.
- Defining a final secret-provider implementation beyond the assignment-scoped
  boundary and no-plaintext-persistence rule.
- Persisting full provenance records for every variable resolution event.

## Architectural Context

The current workflow submission path accepts a workflow and optional submitted
variables, builds temporary workflow and submission scopes, compiles the workflow
through `internal/workflow`, and appends every generated work item to the pending
queue. That creates two related problems:

1. Downstream steps can become assignable before predecessor steps complete.
2. The controller discards the definition and resolver context needed to compile
   later steps just in time.

The target model separates durable lifecycle state from ephemeral resolution. A
resolver is built for one named compilation or assignment event, resolves the
required values, produces validated outputs, and is discarded. The controller
persists or retains the source scopes, generated runtime scopes, resolved
snapshots, fingerprints, secret references, and compiled artifacts needed for
later decisions. Provenance is computed on demand from those canonical records,
not stored as a separate normal workflow artifact.

Conceptually:

```text
durable records / immutable cached definitions / assignment state
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
 resolve required values and optionally compute provenance
              |
              v
 validate and persist/return lifecycle artifact
              |
              v
        discard resolver
```

The variable package remains responsible for typed expression evaluation,
precedence lookup, recursion limits, cycle detection, interpolation, accessors,
and optional resolution-path/provenance calculation. The controller remains
responsible for deciding which scopes apply, when a lifecycle event occurs,
which values must be resolved, what transaction publishes compiled work or
assignment state, and which durable records are required for restart.

## Authoritative Resolver Reconstruction Inputs

Resolver reconstruction is case-specific. The controller must assemble the
required sources for the lifecycle case, construct a short-lived resolver,
resolve the required values, and discard the resolver.

| Case | Purpose | Authoritative inputs |
|---|---|---|
| Case 1 | Controller startup | `defaults.json`, controller environment, `controller.json`, and caller/client overrides. |
| Case 2 | Workflow submission compilation | Case 1 exportable inputs plus `workflow.json`, `project.json`, and generated runtime records. |
| Case 2.5 | Assignment-time resolution | Compiled work item, assignment/attempt identity, selected worker identity/capabilities, assignment runtime values, approved secret references, and controller secret source. |
| Case 3 | Ready-step compilation | Case 2 run basis plus completed worker-produced step `output_json` records. |

For Case 2, persistence must record the workflow and project source identities:
GitHub repository, resolved commit SHA, repository-relative path, and any
fingerprint/hash required by the persistence design. It must also record the
runtime records needed to recreate the run basis.

For Case 2.5, persistence must record the compiled work item and the assignment
or attempt identity. It may record durable worker identity/capability facts
needed to explain assignment decisions. It records secret references and required
secret names, not plaintext secret material. Plaintext secrets are materialized
from the controller secret source only for the current assignment boundary.

For Case 3, persistence additionally records the completed step output JSONs that
become typed read-only inputs to downstream resolution.

Client environment and client configuration may later join these inputs. They are
currently treated as empty sources until their capture and authorization model is
designed.

## Provenance Model

Provenance is a derived explanation, not the source of truth. The source of truth
is the canonical set of documents and runtime records used to build a resolver.
When a caller needs to explain a resolution decision, the controller reconstructs
the same lifecycle resolver from those canonical inputs and asks the resolver to
produce an explanation of how the value was selected.

Normal workflow persistence should not store a separate provenance document for
every consumed value. This avoids duplicating the workflow/configuration model,
reducing storage volume, and creating another artifact that must be versioned or
kept in sync with the authoritative records.

Future debug or audit modes may log redacted provenance for selected lifecycle
events. Such logs are diagnostic artifacts, not required inputs for replaying or
continuing a workflow. Secret values must never appear in provenance output; at
most, provenance may identify the redacted source reference that was used.

## Fingerprint Model

Fingerprints identify semantic content, not merely where that content was found.
Repository, commit SHA, and path are still recorded because they provide a known
valid source location for reload, restart, and audit, but they are locators rather
than the primary semantic identity.

The project fingerprint is the canonical hash of `project.json`. The workflow
fingerprint is the canonical hash of `workflow.json`. If the same canonical file
content appears in multiple Git commits, the fingerprint remains the same even
though the recorded locator may differ.

Work-item fingerprints compose the semantic inputs that define the logical unit
of work. They should include, as applicable:

- controller version;
- plugin version;
- project configuration hash;
- workflow configuration hash;
- resolved input-variable hash;
- declared or resolved output-variable hash;
- pre-state hash for plugin-defined external file/system state.

Completion or result fingerprints compose the work-item fingerprint with the
observed result:

- work-item fingerprint;
- output-variable hash;
- post-state hash for plugin-defined external file/system state.

Controller and plugin versions may be represented by Git commit SHAs or another
immutable code-version identifier. Unlike project/workflow source commit SHAs,
these code-version identifiers participate in semantic identity because changing
controller or plugin code may change compilation or execution behavior.

## Minimum First Implementation

The first implementation should prove the compilation boundary without requiring
the final database schema, full restart recovery, persistent provenance, or final
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
Each compiled work item must have all non-secret logical worker-facing variables
resolved. An unresolved non-secret variable in a compiled work item is a
submission/compilation error, and no partially resolved work should be published.
Secret-bearing values may remain as approved secret references or required-secret
entries for Case 2.5 assignment-time materialization.

After the stage 0 work items are inserted, the resolver is discarded. The work
item is now the persisted outcome of compilation and must be a self-contained
logical unit of execution. A worker executing that item should not need to read
`defaults.json`, `controller.json`, `project.json`, or `workflow.json`.

When a worker claims the work item, Case 2.5 constructs a fresh assignment-time
resolver from the work item, assignment/attempt identity, selected worker facts,
assignment runtime values, and approved secret references. It returns or exposes
only the assignment-scoped values the worker is authorized to receive. A worker
may request assigned secrets through an assignment-bound endpoint such as
`/assignments/{assignment_id}/secrets/{name}`.

The workflow instance still retains the source references and runtime context
needed for continuation. Later, when stage 1 or higher becomes ready, Case 3 uses
that workflow-instance run basis plus completed step `output_json` records to
rebuild a resolver and compile the next stage. Case 3 reloads or reconstructs the
workflow definition from the retained authoritative workflow identity rather than
from separately persisted extracted expressions.

This creates the intended split:

```text
compiled work item       = self-contained logical execution payload
assignment finalization  = worker-specific binding and secret materialization
workflow instance        = retained run basis for future compilation
workflow definition      = authoritative source for future expressions
fingerprints             = content-addressed semantic identities
source commit/path       = source locator for a known-valid copy
provenance               = reconstructed explanation, not durable state
resolver                 = discarded evaluation object
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
- resolved non-secret input snapshots and fingerprints for published work;
- approved secret references or required-secret declarations;
- optional in-memory or debug-only redacted provenance;
- idempotent markers preventing duplicate publication of the same initial stage.

### Case 2.5: Assignment-Time Resolution

Purpose: bind one compiled logical work item to one concrete worker assignment
and finalize values that were not known or should not be materialized at
workflow-compilation time.

Typical inputs:

- the compiled work item and resolved non-secret logical inputs;
- approved secret references or required-secret declarations from compilation;
- current assignment ID and attempt ID;
- selected worker identity, capabilities, and relevant worker-local facts;
- configured worker environment for the selected execution environment;
- controller secret source or secret-provider references;
- generated assignment runtime values such as claim time, lease identity, and
  assignment-scoped endpoint URLs.

Typical outputs:

- an assignment payload returned to the worker after claim;
- assignment-scoped resolved values such as localized paths, worker capability
  bindings, scratch locations, and lease metadata;
- assignment-scoped secret handles or values made available through an approved
  delivery mechanism;
- optional in-memory or debug-only redacted provenance and access metadata;
- no persisted plaintext secret material.

Workers may request assigned secrets through an assignment-scoped endpoint such
as `/assignments/{assignment_id}/secrets/{name}`. The controller must verify that
`name` is declared by the assigned work item, that the worker holds the current
assignment/attempt, and that the assignment or lease has not expired or reached a
terminal state.

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
- resolved input snapshots and fingerprints;
- approved secret references or required-secret declarations for newly compiled
  work;
- optional in-memory or debug-only redacted provenance;
- idempotent markers preventing the same ready stage from compiling twice.

Case 3 must not read mutable current workflow definitions, mutable project
configuration, a current Git branch, the current client process environment, or
unbounded controller runtime metrics to determine workflow semantics. It uses
the run's durable recipe inputs and captured values, adding only lifecycle inputs
that are legitimately newly available, such as predecessor outputs and generated
step/work-item identities.

## Shared Compilation Operation

Case 2 and Case 3 should be implemented as two callers of one shared conceptual
operation rather than as unrelated compilation paths. Case 2.5 should reuse the
same resolver construction and optional provenance mechanics where practical,
but its artifact is an assignment finalization rather than compiled workflow
work.

A candidate shape is:

```text
BuildLifecycleContext(event, run_basis, work_or_stage_basis, bindings)
        |
        v
ReconstructResolverRecipe(context)
        |
        v
BuildResolver(recipe)
        |
        v
ResolveLifecycleArtifact(context, resolver)
        |
        v
PersistOrReturnLifecycleResult(result)
```

Names are provisional. The important design point is that the shared operation
receives explicit lifecycle inputs and returns explicit lifecycle outputs. It
does not retain a resolver and does not hide workflow or assignment state inside
a controller-global mutable object.

## Relationship To Other Epics

- `dependency-aware-workflows` should consume this epic. It decides when a stage
  is dependency-ready and calls the compilation boundary to publish work for that
  stage.
- `workflow-execution-persistence` owns the durable database-backed run, step,
  work-item, attempt, output, configuration-snapshot, assignment, and restart
  state needed to make this model authoritative across restart.
- `attempt-liveness-recovery` owns worker heartbeat leases, fencing, abandoned
  attempts, and retry/requeue behavior after work has been published or assigned.
- `resource-constraint` remains an independent assignment gate after dependency
  readiness and compilation have produced assignable work.
- `workflow-dependency-resolution` should build cross-workflow invocation and
  GitHub workflow lookup on top of the workflow-instance lifecycle and typed
  output model supplied by dependency-aware execution and this compilation
  boundary.
- `sensitive-variable-propagation` must define how sensitive values, protected
  references, redaction, optional provenance calculation, persistence, and
  assignment-time secret materialization interact with resolved snapshots.
- `execution-observability` may expose compilation events, assignment-resolution
  events, resolver diagnostics, source identity, and readiness state without
  owning compilation or assignment state.

## Candidate Slices

No implementation slices are agreed yet. Candidate slices below are planning
ideas only and should be revised after the open questions are answered.

### 001 Document Compilation Lifecycle Boundaries

Clarify Case 2, Case 2.5, and Case 3 terminology, shared invariants, lifecycle
inputs, required outputs, and the create-use-discard resolver rule.

### 002 Define Resolver Recipe Reconstruction Contract

Define what authoritative state must exist to reconstruct a resolver recipe for
a compilation or assignment event. The recipe is a contract assembled by the
controller, not a persisted object.

### 003 Extract Shared Lifecycle Context Builder

Introduce an internal controller helper for assembling scopes and constructing a
resolver from explicit lifecycle inputs. The helper should not talk directly to
the database or mutate workflow/assignment state.

### 004 Compile Initial Stage Through Shared Boundary

Retool workflow submission so it uses the shared compilation boundary for the
initial ready stage instead of compiling all steps eagerly.

### 005 Compile Ready Stage Through Shared Boundary

Retool downstream activation so a newly ready stage uses the same compilation
boundary with added predecessor-output and step/work-item binding scopes.

### 006 Add Assignment-Time Resolution Boundary

Add Case 2.5 support for worker-specific assignment values, approved secret
references, and assignment-scoped secret access. Plaintext secrets must not be
persisted.

### 007 Add JIT Provenance Diagnostics

Teach resolver reconstruction to optionally compute redacted provenance from
canonical lifecycle inputs for debugging or diagnostics without adding normal
persistent provenance records.

### 008 Define Content-Addressed Fingerprints

Define and test project, workflow, work-item, input, output, and result
fingerprints as canonical content hashes. Source repository, commit SHA, and path
remain locators for known-valid copies rather than primary semantic identities.

### 009 Capture Resolved Input Snapshots

Persist or otherwise expose resolved values consumed by compiled work and
assignment finalization according to the agreed redaction and fingerprint policy.
Provenance remains reconstructable unless a later debug/audit mode explicitly
logs it.

### 010 Add Idempotent Compilation Markers

Define and test the marker that prevents the same stage from being compiled and
published twice after retry, duplicate completion handling, or controller
restart.

### 011 Align Existing Dependency-Aware Workflow Epic

Update `dependency-aware-workflows/README.md` so it delegates compilation details
to this epic and focuses on dependency graph normalization, readiness, step
state, outputs, and downstream activation.

## Open Questions

1. What exact provenance shape should the resolver compute just in time to
   explain precedence without leaking sensitive values?
2. Should compilation produce work items directly, or should it first produce an
   intermediate compiled-stage representation that persistence later publishes?
3. Does fan-out expansion belong inside the shared compilation operation, or is
   fan-out a separate compiler phase that consumes a resolver?
4. What is the exact typed output shape made available from predecessor steps?
5. What namespace exposes predecessor outputs to downstream expressions?
6. Are predecessor outputs read as `workflow.step[index]`, a generated runtime
   scope, a workflow read-only scope, or another typed scope?
7. What happens when a downstream expression references a future or unavailable
   step output?
8. Should empty fan-out create no work item and immediately complete the stage,
   or create a deterministic skipped/no-op work item?
9. Which stage-level or step-level values may be recomputed during Case 3, and
   which must come only from the Case 2 run snapshot?
10. How should protected sensitive values captured at Case 2 be materialized for
    Case 2.5 or Case 3 without persisting plaintext?
11. Should compilation be allowed to read current controller operational metrics,
    or must those remain outside workflow semantics unless explicitly captured?
12. What is the boundary between compilation, assignment resolution, and database
    transaction ownership?
13. Can the shared lifecycle context builder be pure/in-memory, with callers
    responsible for loading and persisting state?
14. How should compilation or assignment-resolution failures be represented:
    submission rejection, blocked run, failed stage, failed assignment, or
    retryable controller error?
15. Can a ready-stage compilation be retried safely after a crash before its
    transaction commits?
16. What idempotency key uniquely identifies an already-compiled stage?
17. How should compilation versioning and schema evolution be recorded so older
    runs remain explainable?
18. Which pieces of compiled work and assignment finalization are immutable after
    publication?
19. Can any compilation or assignment artifacts be cached, and if so what is the
    cache key and eviction policy?
20. How much of the existing `internal/workflow` compiler should be reused versus
    wrapped by a controller-side compilation boundary?
21. What is the eventual boundary for plugin-provided compile-time behavior, if
    plugins need to contribute declared outputs, package references, parameter
    schemas, or secret requirements?
22. How should this epic divide responsibilities with
    `workflow-execution-persistence` so neither document owns the same durable
    state twice?
23. Should `dependency-aware-workflows` be revised immediately after this epic is
    accepted, or after the first implementation slice proves the boundary?

## Completion Criteria

- The controller has an agreed workflow compilation-resolution boundary for
  Case 2 and Case 3.
- The controller has an agreed assignment-time resolution boundary for Case 2.5.
- The design explicitly states that resolvers are short-lived and never retained
  as workflow execution or assignment state.
- A resolver recipe is treated as a reconstructable controller contract, not a
  persisted object.
- Provenance is treated as a reconstructable explanation computed from canonical
  inputs, not as normal durable workflow state.
- Fingerprints are content-addressed semantic identities; GitHub source
  references are locators for known-valid copies.
- Project and workflow fingerprints are canonical JSON hashes.
- Work-item and result fingerprints compose controller/plugin versions,
  project/workflow hashes, input/output variable hashes, and pre/post external
  state hashes as applicable.
- The authoritative resolver reconstruction inputs are defined for Case 1, Case
  2, Case 2.5, and Case 3.
- The minimum first implementation publishes only initially ready work and keeps
  the workflow instance source references needed for later Case 3 compilation.
- Case 2 retains the authoritative workflow definition identity for future
  expressions instead of persisting extracted expression records.
- Case 2.5 supports assignment-scoped values for heterogeneous workers, including
  worker capabilities, worker-local paths, assignment identity, and approved
  secret references.
- Case 2 and Case 3 share the same conceptual resolver recipe and compilation
  path wherever their inputs overlap.
- Case 2.5 reuses resolver construction/provenance mechanics where practical
  while producing assignment finalization rather than compiled work.
- The design identifies which lifecycle-specific inputs distinguish submission
  compilation, assignment-time resolution, and ready-step compilation.
- The controller can retain or reconstruct the inputs required to compile later
  ready stages without using a global resolver.
- Initially ready work is compiled from a submission resolver context, not by
  eagerly compiling every downstream step.
- Downstream ready work can be compiled from the retained run context plus
  predecessor outputs and new lifecycle bindings.
- Compiled work contains concrete resolved non-secret parameters whenever values
  are known at compilation time.
- A compiled work item is self-contained for logical worker execution and does
  not depend on configuration files at worker runtime.
- The workflow instance retains project/workflow source references and runtime
  context for later ready-step compilation.
- Plaintext secrets are not persisted in workflow, work-item, assignment,
  provenance, diagnostics, status, or log records.
- Workers can receive or retrieve only secrets declared by their current assigned
  work item through an assignment-scoped authorization boundary.
- Resolved snapshots are captured according to the agreed redaction and
  persistence policy; provenance remains reconstructable unless a future debug or
  audit mode explicitly logs redacted provenance.
- Compilation publication is idempotent for a workflow instance and stage.
- Workflow-instance, step-instance, work-item, attempt, assignment, and runtime
  identities remain attributable in compiled artifacts and assignment metadata.
- Sensitive values are not leaked through diagnostics, provenance, fingerprints,
  status, logs, or persisted resolved snapshots.
- `dependency-aware-workflows` can delegate compilation details to this epic
  instead of owning resolver construction itself.
- Relevant controller, workflow, variable, persistence-boundary, assignment,
  secret-access, and integration tests pass for the agreed slice scope.
