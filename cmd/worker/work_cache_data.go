package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"goetl/internal/model"
)

func (w Worker) cacheData(item model.WorkItem) (WorkEvidence, error) {
	payload, asset, err := cacheDataPayloadFromWorkItem(item)
	if err != nil {
		return WorkEvidence{}, err
	}

	attemptID := item.AttemptID
	if attemptID == "" {
		attemptID = item.ID + "-attempt"
	}
	workDir := filepath.Join(w.Config.TmpDir, "attempts", attemptID, "work")
	if err := os.MkdirAll(workDir, 0755); err != nil {
		return WorkEvidence{}, fmt.Errorf("create cache_data work dir %s: %w", workDir, err)
	}

	preState := map[string]any{
		"operator":              string(model.WorkItemTypeCacheData),
		"asset_key":             payload.AssetKey,
		"target_environment_id": payload.TargetEnvironmentID,
	}
	preStateSHA256, err := canonicalObservationSHA256(preState)
	if err != nil {
		return WorkEvidence{}, err
	}

	materializer := assetMaterializer{config: w.Config, workDir: workDir}
	materialized, err := materializer.materialize(asset)
	if err != nil {
		return WorkEvidence{}, err
	}
	manifest := model.MaterializedDataAssetManifest{
		Schema:              model.MaterializedDataAssetManifestSchemaV1,
		AssetKey:            payload.AssetKey,
		TargetEnvironmentID: payload.TargetEnvironmentID,
		Assets:              []model.MaterializedDataAsset{materialized},
	}
	if err := manifest.Validate(); err != nil {
		return WorkEvidence{}, fmt.Errorf("validate cache_data manifest: %w", err)
	}
	manifestJSON, err := json.Marshal(manifest)
	if err != nil {
		return WorkEvidence{}, fmt.Errorf("encode cache_data manifest: %w", err)
	}
	outputJSON, outputSHA256, _, err := canonicalJSONDocument(manifestJSON, "cache_data manifest")
	if err != nil {
		return WorkEvidence{}, err
	}

	postState := map[string]any{
		"operator":              string(model.WorkItemTypeCacheData),
		"asset_key":             payload.AssetKey,
		"target_environment_id": payload.TargetEnvironmentID,
		"output_sha256":         outputSHA256,
	}
	postStateSHA256, err := canonicalObservationSHA256(postState)
	if err != nil {
		return WorkEvidence{}, err
	}
	preStateJSON, err := json.Marshal(preState)
	if err != nil {
		return WorkEvidence{}, fmt.Errorf("encode cache_data pre-state: %w", err)
	}
	postStateJSON, err := json.Marshal(postState)
	if err != nil {
		return WorkEvidence{}, fmt.Errorf("encode cache_data post-state: %w", err)
	}

	return WorkEvidence{
		InputSHA256:     preStateSHA256,
		OutputSHA256:    outputSHA256,
		PreStateSHA256:  preStateSHA256,
		PostStateSHA256: postStateSHA256,
		OutputJSON:      string(outputJSON),
		PreStateJSON:    string(preStateJSON),
		PostStateJSON:   string(postStateJSON),
	}, nil
}

func cacheDataPayloadFromWorkItem(item model.WorkItem) (model.CacheDataWorkItemPayload, model.BoundDataAsset, error) {
	parameter, ok := item.Parameters["cache_data"]
	if !ok {
		return model.CacheDataWorkItemPayload{}, model.BoundDataAsset{}, fmt.Errorf("cache_data parameter is required")
	}
	if parameter.Type != "cache_data" {
		return model.CacheDataWorkItemPayload{}, model.BoundDataAsset{}, fmt.Errorf("parameter cache_data has type %s, want cache_data", parameter.Type)
	}
	data, err := json.Marshal(parameter.Value)
	if err != nil {
		return model.CacheDataWorkItemPayload{}, model.BoundDataAsset{}, fmt.Errorf("encode cache_data parameter: %w", err)
	}
	var payload model.CacheDataWorkItemPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return model.CacheDataWorkItemPayload{}, model.BoundDataAsset{}, fmt.Errorf("decode cache_data parameter: %w", err)
	}
	if err := payload.Validate(); err != nil {
		return model.CacheDataWorkItemPayload{}, model.BoundDataAsset{}, err
	}

	assets, err := boundDataAssetsFromWorkItem(item)
	if err != nil {
		return model.CacheDataWorkItemPayload{}, model.BoundDataAsset{}, err
	}
	if len(assets) != 1 {
		return model.CacheDataWorkItemPayload{}, model.BoundDataAsset{}, fmt.Errorf("cache_data requires exactly one bound data asset, got %d", len(assets))
	}
	asset := assets[0]
	if dataAssetTransferPolicyEmpty(asset.TransferPolicy) {
		asset.TransferPolicy = payload.TransferPolicy
	}
	if asset.TransferPolicy.MaxBytesPerSecond == 0 && payload.TransferLimits.MaxBytesPerSecond > 0 {
		asset.TransferPolicy.MaxBytesPerSecond = payload.TransferLimits.MaxBytesPerSecond
	}
	return payload, asset, nil
}

func dataAssetTransferPolicyEmpty(policy model.DataAssetTransferPolicy) bool {
	return policy.MaxConcurrentSourceTransfers == 0 &&
		policy.RequestedBandwidthMiBPerSecond == 0 &&
		policy.MaxBytesPerSecond == 0 &&
		len(policy.ProviderArgs) == 0
}
