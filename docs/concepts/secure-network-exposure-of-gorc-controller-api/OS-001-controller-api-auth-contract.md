# OS-001: Controller API Authentication Contract

Status: Proposed  
Minimum recommended model: GPT-5.5 with high reasoning  
Reference: EC-3 / operational slice / files(5)+tests+doc

## Objective

Define the controller's authentication configuration, role model, route-policy
model, and startup safety interlocks without yet wiring middleware into every
handler.

This slice establishes the contract later slices must follow.

## Current State

The controller configuration contains typed variables and an
`execution_environment`, but no explicit API authentication object.

The default listener is loopback, and `controller_url` is independently
configurable. Nothing currently prevents an operator from selecting a non-loopback
listen address while leaving all routes unauthenticated.

## Target State

The controller can decode and validate a structured authentication declaration:

```json
{
  "authentication": {
    "mode": "bearer",
    "credentials": [
      {
        "id": "primary-client",
        "role": "client",
        "token_env": "GOET_CONTROLLER_CLIENT_TOKEN"
      },
      {
        "id": "worker-pool",
        "role": "worker",
        "token_file": "/etc/goet/secrets/controller-worker-token"
      },
      {
        "id": "operator",
        "role": "admin",
        "token_file": "/etc/goet/secrets/controller-admin-token"
      }
    ]
  }
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

- `disabled` is allowed only when the resolved listen address is loopback.
- `bearer` is required for laptop-external and production profiles.
- A non-loopback listener with disabled authentication fails startup.
- An externally advertised plain `http://` URL fails startup unless:
  - it is loopback; or
  - an explicit test-only insecure override is enabled.
- Each credential has exactly one protected source.
- Phase 1 supports `token_env` and `token_file`.
- Raw token literals are not valid configuration fields.
- Empty token values fail startup.
- Duplicate credential IDs fail validation.
- Duplicate token material across credentials fails startup.
- Unknown roles and modes fail validation.
- Token source names/paths may appear in diagnostics; token contents may not.
- Token material is loaded once at startup.
- In-memory matching should use a fixed-size digest plus constant-time comparison.
- Authentication state is not persisted to SQLite.
- Authentication state is not exposed through status or logs.

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

The initial public route is:

```text
GET /healthz
```

The implementation agent must inventory all routes registered by
`registerControllerRoutes` and inspect actual client/worker callers before
finalizing role assignments.

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
- `cmd/controller/defaults.json` only if a safe scalar default is required
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
- sentinel proof that raw tokens are absent from errors and serialized config
  snapshots.
