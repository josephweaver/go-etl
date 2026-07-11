# Canonical Workflow, Data, and Configuration Document Model

Status: Proposed  
Cadence: Strategic Concept with ordered Operational Slices  
Target repository: `josephweaver/go-etl`  
Reviewed against repository `main`: 2026-07-11

## Purpose

Define one canonical public document boundary for GOET/GORC controller, project, workflow, and override inputs.

The boundary must:

- accept JSON and a strict YAML subset;
- normalize both encodings into one JSON-compatible semantic model;
- clean up the public workflow shape instead of exposing internal Go structures;
- load ordinary JSON/YAML values into the typed variable system with implicit source namespaces;
- compose named project, workflow, and submission data trees with workflow-over-project precedence;
- move data acquisition and publication declarations out of compute work-item parameters;
- support parameterized data assets instantiated after fan-out values are known;
- keep `cache_data` and `commit_data` as explicit authored work items rather than hidden generated work;
- resolve materialized paths through a step-local `data` namespace;
- retain `materialization.scope: shared | worker`, implementing `shared` first and rejecting `worker` with a recognized not-implemented error;
- normalize structured function-call authoring syntax into the internal expression model without teaching `internal/variable` about `$expr` JSON containers.

## Strategic Decision

The canonical boundary is:

```text
JSON source ─┐
             ├─> source decoder ─> canonical document normalizer
YAML source ─┘                         |
                                        v
                          canonical controller/project/workflow
                                        |
                                        v
                     typed variables + effective named data tree
                                        |
                                        v
                       workflow compilation and explicit work
                                        |
                                        v
                   assignment-time data-path finalization
```

JSON and YAML are source encodings. They are not separate configuration systems.

## Superseded or Refined Planning

This SC supersedes or narrows the following proposed directions:

1. The placeholder **Canonical JSON Variable Loading** concept is absorbed and expanded to cover YAML, workflow normalization, named data trees, and assignment-time data projections.
2. `expression-function-framework/001-expression-container-forms.md` is retired. `$expr` and `type: expression` must not become part of `internal/variable.TypedExpression` JSON decoding.
3. The remaining expression-function idea is retained through a structured semantic function-call model and canonical-loader directives.
4. The implemented data-operator runtime remains valuable, but the current planner behavior that discovers `data_assets` or `publish` inside compute parameters and silently generates operator work is a legacy authoring path. The target model requires visible `cache_data` and `commit_data` steps.

## Current Repository State

The repository already has most of the low-level runtime pieces:

- `internal/variable` provides typed values, namespaces, references, accessors, recursive resolution, and sensitivity propagation.
- `cmd/controller/main.go` contains `projectVariablesFromJSON` and `typedExpressionFromJSON`, an early implementation of canonical literal loading for inline projects.
- `workflow.Workflow` exposes internal `[]variable.Variable` and Go-shaped step fields directly to current workflow JSON.
- controller startup documents still require explicit serialized `[]variable.Variable` entries.
- `internal/model/data_asset.go` defines step bindings, bound assets, cache/integrity/archive/materialization fields, and materialized-data manifests.
- `internal/workflow/cache_data_plan.go` currently scans compute parameters and generates `cache_data` and `commit_data` work items.
- `cmd/controller/cache_data_hydration.go` hydrates compute work from completed dependency manifests.
- worker code already performs data acquisition, archive extraction, cache promotion, Python argument binding, and publication.

The missing boundary is not provider execution. It is a coherent public document and ownership model.

## Core Invariants

### One semantic model

Equivalent JSON and YAML documents normalize to the same semantic tree and semantic fingerprint.

### Public documents do not serialize internal Go types

Authors should not need to provide:

```json
{
  "name": {"namespace": "workflow", "key": "tiles"},
  "type": "list",
  "expression": []
}
```

for ordinary values. Public documents use ordinary values:

```yaml
variables:
  tiles:
    - h18v07
    - h18v08
```

### Source determines variable namespace

Only variable-bearing sections receive implicit namespaces:

| Source | Variable-bearing section | Internal namespace |
|---|---|---|
| `controller.json` / YAML equivalent | `variables` | `controller_config` |
| `project.json` / YAML equivalent | `variables` | `project_config` |
| `workflow.json` / YAML equivalent | `variables` | `workflow` |
| submission envelope | `overrides` | `override` |

Structural document fields such as workflow steps, project data bindings, controller execution environments, and source manifests are not automatically imported as variables.

### Workflow overrides project

Effective data composition order is:

```text
project.data
    < workflow.data
    < submission data overrides
    < step-local usage bindings
    < fan-out/work-item/assignment/runtime values
```

The canonical document normalizer must assemble scopes in this order explicitly. It must not rely on the current call order of `variable.NewSet` being accidentally correct.

### Named data trees overlay recursively

For named data trees:

```text
map + map         -> recursive overlay by key
scalar + scalar   -> later scalar replaces earlier scalar
list + list       -> later list replaces earlier list
omitted field     -> inherited
null              -> unsupported in phase 1
type mismatch     -> validation error
```

The overlay occurs at the canonical document boundary. The current variable resolver selects a complete root variable and does not deep-merge object nodes across namespaces.

### Selection narrows an asset

A project may define the general Yan-Roy asset with both raster and header roles. A workflow or step may select only `header` without deleting the project’s general file definitions.

```yaml
select:
  - header
```

Lists replace rather than concatenate, so workflow selection naturally overrides project defaults.

### Asset definition and instance are separate

An asset definition is reusable:

```text
yan_roy_field_segments(tile)
```

A concrete instance is definition plus resolved parameters and selection:

```text
yan_roy_field_segments[tile=h18v07;select=header]
```

The bracketed form is diagnostic text only. It is not executable `${...}` syntax.

### Asset instances may contain multiple named files

Yan-Roy is one logical asset with named file roles:

```text
raster
header
```

The effective `select` list defines which roles are materialized and their ordered projection.

### `data.<alias>.path` is an ordered list

A materialized step binding exposes:

```text
${data.field_segments.path[0]}
${data.field_segments.path[1]}
${data.field_segments.files.header.path}
```

`data.field_segments.path` is always a typed list of paths, including for a one-file selection. Order comes from the effective `select` list. Named file-role access remains available when positional access would be unclear.

### No hidden work items

The compiler must not silently introduce data movement because a compute parameter happens to contain `data_assets` or `publish`.

Inbound and outbound operations are visible authored steps:

```text
cache_data -> compute -> commit_data
```

The compiler may normalize, validate, fingerprint, attach constraints, and compile these explicit steps. It may not invent them.

### Materialization scope is explicit

The document vocabulary supports:

```yaml
materialization:
  scope: shared
```

and:

```yaml
materialization:
  scope: worker
```

Phase 1 behavior:

| Scope | Definition validity | Runtime support |
|---|---|---|
| `shared` | valid | implemented |
| `worker` | valid | recognized but returns `ErrMaterializationScopeNotImplemented` during supported-capability validation |
| any other value | invalid | rejected as malformed configuration |

No silent default is permitted. The scope changes where paths are valid and must be explicit or inherited from an explicit higher-precedence binding.

### Shared paths belong to a materialization domain

A shared materialization is reusable only inside its declared target/materialization domain. Asset identity is global; materialized paths are domain-local.

```text
asset instance identity
    + materialization domain
    -> concrete materialization record and path list
```

### Worker resolution remains internal

The controller compiles logical asset requirements. The worker may use the internal resolver for final assignment-dependent values, including worker/runtime paths. Public workflow documents do not ship a second expression engine to worker plugins.

### `source_manifest` and `data` remain distinct

- `source_manifest` admits code, scripts, environment definitions, and other repository source files.
- `data` describes runtime datasets and durable outputs.

A repository file consumed as data belongs in `data`, even when its provider is the admitted repository.

### Structured expression directives normalize before resolution

Public variable-bearing sections may later use structured directives such as:

```yaml
pairs:
  $type: list
  $call: list.crossproduct
  args:
    - $ref: A
    - $ref: B
```

The loader converts this to an internal semantic call. `$expr` text containers are not part of internal variable JSON.

## Canonical Public Document Shapes

The examples retain the repository’s current `goet/v1alpha1` vocabulary. Renaming the API group to `gorc` is a separate compatibility decision.

### Project

```yaml
api_version: goet/v1alpha1
kind: Project
id: landcore-rci

variables:
  target_environment_id: msu-hpcc

# Project supplies general physical bindings and defaults.
data:
  inputs:
    yan_roy_field_segments:
      kind: envi_field_segments

      parameters:
        tile:
          type: string

      files:
        raster:
          member: "${asset.tile}/WELD_${asset.tile}_2010_field_segments"
          as: "WELD_${asset.tile}_2010_field_segments"
          required: true

        header:
          member: "${asset.tile}/WELD_${asset.tile}_2010_field_segments.hdr"
          as: "WELD_${asset.tile}_2010_field_segments.hdr"
          required: true

      select:
        - raster
        - header

      binding:
        provider_name: gdrive_release_data
        provider: gdrive_rclone

        location:
          remote: gdrive
          drive_path: Data/Field_Boundaries/ReleaseData.7z

        archive:
          type: seven_zip
          expose: selected_directory

        integrity:
          size_bytes: 261861012
          required: true

        cache:
          strategy: worker_cache
          cache_key: gdrive/field_boundaries/release-data/source.7z
          immutable: true

        materialization:
          scope: shared
          strategy: worker_cache
```

### Workflow overriding project selection

```yaml
api_version: goet/v1alpha1
kind: Workflow
id: yan-roy-header-analysis

variables:
  tiles:
    - h18v07
    - h18v08

# Workflow overrides the project default selection but inherits the provider.
data:
  inputs:
    yan_roy_field_segments:
      select:
        - header

steps:
  - id: cache-field-segment-headers

    fan_out:
      over: "${workflow.tiles[*]}"
      as: tile
      id: "${fanout.tile}"

    data:
      materialize:
        field_segments:
          asset: yan_roy_field_segments
          with:
            tile: "${fanout.tile}"

    work:
      type: cache_data

  - id: analyze-field-segment-headers

    fan_out:
      over: "${workflow.tiles[*]}"
      as: tile
      id: "${fanout.tile}"

    data:
      inputs:
        field_segments:
          asset: yan_roy_field_segments
          with:
            tile: "${fanout.tile}"

    work:
      type: python_script
      parameters:
        python_entrypoint: scripts/analyze_header.py
        args:
          - --header
          - "${data.field_segments.path[0]}"

source_manifest:
  files:
    - role: python_entrypoint
      path: scripts/analyze_header.py
      content_type: text/x-python
```

The two steps instantiate the same canonical asset instance for each tile. The first explicitly materializes it. The second consumes the completed shared materialization.

### Step-local projection

For `tile=h18v07` and `select=[header]`, assignment-time resolution exposes an equivalent value:

```json
{
  "data": {
    "field_segments": {
      "asset_key": "sha256:...",
      "materialization_domain_id": "msu-hpcc",
      "path": [
        "/shared/cache/.../WELD_h18v07_2010_field_segments.hdr"
      ],
      "files": {
        "header": {
          "path": "/shared/cache/.../WELD_h18v07_2010_field_segments.hdr",
          "size_bytes": 1234,
          "sha256": "..."
        }
      }
    }
  }
}
```

## Resolution and Execution Lifecycle

```text
1. Decode JSON or YAML.
2. Normalize keys, scalar types, and directive objects.
3. Validate controller/project/workflow envelope.
4. Load variable-bearing maps into implicit namespaces.
5. Overlay project.data, workflow.data, and submission data overrides.
6. Normalize workflow stages and fan-out declarations.
7. Bind one fan-out item into the `fanout` scope.
8. Bind asset parameters into the temporary `asset` scope.
9. Resolve file templates and effective selection.
10. Compute canonical asset-instance identity.
11. Compile the authored cache_data or compute work item.
12. cache_data materializes into the shared domain and emits a manifest.
13. A later compute assignment finds the matching manifest by asset identity and domain.
14. Build the assignment-local `data` scope.
15. Resolve `${data.<alias>.path[...]}` and named file-role paths.
16. Execute the worker operation.
17. Explicit commit_data work may publish declared outputs.
```

## Identity Model

### Human-readable identity

```text
yan_roy_field_segments[tile=h18v07;select=header]
```

### Canonical semantic identity

The asset key must include equivalent facts to:

```json
{
  "asset_definition": "yan_roy_field_segments",
  "resolved_parameters": {"tile": "h18v07"},
  "selection": ["header"],
  "provider": "gdrive_rclone",
  "resolved_location": {},
  "resolved_file_members": [],
  "integrity": {},
  "cache": {},
  "materialization_scope": "shared",
  "binding_fingerprint": "sha256:..."
}
```

The materialization lookup key additionally includes the materialization domain.

Aliases do not affect physical asset identity. Two step aliases may point to the same materialization.

## Shared Materialization Contract

An explicit `cache_data` step must:

1. receive a fully resolved asset instance;
2. acquire any provider resource constraint attached to that work item;
3. stage source and selected files outside the ready cache path;
4. verify all required selected roles;
5. promote the complete selected bundle atomically where practical;
6. emit one compact manifest containing ordered paths and named file roles;
7. release its work-item resource constraints when the cache operation completes.

A compute work item must not hold a cache acquisition constraint throughout its entire computation.

## Worker Scope Future Contract

`scope: worker` is reserved for a future visible preparation phase inside the assigned work item or for explicit worker-affined cache work. It is not implemented by this SC’s phase-one runtime slices.

Phase-one validation must return a sentinel-compatible error such as:

```go
var ErrMaterializationScopeNotImplemented = errors.New(
    "materialization scope is not implemented",
)
```

The error should identify the document path and the recognized value `worker`.

## Explicit Output Publication

Publishing follows the same visible-operator rule:

```yaml
steps:
  - id: publish-report
    data:
      outputs:
        report_archive:
          from:
            step: build-report
            artifact: report_archive
          target: report_archive

    work:
      type: commit_data
```

The effective target definition comes from the overlaid project/workflow `data.outputs` tree. The compiler must not discover a `publish` parameter inside a Python step and silently append `commit_data` work.

## YAML Profile

Phase 1 YAML support is intentionally narrow:

- mappings require string keys;
- duplicate mapping keys are errors;
- sequences preserve order;
- scalars normalize to JSON strings, booleans, and integers;
- fractional numbers are unsupported until the variable type system supports them;
- null is unsupported;
- arbitrary tags are rejected;
- aliases and anchors are either rejected or expanded with strict depth/node limits;
- timestamps are not implicitly converted to datetime values;
- comments and formatting do not participate in semantic fingerprints.

Explicit schema context or `$type` directives distinguish `path` and `datetime` from ordinary strings.

## Compatibility and Migration

GOET has not declared a stable production workflow schema. This SC intentionally permits a breaking migration.

The migration should:

- preserve low-level provider, archive, cache, integrity, transfer, materialization, and publish behavior;
- replace Go-field casing with canonical `snake_case` public fields;
- replace variable arrays with ordinary variable maps;
- move `data_assets` from Python parameters into project/workflow/step data sections;
- move `publish` from Python parameters into data outputs and explicit `commit_data` steps;
- replace implicit generated data operators with explicit authored steps;
- update current smoke fixtures in one controlled migration slice;
- reject legacy workflow shapes after migration rather than supporting two permanent public models.

## Non-Goals

- Implementing `materialization.scope: worker`.
- Creating hidden cache or publication work items.
- Building a mutable global data catalog.
- Adding worker affinity or per-node cache scheduling.
- Streaming remote data directly into plugins.
- Supporting arbitrary YAML tags or language-specific objects.
- Supporting JSON/YAML null values.
- Adding floating-point variables.
- Creating a general-purpose expression language.
- Allowing expression functions to access files, networks, environments, secrets, or plugins.
- Renaming the API group from `goet` to `gorc`.
- Changing dependency-aware execution from sequential stages to per-asset pipelining in phase one.

## Operational Slice Summary

### Document and workflow foundation

1. `001-canonical-public-document-contracts.md`
2. `002-json-yaml-source-decoder.md`
3. `003-canonical-typed-variable-loader.md`
4. `004-workflow-document-normalization.md`
5. `005-named-data-tree-overlay-and-precedence.md`

### Data model and execution

6. `006-data-asset-definition-binding-and-selection.md`
7. `007-parameterized-asset-instantiation.md`
8. `008-materialization-scope-and-shared-domain.md`
9. `009-explicit-cache-data-step.md`
10. `010-shared-materialization-hydration.md`
11. `011-step-data-projections-and-worker-resolution.md`
12. `012-explicit-commit-data-step.md`
13. `013-workflow-migration-and-equivalence-smoke.md`

### Expression-function refinement

14. `014-structured-function-call-model-and-loader-directives.md`
15. `015-resolver-jit-function-evaluation.md`
16. `016-list-crossproduct-function.md`
17. `017-list-zip-function.md`
18. `018-list-flatten-function.md`
19. `019-list-length-function.md`
20. `020-function-produced-fanout-proof.md`

## Completion Criteria

- Controller, project, and workflow documents have canonical public schemas independent of internal Go struct serialization.
- JSON and YAML normalize into equivalent canonical documents.
- Ordinary variable values load with implicit source namespaces.
- Project data is recursively overlaid by workflow data and submission overrides.
- Workflow or step selection can narrow a project-defined multi-file asset.
- Fan-out can instantiate parameterized asset definitions after item binding.
- `data.<alias>.path` resolves to an ordered list of materialized paths.
- Named file-role projections resolve correctly.
- `scope: shared` executes successfully inside a configured shared domain.
- `scope: worker` produces a recognized not-implemented error.
- Cache and publication work are explicit authored steps.
- Legacy implicit data-operator generation is removed after fixtures migrate.
- Current data-asset and publication smoke behavior is preserved under the new shape.
- Structured function calls normalize without `$expr` support in `internal/variable` JSON decoding.
- Initial list functions and fan-out proof pass.
- Focused tests and `go test ./...` pass after the migration slice.
