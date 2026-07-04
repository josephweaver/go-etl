# Sensitive Variable Metadata and Propagation Epic

Status: Proposed

## Purpose

Add sensitivity metadata to GOET's standard typed-variable model and preserve
that sensitivity through resolution so secrets and other protected values can
participate in controller, project, workflow, and runtime configuration without
being exposed through persistence, diagnostics, logs, fingerprints, or public
APIs.

## Goals

- Allow a standard variable declaration to mark its value as sensitive.
- Preserve sensitivity on resolved scalar, object, and list values.
- Propagate sensitivity from referenced values through recursive resolution,
  accessors, and interpolation.
- Prevent an explicitly non-sensitive destination from declassifying a
  sensitive dependency.
- Redact sensitive values from errors, logs, status output, provenance, and
  diagnostic representations.
- Make safe rendering the default for resolved sensitive values and require an
  explicit operation to obtain plaintext at an authorized execution boundary.
- Sanitize GOET-controlled structured events and captured subprocess output
  before they are persisted or transmitted.
- Exclude sensitive material from fingerprints while retaining enough
  non-secret identity to explain which source or protected reference was used.
- Define how sensitive values cross client, controller, workflow-run,
  work-item, worker, and attempt boundaries.
- Define a durable protected-value/reference contract for values needed after
  their original environment or client process is unavailable.
- Preserve the deterministic, in-memory responsibility of
  `internal/variable.Resolver`; secret lookup, encryption, and decryption remain
  explicit operations outside the resolver.
- Keep the serialized declaration language-neutral for JSON, CLI, REST, and
  future Python and R adapters.

## Non-Goals

- Selecting or implementing a specific external secret manager in this
  placeholder.
- Adding arbitrary environment-variable access to workflows.
- Treating `sensitive` as a new variable value type.
- Allowing sensitivity metadata to bypass namespace or environment-key access
  policy.
- Defining general data-classification levels beyond sensitive/non-sensitive.
- Redesigning variable precedence or expression grammar.
- Implementing the epic before its storage, transport, and redaction contracts
  are agreed.
- Inspecting every file, network request, or external log produced by arbitrary
  plugin or subprocess code.
- Treating artifact content scanning as the primary sensitive-variable
  protection mechanism; optional data-loss-prevention policy may be designed
  separately.

## Architectural Context

Sensitivity is metadata on the existing typed-variable and resolved-value
model. A declaration may conceptually contain:

```json
{
  "name": {
    "namespace": "project_config",
    "key": "postgres_password"
  },
  "type": "string",
  "expression": "${client_env.DB_PASSWORD}",
  "sensitive": true
}
```

The referenced environment value is captured according to namespace and
environment access policy. Sensitivity follows the value through resolution;
it is not removed by assigning the value to another variable.

The capability primarily belongs to `internal/variable`, which owns variable
and resolved-value shapes and resolution behavior. Enforcement also crosses
consumer boundaries:

- clients and HTTP handlers must transport sensitive submission values safely;
- controller diagnostics and logging must redact them;
- workflow-run persistence must store an opaque protected reference rather
  than plaintext when later resolution requires the value;
- work-item and worker contracts must expose only the sensitive values needed
  by the assigned operation;
- attempt snapshots and fingerprints must not leak sensitive material.

The controller internal data-model draft defines the create-use-discard
resolver lifecycle and the need to reconstruct later resolvers without
persisting plaintext secrets. This epic supplies the sensitivity contract that
workflow execution persistence and dependency-aware compilation must consume.

Relevant documents and packages include:

- `docs/controller.internal.datamodel.md`
- `docs/ARCHITECTURE_OVERVIEW.md`
- `docs/CUSTOMER_API.md`
- `docs/concepts/workflow-execution-persistence/README.md`
- `docs/concepts/dependency-aware-workflows/README.md`
- `internal/variable`

## Proposed Slices

No implementation slices are agreed yet. Slice decomposition should begin only
after the open questions below are resolved and the epic is explicitly moved
from `Proposed` to `Ready`.

## Agreed Decisions

- A resolved value carries one typed plaintext value together with sensitivity
  metadata, a non-secret redaction label, and non-secret provenance. It does
  not retain separate real and redacted copies of the value.
- Normal formatting, diagnostics, and serialization of a sensitive resolved
  value produce a safe representation such as
  `${controller_env.DB_PASSWORD}`. Obtaining plaintext requires an explicit
  operation at an authorized consumption boundary.
- Sensitivity propagates with resolved values from client to controller, into
  durable protected references, and from controller to the worker or plugin
  operation that requires the value. Operations that do not require a
  sensitive value must not receive it.
- Sensitive values must not be placed in subprocess command arguments. An
  executor should inject plaintext through a narrower mechanism such as the
  subprocess environment, standard input, or a protected temporary file,
  according to the plugin contract.
- Sensitivity-aware formatting is the primary protection against GOET
  intentionally logging a secret. As defense in depth, every GOET-controlled
  log and event sink, including captured subprocess standard output and
  standard error, sanitizes exact occurrences of materialized sensitive values
  before persistence or transmission. Secret text is treated as a literal,
  not as a regular expression.
- A sanitizer registers only sensitive values materialized for the bounded
  operation. It must not enumerate an environment namespace or persist its
  plaintext registry.
- Sanitization cannot guarantee removal of split, encoded, hashed, or otherwise
  transformed secrets. A plugin remains responsible for plaintext after it
  explicitly obtains it and sends it to a file, network service, or logging
  path outside GOET-controlled sinks.
- Scanning every plugin-created file for known secret strings is not required.
  It is incomplete, occurs after plaintext has been written, and can be costly
  or produce false positives for legitimate artifact content.

## Open Questions

1. Is sensitivity stored only on root `Variable` and `ResolvedValue` objects,
   or may each nested typed-expression node declare sensitivity independently?
2. What exact propagation rules apply to object fields, list items, accessors,
   and string/path interpolation?
3. Which qualified variable name or opaque protected identity should supply the
   redaction label when a sensitive value is derived from multiple inputs?
4. How does a client safely transmit a sensitive `client_env` value to a remote
   controller?
5. Which protected-value store backs values required by later workflow steps,
   and what opaque reference is persisted in the run snapshot?
6. May fingerprints include a keyed, non-reversible digest of sensitive input,
   or must sensitive inputs be excluded entirely and represented by separate
   protected identity/version metadata?
7. How are sensitive values scoped and removed from worker assignment payloads
   when an operation does not require them?
8. Which test helpers guarantee that errors, logs, JSON, database records, and
   HTTP responses do not contain known sentinel secrets?

## Completion Criteria

- The language-neutral variable schema has an agreed sensitivity field and
  compatibility policy.
- Sensitivity propagation rules are documented and tested for scalar,
  structured, referenced, accessed, and interpolated values.
- Sensitive dependencies cannot be accidentally declassified.
- Diagnostics, logging, status, provenance, and serialization follow one
  tested redaction contract.
- Sensitive material is not stored as plaintext in workflow-run, work-item, or
  attempt persistence.
- Later resolver construction can obtain an authorized protected value without
  making `variable.Resolver` perform secret-store I/O.
- Client-to-controller and controller-to-worker sensitive transport boundaries
  are explicit and tested.
- Fingerprinting and reuse behavior for sensitive inputs is explicitly agreed
  and tested.
- All agreed implementation slices are complete and relevant tests pass.
