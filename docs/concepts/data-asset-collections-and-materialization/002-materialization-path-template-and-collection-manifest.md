# 002 Materialization Path Template and Collection Manifest

Status: proposed

## Objective

Add a safe materialization-domain-relative destination template to data-asset definitions and add the compact logical manifest used to represent a completed materialized collection.

This slice defines transport and validation contracts only.

## Current State

`DataDefinitionMaterialization` in `internal/model/data_definition.go` currently contains:

```go
type DataDefinitionMaterialization struct {
    Scope    string
    Strategy string
}
```

The worker chooses source-cache and extraction paths from worker configuration and cache keys.

`MaterializedDataAssetManifest` in `internal/model/data_asset.go` represents concrete materialized assets. `MaterializedDataProjection` in `internal/model/materialized_projection.go` exposes concrete local paths.

There is no safe asset-authored destination template and no schema for a collection-level logical output.

## Target State

### Definition-time path template

`DataDefinitionMaterialization` has:

```go
PathTemplate string `json:"path_template,omitempty"`
```

For a collection asset, phase one requires a non-empty path template such as:

```yaml
materialization:
  scope: shared
  strategy: worker_cache
  path_template: "cdl/${asset.year}.tif"
```

Validation must parse placeholders structurally rather than accepting arbitrary unresolved interpolation.

Rules:

- the template is slash-relative;
- no absolute path, drive prefix, backslash, empty segment, `.`, or `..`;
- placeholders use exactly `${asset.<parameter>}`;
- every referenced name is a declared asset parameter;
- every collection dimension appears in the template;
- unsupported interpolation syntax fails;
- escaped literal `${` behavior is explicit and tested;
- after fixed parameters are bound, collection-dimension placeholders can be normalized from `${asset.year}` to `${year}` for logical output;
- scalar assets may omit the template and retain existing materialization behavior;
- phase-one collection assets expose exactly one selected path per member.

### Collection logical manifest

Add a transport model equivalent to:

```go
const MaterializedAssetCollectionManifestSchemaV1 =
    "goet/materialized-asset-collection/v1"

type MaterializedAssetCollectionManifest struct {
    Schema                    string
    Asset                     string
    MaterializationDomainID   string
    DimensionOrder            []string
    Dimensions                map[string]MaterializedAssetCollectionDimension
    Path                      string
    RequiredBindings          []string
    MemberCount               int
    MembersSHA256             string
    CollectionFingerprint     string
}

type MaterializedAssetCollectionDimension struct {
    Type   string
    Values []any
}
```

The exact field names may follow repository conventions. The public JSON semantics in the Strategic Concept README are authoritative.

The manifest is compact and does not contain the ordinary list of member work-item outputs.

### Member metadata

Define bounded optional collection-member metadata that can accompany each concrete `MaterializedDataAssetManifest`:

```text
collection fingerprint
member index
member count
dimension order
member bindings
destination-relative path
path-template identity
```

This metadata lets the controller validate and aggregate member outputs without adding a second unbounded payload.

## Concept Decision

This slice adds two related model concepts:

1. a definition-time destination template;
2. a completed collection descriptor.

Put collection manifest/member types in a separate file because they have independent validation and versioned schema behavior.

Do not add a new `internal/variable` kind. The deferred path is valid only inside the versioned collection manifest and is bound by data-asset collection logic in a later slice.

## Required Context

Read these files first:

- `AGENTS.md`
- `docs/concepts/data-asset-collections-and-materialization/README.md`
- `docs/concepts/data-asset-collections-and-materialization/001-finite-asset-collection-domain-model.md`
- `internal/model/data_definition.go`
- `internal/model/data_asset.go`
- `internal/model/materialized_projection.go`
- `internal/model/data_definition_test.go`
- `internal/model/data_asset_test.go`
- `internal/model/materialized_projection_test.go`
- `internal/model/artifact_manifest.go` for established path-safety style

Do not read worker acquisition, controller persistence, scheduler, transport, or provider subprocess files for this slice.

## Allowed Production Files

- `internal/model/data_definition.go`
- `internal/model/data_asset_collection.go`
- `internal/model/materialized_asset_collection.go` (new)
- `internal/model/data_asset.go` only for optional collection-member metadata on the concrete manifest

## Allowed Test Files

- `internal/model/data_definition_test.go`
- `internal/model/data_asset_collection_test.go`
- `internal/model/materialized_asset_collection_test.go` (new)
- `internal/model/data_asset_test.go` only for member-metadata round trips

## Allowed Documentation Files

- `docs/concepts/data-asset-collections-and-materialization/002-materialization-path-template-and-collection-manifest.md`
- `PROJECT_STATE.md` only after implementation

## Out Of Scope

- Resolving templates into concrete member destinations.
- Adding a generic path-template variable type.
- Letting arbitrary unresolved variables survive ordinary resolution.
- Work-item compilation.
- Worker filesystem promotion.
- Controller collection-output synthesis.
- Downstream hydration.
- Multi-role collection output.
- Multiple path templates per collection member.
- Worker-scope materialization.
- Absolute destination paths authored by projects.
- Provider credentials or secret resolution.

## Acceptance Criteria

- `path_template: "cdl/${asset.year}.tif"` validates for a collection dimension named `year`.
- A scalar asset may omit `path_template`.
- A collection asset without `path_template` fails.
- A collection template that omits one collection dimension fails.
- A template referencing an undeclared parameter fails.
- Absolute, drive-qualified, backslash-containing, traversing, empty-segment, and unclean templates fail.
- A nested or malformed interpolation expression fails.
- Template parsing returns an ordered placeholder list without relying on map iteration.
- Fixed parameters and collection dimensions can both appear, but only collection dimensions remain deferred after fixed binding.
- Output-template normalization converts `${asset.year}` to `${year}` without evaluating it as a generic variable.
- A valid `goet/materialized-asset-collection/v1` manifest validates.
- Manifest dimension order and required bindings agree.
- Manifest dimension values are non-empty and type-correct.
- Manifest member count equals the checked Cartesian-product cardinality.
- Manifest path contains exactly the declared required binding placeholders.
- Manifest hashes use the repository's accepted `sha256:` convention.
- A manifest with an ordinary member-output list is not required and no `members` byte payload is added.
- Concrete materialized-data manifest JSON round trips preserve optional member metadata.
- Existing concrete materialized-data manifest behavior remains backward-compatible when collection metadata is absent.
- `go test ./internal/model` passes.

## Notes

- The path template is relative to the configured materialization root; the project must not know a worker-local absolute root.
- The logical output becomes absolute only after successful worker materialization establishes one common root.
- Do not overload artifact paths or source-manifest path validation with unresolved placeholders; use a dedicated template validator that validates literal segments around recognized placeholders.
- Suggested HCI: `EC-3 / operational slice / files(4)+test+doc+newfile`.
