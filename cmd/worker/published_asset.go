package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"goetl/internal/model"
)

func (w Worker) publishPromotedArtifacts(item model.WorkItem, promoted model.ArtifactManifest) ([]model.PublishedDataAsset, error) {
	targets, err := publishTargetsFromWorkItem(item)
	if err != nil {
		return nil, err
	}
	return w.publishPromotedArtifactsForTargets(targets, promoted, "")
}

func (w Worker) publishPromotedArtifactsForTargets(targets []model.BoundPublishTarget, promoted model.ArtifactManifest, sourceWorkItemID string) ([]model.PublishedDataAsset, error) {
	if len(targets) == 0 {
		return nil, nil
	}

	artifacts := make(map[string]model.ArtifactDescriptor, len(promoted.Artifacts))
	for _, artifact := range promoted.Artifacts {
		artifacts[artifact.Name] = artifact
	}

	plans := make([]publishPlan, 0, len(targets))
	seenNames := map[string]struct{}{}
	seenDestinations := map[string]struct{}{}
	for _, target := range targets {
		artifact, ok := artifacts[target.FromArtifact]
		if !ok {
			return nil, fmt.Errorf("publish target %q references unknown artifact %q", target.Name, target.FromArtifact)
		}
		plan, err := w.publishPlanForTarget(target, artifact)
		if err != nil {
			return nil, err
		}
		plan.sourceWorkItemID = sourceWorkItemID
		if _, ok := seenNames[plan.target.Name]; ok {
			return nil, fmt.Errorf("duplicate publish target name %q", plan.target.Name)
		}
		if _, ok := seenDestinations[plan.destinationKey]; ok {
			return nil, fmt.Errorf("duplicate publish destination path %q", plan.destinationKey)
		}
		seenNames[plan.target.Name] = struct{}{}
		seenDestinations[plan.destinationKey] = struct{}{}
		plans = append(plans, plan)
	}

	for _, plan := range plans {
		if err := w.checkPublishDestinationAvailable(plan); err != nil {
			return nil, err
		}
	}

	published := make([]model.PublishedDataAsset, 0, len(plans))
	for i, plan := range plans {
		evidence, err := w.publishArtifactForPlan(plan)
		if err != nil {
			cleanupPublishTemps(plans)
			return nil, err
		}
		if err := verifyPublishedArtifactEvidence(plan, evidence); err != nil {
			cleanupPublishTemps(plans)
			return nil, err
		}
		plans[i].sizeBytes = &evidence.size
		plans[i].sha256 = evidence.sha256
		published = append(published, plans[i].evidence())
	}

	var completed []publishPlan
	for _, plan := range plans {
		if plan.target.Location.Type == model.DataProviderGDriveRclone {
			completed = append(completed, plan)
			continue
		}
		if err := os.MkdirAll(filepath.Dir(plan.finalPath), 0755); err != nil {
			cleanupPublishTemps(plans)
			rollbackPublishedTargets(completed)
			return nil, fmt.Errorf("create publish destination parent %s: %w", filepath.Dir(plan.finalPath), err)
		}
		if err := os.Rename(plan.tempPath, plan.finalPath); err != nil {
			cleanupPublishTemps(plans)
			rollbackPublishedTargets(completed)
			return nil, fmt.Errorf("publish %s to %s: %w", plan.tempPath, plan.finalPath, err)
		}
		completed = append(completed, plan)
	}
	cleanupPublishTemps(plans)
	return published, nil
}

type publishPlan struct {
	target           model.BoundPublishTarget
	source           model.ArtifactDescriptor
	sourceWorkItemID string
	sourcePath       string
	finalPath        string
	tempPath         string
	destinationKey   string
	sizeBytes        *int64
	sha256           string
}

func (plan publishPlan) evidence() model.PublishedDataAsset {
	storageScope := plan.target.Location.Type
	locationName := plan.target.Location.LocationName
	path := plan.target.Location.Path
	if plan.target.Location.Type == model.DataProviderGDriveRclone {
		storageScope = model.DataProviderGDriveRclone
		locationName = plan.target.Location.Remote
		path = plan.target.Location.DrivePath
	}
	return model.PublishedDataAsset{
		Name:            plan.target.Name,
		FromWorkItemID:  plan.sourceWorkItemID,
		FromArtifact:    plan.target.FromArtifact,
		ContentType:     plan.source.ContentType,
		StorageScope:    storageScope,
		LocationName:    locationName,
		Path:            path,
		SizeBytes:       plan.sizeBytes,
		SHA256:          plan.sha256,
		OverwritePolicy: plan.target.OverwritePolicy,
	}
}

func (w Worker) commitData(ctx OperationContext) (WorkEvidence, error) {
	item := ctx.WorkItem
	payload, manifest, err := commitDataPayloadFromWorkItem(item)
	if err != nil {
		return WorkEvidence{}, err
	}

	preState := map[string]any{
		"operator":              string(model.WorkItemTypeCommitData),
		"source_work_item_id":   payload.Source.FromWorkItemID,
		"from_artifact":         payload.Source.FromArtifact,
		"target_environment_id": payload.TargetEnvironmentID,
		"publish_target":        payload.PublishTarget.Name,
	}
	preStateSHA256, err := canonicalObservationSHA256(preState)
	if err != nil {
		return WorkEvidence{}, err
	}

	published, err := w.publishPromotedArtifactsForTargets([]model.BoundPublishTarget{payload.PublishTarget}, manifest, payload.Source.FromWorkItemID)
	if err != nil {
		return WorkEvidence{}, err
	}
	output := model.PublishedDataAssetManifest{
		Schema:              model.PublishedDataAssetManifestSchemaV1,
		TargetEnvironmentID: payload.TargetEnvironmentID,
		PublishedAssets:     published,
	}
	if err := output.Validate(); err != nil {
		return WorkEvidence{}, fmt.Errorf("validate commit_data manifest: %w", err)
	}
	manifestJSON, err := json.Marshal(output)
	if err != nil {
		return WorkEvidence{}, fmt.Errorf("encode commit_data manifest: %w", err)
	}
	outputJSON, outputSHA256, _, err := canonicalJSONDocument(manifestJSON, "commit_data manifest")
	if err != nil {
		return WorkEvidence{}, err
	}

	postState := map[string]any{
		"operator":              string(model.WorkItemTypeCommitData),
		"source_work_item_id":   payload.Source.FromWorkItemID,
		"from_artifact":         payload.Source.FromArtifact,
		"target_environment_id": payload.TargetEnvironmentID,
		"publish_target":        payload.PublishTarget.Name,
		"output_sha256":         outputSHA256,
	}
	postStateSHA256, err := canonicalObservationSHA256(postState)
	if err != nil {
		return WorkEvidence{}, err
	}
	preStateJSON, err := json.Marshal(preState)
	if err != nil {
		return WorkEvidence{}, fmt.Errorf("encode commit_data pre-state: %w", err)
	}
	postStateJSON, err := json.Marshal(postState)
	if err != nil {
		return WorkEvidence{}, fmt.Errorf("encode commit_data post-state: %w", err)
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

func commitDataPayloadFromWorkItem(item model.WorkItem) (model.CommitDataWorkItemPayload, model.ArtifactManifest, error) {
	parameter, ok := item.Parameters["commit_data"]
	if !ok {
		return model.CommitDataWorkItemPayload{}, model.ArtifactManifest{}, fmt.Errorf("commit_data parameter is required")
	}
	if parameter.Type != "commit_data" {
		return model.CommitDataWorkItemPayload{}, model.ArtifactManifest{}, fmt.Errorf("parameter commit_data has type %s, want commit_data", parameter.Type)
	}
	data, err := json.Marshal(parameter.Value)
	if err != nil {
		return model.CommitDataWorkItemPayload{}, model.ArtifactManifest{}, fmt.Errorf("encode commit_data parameter: %w", err)
	}
	var payload model.CommitDataWorkItemPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return model.CommitDataWorkItemPayload{}, model.ArtifactManifest{}, fmt.Errorf("decode commit_data parameter: %w", err)
	}
	if err := payload.Validate(); err != nil {
		return model.CommitDataWorkItemPayload{}, model.ArtifactManifest{}, err
	}

	manifestParameter, ok := item.Parameters["artifact_manifest"]
	if !ok {
		return model.CommitDataWorkItemPayload{}, model.ArtifactManifest{}, fmt.Errorf("artifact_manifest parameter is required")
	}
	if manifestParameter.Type != "artifact_manifest" {
		return model.CommitDataWorkItemPayload{}, model.ArtifactManifest{}, fmt.Errorf("parameter artifact_manifest has type %s, want artifact_manifest", manifestParameter.Type)
	}
	manifestData, err := json.Marshal(manifestParameter.Value)
	if err != nil {
		return model.CommitDataWorkItemPayload{}, model.ArtifactManifest{}, fmt.Errorf("encode artifact_manifest parameter: %w", err)
	}
	var manifest model.ArtifactManifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		return model.CommitDataWorkItemPayload{}, model.ArtifactManifest{}, fmt.Errorf("decode artifact_manifest parameter: %w", err)
	}
	if err := manifest.Validate(); err != nil {
		return model.CommitDataWorkItemPayload{}, model.ArtifactManifest{}, fmt.Errorf("validate artifact_manifest parameter: %w", err)
	}
	if manifest.WorkItemID != "" && manifest.WorkItemID != payload.Source.FromWorkItemID {
		return model.CommitDataWorkItemPayload{}, model.ArtifactManifest{}, fmt.Errorf("artifact_manifest work_item_id %q does not match commit_data source %q", manifest.WorkItemID, payload.Source.FromWorkItemID)
	}
	return payload, manifest, nil
}

func (w Worker) publishPlanForTarget(target model.BoundPublishTarget, artifact model.ArtifactDescriptor) (publishPlan, error) {
	if target.Name == "" {
		target.Name = target.TargetName
	}
	if target.TargetName == "" {
		target.TargetName = target.Name
	}
	if target.Name != target.TargetName {
		return publishPlan{}, fmt.Errorf("publish target name %q does not match target_name %q", target.Name, target.TargetName)
	}
	if err := target.Validate(); err != nil {
		return publishPlan{}, err
	}

	sourcePath, err := resolveArtifactPathInsideRoot(w.Config.DataDir, artifact.Path, "published artifact source path")
	if err != nil {
		return publishPlan{}, err
	}
	info, err := os.Stat(sourcePath)
	if err != nil {
		return publishPlan{}, fmt.Errorf("check published artifact source %s: %w", sourcePath, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return publishPlan{}, fmt.Errorf("published artifact source must not be a symlink: %s", artifact.Path)
	}
	if artifact.Kind == model.ArtifactKindFile && info.IsDir() {
		return publishPlan{}, fmt.Errorf("published artifact source is a directory: %s", artifact.Path)
	}
	if artifact.Kind == model.ArtifactKindDirectory && !info.IsDir() {
		return publishPlan{}, fmt.Errorf("published artifact source is not a directory: %s", artifact.Path)
	}

	plan := publishPlan{
		target:     target,
		source:     artifact,
		sourcePath: sourcePath,
	}
	switch target.Location.Type {
	case model.DataProviderRegisteredLocation:
		root, err := w.resolvePublishLocationRoot(target.Location.LocationName)
		if err != nil {
			return publishPlan{}, err
		}
		finalPath, err := resolveArtifactPathInsideRoot(root, target.Location.Path, "publish destination path")
		if err != nil {
			return publishPlan{}, err
		}
		tempRel := filepath.ToSlash(filepath.Join(".tmp-"+randomHex(8), target.Location.Path))
		tempPath, err := resolveArtifactPathInsideRoot(root, tempRel, "temporary publish destination path")
		if err != nil {
			return publishPlan{}, err
		}
		plan.finalPath = finalPath
		plan.tempPath = tempPath
		plan.destinationKey = target.Location.Type + ":" + target.Location.LocationName + ":" + target.Location.Path
	case model.DataProviderGDriveRclone:
		if artifact.Kind != model.ArtifactKindFile {
			return publishPlan{}, fmt.Errorf("gdrive_rclone publish supports file artifacts only")
		}
		if !w.Config.EnableGDriveRcloneProvider {
			return publishPlan{}, fmt.Errorf("gdrive_rclone provider is disabled")
		}
		if strings.TrimSpace(w.Config.RcloneExecutable) == "" {
			return publishPlan{}, fmt.Errorf("gdrive_rclone publish requires configured rclone_executable")
		}
		if target.Location.FileID != "" {
			return publishPlan{}, fmt.Errorf("gdrive_rclone file_id publish is not implemented; use drive_path")
		}
		plan.destinationKey = target.Location.Type + ":" + target.Location.Remote + ":" + target.Location.DrivePath
	default:
		return publishPlan{}, fmt.Errorf("unsupported publish target location type %q", target.Location.Type)
	}
	return plan, nil
}

func (w Worker) checkPublishDestinationAvailable(plan publishPlan) error {
	switch plan.target.Location.Type {
	case model.DataProviderRegisteredLocation:
		if _, err := os.Stat(plan.finalPath); err == nil {
			return fmt.Errorf("publish destination already exists: %s", plan.finalPath)
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("check publish destination %s: %w", plan.finalPath, err)
		}
		return nil
	case model.DataProviderGDriveRclone:
		provider := gdriveRcloneProvider{executable: w.Config.RcloneExecutable, configPath: w.Config.RcloneConfigPath}
		exists, err := provider.exists(context.Background(), plan.target.Location.Remote, plan.target.Location.DrivePath)
		if err != nil {
			return fmt.Errorf("check gdrive_rclone publish destination %s: %w", plan.destinationKey, err)
		}
		if exists {
			return fmt.Errorf("publish destination already exists: %s", plan.destinationKey)
		}
		return nil
	default:
		return fmt.Errorf("unsupported publish target location type %q", plan.target.Location.Type)
	}
}

func (w Worker) publishArtifactForPlan(plan publishPlan) (assetEvidence, error) {
	switch plan.target.Location.Type {
	case model.DataProviderRegisteredLocation:
		return publishArtifactToTemp(plan)
	case model.DataProviderGDriveRclone:
		evidence, err := hashFileWithLimit(plan.sourcePath, w.Config.effectiveMaxAssetBytes())
		if err != nil {
			return assetEvidence{}, err
		}
		provider := gdriveRcloneProvider{executable: w.Config.RcloneExecutable, configPath: w.Config.RcloneConfigPath}
		if err := provider.uploadFile(context.Background(), plan.sourcePath, plan.target.Location.Remote, plan.target.Location.DrivePath, model.DataAssetTransferPolicy{}); err != nil {
			return assetEvidence{}, err
		}
		return evidence, nil
	default:
		return assetEvidence{}, fmt.Errorf("unsupported publish target location type %q", plan.target.Location.Type)
	}
}

func verifyPublishedArtifactEvidence(plan publishPlan, evidence assetEvidence) error {
	if plan.source.SizeBytes != nil && *plan.source.SizeBytes != evidence.size {
		return fmt.Errorf("published artifact %q size mismatch: got %d, want %d", plan.source.Name, evidence.size, *plan.source.SizeBytes)
	}
	expectedSHA256 := plan.source.SHA256
	if expectedSHA256 == "" {
		expectedSHA256 = plan.source.ManifestSHA256
	}
	if expectedSHA256 != "" && expectedSHA256 != evidence.sha256 {
		return fmt.Errorf("published artifact %q sha256 mismatch: got %s, want %s", plan.source.Name, evidence.sha256, expectedSHA256)
	}
	return nil
}

func publishArtifactToTemp(plan publishPlan) (assetEvidence, error) {
	if err := os.MkdirAll(filepath.Dir(plan.tempPath), 0755); err != nil {
		return assetEvidence{}, fmt.Errorf("create temporary publish parent %s: %w", filepath.Dir(plan.tempPath), err)
	}

	switch plan.source.Kind {
	case model.ArtifactKindFile:
		evidence, err := copyFileWithLimit(plan.sourcePath, plan.tempPath, 0, 0)
		if err != nil {
			return assetEvidence{}, fmt.Errorf("copy published file %s to %s: %w", plan.sourcePath, plan.tempPath, err)
		}
		return evidence, nil
	case model.ArtifactKindDirectory:
		if err := copyArtifactDirectory(plan.sourcePath, plan.tempPath); err != nil {
			return assetEvidence{}, fmt.Errorf("copy published directory %s to %s: %w", plan.sourcePath, plan.tempPath, err)
		}
		evidence, err := directoryManifestEvidence(plan.tempPath)
		if err != nil {
			return assetEvidence{}, fmt.Errorf("compute published directory evidence %s: %w", plan.tempPath, err)
		}
		return evidence, nil
	default:
		return assetEvidence{}, fmt.Errorf("unsupported published artifact kind: %s", plan.source.Kind)
	}
}

func cleanupPublishTemps(plans []publishPlan) {
	for _, plan := range plans {
		if plan.tempPath == "" {
			continue
		}
		_ = os.RemoveAll(artifactTempRoot(plan.tempPath))
	}
}

func rollbackPublishedTargets(plans []publishPlan) {
	for _, plan := range plans {
		if plan.finalPath == "" {
			continue
		}
		_ = os.RemoveAll(plan.finalPath)
	}
}

func (w Worker) resolvePublishLocationRoot(name string) (string, error) {
	root, ok := w.Config.DataLocationRoots[name]
	if !ok || strings.TrimSpace(root) == "" {
		return "", fmt.Errorf("publish location root %q is not configured", name)
	}

	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("resolve publish location root %s: %w", root, err)
	}
	if err := os.MkdirAll(rootAbs, 0755); err != nil {
		return "", fmt.Errorf("create publish location root %s: %w", rootAbs, err)
	}
	return rootAbs, nil
}

func publishTargetsFromWorkItem(item model.WorkItem) ([]model.BoundPublishTarget, error) {
	parameter, ok := item.Parameters["publish"]
	if !ok {
		parameter, ok = item.Parameters["publish_targets"]
		if !ok {
			return nil, nil
		}
	}
	if parameter.Value == nil {
		return nil, nil
	}
	if parameter.Type != "" && parameter.Type != "publish" && parameter.Type != "publish_targets" && parameter.Type != "list" && parameter.Type != "object" {
		return nil, fmt.Errorf("parameter publish has type %s, want publish, publish_targets, list, or object", parameter.Type)
	}

	data, err := json.Marshal(parameter.Value)
	if err != nil {
		return nil, fmt.Errorf("encode publish parameter: %w", err)
	}

	var list []model.BoundPublishTarget
	if err := json.Unmarshal(data, &list); err == nil {
		if len(list) == 0 {
			return nil, nil
		}
		return normalizedPublishTargets(list)
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err == nil {
		if len(raw) == 0 {
			return nil, nil
		}
		keys := make([]string, 0, len(raw))
		for key := range raw {
			keys = append(keys, key)
		}
		sort.Strings(keys)

		targets := make([]model.BoundPublishTarget, 0, len(raw))
		for _, key := range keys {
			var target model.BoundPublishTarget
			if err := json.Unmarshal(raw[key], &target); err != nil {
				return nil, fmt.Errorf("decode publish target %q: %w", key, err)
			}
			if target.Name != "" && target.Name != key {
				return nil, fmt.Errorf("publish target %q name %q does not match object key", key, target.Name)
			}
			if target.Name == "" {
				target.Name = key
			}
			if target.TargetName == "" && target.Target != "" {
				target.TargetName = target.Target
			}
			if target.TargetName == "" {
				target.TargetName = target.Name
			}
			targets = append(targets, target)
		}
		return normalizedPublishTargets(targets)
	}

	return nil, fmt.Errorf("parameter publish must be a publish target list or object")
}

func normalizedPublishTargets(targets []model.BoundPublishTarget) ([]model.BoundPublishTarget, error) {
	normalized := make([]model.BoundPublishTarget, 0, len(targets))
	for i, target := range targets {
		if target.TargetName == "" && target.Target != "" {
			target.TargetName = target.Target
		}
		if target.Name == "" {
			target.Name = target.TargetName
		}
		if target.TargetName == "" {
			target.TargetName = target.Name
		}
		if target.Target != "" && target.TargetName != target.Target {
			return nil, fmt.Errorf("publish target %q target_name %q does not match target %q", target.Name, target.TargetName, target.Target)
		}
		if target.Name != target.TargetName {
			return nil, fmt.Errorf("publish target name %q does not match target_name %q", target.Name, target.TargetName)
		}
		if err := target.Validate(); err != nil {
			return nil, fmt.Errorf("publish target %d: %w", i, err)
		}
		normalized = append(normalized, target)
	}
	return normalized, nil
}
