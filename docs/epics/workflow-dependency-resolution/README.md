# Workflow Dependency Resolution Epic

Status: Proposed

## Purpose

Allow a workflow to declare that it depends on another workflow, locate the
dependency's definition from a configured GitHub repository, and ensure the
dependent workflow does not execute until the required workflow instance has
completed successfully and exposed its agreed outputs.

This epic owns dependencies between complete workflows. Dependencies between
steps inside one workflow remain owned by the
`dependency-aware-workflows` epic.

## Goals

- Define a language-neutral `dependent_workflow` declaration in a workflow
  definition.
- Resolve a workflow dependency to a workflow definition stored in a GitHub
  repository.
- Invoke the resolved prerequisite workflow as controller-owned execution state
  associated with the dependent workflow instance.
- Allow a workflow to invoke multiple prerequisite workflows and require every
  prerequisite to complete successfully before the dependent workflow becomes
  runnable.
- Give each resolved definition an unambiguous repository, path, and version
  identity.
- Validate missing definitions, invalid references, dependency cycles, and
  incompatible definitions before dependent execution begins.
- Create or locate the required workflow instance according to an agreed
  invocation model.
- Prevent the dependent workflow from becoming runnable before the required
  workflow reaches the agreed successful terminal state.
- Expose dependency outputs as typed, read-only values that the dependent
  workflow can consume through the variable system.
- Preserve workflow-definition and workflow-instance identity through logs,
  status, attempts, and fingerprints.
- Define caching, refresh, authentication, and offline behavior for GitHub
  workflow lookup.
- Keep workflow dependency state isolated across submissions and controller
  instances according to the agreed persistence boundary.

## Non-Goals

- Defining dependencies between steps inside a single workflow.
- Using GitHub Actions as the GOET execution engine.
- Allowing workers to fetch or select workflow definitions independently.
- Treating an unversioned mutable GitHub branch as a stable execution
  fingerprint without recording the resolved commit.
- Implementing arbitrary package management for code or worker artifacts.
- Implementing resource-constraint admission control.
- Supporting every possible Git hosting provider in the first version.

## Architectural Context

Workflow dependency lookup and readiness are controller-owned orchestration
responsibilities. A workflow author declares a dependency; the controller
resolves its definition, records the exact source identity, validates the
workflow dependency graph, and creates execution state. Workers receive only
concrete work items from workflows that are ready.

The conceptual flow is:

```text
submit dependent workflow
          |
          v
resolve dependent_workflow reference from configured GitHub catalog
          |
          v
record repository + path + resolved commit identity
          |
          v
validate complete workflow dependency graph
          |
          v
run or locate prerequisite workflow instance
          |
          v
capture successful typed workflow outputs
          |
          v
activate dependent workflow instance
```

GitHub is initially a definition catalog, not runtime state storage. Durable
workflow-instance state, completion state, and outputs belong to the
controller's persistence model. A repository lookup can provide a definition,
but it cannot by itself prove that a particular prerequisite workflow instance
has completed.

A `dependent_workflow` declaration is an invocation request, not a query for an
independently submitted workflow instance. After resolving and validating the
referenced definition, GOET creates the prerequisite workflow instance and
associates it with the dependent workflow instance. The controller does not
search existing unrelated submissions by name, fingerprint, or current state
to satisfy the dependency implicitly. Deliberate reuse, if added, must be a
validated execution decision based on persisted fingerprints and restorable
typed outputs.

A workflow may declare multiple `dependent_workflow` entries. They use AND
semantics: the dependent workflow remains blocked until every prerequisite
workflow instance completes successfully. Prerequisites may execute in
parallel when their own dependency graphs permit it. Recursive definition
resolution produces a workflow dependency DAG that must be validated before
any workflow in the graph becomes assignable.

For reproducibility, the controller should record the immutable commit SHA
resolved from any author-facing branch, tag, or default-branch reference. The
workflow definition fingerprint should include normalized definition content
and source identity. Whether mutable references are permitted at submission is
an open policy decision.

Dependency outputs should enter the dependent workflow through a typed,
read-only scope owned by the variable system. The exact namespace and output
contract must be agreed with dependency-aware execution; a parallel lookup API
must not become a second variable-resolution mechanism.

## Relationship To Other Epics

- `dependency-aware-workflows` is a prerequisite. It supplies workflow-instance
  lifecycle, terminal state, typed outputs, and JIT readiness transitions.
- `structured-variable-resolution` supplies typed output access and contextual
  resolution.
- `controller-resilience` owns durable restoration of controller and workflow-
  instance state after restart.
- `execution-observability` should expose resolved source identity and
  dependency readiness without owning the state transition.
- `resource-constraint` remains an independent assignment gate after workflow
  and step dependencies are satisfied.

## Candidate Definition Shape

The public shape is not yet agreed. A candidate is:

```yaml
dependent_workflows:
  - name: prepare-environment
    repository: organization/workflows
    path: workflows/prepare-environment.json
    ref: main
```

The controller would resolve `ref` to a commit SHA and retain that immutable
identity with the workflow instance. Field names and whether repository/ref
defaults come from controller or project configuration remain open questions.

## Proposed Slices

No implementation slices are agreed yet. Slice decomposition begins only
after the reference identity, invocation model, output contract, authentication
boundary, and persistence requirements below are resolved.

## Open Questions

1. What fields identify a catalog entry: logical name, repository, path, Git
   ref, or some combination?
2. Must references be pinned to a commit SHA in submitted definitions, or may
   branches and tags be accepted if the controller records the resolved SHA?
3. Is there one configured workflow repository per controller/project, or may
   each dependency name a different repository?
4. Are private repositories required initially, and where are GitHub
   credentials configured and refreshed?
5. Does the controller use the GitHub API, a local Git checkout/cache, or an
   injected repository interface supporting both?
6. What is the cache key and refresh policy, and what happens when GitHub is
   unavailable but a previously resolved immutable definition is cached?
7. How are workflow-level typed outputs declared, persisted, and referenced by
   the dependent workflow?
8. What failure, cancellation, retry, or reuse state of the prerequisite
    satisfies or permanently blocks the dependent workflow?
9. Must workflow dependency instances and outputs survive controller restart
    in the first implementation?
10. How are dependency cycles detected when resolving definitions recursively
    across repository files?

## Completion Criteria

- A workflow can declare at least one dependency on another workflow.
- The controller resolves each dependency to an unambiguous workflow definition
  and immutable source identity.
- GOET creates and tracks the prerequisite workflow instance; unrelated
  independently submitted workflow instances cannot satisfy the dependency by
  coincidence.
- Multiple prerequisites may run concurrently, but the dependent workflow
  remains blocked until all have completed successfully.
- Missing definitions, invalid definitions, unknown references, and dependency
  cycles fail before dependent work becomes assignable.
- The dependent workflow cannot run before every required workflow dependency
  reaches the agreed successful terminal state.
- Dependency workflow configurations and outputs do not leak across unrelated
  workflow instances.
- Successful prerequisite outputs are persisted and exposed as typed,
  read-only inputs to dependent workflow resolution.
- Workflow and attempt metadata retain the dependency definition fingerprint,
  resolved commit, and relevant workflow-instance identities.
- GitHub authentication, caching, refresh, and unavailable-service behavior
  match the agreed policy and are tested through an injected boundary.
- Restart behavior matches the agreed persistence scope and is not implied
  beyond what is implemented.
- Intra-workflow step dependencies continue to use the single readiness model
  established by dependency-aware workflow execution.
- Relevant catalog, workflow, controller, variable, persistence, and
  integration tests pass.
