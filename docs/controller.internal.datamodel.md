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
| Project execution-environment definition | Project/content revision | Project configuration | Immutable per run snapshot |
| Workflow definition | Content revision | Repository or definition store | Immutable per revision |
| Workflow run snapshot | Workflow run | Database | Immutable after submission |
| Step/stage state | Workflow run | Database | Transactional lifecycle transitions |
| Typed step outputs | Workflow run and retention period | Database/artifact store | Immutable after successful completion |
| Work item | Logical work lifetime | Database | Definition immutable; placement changes |
| Attempt | Attempt lifetime and retention period | Database | Append/transition under fencing rules |
| Definition cache | Controller process | Derived from immutable source | Replaceable and disposable |
| Resolver | One evaluation | In-memory recipe inputs | No mutation after construction |

The controller may keep persistent handles to the database, immutable config,
execution-environment factories/capabilities, and definition caches. A
concrete customer execution environment is selected from project/run context;
it is not global controller state. These objects supply resolver inputs; they
do not turn the resolver into long-running state.

## Configuration Retention and Reference Strength

Controller, project, and workflow configurations have different retention
requirements. Treating all of them as permanently resident controller objects
would make controller memory grow with the complete customer catalog.

### Controller configuration: process invariant

The effective controller configuration produced by Case 1 is invariant for one
constructed controller runtime. The controller keeps this validated,
non-secret runtime config resident because nearly every operation may need it
and there is only one effective controller configuration per process.

The source JSON file, environment capture, and command-line overrides remain
provenance for that effective config. Editing any source does not mutate the
running controller. A reload is a separate lifecycle that builds and validates
a replacement runtime config before swapping explicitly reloadable components.

This invariance does not mean every controller value is copied into every
workflow run. Case 2 exports only the subset allowed to influence workflow
execution.

### Project and workflow configurations: reloadable references

The controller catalog should retain lightweight references to projects and
workflows rather than keeping every decoded definition resident. In this
document, a **weak definition reference** means a durable identity plus enough
information to reload and verify the definition. It does not mean a Go runtime
weak pointer.

A project reference should identify at least:

- project ID;
- source/repository identity;
- immutable source revision or content version;
- project-config path or object key;
- expected content fingerprint;
- schema version.

A workflow reference should identify at least:

- owning project ID;
- workflow definition ID;
- immutable source revision or content version;
- workflow path or object key;
- expected content fingerprint;
- schema version.

The exact locator depends on the definition store. A Git-backed source may use
repository, commit, and relative path. An object store may use bucket, object
key, version, and digest. A local development source may use a path plus content
digest, but an unversioned mutable path alone is not an immutable identity.

### Bounded definition cache

Decoded project and workflow definitions may be cached by immutable content
identity:

```text
(definition kind, source identity, immutable revision, path/key, fingerprint)
```

The cache is an optimization with explicit bounds, such as maximum entries,
maximum decoded bytes, and/or time-based eviction. It may use different bounds
for projects and workflows. Eviction removes only the decoded object; it does
not delete the durable catalog reference or an active run snapshot.

On access:

```text
lookup immutable cache key
        |
        +-- hit --> return validated immutable definition
        |
        +-- miss --> load from definition store
                         |
                         v
                    validate schema
                         |
                         v
                    verify fingerprint
                         |
                         v
                    cache and return
```

Concurrent misses for the same immutable key should be coalesced so a burst of
submissions does not reload and decode the same document repeatedly. Cache
entries must be immutable after publication. Cache statistics may feed
controller metrics but do not affect resolver semantics.

The controller should not preload or periodically scan all projects and
workflows. Lookup begins from the requested project/workflow identity. This
keeps resident memory proportional to the active working set rather than a
catalog containing, for example, 1,000 projects and 100,000 workflows.

### Missing and changed definitions

A cache miss is normal and causes reload. A missing or unverifiable source is
not equivalent to an undefined workflow:

- before run acceptance, submission fails with a definition-unavailable or
  integrity error;
- after run acceptance, execution uses the run's strong immutable snapshot;
- background catalog/status operations may report the weak reference as
  unavailable without invalidating already accepted runs.

When a locator returns content whose fingerprint differs from the reference,
the controller rejects it. It must not silently update the reference or run the
new content under the old identity. A changed definition receives a new
content identity/revision.

Negative lookup caching may prevent repeated load pressure for missing
definitions, but it must be short-lived or explicitly invalidated so a newly
restored source becomes visible.

### Accepted runs: strong immutable pins

Weak catalog references are insufficient once a workflow run is accepted. The
run must strongly pin the exact project and workflow content used to construct
its resolver. This is required because later steps may compile after cache
eviction, controller restart, or loss of the upstream definition source.

A strong run pin consists of immutable content identities plus either:

- canonical definition/config documents stored with the run; or
- references to a controller-managed immutable content store whose retention
  is at least as long as the run and required audit/reuse period.

The run must never rely only on an external mutable path. Case 3 reconstructs
its resolver from the strong run snapshot, not from the weak catalog reference
or definition cache.

This produces three clear tiers:

| Tier | Retention | Role |
|---|---|---|
| Effective controller runtime config | Resident and invariant for process lifetime | Constructs and governs the controller |
| Project/workflow catalog reference and cache | Durable lightweight reference plus bounded, evictable decoded cache | Discovers and validates definitions on demand |
| Accepted workflow-run snapshot | Strong immutable pin for run/audit lifetime | Reconstructs later resolvers and proves execution lineage |

Strong run snapshots may be deduplicated by content hash so many runs using the
same project/workflow revision do not store duplicate canonical documents.
Reference counting or retention queries must be database-backed; in-memory
cache reachability is not evidence that durable content can be deleted.

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
- Which execution-environment component types and definition stores can this
  controller support?
- What resolver limits, logging policy, retention policy, and reconciliation
  timing apply?

Not every answer is a plain variable. Configuration has two forms:

- **Structural configuration** selects and wires components, such as database
  driver, secret-provider type, transport, scheduler, or runtime.
- **Typed variables** supply values consumed by those components, such as a
  database path, host, port, timeout, controller URL, or secret reference.

The current `ControllerConfig` has this split: `ExecutionEnvironment` is
structural while `Variables` are resolved values. The structural/value split
is legitimate, but placing the concrete execution environment in controller
config is transitional. Project configuration should own the customer's
environment definition or environment-profile selection.

#### Bootstrap input sources

For the current controller, startup resolution has four sources. The first
three are external configuration inputs; the fourth is generated by the
controller:

1. **Controller config JSON file.** By default, this file is located next to
   the controller executable. The command line may specify a different path.
2. **Specific system environment variables.** The controller reads only an
   explicit allowlist of supported variables.
3. **Command-line overrides.** These may be supplied directly by a person or
   service invoking the controller command, or generated by a client that
   starts a controller on demand.
4. **Controller runtime variables.** These are read-only values determined by
   the running controller, such as process ID, controller instance ID, startup
   time, queued-item count, or worker count.

The precedence from lowest to highest is:

```text
controller config JSON
    < allowlisted system environment
    < command-line overrides
    < controller-generated read-only runtime values
```

Runtime values occupy the highest variable namespace so users cannot override
identities, timestamps, or operational observations owned by the controller.
They are a resolver source, but not user configuration.

Built-in defaults are also not a fourth external source. They are part of the
controller's versioned configuration schema. A default applies only when none
of the three external sources supplies that setting, and the effective value
must be visible in the validated runtime configuration.

##### Config-file selection

Config-file selection occurs before value resolution:

```text
explicit command-line config path
    else config file next to controller executable
    else startup error
```

The selected file establishes the base structural configuration and
`controller_config` variable scope. A command-line config path selects the
base document; it does not by itself become a resolved controller variable.

"Next to the executable" means resolving the executable's directory and
joining the defined config filename to it. It does not mean the process's
current working directory. This matters when a client starts the controller
from another directory.

The current implementation does not yet meet this rule: it first probes
`cmd/controller/controller-default-config.json` and then
`controller-default-config.json` relative to the current working directory.

##### Environment capture

Supported environment variables must be mapped deliberately into typed
`controller_env` variables. For each supported key, the controller owns:

- the external environment-variable name;
- the internal variable key and type;
- whether it is required;
- whether its value is secret material, a secret reference, or non-secret;
- parsing and redaction behavior.

The controller must not import its entire process environment. An allowlist
makes startup reproducible and prevents incidental host variables from
changing controller behavior.

Environment values are captured once during startup. Later mutations to the
parent process or host environment do not change the constructed controller
runtime.

##### Command-line overrides

Command-line overrides map into typed `override` variables. They use the same
keys and types as the settings they replace; flags must not create a parallel
configuration vocabulary.

For example, a direct invocation and an on-demand client launch should produce
the same controller argument contract:

```text
controller --config <path> --set controller_url=<value>
```

The exact flag syntax remains to be designed. The invariant is that the client
does not gain an in-process configuration channel into the controller. It
constructs an ordinary controller command line, and the started controller
parses and validates those overrides exactly as it would for any other caller.

Command-line secret values should be avoided because process arguments may be
visible through process inspection and shell history. The command line should
prefer overriding a secret reference, while the selected environment variable
or mounted secret source holds the material.

##### Normalization and resolution

After all three sources are loaded, startup normalizes them into separate
immutable scopes:

```text
JSON variables          -> controller_config
environment variables   -> controller_env
command-line values      -> override
generated startup values -> runtime
```

The startup resolver is then built once from those scopes in declared
precedence order. Structural settings that select component types follow the
same source precedence even if they are decoded into Go structs rather than
represented as variables. A structural setting and a typed variable must not
both claim authority over the same concept.

The resulting effective runtime configuration should record non-secret
provenance: which source supplied each winning setting. This allows startup
diagnostics to explain, for example, that a database host came from the JSON
file while its credential reference came from an environment override.

##### Controller HTTP server configuration

The controller's HTTP server is controller-level infrastructure and belongs in
the controller startup configuration. It must not be inferred from project or
workflow configuration.

The configuration must distinguish two addresses:

- **Listen address** tells the operating system where the controller process
  binds. It consists of a host/interface and port.
- **Advertised URL** tells clients and workers how to reach the controller.

These values are often different. A controller may listen on `0.0.0.0:8080`
inside a container while workers use
`http://host.docker.internal:8080`, or it may listen behind a reverse proxy and
advertise an HTTPS URL with a different public port.

The initial server settings should include:

| Setting | Type | Purpose |
|---|---|---|
| `controller_listen_host` | string | Interface or IP on which to bind |
| `controller_listen_port` | int | TCP port on which to bind |
| `controller_url` | string | Base URL advertised to clients and workers |
| `controller_read_header_timeout_seconds` | int | Limit for reading request headers |
| `controller_read_timeout_seconds` | int | Limit for reading complete requests |
| `controller_write_timeout_seconds` | int | Limit for writing responses |
| `controller_idle_timeout_seconds` | int | Keep-alive idle limit |
| `controller_shutdown_timeout_seconds` | int | Graceful shutdown limit |
| `controller_max_request_bytes` | int | Maximum accepted request body size |
| `controller_max_header_bytes` | int | Maximum accepted request-header size |

Optional direct-TLS settings may include a TLS-enabled flag and references to
the server certificate and private key. Private key material follows the
secret-handling rules and must not be stored as an ordinary resolved value.
When TLS terminates at a reverse proxy, the controller may listen with plain
HTTP on its private interface while advertising an HTTPS URL.

JSON provides the base values. Supported environment variables and
command-line flags may override the same typed keys through the normal startup
precedence. They must not introduce separate `http.Server` settings that bypass
the resolver.

The startup operation resolves these variables into a validated immutable
server config before constructing `http.Server`. Validation should reject:

- an empty listen host unless an explicit all-interfaces spelling is agreed;
- ports outside `1..65535`, except port `0` when explicitly permitted for tests;
- negative timeouts or size limits;
- a malformed advertised URL;
- an advertised URL without `http` or `https` scheme;
- incomplete direct-TLS configuration;
- direct-TLS settings inconsistent with the advertised scheme.

Authentication, trusted proxies, cross-origin policy, and per-endpoint rate
limits are also controller-level HTTP policy, but their concrete settings
should be added only when their security model is designed. They should not be
silently enabled by generic web-framework defaults.

Binding to all interfaces is a deployment decision, not a safe universal
default. Local development may default to loopback. Container or remote-worker
deployments may explicitly select `0.0.0.0` or `[::]` and must rely on the
deployment's authentication, firewall, and TLS policy.

The listener should be bound only after the database, schema, and required
controller services are ready. If binding fails, startup fails. The actual
bound address should become a process-stable runtime variable. This is
especially important when tests allow port `0`, because the selected port is
known only after binding.

Example conceptual values:

```text
controller_config.controller_listen_host = "127.0.0.1"
controller_config.controller_listen_port = 8080
controller_config.controller_url = "http://localhost:8080"
controller_config.controller_read_header_timeout_seconds = 5
controller_config.controller_shutdown_timeout_seconds = 30
```

The current implementation only partially models this. The JSON file contains
`controller_config.controller_url`, but `main.go` always binds an
`http.Server` to hard-coded `:8080`. The resolved URL and actual listener can
therefore disagree, and the server does not yet configure explicit production
timeouts or request-size limits.

##### Controller runtime variables

Controller-generated runtime variables have two different lifetimes.

**Process-stable runtime variables** are captured once during startup and stay
constant for that controller process:

- controller instance ID;
- operating-system process ID;
- startup timestamp;
- controller executable or build version;
- host or deployment identity, when explicitly configured or discovered.

**Operational runtime variables** are observations of controller state at one
evaluation time:

- queued-item count;
- running or assigned-item count;
- active worker count;
- available worker capacity;
- failed-item count;
- current reconciliation time;
- database or scheduler health observations.

Operational values must not be stored and continually updated inside a
resolver. The controller reads them from the authoritative execution store,
worker registry, scheduler state, or metrics source immediately before a
decision. It converts that observation into an immutable `runtime` scope,
builds a resolver, performs the decision, and discards the resolver.

```text
authoritative state at evaluation time
               |
               v
 capture typed runtime observation + observed_at
               |
               v
 build resolver -> evaluate decision -> discard resolver
```

Every operational runtime snapshot should include an observation timestamp.
Where consistency matters, related values should come from one database
transaction or one coherent status read. Combining a queue count from before a
claim with a worker count from after the claim can produce a state that never
actually existed.

These runtime variables are suitable for controller decisions such as worker
scaling, admission control, status generation, and reconciliation. They should
not automatically become workflow inputs or fingerprint material. A workflow
run snapshot includes a runtime value only when the lifecycle contract
explicitly requires that captured value. Process ID and transient queue depth,
for example, normally have no place in a work-item correctness fingerprint.

Metrics and resolver variables overlap, but they are not identical concepts:

- a metric is an observation exported for monitoring, usually as a numeric
  time series;
- a runtime variable is a typed value made available to one controller
  evaluation;
- the same authoritative measurement may feed both;
- the resolver must not query the metrics backend as its source of truth.

Names should describe the observation precisely, including units where
needed. Examples include `runtime.controller_process_id`,
`runtime.queued_item_count`, `runtime.active_worker_count`, and
`runtime.observed_at`.

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

The standard variable declaration may mark any value as sensitive. Sensitivity
is metadata on the variable, not a separate secret-variable type. For example:

```json
{
  "name": {
    "namespace": "project_config",
    "key": "postgres_password"
  },
  "type": "string",
  "expression": "${client_env.DB_PASSWORD}",
  "sensitive": true
}
```

This declaration says that `project_config.postgres_password` is a string,
gets its value from the client's captured `DB_PASSWORD`, and must receive
sensitive handling.

For long-lived deployment credentials, typed configuration should still prefer
secret references over embedding secret values. For example, a database
password variable may identify a secret by name or URI; the secret provider
materializes its value only when the database component is constructed.

```text
controller_config.database_password_ref = "env://GOET_DB_PASSWORD"
```

The exact reference syntax and supported providers remain design decisions.
Likely early providers are an explicitly named environment variable and a
mounted file. OS keychains or external secret managers can be added behind the
same provider boundary when required.

Sensitive material follows stricter rules than ordinary resolved values:

- never store it in workflow-run resolver snapshots;
- never include it in fingerprints or provenance payloads;
- never return it through status or diagnostics;
- never format it into an error message or log record;
- expose it to a resolver only for the bounded evaluation that requires it;
- retain it in memory only as long as its consuming component requires;
- preserve the secret reference and provider identity for audit, not the
  materialized value.

Sensitivity propagates through resolution. A resolved value is sensitive when
its declaration is marked sensitive or when it depends on any sensitive value.
Setting `sensitive: false` must not declassify a sensitive dependency. For
structured values, field/item sensitivity should be retained, and a containing
value must be treated as sensitive when any descendant is sensitive.

This creates an intentional boundary: the ordinary variable resolver may
operate on a temporarily materialized sensitive value, but secret lookup and
durable decryption remain outside `variable.Resolver.Resolve`. Those are I/O
operations, while the resolver should remain a deterministic in-memory
evaluation operation.

For database clients that internally retain credentials or tokens, the
controller cannot guarantee immediate memory destruction. It can still avoid
duplicating the secret, constrain its scope, and delegate rotation/reconnect
behavior to an explicit database credential component.

The current Go model does not implement this contract. `variable.Variable`
contains a name and typed expression, while `ResolvedValue` has no sensitivity
or redaction metadata. Adding `sensitive` requires coordinated JSON-model,
resolution-propagation, diagnostics, persistence, and test design; it is not
only an extra decoded field.

#### Phased startup lifecycle

```text
1. Parse command line and select the controller config JSON path
2. Load and definition-validate the JSON document
3. Capture supported system environment variables
4. Parse and type-check command-line value overrides
5. Normalize JSON, environment, override, and generated runtime scopes
6. Build the startup resolver
7. Resolve and validate database connection intent and secret references
8. Materialize required secrets through the selected secret provider
9. Open database and verify connectivity
10. Read schema version and apply allowed migrations
11. Load database-backed controller metadata, if any
12. Register supported execution-environment component factories and construct
    remaining controller services
13. Start reconciliation/background loops
14. Bind HTTP listener and report readiness
15. Discard startup resolver and temporary secret material
```

Ordering matters. The controller must not report readiness merely because the
HTTP socket is open. Readiness means the database is usable, schema policy has
completed, required component types are available, and the controller can
safely reconcile durable work. Project-specific environment validity is
checked when a project or run is loaded, not by assuming one global startup
environment.

The database connection handle, stores, component registry, logger, reconciler,
and validated non-secret startup snapshot may be long-lived. A project-specific
environment may be cached or pooled after construction, but its identity and
lifetime remain attached to project/run context. The startup resolver is not
long-lived.

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
- missing execution-environment component type required to resume durable work;
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
  supported execution-environment component types and deployment policy
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
- one concrete execution environment is currently read from controller config
  and stored globally on `Controller.env`;
- readiness and phased startup are not explicit;
- the validated safe controller-variable subset is not retained for later run
  recipes;
- no reload or credential-rotation boundary exists.

#### Execution-environment ownership

A concrete execution environment is project-level configuration because it
describes where that project's work is allowed and expected to run. Different
customers or projects may use local processes, distinct Slurm clusters, cloud
accounts, containers, credentials, mounts, queues, and worker limits while
sharing one controller service.

The ownership split should be:

| Concern | Owner |
|---|---|
| Supported transport, dialect, scheduler, and runtime implementations | Controller deployment/code |
| Policies restricting allowed component types, hosts, accounts, or limits | Controller deployment |
| Concrete environment definition or approved profile selection | Project configuration |
| Per-run immutable selected environment snapshot | Workflow run |
| Secret material used to connect to the environment | Secret provider |
| Secret references and non-secret connection intent | Project config/run snapshot, subject to redaction policy |
| Constructed clients, sessions, and connection pools | Controller runtime, keyed by environment identity |

This avoids two incorrect extremes:

- a single `Controller.env` that forces every project onto the same compute;
- project configuration that can load arbitrary controller code or bypass
  deployment security policy.

The project selects and configures from component types the controller supports
and permits. For example, a project may select an approved `ssh + bash + slurm
+ worker` composition and supply its host, queue, mount, and secret references.
The controller still owns the implementations, validates policy, resolves
secrets, constructs connections, and performs scheduling.

The project definition is immutable by content revision. At workflow
submission, the controller resolves the selected environment using the
project's configuration plus allowed submission overrides and snapshots the
effective non-secret environment definition into the workflow run. Later
step compilation and assignment use that run snapshot, not a newly edited
project definition.

```text
controller capabilities and policy
                 +
project environment definition
                 +
allowed submission overrides
                 |
                 v
validated per-run environment snapshot
                 |
                 v
constructed/cached runtime components keyed by environment identity
```

The environment snapshot and the live connection are different objects. The
snapshot is durable configuration and lineage. The connection is disposable
runtime state that can be recreated after controller restart.

Environment caching is an optimization. Cache keys must include the immutable
effective environment identity and credential/provider identity needed for
safe isolation. Cache eviction must not change workflow meaning. Credential
rotation may recreate the live connection without rewriting the run's
non-secret environment snapshot.

This boundary also changes startup readiness. Startup validates that the
controller can load its database and register required component
implementations. It does not need to contact every customer's compute system.
Project environment validation or preflight happens when the project/run is
accepted and may be repeated before scheduling according to policy.

### 2. Workflow submission

Purpose: combine controller, project, workflow, client-override, and declared
environment inputs into one immutable workflow-run snapshot, then compile only
the initially ready stage.

Case 2 is the boundary where external configuration becomes durable run
context. Later compilation must not depend on those external sources still
being unchanged or available.

#### Case 2 input layers

The submission resolver is assembled from these layers in normal namespace
precedence:

```text
eligible controller startup values
    + project configuration and execution environment
    + workflow configuration
    + client submission overrides
    + declared environment values used by project/workflow
    + generated workflow-run runtime values
```

More specifically:

1. **Controller base.** A safe subset of the validated controller runtime
   configuration from Case 1. This may include resolver policy, controller URL,
   supported capability/version information, and deployment policy needed to
   validate the run. It excludes database credentials, HTTP listener internals,
   transient queue metrics, and unrelated controller secrets.
2. **Project configuration.** The immutable project definition at a specific
   content revision, including its execution-environment definition or
   approved environment-profile selection.
3. **Workflow configuration.** The immutable workflow definition and its typed
   workflow variables at a specific content revision within the project.
4. **Client overrides.** Typed values supplied in the submission envelope. The
   controller validates which keys may be overridden; generated runtime values
   and deployment security policy are never overrideable.
5. **Declared environment capture.** Only the client, controller, and configured
   worker environment keys explicitly required by the project or workflow.
6. **Generated workflow runtime.** Read-only values such as run ID, workflow
   definition ID, submission time, source revisions, and definition
   fingerprint.

"Prior controller elements" therefore means an explicit exportable subset of
Case 1 output. It does not mean copying the entire startup resolver or every
controller variable into the workflow run.

#### Environment-variable capture

Environment use is declared through the same standard variable declarations
used by controller, project, and workflow configuration. There is no separate
environment-dependency document. A qualified expression such as
`${client_env.DB_PASSWORD}` declares the source namespace and key.

Environment access must still be explicit and bounded. Before resolution, the
controller recursively inspects typed expressions for qualified `client_env`,
`controller_env`, and `worker_env` references. Computed environment-variable
names are not supported.

The relevant namespaces remain distinct:

- `client_env` contains approved values captured by the submitting client and
  transmitted in the submission;
- `controller_env` contains approved values captured by the controller;
- `worker_env` contains the configured environment that the selected project
  execution environment will inject into workers.

The controller must not satisfy `client_env` from its own process environment,
or infer `worker_env` by inspecting an arbitrary active worker. Each namespace
has a different authority. A workflow reference is a request for a value, not
authorization to read it: client and controller capture policies still decide
which environment keys may be exposed.

Only referenced keys are captured. This reduces accidental coupling and secret
exposure. Non-sensitive captured values may enter the ordinary run snapshot.
Sensitive captured values may enter a bounded in-memory resolver, but must not
be stored as plaintext in the run's JSON documents.

If a sensitive client value is needed by Case 3 after the client exits, Case 2
must place it in an approved protected-value store and persist an opaque value
reference in the run snapshot. Case 3 temporarily materializes that protected
value while reconstructing its resolver. An alternative is to prove that all
uses were fully resolved and safely persisted during Case 2; silently rereading
the environment later is not valid.

Environment capture occurs once for the run. Case 3 uses the captured values,
not the current environment. This guarantees that a controller restart, client
exit, or host environment change does not alter downstream resolution.

#### Case 2 sequence

```text
1. Parse submission and identify project/workflow revisions
2. Load immutable project and workflow definitions
3. Definition-validate typed expressions and dependency structure
4. Discover qualified environment references and authorize their keys
5. Capture the required client/controller/configured-worker environment values
6. Validate and normalize client overrides
7. Generate workflow-run identity and runtime values
8. Assemble scopes and create the submission resolver
9. Resolve run-level values and compile only the initially ready stage
10. Persist the run snapshot, stage plan, and compiled work atomically
11. Commit or roll back, then discard the resolver
```

If any required value is missing, has the wrong type, violates policy, or
cannot be resolved, submission fails before the run becomes visible. The
controller must not persist a run without the recipe required to resume it.

#### Required durable outputs

Case 2 persists:

- project and workflow content identities/revisions;
- immutable project and workflow source documents or durable references to
  them;
- captured controller, environment, and override source scopes, with sensitive
  values replaced by protected-value references in durable JSON;
- generated workflow-run runtime scope;
- resolver policy and evaluation timestamp;
- normalized stage/step definitions and identities;
- initially compiled work items and their resolved input snapshots;
- workflow and initially available step/work-item fingerprints;
- redacted provenance showing which source won each consumed value.

The system should persist source expressions/scopes needed by later steps as
well as resolved values consumed during initial compilation. Persisting only a
flattened set of Case 2 results is insufficient because Case 3 introduces
predecessor outputs that were unavailable at submission.

The submission resolver is discarded after the transaction commits or rolls
back. It must not be retained for later stages.

### 3. Ready-step compilation

Purpose: reconstruct the Case 2 run context after dependencies complete, add
the newly available predecessor outputs and step bindings, and compile one
ready step or stage.

Case 2 and Case 3 use the same project/workflow basis, but they differ in where
their inputs come from:

| Concern | Case 2: submission | Case 3: ready step |
|---|---|---|
| Controller/project/workflow config | Read, validate, and snapshot | Reload from run snapshot |
| Environment values | Capture declared values | Reload captured values |
| Client overrides | Validate and snapshot | Reload from run snapshot |
| Workflow runtime | Generate and snapshot | Reload existing run values |
| Predecessor outputs | Usually unavailable | Load completed typed outputs |
| Step/work-item runtime | Initial stage only | Generate for newly ready stage |
| Result | Create run and initial work | Add later work to existing run |

Case 3 must not reread the current project file, workflow file, client process
environment, controller process environment, or mutable controller
configuration. Doing so would allow step 2 to execute under different inputs
than step 1.

Typical Case 3 additions are:

- completed predecessor outputs as typed, read-only values;
- step-local bindings;
- fan-out item bindings;
- generated step/work-item identities;
- the step evaluation timestamp and derived fingerprints.

Live controller metrics such as current queue depth or active worker count are
not workflow expression inputs by default. They may drive a separate
controller scheduling-policy resolver, but admitting them into workflow
semantics would make recompilation timing affect work-item meaning. Any future
exception requires an explicit capture and replay rule.

Case 3 produces and atomically persists:

- step-instance state;
- zero or more immutable work items;
- ordered fan-out bindings;
- resolved input snapshots, provenance, and fingerprints;
- an idempotent marker preventing the same stage from compiling twice.

This is the most important create-use-discard case. Reconstructing the recipe
from durable records prevents a long-running controller object from becoming
the hidden owner of workflow correctness.

If compilation produces zero work items, the transaction records the typed
empty output and advances readiness without creating a placeholder item. The
Case 3 resolver is discarded after commit or rollback.

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
15. **No scalable definition-reference/cache boundary exists.** Project and
    workflow definitions currently arrive in the submission payload. There is
    no weak catalog reference, immutable definition loader, bounded decoded
    cache, fingerprint verification path, or strong run pin separated from
    cache residency.

## Recommended Internal Boundaries

The following are responsibilities, not agreed Go type names.

### Definition stores and caches

Load immutable project and workflow documents by content identity. A cache may
retain decoded immutable documents, keyed by revision and fingerprint. Cache
eviction must not affect correctness because the source can be reloaded.
Accepted runs use their strong immutable run pins instead of depending on this
cache.

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
