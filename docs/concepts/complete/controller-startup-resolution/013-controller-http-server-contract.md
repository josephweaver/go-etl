# 013 Controller HTTP Server Contract

Status: implemented

## Objective

Resolve and validate the controller HTTP listen host, listen port, advertised
URL, timeout, request-size, and header-size variables through a caller-supplied
bounded startup resolver, then pass the normalized values into the later HTTP
server constructor.

## Required Context

Read these files first:

- `docs/epics/complete/controller-startup-resolution/README.md`
- `docs/epics/complete/controller-startup-resolution/012-controller-operational-policy-contracts.md`
- `cmd/controller/main.go`
- `cmd/controller/main_test.go`
- `cmd/controller/defaults.json`

Do not read unrelated files unless a targeted test failure directly requires it.

## Allowed Production Files

- `cmd/controller/main.go`

Implementation remains limited to one production file per prompt.

## Allowed Test Files

- `cmd/controller/main_test.go`

## Out Of Scope

- Changing HTTP routes, handlers, or response bodies
- Binding the HTTP listener or changing readiness semantics
- Resolving database, filesystem, policy, or execution-environment settings
- Constructing or retaining a long-lived resolver
- Introducing a duplicate aggregate controller runtime-config object
- Adding recovery-mode admission, caretaker handoff, or worker-start behavior
- Adding new HTTP configuration variables beyond the agreed startup contract

## Acceptance Criteria

- A startup consumer resolves the agreed HTTP variables from the standard
  precedence model, using the caller-supplied bounded resolver.
- The consumer covers the following values:
  - `controller_listen_host`
  - `controller_listen_port`
  - `controller_url`
  - `controller_read_header_timeout_milliseconds`
  - `controller_read_timeout_milliseconds`
  - `controller_write_timeout_milliseconds`
  - `controller_idle_timeout_milliseconds`
  - `controller_shutdown_timeout_milliseconds`
  - `controller_max_request_bytes`
  - `controller_max_header_bytes`
- `controller_listen_host` and `controller_url` are validated as non-empty
  string values.
- `controller_listen_port` is validated as an integer that can be used as an
  HTTP port.
- Timeout values are validated as positive integers.
- Request-size and header-size values are validated as positive integers.
- Missing, wrong-type, empty, or invalid values return errors that identify
  the HTTP consumer and the affected variable.
- Host, port, and advertised URL are resolved independently; the consumer does
  not infer one from another.
- Resolution returns one small value containing the normalized HTTP inputs for
  use by later constructors without becoming a second serialized configuration
  authority.
- Live startup resolves the HTTP contract only after the earlier startup
  contracts are ready and before the HTTP server is constructed.
- The bounded startup resolver is not retained after HTTP resolution.
- Targeted controller HTTP-contract tests pass.

## Notes

- This slice should stay at the settings-resolution boundary and not drift into
- listener lifecycle, handler wiring, or readiness policy.
- The later implementation prompt should be able to work from one production
  file at a time, with tests expanded only as needed for the same slice.
