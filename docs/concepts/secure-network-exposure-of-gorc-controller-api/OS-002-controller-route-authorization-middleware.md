# OS-002: Controller Route Authorization Middleware

Status: Proposed  
Minimum recommended model: GPT-5.5 with high reasoning  
Reference: EC-3 / operational slice / files(4)+tests+doc

## Objective

Apply the OS-001 authentication and route-policy contract to the live controller
HTTP server.

## Current State

`registerControllerRoutes` attaches handlers directly to `http.ServeMux`.
Handlers generally enforce HTTP methods and domain behavior but do not share an
authentication or role-authorization layer.

The current shutdown route can stop the server after method and controller-state
checks. It must not be reachable by an unauthenticated network caller.

## Target State

Controller requests flow through one security boundary:

```text
request
  -> request metadata normalization
  -> public-route decision or bearer authentication
  -> role authorization
  -> existing handler
```

Expected status semantics:

```text
401 Unauthorized  missing, malformed, or invalid credential
403 Forbidden     valid credential with insufficient role
404 Not Found     route not registered
405 Method Not Allowed authenticated caller used unsupported method
```

The final 404/405 ordering may follow the standard mux behavior, but it must not
accidentally disclose protected data.

## Requirements

- Add middleware around the controller mux or route registration.
- Do not duplicate bearer parsing in individual handlers.
- Parse only the standard `Authorization: Bearer <token>` form.
- Reject multiple authorization values.
- Reject empty bearer values.
- Do not echo authorization input.
- Use OS-001 constant-time credential matching.
- Attach the authenticated principal/role to request context.
- Require the explicit route-role policy before calling a handler.
- Set `Cache-Control: no-store` on authentication failures.
- Use a generic response body for `401` and `403`.
- Do not log raw authorization headers.
- Preserve request-size limits and existing server timeouts.
- Keep `/healthz` minimal and public.
- Change SSH callback preflight from `/status` to `/healthz` unless it deliberately
  supplies worker authentication.
- Require `admin` for `/shutdown`.
- Ensure an admin is not accidentally accepted everywhere unless the OS-001 policy
  explicitly defines admin as a reviewed superset.

## Required Context

Read first:

- implemented OS-001 files;
- `cmd/controller/main.go`;
- `cmd/controller/main_test.go`;
- `cmd/controller/ssh_callback_tunnel.go` or the current callback-tunnel/preflight
  implementation;
- `docs/concepts/ssh-refinement/OS-002-reverse-controller-callback-tunnel.md`;
- current controller route caller tests.

## Allowed Production Files

- `cmd/controller/auth_middleware.go`
- `cmd/controller/main.go`
- current SSH callback/preflight file only to change liveness checks from
  `/status` to `/healthz`
- `internal/controllerauth/*` only for small contract corrections discovered while
  wiring the middleware
- this concept README only for tracker/status updates

## Allowed Test Files

- `cmd/controller/auth_middleware_test.go`
- `cmd/controller/main_test.go`
- current callback-tunnel/preflight tests
- `internal/controllerauth/*_test.go` only for contract corrections

## Out of Scope

- Client credential loading.
- Worker credential loading.
- TLS or reverse-proxy configuration.
- Rate limiting.
- Browser cookies.
- OIDC.
- Database schema changes.
- Audit event persistence beyond existing safe logs.

## Acceptance Criteria

- Every protected route rejects a missing credential with `401`.
- Every protected route rejects an invalid credential with `401`.
- A valid credential with the wrong role receives `403`.
- `/shutdown` succeeds only for the administrator role.
- A worker cannot submit workflows or shut down the controller.
- A client cannot claim or complete worker assignments.
- `/healthz` works without authentication and returns only minimal liveness.
- `/status` is no longer used as an unauthenticated reachability probe.
- Existing authorized handler behavior and response payloads remain unchanged.
- Authorization headers and token sentinels are absent from logs and errors.
- Recovery-mode behavior remains intact after authentication.
- The controller still works in explicit loopback-only authentication-disabled
  development mode.

## Security Tests

At minimum, table-drive:

```text
route
method
credential state
role
expected status
handler called?
```

Include:

- no header;
- malformed scheme;
- empty bearer;
- invalid token;
- client token;
- worker token;
- admin token;
- duplicate header;
- unknown path;
- wrong method.

## Stop Conditions

Stop and append to `issues.md` if:

- a handler performs work before authorization;
- route matching can bypass policy through path normalization or a prefix edge
  case;
- a route's actual caller cannot be determined;
- auth failure logging contains header values;
- an existing test requires public access to non-health controller state.
