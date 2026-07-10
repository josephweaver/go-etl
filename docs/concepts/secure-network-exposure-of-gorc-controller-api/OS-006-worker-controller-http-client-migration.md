# OS-006: Worker Controller HTTP Client Migration

Status: Proposed  
Minimum recommended model: GPT-5.4-mini  
Reference: EC-3 / operational slice / files(4)+tests+doc

## Objective

Migrate worker work-claim, completion, and failure requests from package-level HTTP
functions to the OS-003 authenticated controller client.

## Current State

`cmd/worker/state.go` uses:

```go
http.Get(...)
http.Post(...)
```

The worker loop passes only a controller URL into helper functions.

## Target State

Worker startup:

```text
load worker config
load controller token file
construct one controller HTTP client
validate worker
run worker loop using the client
```

All controller requests share:

- base URL validation;
- bearer authorization;
- timeout;
- redirect policy;
- safe errors;
- caller user-agent.

## Requirements

- Add a worker-owned controller client field or dependency.
- Load the token file once at worker startup.
- Keep token material in memory only.
- Do not reopen the token file for every request.
- Migrate:
  - work claim;
  - completion report;
  - failure report;
  - any current worker observation/source-bundle controller calls.
- Preserve existing request JSON and expected status codes.
- Preserve `attempt_id` behavior.
- Do not add automatic retries to completion/failure reports.
- If work claiming later gains polling/retry, keep that separate from this slice.
- Return safe distinctions for:
  - unauthenticated;
  - forbidden;
  - controller unavailable;
  - TLS trust failure;
  - malformed controller response.
- Ensure token sentinels are absent from errors and worker logs.
- Remove package-level `http.Get`/`http.Post` controller calls.

## Required Context

Read first:

- `cmd/worker/main.go`
- `cmd/worker/state.go`
- `cmd/worker/config.go`
- worker tests
- OS-003 shared client
- OS-005 bootstrap contract

## Allowed Production Files

- `cmd/worker/main.go`
- `cmd/worker/state.go`
- `cmd/worker/config.go`
- `cmd/worker/controller_client.go`
- this concept README only for tracker/status updates

## Allowed Test Files

- `cmd/worker/main_test.go`
- `cmd/worker/state_test.go`
- `cmd/worker/config_test.go`
- `cmd/worker/controller_client_test.go`

## Out of Scope

- Changing work payload schemas.
- Retry/idempotency redesign.
- Per-worker identity issuance.
- Worker registration API.
- TLS certificate management.
- Deployment scripts.
- Workflow execution-secret changes.

## Acceptance Criteria

- Worker claims authenticated work through the shared client.
- Worker reports authenticated completion and failure.
- Wrong/missing credentials produce safe actionable failures.
- Token values never appear in worker logs or errors.
- Worker config token-file path is resolved correctly.
- No package-level `http.Get` or `http.Post` remains for controller communication.
- Existing work completion/failure payload tests remain stable.
- HTTPS test-server coverage proves standard TLS behavior without external
  networking.
- Loopback development can be explicitly unauthenticated where the controller
  permits it.

## Stop Conditions

Stop and append to `issues.md` if:

- a worker controller request exists outside the inspected worker package;
- shared-client error handling changes attempt semantics;
- a current redirect is required for normal controller operation;
- completion/failure would need retries to pass existing tests.
