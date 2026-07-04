package reposource

import (
	"fmt"
	"os"
)

// CacheAccess resolves admitted manifest entries to controller-owned cache paths.
type CacheAccess struct {
	layout   CacheLayout
	manifest AdmittedSourceManifest
}

func NewCacheAccess(layout CacheLayout, manifest AdmittedSourceManifest) (CacheAccess, error) {
	if _, err := layout.PathsForManifest(manifest); err != nil {
		return CacheAccess{}, err
	}
	return CacheAccess{layout: layout, manifest: manifest}, nil
}

func (a CacheAccess) Paths() (CachePaths, error) {
	return a.layout.PathsForManifest(a.manifest)
}

func (a CacheAccess) FilePath(cachePath string) (string, error) {
	if !a.manifestContainsCachePath(cachePath) {
		return "", fmt.Errorf("cache path %s is not in admitted manifest", cachePath)
	}
	return a.layout.FilePath(a.manifest, cachePath)
}

func (a CacheAccess) FilePathForManifestFile(file AdmittedSourceManifestFile) (string, error) {
	return a.FilePath(file.CachePath)
}

func (a CacheAccess) ReadFile(cachePath string) ([]byte, error) {
	file, ok := a.manifestFile(cachePath)
	if !ok {
		return nil, fmt.Errorf("cache path %s is not in admitted manifest", cachePath)
	}
	path, err := a.layout.FilePath(a.manifest, file.CachePath)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%w: %s", ErrCacheMiss, file.CachePath)
		}
		return nil, fmt.Errorf("read cached file %s: %w", file.CachePath, err)
	}
	if err := VerifyCachedFile(file, data); err != nil {
		return nil, err
	}
	return data, nil
}

func (a CacheAccess) manifestContainsCachePath(cachePath string) bool {
	_, ok := a.manifestFile(cachePath)
	return ok
}

func (a CacheAccess) manifestFile(cachePath string) (AdmittedSourceManifestFile, bool) {
	clean, err := ValidateCacheRelativePath(cachePath)
	if err != nil {
		return AdmittedSourceManifestFile{}, false
	}
	for _, file := range a.manifest.Files {
		manifestPath, err := ValidateCacheRelativePath(file.CachePath)
		if err == nil && manifestPath == clean {
			return file, true
		}
	}
	return AdmittedSourceManifestFile{}, false
}
