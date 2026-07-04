package reposource

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestPublishAdmittedSourceWritesLocalManifestAndNestedFiles(t *testing.T) {
	layout, err := NewCacheLayout(t.TempDir())
	if err != nil {
		t.Fatalf("NewCacheLayout() error = %v", err)
	}
	manifest, reads := testCacheManifest(t, "local", "run-1", "workflows/train.json", []byte(`{"name":"demo"}`))

	if err := PublishAdmittedSource(layout, manifest, reads); err != nil {
		t.Fatalf("PublishAdmittedSource() error = %v", err)
	}

	access, err := NewCacheAccess(layout, manifest)
	if err != nil {
		t.Fatalf("NewCacheAccess() error = %v", err)
	}
	got, err := access.ReadFile("workflows/train.json")
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(got) != `{"name":"demo"}` {
		t.Fatalf("ReadFile() = %q", string(got))
	}

	paths, err := access.Paths()
	if err != nil {
		t.Fatalf("Paths() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(paths.FilesRoot, "workflows", "train.json")); err != nil {
		t.Fatalf("cached nested file missing: %v", err)
	}
	data, err := os.ReadFile(paths.ManifestPath)
	if err != nil {
		t.Fatalf("ReadFile(manifest) error = %v", err)
	}
	var decoded AdmittedSourceManifest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal(manifest) error = %v", err)
	}
	if decoded.RunID != "run-1" {
		t.Fatalf("manifest run id = %q", decoded.RunID)
	}
}

func TestPublishAdmittedSourceWritesGitHubManifestAndReadsBack(t *testing.T) {
	layout, err := NewCacheLayout(t.TempDir())
	if err != nil {
		t.Fatalf("NewCacheLayout() error = %v", err)
	}
	manifest, reads := testCacheManifest(t, "github", "run-1", "project.json", []byte(`{"name":"demo"}`))
	if err := PublishAdmittedSource(layout, manifest, reads); err != nil {
		t.Fatalf("PublishAdmittedSource() error = %v", err)
	}

	access, err := NewCacheAccess(layout, manifest)
	if err != nil {
		t.Fatalf("NewCacheAccess() error = %v", err)
	}
	got, err := access.ReadFile("project.json")
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(got) != `{"name":"demo"}` {
		t.Fatalf("ReadFile() = %q", string(got))
	}

	paths, err := access.Paths()
	if err != nil {
		t.Fatalf("Paths() error = %v", err)
	}
	wantManifest := filepath.Join(layout.Root(), "github", "repos", "github.com_acme_demo", *manifest.Source.RevisionID, "manifests", "run-1.json")
	if paths.ManifestPath != wantManifest {
		t.Fatalf("ManifestPath = %q, want %q", paths.ManifestPath, wantManifest)
	}
	if _, err := os.Stat(paths.ManifestPath); err != nil {
		t.Fatalf("github manifest missing: %v", err)
	}
}

func TestPublishAdmittedSourceAllowsGitHubAppendOnlyNewFiles(t *testing.T) {
	layout, err := NewCacheLayout(t.TempDir())
	if err != nil {
		t.Fatalf("NewCacheLayout() error = %v", err)
	}
	manifest1, reads1 := testCacheManifest(t, "github", "run-1", "project.json", []byte(`{"name":"demo"}`))
	if err := PublishAdmittedSource(layout, manifest1, reads1); err != nil {
		t.Fatalf("PublishAdmittedSource(run-1) error = %v", err)
	}
	manifest2, reads2 := testCacheManifest(t, "github", "run-2", "scripts/run.py", []byte("print('hi')\n"))
	manifest2.Source.RevisionID = manifest1.Source.RevisionID
	if err := PublishAdmittedSource(layout, manifest2, reads2); err != nil {
		t.Fatalf("PublishAdmittedSource(run-2) error = %v", err)
	}

	access2, err := NewCacheAccess(layout, manifest2)
	if err != nil {
		t.Fatalf("NewCacheAccess() error = %v", err)
	}
	if _, err := access2.ReadFile("scripts/run.py"); err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
}

func TestPublishAdmittedSourceReplacesCorruptExistingGitHubFile(t *testing.T) {
	layout, err := NewCacheLayout(t.TempDir())
	if err != nil {
		t.Fatalf("NewCacheLayout() error = %v", err)
	}
	manifest, reads := testCacheManifest(t, "github", "run-1", "project.json", []byte(`{"name":"demo"}`))
	if err := PublishAdmittedSource(layout, manifest, reads); err != nil {
		t.Fatalf("PublishAdmittedSource() error = %v", err)
	}
	paths, err := NewCacheAccess(layout, manifest)
	if err != nil {
		t.Fatalf("NewCacheAccess() error = %v", err)
	}
	filePath, err := paths.FilePath("project.json")
	if err != nil {
		t.Fatalf("FilePath() error = %v", err)
	}
	if err := os.WriteFile(filePath, []byte(`{"name":"changed"}`), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	if err := PublishAdmittedSource(layout, manifest, reads); err != nil {
		t.Fatalf("PublishAdmittedSource() error = %v", err)
	}
	data, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(data) != `{"name":"demo"}` {
		t.Fatalf("cached data = %q, want repaired data", string(data))
	}
}
