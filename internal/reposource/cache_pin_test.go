package reposource

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestCachePinForLocalManifest(t *testing.T) {
	layout, err := NewCacheLayout(t.TempDir())
	if err != nil {
		t.Fatalf("NewCacheLayout() error = %v", err)
	}
	manifest, _ := multiFilePinManifest(t, "local")

	pin, pinPath, err := CachePinForManifest(layout, manifest)
	if err != nil {
		t.Fatalf("CachePinForManifest() error = %v", err)
	}
	if pin.Schema != SourceCachePinSchemaV1 {
		t.Fatalf("schema = %q", pin.Schema)
	}
	if pin.PinID != "run-run-1" {
		t.Fatalf("pin id = %q", pin.PinID)
	}
	if pin.Reason != CachePinReasonWorkflowRun {
		t.Fatalf("reason = %q", pin.Reason)
	}
	if pin.WorkflowRunID != "run-1" {
		t.Fatalf("workflow run id = %q", pin.WorkflowRunID)
	}
	if pin.SourceIdentity != "local:demo" {
		t.Fatalf("source identity = %q", pin.SourceIdentity)
	}
	if pin.SourceRevisionID != nil {
		t.Fatalf("source revision id = %v, want nil", pin.SourceRevisionID)
	}
	if len(pin.PinnedCachePaths) != 2 {
		t.Fatalf("pinned cache paths = %v", pin.PinnedCachePaths)
	}
	wantPath := filepath.Join(layout.Root(), "local", "runs", "run-1", "pins", "run-run-1.json")
	if pinPath != wantPath {
		t.Fatalf("pin path = %q, want %q", pinPath, wantPath)
	}
}

func TestCachePinForGitHubManifest(t *testing.T) {
	layout, err := NewCacheLayout(t.TempDir())
	if err != nil {
		t.Fatalf("NewCacheLayout() error = %v", err)
	}
	manifest, _ := multiFilePinManifest(t, "github")

	pin, pinPath, err := CachePinForManifest(layout, manifest)
	if err != nil {
		t.Fatalf("CachePinForManifest() error = %v", err)
	}
	if pin.SourceIdentity != "github.com/acme/demo" {
		t.Fatalf("source identity = %q", pin.SourceIdentity)
	}
	if pin.SourceRevisionID == nil || *pin.SourceRevisionID != *manifest.Source.RevisionID {
		t.Fatalf("source revision id = %v", pin.SourceRevisionID)
	}
	wantPath := filepath.Join(layout.Root(), "github", "repos", "github.com_acme_demo", *manifest.Source.RevisionID, "pins", "run-run-1.json")
	if pinPath != wantPath {
		t.Fatalf("pin path = %q, want %q", pinPath, wantPath)
	}
}

func TestWriteCachePinWritesJSON(t *testing.T) {
	layout, err := NewCacheLayout(t.TempDir())
	if err != nil {
		t.Fatalf("NewCacheLayout() error = %v", err)
	}
	manifest, _ := multiFilePinManifest(t, "local")

	pin, pinPath, err := WriteCachePin(layout, manifest)
	if err != nil {
		t.Fatalf("WriteCachePin() error = %v", err)
	}
	data, err := os.ReadFile(pinPath)
	if err != nil {
		t.Fatalf("ReadFile(pin) error = %v", err)
	}
	var decoded SourceCachePin
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal(pin) error = %v", err)
	}
	if decoded.PinID != pin.PinID {
		t.Fatalf("decoded pin id = %q, want %q", decoded.PinID, pin.PinID)
	}
	if len(decoded.PinnedCachePaths) != 2 {
		t.Fatalf("decoded pinned cache paths = %v", decoded.PinnedCachePaths)
	}
}

func multiFilePinManifest(t *testing.T, provider string) (AdmittedSourceManifest, []ReadFileResult) {
	t.Helper()
	source := ResolvedSourceReference{
		Repository:   RepositoryIdentity{Value: "local:demo"},
		RequestedRef: "main",
	}
	if provider == "github" {
		revision := "0123456789abcdef0123456789abcdef01234567"
		source.Repository = RepositoryIdentity{Value: "github.com/acme/demo"}
		source.RevisionID = &revision
	}
	project := newReadFileResult(SourceFileRequest{Repository: source.Repository, RevisionID: source.RevisionID, SourcePath: "project.json"}, []byte(`{"name":"demo"}`), nil)
	script := newReadFileResult(SourceFileRequest{Repository: source.Repository, RevisionID: source.RevisionID, SourcePath: "scripts/run.py"}, []byte("print('hi')\n"), nil)
	manifest, err := BuildAdmittedSourceManifest("run-1", source, []DeclaredSourceFile{
		{Role: FileRoleProject, SourcePath: "project.json", CachePath: "project.json", ContentType: "application/json"},
		{Role: FileRolePythonEntrypoint, SourcePath: "scripts/run.py", CachePath: "scripts/run.py", ContentType: "text/x-python"},
	}, []ReadFileResult{project, script})
	if err != nil {
		t.Fatalf("BuildAdmittedSourceManifest() error = %v", err)
	}
	return manifest, []ReadFileResult{project, script}
}
