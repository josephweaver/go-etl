# Controller Startup Resolution Epic

Status: Proposed

## Purpose

Make controller startup entirely driven by the standard typed-variable system:
load the serialized controller JSON and approved startup sources, construct a
short-lived resolver, require each controller component to resolve and validate
the variables it needs, and fail startup before normal API admission when a
required value or dependency is unavailable.

The serialized controller JSON remains the configuration authority. This epic
must not introduce a duplicate aggregate `ControllerRuntimeConfig`.

## Goals

- Locate the controller JSON next to the executable by default or use an
  explicit command-line path.
- Load controller startup values from four sources:
  - serialized controller JSON;
  - explicitly supported controller process environment variables;
  - command-line overrides supplied directly or by an on-demand client;
  - generated read-only controller runtime variables.
- Normalize each source into its standard variable namespace.
- Make client/command-line `override` the highest authorized configurable
  source while keeping generated `runtime` values read-only and
  non-overridable.
- Retain the immutable serialized/captured source scopes for the lifetime of
  the constructed controller process.
- Build and discard resolvers for bounded startup decisions rather than storing
  one long-running resolver.
- Let each component own a small required-variable contract and produce
  contextual missing, wrong-type, invalid-value, or dependency errors.
- Require the main database connection variables and fail closed when the
  connection string or one of its environment dependencies is missing.
- Support sensitive controller variables without logging, persisting, or
  returning materialized secret values.
- Resolve and validate HTTP listen/advertise settings before constructing the
  web server.
- Resolve Git-cache, controller-temp, artifact-cache, caretaker, logging, and
  other controller-owned operational settings through the same variable model.
- Generate process-stable runtime values such as controller process ID,
  controller instance ID, startup time, and recovery-start time at their
  correct lifecycle boundaries.
- Construct required services before reporting normal readiness.
- Expose heartbeat/report APIs in recovery mode before admitting submissions or
  new work claims, then hand control to caretaker recovery.
- Preserve non-secret provenance showing which source supplied each consumed
  value.

## Non-Goals

- Resolving project, workflow, step, work-item, or assignment variables.
- Loading a concrete customer execution environment at controller startup;
  execution environments are project/run scoped.
- Creating or retaining one global resolver.
- Creating an aggregate runtime-configuration object that becomes a second
  configuration authority.
- Implementing sensitivity propagation or a protected secret store; those are
  owned by `sensitive-variable-propagation`.
- Implementing the heartbeat tracker or caretaker abandonment policy; those are
  owned by `attempt-liveness-recovery`.
- Implementing controller high availability, leader election, or multiple
  active controllers.
- Designing live configuration reload in the first implementation. Reload is a
  separate lifecycle that may reuse the same resolution contracts later.
- Defining project/workflow Git dependency resolution or worker artifact
  packaging behavior.

## Architectural Context

This is Case 1 from `docs/controller.internal.datamodel.md`. It builds the
controller's own infrastructure before workflow submission begins.

The current controller loads `ControllerConfig`, normalizes its variables into
`controller_config`, creates a temporary resolver only for `ledger_db_path`,
constructs one global execution environment directly from a struct, and binds
HTTP to hard-coded `:8080`. Environment capture, general CLI overrides,
sensitive metadata, component-owned variable contracts, cache paths, and
phased readiness are not implemented.

The target source model is:

```text
serialized controller JSON
+ approved controller_env values
+ command-line override values
+ generated runtime values
              |
              v
      short-lived startup resolver
              |
              +-- database contract --> database handle
              +-- HTTP contract -----> HTTP server
              +-- Git/cache contract -> cache services
              +-- caretaker contract -> caretaker configuration
              +-- logging contract --> controller logger
```

Constructed handles and services may be long-lived. The resolver is not.

### Serialized controller document

The default config location is a defined filename next to the controller
executable, not relative to the process working directory. A command-line config
path selects a different base document before variable resolution begins.

The document has required language-neutral metadata around the standard
variable declarations:

```json
{
  "api_version": "goet/v1alpha1",
  "kind": "Controller",
  "variables": [
    {
      "name": {
        "namespace": "controller_config",
        "key": "main_database_connection_string"
      },
      "type": "string",
      "expression": "postgres://goet:${controller_env.DB_PASSWORD}@db.example/goet",
      "sensitive": true
    }
  ]
}
```

`api_version` selects the document schema. `kind` prevents a project or
workflow document from being accepted accidentally as controller config. Both
are validated before variable definition validation or resolution and do not
participate in variable precedence.

Structural component selection should not become an uncontrolled parallel
configuration system. Component implementations are supplied by controller
code/plugins; their configurable values should be standard variables whenever
the value participates in source precedence or diagnostics.

### Required-variable contracts

Each startup consumer requests only the keys it owns. The database consumer,
for example, requires `main_database_connection_string` and any agreed pool,
timeout, TLS, or migration-policy variables. A missing root key differs from a
root expression whose dependency is missing:

```text
controller startup: required variable
controller_config.main_database_connection_string is missing
```

```text
controller startup: resolve
controller_config.main_database_connection_string:
controller_env.DB_PASSWORD is missing
```

Error paths may evolve, but they must identify the consumer and variable while
redacting sensitive values.

Narrow constructor arguments copied into a database or HTTP library are normal
implementation details. They are not retained as an alternative configuration
model.

### Startup phases

The target lifecycle is:

```text
1. Parse command line and select controller JSON path
2. Load and definition-validate the serialized document
3. Capture supported controller environment variables
4. Parse and type-check command-line overrides
5. Generate startup runtime variables
6. Assemble immutable scopes in agreed precedence
7. Resolve and construct the main database connection
8. Verify schema and apply allowed migrations
9. Resolve and construct cache, logging, and controller services
10. Rebuild active Git pins and load durable recovery state
11. Resolve and construct the HTTP server
12. Expose heartbeat/report APIs in recovery mode
13. Capture runtime.controller_recovery_started_at
14. Hand off to caretaker recovery
15. Admit normal API traffic only after recovery policy permits it
```

Startup fails closed if any required phase fails. Merely binding the HTTP socket
does not mean the controller is ready for normal work.

### Runtime variables

Process-stable generated values include:

```text
runtime.controller_process_id
runtime.controller_instance_id
runtime.controller_started_at
runtime.controller_build_version
```

`runtime.controller_recovery_started_at` is generated later, when workers can
reach heartbeat/report endpoints. Mutable operational observations such as
queued-item or active-worker counts are captured into fresh runtime scopes for
individual controller decisions; they are not continuously updated in a
startup resolver.

### Relationship to the other resolution cases

Case 2 consumes only an explicitly approved subset of the retained Case 1
source scopes. It must not inherit database credentials, HTTP listener
internals, unrelated environment variables, or other controller-only secrets.

Cases 3 and 4 consume the durable Case 2 run recipe rather than rereading
mutable controller startup sources.

## Proposed Slices

No implementation slices are agreed yet. Slice decomposition should begin only
after the variable catalog, precedence, secret boundary, and startup readiness
questions below are resolved and the epic is explicitly moved to `Ready`.

## Agreed Decisions

- Startup precedence from lowest to highest is:

  ```text
  controller_env
      < controller_config
      < override
      < runtime
  ```

- The serialized controller JSON therefore wins when `controller_env` and
  `controller_config` provide the same unqualified key.
- A controller-config expression may still explicitly reference a qualified
  environment value such as `${controller_env.DB_PASSWORD}`.
- Accepted client/command-line overrides win over controller config for keys
  that policy permits callers to override.
- Generated runtime values remain read-only and non-overridable.
- Controller JSON requires `api_version` and `kind` metadata. The initial
  values are `goet/v1alpha1` and `Controller`.
- The main database consumer requires both variables below. Neither has a
  default or permits client/command-line override:

  | Key | Type | Sensitive |
  |---|---|---|
  | `main_database_driver` | string | No |
  | `main_database_connection_string` | string | Declared or propagated from sensitive dependencies |

- The database driver is explicit rather than inferred from the connection
  string. Pool, connection-lifetime, and migration-policy variables are
  deferred until a concrete requirement exists.
- The HTTP server consumer requires the following non-sensitive variables.
  Each has no default and permits an authorized startup command-line/client
  override:

  | Key | Type |
  |---|---|
  | `controller_listen_host` | string |
  | `controller_listen_port` | int |
  | `controller_url` | string |

- The listen host/port and advertised controller URL remain separate because
  container networking, port forwarding, and reverse proxies may make them
  different.

## Open Questions

1. What is the complete initial required/optional variable catalog, including
   types, defaults, sensitivity, allowed override status, and owning consumer?
2. Which controller environment variables are supported initially, and how are
   external names mapped to typed internal keys?
3. What command-line syntax supplies typed overrides, and how does it represent
   structured values without inventing a second schema?
4. Which keys are forbidden from client/command-line override even though the
   `override` namespace otherwise has highest configurable precedence?
5. Which first secret source materializes `controller_env.DB_PASSWORD`, and
   what transport/storage guarantees are prerequisite?
6. What schedule syntax represents caretaker and other interval values before
   GOET has a duration type?
7. Which settings have defaults, and how are defaults represented so
    provenance remains visible?
8. Which startup failures may expose a limited diagnostic HTTP endpoint, and
    which require the process to exit without binding?
9. Does controller exclusivity/database locking belong in this epic's startup
    readiness boundary or exclusively in `controller-resilience`?

## Completion Criteria

- The controller finds the default JSON relative to its executable and honors
  an explicit command-line path.
- Startup rejects missing, unsupported, or incorrect `api_version`/`kind`
  metadata before resolving variables.
- Controller JSON, approved environment variables, command-line overrides, and
  generated runtime values assemble into one tested precedence model.
- Client override wins for authorized configurable keys; runtime values remain
  read-only.
- Every initial startup consumer has a documented and tested variable contract.
- Database startup requires an explicit driver and connection string and does
  not infer the driver or accept client override of either key.
- A missing main database connection key and a missing referenced database
  password produce distinct redacted errors.
- Sensitive startup values never appear in logs, diagnostics, persistence, or
  HTTP responses.
- HTTP listen and advertised addresses are resolved independently and validated.
- Git cache, temp, artifact, caretaker, and logging settings come from typed
  variables rather than hidden defaults or parallel config paths.
- No global resolver or duplicate aggregate configuration authority is added.
- Required services and database schema are ready before normal API admission.
- Heartbeat/report APIs become reachable at the documented recovery boundary.
- Only the approved Case 1 subset is available to workflow submission.
- Relevant config, resolver, database, HTTP, startup, diagnostics, and
  integration tests pass.
