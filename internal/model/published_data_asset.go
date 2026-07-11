package model

import (
	"fmt"
	"strings"
)

const (
	PublishedDataAssetManifestSchemaV1      = "goet/published-data-assets/v1"
	PublishedDataAssetOverwriteFailIfExists = "fail_if_exists"
)

type PublishedDataAssetTarget struct {
	Name            string                   `json:"name"`
	Kind            string                   `json:"kind"`
	Format          string                   `json:"format,omitempty"`
	Location        DataLocationPathTemplate `json:"location"`
	Parameters      []string                 `json:"parameters,omitempty"`
	OverwritePolicy string                   `json:"overwrite_policy,omitempty"`
	Metadata        map[string]any           `json:"metadata,omitempty"`
}

type PublishedDataAsset struct {
	Name            string `json:"name"`
	FromWorkItemID  string `json:"from_work_item_id,omitempty"`
	FromArtifact    string `json:"from_artifact"`
	ContentType     string `json:"content_type,omitempty"`
	StorageScope    string `json:"storage_scope"`
	LocationName    string `json:"location_name"`
	Path            string `json:"path"`
	SizeBytes       *int64 `json:"size_bytes,omitempty"`
	SHA256          string `json:"sha256,omitempty"`
	OverwritePolicy string `json:"overwrite_policy,omitempty"`
}

type PublishedDataAssetManifest struct {
	Schema              string               `json:"schema"`
	TargetEnvironmentID string               `json:"target_environment_id"`
	PublishedAssets     []PublishedDataAsset `json:"published_assets"`
}

type BoundPublishTarget struct {
	Name            string            `json:"name"`
	FromArtifact    string            `json:"from_artifact"`
	TargetName      string            `json:"target_name,omitempty"`
	Target          string            `json:"target,omitempty"`
	Location        DataAssetLocation `json:"location"`
	OverwritePolicy string            `json:"overwrite_policy,omitempty"`
	Parameters      map[string]any    `json:"parameters,omitempty"`
	Metadata        map[string]any    `json:"metadata,omitempty"`
}

func (target PublishedDataAssetTarget) Validate() error {
	if err := validateDataName(target.Name, "published data asset target name"); err != nil {
		return err
	}
	if strings.TrimSpace(target.Kind) == "" {
		return fmt.Errorf("published data asset target kind is required")
	}
	if err := target.Location.Validate(); err != nil {
		return err
	}
	declared, err := validateParameterNames(target.Parameters)
	if err != nil {
		return err
	}
	for _, name := range templateParameterNames(target.Location.PathTemplate) {
		if _, ok := declared[name]; !ok {
			return fmt.Errorf("published data asset location path_template references undeclared parameter %q", name)
		}
	}
	return validateOverwritePolicy(target.OverwritePolicy)
}

func (target PublishedDataAssetTarget) Bind(name, fromArtifact string, parameters map[string]any) (BoundPublishTarget, error) {
	if err := target.Validate(); err != nil {
		return BoundPublishTarget{}, err
	}
	if err := validateDataName(name, "publish binding name"); err != nil {
		return BoundPublishTarget{}, err
	}
	if err := validateDataName(fromArtifact, "publish binding from_artifact"); err != nil {
		return BoundPublishTarget{}, err
	}
	for _, parameterName := range target.Parameters {
		value, ok := parameters[parameterName]
		if !ok || value == nil {
			return BoundPublishTarget{}, fmt.Errorf("required parameter %q is missing", parameterName)
		}
	}
	bound := BoundPublishTarget{
		Name:            name,
		FromArtifact:    fromArtifact,
		TargetName:      target.Name,
		OverwritePolicy: target.OverwritePolicy,
		Parameters:      parameters,
		Metadata:        target.Metadata,
		Location: DataAssetLocation{
			Type:         DataProviderRegisteredLocation,
			LocationName: target.Location.Name,
			Path:         renderTemplate(target.Location.PathTemplate, parameters),
		},
	}
	if err := bound.Validate(); err != nil {
		return BoundPublishTarget{}, err
	}
	return bound, nil
}

func (target BoundPublishTarget) Validate() error {
	if err := validateDataName(target.Name, "publish binding name"); err != nil {
		return err
	}
	if err := validateDataName(target.FromArtifact, "publish binding from_artifact"); err != nil {
		return err
	}
	effectiveTargetName := target.TargetName
	if effectiveTargetName == "" {
		effectiveTargetName = target.Target
	}
	if target.TargetName != "" && target.Target != "" && target.TargetName != target.Target {
		return fmt.Errorf("publish binding target_name does not match target")
	}
	if err := validateDataName(effectiveTargetName, "publish binding target_name"); err != nil {
		return err
	}
	if err := target.Location.Validate(); err != nil {
		return err
	}
	return validateOverwritePolicy(target.OverwritePolicy)
}

func (asset PublishedDataAsset) Validate() error {
	if err := validateDataName(asset.Name, "published data asset name"); err != nil {
		return err
	}
	if strings.TrimSpace(asset.FromWorkItemID) == "" && asset.FromWorkItemID != "" {
		return fmt.Errorf("published data asset from_work_item_id must not be empty when set")
	}
	if err := validateDataName(asset.FromArtifact, "published data asset from_artifact"); err != nil {
		return err
	}
	switch asset.StorageScope {
	case DataLocationTypeRegistered:
		if err := validateDataName(asset.LocationName, "published data asset location_name"); err != nil {
			return err
		}
		if _, err := ValidateArtifactRelativePath(asset.Path); err != nil {
			return err
		}
	case DataProviderGDriveRclone:
		if err := validateRcloneRemote(asset.LocationName); err != nil {
			return fmt.Errorf("published data asset gdrive remote: %w", err)
		}
		if _, err := validateDataRelativePath(asset.Path, "published data asset gdrive path"); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unsupported published data asset storage scope %q", asset.StorageScope)
	}
	if err := validateOptionalSize("published data asset size_bytes", asset.SizeBytes); err != nil {
		return err
	}
	if err := validateOptionalSHA256("published data asset sha256", asset.SHA256); err != nil {
		return err
	}
	return validateOverwritePolicy(asset.OverwritePolicy)
}

func (manifest PublishedDataAssetManifest) EffectiveSchema() string {
	if strings.TrimSpace(manifest.Schema) == "" {
		return PublishedDataAssetManifestSchemaV1
	}
	return manifest.Schema
}

func (manifest PublishedDataAssetManifest) Validate() error {
	if manifest.EffectiveSchema() != PublishedDataAssetManifestSchemaV1 {
		return fmt.Errorf("unsupported published data asset manifest schema: %s", manifest.Schema)
	}
	if strings.TrimSpace(manifest.TargetEnvironmentID) == "" {
		return fmt.Errorf("published data asset manifest target_environment_id is required")
	}
	if len(manifest.PublishedAssets) == 0 {
		return fmt.Errorf("at least one published asset is required")
	}
	for i, asset := range manifest.PublishedAssets {
		if err := asset.Validate(); err != nil {
			return fmt.Errorf("published asset %d: %w", i, err)
		}
	}
	return nil
}

func validateOverwritePolicy(policy string) error {
	switch policy {
	case "", PublishedDataAssetOverwriteFailIfExists:
		return nil
	default:
		return fmt.Errorf("unsupported published data asset overwrite policy %q", policy)
	}
}
