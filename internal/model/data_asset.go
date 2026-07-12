package model

import (
	"fmt"
	"strings"
)

const MaterializedDataAssetManifestSchemaV1 = "goet/materialized-data-assets/v1"

const (
	DataAssetCacheStrategyWorkerCache = "worker_cache"
	DataAssetCacheStrategyReference   = "reference"
)

type StepDataBinding struct {
	BindingName  string         `json:"binding_name"`
	ProviderName string         `json:"provider_name"`
	Parameters   map[string]any `json:"parameters,omitempty"`
	Metadata     map[string]any `json:"metadata,omitempty"`
}

type BoundDataAsset struct {
	BindingName     string                   `json:"binding_name"`
	ProviderName    string                   `json:"provider_name"`
	Kind            string                   `json:"kind"`
	Format          string                   `json:"format,omitempty"`
	Provider        string                   `json:"provider"`
	Location        DataAssetLocation        `json:"location"`
	Integrity       DataAssetIntegrity       `json:"integrity,omitempty"`
	Cache           DataAssetCache           `json:"cache,omitempty"`
	Archive         *DataAssetArchive        `json:"archive,omitempty"`
	Materialization DataAssetMaterialization `json:"materialization,omitempty"`
	TransferPolicy  DataAssetTransferPolicy  `json:"transfer_policy,omitempty"`
	Parameters      map[string]any           `json:"parameters,omitempty"`
	Metadata        map[string]any           `json:"metadata,omitempty"`
}

type DataAssetLocation struct {
	Type         string `json:"type"`
	URI          string `json:"uri,omitempty"`
	LocationName string `json:"location_name,omitempty"`
	Path         string `json:"path,omitempty"`
	Remote       string `json:"remote,omitempty"`
	DrivePath    string `json:"drive_path,omitempty"`
	FileID       string `json:"file_id,omitempty"`
}

type DataAssetIntegrityTemplate struct {
	SHA256Template string `json:"sha256,omitempty"`
	SizeBytes      *int64 `json:"size_bytes,omitempty"`
	Required       bool   `json:"required,omitempty"`
}

type DataAssetIntegrity struct {
	SHA256    string `json:"sha256,omitempty"`
	SizeBytes *int64 `json:"size_bytes,omitempty"`
	Required  bool   `json:"required,omitempty"`
}

type DataAssetCacheTemplate struct {
	Strategy         string `json:"strategy,omitempty"`
	CacheKeyTemplate string `json:"cache_key_template,omitempty"`
	Immutable        *bool  `json:"immutable,omitempty"`
}

type DataAssetCache struct {
	Strategy  string `json:"strategy,omitempty"`
	CacheKey  string `json:"cache_key,omitempty"`
	Immutable *bool  `json:"immutable,omitempty"`
}

type DataAssetMaterializationTemplate struct {
	Scope        string `json:"scope,omitempty"`
	Strategy     string `json:"strategy,omitempty"`
	PathTemplate string `json:"path_template,omitempty"`
}

type DataAssetMaterialization struct {
	Scope        string `json:"scope,omitempty"`
	Strategy     string `json:"strategy,omitempty"`
	PathTemplate string `json:"path_template,omitempty"`
}

type DataAssetTransferPolicy struct {
	MaxConcurrentSourceTransfers   int               `json:"max_concurrent_source_transfers,omitempty"`
	RequestedBandwidthMiBPerSecond int64             `json:"requested_bandwidth_mib_per_second,omitempty"`
	MaxBytesPerSecond              int64             `json:"max_bytes_per_second,omitempty"`
	ProviderArgs                   map[string]string `json:"provider_args,omitempty"`
}

type MaterializedDataAssetManifest struct {
	Schema              string                  `json:"schema"`
	AssetKey            string                  `json:"asset_key,omitempty"`
	TargetEnvironmentID string                  `json:"target_environment_id,omitempty"`
	Assets              []MaterializedDataAsset `json:"assets"`
}

type MaterializedDataAsset struct {
	BindingName             string                                 `json:"binding_name"`
	ProviderName            string                                 `json:"provider_name,omitempty"`
	ProviderType            string                                 `json:"provider_type,omitempty"`
	Kind                    string                                 `json:"kind"`
	Format                  string                                 `json:"format,omitempty"`
	LocalPath               string                                 `json:"local_path"`
	MaterializationKey      string                                 `json:"materialization_key,omitempty"`
	MaterializationDomainID string                                 `json:"materialization_domain_id,omitempty"`
	DestinationRelativePath string                                 `json:"destination_relative_path,omitempty"`
	DestinationSizeBytes    *int64                                 `json:"destination_size_bytes,omitempty"`
	DestinationSHA256       string                                 `json:"destination_sha256,omitempty"`
	MaterializationStrategy string                                 `json:"materialization_strategy,omitempty"`
	CacheKey                string                                 `json:"cache_key,omitempty"`
	CacheImmutable          *bool                                  `json:"cache_immutable,omitempty"`
	SourceSizeBytes         *int64                                 `json:"source_size_bytes,omitempty"`
	SourceSHA256            string                                 `json:"source_sha256,omitempty"`
	SelectedSizeBytes       *int64                                 `json:"selected_size_bytes,omitempty"`
	SelectedSHA256          string                                 `json:"selected_sha256,omitempty"`
	ArchiveType             string                                 `json:"archive_type,omitempty"`
	ArchiveMembers          []MaterializedArchiveMember            `json:"archive_members,omitempty"`
	Metadata                map[string]any                         `json:"metadata,omitempty"`
	CollectionMember        *MaterializedDataAssetCollectionMember `json:"collection_member,omitempty"`
}

type MaterializedArchiveMember struct {
	Member    string `json:"member"`
	LocalPath string `json:"local_path"`
	SizeBytes *int64 `json:"size_bytes,omitempty"`
	SHA256    string `json:"sha256,omitempty"`
}

func (binding StepDataBinding) Validate() error {
	if err := validateDataName(binding.BindingName, "data binding name"); err != nil {
		return err
	}
	if err := validateDataName(binding.ProviderName, "data binding provider_name"); err != nil {
		return err
	}
	return nil
}

func (asset BoundDataAsset) Validate() error {
	if err := validateDataName(asset.BindingName, "bound data asset binding_name"); err != nil {
		return err
	}
	if err := validateDataName(asset.ProviderName, "bound data asset provider_name"); err != nil {
		return err
	}
	if strings.TrimSpace(asset.Kind) == "" {
		return fmt.Errorf("bound data asset kind is required")
	}
	if !isSupportedDataProvider(asset.Provider) {
		return fmt.Errorf("unsupported data provider %q", asset.Provider)
	}
	if err := asset.Location.Validate(); err != nil {
		return err
	}
	if err := asset.Integrity.Validate(); err != nil {
		return err
	}
	if err := asset.Cache.Validate(); err != nil {
		return err
	}
	if asset.Archive != nil {
		if err := asset.Archive.Validate(); err != nil {
			return err
		}
	}
	if err := asset.Materialization.Validate(); err != nil {
		return err
	}
	return asset.TransferPolicy.Validate()
}

func (location DataAssetLocation) Validate() error {
	if !isSupportedDataProvider(location.Type) {
		return fmt.Errorf("unsupported data asset location type %q", location.Type)
	}
	switch location.Type {
	case DataProviderHTTP:
		if strings.TrimSpace(location.URI) == "" {
			return fmt.Errorf("data asset location uri is required")
		}
		if !isHTTPURI(location.URI) {
			return fmt.Errorf("data asset location uri must use http or https")
		}
	case DataProviderLocalFile, DataProviderRegisteredLocation:
		if err := validateDataName(location.LocationName, "data asset location_name"); err != nil {
			return err
		}
		if _, err := validateDataRelativePath(location.Path, "data asset location path"); err != nil {
			return err
		}
	case DataProviderGDriveRclone:
		if err := validateRcloneRemote(location.Remote); err != nil {
			return err
		}
		if _, err := validateDataRelativePath(location.DrivePath, "data asset drive_path"); err != nil {
			return err
		}
		if strings.TrimSpace(location.FileID) != location.FileID {
			return fmt.Errorf("data asset file_id must not contain leading or trailing whitespace")
		}
	}
	return nil
}

func (integrity DataAssetIntegrityTemplate) Validate() error {
	if len(templateParameterNames(integrity.SHA256Template)) > 0 {
		if err := validateOptionalSize("size_bytes", integrity.SizeBytes); err != nil {
			return err
		}
		if integrity.Required && integrity.SHA256Template == "" && integrity.SizeBytes == nil {
			return fmt.Errorf("required integrity needs sha256 or size_bytes")
		}
		return nil
	}
	return validateIntegrity(integrity.SHA256Template, integrity.SizeBytes, integrity.Required)
}

func (integrity DataAssetIntegrity) Validate() error {
	return validateIntegrity(integrity.SHA256, integrity.SizeBytes, integrity.Required)
}

func (cache DataAssetCacheTemplate) Validate() error {
	if err := validateCacheStrategy(cache.Strategy); err != nil {
		return err
	}
	if cache.CacheKeyTemplate != "" {
		if _, err := validateDataRelativePath(cache.CacheKeyTemplate, "cache_key_template"); err != nil {
			return err
		}
	}
	return nil
}

func (cache DataAssetCache) Validate() error {
	if err := validateCacheStrategy(cache.Strategy); err != nil {
		return err
	}
	if cache.CacheKey != "" {
		if _, err := validateDataRelativePath(cache.CacheKey, "cache_key"); err != nil {
			return err
		}
	}
	return nil
}

func (cache DataAssetCacheTemplate) EffectiveImmutable() bool {
	if cache.Immutable != nil {
		return *cache.Immutable
	}
	return cache.Strategy == DataAssetCacheStrategyWorkerCache
}

func (cache DataAssetCache) EffectiveImmutable() bool {
	if cache.Immutable != nil {
		return *cache.Immutable
	}
	return cache.Strategy == DataAssetCacheStrategyWorkerCache
}

func (materialization DataAssetMaterializationTemplate) Validate() error {
	if err := ValidateMaterializationScope(materialization.Scope); err != nil {
		return err
	}
	if materialization.PathTemplate != "" {
		if _, err := ValidateArtifactRelativePath(materialization.PathTemplate); err != nil {
			return fmt.Errorf("materialization path_template: %w", err)
		}
	}
	return validateMaterializationStrategy(materialization.Strategy)
}

func (materialization DataAssetMaterialization) Validate() error {
	if err := ValidateMaterializationScope(materialization.Scope); err != nil {
		return err
	}
	if materialization.PathTemplate != "" {
		if _, err := ValidateArtifactRelativePath(materialization.PathTemplate); err != nil {
			return fmt.Errorf("materialization path_template: %w", err)
		}
	}
	return validateMaterializationStrategy(materialization.Strategy)
}

func (policy DataAssetTransferPolicy) Validate() error {
	if policy.MaxConcurrentSourceTransfers < 0 {
		return fmt.Errorf("max_concurrent_source_transfers must be non-negative")
	}
	if policy.RequestedBandwidthMiBPerSecond < 0 {
		return fmt.Errorf("requested_bandwidth_mib_per_second must be non-negative")
	}
	if policy.MaxBytesPerSecond < 0 {
		return fmt.Errorf("max_bytes_per_second must be non-negative")
	}
	for key, value := range policy.ProviderArgs {
		if strings.TrimSpace(key) == "" {
			return fmt.Errorf("transfer policy provider arg key is required")
		}
		if key != "rclone_bwlimit" {
			return fmt.Errorf("unsupported transfer policy provider arg %q", key)
		}
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("transfer policy provider arg %s value is required", key)
		}
		if strings.TrimSpace(value) != value {
			return fmt.Errorf("transfer policy provider arg %s must not contain leading or trailing whitespace", key)
		}
	}
	return nil
}

func (manifest MaterializedDataAssetManifest) EffectiveSchema() string {
	if strings.TrimSpace(manifest.Schema) == "" {
		return MaterializedDataAssetManifestSchemaV1
	}
	return manifest.Schema
}

func (manifest MaterializedDataAssetManifest) Validate() error {
	if manifest.EffectiveSchema() != MaterializedDataAssetManifestSchemaV1 {
		return fmt.Errorf("unsupported materialized data asset manifest schema %q", manifest.Schema)
	}
	if manifest.AssetKey != "" {
		if !strings.HasPrefix(manifest.AssetKey, "sha256:") {
			return fmt.Errorf("materialized data asset manifest asset_key must use sha256: prefix")
		}
		if err := validateOptionalSHA256("materialized data asset manifest asset_key", strings.TrimPrefix(manifest.AssetKey, "sha256:")); err != nil {
			return err
		}
	}
	if strings.TrimSpace(manifest.TargetEnvironmentID) != manifest.TargetEnvironmentID {
		return fmt.Errorf("materialized data asset manifest target_environment_id must not contain leading or trailing whitespace")
	}
	if len(manifest.Assets) == 0 {
		return fmt.Errorf("at least one materialized data asset is required")
	}
	for i, asset := range manifest.Assets {
		if err := asset.Validate(); err != nil {
			return fmt.Errorf("materialized data asset %d: %w", i, err)
		}
	}
	return nil
}

func (asset MaterializedDataAsset) Validate() error {
	if err := validateDataName(asset.BindingName, "materialized data asset binding_name"); err != nil {
		return err
	}
	if strings.TrimSpace(asset.Kind) == "" {
		return fmt.Errorf("materialized data asset kind is required")
	}
	if !isSupportedDataProvider(asset.ProviderType) {
		return fmt.Errorf("unsupported materialized data asset provider_type %q", asset.ProviderType)
	}
	if strings.TrimSpace(asset.LocalPath) == "" {
		return fmt.Errorf("materialized data asset local_path is required")
	}
	if asset.MaterializationKey != "" {
		if err := validatePrefixedSHA256("materialized data asset materialization_key", asset.MaterializationKey); err != nil {
			return err
		}
	}
	if strings.TrimSpace(asset.MaterializationDomainID) != asset.MaterializationDomainID {
		return fmt.Errorf("materialized data asset materialization_domain_id must not contain leading or trailing whitespace")
	}
	if asset.DestinationRelativePath != "" {
		if _, err := ValidateArtifactRelativePath(asset.DestinationRelativePath); err != nil {
			return fmt.Errorf("materialized data asset destination_relative_path: %w", err)
		}
	}
	if asset.CacheKey != "" {
		if _, err := validateDataRelativePath(asset.CacheKey, "materialized data asset cache_key"); err != nil {
			return err
		}
	}
	if err := validateOptionalSize("source_size_bytes", asset.SourceSizeBytes); err != nil {
		return err
	}
	if err := validateOptionalSize("selected_size_bytes", asset.SelectedSizeBytes); err != nil {
		return err
	}
	if err := validateOptionalSize("destination_size_bytes", asset.DestinationSizeBytes); err != nil {
		return err
	}
	if err := validateOptionalSHA256("source_sha256", asset.SourceSHA256); err != nil {
		return err
	}
	if err := validateOptionalSHA256("selected_sha256", asset.SelectedSHA256); err != nil {
		return err
	}
	if err := validateOptionalSHA256("destination_sha256", asset.DestinationSHA256); err != nil {
		return err
	}
	if asset.ArchiveType != "" {
		if err := validateArchiveType(asset.ArchiveType); err != nil {
			return err
		}
	}
	for i, member := range asset.ArchiveMembers {
		if err := member.Validate(); err != nil {
			return fmt.Errorf("archive member %d: %w", i, err)
		}
	}
	if asset.CollectionMember != nil {
		if err := asset.CollectionMember.Validate(); err != nil {
			return fmt.Errorf("collection_member: %w", err)
		}
	}
	return nil
}

func (member MaterializedArchiveMember) Validate() error {
	if _, err := validateDataRelativePath(member.Member, "materialized archive member"); err != nil {
		return err
	}
	if strings.TrimSpace(member.LocalPath) == "" {
		return fmt.Errorf("materialized archive member local_path is required")
	}
	if err := validateOptionalSize("archive member size_bytes", member.SizeBytes); err != nil {
		return err
	}
	return validateOptionalSHA256("archive member sha256", member.SHA256)
}

func validateIntegrity(sha256 string, sizeBytes *int64, required bool) error {
	if err := validateOptionalSHA256("sha256", sha256); err != nil {
		return err
	}
	if err := validateOptionalSize("size_bytes", sizeBytes); err != nil {
		return err
	}
	if required && sha256 == "" && sizeBytes == nil {
		return fmt.Errorf("required integrity needs sha256 or size_bytes")
	}
	return nil
}

func validateOptionalSHA256(field, value string) error {
	if value == "" {
		return nil
	}
	if len(value) != 64 {
		return fmt.Errorf("%s must be a 64-character lowercase SHA-256 hex value", field)
	}
	for _, r := range value {
		if (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') {
			continue
		}
		return fmt.Errorf("%s must be a 64-character lowercase SHA-256 hex value", field)
	}
	return nil
}

func validateOptionalSize(field string, value *int64) error {
	if value != nil && *value < 0 {
		return fmt.Errorf("%s must be non-negative", field)
	}
	return nil
}

func validateCacheStrategy(strategy string) error {
	switch strategy {
	case "", DataAssetCacheStrategyWorkerCache, DataAssetCacheStrategyReference:
		return nil
	default:
		return fmt.Errorf("unsupported cache strategy %q", strategy)
	}
}

func validateMaterializationStrategy(strategy string) error {
	switch strategy {
	case "", DataAssetCacheStrategyWorkerCache, DataAssetCacheStrategyReference:
		return nil
	default:
		return fmt.Errorf("unsupported materialization strategy %q", strategy)
	}
}
