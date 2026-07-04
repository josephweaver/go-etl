package reposource

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

var safeCacheKeyPattern = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

// CacheLayout derives controller-owned repository cache paths.
type CacheLayout struct {
	root string
}

// CachePaths describes the physical cache paths for one admitted source.
type CachePaths struct {
	FilesRoot    string
	ManifestPath string
	LocksPath    string
	TmpPath      string
}

func NewCacheLayout(cacheRoot string) (CacheLayout, error) {
	if cacheRoot == "" {
		return CacheLayout{}, fmt.Errorf("cache root is required")
	}
	return CacheLayout{root: filepath.Clean(cacheRoot)}, nil
}

func (l CacheLayout) Root() string {
	return l.root
}

func (l CacheLayout) PathsForManifest(manifest AdmittedSourceManifest) (CachePaths, error) {
	if l.root == "" {
		return CachePaths{}, fmt.Errorf("cache root is required")
	}
	if manifest.RunID == "" {
		return CachePaths{}, fmt.Errorf("run id is required")
	}
	if isGitHubRepository(manifest.Source.Repository.Value) {
		if manifest.Source.RevisionID == nil || *manifest.Source.RevisionID == "" {
			return CachePaths{}, fmt.Errorf("github manifest requires revision id")
		}
		return l.githubPaths(manifest.Source.Repository, *manifest.Source.RevisionID, manifest.RunID)
	}
	return l.localPaths(manifest.RunID), nil
}

func (l CacheLayout) FilePath(manifest AdmittedSourceManifest, cachePath string) (string, error) {
	paths, err := l.PathsForManifest(manifest)
	if err != nil {
		return "", err
	}
	clean, err := ValidateCacheRelativePath(cachePath)
	if err != nil {
		return "", err
	}
	return joinUnderRoot(paths.FilesRoot, filepath.FromSlash(clean))
}

func (l CacheLayout) localPaths(runID string) CachePaths {
	base := filepath.Join(l.root, "local", "runs", runID)
	return CachePaths{
		FilesRoot:    filepath.Join(base, "files"),
		ManifestPath: filepath.Join(base, "manifest.json"),
	}
}

func (l CacheLayout) githubPaths(repository RepositoryIdentity, revisionID string, runID string) (CachePaths, error) {
	repositoryKey, err := GitHubRepositoryKey(repository)
	if err != nil {
		return CachePaths{}, err
	}
	contentKey, err := safeCacheKey("github content key", revisionID)
	if err != nil {
		return CachePaths{}, err
	}
	repoBase := filepath.Join(l.root, "github", "repos", repositoryKey)
	contentBase := filepath.Join(repoBase, contentKey)
	return CachePaths{
		FilesRoot:    filepath.Join(contentBase, "files"),
		ManifestPath: filepath.Join(contentBase, "manifests", runID+".json"),
		LocksPath:    filepath.Join(repoBase, "locks"),
		TmpPath:      filepath.Join(repoBase, "tmp"),
	}, nil
}

func GitHubRepositoryKey(repository RepositoryIdentity) (string, error) {
	value := repository.Value
	if !isGitHubRepository(value) {
		return "", fmt.Errorf("github repository identity must start with github.com/")
	}
	if strings.ContainsAny(value, "@:?") {
		return "", fmt.Errorf("github repository identity must not contain credentials or URL punctuation")
	}
	parts := strings.Split(value, "/")
	if len(parts) != 3 || parts[1] == "" || parts[2] == "" {
		return "", fmt.Errorf("github repository identity must be github.com/<owner>/<repo>")
	}
	key := strings.Join(parts, "_")
	return safeCacheKey("github repository key", key)
}

func isGitHubRepository(value string) bool {
	return strings.HasPrefix(value, "github.com/")
}

func safeCacheKey(name string, value string) (string, error) {
	if value == "" {
		return "", fmt.Errorf("%s is required", name)
	}
	if !safeCacheKeyPattern.MatchString(value) {
		return "", fmt.Errorf("%s contains unsafe filename characters", name)
	}
	return value, nil
}

func joinUnderRoot(root string, child string) (string, error) {
	candidate := filepath.Join(root, child)
	rel, err := filepath.Rel(root, candidate)
	if err != nil {
		return "", fmt.Errorf("check cache path: %w", err)
	}
	if rel == ".." || filepath.IsAbs(rel) || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("cache path escapes files root")
	}
	return candidate, nil
}
