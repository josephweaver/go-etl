# 005 Data Location, Provider, and Binding Model

Status: proposed

## Objective

Add shared model types for named data locations, reusable data provider templates, concrete step data bindings, archive-selection declarations, cache policies, integrity checks, materialized input asset manifests, predefined publish targets, and publish bindings.

This slice defines how projects and workflows describe large inputs and final output destinations without treating those paths as source files. It does not implement downloads, local caching, archive extraction, Python argument interpolation, artifact publication, HPCC runs, or catalog registration.

## Current State

Workflow source documents can declare source files through `source_manifest`. That is appropriate for Python entrypoints, environment specs, and support files. It is not appropriate for multi-GB public rasters, Google Drive release archives, shared HPCC-resident tile datasets, or generated field/crop/year products.

The previous version of this concept modeled a data asset as a concrete declaration with `name`, `location`, and `materialization`. That was useful but not ergonomic enough for reusable sources such as CDL by year, Yan/Roy release archives, local manually downloaded files, or Google Drive Shared Drive assets.

The needed shape is:

```text
project/workflow defines provider templates, cache policies, archive selectors, and publish targets
step binds concrete provider instances to aliases
worker materializes aliases and exposes local paths
worker later publishes selected artifacts to named locations
```

## Target State

`internal/model` exposes data-location and data-provider model types equivalent to the following semantics.

### Data locations

```go
type DataLocation struct {
    Name     string         `json:"name"`
    Type     string         `json:"type"` // registered_location initially
    Access   string         `json:"access,omitempty"` // read_only, write_only, read_write
    RootRef  string         `json:"root_ref,omitempty"`
    Metadata map[string]any `json:"metadata,omitempty"`
}

type DataLocationPathTemplate struct {
    Name         string `json:"name"`
    PathTemplate string `json:"path_template"`
}
```

### Provider templates

The model may use provider-specific structs or a single struct with optional provider-specific fields. Equivalent semantics are required:

```go
type DataProviderTemplate struct {
    Name       string `json:"name"`
    Kind       string `json:"kind"`
    Format     string `json:"format,omitempty"`
    Provider   string `json:"provider"` // http, local_file, registered_location, gdrive_rclone

    // http
    URLTemplate string `json:"url_template,omitempty"`
    URITemplate string `json:"uri_template,omitempty"` // accepted alias if existing code prefers URI

    // local_file and registered_location
    Location *DataLocationPathTemplate `json:"location,omitempty"`

    // gdrive_rclone
    GDrive *GDriveRcloneTemplate `json:"gdrive,omitempty"`

    Parameters      []string                         `json:"parameters,omitempty"`
    Integrity       DataAssetIntegrityTemplate       `json:"integrity,omitempty"`
    Cache           DataAssetCacheTemplate           `json:"cache,omitempty"`
    Archive         *DataAssetArchiveTemplate        `json:"archive,omitempty"`
    Materialization DataAssetMaterializationTemplate `json:"materialization,omitempty"`
    Metadata        map[string]any                   `json:"metadata,omitempty"`
}

type GDriveRcloneTemplate struct {
    Remote       string `json:"remote"`
    PathTemplate string `json:"path_template"`
    FileIDTemplate string `json:"file_id_template,omitempty"` // reserved if a later adapter supports ID-based addressing
}
```

Supported provider values for this concept are:

```text
http
local_file
registered_location
gdrive_rclone
```

`https` may be accepted as an alias only if the implementation already distinguishes URI schemes separately. Prefer `provider: "http"` with an `https://...` URL.

### Integrity and cache policy

```go
type DataAssetIntegrityTemplate struct {
    SHA256Template string `json:"sha256,omitempty"`
    SizeBytes      *int64 `json:"size_bytes,omitempty"`
    Required       bool   `json:"required,omitempty"`
}

type DataAssetCacheTemplate struct {
    Strategy         string `json:"strategy,omitempty"` // worker_cache, reference
    CacheKeyTemplate string `json:"cache_key_template,omitempty"`
    Immutable        *bool  `json:"immutable,omitempty"`
}
```

Semantics:

- `worker_cache` means the worker may copy, download, or extract data into a cache root.
- `reference` means the worker exposes an existing path under a configured named root without copying it.
- `immutable: true` means a cache key must not silently change content. Existing cache entries must be reverified before reuse.
- If `immutable` is omitted for `worker_cache`, default to true.
- If an expected SHA-256 or size is present, mismatches must fail before plugin execution.
- If no expected SHA-256 is present, the first successful materialization may record observed evidence; later reuse under the same immutable cache key must match that evidence.

### Archive selection

```go
type DataAssetArchiveTemplate struct {
    Type   string                        `json:"type"` // zip, seven_zip
    Select []DataAssetArchiveSelectTemplate `json:"select,omitempty"`
    Expose string                        `json:"expose,omitempty"` // selected_path, selected_directory
}

type DataAssetArchiveSelectTemplate struct {
    MemberTemplate string `json:"member_template"`
    As             string `json:"as,omitempty"`
    Required       *bool  `json:"required,omitempty"`
}
```

Semantics:

- `zip` is the first built-in extractor target.
- `seven_zip` is a reserved external-executable extractor for archives such as `ReleaseData.7z`.
- `select` entries are archive-member selectors, not host paths.
- `member_template` and `as` must use safe slash-relative path rules.
- `selected_path` is valid only when exactly one required file is selected.
- `selected_directory` exposes the directory containing selected extracted members.
- The extractor must never write outside its extraction root.

### Bound input assets

A workflow step binding should compile into a concrete bound input asset equivalent to:

```go
type BoundDataAsset struct {
    BindingName string `json:"binding_name"` // e.g. cropland_year
    ProviderName string `json:"provider_name"` // e.g. cdl_zip
    Kind string `json:"kind"`
    Format string `json:"format,omitempty"`
    Provider string `json:"provider"` // resolved provider type
    Location DataAssetLocation `json:"location"`
    Integrity DataAssetIntegrity `json:"integrity,omitempty"`
    Cache DataAssetCache `json:"cache,omitempty"`
    Archive *DataAssetArchive `json:"archive,omitempty"`
    Materialization DataAssetMaterialization `json:"materialization,omitempty"`
    Parameters map[string]any `json:"parameters,omitempty"`
    Metadata map[string]any `json:"metadata,omitempty"`
}
```

Keep the concrete location model simple:

```go
type DataAssetLocation struct {
    Type string `json:"type"` // http, local_file, registered_location, gdrive_rclone
    URI  string `json:"uri,omitempty"`
    LocationName string `json:"location_name,omitempty"`
    Path string `json:"path,omitempty"`
    Remote string `json:"remote,omitempty"`
    DrivePath string `json:"drive_path,omitempty"`
    FileID string `json:"file_id,omitempty"`
}
```

### Published targets

A project/workflow may also define published data asset targets:

```go
type PublishedDataAssetTarget struct {
    Name string `json:"name"`
    Kind string `json:"kind"`
    Format string `json:"format,omitempty"`
    Location DataLocationPathTemplate `json:"location"`
    Parameters []string `json:"parameters,omitempty"`
    OverwritePolicy string `json:"overwrite_policy,omitempty"` // fail_if_exists default
    Metadata map[string]any `json:"metadata,omitempty"`
}
```

A workflow step publish binding should compile into a concrete bound publish target:

```go
type BoundPublishTarget struct {
    Name string `json:"name"`
    FromArtifact string `json:"from_artifact"`
    TargetName string `json:"target_name"`
    Location DataAssetLocation `json:"location"`
    OverwritePolicy string `json:"overwrite_policy,omitempty"`
    Parameters map[string]any `json:"parameters,omitempty"`
    Metadata map[string]any `json:"metadata,omitempty"`
}
```

### Materialized manifest

A separate worker-facing materialized manifest type should represent assets actually present in the execution environment:

```go
type MaterializedDataAssetManifest struct {
    Schema string `json:"schema"`
    Assets []MaterializedDataAsset `json:"assets"`
}

type MaterializedDataAsset struct {
    BindingName string `json:"binding_name"`
    ProviderName string `json:"provider_name,omitempty"`
    ProviderType string `json:"provider_type,omitempty"`
    Kind string `json:"kind"`
    Format string `json:"format,omitempty"`
    LocalPath string `json:"local_path"`
    MaterializationStrategy string `json:"materialization_strategy,omitempty"`
    CacheKey string `json:"cache_key,omitempty"`
    CacheImmutable *bool `json:"cache_immutable,omitempty"`
    SourceSizeBytes *int64 `json:"source_size_bytes,omitempty"`
    SourceSHA256 string `json:"source_sha256,omitempty"`
    SelectedSizeBytes *int64 `json:"selected_size_bytes,omitempty"`
    SelectedSHA256 string `json:"selected_sha256,omitempty"`
    ArchiveType string `json:"archive_type,omitempty"`
    ArchiveMembers []MaterializedArchiveMember `json:"archive_members,omitempty"`
    Metadata map[string]any `json:"metadata,omitempty"`
}

type MaterializedArchiveMember struct {
    Member string `json:"member"`
    LocalPath string `json:"local_path"`
    SizeBytes *int64 `json:"size_bytes,omitempty"`
    SHA256 string `json:"sha256,omitempty"`
}
```

The model should validate names, supported provider/location types, required template fields, parameter references, hash shape, cache-key/path-template safety, archive member safety, rclone remote/path shape, and conservative publish overwrite policies. It should not fetch, copy, extract, or publish data.

## Concept Decision

Provider templates and publish targets are declaration-time constructs. Workers should receive concrete bound data assets and concrete bound publish targets.

Do not overload `source_manifest` for data assets. `source_manifest` remains for admitted execution source. Data provider templates, bound assets, archive selectors, cache policies, and publish targets are execution data contracts, not code.

Do not require a central data registry in this slice. A project can define the intended published asset target ahead of time; later workflow execution copies bytes there and reports evidence.

## Required Context

Read these files first:

- `AGENTS.md`
- `PROJECT_STATE.md`
- `docs/concepts/data-assets-and-materialized-outputs/README.md`
- `docs/concepts/source-control-resolution-and-cache/README.md`
- `docs/concepts/dependency-aware-workflows/README.md`
- `internal/model/work_item.go`
- `internal/model/artifact_manifest.go`
- `internal/reposource/source_declaration.go`

Do not read worker, controller, scheduler, or transport files unless compile or test failures directly require them.

## Allowed Production Files

- `internal/model/data_location.go`
- `internal/model/data_provider.go`
- `internal/model/data_asset.go`
- `internal/model/data_archive.go`
- `internal/model/published_data_asset.go`
- `internal/model/work_item.go` only if adding optional bound data/publish fields is chosen in this slice

## Allowed Test Files

- `internal/model/data_location_test.go`
- `internal/model/data_provider_test.go`
- `internal/model/data_asset_test.go`
- `internal/model/data_archive_test.go`
- `internal/model/published_data_asset_test.go`
- `internal/model/work_item_test.go` only if `WorkItem` JSON round-trip changes

## Out Of Scope

- Controller workflow parsing of top-level `data_providers` or `published_data_assets` unless existing compiler plumbing makes it trivial and narrow.
- Worker downloading, copying, caching, or extracting assets.
- Invoking rclone or 7z.
- Writing `GOET_DATA_ASSETS_JSON`.
- Python runner changes.
- Command interpolation.
- Artifact publication/copying.
- Persistence schema changes.
- Sensitive variables or private credentials.
- Real CDL/Yan/Roy downloads.
- Real Google Drive access.
- Catalog registration.

## Acceptance Criteria

- A valid named `registered_location` validates.
- A valid `http` provider template with an HTTPS URL template and required parameters validates.
- A valid `local_file` provider template with configured location name and safe `path_template` validates.
- A valid `registered_location` provider template with a safe `path_template` validates.
- A valid `gdrive_rclone` provider template with remote and safe drive path template validates.
- A valid step data binding can be represented and JSON round-tripped.
- A provider template can be bound with concrete parameters into a `BoundDataAsset` or equivalent concrete declaration.
- Missing provider name, kind, provider type, URL/path template, rclone remote/path, or required parameter fails validation.
- Unsupported provider/location types fail validation unless explicitly documented as reserved.
- Invalid SHA-256 values fail validation.
- Unsafe cache keys, location path templates, archive member templates, archive `as` paths, and publish path templates are rejected using slash-relative path rules.
- `cache.immutable` is represented and defaults are documented.
- A valid integrity declaration can represent SHA-256 and size expectations.
- A valid ZIP archive selector with one required member validates.
- A valid `seven_zip` archive selector validates as a model declaration without requiring a real 7z binary in this slice.
- A valid published data asset target with a named location and relative path template validates.
- A publish binding can be represented without requiring runtime registration.
- Destructive overwrite policies are rejected unless explicitly implemented; `fail_if_exists` is accepted as the default.
- Materialized data asset manifests validate required binding name, kind, local path, provider type, and optional evidence fields.
- JSON round-trip tests preserve provider templates, cache policies, integrity expectations, archive selectors, bound data assets, materialized asset manifests, publish targets, and publish bindings.
- If `WorkItem` gains optional bound data/publish fields, existing work-item tests still pass and omission remains backward-compatible.
- `go test ./internal/model` passes.

## Notes

- CDL URLs and Yan/Roy tile paths should be examples in docs or fixtures, not hard-coded into the model.
- A missing expected hash may be allowed for exploratory runs, but the model should make it obvious that verified reuse requires stronger evidence.
- The worker should not receive unresolved `${year}` templates. Compilation should resolve ordinary variables into concrete bound asset locations before dispatch.
- `local_file` means worker-environment local, not controller-local.
- `gdrive_rclone` credentials and remote definitions belong to worker/container configuration, not workflow JSON.
