package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"goetl/internal/model"
)

const artifactStorageScopeWorkerDataDir = "worker_data_dir"

type ArtifactPromotionRequest struct {
	StagingRoot string
	DataRoot    string
	RunID       string
	StageIndex  *int
	StepIndex   *int
	WorkItemID  string
	AttemptID   string
	Manifest    model.ArtifactManifest
}

type artifactPromotionPlan struct {
	descriptor model.ArtifactDescriptor
	sourcePath string
	finalRel   string
	finalPath  string
	tempPath   string
}

func PromoteArtifacts(ctx context.Context, req ArtifactPromotionRequest) (model.ArtifactManifest, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if strings.TrimSpace(req.StagingRoot) == "" {
		return model.ArtifactManifest{}, fmt.Errorf("artifact staging root is required")
	}
	if strings.TrimSpace(req.DataRoot) == "" {
		return model.ArtifactManifest{}, fmt.Errorf("artifact data root is required")
	}
	if strings.TrimSpace(req.WorkItemID) == "" {
		return model.ArtifactManifest{}, fmt.Errorf("artifact promotion work item id is required")
	}

	declared := req.Manifest
	declared.Schema = model.ArtifactManifestSchemaV1
	declared.StorageScope = "artifact_staging"
	if err := declared.Validate(); err != nil {
		return model.ArtifactManifest{}, err
	}

	plans, err := artifactPromotionPlans(req)
	if err != nil {
		return model.ArtifactManifest{}, err
	}

	for _, plan := range plans {
		if err := ctx.Err(); err != nil {
			return model.ArtifactManifest{}, err
		}
		if err := copyArtifactToTemp(plan); err != nil {
			cleanupArtifactTemps(plans)
			return model.ArtifactManifest{}, err
		}
	}

	promoted := req.Manifest
	promoted.Schema = model.ArtifactManifestSchemaV1
	promoted.RunID = req.RunID
	promoted.StageIndex = req.StageIndex
	promoted.StepIndex = req.StepIndex
	promoted.WorkItemID = req.WorkItemID
	promoted.AttemptID = req.AttemptID
	promoted.StorageScope = artifactStorageScopeWorkerDataDir
	promoted.Artifacts = make([]model.ArtifactDescriptor, 0, len(plans))

	for _, plan := range plans {
		if err := ctx.Err(); err != nil {
			cleanupArtifactTemps(plans)
			return model.ArtifactManifest{}, err
		}
		if err := os.RemoveAll(plan.finalPath); err != nil {
			cleanupArtifactTemps(plans)
			return model.ArtifactManifest{}, fmt.Errorf("remove existing artifact destination %s: %w", plan.finalPath, err)
		}
		if err := os.MkdirAll(filepath.Dir(plan.finalPath), 0755); err != nil {
			cleanupArtifactTemps(plans)
			return model.ArtifactManifest{}, fmt.Errorf("create artifact destination parent %s: %w", filepath.Dir(plan.finalPath), err)
		}
		if err := os.Rename(plan.tempPath, plan.finalPath); err != nil {
			cleanupArtifactTemps(plans)
			return model.ArtifactManifest{}, fmt.Errorf("promote artifact %s to %s: %w", plan.tempPath, plan.finalPath, err)
		}

		descriptor, err := promotedArtifactDescriptor(plan.descriptor, plan.finalPath, plan.finalRel)
		if err != nil {
			return model.ArtifactManifest{}, err
		}
		promoted.Artifacts = append(promoted.Artifacts, descriptor)
	}
	cleanupArtifactTemps(plans)

	if err := promoted.Validate(); err != nil {
		return model.ArtifactManifest{}, err
	}
	return promoted, nil
}

func artifactPromotionPlans(req ArtifactPromotionRequest) ([]artifactPromotionPlan, error) {
	stagingRoot, err := filepath.Abs(req.StagingRoot)
	if err != nil {
		return nil, fmt.Errorf("resolve artifact staging root %s: %w", req.StagingRoot, err)
	}
	dataRoot, err := filepath.Abs(req.DataRoot)
	if err != nil {
		return nil, fmt.Errorf("resolve artifact data root %s: %w", req.DataRoot, err)
	}

	baseRel, err := artifactDestinationBase(req)
	if err != nil {
		return nil, err
	}

	plans := make([]artifactPromotionPlan, 0, len(req.Manifest.Artifacts))
	seenDestinations := map[string]struct{}{}
	for i, artifact := range req.Manifest.Artifacts {
		artifactRel, err := model.ValidateArtifactRelativePath(artifact.Path)
		if err != nil {
			return nil, fmt.Errorf("artifact %d path: %w", i, err)
		}
		sourcePath, err := resolveArtifactPathInsideRoot(stagingRoot, artifactRel, "artifact source path")
		if err != nil {
			return nil, err
		}
		info, err := os.Stat(sourcePath)
		if err != nil {
			return nil, fmt.Errorf("check artifact source %s: %w", sourcePath, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("artifact source must not be a symlink: %s", artifact.Path)
		}
		if artifact.Kind == model.ArtifactKindFile && info.IsDir() {
			return nil, fmt.Errorf("file artifact source is a directory: %s", artifact.Path)
		}
		if artifact.Kind == model.ArtifactKindDirectory && !info.IsDir() {
			return nil, fmt.Errorf("directory artifact source is not a directory: %s", artifact.Path)
		}

		finalRel := baseRel + "/" + artifactRel
		if _, ok := seenDestinations[finalRel]; ok {
			return nil, fmt.Errorf("duplicate artifact destination path %q", finalRel)
		}
		seenDestinations[finalRel] = struct{}{}
		finalPath, err := resolveArtifactPathInsideRoot(dataRoot, finalRel, "artifact destination path")
		if err != nil {
			return nil, err
		}
		tempRel := baseRel + "/.tmp-" + randomHex(8) + "/" + artifactRel
		tempPath, err := resolveArtifactPathInsideRoot(dataRoot, tempRel, "temporary artifact destination path")
		if err != nil {
			return nil, err
		}

		plans = append(plans, artifactPromotionPlan{
			descriptor: artifact,
			sourcePath: sourcePath,
			finalRel:   finalRel,
			finalPath:  finalPath,
			tempPath:   tempPath,
		})
	}
	return plans, nil
}

func artifactDestinationBase(req ArtifactPromotionRequest) (string, error) {
	workItem := artifactPathSegment(req.WorkItemID)
	if req.StageIndex == nil || req.StepIndex == nil {
		return "artifacts/raw/" + workItem, nil
	}
	runID := artifactPathSegment(req.RunID)
	if runID == "" {
		return "", fmt.Errorf("artifact promotion run id is required when stage and step indexes are set")
	}
	return fmt.Sprintf("artifacts/%s/stage-%03d/step-%03d/%s", runID, *req.StageIndex, *req.StepIndex, workItem), nil
}

func artifactPathSegment(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	var builder strings.Builder
	changed := false
	for _, r := range value {
		switch {
		case unicode.IsLetter(r), unicode.IsDigit(r), r == '_', r == '-', r == '.':
			builder.WriteRune(r)
		default:
			builder.WriteByte('_')
			changed = true
		}
	}
	segment := strings.Trim(builder.String(), ".")
	if segment == "" {
		segment = "id"
		changed = true
	}
	if changed {
		sum := sha256.Sum256([]byte(value))
		segment = segment + "-" + hex.EncodeToString(sum[:4])
	}
	return segment
}

func resolveArtifactPathInsideRoot(root string, relativePath string, name string) (string, error) {
	safe, err := model.ValidateArtifactRelativePath(relativePath)
	if err != nil {
		return "", fmt.Errorf("%s: %w", name, err)
	}
	candidate := filepath.Join(root, filepath.FromSlash(safe))
	rel, err := filepath.Rel(root, candidate)
	if err != nil {
		return "", fmt.Errorf("resolve %s %s: %w", name, candidate, err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%s escapes root: %s", name, relativePath)
	}
	return candidate, nil
}

func copyArtifactToTemp(plan artifactPromotionPlan) error {
	if err := os.MkdirAll(filepath.Dir(plan.tempPath), 0755); err != nil {
		return fmt.Errorf("create temporary artifact parent %s: %w", filepath.Dir(plan.tempPath), err)
	}
	switch plan.descriptor.Kind {
	case model.ArtifactKindFile:
		return copyArtifactFile(plan.sourcePath, plan.tempPath)
	case model.ArtifactKindDirectory:
		return copyArtifactDirectory(plan.sourcePath, plan.tempPath)
	default:
		return fmt.Errorf("unsupported artifact kind: %s", plan.descriptor.Kind)
	}
}

func copyArtifactFile(source string, destination string) error {
	src, err := os.Open(source)
	if err != nil {
		return fmt.Errorf("open artifact source %s: %w", source, err)
	}
	defer src.Close()

	info, err := src.Stat()
	if err != nil {
		return fmt.Errorf("stat artifact source %s: %w", source, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("artifact source must not be a symlink: %s", source)
	}

	dst, err := os.OpenFile(destination, os.O_CREATE|os.O_WRONLY|os.O_EXCL, info.Mode().Perm())
	if err != nil {
		return fmt.Errorf("create temporary artifact %s: %w", destination, err)
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		return fmt.Errorf("copy artifact %s to %s: %w", source, destination, err)
	}
	return nil
}

func copyArtifactDirectory(source string, destination string) error {
	return filepath.WalkDir(source, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, rel)
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("artifact directory must not contain symlink: %s", path)
		}
		if entry.IsDir() {
			return os.MkdirAll(target, info.Mode().Perm())
		}
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return err
		}
		return copyArtifactFile(path, target)
	})
}

func promotedArtifactDescriptor(artifact model.ArtifactDescriptor, path string, relativePath string) (model.ArtifactDescriptor, error) {
	artifact.Path = filepath.ToSlash(relativePath)
	artifact.SizeBytes = nil
	artifact.SHA256 = ""
	artifact.ManifestSHA256 = ""

	switch artifact.Kind {
	case model.ArtifactKindFile:
		info, err := os.Stat(path)
		if err != nil {
			return model.ArtifactDescriptor{}, fmt.Errorf("check promoted artifact %s: %w", path, err)
		}
		if info.IsDir() {
			return model.ArtifactDescriptor{}, fmt.Errorf("promoted file artifact is a directory: %s", path)
		}
		hash, err := streamingFileSHA256(path)
		if err != nil {
			return model.ArtifactDescriptor{}, err
		}
		size := info.Size()
		artifact.SizeBytes = &size
		artifact.SHA256 = hash
	case model.ArtifactKindDirectory:
		info, err := os.Stat(path)
		if err != nil {
			return model.ArtifactDescriptor{}, fmt.Errorf("check promoted artifact %s: %w", path, err)
		}
		if !info.IsDir() {
			return model.ArtifactDescriptor{}, fmt.Errorf("promoted directory artifact is not a directory: %s", path)
		}
		evidence, err := directoryManifestEvidence(path)
		if err != nil {
			return model.ArtifactDescriptor{}, err
		}
		size := evidence.size
		artifact.SizeBytes = &size
		artifact.ManifestSHA256 = evidence.sha256
	default:
		return model.ArtifactDescriptor{}, fmt.Errorf("unsupported artifact kind: %s", artifact.Kind)
	}

	return artifact, nil
}

func cleanupArtifactTemps(plans []artifactPromotionPlan) {
	for _, plan := range plans {
		_ = os.RemoveAll(artifactTempRoot(plan.tempPath))
	}
}

func artifactTempRoot(tempPath string) string {
	current := tempPath
	for {
		base := filepath.Base(current)
		if strings.HasPrefix(base, ".tmp-") {
			return current
		}
		parent := filepath.Dir(current)
		if parent == current {
			return tempPath
		}
		current = parent
	}
}
