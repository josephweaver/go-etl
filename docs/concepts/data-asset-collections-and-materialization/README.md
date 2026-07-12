# Data Asset Collections and Explicit Materialization

Status: Ready  
Cadence: CSxIx  
Target repository: `josephweaver/go-etl`  
Reviewed against repository `main`: 2026-07-12

## Purpose

Add a first-class collection form for parameterized data assets and make `asset.materialize` the single explicit operation for placing those assets at deterministic paths inside a materialization domain.

The motivating case is USDA Cropland Data Layer data for a finite set of years:

```text
CDL 2008 through 2023
```

The project should be able to declare that finite domain once, declare the provider/archive/member templates once, and declare one deterministic destination template:

```text
cdl/${year}.tif
```

A workflow can then author one explicit step:

```text
asset.materialize(cdl)
```

The compiler may expand that step into one concrete work item per year, but the logical step output is a compact collection descriptor:

```text
domain + deferred path template + completion fingerprint
```

It is not a normal fan-out list of sixteen work-item outputs.

## Goals

- Let a data-asset definition declare an ordered, finite collection domain over existing asset parameters.
- Support explicit scalar values and inclusive integer ranges as the first collection-domain sources.
- Let a data-asset definition declare a safe materialization-domain-relative destination template.
- Make `asset.materialize` the only authored and worker operation for inbound asset materialization.
- Replace the current `cache_data` work-item name, payload name, compiler terminology, controller terminology, worker dispatch terminology, fixtures, and documentation.
- Reuse the current provider acquisition, archive selection, integrity verification, transfer throttling, immutable source cache, and resource-constraint implementation.
- Compile one authored collection materialization step into deterministic concrete member work items.
- Keep each member work item fully concrete: provider location, archive selection, parameter bindings, destination path, asset identity, and materialization identity.
- Materialize each member to a deterministic destination under the configured shared materialization root.
- Fail rather than overwrite when an existing destination does not match the expected asset identity and evidence.
- Preserve concrete member completion evidence for recovery and downstream hydration.
- Produce one compact logical collection output containing the finite domain and a deferred absolute path template.
- Let later work fan out over the collection domain and bind one concrete member through the ordinary `data` namespace.
- Avoid nested interpolation and avoid treating arbitrary unresolved variables as successful resolution.
- Preserve current sequential-stage semantics; a later stage starts after the complete requested collection is materialized.

## Non-Goals

- Do not create an `asset.materialization` operation.
- Do not retain `cache_data` as a permanent alias or second public operation.
- Do not create hidden materialization work merely because a compute step mentions an asset.
- Do not build a mutable global data catalog.
- Do not discover collection members by listing a remote provider.
- Do not support open-ended, streaming, or provider-enumerated collection domains in phase one.
- Do not support collection subsets at invocation time in phase one.
- Do not support workflow-authored fan-out combined with asset collection expansion in the same `asset.materialize` step in phase one.
- Do not add a general-purpose partial-resolution mode to `internal/variable`.
- Do not allow unresolved project, workflow, provider, secret, or runtime variables to survive compilation accidentally.
- Do not resolve worker-local roots during project or workflow document loading.
- Do not implement `materialization.scope: worker`.
- Do not add worker affinity or node-local cache scheduling.
- Do not change `commit_data`.
- Do not add real CDL downloads to unit tests or default smoke tests.
- Do not add GDAL, rasterio, numpy, pandas, pyarrow, or other geospatial dependencies to the Go runtime.
- Do not rename the API group, module, executable, environment variables, or manifest schemas from `goet` to `gorc` in this concept.
- Do not implement multi-role collection outputs in phase one; one exposed materialized path per collection member is required.

## Architectural Context

This Strategic Concept refines the data-execution portion of:

```text
docs/concepts/canonical-workflow-data-document-model/README.md
```

In particular, it supersedes the proposed public operation name `cache_data` and refines the behavior planned around explicit materialization, shared hydration, and step data projection.

It builds on the implemented mechanics described by:

```text
docs/concepts/data-assets-and-materialized-outputs/README.md
docs/concepts/dependency-aware-workflows/README.md
docs/concepts/fan-out-value-binding-refinement/README.md
PROJECT_STATE.md
```

The ownership boundary remains:

```text
internal/model       transport and validation contracts
internal/workflow    asset instantiation, collection expansion, and work compilation
cmd/controller       dependency state, completion aggregation, recovery, and hydration
cmd/worker           provider acquisition, archive extraction, destination promotion, and evidence
internal/variable    ordinary typed values, scopes, references, and fully resolved expressions
```

Collection-template binding is owned by the data-asset/compiler boundary, not by a new global expression language.

## Terminology

### Asset definition

A named project/workflow data input containing logical kind, format, parameter definitions, file-role templates, provider binding, archive selection, integrity policy, source-cache policy, and materialization policy.

### Collection domain

An ordered finite set of values assigned to one or more existing asset parameters.

Example:

```text
year = [2008, 2009, ..., 2023]
```

### Collection member

One concrete asset instance produced by binding one value for every collection dimension together with any fixed non-dimension parameters.

Example:

```text
cdl[year=2017]
```

### Source asset identity

The canonical identity of the concrete provider object and selected content. It includes resolved provider location, parameters, archive selection, integrity expectations, source-cache policy, and materialization scope. It excludes the step-local alias.

### Materialization identity

The source asset identity plus the materialization-domain identity and resolved destination-relative path.

The same source asset may be materialized at two destinations. Those are two materializations but may reuse one source-cache entry.

### Materialization domain

The configured shared filesystem domain in which deterministic destination paths are meaningful. Phase one uses the current shared/target-environment boundary and configured asset cache root.

### Destination template

A safe slash-relative path template declared by the asset definition, such as:

```text
cdl/${asset.year}.tif
```

At concrete member compilation, every collection dimension is bound and the destination becomes:

```text
cdl/2017.tif
```

### Deferred output path

The logical collection output exposes the common absolute path template after the worker-owned materialization root is known:

```text
/mnt/scratch/weave151/etl/runtime/cache/cdl/${year}.tif
```

The remaining placeholder is intentional collection metadata. It is not an unresolved generic variable.

## Current State

### Strategic state

GOET already treats large input data as execution-environment data rather than repository-source bytes or SQLite payload bytes. Provider acquisition, archive extraction, integrity evidence, immutable source caching, transfer limits, resource constraints, and materialized-data manifests exist.

The canonical document concept already moves acquisition out of compute parameters and requires a visible materialization step. Its current public name is `cache_data`.

### Operational state

The current repository has these concrete behaviors:

- `internal/model/data_definition.go` defines `DataInputDefinition`, parameter definitions, file roles, provider bindings, cache policy, and `DataDefinitionMaterialization` with `scope` and `strategy`.
- A data input does not declare a finite collection domain.
- `DataDefinitionMaterialization` does not declare a deterministic destination template.
- `internal/workflow/data_instance.go` instantiates one fully bound asset from explicit parameter bindings.
- `internal/model/work_item.go` defines `WorkItemTypeCacheData = "cache_data"` and `CacheDataWorkItemPayload`.
- `internal/workflow/explicit_cache_data.go` compiles an explicit `cache_data` item for one bound asset.
- `internal/workflow/cache_data_plan.go` still contains legacy planning that can generate materialization work from compute-item data parameters.
- `cmd/worker/work_cache_data.go` validates the `cache_data` payload and delegates actual work to the existing `assetMaterializer`.
- `cmd/worker/data_asset_materializer.go` acquires sources into the worker asset cache, verifies integrity, and applies archive selection.
- The current materializer chooses its cache/extraction paths from cache keys and worker configuration; the asset definition does not name the final deterministic member path.
- `cmd/controller/cache_data_dependencies.go` and `cache_data_hydration.go` own dependency release and completed-manifest hydration using `cache_data` terminology.
- `cmd/controller/workflow_outputs.go` returns one object for a one-item step and a JSON list for a step with multiple completed work items.
- `internal/model/materialized_projection.go` and `cmd/worker/data_scope.go` project concrete materialized paths into the step-local `data` namespace.
- Arbitrary unresolved interpolation fails during ordinary variable resolution, which is the correct default.

The mismatch is that an authored fan-out materialization step naturally produces a list of completed outputs, while the desired collection abstraction is one logical asset collection with a finite domain and an indexed path template.

## Target State

### Strategic Decisions

#### 1. Replace `cache_data` with `asset.materialize`

The public and worker operation is:

```text
asset.materialize
```

The noun `materialization` remains valid in model and policy names. The verb operation is not named `asset.materialization`.

The word `cache` remains valid for the internal/source-cache policy:

```yaml
cache:
  strategy: worker_cache
  cache_key_template: "cdl/source/${asset.year}.zip"
```

It is no longer the authored operation name.

#### 2. The collection domain belongs to the asset definition

A collection dimension references an existing declared asset parameter. Dimensions are ordered.

Phase-one dimension sources are:

- a non-empty explicit list of scalar values;
- an inclusive ascending integer range.

Example:

```yaml
parameters:
  year:
    type: int

collection:
  dimensions:
    - parameter: year
      range:
        from: 2008
        through: 2023
```

The domain is finite before workflow compilation begins.

#### 3. The destination template belongs to materialization policy

The project/workflow asset definition declares a materialization-domain-relative path:

```yaml
materialization:
  scope: shared
  strategy: worker_cache
  path_template: "cdl/${asset.year}.tif"
```

The template may reference declared asset parameters. Every collection dimension must participate in the phase-one destination template so distinct members cannot collapse onto the same path accidentally.

The execution environment supplies the absolute materialization root.

#### 4. One authored step may compile into many concrete member work items

This authored step:

```yaml
- id: materialize-cdl
  data:
    materialize:
      cdl:
        asset: cdl
  work:
    type: asset.materialize
```

compiles into sixteen concrete work items for years 2008 through 2023.

This is explicit work expansion of an explicit operation. It is not hidden acquisition inserted because a compute step referenced data.

#### 5. Concrete member outputs and logical step output are different contracts

Every member work item emits concrete evidence including:

- collection fingerprint;
- member index and expected member count;
- dimension bindings;
- source asset key;
- materialization identity;
- materialization domain;
- destination-relative path;
- absolute local path;
- byte count and hash evidence;
- archive-selection evidence when applicable.

Those member outputs remain available as terminal execution evidence and recovery facts.

After all expected members complete, the controller produces one logical step output:

```json
{
  "schema": "goet/materialized-asset-collection/v1",
  "asset": "cdl",
  "materialization_domain_id": "msu-hpcc",
  "dimension_order": ["year"],
  "dimensions": {
    "year": {
      "type": "int",
      "values": [2008, 2009, 2010, 2011, 2012, 2013, 2014, 2015, 2016, 2017, 2018, 2019, 2020, 2021, 2022, 2023]
    }
  },
  "path": "/mnt/scratch/weave151/etl/runtime/cache/cdl/${year}.tif",
  "required_bindings": ["year"],
  "member_count": 16,
  "members_sha256": "sha256:..."
}
```

The logical output does not contain a normal list of sixteen work-item outputs.

#### 6. Deferred output paths are schema-governed, not generic partial resolution

Only a validated `goet/materialized-asset-collection/v1` descriptor may carry the deferred collection path.

The descriptor declares:

```text
required_bindings
dimension values
materialization domain
member count
completion fingerprint
```

Ordinary string/path interpolation continues to fail if a required variable is absent.

#### 7. Downstream execution binds a concrete asset member normally

A later step fans out over the completed collection domain:

```yaml
fan_out:
  over: "${workflow.step[0].dimensions.year.values[*]}"
  as: year
  id: "${fanout.year}"
```

It requests the concrete member using the existing asset-binding shape:

```yaml
data:
  inputs:
    cdl:
      asset: cdl
      with:
        year: "${fanout.year}"
```

The controller computes the same concrete source asset key and materialization identity, selects the completed member evidence, and supplies a normal concrete `data.cdl.path` projection.

The compute operation does not evaluate:

```text
${some_function(cdl, ${year})}
```

and does not depend on a second interpolation pass over an arbitrary string.

#### 8. Source-cache identity and destination identity remain separate

Two members may share provider bytes or archives and still have distinct selected outputs and destinations.

The implementation must preserve:

```text
source cache reuse
    !=
destination materialization deduplication
```

A destination collision between different materialization identities is a compile-time or pre-execution error.

## Canonical Project Example

```yaml
api_version: goet/v1alpha1
kind: Project
id: cdl-analysis

data:
  inputs:
    cdl:
      kind: raster
      format: geotiff

      parameters:
        year:
          type: int

      collection:
        dimensions:
          - parameter: year
            range:
              from: 2008
              through: 2023

      files:
        raster:
          member: "${asset.year}_30m_cdls.tif"
          as: "${asset.year}.tif"
          required: true

      select:
        - raster

      binding:
        provider_name: usda_cdl
        provider: http

        location:
          url_template: "https://example.invalid/cdl/${asset.year}_30m_cdls.zip"

        archive:
          type: zip
          expose: selected_path

        cache:
          strategy: worker_cache
          cache_key_template: "cdl/source/${asset.year}.zip"
          immutable: true

        materialization:
          scope: shared
          strategy: worker_cache
          path_template: "cdl/${asset.year}.tif"
```

The URL is illustrative. Tests and default smoke paths must use local fixture HTTP servers or fixture files, not this external address.

## Canonical Workflow Example

```yaml
api_version: goet/v1alpha1
kind: Workflow
id: cdl-analysis

steps:
  - id: materialize-cdl

    data:
      materialize:
        cdl:
          asset: cdl

    work:
      type: asset.materialize

  - id: inspect-cdl

    fan_out:
      over: "${workflow.step[0].dimensions.year.values[*]}"
      as: year
      id: "${fanout.year}"

    data:
      inputs:
        cdl:
          asset: cdl
          with:
            year: "${fanout.year}"

    work:
      type: python_script
      parameters:
        python_entrypoint: scripts/inspect_cdl.py
        args:
          - --year
          - "${fanout.year}"
          - --input
          - "${data.cdl.path[0]}"

source_manifest:
  files:
    - role: python_entrypoint
      path: scripts/inspect_cdl.py
      content_type: text/x-python
```

## Resolution and Execution Lifecycle

```text
1. Decode and normalize project/workflow documents.
2. Overlay project, workflow, and submission data definitions.
3. Validate the asset parameter definitions.
4. Validate the ordered finite collection domain.
5. Validate the domain-relative materialization path template.
6. Expand the collection domain deterministically.
7. Bind one dimension tuple plus fixed parameters into the asset scope.
8. Resolve provider location, archive selection, source cache key, and destination-relative path.
9. Compute the source asset key.
10. Compute the materialization identity from source asset key + domain + destination.
11. Reject destination collisions.
12. Compile one concrete asset.materialize member work item per tuple.
13. Acquire provider/source resource constraints per member.
14. Worker acquires or reuses the source cache.
15. Worker verifies integrity and applies archive selection.
16. Worker stages and atomically promotes the selected result to the declared deterministic destination.
17. Worker emits concrete member evidence.
18. Controller records each member terminal result.
19. After all expected members complete, controller validates collection completeness.
20. Controller writes one compact collection descriptor as the logical step output.
21. A later step fans out over the descriptor's dimension values.
22. The later step binds one concrete asset member through data.inputs.
23. Controller hydrates that binding from completed member evidence.
24. Worker receives an ordinary concrete data projection and executes compute work.
```

## Identity and Collision Rules

### Source asset key

Equivalent facts include:

```json
{
  "asset_definition": "cdl",
  "resolved_parameters": {"year": 2017},
  "selection": ["raster"],
  "provider": "http",
  "resolved_location": {"uri": ".../2017_30m_cdls.zip"},
  "resolved_archive_members": ["2017_30m_cdls.tif"],
  "integrity": {},
  "cache": {},
  "materialization_scope": "shared"
}
```

The step alias does not participate.

### Materialization identity

Equivalent facts include:

```json
{
  "source_asset_key": "sha256:...",
  "materialization_domain_id": "msu-hpcc",
  "destination_relative_path": "cdl/2017.tif"
}
```

### Collection fingerprint

Equivalent facts include:

```json
{
  "asset_definition": "cdl",
  "dimension_order": ["year"],
  "dimension_values": {"year": [2008, "...", 2023]},
  "fixed_parameters": {},
  "selection": ["raster"],
  "path_template": "cdl/${year}.tif",
  "materialization_domain_id": "msu-hpcc"
}
```

### Collision policy

- Same source asset key + same domain + same destination: one materialization identity; duplicate authored materializers are rejected or deduplicated according to the explicit-step conflict rule.
- Same source asset key + different destination: two materializations; source cache may be reused.
- Different source asset keys + same destination: fail before overwrite.
- Existing destination + matching pinned destination evidence: cache hit.
- Existing destination + missing or conflicting destination evidence: fail.
- No silent overwrite.

## Failure Semantics

The collection step fails when any required member fails.

The logical collection output is written only after all expected members have completed or been validly reused.

A failed or incomplete collection must not expose a successful collection descriptor.

Downstream work must fail admission/activation before worker execution when:

- the requested dimension is absent;
- the requested value is outside the declared domain;
- no completed member evidence matches the source asset key and materialization domain;
- destination evidence conflicts;
- the collection descriptor fingerprint does not match the admitted asset definition.

## Compatibility and Migration

GOET interfaces are currently experimental. This concept uses a deliberate breaking migration.

Final state:

```text
asset.materialize   accepted
cache_data          rejected
asset.materialization rejected
```

The migration should preserve behavior while changing names first, then add collection behavior.

Historical documentation may mention `cache_data` as the superseded name. Production code, current schemas, fixtures, active tests, and canonical examples must use `asset.materialize`.

## Relationship to `commit_data`

This concept changes inbound data materialization only.

Outbound publication remains:

```text
commit_data
```

A future naming review may consider a namespaced publication operation, but that is not part of this work.

## Proposed Slices

1. `001-finite-asset-collection-domain-model.md`  
   Add ordered finite collection dimensions that reference existing data-asset parameters.

2. `002-materialization-path-template-and-collection-manifest.md`  
   Add safe destination templates and the compact logical collection descriptor contract.

3. `003-collection-member-expansion-and-identity.md`  
   Expand finite domains into deterministic concrete members and define source/materialization/collection identities.

4. `004-rename-cache-data-to-asset-materialize.md`  
   Atomically replace the current `cache_data` operation, payload, compiler, controller, worker, tests, and current docs with `asset.materialize`.

5. `005-compile-explicit-collection-materialization.md`  
   Compile one authored `asset.materialize` step into concrete collection-member work items without workflow-authored fan-out.

6. `006-worker-deterministic-destination-materialization.md`  
   Promote each acquired/selected member into its declared deterministic destination with pinned conflict-safe evidence.

7. `007-collection-step-output-synthesis.md`  
   Preserve member evidence while synthesizing one compact collection descriptor instead of a normal output list.

8. `008-downstream-member-hydration-and-data-projection.md`  
   Fan out over collection dimensions and hydrate each concrete member into the normal step-local `data` namespace.

9. `009-remove-legacy-hidden-materialization-and-migrate-fixtures.md`  
   Remove compute-parameter scanning and migrate active workflows, fixtures, tests, and canonical docs.

10. `010-collection-materialization-smoke-and-concept-closure.md`  
    Prove the complete fixture path, update current-state documentation, and close the concept after review.

## Completion Criteria

- `DataInputDefinition` can declare an ordered finite collection domain.
- A dimension references an existing asset parameter and has either explicit values or an inclusive integer range.
- Collection validation rejects empty domains, duplicate dimensions, type mismatches, invalid ranges, and cardinality overflow.
- `binding.materialization.path_template` is safe, slash-relative, and references only declared asset parameters.
- Every collection dimension participates in the phase-one destination template.
- The work-item operation is `asset.materialize`.
- Current production code has no active `cache_data` work-item type, payload, parameter, dispatch, or planner.
- `asset.materialization` is not accepted as an operation.
- One explicit collection materialization step compiles into a deterministic ordered set of concrete members.
- Each concrete member has a source asset key and a distinct materialization identity.
- Distinct members cannot resolve to one destination silently.
- The worker reuses current source acquisition and archive-selection mechanics.
- The worker places the selected result at the declared deterministic destination.
- Existing matching destination evidence is reusable.
- Existing missing/conflicting destination evidence fails without overwrite.
- Every concrete member emits bounded completion evidence.
- A completed collection step emits one `goet/materialized-asset-collection/v1` logical output.
- The logical output contains a finite domain, one deferred path template, member count, and member-evidence fingerprint.
- The logical output is not a JSON list of ordinary work-item outputs.
- A later step can fan out over the collection dimension values.
- A later asset binding with one dimension value hydrates the matching concrete member and exposes a concrete `data.<alias>.path`.
- Missing or out-of-domain member bindings fail before worker execution.
- Controller restart can reconstruct collection completion and member hydration from durable facts.
- Legacy hidden materialization generation is removed.
- Fixture-sized unit and integration tests use no external network and no large data.
- The end-to-end smoke proves materialization, compact collection output, downstream fan-out, concrete path hydration, cache reuse, and conflict failure.
- `go test ./...` passes.
- `PROJECT_STATE.md`, the concept index, and affected canonical document-model docs describe the implemented current state accurately.
- The Strategic Concept is marked Implemented only after implementation review and human acceptance.
