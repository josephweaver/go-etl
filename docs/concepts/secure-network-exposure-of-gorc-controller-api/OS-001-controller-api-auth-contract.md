# OS-001: Controller API Authentication Contract

Status: Proposed  
Minimum recommended model: GPT-5.5 with high reasoning

## Objective

Define the controller's authentication configuration, role model, route-policy
model, and startup safety interlocks without yet wiring middleware into every
handler.

This slice establishes the contract later slices must follow.

## Current State

The controller configuration contains typed variables and an
`execution_environment`, but no `controller_config.authentication` variable.

The default listener is loopback, and `controller_url` is independently
configurable. Nothing currently prevents an operator from selecting a non-loopback
listen address while leaving all routes unauthenticated.

## Target State

The controller decodes and validates a structured authentication declaration from
the typed variable:

```text
controller_config.authentication
```

The declaration is a `type: "object"` variable. A bearer configuration looks like:

```json
{
  "name": {"namespace": "controller_config", "key": "authentication"},
  "type": "object",
  "expression": {
    "mode": {"type": "string", "expression": "bearer"},
    "credentials": {
      "type": "list",
      "expression": [
        {
          "type": "object",
          "expression": {
            "id": {"type": "string", "expression": "primary-client"},
            "role": {"type": "string", "expression": "client"},
            "token_env": {"type": "string", "expression": "GOET_CONTROLLER_CLIENT_TOKEN"}
          }
        },
        {
          "type": "object",
          "expression": {
            "id": {"type": "string", "expression": "worker-pool"},
            "role": {"type": "string", "expression": "worker"},
            "token_file": {"type": "path", "expression": "/etc/goet/secrets/controller-worker-token"}
          }
        },
        {
          "type": "object",
          "expression": {
            "id": {"type": "string", "expression": "operator"},
            "role": {"type": "string", "expression": "admin"},
            "token_file": {"type": "path", "expression": "/etc/goet/secrets/controller-admin-token"}
          }
        }
      ]
    }
  }
}
```

The safe local default belongs in `cmd/controller/defaults.json` as:

```json
{
  "name": {"namespace": "controller_config", "key": "authentication"},
  "type": "object",
  "expression": {
    "mode": {"type": "string", "expression": "disabled"},
    "credentials": {"type": "list", "expression": []}
  }
}
```

The test-only insecure advertised-HTTP override belongs in
`cmd/controller/defaults.json` as:

```json
{
  "name": {
    "namespace": "controller_config",
    "key": "controller_insecure_external_http_allowed"
  },
  "type": "bool",
  "expression": false
}
```

Supported phase-1 modes:

```text
disabled
bearer
```

Supported roles:

```text
client
worker
admin
```

## Strategic Rules

- `disabled` is allowed only when the resolved listen host is loopback.
- `bearer` is required for laptop-external and production profiles.
- A non-loopback listener with disabled authentication fails startup.
- An externally advertised plain `http://` URL fails startup unless:
  - it is loopback; or
  - `controller_config.controller_insecure_external_http_allowed` is `true`.
- Each credential has exactly one protected source.
- Phase 1 supports `token_env` and `token_file`.
- Raw token literals are not valid configuration fields.
- Authentication objects reject unknown fields.
- Empty token values fail startup.
- Duplicate credential IDs fail validation.
- Duplicate token material across credentials fails startup.
- Unknown roles and modes fail validation.
- Token source names/paths may appear in diagnostics; token contents may not.
- Token material is loaded once at startup.
- In-memory matching should use a fixed-size digest plus constant-time comparison.
- Authentication state is not persisted to SQLite.
- Authentication state is not exposed through status or logs.

## Startup Safety Rules

Loopback classification is deterministic and does not perform DNS lookup.

The listen host is loopback only when it is:

```text
localhost
127.0.0.0/8
::1
```

The listen host is non-loopback when it is:

```text
0.0.0.0
::
any non-loopback IP literal
any hostname other than localhost
```

For `controller_config.controller_url`:

- `https://...` is allowed.
- `http://localhost...`, `http://127.x.x.x...`, and `http://[::1]...` are
  allowed.
- `http://` with any other host fails unless
  `controller_config.controller_insecure_external_http_allowed` is `true`.
- missing scheme, missing host, and unsupported schemes fail startup validation.

If `controller_config.authentication.mode` is `disabled`,
`controller_config.controller_listen_host` must classify as loopback.

If `controller_config.authentication.mode` is `bearer`, credentials must be
non-empty and valid.

## Credential Source Rules

`controller_config.authentication` may contain only:

```text
mode
credentials
```

Each credential may contain only:

```text
id
role
token_env
token_file
```

Each credential must contain exactly one of:

```text
token_env
token_file
```

`token_env` names an environment variable read through the controller startup
environment lookup. `token_file` names a local file path read once during
startup.

Token values are normalized by trimming one trailing line ending from environment
or file sources. Interior whitespace and additional trailing whitespace are part
of the token. The resulting token must be non-empty.

Token files are restrictive when the platform exposes permission bits and group
or other users have no read, write, or execute permissions. On platforms where
permission bits are unavailable or unreliable, the implementation must document
the skipped permission check in the test name or error path.

Credential validation errors may name credential IDs, token environment variable
names, and token file paths. They must not include token contents.

## Route Policy Model

Create a pure route policy representation that can answer:

```go
Authorize(method, path, role) Decision
```

The policy must support:

- exact routes;
- route families such as `/submissions/`;
- method-specific policy;
- explicitly public routes;
- explicit denial for unregistered routes.

No default-allow behavior is permitted.

Admin is a reviewed superset role for phase 1. Admin may access every registered
route and method listed below. No other role receives access unless listed.

| Method | Route | Access |
|---|---|---|
| `GET` | `/healthz` | public |
| `POST` | `/workflow` | `client`, `admin` |
| `GET` | `/workflow-runs/{run}/source-bundle.zip` | `client`, `worker`, `admin` |
| `GET` | `/submissions/{id}/status` | `client`, `admin` |
| `GET` | `/submissions/{id}/logs` | `client`, `admin` |
| `POST` | `/work` | `client`, `admin` |
| `GET` | `/work/next` | `worker`, `admin` |
| `POST` | `/work/complete` | `worker`, `admin` |
| `POST` | `/work/fail` | `worker`, `admin` |
| `GET` | `/status` | `client`, `admin` |
| `GET` | `/observations/logs` | `client`, `admin` |
| `POST` | `/shutdown` | `admin` |

Wrong-method route combinations deny access in the pure policy. OS-002 may map
that denial to the live server's final `405 Method Not Allowed` behavior after
authentication and authorization are wired.

## Required Context

Read first:

- `cmd/controller/main.go`
- `cmd/controller/config.go`
- `cmd/controller/defaults.json`
- `cmd/controller/main_test.go`
- `cmd/controller/config_test.go`
- `docs/ARCHITECTURE_STATE.md`
- `docs/concepts/sensitive-variable-propagation/README.md`
- `docs/concepts/ssh-refinement/README.md`
- this concept README

## Allowed Production Files

- `internal/controllerauth/model.go`
- `internal/controllerauth/policy.go`
- `internal/controllerauth/credentials.go`
- `cmd/controller/config.go`
- `cmd/controller/main.go` only for startup validation/construction hooks
- `cmd/controller/defaults.json` for safe authentication and insecure-HTTP
  defaults
- this concept README only for tracker/status changes

Equivalent file names under one new `internal/controllerauth` package are allowed.
Do not spread policy logic through individual handlers in this slice.

## Allowed Test Files

- `internal/controllerauth/model_test.go`
- `internal/controllerauth/policy_test.go`
- `internal/controllerauth/credentials_test.go`
- `cmd/controller/config_test.go`
- `cmd/controller/main_test.go`

## Out of Scope

- HTTP middleware.
- Modifying client or worker requests.
- TLS termination.
- Caddy/Tailscale/Cloudflare configuration.
- OAuth/OIDC.
- Per-user identities.
- Per-project authorization.
- Token rotation APIs.
- Keystore selection.
- Automatic retry behavior.

## Acceptance Criteria

- Authentication configuration decodes and validates.
- Authentication defaults to disabled mode through `controller_config.authentication`
  in `defaults.json`.
- No configuration field accepts a raw token literal.
- Credential source resolution supports environment and restrictive files.
- Token contents never appear in returned errors.
- Empty and duplicate tokens are rejected.
- Route policy covers every currently registered route and method.
- Unknown route/method combinations deny access.
- `GET /healthz` is the only intentionally public route unless the slice records a
  reviewed exception.
- Disabled authentication plus non-loopback listen address fails startup
  validation.
- External HTTP advertised URLs fail closed except for explicit loopback or
  test-only override cases.
- `controller_config.controller_insecure_external_http_allowed` defaults to
  `false`.
- Pure policy tests cover public, allowed, wrong-role, unknown-route, and
  wrong-method cases.
- Default local `localhost:8080` development remains possible.

## Stop Conditions

Stop and append to `issues.md` if:

- current callers require one route to serve both client and worker behavior in a
  way that cannot be safely separated;
- credential material would need to be persisted;
- controller startup cannot determine whether the listen host is loopback;
- the canonical configuration work has established a conflicting structured
  configuration rule;
- authentication cannot be tested without changing route handlers.

## Completion Evidence

Record:

- final configuration shape;
- complete route-role table;
- startup interlock tests;
- sentinel proof that raw tokens are absent from authentication errors,
  controller status, controlled logs, and inspected SQLite persistence.
