# ZIP Manifest

This archive contains one Strategic Concept, decision/reference material, and twenty ordered Operational Slices for the canonical workflow/data/configuration document boundary.

## Intended Repository Location

```text
docs/concepts/canonical-workflow-data-document-model/
```

## Core Files

- `README.md` — Strategic Concept.
- `DECISIONS.md` — accepted design decisions.
- `REFERENCE_EXAMPLES.md` — target JSON/YAML examples.
- `IMPLEMENTATION_ORDER.md` — dependency graph and integration gates.
- `MODEL_ASSIGNMENTS.md` — minimum model guidance.
- `MANIFEST.md` — this file.

## Operational Slices

- `001-canonical-public-document-contracts.md` — 001 Canonical Public Document Contracts.
- `002-json-yaml-source-decoder.md` — 002 JSON YAML Source Decoder.
- `003-canonical-typed-variable-loader.md` — 003 Canonical Typed Variable Loader.
- `004-workflow-document-normalization.md` — 004 Workflow Document Normalization.
- `005-named-data-tree-overlay-and-precedence.md` — 005 Named Data Tree Overlay and Precedence.
- `006-data-asset-definition-binding-and-selection.md` — 006 Data Asset Definition Binding and Selection.
- `007-parameterized-asset-instantiation.md` — 007 Parameterized Asset Instantiation.
- `008-materialization-scope-and-shared-domain.md` — 008 Materialization Scope and Shared Domain.
- `009-explicit-cache-data-step.md` — 009 Explicit Cache Data Step.
- `010-shared-materialization-hydration.md` — 010 Shared Materialization Hydration.
- `011-step-data-projections-and-worker-resolution.md` — 011 Step Data Projections and Worker Resolution.
- `012-explicit-commit-data-step.md` — 012 Explicit Commit Data Step.
- `013-workflow-migration-and-equivalence-smoke.md` — 013 Workflow Migration and Equivalence Smoke.
- `014-structured-function-call-model-and-loader-directives.md` — 014 Structured Function Call Model and Loader Directives.
- `015-resolver-jit-function-evaluation.md` — 015 Resolver JIT Function Evaluation.
- `016-list-crossproduct-function.md` — 016 List Crossproduct Function.
- `017-list-zip-function.md` — 017 List Zip Function.
- `018-list-flatten-function.md` — 018 List Flatten Function.
- `019-list-length-function.md` — 019 List Length Function.
- `020-function-produced-fanout-proof.md` — 020 Function Produced Fanout Proof.

## Source Review Inputs

- current `go-etl` repository main branch files cited in the SC;
- the proposed expression-function-framework archive;
- the Canonical JSON Variable Loading placeholder README;
- `gdrive-field-boundaries-cache-smoke.json`;
- `gdrive-release-data-h18v07-cache-smoke.json`;
- `gdrive-publish-archive-smoke.json`.
