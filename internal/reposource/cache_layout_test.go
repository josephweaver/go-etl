package reposource

import (
	"path/filepath"
	"testing"
)

func TestCacheLayoutRejectsEmptyRoot(t *testing.T) {
	if _, err := NewCacheLayout(""); err == nil {
		t.Fatal("expected empty cache root error")
	}
}

func TestCacheLayoutDerivesLocalManifestPaths(t *testing.T) {
	layout, err := NewCacheLayout(filepath.Join("cache", "root"))
	if err != nil {
		t.Fatalf("NewCacheLayout() error = %v", err)
	}
	manifest := AdmittedSourceManifest{
		RunID: "run-1",
		Source: ResolvedSourceReference{
			Repository:   RepositoryIdentity{Value: "local:demo"},
			RequestedRef: "working-tree",
		},
	}

	paths, err := layout.PathsForManifest(manifest)
	if err != nil {
		t.Fatalf("PathsForManifest() error = %v", err)
	}
	if paths.FilesRoot != filepath.Join("cache", "root", "local", "runs", "run-1", "files") {
		t.Fatalf("FilesRoot = %q", paths.FilesRoot)
	}
	if paths.ManifestPath != filepath.Join("cache", "root", "local", "runs", "run-1", "manifest.json") {
		t.Fatalf("ManifestPath = %q", paths.ManifestPath)
	}
	if paths.LocksPath != "" || paths.TmpPath != "" {
		t.Fatalf("local locks/tmp = %q/%q, want empty", paths.LocksPath, paths.TmpPath)
	}
}

func TestCacheLayoutDerivesGitHubManifestPaths(t *testing.T) {
	layout, err := NewCacheLayout(filepath.Join("cache", "root"))
	if err != nil {
		t.Fatalf("NewCacheLayout() error = %v", err)
	}
	revision := "0123456789abcdef0123456789abcdef01234567"
	manifest := AdmittedSourceManifest{
		RunID: "run-1",
		Source: ResolvedSourceReference{
			Repository:   RepositoryIdentity{Value: "github.com/acme/demo"},
			RequestedRef: "main",
			RevisionID:   &revision,
		},
	}

	paths, err := layout.PathsForManifest(manifest)
	if err != nil {
		t.Fatalf("PathsForManifest() error = %v", err)
	}
	repoRoot := filepath.Join("cache", "root", "github", "repos", "github.com_acme_demo")
	contentRoot := filepath.Join(repoRoot, revision)
	if paths.FilesRoot != filepath.Join(contentRoot, "files") {
		t.Fatalf("FilesRoot = %q", paths.FilesRoot)
	}
	if paths.ManifestPath != filepath.Join(contentRoot, "manifests", "run-1.json") {
		t.Fatalf("ManifestPath = %q", paths.ManifestPath)
	}
	if paths.LocksPath != filepath.Join(repoRoot, "locks") {
		t.Fatalf("LocksPath = %q", paths.LocksPath)
	}
	if paths.TmpPath != filepath.Join(repoRoot, "tmp") {
		t.Fatalf("TmpPath = %q", paths.TmpPath)
	}
}

func TestCacheLayoutFilePathStaysUnderFilesRoot(t *testing.T) {
	layout, err := NewCacheLayout(filepath.Join("cache", "root"))
	if err != nil {
		t.Fatalf("NewCacheLayout() error = %v", err)
	}
	manifest := AdmittedSourceManifest{
		RunID: "run-1",
		Source: ResolvedSourceReference{
			Repository: RepositoryIdentity{Value: "local:demo"},
		},
	}

	got, err := layout.FilePath(manifest, "workflows/train.json")
	if err != nil {
		t.Fatalf("FilePath() error = %v", err)
	}
	want := filepath.Join("cache", "root", "local", "runs", "run-1", "files", "workflows", "train.json")
	if got != want {
		t.Fatalf("FilePath() = %q, want %q", got, want)
	}
}

func TestCacheLayoutRejectsUnsafeCachePaths(t *testing.T) {
	layout, err := NewCacheLayout(filepath.Join("cache", "root"))
	if err != nil {
		t.Fatalf("NewCacheLayout() error = %v", err)
	}
	manifest := AdmittedSourceManifest{
		RunID: "run-1",
		Source: ResolvedSourceReference{
			Repository: RepositoryIdentity{Value: "local:demo"},
		},
	}
	for _, cachePath := range []string{"", ".", "../project.json", "/project.json", "C:/project.json", `workflows\train.json`} {
		t.Run(cachePath, func(t *testing.T) {
			if _, err := layout.FilePath(manifest, cachePath); err == nil {
				t.Fatal("expected unsafe cache path error")
			}
		})
	}
}

func TestGitHubRepositoryKeySanitizesProviderIdentity(t *testing.T) {
	got, err := GitHubRepositoryKey(RepositoryIdentity{Value: "github.com/acme/demo.repo"})
	if err != nil {
		t.Fatalf("GitHubRepositoryKey() error = %v", err)
	}
	if got != "github.com_acme_demo.repo" {
		t.Fatalf("GitHubRepositoryKey() = %q", got)
	}

	for _, value := range []string{"https://github.com/acme/demo", "github.com/acme/demo?token=secret", "github.com/acme"} {
		t.Run(value, func(t *testing.T) {
			if _, err := GitHubRepositoryKey(RepositoryIdentity{Value: value}); err == nil {
				t.Fatal("expected invalid repository identity error")
			}
		})
	}
}

func TestCacheLayoutDoesNotUseWholeRepositoryCheckoutPaths(t *testing.T) {
	layout, err := NewCacheLayout(filepath.Join("cache", "root"))
	if err != nil {
		t.Fatalf("NewCacheLayout() error = %v", err)
	}
	revision := "0123456789abcdef0123456789abcdef01234567"
	manifest := AdmittedSourceManifest{
		RunID: "run-1",
		Source: ResolvedSourceReference{
			Repository: RepositoryIdentity{Value: "github.com/acme/demo"},
			RevisionID: &revision,
		},
	}
	got, err := layout.FilePath(manifest, "project.json")
	if err != nil {
		t.Fatalf("FilePath() error = %v", err)
	}
	if filepath.Base(filepath.Dir(filepath.Dir(got))) != revision {
		t.Fatalf("path %q is not under revision content key", got)
	}
	if filepath.Base(filepath.Dir(got)) != "files" {
		t.Fatalf("path %q is not under files root", got)
	}
}
