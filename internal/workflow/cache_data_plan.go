package workflow

import (
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"sort"
	"strings"

	fp "goetl/internal/fingerprint"
	"goetl/internal/model"
)

const defaultTargetEnvironmentID = "target-local"

func PlanCacheDataWorkItems(result CompileStageResult) (CompileStageResult, error) {
	if len(result.WorkItems) == 0 {
		return result, nil
	}

	cacheByAssetKey := map[string]CompileStageWorkItem{}
	cacheOrder := []string{}
	transformed := CompileStageResult{
		WorkflowID: result.WorkflowID,
		StageIndex: result.StageIndex,
		Steps:      result.Steps,
		WorkItems:  make([]CompileStageWorkItem, 0, len(result.WorkItems)),
	}

	for _, item := range result.WorkItems {
		assets, err := boundDataAssetsFromParameters(item.WorkItem.Parameters)
		if err != nil {
			return CompileStageResult{}, fmt.Errorf("plan cache_data for work item %s: %w", item.WorkItem.ID, err)
		}
		if len(assets) == 0 {
			transformed.WorkItems = append(transformed.WorkItems, item)
			continue
		}

		targetEnvironmentID, err := targetEnvironmentIDFromParameters(item.WorkItem.Parameters)
		if err != nil {
			return CompileStageResult{}, fmt.Errorf("plan cache_data for work item %s: %w", item.WorkItem.ID, err)
		}

		for _, asset := range assets {
			assetKey, err := CacheDataAssetKey(asset, targetEnvironmentID)
			if err != nil {
				return CompileStageResult{}, fmt.Errorf("plan cache_data for work item %s binding %s: %w", item.WorkItem.ID, asset.BindingName, err)
			}
			cacheItem, ok := cacheByAssetKey[assetKey]
			if !ok {
				payload, constraints, err := CacheDataPayload(asset, targetEnvironmentID, assetKey)
				if err != nil {
					return CompileStageResult{}, fmt.Errorf("plan cache_data for work item %s binding %s: %w", item.WorkItem.ID, asset.BindingName, err)
				}
				cacheItem = CompileStageWorkItem{
					WorkflowID:    item.WorkflowID,
					StageIndex:    item.StageIndex,
					StepIndex:     item.StepIndex,
					StepID:        item.StepID,
					WorkItemIndex: item.WorkItemIndex,
					WorkItem: model.WorkItem{
						ID:             cacheDataWorkItemID(assetKey),
						Type:           model.WorkItemTypeCacheData,
						OutputFilename: cacheDataOutputFilename(assetKey),
						Parameters: model.Parameters{
							"cache_data": {
								Type:  "cache_data",
								Value: payload,
							},
							"data_assets": {
								Type:  "data_assets",
								Value: []model.BoundDataAsset{asset},
							},
							"target_environment_id": {
								Type:  "string",
								Value: targetEnvironmentID,
							},
						},
					},
					ResourceConstraints: constraints,
				}
				cacheByAssetKey[assetKey] = cacheItem
				cacheOrder = append(cacheOrder, assetKey)
			}
			item.WorkItem.DependsOn = appendUniqueString(item.WorkItem.DependsOn, cacheItem.WorkItem.ID)
		}
		transformed.WorkItems = append(transformed.WorkItems, item)
	}

	sort.Strings(cacheOrder)
	planned := CompileStageResult{
		WorkflowID: transformed.WorkflowID,
		StageIndex: transformed.StageIndex,
		Steps:      transformed.Steps,
		WorkItems:  make([]CompileStageWorkItem, 0, len(cacheOrder)+len(transformed.WorkItems)),
	}
	for _, assetKey := range cacheOrder {
		planned.WorkItems = append(planned.WorkItems, cacheByAssetKey[assetKey])
	}
	planned.WorkItems = append(planned.WorkItems, transformed.WorkItems...)
	return planned, nil
}

func CacheDataAssetKey(asset model.BoundDataAsset, targetEnvironmentID string) (string, error) {
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

func CacheDataPayload(asset model.BoundDataAsset, targetEnvironmentID string, assetKey string) (model.CacheDataWorkItemPayload, []model.WorkItemResourceConstraint, error) {
	constraints, err := CacheDataResourceConstraints(asset, targetEnvironmentID)
	if err != nil {
		return model.CacheDataWorkItemPayload{}, nil, err
	}
	limits := model.DataAssetTransferLimits{}
	if asset.TransferPolicy.MaxBytesPerSecond > 0 {
		limits.MaxBytesPerSecond = asset.TransferPolicy.MaxBytesPerSecond
	}
	return model.CacheDataWorkItemPayload{
		Operator:            string(model.WorkItemTypeCacheData),
		TargetEnvironmentID: targetEnvironmentID,
		AssetKey:            assetKey,
		DedupeKey:           fmt.Sprintf("cache_data:%s:%s", targetEnvironmentID, assetKey),
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

func CacheDataResourceConstraints(asset model.BoundDataAsset, targetEnvironmentID string) ([]model.WorkItemResourceConstraint, error) {
	if err := asset.Validate(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(targetEnvironmentID) == "" {
		return nil, fmt.Errorf("target_environment_id is required")
	}
	sourceKey, err := cacheDataProviderResourceKey(asset)
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

func cacheDataProviderResourceKey(asset model.BoundDataAsset) (string, error) {
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

func cacheDataWorkItemID(assetKey string) string {
	return "cache-data-" + strings.TrimPrefix(assetKey, "sha256:")
}

func cacheDataOutputFilename(assetKey string) string {
	return cacheDataWorkItemID(assetKey) + ".json"
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
