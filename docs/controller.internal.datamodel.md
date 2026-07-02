# Controller Internal Data Model

Status: Draft for design discussion

## Purpose

Define how the controller owns durable execution data while using
`internal/variable.Resolver` as a short-lived, stateless evaluation object.

The intended rule is:

> Persist the inputs and outputs of resolution, not the resolver.

A resolver is created for one specific decision, resolves the required values,
and is discarded. Controller configuration, immutable workflow and project
definitions, run snapshots, step outputs, and work-item records outlive it.

This document describes the target lifecycle and the gap from the current
implementation. It does not define a database schema or an implementation
slice.

## Terms

### Durable source data

Data that must survive the resolution call and, for execution state, a
controller restart. Examples include controller configuration, immutable
project and workflow definitions, submission overrides, run snapshots,
compiled work items, attempts, and completed step outputs.

### Resolver recipe

The ordered, immutable inputs needed to construct a resolver for one lifecycle
event. A recipe consists conceptually of:

- the resolution purpose and evaluation timestamp;
- the applicable typed variable scopes in precedence order;
- generated read-only runtime variables;
- the resolver configuration, including maximum depth;
- identifiers for the workflow run, step, work item, or attempt in context.

The recipe is controller-owned data. It is not necessarily one Go struct or one
database row. Durable records must contain enough information to reproduce it.

### Resolution snapshot

The resolved, typed values actually consumed by a decision or compiled
artifact, including provenance when available. A snapshot is an output of
resolution and may be persisted for restart, audit, fingerprinting, and reuse.

### Resolver

`variable.Resolver` is an ephemeral evaluator over a `variable.Set` and
`ResolverConfig`. It owns expression evaluation, precedence lookup, typed
access, recursion limits, cycle detection, interpolation, and accessors. It
does not own controller lifecycle or persistence.

"Stateless" here means that resolving a value does not change durable state or
affect a later resolution. The current resolver contains immutable request
context (`Set` and configuration), so it is not a global singleton or an empty
object, but it is already suitable for create-use-discard operation.

## Core Invariants

1. The controller never stores a long-running resolver as execution state.
2. Every resolver is built for one named lifecycle event and one identity
   boundary.
3. Input scopes are immutable snapshots for the workflow run. Later project,
   workflow, or deployment changes do not alter an existing run.
4. New lifecycle scopes are added by constructing a new resolver, not by
   mutating a prior resolver or scope.
5. The ordered namespace precedence in `internal/variable/namespace.go` remains
   the authority for unqualified lookup.
6. Generated `runtime` values are read-only and exist only at or below the
   lifecycle boundary where they become known.
7. Workers receive concrete resolved parameters whenever practical. They do
   not independently evaluate workflow expressions.
8. The database is the execution source of truth. Caches may accelerate reads
   of immutable definitions, but must not become a second queue or lifecycle
   authority.
9. A resolver error causes the surrounding controller operation or transaction
   to fail; partially resolved work is not published.
10. Persisted work items contain the concrete values needed for execution and
    enough lineage to identify the recipe that produced them.

## Data Ownership

| Object | Lifetime | Authority | Mutable? |
|---|---|---|---|
| Controller deployment config | Controller process/deployment | Config source | Reload by explicit policy only |
| Project definition/config | Content revision | Repository or definition store | Immutable per revision |
| Workflow definition | Content revision | Repository or definition store | Immutable per revision |
| Workflow run snapshot | Workflow run | Database | Immutable after submission |
| Step/stage state | Workflow run | Database | Transactional lifecycle transitions |
| Typed step outputs | Workflow run and retention period | Database/artifact store | Immutable after successful completion |
| Work item | Logical work lifetime | Database | Definition immutable; placement changes |
| Attempt | Attempt lifetime and retention period | Database | Append/transition under fencing rules |
| Definition cache | Controller process | Derived from immutable source | Replaceable and disposable |
| Resolver | One evaluation | In-memory recipe inputs | No mutation after construction |

The controller may keep persistent handles to the database, immutable config,
execution-environment components, and definition caches. Those objects supply
resolver inputs; they do not turn the resolver into long-running state.

## Resolver Construction Model

The target construction flow is:

```text
durable records / immutable caches
              |
              v
       load applicable scopes
              |
              v
 add event-specific bindings and runtime values
              |
              v
   build Set, then build Resolver
              |
              v
 resolve required values and capture provenance
              |
              v
 validate and persist the resulting artifact
              |
              v
        discard Resolver
```

Scope assembly should be an explicit controller responsibility. The variable
package should remain unaware of databases, workflows, steps, workers, and
controller transactions.

## Lifecycle Events

### 1. Controller startup

Purpose: turn a small externally supplied bootstrap configuration into a
validated set of long-lived controller services.

Startup has a strict dependency boundary: the controller must know how to find
and authenticate to its database before it can read anything from that
database. Database location, database authentication references, secret-source
configuration, and enough TLS configuration to establish the connection must
therefore come from outside the controller database.

#### Startup questions

At minimum, startup must answer:

- Which controller instance or deployment is this?
- Which database driver and endpoint should it use?
- Which database/schema belongs to this controller deployment?
- How are database credentials obtained?
- Which TLS trust roots, client certificate, or connection policy apply?
- Which schema versions can this controller read and migrate?
- Where should the HTTP control plane listen and advertise itself?
- Which execution environments and definition stores are available?
- What resolver limits, logging policy, retention policy, and reconciliation
  timing apply?

Not every answer is a plain variable. Configuration has two forms:

- **Structural configuration** selects and wires components, such as database
  driver, secret-provider type, transport, scheduler, or runtime.
- **Typed variables** supply values consumed by those components, such as a
  database path, host, port, timeout, controller URL, or secret reference.

The current `ControllerConfig` already has this split: `ExecutionEnvironment`
is structural while `Variables` are resolved values. The split is legitimate,
but it needs a declared rule so component settings do not become an
uncontrolled second variable system.

#### Bootstrap input sources

The startup resolver may assemble these scopes, from lower to higher
precedence:

1. Built-in non-secret defaults.
2. Global deployment configuration.
3. A captured allowlist of controller environment variables.
4. The selected controller configuration document.
5. Explicit process arguments or startup overrides.
6. Generated startup runtime values, such as controller instance ID and
   startup timestamp.

The controller should not import its entire process environment implicitly.
An allowlist makes the startup recipe inspectable and prevents unrelated host
variables from unexpectedly changing behavior.

The bootstrap document must be locally or externally available before the
database opens. It may be supplied as a file, command-line-selected profile,
container secret mount, or deployment service. It must be sufficiently small
to audit and must not depend on workflow/project state.

#### Database configuration

Database configuration should describe connection intent without requiring a
single opaque connection string. The initial SQLite case needs only a driver
and path. A network database may need:

- driver;
- host and port;
- database or schema name;
- connect and query timeouts;
- TLS mode and trust material references;
- username reference and password/token reference;
- pool limits;
- migration policy.

Individual typed values improve validation and redaction. A derived connection
string may be built immediately before opening the database, but it should not
become the persisted configuration authority.

Database configuration has three sensitivity classes:

| Class | Examples | May persist in controller config? |
|---|---|---|
| Public operational | driver, host, port, database name, SQLite path | Yes |
| Sensitive reference | secret URI, credential name, certificate path | Yes, with access controls |
| Secret material | password, token, private key contents | No |

#### Secret handling

Typed configuration should contain secret references, not secret values. For
example, a database password variable may identify a secret by name or URI;
the secret provider materializes its value only when the database component is
constructed.

```text
controller_config.database_password_ref = "env://GOET_DB_PASSWORD"
```

The exact reference syntax and supported providers remain design decisions.
Likely early providers are an explicitly named environment variable and a
mounted file. OS keychains or external secret managers can be added behind the
same provider boundary when required.

Secret material follows stricter rules than ordinary resolved values:

- never store it in workflow-run resolver snapshots;
- never include it in fingerprints or provenance payloads;
- never return it through status or diagnostics;
- never format it into an error message or log record;
- avoid placing it in a general `variable.Set` if a narrower secret-material
  argument can be used;
- retain it in memory only as long as its consuming component requires;
- preserve the secret reference and provider identity for audit, not the
  materialized value.

This creates an intentional boundary: the ordinary variable resolver may
resolve *which secret reference wins*, while a secret provider resolves that
reference into sensitive bytes. Secret lookup is an I/O operation and should
not be hidden inside `variable.Resolver.Resolve`, which is currently a
deterministic in-memory evaluation operation.

For database clients that internally retain credentials or tokens, the
controller cannot guarantee immediate memory destruction. It can still avoid
duplicating the secret, constrain its scope, and delegate rotation/reconnect
behavior to an explicit database credential component.

#### Phased startup lifecycle

```text
1. Locate bootstrap source
2. Parse and definition-validate structural config and typed expressions
3. Capture allowlisted controller environment
4. Build startup resolver from non-secret scopes
5. Resolve and validate database connection intent and secret references
6. Materialize required secrets through the selected secret provider
7. Open database and verify connectivity
8. Read schema version and apply allowed migrations
9. Load database-backed controller metadata, if any
10. Construct execution environments and remaining services
11. Start reconciliation/background loops
12. Bind HTTP listener and report readiness
13. Discard startup resolver and temporary secret material
```

Ordering matters. The controller must not report readiness merely because the
HTTP socket is open. Readiness means the database is usable, schema policy has
completed, required execution-environment configuration is valid, and the
controller can safely reconcile durable work.

The database connection handle, stores, execution environments, logger,
reconciler, and validated non-secret startup snapshot may be long-lived. The
startup resolver is not.

#### Startup failure behavior

Startup should fail closed before serving work when any required bootstrap
operation fails. Errors should identify the configuration key or secret
reference that failed without exposing secret material.

Examples include:

- missing or malformed bootstrap document;
- duplicate or invalid variables;
- unresolved required database setting;
- inaccessible secret reference;
- database authentication or TLS failure;
- database schema newer than the controller supports;
- failed required migration;
- invalid required execution environment;
- listener bind failure.

Optional services must be explicitly marked optional. A missing database is
not optional once durable workflow execution is the controller's source of
truth.

If startup mutates the database through migrations, each migration follows the
transaction and backup policy defined by persistence design. Failure must not
leave the controller serving against a partially upgraded schema.

#### Reload and secret rotation

Startup configuration is immutable for one constructed controller runtime
unless a separate reload lifecycle is designed. Editing the source file must
not mutate an already constructed resolver or silently reconfigure active
runs.

A later reload operation should create a new resolver and a candidate runtime
configuration, validate it completely, then atomically replace only components
declared reloadable. Database credential rotation may require a new connection
pool. Run-snapshotted values remain unchanged even when deployment policy is
reloaded.

#### Startup resolution outputs

The startup operation should produce a validated, non-secret runtime
configuration with fields required to construct services. Conceptually:

```text
ControllerRuntimeConfig
  controller instance identity
  database connection intent
  secret references (not materialized values)
  resolver policy
  API listen/advertise settings
  execution-environment definitions
  reconciliation and retention policy
  logging policy
```

This runtime config is a normal immutable Go value passed to constructors. It
is not a global variable manager. It may also expose the safe subset of
controller variables eligible to be snapshotted into a new workflow run.

Current behavior partially follows this pattern for `ledger_db_path`: the
controller loads a config document, builds a temporary resolver, opens SQLite,
and discards the resolver. Important gaps are:

- the database driver is implicit and only SQLite is supported;
- secrets, TLS, connection pooling, and database identity are not modeled;
- controller environment and startup overrides are not assembled;
- the resolver policy is not itself resolved from startup configuration;
- execution-environment construction consumes structural config directly;
- readiness and phased startup are not explicit;
- the validated safe controller-variable subset is not retained for later run
  recipes;
- no reload or credential-rotation boundary exists.

### 2. Workflow submission

Purpose: create a durable workflow run and compile only the initially ready
stage.

Typical inputs:

- an immutable controller-config subset relevant to execution;
- configured client, controller, and worker environments;
- immutable project config and workflow definition;
- workflow variables;
- submission overrides;
- generated workflow-run identity, timestamps, and fingerprints.

Required durable outputs:

- the immutable workflow-run recipe inputs;
- normalized stage/step definitions and identities;
- initially compiled work items and their resolved input snapshots;
- workflow-level fingerprints and runtime values.

The submission resolver is discarded after the transaction commits or rolls
back. It must not be retained so that later stages can use it.

### 3. Ready-step compilation

Purpose: compile a later step or stage after its dependencies complete.

Typical inputs:

- the immutable workflow-run snapshot captured at submission;
- the retained workflow definition at its original revision;
- completed predecessor outputs as typed, read-only values;
- step-local bindings;
- generated step identity, evaluation time, and fingerprints.

Required durable outputs:

- step-instance state;
- zero or more immutable work items;
- ordered fan-out bindings;
- resolved input snapshots and fingerprints.

This is the most important create-use-discard case. Reconstructing the recipe
from durable records prevents a long-running controller object from becoming
the hidden owner of workflow correctness.

If compilation produces zero work items, the transaction records the typed
empty output and advances readiness without creating a placeholder item.

### 4. Worker request and assignment finalization

Purpose: claim one eligible logical work item and, only when necessary,
finalize values tied to a specific worker environment.

Typical inputs:

- the already compiled work item and its immutable resolved inputs;
- configured worker environment and execution target;
- worker identity/capabilities supplied by the request;
- generated attempt ID, assignment time, lease, and fencing values.

Required durable outputs:

- an atomic queued-to-running transition;
- the attempt identity and assignment snapshot;
- a concrete assignment payload returned only after commit.

Most workflow and step expressions should already be resolved when the work
item is compiled. Assignment-time resolution is justified for values that
cannot be known until a worker or target is selected, such as localized mount
paths or heterogeneous worker capabilities.

Therefore, a worker request should not normally rebuild and recompile the
entire work item. It may build a narrow assignment resolver. Any finalized
values must be written transactionally to the attempt or immutable assignment
snapshot before the payload is returned. Repeated requests must not silently
change the logical work item's fingerprints.

### 5. Work completion and downstream activation

Purpose: record one attempt outcome, construct typed logical outputs, and make
new stages ready.

Typical inputs:

- the active attempt and fencing identity;
- worker-reported result and observed state;
- the immutable work-item assignment snapshot;
- all terminal outputs required to determine step completion.

Required durable outputs:

- terminal attempt and work placement;
- immutable typed work-item and step outputs;
- step/stage completion state;
- newly compiled downstream work, when the transition completes a stage.

Completion may invoke a fresh ready-step resolver inside the same idempotent
database transition. The completed attempt's resolver is not reused: the
downstream step has a different identity boundary and a newly assembled recipe.

### 6. Controller restart and reconciliation

Purpose: resume decisions from durable state without restoring resolver
objects.

The controller reloads incomplete runs, placement state, definitions, and
snapshots. It creates new resolvers only for currently required decisions:
recompiling an idempotently ready stage, finalizing an assignment, evaluating
reuse, or reconciling runtime policy. Restart correctness is evidence that the
resolver lifecycle is truly ephemeral.

## Current Implementation Gap Analysis

### Already aligned

- `variable.Resolver` is a small value with no database, controller pointer,
  mutable memoization cache, or package-global state.
- Resolver methods use value receivers and resolution context is local to each
  call.
- `variable.Set` supports qualified lookup and precedence-aware unqualified
  lookup.
- Controller startup builds and discards a resolver for ledger configuration.
- Workflow submission builds and discards a resolver from workflow and
  submitted scopes.
- The workflow compiler accepts a resolver as an input instead of owning one.
- Target design documents already require immutable scopes, JIT resolver
  reconstruction, typed outputs, and database-backed run context.

### Missing or inconsistent

1. **Incomplete scope assembly.** Workflow submission currently constructs a
   resolver from only workflow variables and submitted variables. It omits
   controller config, environment, worker config, project config, and generated
   lifecycle scopes.
2. **Controller config is split.** `ControllerConfig.Variables` participates in
   startup ledger resolution, while `ExecutionEnvironmentConfig` follows a
   separate struct path. The boundary between typed resolution inputs and
   structural component configuration is not yet explicit.
3. **No durable run recipe.** The submitted workflow definition and effective
   scope snapshots are discarded after the HTTP request.
4. **Eager compilation.** All workflow steps compile at submission, before
   dependency outputs exist.
5. **No typed predecessor-output scope.** Completion records attempts but does
   not produce the typed logical output tree required by later steps.
6. **No lifecycle-specific assembler.** Call sites manually create scopes,
   sets, and resolvers. There is no controller-owned operation that states
   which scopes are valid for startup, workflow, step, work-item, or attempt
   resolution.
7. **Precedence depends on call order.** `NewSet` merges scopes in the order
   passed; it does not consult `variable.Precedence`. The declared precedence
   list is therefore documentation unless every assembler supplies scopes in
   exactly that order.
8. **No provenance output.** Resolution returns a typed value but not the
   winning source variable or the dependency set traversed. This limits
   explainability and makes execution-relevant fingerprint selection harder.
9. **Current queue is in memory.** `pending` and `assigned` cannot supply
   restart-safe assignment recipes or atomic claim/finalization behavior.
10. **Work-item persistence is incomplete.** The attempt ledger stores terminal
    snapshots, but the database does not yet own immutable compiled work items,
    queued/running placement, run snapshots, or step state.
11. **Assignment context is underspecified.** `GET /work/next` does not identify
    worker capabilities or a selected environment, so there is no principled
    input boundary for assignment-time resolution.
12. **Runtime metadata is added after compilation.** IDs and fingerprints are
    currently attached to compiled `model.WorkItem` values rather than first
    becoming read-only runtime variables available at their lifecycle
    boundary.
13. **No transaction boundary around resolution output.** Compilation,
    in-memory queue mutation, worker scaling, and worker launch are separate
    operations, so a resolver's results are not atomically published as durable
    work.
14. **Configuration time policy is undefined.** The design must distinguish
    controller values snapshotted per run from live deployment policy that may
    legitimately change between assignments or reconciliation cycles.

## Recommended Internal Boundaries

The following are responsibilities, not agreed Go type names.

### Definition stores and caches

Load immutable project and workflow documents by content identity. A cache may
retain decoded immutable documents, keyed by revision and fingerprint. Cache
eviction must not affect correctness because the source can be reloaded.

### Run snapshot store

Persist the immutable definition identities and variable source documents
needed to reconstruct every workflow-, step-, and work-item-level recipe for a
run. It should preserve typed expressions, not only currently resolved scalar
values, because later steps may resolve expressions against predecessor
outputs.

### Resolution-context assembler

Given an explicit purpose and lifecycle identity, load the applicable records,
validate namespace ownership, order scopes, create generated runtime bindings,
and return a fresh `variable.Resolver` plus metadata needed to capture its
outputs.

This belongs above `internal/variable`, likely in controller or workflow-run
orchestration code. It must not become a mutable manager holding every active
workflow's scopes.

### Compiler

Consume a fresh resolver and an immutable step definition. Produce immutable
logical work items, resolved inputs, dependency/provenance information, and
fingerprints without mutating queue state.

### Execution store

Persist compiler output and own transactional work placement, attempts,
outputs, and readiness transitions. This is the authority reconstructed after
restart.

### Reconciler

Observe durable state and initiate bounded operations such as ready-stage
compilation or worker scaling. It may repeatedly create resolvers; it does not
retain them between reconciliation passes.

## Decisions Needed Before Implementation

1. Which controller and environment values are immutable run inputs, and which
   are live deployment policy?
2. Are run snapshots stored as complete source documents, normalized variable
   rows, or both? Complete versioned JSON documents plus indexed identity
   columns currently align with the persistence direction.
3. How should predecessor outputs enter the variable model? The dependency
   design currently proposes a generated read-only `workflow.step[index]`
   structure, but its namespace and construction contract need to be fixed.
4. Should `variable.NewSet` itself enforce `Precedence`, or should a single
   controller assembler be responsible for ordered scope input? Relying on
   arbitrary call-site order is too weak.
5. What provenance must a resolution result expose: winning namespace only,
   complete reference dependency graph, or both?
6. Which values are permitted to remain unresolved until assignment, and can
   any of them affect logical work-item identity or reuse fingerprints?
7. Does a worker request identify one worker environment explicitly, or does
   the controller assign only from a homogeneous preconfigured pool?
8. What is the atomic boundary when successful completion activates the next
   stage: terminal output recording and downstream compilation in one
   transaction, or an idempotent durable ready marker consumed by a
   reconciler?

## Recommended First Design Slice

Before changing `resolver.go`, define the controller-owned resolver recipe for
one workflow submission and one later ready-step compilation. The artifact
should enumerate each source scope, whether it is snapshotted or live, when its
runtime values become known, and which resolved outputs are persisted.

That slice will show whether production changes belong in `variable.Set`, in a
new controller-side assembler, in persistence records, or in all three. The
present evidence does not justify adding mutable state or lifecycle knowledge
to `variable.Resolver`.
