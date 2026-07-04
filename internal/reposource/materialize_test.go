package reposource

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestMaterializeManifestWritesNestedFiles(t *testing.T) {
	layout, manifest, _ := publishMaterializeFixture(t)
	destination := t.TempDir()
	access, err := NewCacheAccess(layout, manifest)
	if err != nil {
		t.Fatalf("NewCacheAccess() error = %v", err)
	}

	if err := MaterializeManifest(context.Background(), access, destination); err != nil {
		t.Fatalf("MaterializeManifest() error = %v", err)
	}

	project, err := os.ReadFile(filepath.Join(destination, "project.json"))
	if err != nil {
		t.Fatalf("ReadFile(project) error = %v", err)
	}
	if string(project) != `{"name":"demo"}` {
		t.Fatalf("project = %q", string(project))
	}
	script, err := os.ReadFile(filepath.Join(destination, "scripts", "run.py"))
	if err != nil {
		t.Fatalf("ReadFile(script) error = %v", err)
	}
	if string(script) != "print('hi')\n" {
		t.Fatalf("script = %q", string(script))
	}
}

func TestMaterializeManifestOverwritesExistingFileAfterRead(t *testing.T) {
	layout, manifest, _ := publishMaterializeFixture(t)
	destination := t.TempDir()
	target := filepath.Join(destination, "project.json")
	if err := os.WriteFile(target, []byte("old"), 0o644); err != nil {
		t.Fatalf("WriteFile(old) error = %v", err)
	}
	access, err := NewCacheAccess(layout, manifest)
	if err != nil {
		t.Fatalf("NewCacheAccess() error = %v", err)
	}

	if err := MaterializeManifest(context.Background(), access, destination); err != nil {
		t.Fatalf("MaterializeManifest() error = %v", err)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("ReadFile(target) error = %v", err)
	}
	if string(data) != `{"name":"demo"}` {
		t.Fatalf("target = %q", string(data))
	}
}

func TestMaterializeManifestRejectsUnsafeDestination(t *testing.T) {
	layout, manifest, _ := publishMaterializeFixture(t)
	access, err := NewCacheAccess(layout, manifest)
	if err != nil {
		t.Fatalf("NewCacheAccess() error = %v", err)
	}
	if err := MaterializeManifest(context.Background(), access, ""); err == nil {
		t.Fatal("expected empty destination error")
	}
}

func TestMaterializeManifestRejectsUnsafeManifestPath(t *testing.T) {
	layout, manifest, _ := publishMaterializeFixture(t)
	manifest.Files[0].CachePath = "../project.json"
	access, err := NewCacheAccess(layout, manifest)
	if err != nil {
		t.Fatalf("NewCacheAccess() error = %v", err)
	}

	if err := MaterializeManifest(context.Background(), access, t.TempDir()); err == nil {
		t.Fatal("expected unsafe manifest path error")
	}
}

func TestMaterializeManifestPropagatesCacheMiss(t *testing.T) {
	layout, manifest, _ := publishMaterializeFixture(t)
	access, err := NewCacheAccess(layout, manifest)
	if err != nil {
		t.Fatalf("NewCacheAccess() error = %v", err)
	}
	cachePath, err := access.FilePath("project.json")
	if err != nil {
		t.Fatalf("FilePath() error = %v", err)
	}
	if err := os.Remove(cachePath); err != nil {
		t.Fatalf("Remove() error = %v", err)
	}

	err = MaterializeManifest(context.Background(), access, t.TempDir())
	if !errors.Is(err, ErrCacheMiss) {
		t.Fatalf("MaterializeManifest() error = %v, want cache miss", err)
	}
}

func TestMaterializeManifestPropagatesVerificationFailureWithoutOverwriting(t *testing.T) {
	layout, manifest, _ := publishMaterializeFixture(t)
	access, err := NewCacheAccess(layout, manifest)
	if err != nil {
		t.Fatalf("NewCacheAccess() error = %v", err)
	}
	cachePath, err := access.FilePath("project.json")
	if err != nil {
		t.Fatalf("FilePath() error = %v", err)
	}
	if err := os.WriteFile(cachePath, []byte(`{"name":"corrupt"}`), 0o644); err != nil {
		t.Fatalf("WriteFile(corrupt) error = %v", err)
	}
	destination := t.TempDir()
	target := filepath.Join(destination, "project.json")
	if err := os.WriteFile(target, []byte("old"), 0o644); err != nil {
		t.Fatalf("WriteFile(old target) error = %v", err)
	}

	err = MaterializeManifest(context.Background(), access, destination)
	if !errors.Is(err, ErrCacheCorruption) {
		t.Fatalf("MaterializeManifest() error = %v, want cache corruption", err)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("ReadFile(target) error = %v", err)
	}
	if string(data) != "old" {
		t.Fatalf("target was overwritten after verification failure: %q", string(data))
	}
}

func publishMaterializeFixture(t *testing.T) (CacheLayout, AdmittedSourceManifest, []ReadFileResult) {
	t.Helper()
	layout, err := NewCacheLayout(t.TempDir())
	if err != nil {
		t.Fatalf("NewCacheLayout() error = %v", err)
	}
	source := ResolvedSourceReference{
		Repository:   RepositoryIdentity{Value: "local:demo"},
		RequestedRef: "working-tree",
	}
	project := newReadFileResult(SourceFileRequest{Repository: source.Repository, SourcePath: "project.json"}, []byte(`{"name":"demo"}`), nil)
	script := newReadFileResult(SourceFileRequest{Repository: source.Repository, SourcePath: "scripts/run.py"}, []byte("print('hi')\n"), nil)
	canonical := canonicalProjectHash(t)
	manifest, err := BuildAdmittedSourceManifest("run-1", source, []DeclaredSourceFile{
		{
			Role:                FileRoleProject,
			SourcePath:          "project.json",
			CachePath:           "project.json",
			ContentType:         "application/json",
			CanonicalJSONSHA256: &canonical,
		},
		{
			Role:        FileRolePythonEntrypoint,
			SourcePath:  "scripts/run.py",
			CachePath:   "scripts/run.py",
			ContentType: "text/x-python",
		},
	}, []ReadFileResult{project, script})
	if err != nil {
		t.Fatalf("BuildAdmittedSourceManifest() error = %v", err)
	}
	reads := []ReadFileResult{project, script}
	if err := PublishAdmittedSource(layout, manifest, reads); err != nil {
		t.Fatalf("PublishAdmittedSource() error = %v", err)
	}
	return layout, manifest, reads
}

func canonicalProjectHash(t *testing.T) string {
	t.Helper()
	manifest, _ := testCacheManifest(t, "local", "run-1", "project.json", []byte(`{"name":"demo"}`))
	if manifest.Files[0].CanonicalJSONSHA256 == nil {
		t.Fatal("test manifest canonical json sha is nil")
	}
	return *manifest.Files[0].CanonicalJSONSHA256
}
