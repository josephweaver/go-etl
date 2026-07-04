package reposource

import (
	"encoding/json"
	"fmt"
	"os"
)

type CachePinReconstructionResult struct {
	Pin  SourceCachePin
	Path string
}

func ReconstructCachePins(layout CacheLayout, manifests []AdmittedSourceManifest) ([]CachePinReconstructionResult, error) {
	results := make([]CachePinReconstructionResult, 0, len(manifests))
	for _, manifest := range manifests {
		pin, path, err := WriteCachePin(layout, manifest)
		if err != nil {
			return nil, err
		}
		results = append(results, CachePinReconstructionResult{
			Pin:  pin,
			Path: path,
		})
	}
	return results, nil
}

func ReconstructCachePinsFromManifestPaths(layout CacheLayout, manifestPaths []string) ([]CachePinReconstructionResult, error) {
	manifests := make([]AdmittedSourceManifest, 0, len(manifestPaths))
	for _, manifestPath := range manifestPaths {
		data, err := os.ReadFile(manifestPath)
		if err != nil {
			if os.IsNotExist(err) {
				return nil, fmt.Errorf("%w: admitted manifest %s", ErrCacheMiss, manifestPath)
			}
			return nil, fmt.Errorf("read admitted manifest %s: %w", manifestPath, err)
		}
		var manifest AdmittedSourceManifest
		if err := json.Unmarshal(data, &manifest); err != nil {
			return nil, fmt.Errorf("decode admitted manifest %s: %w", manifestPath, err)
		}
		manifests = append(manifests, manifest)
	}
	return ReconstructCachePins(layout, manifests)
}
