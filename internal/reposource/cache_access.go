package reposource

import "fmt"

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

func (a CacheAccess) manifestContainsCachePath(cachePath string) bool {
	clean, err := ValidateCacheRelativePath(cachePath)
	if err != nil {
		return false
	}
	for _, file := range a.manifest.Files {
		manifestPath, err := ValidateCacheRelativePath(file.CachePath)
		if err == nil && manifestPath == clean {
			return true
		}
	}
	return false
}
