package reposource

import (
	"errors"
	"os"
	"testing"
)

func TestReconstructCachePinsIsIdempotent(t *testing.T) {
	layout, err := NewCacheLayout(t.TempDir())
	if err != nil {
		t.Fatalf("NewCacheLayout() error = %v", err)
	}
	manifest, _ := multiFilePinManifest(t, "local")

	first, err := ReconstructCachePins(layout, []AdmittedSourceManifest{manifest})
	if err != nil {
		t.Fatalf("ReconstructCachePins(first) error = %v", err)
	}
	firstData, err := os.ReadFile(first[0].Path)
	if err != nil {
		t.Fatalf("ReadFile(first pin) error = %v", err)
	}
	second, err := ReconstructCachePins(layout, []AdmittedSourceManifest{manifest})
	if err != nil {
		t.Fatalf("ReconstructCachePins(second) error = %v", err)
	}
	secondData, err := os.ReadFile(second[0].Path)
	if err != nil {
		t.Fatalf("ReadFile(second pin) error = %v", err)
	}
	if string(firstData) != string(secondData) {
		t.Fatalf("pin rewrite not idempotent\nfirst=%s\nsecond=%s", string(firstData), string(secondData))
	}
}

func TestReconstructCachePinsFromManifestPaths(t *testing.T) {
	layout, manifest, reads := publishPinManifest(t, "github")
	if err := PublishAdmittedSource(layout, manifest, reads); err != nil {
		t.Fatalf("PublishAdmittedSource() error = %v", err)
	}
	access, err := NewCacheAccess(layout, manifest)
	if err != nil {
		t.Fatalf("NewCacheAccess() error = %v", err)
	}
	paths, err := access.Paths()
	if err != nil {
		t.Fatalf("Paths() error = %v", err)
	}

	results, err := ReconstructCachePinsFromManifestPaths(layout, []string{paths.ManifestPath})
	if err != nil {
		t.Fatalf("ReconstructCachePinsFromManifestPaths() error = %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("len(results) = %d, want 1", len(results))
	}
	if results[0].Pin.WorkflowRunID != manifest.RunID {
		t.Fatalf("workflow run id = %q", results[0].Pin.WorkflowRunID)
	}
}

func TestReconstructCachePinsFromManifestPathsReportsMissingManifest(t *testing.T) {
	layout, err := NewCacheLayout(t.TempDir())
	if err != nil {
		t.Fatalf("NewCacheLayout() error = %v", err)
	}
	_, err = ReconstructCachePinsFromManifestPaths(layout, []string{"missing-manifest.json"})
	if !errors.Is(err, ErrCacheMiss) {
		t.Fatalf("ReconstructCachePinsFromManifestPaths() error = %v, want cache miss", err)
	}
}

func publishPinManifest(t *testing.T, provider string) (CacheLayout, AdmittedSourceManifest, []ReadFileResult) {
	t.Helper()
	layout, err := NewCacheLayout(t.TempDir())
	if err != nil {
		t.Fatalf("NewCacheLayout() error = %v", err)
	}
	manifest, reads := multiFilePinManifest(t, provider)
	return layout, manifest, reads
}
