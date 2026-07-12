package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"goetl/internal/model"
	"goetl/internal/persistence"
	"goetl/internal/workflow"
)

func (c *Controller) hydrateAssetMaterializeDependentWorkItem(ctx context.Context, claim persistence.ClaimedWorkRecord, item model.WorkItem) (model.WorkItem, error) {
	if item.Type == model.WorkItemTypeAssetMaterialize || item.Type == model.WorkItemTypeCommitData {
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

	requirements, err := sharedMaterializationRequirements(item)
	if err != nil {
		return model.WorkItem{}, err
	}
	if len(requirements) > 0 {
		candidates, err := completedPriorMaterializationCandidates(terminals, claim, item.DependsOn)
		if err != nil {
			return model.WorkItem{}, err
		}
		manifest, err := projectSharedMaterializationRequirements(requirements, candidates)
		if err != nil {
			return model.WorkItem{}, err
		}
		if item.Parameters == nil {
			item.Parameters = model.Parameters{}
		}
		item.Parameters["materialized_data_assets"] = model.Parameter{
			Type:  "materialized_data_assets",
			Value: manifest,
		}
		return item, nil
	}

	if len(item.DependsOn) == 0 {
		return item, nil
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
			return model.WorkItem{}, fmt.Errorf("asset_materialize dependency target_environment_id mismatch: %s != %s", manifest.TargetEnvironmentID, combined.TargetEnvironmentID)
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

type sharedMaterializationRequirement struct {
	BindingName             string
	AssetKeys               []string
	Domain                  model.MaterializationDomain
	DestinationRelativePath string
	MaterializationKeys     []string
}

type completedMaterializationCandidate struct {
	WorkItemID string
	Manifest   model.MaterializedDataAssetManifest
	Domain     model.MaterializationDomain
}

func sharedMaterializationRequirements(item model.WorkItem) ([]sharedMaterializationRequirement, error) {
	assets, found, err := boundDataAssetsFromWorkItemParameters(item.Parameters)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, nil
	}

	var targetEnvironmentID string
	requirements := []sharedMaterializationRequirement{}
	for i, asset := range assets {
		switch asset.Materialization.Scope {
		case "":
			continue
		case model.DataMaterializationScopeShared:
			if targetEnvironmentID == "" {
				targetEnvironmentID, err = stringParameterValue(item.Parameters, "target_environment_id")
				if err != nil {
					return nil, fmt.Errorf("shared materialization data_assets[%d]: %w", i, err)
				}
			}
			domain, err := model.ResolveMaterializationDomain(asset.Materialization, targetEnvironmentID)
			if err != nil {
				return nil, fmt.Errorf("shared materialization data_assets[%d]: %w", i, err)
			}
			assetKeys, err := materializationAssetKeyCandidates(asset, targetEnvironmentID)
			if err != nil {
				return nil, fmt.Errorf("shared materialization data_assets[%d]: %w", i, err)
			}
			materializationKeys, err := materializationKeyCandidates(assetKeys, domain.ID, asset.Materialization.PathTemplate)
			if err != nil {
				return nil, fmt.Errorf("shared materialization data_assets[%d]: %w", i, err)
			}
			requirements = append(requirements, sharedMaterializationRequirement{
				BindingName:             asset.BindingName,
				AssetKeys:               assetKeys,
				Domain:                  domain,
				DestinationRelativePath: asset.Materialization.PathTemplate,
				MaterializationKeys:     materializationKeys,
			})
		case model.DataMaterializationScopeWorker:
			return nil, fmt.Errorf("%w: %s", model.ErrMaterializationScopeNotImplemented, model.DataMaterializationScopeWorker)
		default:
			if err := asset.Materialization.Validate(); err != nil {
				return nil, fmt.Errorf("shared materialization data_assets[%d]: %w", i, err)
			}
		}
	}
	return requirements, nil
}

func boundDataAssetsFromWorkItemParameters(parameters model.Parameters) ([]model.BoundDataAsset, bool, error) {
	parameter, ok := parameters["data_assets"]
	if !ok {
		return nil, false, nil
	}
	if parameter.Type != "data_assets" && parameter.Type != "list" {
		return nil, true, fmt.Errorf("parameter data_assets has type %s, want data_assets or list", parameter.Type)
	}

	data, err := json.Marshal(parameter.Value)
	if err != nil {
		return nil, true, fmt.Errorf("encode data_assets parameter: %w", err)
	}
	var assets []model.BoundDataAsset
	if err := json.Unmarshal(data, &assets); err != nil {
		return nil, true, fmt.Errorf("decode data_assets parameter: %w", err)
	}
	for i, asset := range assets {
		if err := asset.Validate(); err != nil {
			return nil, true, fmt.Errorf("data_assets[%d]: %w", i, err)
		}
	}
	return assets, true, nil
}

func stringParameterValue(parameters model.Parameters, name string) (string, error) {
	parameter, ok := parameters[name]
	if !ok {
		return "", fmt.Errorf("parameter %s is required", name)
	}
	if parameter.Type != "" && parameter.Type != "string" {
		return "", fmt.Errorf("parameter %s has type %s, want string", name, parameter.Type)
	}
	value, ok := parameter.Value.(string)
	if !ok {
		return "", fmt.Errorf("parameter %s value must be a string", name)
	}
	if strings.TrimSpace(value) == "" {
		return "", fmt.Errorf("parameter %s is required", name)
	}
	return value, nil
}

func materializationAssetKeyCandidates(asset model.BoundDataAsset, targetEnvironmentID string) ([]string, error) {
	candidates := []string{}
	definitionName := asset.DefinitionName
	if definitionName == "" {
		definitionName = asset.BindingName
	}
	canonicalKey, err := workflow.CanonicalDataAssetInstanceKey(definitionName, nil, asset)
	if err != nil {
		return nil, err
	}
	candidates = append(candidates, canonicalKey)

	cacheKey, err := workflow.AssetMaterializeAssetKey(asset, targetEnvironmentID)
	if err != nil {
		return nil, err
	}
	if cacheKey != canonicalKey {
		candidates = append(candidates, cacheKey)
	}
	return candidates, nil
}

func completedPriorMaterializationCandidates(terminals []persistence.TerminalAttemptRecord, claim persistence.ClaimedWorkRecord, dependsOn []string) ([]completedMaterializationCandidate, error) {
	dependencyIDs := map[string]struct{}{}
	for _, dependencyID := range dependsOn {
		dependencyIDs[dependencyID] = struct{}{}
	}

	candidates := []completedMaterializationCandidate{}
	for _, terminal := range terminals {
		if terminal.TerminalState != "completed" {
			continue
		}
		if _, ok := dependencyIDs[terminal.WorkItem.ID]; !ok && terminal.WorkItem.StageIndex >= claim.WorkItem.StageIndex {
			continue
		}
		if strings.TrimSpace(terminal.OutputJSON) == "" {
			continue
		}
		manifest, found, err := materializedDataAssetManifestFromOutputJSON(terminal.OutputJSON)
		if err != nil {
			return nil, err
		}
		if !found {
			continue
		}
		domain, err := model.SharedMaterializationDomain(manifest.TargetEnvironmentID)
		if err != nil {
			return nil, fmt.Errorf("completed materialized manifest %s: %w", terminal.WorkItem.ID, err)
		}
		candidates = append(candidates, completedMaterializationCandidate{
			WorkItemID: terminal.WorkItem.ID,
			Manifest:   manifest,
			Domain:     domain,
		})
	}
	return candidates, nil
}

func materializationKeyCandidates(assetKeys []string, materializationDomainID string, destinationRelativePath string) ([]string, error) {
	if strings.TrimSpace(destinationRelativePath) == "" {
		return nil, nil
	}
	if _, err := model.ValidateArtifactRelativePath(destinationRelativePath); err != nil {
		return nil, fmt.Errorf("materialization destination_relative_path: %w", err)
	}
	keys := make([]string, 0, len(assetKeys))
	for _, assetKey := range assetKeys {
		key, err := workflow.MaterializationIdentityKey(assetKey, materializationDomainID, destinationRelativePath)
		if err != nil {
			return nil, err
		}
		keys = append(keys, key)
	}
	return keys, nil
}

func projectSharedMaterializationRequirements(requirements []sharedMaterializationRequirement, candidates []completedMaterializationCandidate) (model.MaterializedDataAssetManifest, error) {
	combined := model.MaterializedDataAssetManifest{
		Schema: model.MaterializedDataAssetManifestSchemaV1,
		Assets: []model.MaterializedDataAsset{},
	}
	seenBindings := map[string]struct{}{}

	for _, requirement := range requirements {
		matches := []completedMaterializationCandidate{}
		wrongDomains := []model.MaterializationDomain{}
		for _, candidate := range candidates {
			if !containsString(requirement.AssetKeys, candidate.Manifest.AssetKey) {
				continue
			}
			if candidate.Domain != requirement.Domain {
				wrongDomains = append(wrongDomains, candidate.Domain)
				continue
			}
			if len(candidate.Manifest.Assets) != 1 {
				matches = append(matches, candidate)
				continue
			}
			candidateAsset := candidate.Manifest.Assets[0]
			if !materializedCandidateSatisfiesIdentity(requirement, candidateAsset) {
				continue
			}
			matches = append(matches, candidate)
		}
		if len(matches) == 0 {
			if len(wrongDomains) > 0 {
				return model.MaterializedDataAssetManifest{}, fmt.Errorf("materialized data asset %q found in domain %s, want %s", requirement.BindingName, materializationDomainLabel(wrongDomains[0]), materializationDomainLabel(requirement.Domain))
			}
			return model.MaterializedDataAssetManifest{}, fmt.Errorf("no completed shared materialization found for data asset %q in domain %s", requirement.BindingName, materializationDomainLabel(requirement.Domain))
		}
		if len(matches) > 1 {
			return model.MaterializedDataAssetManifest{}, fmt.Errorf("multiple completed shared materializations found for data asset %q in domain %s", requirement.BindingName, materializationDomainLabel(requirement.Domain))
		}

		match := matches[0]
		if len(match.Manifest.Assets) != 1 {
			return model.MaterializedDataAssetManifest{}, fmt.Errorf("completed shared materialization %s has %d assets, want 1", match.WorkItemID, len(match.Manifest.Assets))
		}
		if _, ok := seenBindings[requirement.BindingName]; ok {
			return model.MaterializedDataAssetManifest{}, fmt.Errorf("duplicate materialized data binding %q", requirement.BindingName)
		}
		seenBindings[requirement.BindingName] = struct{}{}

		if combined.TargetEnvironmentID == "" {
			combined.TargetEnvironmentID = match.Manifest.TargetEnvironmentID
		} else if match.Manifest.TargetEnvironmentID != combined.TargetEnvironmentID {
			return model.MaterializedDataAssetManifest{}, fmt.Errorf("shared materialization target_environment_id mismatch: %s != %s", match.Manifest.TargetEnvironmentID, combined.TargetEnvironmentID)
		}

		asset := match.Manifest.Assets[0]
		asset.BindingName = requirement.BindingName
		combined.Assets = append(combined.Assets, asset)
	}

	if err := combined.Validate(); err != nil {
		return model.MaterializedDataAssetManifest{}, fmt.Errorf("validate shared materialized data assets: %w", err)
	}
	return combined, nil
}

func materializedCandidateSatisfiesIdentity(requirement sharedMaterializationRequirement, asset model.MaterializedDataAsset) bool {
	if asset.MaterializationDomainID != "" && asset.MaterializationDomainID != requirement.Domain.ID {
		return false
	}
	if requirement.DestinationRelativePath != "" && asset.DestinationRelativePath != requirement.DestinationRelativePath {
		return false
	}
	if len(requirement.MaterializationKeys) > 0 {
		if asset.MaterializationKey == "" {
			return false
		}
		return containsString(requirement.MaterializationKeys, asset.MaterializationKey)
	}
	return true
}

func materializationDomainLabel(domain model.MaterializationDomain) string {
	return domain.Scope + "/" + domain.ID
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
