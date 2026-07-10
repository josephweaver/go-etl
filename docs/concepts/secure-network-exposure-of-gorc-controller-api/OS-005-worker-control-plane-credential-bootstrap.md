# OS-005: Worker Control-Plane Credential Bootstrap

Status: Proposed  
Minimum recommended model: GPT-5.5 with high reasoning  
Reference: EC-4 / operational slice / files(6)+tests+doc

## Objective

Define and implement how a worker obtains the bearer credential it needs before it
can claim its first assignment.

This is a control-plane bootstrap secret, not a workflow variable.

## Current State

Generated worker configuration contains `controller_url` but no credential source.

The controller runtime writes worker JSON into the execution environment. Placing a
token literal into that JSON would make the credential part of ordinary generated
configuration and increase the chance of persistence, logging, or accidental
commit.

## Target State

Generated worker JSON contains only a protected file reference:

```json
{
  "controller_url": "https://controller.example.org",
  "controller_token_file": "/data/goetl/secrets/controller-worker-token"
}
```

The referenced file is pre-provisioned on the worker side.

Supported phase-1 bootstrap modes:

```text
pre-provisioned file
container secret mount resolving to a file
```

A future keystore provider may materialize the same file or supply an equivalent
token provider without changing the worker HTTP contract.

## Strategic Decision

The first production-safe implementation should **not** copy a token literal
through generated worker JSON, Slurm arguments, or workflow variables.

Runtime preparation validates the configured secret path but does not place the
secret in ordinary artifacts.

For local fake-HPCC tests, test setup may create or mount the token file with
restrictive permissions.

For a real HPCC, the operator provisions the file in an account-private path before
launching workers.

## Candidate Runtime Configuration

```json
{
  "execution_environment": {
    "runtime": {
      "type": "worker",
      "settings": {
        "root": "/data/goetl",
        "controller_url": "https://controller.example.org",
        "controller_token_file": "/data/goetl/secrets/controller-worker-token"
      }
    }
  }
}
```

## Requirements

- Extend controller-side `WorkerRuntime` and generated `WorkerConfig` with a token
  file path.
- Extend worker `Config` with the same path.
- Resolve relative local paths safely where local mode supports them.
- Do not read the token in controller workflow compilation.
- Do not serialize the token value.
- Validate that external/authenticated controller URLs require a worker token
  source.
- Runtime preflight verifies:
  - path is configured;
  - file exists in the execution environment;
  - file is regular;
  - file is readable by the worker account;
  - file is non-empty and below a small maximum size;
  - permissions are restrictive where the platform exposes them.
- Preflight error messages name the path but never the file contents.
- Slurm scripts receive the worker config path only, not a token argument.
- Container examples mount a secret file read-only.
- The token path is not included in workflow fingerprints; it is deployment
  configuration.
- Sentinel token contents do not appear in controller logs, generated JSON,
  command lines, or SQLite.

## Required Context

Read first:

- `cmd/controller/runtime.go`
- `cmd/controller/runtime_test.go`
- `cmd/controller/execution_environment.go`
- `cmd/controller/preparer.go`
- `cmd/controller/slurm_worker_script.go`
- `cmd/worker/config.go`
- `docs/concepts/sensitive-variable-propagation/README.md`
- implemented data/runtime preparation tests
- OS-001 through OS-004

## Allowed Production Files

- `cmd/controller/runtime.go`
- `cmd/controller/runtime_test.go` only if repository convention co-locates small
  helpers there; otherwise use test files below
- current execution-environment/preflight file
- `cmd/worker/config.go`
- container/fake-HPCC worker configuration templates
- this concept README only for tracker/status updates

A small shared path-validation helper may be added if needed.

## Allowed Test Files

- `cmd/controller/runtime_test.go`
- current execution-environment/preflight tests
- `cmd/worker/config_test.go`
- fake-HPCC fixture secret files generated at test runtime only

Do not commit a realistic bearer token. Use an unmistakable sentinel generated or
written by the test.

## Out of Scope

- External secret-manager implementation.
- Copying a client secret through the controller.
- Token rotation API.
- Per-worker unique credentials.
- Short-lived signed worker credentials.
- OIDC workload identity.
- Changing workflow sensitive-variable semantics.
- Automatic remote secret installation for real HPCC.

## Acceptance Criteria

- Generated worker JSON contains `controller_token_file`.
- Generated worker JSON never contains token material.
- Missing token-file configuration fails authenticated remote worker preparation.
- Missing/unreadable/empty/oversized token files fail preflight safely.
- Local and fake-HPCC tests can provision a restrictive token file.
- Slurm command text and scripts contain no token literal.
- Controller status, logs, and SQLite contain no token sentinel.
- Existing unauthenticated loopback-only worker fixtures can remain only where
  explicitly configured for local development.
- Current local, Docker, SSH, Slurm, and Singularity runtime tests continue to
  compile.

## Stop Conditions

Stop and append to `issues.md` if:

- a target execution environment cannot provide a protected readable file;
- the token would have to appear in `sbatch` arguments or environment dumps;
- runtime preparation cannot verify the path from the actual worker context;
- the implementation starts treating a control-plane token as a workflow input;
- a real HPCC requires a site-specific secret distribution method not represented
  by the provider-neutral contract.
