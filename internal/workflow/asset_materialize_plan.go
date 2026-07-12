package workflow

import (
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strings"

	fp "goetl/internal/fingerprint"
	"goetl/internal/model"
)

const defaultTargetEnvironmentID = "target-local"

func AssetMaterializeAssetKey(asset model.BoundDataAsset, targetEnvironmentID string) (string, error) {
	if err := asset.Validate(); err != nil {
		return "", err
	}
	if strings.TrimSpace(targetEnvironmentID) == "" {
		return "", fmt.Errorf("target_environment_id is required")
	}
	identity := map[string]any{
		"provider_type":            asset.Provider,
		"resolved_source_location": asset.Location,
		"resolved_parameters":      asset.Parameters,
		"cache_strategy":           effectiveDataAssetCacheStrategy(asset),
		"cache_key":                asset.Cache.CacheKey,
		"immutable":                asset.Cache.EffectiveImmutable(),
		"integrity_expectations":   asset.Integrity,
		"archive_selection":        asset.Archive,
		"expose_mode":              archiveExposeMode(asset.Archive),
		"target_environment_id":    targetEnvironmentID,
	}
	_, hash, err := fp.CanonicalJSONSHA256(normalizedCanonicalValue(identity))
	if err != nil {
		return "", err
	}
	return "sha256:" + hash, nil
}

func AssetMaterializePayload(asset model.BoundDataAsset, targetEnvironmentID string, assetKey string) (model.AssetMaterializeWorkItemPayload, []model.WorkItemResourceConstraint, error) {
	constraints, err := AssetMaterializeResourceConstraints(asset, targetEnvironmentID)
	if err != nil {
		return model.AssetMaterializeWorkItemPayload{}, nil, err
	}
	limits := model.DataAssetTransferLimits{}
	if asset.TransferPolicy.MaxBytesPerSecond > 0 {
		limits.MaxBytesPerSecond = asset.TransferPolicy.MaxBytesPerSecond
	}
	return model.AssetMaterializeWorkItemPayload{
		Operator:            string(model.WorkItemTypeAssetMaterialize),
		TargetEnvironmentID: targetEnvironmentID,
		AssetKey:            assetKey,
		DedupeKey:           fmt.Sprintf("asset_materialize:%s:%s", targetEnvironmentID, assetKey),
		BindingName:         asset.BindingName,
		ProviderName:        asset.ProviderName,
		ProviderType:        asset.Provider,
		Kind:                asset.Kind,
		Format:              asset.Format,
		ResolvedLocation:    asset.Location,
		Cache:               asset.Cache,
		Integrity:           asset.Integrity,
		Archive:             asset.Archive,
		ResourceConstraints: constraints,
		TransferPolicy:      asset.TransferPolicy,
		TransferLimits:      limits,
		Parameters:          asset.Parameters,
		Metadata:            asset.Metadata,
	}, constraints, nil
}

func AssetMaterializeResourceConstraints(asset model.BoundDataAsset, targetEnvironmentID string) ([]model.WorkItemResourceConstraint, error) {
	if err := asset.Validate(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(targetEnvironmentID) == "" {
		return nil, fmt.Errorf("target_environment_id is required")
	}
	sourceKey, err := AssetMaterializeProviderResourceKey(asset)
	if err != nil {
		return nil, err
	}
	targetUnits := asset.TransferPolicy.MaxConcurrentSourceTransfers
	if targetUnits == 0 {
		targetUnits = 1
	}
	return []model.WorkItemResourceConstraint{
		{
			ConstraintIndex: 0,
			ResourceKey:     sourceKey,
			RequestedUnits:  1,
			Operator:        model.WorkItemResourceConstraintOperatorLessEq,
			TargetUnits:     targetUnits,
		},
	}, nil
}

func CommitDataResourceConstraints(target model.BoundPublishTarget, targetEnvironmentID string) ([]model.WorkItemResourceConstraint, error) {
	if err := target.Validate(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(targetEnvironmentID) == "" {
		return nil, fmt.Errorf("target_environment_id is required")
	}
	resourceKey := ""
	switch target.Location.Type {
	case model.DataProviderRegisteredLocation:
		resourceKey = "target:" + sanitizeResourceKeySegment(targetEnvironmentID) + "/published-data-write:" + sanitizeResourceKeySegment(target.Location.LocationName)
	case model.DataProviderGDriveRclone:
		resourceKey = "provider:gdrive-rclone:" + sanitizeResourceKeySegment(target.Location.Remote) + "/upload"
	default:
		return nil, fmt.Errorf("unsupported commit_data publish target location type %q", target.Location.Type)
	}
	return []model.WorkItemResourceConstraint{
		{
			ConstraintIndex: 0,
			ResourceKey:     resourceKey,
			RequestedUnits:  1,
			Operator:        model.WorkItemResourceConstraintOperatorLessEq,
			TargetUnits:     1,
		},
	}, nil
}

func AssetMaterializeProviderResourceKey(asset model.BoundDataAsset) (string, error) {
	switch asset.Provider {
	case model.DataProviderHTTP:
		parsed, err := url.Parse(asset.Location.URI)
		if err != nil {
			return "", fmt.Errorf("parse http data asset uri: %w", err)
		}
		host := parsed.Hostname()
		if host == "" {
			host = parsed.Host
			if h, _, err := net.SplitHostPort(host); err == nil {
				host = h
			}
		}
		return "provider:http:" + sanitizeResourceKeySegment(host) + "/download", nil
	case model.DataProviderGDriveRclone:
		return "provider:gdrive-rclone:" + sanitizeResourceKeySegment(asset.Location.Remote) + "/download", nil
	case model.DataProviderLocalFile:
		return "provider:local-file:" + sanitizeResourceKeySegment(asset.Location.LocationName) + "/read", nil
	case model.DataProviderRegisteredLocation:
		return "provider:registered-location:" + sanitizeResourceKeySegment(asset.Location.LocationName) + "/read", nil
	default:
		return "", fmt.Errorf("unsupported data provider %q", asset.Provider)
	}
}

func sanitizeResourceKeySegment(value string) string {
	trimmed := strings.TrimSpace(strings.ToLower(value))
	if trimmed == "" {
		return "unknown"
	}
	sanitized := resourceKeyUnsafeSegmentPattern.ReplaceAllString(trimmed, "-")
	sanitized = strings.Trim(sanitized, "-")
	if sanitized == "" {
		return "unknown"
	}
	return sanitized
}

var resourceKeyUnsafeSegmentPattern = regexp.MustCompile(`[^a-z0-9._-]+`)

func boundDataAssetsFromParameters(parameters model.Parameters) ([]model.BoundDataAsset, error) {
	parameter, ok := parameters["data_assets"]
	if !ok {
		return nil, nil
	}
	if parameter.Type != "data_assets" && parameter.Type != "list" {
		return nil, fmt.Errorf("parameter data_assets has type %s, want data_assets or list", parameter.Type)
	}

	data, err := json.Marshal(parameter.Value)
	if err != nil {
		return nil, fmt.Errorf("encode data_assets parameter: %w", err)
	}
	var assets []model.BoundDataAsset
	if err := json.Unmarshal(data, &assets); err != nil {
		return nil, fmt.Errorf("decode data_assets parameter: %w", err)
	}
	for i, asset := range assets {
		if err := asset.Validate(); err != nil {
			return nil, fmt.Errorf("data_assets[%d]: %w", i, err)
		}
	}
	return assets, nil
}

func targetEnvironmentIDFromParameters(parameters model.Parameters) (string, error) {
	parameter, ok := parameters["target_environment_id"]
	if !ok {
		return defaultTargetEnvironmentID, nil
	}
	if parameter.Type != "string" {
		return "", fmt.Errorf("parameter target_environment_id has type %s, want string", parameter.Type)
	}
	value, ok := parameter.Value.(string)
	if !ok || strings.TrimSpace(value) == "" {
		return "", fmt.Errorf("parameter target_environment_id value must be a non-empty string")
	}
	if strings.TrimSpace(value) != value {
		return "", fmt.Errorf("parameter target_environment_id must not contain leading or trailing whitespace")
	}
	return value, nil
}

func effectiveDataAssetCacheStrategy(asset model.BoundDataAsset) string {
	if asset.Cache.Strategy != "" {
		return asset.Cache.Strategy
	}
	if asset.Materialization.Strategy != "" {
		return asset.Materialization.Strategy
	}
	switch asset.Provider {
	case model.DataProviderHTTP, model.DataProviderGDriveRclone:
		return model.DataAssetCacheStrategyWorkerCache
	case model.DataProviderLocalFile, model.DataProviderRegisteredLocation:
		return model.DataAssetCacheStrategyReference
	default:
		return ""
	}
}

func archiveExposeMode(archive *model.DataAssetArchive) string {
	if archive == nil {
		return ""
	}
	return archive.Expose
}

func appendUniqueString(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func normalizedCanonicalValue(value any) any {
	data, err := json.Marshal(value)
	if err != nil {
		return value
	}
	var decoded any
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.UseNumber()
	if err := decoder.Decode(&decoded); err != nil {
		return value
	}
	return decoded
}
