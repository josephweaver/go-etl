# Fingerprinting Strategy

Status: Draft

## Purpose

This document defines GOET's fingerprinting strategy. Fingerprints provide stable
semantic identities for configuration, workflow definitions, compiled work,
assignments, results, and external state. They support reproducibility,
re-execution, caching, prior-work lookup, restart, and audit.

The central rule is:

> Fingerprints identify semantic content, not merely where that content was
> found.

A GitHub repository, commit SHA, and path are source locators. They identify one
known-valid place from which GOET loaded content. They are important for reload,
restart, and audit, but they are not the primary semantic identity of project or
workflow configuration.

The semantic identity of a project or workflow document is its canonical content
hash.

## Design Principles

- Fingerprints are deterministic.
- Fingerprints are content-addressed.
- Fingerprints are based on canonical representations.
- Fingerprints identify semantics, not incidental storage location.
- Fingerprints compose from smaller semantic identities.
- Source locators are recorded separately from semantic fingerprints.
- Debug data, provenance output, timestamps, controller uptime, and resolver
  internals do not participate in semantic fingerprints unless explicitly part of
  the artifact's meaning.
- Secret plaintext never participates directly in a persisted fingerprint.
  Secret references may participate where they affect semantics, but the secret
  value itself must not be stored or recoverable from fingerprint material.

## Canonicalization

Fingerprints must be computed from canonical data, not arbitrary in-memory or
file-system representations.

For JSON documents, canonicalization should include:

- stable object-key ordering;
- stable string encoding;
- no insignificant whitespace;
- deterministic number formatting;
- explicit treatment of null versus missing values;
- omission of non-semantic metadata where the owning schema declares it
  non-semantic.

Unless a stronger requirement is introduced later, GOET fingerprints should use
SHA-256 over the canonical byte representation.

Canonicalization is part of the fingerprint contract. If canonicalization changes,
the schema or fingerprint algorithm version must change so older fingerprints
remain explainable.

## Source Locators Versus Semantic Identity

GOET records source locators so it can reload or audit source documents.

Typical source locator fields:

- repository identity;
- resolved commit SHA;
- repository-relative path;
- optional branch or tag originally requested;
- optional retrieval timestamp.

These fields answer:

```text
Where did GOET obtain a known-valid copy?
```

Semantic fingerprints answer:

```text
What content did GOET use?
```

Many Git commits may contain the same `project.json` or `workflow.json` content.
Those commits have different source locators but the same semantic fingerprint.

## Project Fingerprint

A project fingerprint is the canonical SHA-256 hash of `project.json` after
schema-defined canonicalization.

```text
project_fingerprint = sha256(canonical(project.json))
```

The project fingerprint identifies the semantic project configuration. The
GitHub repository, commit SHA, and path identify one known-valid source location
for that configuration.

## Workflow Fingerprint

A workflow fingerprint is the canonical SHA-256 hash of `workflow.json` after
schema-defined canonicalization.

```text
workflow_fingerprint = sha256(canonical(workflow.json))
```

The workflow fingerprint identifies the semantic workflow definition. The GitHub
repository, commit SHA, and path identify one known-valid source location for
that workflow definition.

A normalized workflow representation may be derived from `workflow.json`, but it
is not a separate source of truth unless a later design explicitly promotes it to
one. If a normalized representation is hashed, it must either be proven
canonical-equivalent to the workflow JSON or carry its own schema and algorithm
version.

## Controller and Plugin Versions

Controller and plugin versions may affect compilation or execution behavior.
Unlike project/workflow Git source locators, code versions can participate in
semantic identity.

Controller version may be represented by:

- controller Git commit SHA;
- release version;
- build digest;
- another immutable code-version identifier.

Plugin version may be represented by:

- plugin Git commit SHA;
- package version;
- container image digest;
- binary digest;
- another immutable plugin-version identifier.

If a controller or plugin version can change behavior, it should participate in
work-item or result fingerprints as appropriate.

## Work-Item Fingerprint

A work-item fingerprint identifies a logical unit of work as compiled by the
controller.

A work-item fingerprint should include, as applicable:

- fingerprint schema version;
- controller version;
- plugin version;
- project fingerprint;
- workflow fingerprint;
- workflow run identity or stable run-basis hash when required by semantics;
- stage or step identity;
- fan-out index or work-item index;
- resolved non-secret input-variable hash;
- declared or resolved output-variable hash where applicable;
- plugin-defined pre-state hash for external file or system state;
- approved secret references where the selected secret identity affects
  semantics, but not plaintext secret values.

Conceptually:

```text
work_item_fingerprint = sha256(canonical({
  schema_version,
  controller_version,
  plugin_version,
  project_fingerprint,
  workflow_fingerprint,
  run_basis,
  stage_or_step_identity,
  work_item_index,
  resolved_input_variables_hash,
  output_variable_contract_hash,
  pre_state_hash,
  secret_reference_identities
}))
```

Retries reuse the same logical work-item fingerprint. A retry creates a new
attempt, not a new logical work item.

## Assignment Fingerprint

An assignment fingerprint identifies the binding between a logical work item and
a concrete worker assignment.

Assignment-time resolution may depend on heterogeneous worker facts, such as
worker capabilities, local paths, scratch directories, GPUs, mounted file systems,
lease identity, or execution-environment details.

An assignment fingerprint should include, as applicable:

- fingerprint schema version;
- work-item fingerprint;
- assignment or attempt identity when that identity affects assignment-scoped
  values;
- selected worker identity or worker capability hash;
- assignment-scoped runtime-value hash;
- worker-local binding hash;
- approved secret-reference identities required by the assignment;
- assignment-resolution policy version.

Plaintext secrets must not participate in persisted assignment fingerprints.
Secret references may participate if they affect which credential class or
external identity was selected.

## Output Variable Hash

An output-variable hash identifies the canonical logical output JSON produced by
a work item.

```text
output_variables_hash = sha256(canonical(output_json))
```

This hash covers logical outputs reported by the worker. It does not necessarily
cover external state such as files, cloud objects, database rows, or object-store
metadata unless that state is represented in the canonical output JSON or a
plugin-defined state hash.

## External State Fingerprints

External state fingerprints describe file-system, database, cloud-object, or
other outside-world state observed before or after execution.

The plugin that owns an external operation defines the state fingerprint domain.
Examples include:

- file SHA-256;
- directory manifest hash;
- cloud object generation ID plus object hash;
- database snapshot ID;
- table checksum;
- raster metadata and content hash;
- model artifact manifest hash.

GOET distinguishes:

```text
pre_state_hash  = external state observed before execution
post_state_hash = external state observed after successful execution
```

A later pre-state matching a prior post-state may allow GOET to determine that a
requested external state already exists.

## Result Fingerprint

A result fingerprint identifies the observed result of a logical work item.

A result fingerprint should include, as applicable:

- fingerprint schema version;
- work-item fingerprint;
- output-variable hash;
- post-state hash;
- result policy version.

Conceptually:

```text
result_fingerprint = sha256(canonical({
  schema_version,
  work_item_fingerprint,
  output_variables_hash,
  post_state_hash,
  result_policy_version
}))
```

The work-item fingerprint identifies what was requested. The result fingerprint
identifies what was produced.

## Relationship to Provenance

Provenance explains how a value was resolved. Fingerprints identify canonical
semantic content.

Provenance is reconstructable from canonical documents and runtime records.
Normal workflow persistence does not need to store full provenance documents in
order to compute fingerprints.

Fingerprints should not include provenance output, debug traces, resolver
internals, or explanatory resolution paths. They should include the canonical
semantic inputs that determine the artifact being fingerprinted.

## Relationship to Source Control

GitHub remains important as a source-of-truth transport and audit mechanism.
GOET should record repository, commit SHA, and path for project and workflow
configuration so the exact source document can be reloaded when available.

However, Git commit SHA is not the same as project or workflow semantic identity.
Two commits may contain identical project/workflow documents. Those documents
should have the same project or workflow fingerprint.

For controller and plugin code, Git commit SHA may be a semantic version input
because code changes may change compilation or execution behavior.

## Relationship to Persistence

Persistence records fingerprints beside the entities they identify. Examples:

- `projects.config_sha256` records the canonical project configuration hash.
- `workflows.workflow_sha256` records the canonical workflow configuration hash.
- `work_items.resolved_inputs_sha256` records the canonical resolved input hash.
- `completed_work.output_json_sha256` records the canonical logical output hash.
- `completed_work.pre_state_sha256` records the plugin-defined pre-state hash.
- `completed_work.post_state_sha256` records the plugin-defined post-state hash.

The database may evolve, but the responsibility split remains:

```text
source locator      = where GOET found content
semantic fingerprint = what content or behavior GOET used
runtime record       = when and how GOET used it
```

## Open Questions

- What exact canonical JSON rules should GOET adopt?
- Should fingerprint algorithm versions be embedded in every fingerprinted
  artifact or carried by schema version?
- Which non-semantic metadata fields must be ignored for each document type?
- How should secret reference identities be represented without leaking secret
  material?
- Should assignment fingerprints be required in the first implementation or added
  after assignment-time resolution is implemented?
- Should external-state fingerprint domains be centrally registered or fully
  plugin-defined?
