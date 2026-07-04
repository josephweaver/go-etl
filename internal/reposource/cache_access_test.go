package reposource

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestCacheAccessResolvesOnlyManifestFiles(t *testing.T) {
	layout, err := NewCacheLayout(filepath.Join("cache", "root"))
	if err != nil {
		t.Fatalf("NewCacheLayout() error = %v", err)
	}
	manifest := AdmittedSourceManifest{
		RunID: "run-1",
		Source: ResolvedSourceReference{
			Repository: RepositoryIdentity{Value: "local:demo"},
		},
		Files: []AdmittedSourceManifestFile{
			{Role: FileRoleProject, SourcePath: "project.json", CachePath: "project.json"},
		},
	}
	access, err := NewCacheAccess(layout, manifest)
	if err != nil {
		t.Fatalf("NewCacheAccess() error = %v", err)
	}

	got, err := access.FilePath("project.json")
	if err != nil {
		t.Fatalf("FilePath() error = %v", err)
	}
	want := filepath.Join("cache", "root", "local", "runs", "run-1", "files", "project.json")
	if got != want {
		t.Fatalf("FilePath() = %q, want %q", got, want)
	}
	if _, err := access.FilePath("workflow.json"); err == nil {
		t.Fatal("expected missing manifest file error")
	}
}

func TestCacheAccessReturnsManifestLayoutPaths(t *testing.T) {
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
	access, err := NewCacheAccess(layout, manifest)
	if err != nil {
		t.Fatalf("NewCacheAccess() error = %v", err)
	}

	paths, err := access.Paths()
	if err != nil {
		t.Fatalf("Paths() error = %v", err)
	}
	if paths.ManifestPath != filepath.Join("cache", "root", "github", "repos", "github.com_acme_demo", revision, "manifests", "run-1.json") {
		t.Fatalf("ManifestPath = %q", paths.ManifestPath)
	}
	if paths.LocksPath == "" || paths.TmpPath == "" {
		t.Fatalf("github locks/tmp missing: %q/%q", paths.LocksPath, paths.TmpPath)
	}
}

func TestCacheAccessRejectsInvalidManifest(t *testing.T) {
	layout, err := NewCacheLayout(filepath.Join("cache", "root"))
	if err != nil {
		t.Fatalf("NewCacheLayout() error = %v", err)
	}
	_, err = NewCacheAccess(layout, AdmittedSourceManifest{
		Source: ResolvedSourceReference{Repository: RepositoryIdentity{Value: "local:demo"}},
	})
	if err == nil {
		t.Fatal("expected missing run id error")
	}
}

func TestCacheAccessReadFileReportsCacheMiss(t *testing.T) {
	layout, err := NewCacheLayout(t.TempDir())
	if err != nil {
		t.Fatalf("NewCacheLayout() error = %v", err)
	}
	manifest, _ := testCacheManifest(t, "local", "run-1", "project.json", []byte(`{"name":"demo"}`))
	access, err := NewCacheAccess(layout, manifest)
	if err != nil {
		t.Fatalf("NewCacheAccess() error = %v", err)
	}

	_, err = access.ReadFile("project.json")
	if !errors.Is(err, ErrCacheMiss) {
		t.Fatalf("ReadFile() error = %v, want cache miss", err)
	}
}
