package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"goetl/internal/model"
)

func (w Worker) AssetMaterialize(ctx OperationContext) (WorkEvidence, error) {
	item := ctx.WorkItem
	payload, asset, err := AssetMaterializePayloadFromWorkItem(item)
	if err != nil {
		return WorkEvidence{}, err
	}

	attemptID := item.AttemptID
	if attemptID == "" {
		attemptID = item.ID + "-attempt"
	}
	workDir := filepath.Join(w.Config.TmpDir, "attempts", attemptID, "work")
	if err := os.MkdirAll(workDir, 0755); err != nil {
		return WorkEvidence{}, fmt.Errorf("create asset_materialize work dir %s: %w", workDir, err)
	}

	preState := map[string]any{
		"operator":              string(model.WorkItemTypeAssetMaterialize),
		"asset_key":             payload.AssetKey,
		"target_environment_id": payload.TargetEnvironmentID,
	}
	preStateSHA256, err := canonicalObservationSHA256(preState)
	if err != nil {
		return WorkEvidence{}, err
	}

	destination := assetDestinationRequest{
		root:    w.Config.effectiveAssetCacheDir(),
		payload: payload,
		asset:   asset,
	}
	if materialized, ok, err := existingMaterializedDestination(destination); err != nil || ok {
		if err != nil {
			return WorkEvidence{}, err
		}
		return w.assetMaterializeEvidence(payload, materialized, preStateSHA256, preState)
	}

	materializer := assetMaterializer{config: w.Config, workDir: workDir}
	materialized, err := materializer.materialize(asset)
	if err != nil {
		return WorkEvidence{}, err
	}
	materialized, err = promoteMaterializedDestination(destination, materialized)
	if err != nil {
		return WorkEvidence{}, err
	}
	return w.assetMaterializeEvidence(payload, materialized, preStateSHA256, preState)
}

func (w Worker) assetMaterializeEvidence(
	payload model.AssetMaterializeWorkItemPayload,
	materialized model.MaterializedDataAsset,
	preStateSHA256 string,
	preState map[string]any,
) (WorkEvidence, error) {
	manifest := model.MaterializedDataAssetManifest{
		Schema:              model.MaterializedDataAssetManifestSchemaV1,
		AssetKey:            payload.AssetKey,
		TargetEnvironmentID: payload.TargetEnvironmentID,
		Assets:              []model.MaterializedDataAsset{materialized},
	}
	if err := manifest.Validate(); err != nil {
		return WorkEvidence{}, fmt.Errorf("validate asset_materialize manifest: %w", err)
	}
	manifestJSON, err := json.Marshal(manifest)
	if err != nil {
		return WorkEvidence{}, fmt.Errorf("encode asset_materialize manifest: %w", err)
	}
	outputJSON, outputSHA256, _, err := canonicalJSONDocument(manifestJSON, "asset_materialize manifest")
	if err != nil {
		return WorkEvidence{}, err
	}

	postState := map[string]any{
		"operator":              string(model.WorkItemTypeAssetMaterialize),
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
		return WorkEvidence{}, fmt.Errorf("encode asset_materialize pre-state: %w", err)
	}
	postStateJSON, err := json.Marshal(postState)
	if err != nil {
		return WorkEvidence{}, fmt.Errorf("encode asset_materialize post-state: %w", err)
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

func AssetMaterializePayloadFromWorkItem(item model.WorkItem) (model.AssetMaterializeWorkItemPayload, model.BoundDataAsset, error) {
	parameter, ok := item.Parameters["asset_materialize"]
	if !ok {
		return model.AssetMaterializeWorkItemPayload{}, model.BoundDataAsset{}, fmt.Errorf("asset_materialize parameter is required")
	}
	if parameter.Type != "asset_materialize" {
		return model.AssetMaterializeWorkItemPayload{}, model.BoundDataAsset{}, fmt.Errorf("parameter asset_materialize has type %s, want asset_materialize", parameter.Type)
	}
	data, err := json.Marshal(parameter.Value)
	if err != nil {
		return model.AssetMaterializeWorkItemPayload{}, model.BoundDataAsset{}, fmt.Errorf("encode asset_materialize parameter: %w", err)
	}
	var payload model.AssetMaterializeWorkItemPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return model.AssetMaterializeWorkItemPayload{}, model.BoundDataAsset{}, fmt.Errorf("decode asset_materialize parameter: %w", err)
	}
	if err := payload.Validate(); err != nil {
		return model.AssetMaterializeWorkItemPayload{}, model.BoundDataAsset{}, err
	}

	assets, err := boundDataAssetsFromWorkItem(item)
	if err != nil {
		return model.AssetMaterializeWorkItemPayload{}, model.BoundDataAsset{}, err
	}
	if len(assets) != 1 {
		return model.AssetMaterializeWorkItemPayload{}, model.BoundDataAsset{}, fmt.Errorf("asset_materialize requires exactly one bound data asset, got %d", len(assets))
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
