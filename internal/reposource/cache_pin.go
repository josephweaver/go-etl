package reposource

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const (
	SourceCachePinSchemaV1    = "goet/source-cache-pin/v1"
	CachePinReasonWorkflowRun = "workflow_run"
)

// SourceCachePin is reconstructable operational state for cache retention.
type SourceCachePin struct {
	Schema           string   `json:"schema"`
	PinID            string   `json:"pin_id"`
	Reason           string   `json:"reason"`
	WorkflowRunID    string   `json:"workflow_run_id"`
	SourceIdentity   string   `json:"source_identity"`
	SourceRevisionID *string  `json:"source_revision_id"`
	ManifestPath     string   `json:"manifest_path"`
	PinnedCachePaths []string `json:"pinned_cache_paths"`
}

func CachePinForManifest(layout CacheLayout, manifest AdmittedSourceManifest) (SourceCachePin, string, error) {
	paths, err := layout.PathsForManifest(manifest)
	if err != nil {
		return SourceCachePin{}, "", err
	}
	pinID, err := workflowRunPinID(manifest.RunID)
	if err != nil {
		return SourceCachePin{}, "", err
	}
	pinnedCachePaths := make([]string, 0, len(manifest.Files))
	for _, file := range manifest.Files {
		cachePath, err := ValidateCacheRelativePath(file.CachePath)
		if err != nil {
			return SourceCachePin{}, "", fmt.Errorf("validate pin cache path %q: %w", file.CachePath, err)
		}
		pinnedCachePaths = append(pinnedCachePaths, cachePath)
	}
	pinPath, err := cachePinPath(layout, manifest, pinID)
	if err != nil {
		return SourceCachePin{}, "", err
	}
	return SourceCachePin{
		Schema:           SourceCachePinSchemaV1,
		PinID:            pinID,
		Reason:           CachePinReasonWorkflowRun,
		WorkflowRunID:    manifest.RunID,
		SourceIdentity:   manifest.Source.Repository.Value,
		SourceRevisionID: manifest.Source.RevisionID,
		ManifestPath:     paths.ManifestPath,
		PinnedCachePaths: pinnedCachePaths,
	}, pinPath, nil
}

func WriteCachePin(layout CacheLayout, manifest AdmittedSourceManifest) (SourceCachePin, string, error) {
	pin, pinPath, err := CachePinForManifest(layout, manifest)
	if err != nil {
		return SourceCachePin{}, "", err
	}
	data, err := json.MarshalIndent(pin, "", "  ")
	if err != nil {
		return SourceCachePin{}, "", fmt.Errorf("encode cache pin: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(pinPath), 0o755); err != nil {
		return SourceCachePin{}, "", fmt.Errorf("create cache pin parent: %w", err)
	}
	if err := writeFileAtomically(pinPath, data); err != nil {
		return SourceCachePin{}, "", err
	}
	return pin, pinPath, nil
}

func workflowRunPinID(runID string) (string, error) {
	if runID == "" {
		return "", fmt.Errorf("run id is required")
	}
	return safeCacheKey("cache pin id", "run-"+runID)
}

func cachePinPath(layout CacheLayout, manifest AdmittedSourceManifest, pinID string) (string, error) {
	if layout.root == "" {
		return "", fmt.Errorf("cache root is required")
	}
	if pinID == "" {
		return "", fmt.Errorf("pin id is required")
	}
	if isGitHubRepository(manifest.Source.Repository.Value) {
		if manifest.Source.RevisionID == nil || *manifest.Source.RevisionID == "" {
			return "", fmt.Errorf("github manifest requires revision id")
		}
		repositoryKey, err := GitHubRepositoryKey(manifest.Source.Repository)
		if err != nil {
			return "", err
		}
		contentKey, err := safeCacheKey("github content key", *manifest.Source.RevisionID)
		if err != nil {
			return "", err
		}
		return filepath.Join(layout.Root(), "github", "repos", repositoryKey, contentKey, "pins", pinID+".json"), nil
	}
	return filepath.Join(layout.Root(), "local", "runs", manifest.RunID, "pins", pinID+".json"), nil
}
