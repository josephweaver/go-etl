# OS-003: Shared Controller HTTP Client

Status: Implemented  
Minimum recommended model: GPT-5.4-mini  
Reference: EC-3 / operational slice / files(4)+tests+doc

## Objective

Create one reusable, safe HTTP request layer for all GORC controller callers.

This slice builds the shared package but does not yet migrate every CLI and worker
call site.

## Current State

The user-facing client already accepts an injected `*http.Client`, but request
construction is repeated.

The worker uses package-level `http.Get` and `http.Post`. That prevents consistent:

- authorization headers;
- timeout configuration;
- redirect safety;
- base-URL validation;
- bounded error handling;
- caller identification.

## Target State

Add a package with a narrow shape similar to:

```go
type Client struct {
    BaseURL string
    HTTP    *http.Client
    Token   TokenProvider
    Caller  string
}

func (c *Client) NewRequest(
    ctx context.Context,
    method string,
    path string,
    body io.Reader,
) (*http.Request, error)

func (c *Client) Do(
    req *http.Request,
    expectedStatus ...int,
) (*http.Response, error)
```

The exact API may differ, but all controller requests must be constructible without
exposing token values.

The package must not expose a generic absolute-or-relative URL builder. Fixed
routes use paths such as `/status`. Routes with variable IDs use a helper that
escapes each path segment.

## Requirements

- Parse and normalize the base URL once.
- Reject a base URL containing user-info credentials.
- Reject fragments.
- Join endpoint paths without allowing scheme or host replacement.
- Request paths must start with exactly one `/`, must not start with `//`, and
  must not contain a scheme, host, raw query string, or fragment.
- Provide a helper for variable route segments, for example
  `PathJoin("/submissions", submissionID, "status")`.
- Escape each variable path segment with `url.PathEscape`.
- Reject any route segment that is empty, contains `/`, contains `\`, or would
  decode to a string containing `/` or `\`, including `%2F`.
- Supply query values separately, not by embedding `?` in the path string.
- Add bearer authorization in one place.
- Allow a nil token provider only for explicitly public requests such as
  `GET /healthz`.
- Protected route requests must fail before send when no token provider is
  configured.
- Add a caller/version user agent.
- Set content type only when a body contract requires it.
- Use an injected `*http.Client`.
- Provide a safe default timeout when a caller does not supply one.
- Clone the supplied `*http.Client` value before applying package defaults.
- Install a redirect policy that allows only same-scheme, same-host redirects.
- Never forward a bearer credential to another origin.
- Bound error-body reads to 4 KiB.
- Return structured safe errors with status code and a sanitized short body.
- Close response bodies before returning an error for an unexpected status.
- Leave response bodies open only when the status is expected and the response is
  returned to the caller.
- Do not include the authorization header or token in request/error formatting.
- Require HTTPS for non-loopback URLs.
- Permit plain HTTP for explicit loopback development.
- Reuse the OS-001 deterministic loopback rules: `localhost`, `127.0.0.0/8`, and
  `::1` are loopback; no DNS lookup is performed.
- Do not add automatic retries in this slice.

## Token Provider

Support a narrow interface:

```go
type TokenProvider interface {
    Token(context.Context) (SensitiveToken, error)
}
```

A static in-memory provider may be used after a caller has loaded a token from an
environment variable or file. Token loading is out of scope for this slice.

`SensitiveToken` is owned by `internal/controllerhttp`. Do not reuse worker
`SensitiveValue` or `internal/variable.ResolvedValue`: worker safe values live in
package `main`, and workflow variable values would couple controller HTTP auth to
workflow resolution. `SensitiveToken` stores token text privately, exposes
plaintext only through an explicit method used by request construction, and
redacts `String`, `GoString`, `Error`, and `MarshalJSON`.

## Required Context

Read first:

- existing package containing `ControllerClient`;
- `cmd/worker/state.go`;
- `cmd/worker/main.go`;
- implemented Sensitive Variable Propagation safe-rendering types;
- OS-001 and OS-002.

## Allowed Production Files

- `internal/controllerhttp/client.go`
- `internal/controllerhttp/token.go`
- `internal/controllerhttp/error.go`
- this concept README only for tracker/status updates

## Allowed Test Files

- `internal/controllerhttp/client_test.go`
- `internal/controllerhttp/token_test.go`
- `internal/controllerhttp/error_test.go`

## Out of Scope

- Migrating CLI callers.
- Migrating worker callers.
- Reading token files or environment variables.
- Reusing workflow-variable or worker-only sensitive value types.
- Retry/backoff.
- mTLS.
- Custom private certificate authorities.
- Proxy selection.
- Route authorization.
- Deployment configuration.

## Acceptance Criteria

- Requests use the configured base URL and cannot replace its host.
- Bearer credentials are added correctly.
- Token sentinels never appear in errors or formatted values.
- Redirects to another origin are rejected before credentials can be forwarded.
- Non-loopback HTTP is rejected.
- Loopback HTTP is allowed.
- Public HTTPS is allowed through the normal Go trust store.
- Error bodies are size bounded.
- Unexpected-status errors close the response body.
- Context cancellation works.
- Default client timeout is nonzero.
- Variable path segments cannot introduce extra path separators.
- No test depends on external network access.
- The package has no knowledge of CLI, Slurm, SSH, or workflow semantics.

## Stop Conditions

Stop and append to `issues.md` if:

- a caller currently relies on cross-host redirects;
- endpoint construction cannot preserve the current API paths;
- safe errors require exposing request headers.
