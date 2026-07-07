# Controller Startup Resolution Epic

Status: Complete

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
- Load controller startup values from five sources:
  - namespace-specific declarations from the standard defaults JSON document;
  - serialized controller JSON;
  - direct `controller_env` access to controller process environment variables;
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
standard defaults JSON
+ serialized controller JSON
+ controller_env accessor
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

### Defaults document

Defaults are canonical variable declarations stored in a separate standard
JSON document. The document formerly described as `globals` is not a `global`
namespace and does not own an independent set of runtime variables. Instead,
it supplies namespace-specific default declarations.

The required filename is `defaults.json` beside the selected controller
document. An explicit `--config path/to/controller.json` therefore selects
`path/to/defaults.json`; default executable-relative `controller.json` selects
`defaults.json` in the executable directory. The defaults document uses
`api_version: goet/v1alpha1` and `kind: Defaults`.

Within a namespace, an explicit declaration from the selected controller JSON
replaces a defaults-document declaration with the same qualified name. Normal
namespace precedence then applies to unqualified lookup. A qualified lookup
may therefore resolve a defaults-document declaration when the selected
controller JSON does not replace it. Provenance identifies the defaults
document as the source rather than presenting the value as an implicit code
default.

The defaults document may declare `client_config`, `controller_config`,
`worker_config`, and `project_config`. It cannot declare environment,
`override`, `runtime`, workflow, step, work-item, deprecated global, or legacy
namespaces. Required values without a documented default remain absent until
supplied by an authorized explicit source.

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
3. Initialize the controller environment accessor
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

The following are candidate implementation slices for review. They are ordered
from the controller entry boundary inward and do not yet authorize creation of
numbered slice files:

1. **Controller document envelope** — add and test `api_version` and `kind`
   decoding and validation around the existing controller variable document.
2. **Startup command-line contract** — parse explicit `--config` and repeated
   canonical-JSON `--override` arguments without constructing controller
   services.
3. **Executable-relative config discovery** — select the default controller
   document next to the executable while preserving an explicit config path.
4. **Defaults document loading** — load and validate the required
   config-adjacent `defaults.json` document and its namespace restrictions.
5. **Defaults and controller layering** — retain both validated documents and
   layer explicit controller declarations above matching qualified defaults
   without losing source identity.
6. **Controller environment accessor** — add bounded, cached, string-only
   `controller_env` lookup without enumerating the process environment or
   exposing values in diagnostics.
7. **Startup override scope** — validate each CLI declaration as canonical
   variable JSON, require the `override` namespace, and assemble it above
   configurable startup namespaces.
8. **Generated startup runtime scope** — generate process ID, instance ID,
   startup time, and build version as immutable `runtime` values.
9. **Startup resolver assembly** — construct and discard bounded resolvers from
   defaults, controller config, environment access, overrides, and runtime;
   bootstrap `resolver_max_depth` and preserve non-secret provenance.
10. **Main database contract** — resolve the qualified database driver and
   connection string, reject missing dependencies with redacted context, open
   the database, verify schema/migrations, and fail before HTTP binding.
11. **Controller filesystem contracts** — resolve and validate controller root,
    Git-cache, temporary, artifact-cache, and log paths against the process
    working directory before constructing their consumers.
12. **Operational policy contracts** — resolve and validate the agreed
    millisecond, capacity, concurrency, cleanup, caretaker, and log-level
    variables from the same startup source model.
13. **HTTP server contract** — resolve listen host/port, advertised URL, timeout,
    request-size, header-size, and shutdown settings before constructing the
    HTTP server.
14. **Exclusive database ownership integration** — require the lock or lease
    supplied by `controller-resilience` before recovery or API admission and
    exit a competing controller without binding HTTP.
15. **Recovery-mode admission integration** — after required services and
    durable recovery state are ready, expose only health and worker
    heartbeat/report APIs, capture `runtime.controller_recovery_started_at`,
    hand off to caretaker recovery, and enable normal admission only when the
    `attempt-liveness-recovery` contract permits it.
16. **Startup integration coverage** — exercise the complete success path,
    precedence, qualified lookup protection, redacted failure paths,
    fail-without-bind behavior, recovery-mode boundary, and normal readiness.

Sensitivity propagation and sink sanitization are implemented by
`sensitive-variable-propagation`; database ownership mechanics by
`controller-resilience`; rendered logging behavior by `execution-observability`;
and heartbeat tracking/recovery policy by `attempt-liveness-recovery`. This epic
integrates those completed contracts at startup rather than reimplementing
them.

## Agreed Decisions

- Startup precedence from lowest to highest is:

  ```text
  controller_env
      < controller_config
      < override
      < runtime
  ```

- Before namespace precedence is applied, declarations within each namespace
  are layered from the standard defaults JSON document to the selected
  controller JSON. The explicit controller declaration wins when both define
  the same qualified name.
- Controller-document variable declarations must already use the canonical
  `controller_config` namespace. Loading rejects other namespaces rather than
  silently rewriting the serialized source, so diagnostics and provenance
  remain truthful.
- Until GOET introduces a duration type, controller schedules, intervals,
  timeouts, and retention ages are positive integer milliseconds. Their keys
  use a `_milliseconds` suffix so the unit is explicit.
- Configuration, variable resolution, database, schema, cache, logging, and
  required-service construction failures terminate startup without binding an
  HTTP listener. The controller does not remain alive solely to expose a
  failed-startup diagnostic endpoint. Recovery mode is distinct: after normal
  bootstrap succeeds, it intentionally exposes only health and worker
  heartbeat/report APIs before normal admission begins.
- Each controller process uses exactly one database, and only one active
  controller may own orchestration access to a given database. Startup must
  acquire or verify that ownership after connecting and before recovery or API
  admission. A competing controller that encounters an existing valid lock or
  lease exits without binding HTTP. This startup epic owns the readiness
  requirement; `controller-resilience` owns the concrete lock or lease
  mechanism and stale-owner policy. Separate read-only database observers are
  not controller instances and may operate when the database backend permits
  them.

- The serialized controller JSON therefore wins when `controller_env` and
  `controller_config` provide the same unqualified key.
- A controller-config expression may still explicitly reference a qualified
  environment value such as `${controller_env.DB_PASSWORD}`.
- The CLI accepts an `override` declaration for any key; it does not maintain a
  separate override allowlist or denylist. An unqualified lookup follows normal
  precedence and may select that override. A qualified lookup such as
  `controller_config.main_database_connection_string` reads only that
  namespace and therefore cannot be replaced by an `override` declaration.
  Each startup consumer's required-variable contract chooses deliberately
  between qualified and precedence-based lookup.
- Generated runtime values remain read-only and non-overridable.
- Each command-line override is supplied as a repeated `--override` argument
  whose value is one canonical JSON variable declaration. The declaration uses
  the same recursive typed-expression schema as serialized variables and must
  use the `override` namespace. This supports scalar and structured values
  without introducing a second command-line type syntax. Inline command-line
  overrides are not an approved secret transport because command arguments may
  be exposed through process inspection or shell history.
- Controller JSON requires `api_version` and `kind` metadata. The initial
  values are `goet/v1alpha1` and `Controller`.
- `controller_env` is a direct accessor namespace over the controller process
  environment. `${controller_env.DB_PASSWORD}` reads the operating-system
  environment key `DB_PASSWORD`; no separate mapping table or hard-coded key
  catalog is required. The full environment is not copied into a scope or
  exposed through diagnostics. A resolver reads each referenced key once and
  reuses that value for the bounded resolution so one operation observes a
  consistent value. Every `controller_env` value has type `string`, matching
  the operating-system environment boundary. A typed expression that requires
  a non-string value cannot consume an environment value directly; any future
  string-to-type conversion must be introduced explicitly in the expression
  language rather than inferred by the environment accessor.
- The operating-system process environment, populated by the launcher or
  deployment system, is the initial source for values such as
  `controller_env.DB_PASSWORD`. GOET does not define how that environment is
  populated. Environment values are never enumerated or rendered in
  diagnostics. Explicit sensitivity, propagation, safe rendering, and sink
  sanitization follow the `sensitive-variable-propagation` epic.
- The main database consumer requires both variables below. Neither has a
  default or permits client/command-line override:

  | Key | Type | Sensitive |
  |---|---|---|
  | `main_database_driver` | string | No |
  | `main_database_connection_string` | string | Declared or propagated from sensitive dependencies |

- The database driver is explicit rather than inferred from the connection
  string. Pool, connection-lifetime, and migration-policy variables are
  deferred until a concrete requirement exists.
- The HTTP server consumer uses the following non-sensitive variables. Each
  permits an authorized startup command-line/client override:

  | Key | Type | Declaration required | Defaults document |
  |---|---|---:|---|
  | `controller_listen_host` | string | No | `localhost` |
  | `controller_listen_port` | int | No | `8080` |
  | `controller_url` | string | Yes | None |

- The listen host/port and advertised controller URL remain separate because
  container networking, port forwarding, and reverse proxies may make them
  different.
- Controller filesystem storage uses the following non-sensitive path
  variables. Each permits an authorized startup override:

  | Key | Declaration required | Defaults document | Lifetime |
  |---|---:|---|---|
  | `controller_root_dir` | No | `./.run` | Root for controller-owned local state |
  | `controller_git_cache_path` | No | `${controller_root_dir}/git_cache` | Semi-persistent across controller restarts |
  | `controller_temp_path` | No | `${controller_root_dir}/temp` | Disposable per-operation staging |
  | `controller_artifact_cache_path` | No | `${controller_root_dir}/artifacts` | Retained published worker packages |

- These paths remain separate because they have different integrity, cleanup,
  capacity, and restart semantics. The derived defaults are ordinary typed path
  expressions and therefore follow normal resolution and provenance rules.
- Caretaker startup uses the following non-sensitive, startup-overridable
  integer variables:

  | Key | Declaration required | Defaults document | Validation |
  |---|---:|---:|---|
  | `caretaker_interval_schedule_milliseconds` | No | `60000` | Greater than zero |
  | `caretaker_missed_interval_limit` | No | `1` | Greater than or equal to one |

- A missed-interval limit of one still permits multiple worker heartbeat
  attempts within each 60,000-millisecond caretaker interval; it abandons work
  only when the caretaker consumes an interval containing no report.
- Relative controller-owned paths are resolved against the controller process
  working directory. The executable location and controller-config location do
  not change that base. An on-demand launcher must therefore set the working
  directory deliberately. Absolute paths remain unchanged.
- Resolver recursion uses the non-sensitive, startup-overridable integer
  `resolver_max_depth`, with defaults-document value `10` and validation greater than
  zero. Startup first uses the built-in depth limit to resolve and validate this
  setting, then constructs subsequent resolvers with the resolved value.
- Initial controller logging uses the following non-sensitive,
  startup-overridable variables:

  | Key | Type | Defaults document |
  |---|---|---|
  | `controller_log_root_path` | path | `${controller_root_dir}/logs` |
  | `controller_filesystem_logging_enabled` | bool | `true` |
  | `controller_log_level` | string | `info` |

- Detailed rendered-line formatting remains owned by the execution-observability
  epic rather than this startup-resolution epic.
- Initial HTTP safety policy uses the following non-sensitive,
  startup-overridable variables:

  | Key | Type | Defaults document |
  |---|---|---:|
  | `controller_read_header_timeout_milliseconds` | int | `5000` |
  | `controller_read_timeout_milliseconds` | int | `30000` |
  | `controller_write_timeout_milliseconds` | int | `30000` |
  | `controller_idle_timeout_milliseconds` | int | `120000` |
  | `controller_shutdown_timeout_milliseconds` | int | `30000` |
  | `controller_max_request_bytes` | int | `16777216` |
  | `controller_max_header_bytes` | int | `1048576` |

- Timeout values must be greater than zero, and byte limits must be positive.
- The semi-persistent Git cache uses the following non-sensitive,
  startup-overridable policy variables:

  | Key | Type | Defaults document |
  |---|---|---:|
  | `controller_git_cache_max_size_mb` | int | `10240` |
  | `controller_git_cache_retention_milliseconds` | int | `604800000` |
  | `controller_git_fetch_timeout_milliseconds` | int | `300000` |
  | `controller_git_fetch_concurrency` | int | `4` |

- These values must be positive. Commits required by active runs remain pinned
  even when their objects place the cache above its target size. If the
  controller cannot recover enough space without deleting pinned content, a
  new fetch fails explicitly rather than evicting active-run inputs.
- Temp staging and published artifact retention use the following
  non-sensitive, startup-overridable policy variables:

  | Key | Type | Defaults document |
  |---|---|---:|
  | `controller_temp_cleanup_age_milliseconds` | int | `86400000` |
  | `controller_artifact_cache_max_size_mb` | int | `10240` |
  | `controller_artifact_cache_retention_milliseconds` | int | `604800000` |
  | `controller_storage_min_free_mb` | int | `1024` |

- These values must be positive. Active packaging directories and artifacts
  referenced by queued or running work are pinned. Cleanup removes unpinned
  content oldest-first. New packaging fails clearly before consuming the
  configured free-space reserve.

## Open Questions

No open questions remain. The epic must not move from `Proposed` to `Ready`
until the human explicitly approves it and agrees to begin slice decomposition.

## Completion Criteria

- The controller finds the default JSON relative to its executable and honors
  an explicit command-line path.
- Startup rejects missing, unsupported, or incorrect `api_version`/`kind`
  metadata before resolving variables.
- Defaults JSON, controller JSON, directly accessed environment variables,
  command-line overrides, and generated runtime values assemble into one
  tested precedence model.
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
- Relative controller paths resolve consistently against the controller process
  working directory, including when an on-demand client starts the process.
- No global resolver or duplicate aggregate configuration authority is added.
- Required services and database schema are ready before normal API admission.
- Heartbeat/report APIs become reachable at the documented recovery boundary.
- Only the approved Case 1 subset is available to workflow submission.
- Relevant config, resolver, database, HTTP, startup, diagnostics, and
  integration tests pass.
