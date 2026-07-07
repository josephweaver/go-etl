package main

import (
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
		if _, ok := seenNames[plan.target.Name]; ok {
			return nil, fmt.Errorf("duplicate publish target name %q", plan.target.Name)
		}
		if _, ok := seenDestinations[plan.finalPath]; ok {
			return nil, fmt.Errorf("duplicate publish destination path %q", plan.finalPath)
		}
		seenNames[plan.target.Name] = struct{}{}
		seenDestinations[plan.finalPath] = struct{}{}
		plans = append(plans, plan)
	}

	for _, plan := range plans {
		if _, err := os.Stat(plan.finalPath); err == nil {
			return nil, fmt.Errorf("publish destination already exists: %s", plan.finalPath)
		} else if !os.IsNotExist(err) {
			return nil, fmt.Errorf("check publish destination %s: %w", plan.finalPath, err)
		}
	}

	published := make([]model.PublishedDataAsset, 0, len(plans))
	for i, plan := range plans {
		evidence, err := publishArtifactToTemp(plan)
		if err != nil {
			cleanupPublishTemps(plans)
			return nil, err
		}
		plans[i].sizeBytes = &evidence.size
		plans[i].sha256 = evidence.sha256
		published = append(published, plans[i].evidence())
	}

	var completed []publishPlan
	for _, plan := range plans {
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
	target     model.BoundPublishTarget
	source     model.ArtifactDescriptor
	sourcePath string
	finalPath  string
	tempPath   string
	sizeBytes  *int64
	sha256     string
}

func (plan publishPlan) evidence() model.PublishedDataAsset {
	return model.PublishedDataAsset{
		Name:            plan.target.Name,
		FromArtifact:    plan.target.FromArtifact,
		StorageScope:    model.DataLocationTypeRegistered,
		LocationName:    plan.target.Location.LocationName,
		Path:            plan.target.Location.Path,
		SizeBytes:       plan.sizeBytes,
		SHA256:          plan.sha256,
		OverwritePolicy: plan.target.OverwritePolicy,
	}
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

	root, err := w.resolvePublishLocationRoot(target.Location.LocationName)
	if err != nil {
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

	finalPath, err := resolveArtifactPathInsideRoot(root, target.Location.Path, "publish destination path")
	if err != nil {
		return publishPlan{}, err
	}
	tempRel := filepath.ToSlash(filepath.Join(".tmp-"+randomHex(8), target.Location.Path))
	tempPath, err := resolveArtifactPathInsideRoot(root, tempRel, "temporary publish destination path")
	if err != nil {
		return publishPlan{}, err
	}

	return publishPlan{
		target:     target,
		source:     artifact,
		sourcePath: sourcePath,
		finalPath:  finalPath,
		tempPath:   tempPath,
	}, nil
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
		_ = os.RemoveAll(artifactTempRoot(plan.tempPath))
	}
}

func rollbackPublishedTargets(plans []publishPlan) {
	for _, plan := range plans {
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
