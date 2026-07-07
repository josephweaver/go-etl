package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"goetl/internal/model"
	"goetl/internal/persistence"
)

func (c *Controller) hydrateCacheDataDependentWorkItem(ctx context.Context, claim persistence.ClaimedWorkRecord, item model.WorkItem) (model.WorkItem, error) {
	if len(item.DependsOn) == 0 || item.Type == model.WorkItemTypeCacheData || item.Type == model.WorkItemTypeCommitData {
		return item, nil
	}
	if _, ok := item.Parameters["materialized_data_assets"]; ok {
		return item, nil
	}

	terminals, err := c.workflowStore.ListTerminalAttemptsForRun(ctx, claim.WorkItem.RunID)
	if err != nil {
		return model.WorkItem{}, err
	}
	completedByWorkItemID := make(map[string]persistence.TerminalAttemptRecord, len(terminals))
	for _, terminal := range terminals {
		if terminal.TerminalState == "completed" {
			completedByWorkItemID[terminal.WorkItem.ID] = terminal
		}
	}

	combined := model.MaterializedDataAssetManifest{
		Schema: model.MaterializedDataAssetManifestSchemaV1,
		Assets: []model.MaterializedDataAsset{},
	}
	seenBindings := map[string]struct{}{}
	for _, dependencyID := range item.DependsOn {
		terminal, ok := completedByWorkItemID[dependencyID]
		if !ok {
			continue
		}
		manifest, found, err := materializedDataAssetManifestFromOutputJSON(terminal.OutputJSON)
		if err != nil {
			return model.WorkItem{}, err
		}
		if !found {
			continue
		}
		if combined.TargetEnvironmentID == "" {
			combined.TargetEnvironmentID = manifest.TargetEnvironmentID
		} else if manifest.TargetEnvironmentID != "" && manifest.TargetEnvironmentID != combined.TargetEnvironmentID {
			return model.WorkItem{}, fmt.Errorf("cache_data dependency target_environment_id mismatch: %s != %s", manifest.TargetEnvironmentID, combined.TargetEnvironmentID)
		}
		for _, asset := range manifest.Assets {
			if _, ok := seenBindings[asset.BindingName]; ok {
				return model.WorkItem{}, fmt.Errorf("duplicate materialized data binding %q", asset.BindingName)
			}
			seenBindings[asset.BindingName] = struct{}{}
			combined.Assets = append(combined.Assets, asset)
		}
	}
	if len(combined.Assets) == 0 {
		return item, nil
	}
	if item.Parameters == nil {
		item.Parameters = model.Parameters{}
	}
	if err := combined.Validate(); err != nil {
		return model.WorkItem{}, fmt.Errorf("validate combined materialized data assets: %w", err)
	}
	item.Parameters["materialized_data_assets"] = model.Parameter{
		Type:  "materialized_data_assets",
		Value: combined,
	}
	return item, nil
}

func materializedDataAssetManifestFromOutputJSON(outputJSON string) (model.MaterializedDataAssetManifest, bool, error) {
	decoder := json.NewDecoder(strings.NewReader(outputJSON))
	decoder.UseNumber()

	var decoded any
	if err := decoder.Decode(&decoded); err != nil {
		return model.MaterializedDataAssetManifest{}, false, fmt.Errorf("decode output JSON: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return model.MaterializedDataAssetManifest{}, false, fmt.Errorf("output JSON must contain one JSON document")
	}

	object, ok := decoded.(map[string]any)
	if !ok {
		return model.MaterializedDataAssetManifest{}, false, nil
	}
	schema, ok := object["schema"].(string)
	if !ok || schema != model.MaterializedDataAssetManifestSchemaV1 {
		return model.MaterializedDataAssetManifest{}, false, nil
	}

	data, err := json.Marshal(object)
	if err != nil {
		return model.MaterializedDataAssetManifest{}, false, fmt.Errorf("encode materialized data asset manifest candidate: %w", err)
	}
	var manifest model.MaterializedDataAssetManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return model.MaterializedDataAssetManifest{}, false, fmt.Errorf("decode materialized data asset manifest: %w", err)
	}
	if err := manifest.Validate(); err != nil {
		return model.MaterializedDataAssetManifest{}, false, fmt.Errorf("materialized data asset manifest: %w", err)
	}
	return manifest, true, nil
}
