package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"goetl/internal/model"
)

const materializedAssetDestinationManifestSchemaV1 = "goet/materialized-asset-destination/v1"

type materializedAssetDestinationManifest struct {
	Schema                  string         `json:"schema"`
	AssetKey                string         `json:"asset_key"`
	MaterializationKey      string         `json:"materialization_key"`
	MaterializationDomainID string         `json:"materialization_domain_id"`
	DestinationRelativePath string         `json:"destination_relative_path"`
	Kind                    string         `json:"kind"`
	SizeBytes               int64          `json:"size_bytes"`
	SHA256                  string         `json:"sha256"`
	MemberBindings          map[string]any `json:"member_bindings,omitempty"`
	WrittenAt               string         `json:"written_at"`
}

type assetDestinationRequest struct {
	root    string
	payload model.AssetMaterializeWorkItemPayload
	asset   model.BoundDataAsset
}

func existingMaterializedDestination(req assetDestinationRequest) (model.MaterializedDataAsset, bool, error) {
	if strings.TrimSpace(req.payload.DestinationRelativePath) == "" {
		return model.MaterializedDataAsset{}, false, nil
	}
	destination, manifestPath, err := destinationPaths(req)
	if err != nil {
		return model.MaterializedDataAsset{}, false, err
	}
	destinationExists := pathExists(destination)
	manifestExists := pathExists(manifestPath)
	if !destinationExists && !manifestExists {
		return model.MaterializedDataAsset{}, false, nil
	}
	if !destinationExists || !manifestExists {
		return model.MaterializedDataAsset{}, false, fmt.Errorf("materialized destination %s and manifest %s must both exist or both be absent", destination, manifestPath)
	}
	manifest, err := readDestinationManifest(manifestPath)
	if err != nil {
		return model.MaterializedDataAsset{}, false, err
	}
	if err := manifest.matches(req); err != nil {
		return model.MaterializedDataAsset{}, false, err
	}
	evidence, kind, err := destinationEvidence(destination)
	if err != nil {
		return model.MaterializedDataAsset{}, false, err
	}
	if manifest.Kind != kind || manifest.SizeBytes != evidence.size || manifest.SHA256 != evidence.sha256 {
		return model.MaterializedDataAsset{}, false, fmt.Errorf("materialized destination %s does not match pinned manifest", destination)
	}
	return destinationMaterializedAsset(req, destination, kind, evidence), true, nil
}

func promoteMaterializedDestination(req assetDestinationRequest, source model.MaterializedDataAsset) (model.MaterializedDataAsset, error) {
	if strings.TrimSpace(req.payload.DestinationRelativePath) == "" {
		return source, nil
	}
	destination, manifestPath, err := destinationPaths(req)
	if err != nil {
		return model.MaterializedDataAsset{}, err
	}
	if pathExists(destination) || pathExists(manifestPath) {
		return model.MaterializedDataAsset{}, fmt.Errorf("materialized destination %s already exists; pinned destination reuse is required", destination)
	}
	stageRoot := filepath.Join(req.root, ".goet", "staging", "materialized-destinations", randomHex(8))
	stagePath := filepath.Join(stageRoot, filepath.FromSlash(req.payload.DestinationRelativePath))
	if err := copyMaterializedSourceToStage(source.LocalPath, stagePath); err != nil {
		_ = os.RemoveAll(stageRoot)
		return model.MaterializedDataAsset{}, err
	}
	evidence, kind, err := destinationEvidence(stagePath)
	if err != nil {
		_ = os.RemoveAll(stageRoot)
		return model.MaterializedDataAsset{}, err
	}
	manifest := destinationManifest(req, kind, evidence)
	if err := os.MkdirAll(filepath.Dir(destination), 0755); err != nil {
		_ = os.RemoveAll(stageRoot)
		return model.MaterializedDataAsset{}, fmt.Errorf("create materialized destination parent %s: %w", filepath.Dir(destination), err)
	}
	if pathExists(destination) || pathExists(manifestPath) {
		_ = os.RemoveAll(stageRoot)
		return model.MaterializedDataAsset{}, fmt.Errorf("materialized destination %s already exists; pinned destination reuse is required", destination)
	}
	releaseLocks, err := acquireDestinationPromotionLocks(req, destination, manifestPath)
	if err != nil {
		_ = os.RemoveAll(stageRoot)
		return model.MaterializedDataAsset{}, err
	}
	defer releaseLocks()
	if err := os.Rename(stagePath, destination); err != nil {
		_ = os.RemoveAll(stageRoot)
		return model.MaterializedDataAsset{}, fmt.Errorf("promote materialized destination %s to %s: %w", stagePath, destination, err)
	}
	if err := writeDestinationManifest(manifestPath, manifest); err != nil {
		_ = os.RemoveAll(destination)
		return model.MaterializedDataAsset{}, err
	}
	_ = os.RemoveAll(stageRoot)
	return promotedDestinationMaterializedAsset(req, source, destination, evidence), nil
}

func destinationPaths(req assetDestinationRequest) (string, string, error) {
	if strings.TrimSpace(req.root) == "" {
		return "", "", fmt.Errorf("materialization root is required")
	}
	if err := req.payload.Validate(); err != nil {
		return "", "", err
	}
	if strings.TrimSpace(req.payload.MaterializationKey) == "" {
		return "", "", fmt.Errorf("asset_materialize materialization_key is required for destination materialization")
	}
	if strings.TrimSpace(req.payload.MaterializationDomainID) == "" {
		return "", "", fmt.Errorf("asset_materialize materialization_domain_id is required for destination materialization")
	}
	relativePath, err := model.ValidateArtifactRelativePath(req.payload.DestinationRelativePath)
	if err != nil {
		return "", "", fmt.Errorf("asset_materialize destination_relative_path: %w", err)
	}
	rootAbs, err := filepath.Abs(req.root)
	if err != nil {
		return "", "", fmt.Errorf("resolve materialization root %s: %w", req.root, err)
	}
	destination := filepath.Join(rootAbs, filepath.FromSlash(relativePath))
	rel, err := filepath.Rel(rootAbs, destination)
	if err != nil {
		return "", "", fmt.Errorf("resolve materialized destination %s: %w", destination, err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", "", fmt.Errorf("materialized destination escapes root: %s", req.payload.DestinationRelativePath)
	}
	manifestName := strings.TrimPrefix(req.payload.MaterializationKey, "sha256:") + ".json"
	manifestPath := filepath.Join(rootAbs, ".goet", "materialized-destinations", manifestName)
	return destination, manifestPath, nil
}

func acquireDestinationPromotionLocks(req assetDestinationRequest, destination string, manifestPath string) (func(), error) {
	rootAbs, err := filepath.Abs(req.root)
	if err != nil {
		return nil, fmt.Errorf("resolve materialization root %s: %w", req.root, err)
	}
	lockDir := filepath.Join(rootAbs, ".goet", "locks", "materialized-destinations")
	if err := os.MkdirAll(lockDir, 0755); err != nil {
		return nil, fmt.Errorf("create materialized destination lock directory %s: %w", lockDir, err)
	}
	locks := []string{
		filepath.Join(lockDir, destinationLockName("destination", req.payload.DestinationRelativePath)),
		filepath.Join(lockDir, destinationLockName("manifest", req.payload.MaterializationKey)),
	}
	sort.Strings(locks)

	acquired := make([]string, 0, len(locks))
	for _, lockPath := range locks {
		file, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
		if err != nil {
			for _, acquiredPath := range acquired {
				_ = os.Remove(acquiredPath)
			}
			if os.IsExist(err) {
				return nil, fmt.Errorf("materialized destination promotion is already in progress for %s", req.payload.DestinationRelativePath)
			}
			return nil, fmt.Errorf("create materialized destination promotion lock %s: %w", lockPath, err)
		}
		_, _ = file.WriteString(time.Now().UTC().Format(time.RFC3339Nano))
		_ = file.Close()
		acquired = append(acquired, lockPath)
	}

	if pathExists(destination) || pathExists(manifestPath) {
		for _, acquiredPath := range acquired {
			_ = os.Remove(acquiredPath)
		}
		return nil, fmt.Errorf("materialized destination %s already exists; pinned destination reuse is required", destination)
	}

	return func() {
		for _, lockPath := range acquired {
			_ = os.Remove(lockPath)
		}
	}, nil
}

func destinationLockName(kind string, identity string) string {
	sum := sha256.Sum256([]byte(kind + ":" + identity))
	return kind + "-" + hex.EncodeToString(sum[:]) + ".lock"
}

func copyMaterializedSourceToStage(source string, stagePath string) error {
	info, err := os.Stat(source)
	if err != nil {
		return fmt.Errorf("check materialized source %s: %w", source, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("materialized source must not be a symlink: %s", source)
	}
	if err := os.MkdirAll(filepath.Dir(stagePath), 0755); err != nil {
		return fmt.Errorf("create materialized destination staging parent %s: %w", filepath.Dir(stagePath), err)
	}
	if info.IsDir() {
		return copyArtifactDirectory(source, stagePath)
	}
	return copyArtifactFile(source, stagePath)
}

func destinationEvidence(path string) (assetEvidence, string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return assetEvidence{}, "", fmt.Errorf("check materialized destination %s: %w", path, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return assetEvidence{}, "", fmt.Errorf("materialized destination must not be a symlink: %s", path)
	}
	if info.IsDir() {
		evidence, err := directoryManifestEvidence(path)
		return evidence, "directory", err
	}
	evidence, err := hashFileWithLimit(path, 0)
	return evidence, "file", err
}

func destinationManifest(req assetDestinationRequest, kind string, evidence assetEvidence) materializedAssetDestinationManifest {
	var bindings map[string]any
	if req.payload.CollectionMember != nil {
		bindings = req.payload.CollectionMember.MemberBindings
	}
	return materializedAssetDestinationManifest{
		Schema:                  materializedAssetDestinationManifestSchemaV1,
		AssetKey:                req.payload.AssetKey,
		MaterializationKey:      req.payload.MaterializationKey,
		MaterializationDomainID: req.payload.MaterializationDomainID,
		DestinationRelativePath: req.payload.DestinationRelativePath,
		Kind:                    kind,
		SizeBytes:               evidence.size,
		SHA256:                  evidence.sha256,
		MemberBindings:          bindings,
		WrittenAt:               time.Now().UTC().Format(time.RFC3339Nano),
	}
}

func readDestinationManifest(path string) (materializedAssetDestinationManifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return materializedAssetDestinationManifest{}, fmt.Errorf("read materialized destination manifest %s: %w", path, err)
	}
	var manifest materializedAssetDestinationManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return materializedAssetDestinationManifest{}, fmt.Errorf("decode materialized destination manifest %s: %w", path, err)
	}
	if manifest.Schema != materializedAssetDestinationManifestSchemaV1 {
		return materializedAssetDestinationManifest{}, fmt.Errorf("unsupported materialized destination manifest schema %q", manifest.Schema)
	}
	return manifest, nil
}

func writeDestinationManifest(path string, manifest materializedAssetDestinationManifest) error {
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("encode materialized destination manifest: %w", err)
	}
	if err := atomicWriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write materialized destination manifest %s: %w", path, err)
	}
	return nil
}

func (manifest materializedAssetDestinationManifest) matches(req assetDestinationRequest) error {
	if manifest.AssetKey != req.payload.AssetKey {
		return fmt.Errorf("materialized destination manifest asset_key mismatch: %s != %s", manifest.AssetKey, req.payload.AssetKey)
	}
	if manifest.MaterializationKey != req.payload.MaterializationKey {
		return fmt.Errorf("materialized destination manifest materialization_key mismatch: %s != %s", manifest.MaterializationKey, req.payload.MaterializationKey)
	}
	if manifest.MaterializationDomainID != req.payload.MaterializationDomainID {
		return fmt.Errorf("materialized destination manifest materialization_domain_id mismatch: %s != %s", manifest.MaterializationDomainID, req.payload.MaterializationDomainID)
	}
	if manifest.DestinationRelativePath != req.payload.DestinationRelativePath {
		return fmt.Errorf("materialized destination manifest destination_relative_path mismatch: %s != %s", manifest.DestinationRelativePath, req.payload.DestinationRelativePath)
	}
	return nil
}

func destinationMaterializedAsset(req assetDestinationRequest, destination string, kind string, evidence assetEvidence) model.MaterializedDataAsset {
	materialized := materializedAsset(req.asset, destination, req.asset.Materialization.Strategy, req.asset.Cache.CacheKey, req.asset.Cache.Immutable, evidence)
	size := evidence.size
	materialized.MaterializationKey = req.payload.MaterializationKey
	materialized.MaterializationDomainID = req.payload.MaterializationDomainID
	materialized.DestinationRelativePath = req.payload.DestinationRelativePath
	materialized.DestinationSizeBytes = &size
	materialized.DestinationSHA256 = evidence.sha256
	materialized.CollectionMember = req.payload.CollectionMember
	if kind == "directory" {
		materialized.SelectedSizeBytes = &size
		materialized.SelectedSHA256 = evidence.sha256
	}
	return materialized
}

func promotedDestinationMaterializedAsset(req assetDestinationRequest, source model.MaterializedDataAsset, destination string, evidence assetEvidence) model.MaterializedDataAsset {
	size := evidence.size
	source.LocalPath = destination
	source.MaterializationKey = req.payload.MaterializationKey
	source.MaterializationDomainID = req.payload.MaterializationDomainID
	source.DestinationRelativePath = req.payload.DestinationRelativePath
	source.DestinationSizeBytes = &size
	source.DestinationSHA256 = evidence.sha256
	source.CollectionMember = req.payload.CollectionMember
	return source
}

func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
