# OS-003: Shared Controller HTTP Client

Status: Proposed  
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
    relativePath string,
    body io.Reader,
) (*http.Request, error)

func (c *Client) Do(
    req *http.Request,
    expectedStatus ...int,
) (*http.Response, error)
```

The exact API may differ, but all controller requests must be constructible without
exposing token values.

## Requirements

- Parse and normalize the base URL once.
- Reject a base URL containing user-info credentials.
- Reject fragments.
- Join endpoint paths without allowing host replacement.
- Preserve escaped path components where required.
- Add bearer authorization in one place.
- Add a caller/version user agent.
- Set content type only when a body contract requires it.
- Use an injected `*http.Client`.
- Provide a safe default timeout when a caller does not supply one.
- Disable redirects or allow only same-scheme, same-host redirects.
- Never forward a bearer credential to another origin.
- Bound error-body reads.
- Return structured safe errors with status code and a sanitized short body.
- Do not include the authorization header or token in request/error formatting.
- Require HTTPS for non-loopback URLs.
- Permit plain HTTP for explicit loopback development.
- Do not add automatic retries in this slice.

## Token Provider

Support a narrow interface:

```go
type TokenProvider interface {
    Token(context.Context) (SensitiveToken, error)
}
```

A static in-memory provider may be used after a caller has loaded a token from an
environment variable or file.

`SensitiveToken` must redact default string/JSON formatting. Reuse the repository's
existing sensitive-value primitives if they fit without coupling controller
authentication to workflow variable resolution.

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
- existing sensitive-value package only if a small reusable safe-token type belongs
  there
- this concept README only for tracker/status updates

## Allowed Test Files

- `internal/controllerhttp/client_test.go`
- `internal/controllerhttp/token_test.go`
- `internal/controllerhttp/error_test.go`

## Out of Scope

- Migrating CLI callers.
- Migrating worker callers.
- Reading token files or environment variables.
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
- Context cancellation works.
- Default client timeout is nonzero.
- No test depends on external network access.
- The package has no knowledge of CLI, Slurm, SSH, or workflow semantics.

## Stop Conditions

Stop and append to `issues.md` if:

- the existing sensitive-value types would create a dependency cycle;
- a caller currently relies on cross-host redirects;
- endpoint construction cannot preserve the current API paths;
- safe errors require exposing request headers.
