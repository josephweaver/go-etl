# Sensitive Variable Propagation

Status: Proposed

## Purpose

Add a sensitivity-aware variable and execution-boundary contract to GOET so secrets and protected values can participate in workflow configuration without becoming ordinary controller data, database state, status output, logs, fingerprints, command-line arguments, or accidental provenance.

The main architectural point is simple:

```text
Controller-side planning sees protected references.
Worker-side execution may materialize sensitive values.
Trusted Go operations receive typed sensitive values.
User-defined subprocesses receive only deliberate, minimal materializations.
```

This concept does not pretend that just-in-time materialization makes secrets magically safe. It defines where plaintext may exist, who is allowed to see it, how it must be represented, and which boundaries are intentionally weaker.

## Goals

- Preserve sensitivity metadata through typed variable resolution.
- Represent protected values as references until an authorized worker execution boundary.
- Keep plaintext sensitive values out of controller persistence, status, diagnostics, provenance, and fingerprints.
- Avoid treating `sensitive` as a new value type; it is metadata on typed values and references.
- Support worker-local secret lookup for the first implementation, especially `worker_env` references.
- Let trusted in-process Go work-item handlers receive sensitive values as typed in-memory values, not command-line text.
- Let user-defined subprocess work items receive secrets only through explicit materialization surfaces such as environment variables, stdin, or protected temporary files.
- Ensure GOET-controlled logs, events, captured stdout/stderr, and failure messages redact known materialized sensitive values before persistence or transmission.
- Detect and reject structured work outputs that contain exact materialized sensitive values before those outputs become dependency inputs.
- Make the limitations explicit: arbitrary user code can still leak secrets through files, network requests, transformed output, hashes, encodings, screenshots, external logs, or artifact content.
- Keep `internal/variable.Resolver` deterministic and in-memory; secret lookup remains an explicit worker/provider operation outside normal variable resolution.

## Non-Goals

- Selecting or implementing Vault, AWS Secrets Manager, GCP Secret Manager, Kubernetes Secrets, or another external secret manager in phase 1.
- Making the controller a general encrypted secret store in phase 1.
- Automatically copying client-machine environment variables to remote workers.
- Guaranteeing protection against malicious user-authored Python, R, Bash, or other subprocess code.
- Scanning every artifact, file, directory, network request, or external service log for leaked secrets.
- Adding arbitrary environment access to workflows.
- Allowing sensitive values in subprocess command-line arguments.
- Redesigning variable precedence, expression grammar, fan-out rules, scheduling, retry, or persistence unrelated to sensitive values.
- Defining multi-level classification beyond public versus sensitive.

## Core Design

### 1. Three secret forms

GOET should distinguish three different forms that are easy to confuse:

| Form | Example | Plaintext? | Where it may appear |
|---|---|---:|---|
| Protected reference | `worker_env:GOET_GDRIVE_TOKEN` | No | Controller snapshots, work assignment, provenance labels |
| Typed sensitive value | in-memory `SensitiveValue` | Yes | Worker core and trusted Go work-item handlers only |
| Subprocess materialization | env var, temp file, stdin | Yes | Attempt-local runtime surface for user code |

A protected reference is not the secret. It is a durable, non-secret instruction for how an authorized worker may obtain the secret later.

### 2. Trust boundaries

| Boundary | Trust level | Rule |
|---|---|---|
| Client declaration | mixed | May name a protected value, but should not imply remote secret transfer unless explicitly using a protected store. |
| Controller | trusted for orchestration, not secret custody | Stores references and sensitivity metadata, not plaintext execution secrets. |
| Worker core | trusted execution boundary | May resolve protected references into typed sensitive values. |
| Trusted Go work-item handler | trusted code | May receive typed sensitive values through an execution context. |
| User subprocess | untrusted or user-trusted only | Receives only deliberate materializations; GOET can redact observable outputs but cannot prevent exfiltration. |
| Artifact content | outside phase-1 protection | Not scanned by default. |

The controller is not a good place to materialize an execution secret. If the controller resolves a secret and passes it to a worker, the system has already crossed the secret boundary before the worker has a concrete need for the value.

### 3. Preferred phase-1 secret source: worker-local references

The first implementation should prefer worker-local protected references:

```json
{
  "name": "gdrive_token",
  "type": "string",
  "sensitive": true,
  "protected_ref": {
    "provider": "worker_env",
    "key": "GOET_GDRIVE_TOKEN"
  }
}
```

Controller behavior:

```text
- validate that the declaration is well-formed;
- preserve type, sensitivity, provider, key, and redaction label;
- use only the non-secret reference for scheduling, status, and provenance;
- never read GOET_GDRIVE_TOKEN itself.
```

Worker behavior:

```text
- receive the protected reference;
- resolve worker_env:GOET_GDRIVE_TOKEN only when the assigned operation requires it;
- keep the plaintext in a bounded in-memory SensitiveValue or deliberate subprocess materialization;
- register the materialized plaintext with an attempt-local redactor;
- cleanup temporary materialization after execution.
```

This avoids the weak pattern where the client sends a secret to the controller and the controller forwards it to the worker. That pattern may be supported later through an explicit encrypted protected-value store, but it is not the default phase-1 contract.

### 4. Client environment is not automatically portable

A client-machine environment variable is local to the client process. For remote workers, it is not safely or naturally available.

Therefore:

```text
${client_env.DB_PASSWORD}
```

must not silently become:

```text
plaintext secret copied through controller persistence or worker assignment
```

Acceptable policies:

1. Use `worker_env` for execution secrets that are pre-provisioned on the worker host, worker container, or HPCC job environment.
2. Use controller environment only for controller-owned capabilities that are not forwarded to work items.
3. Use a future encrypted protected-value store for client-submitted secrets that must survive after the client process exits.
4. Reject remote execution of client-only environment secrets unless the user explicitly selects a supported protected transport or store.

### 5. Execution envelope

Workers should receive a structured execution envelope, not a flat map of resolved variables.

Example:

```json
{
  "schema": "goet/execution-envelope/v1",
  "work_item": {
    "id": "download-private-drive-fixture",
    "type": "python_script"
  },
  "variables": {
    "public": {
      "year": {
        "type": "int",
        "value": 2026
      }
    },
    "protected_refs": {
      "gdrive_token": {
        "type": "string",
        "provider": "worker_env",
        "key": "GOET_GDRIVE_TOKEN",
        "redaction_label": "${worker_env.GOET_GDRIVE_TOKEN}",
        "materialize": {
          "mode": "env",
          "target": "GDRIVE_TOKEN"
        }
      }
    }
  }
}
```

The controller may persist this envelope because it contains references, types, labels, and materialization instructions, not the secret values themselves.

### 6. Trusted Go operation contract

Trusted Go work-item handlers are in-process worker code. They should receive a typed operation context:

```go
type OperationContext struct {
    WorkItem  WorkItemSpec
    Public    map[string]ResolvedValue
    Sensitive map[string]SensitiveValue
    Logger    SafeLogger
}
```

`SensitiveValue` should avoid accidental formatting. Its default string, JSON, and error representations must be redacted. Access to plaintext should require an explicit method call with a narrow use site.

This does not make a malicious Go handler safe. It prevents accidental leaks in trusted worker code and makes sensitive handling visible in code review.

### 7. User subprocess contract

User-defined scripts are a weaker boundary. Once GOET passes a secret to arbitrary Python, R, Bash, or another subprocess, GOET cannot prove that the script will not leak it.

Rules for subprocess work items:

- Never pass sensitive values as command-line arguments.
- Materialize only variables declared as required by the work item.
- Prefer protected temporary files for larger credentials such as JSON service-account files.
- Use environment variables only when the tool contract expects them and the risk is acceptable.
- Set restrictive permissions on temporary secret files where the platform supports it.
- Register every materialized plaintext value with the attempt-local redactor.
- Scrub captured stdout/stderr before persistence or status transmission.
- Reject structured output JSON if it contains exact materialized sensitive values.
- Delete temporary secret files after execution.
- Document that transformed leaks, encoded leaks, network exfiltration, and artifact leaks are outside the protection guarantee.

### 8. Redaction contract

Safe rendering is the first line of defense. GOET-controlled formatting of sensitive references and sensitive values should render a redaction label rather than plaintext.

Attempt-local redaction is the second line of defense. When a worker materializes a secret, it registers the exact plaintext value with a redactor for the duration of the attempt.

The redactor:

- treats secrets as literal strings or byte sequences, not regular expressions;
- never persists its registry;
- only registers values materialized for the current operation;
- scrubs captured stdout/stderr, worker failure messages, GOET-controlled events, and controlled logs;
- should not enumerate entire environments or secret stores;
- should not claim to catch split, encoded, hashed, truncated, or transformed values.

Structured outputs that contain exact materialized sensitive values should fail validation rather than silently becoming dependency inputs.

### 9. Fingerprints and reuse

Sensitive plaintext must not be included in fingerprints.

Phase-1 fingerprints may include:

- non-secret protected reference identity;
- provider name;
- redaction label;
- optional declared secret version if the provider exposes one without revealing plaintext.

They must not include:

- plaintext;
- reversible encryption of plaintext;
- ordinary unsalted hashes of plaintext;
- captured environment dumps.

If secret rotation should invalidate reuse in the future, that should be handled through explicit provider version metadata, not by fingerprinting the secret value itself.

## Relationship to Other Concepts

- `internal/variable` owns typed variables, namespaces, references, resolved values, and recursive resolution. Sensitivity metadata starts there, but plaintext lookup does not.
- `workflow-compilation-resolution` should consume protected references when compiling resolved work-item inputs.
- `workflow-execution-persistence` should persist protected references and safe snapshots, not plaintext values.
- `dependency-aware-workflows` should propagate sensitivity when downstream work consumes upstream values.
- `python-workitem` owns subprocess materialization and captured-output scrubbing for Python execution.
- `data-assets-and-materialized-outputs` depends on this concept before supporting credentialed data assets such as private Google Drive or object-store inputs.
- `execution-events` and `logging` should route only redacted representations of sensitive data.
- `resource-constraint` may eventually gate credentialed provider calls or shared network/download pressure, but it does not own secret handling.

## Proposed Slices

1. `001-sensitive-metadata-and-safe-rendering.md` — add sensitivity metadata, propagation, and safe rendering to typed variables/resolved values.
2. `002-protected-reference-model.md` — add protected-reference declarations and validation without plaintext lookup.
3. `003-controller-envelope-and-persistence.md` — preserve public values and protected references through controller work compilation, assignment payloads, status, and snapshots.
4. `004-worker-secret-resolver-and-redactor.md` — add worker-local protected-value resolution plus attempt-local redaction primitives.
5. `005-trusted-go-workitem-sensitive-context.md` — pass typed sensitive values to trusted in-process Go work-item handlers through a safe context.
6. `006-python-subprocess-secret-materialization.md` — materialize sensitive values for Python subprocesses through explicit env/file/stdin surfaces, never command-line arguments.
7. `007-controlled-sink-redaction-tests.md` — add sentinel tests that verify logs, events, stdout/stderr, status, errors, and structured outputs do not persist exact secrets.
8. `008-credentialed-worker-fixture-smoke.md` — prove the boundary with a small worker-local credential fixture, avoiding real secret managers and large data.
9. `009-concept-closure-and-doc-sync.md` — update concept index, project state, and docs after the implementation slices land.

## Completion Criteria

- Variable declarations and resolved values can carry sensitivity metadata.
- Sensitivity propagates through references, structured values, accessors, and interpolation without accidental declassification.
- Protected references can be declared, validated, serialized, and persisted without plaintext.
- Controller snapshots, work assignment payloads, status output, and fingerprints do not contain plaintext sensitive values.
- Worker-local `worker_env` protected references can be resolved only by the worker execution boundary.
- Trusted Go work-item handlers receive sensitive values through a typed context and safe logger.
- User subprocesses receive only deliberate secret materializations and never command-line secret arguments.
- Captured stdout/stderr and GOET-controlled events/logs/failures are scrubbed for exact materialized secret values.
- Structured output JSON containing exact materialized secrets is rejected before persistence.
- Temporary secret files are cleaned up after execution.
- Tests include sentinel values and fail if those values appear in controlled persistence or status surfaces.
- Documentation clearly states that arbitrary user code can still leak secrets through channels GOET does not control.
