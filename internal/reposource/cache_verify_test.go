package reposource

import (
	"errors"
	"testing"

	"goetl/internal/fingerprint"
)

func TestVerifyCachedFileDetectsRawHashMismatch(t *testing.T) {
	manifest, _ := testCacheManifest(t, "local", "run-1", "project.json", []byte(`{"name":"demo"}`))
	err := VerifyCachedFile(manifest.Files[0], []byte(`{"name":"changed"}`))
	if !errors.Is(err, ErrCacheCorruption) {
		t.Fatalf("VerifyCachedFile() error = %v, want cache corruption", err)
	}
}

func TestVerifyCachedFileDetectsCanonicalJSONMismatch(t *testing.T) {
	_, hash, err := fingerprint.CanonicalJSONSHA256(map[string]any{"name": "demo"})
	if err != nil {
		t.Fatalf("CanonicalJSONSHA256() error = %v", err)
	}
	file := AdmittedSourceManifestFile{
		Role:                FileRoleProject,
		CachePath:           "project.json",
		SizeBytes:           int64(len(`{"name":"changed"}`)),
		CanonicalJSONSHA256: &hash,
	}

	err = VerifyCachedFile(file, []byte(`{"name":"changed"}`))
	if !errors.Is(err, ErrCacheCorruption) {
		t.Fatalf("VerifyCachedFile() error = %v, want cache corruption", err)
	}
}

func TestVerifyCachedFileDetectsSizeMismatch(t *testing.T) {
	file := AdmittedSourceManifestFile{
		Role:      FileRoleSupportFile,
		CachePath: "scripts/run.py",
		SizeBytes: 999,
	}
	err := VerifyCachedFile(file, []byte("print('hi')\n"))
	if !errors.Is(err, ErrCacheCorruption) {
		t.Fatalf("VerifyCachedFile() error = %v, want cache corruption", err)
	}
}

func TestVerifyCachedFileAcceptsEquivalentCanonicalJSON(t *testing.T) {
	_, hash, err := fingerprint.CanonicalJSONSHA256(map[string]any{"name": "demo"})
	if err != nil {
		t.Fatalf("CanonicalJSONSHA256() error = %v", err)
	}
	data := []byte("{\n  \"name\": \"demo\"\n}")
	file := AdmittedSourceManifestFile{
		Role:                FileRoleProject,
		CachePath:           "project.json",
		SizeBytes:           int64(len(data)),
		CanonicalJSONSHA256: &hash,
	}
	if err := VerifyCachedFile(file, data); err != nil {
		t.Fatalf("VerifyCachedFile() error = %v", err)
	}
}

func testCacheManifest(t *testing.T, provider string, runID string, cachePath string, data []byte) (AdmittedSourceManifest, []ReadFileResult) {
	t.Helper()
	repository := RepositoryIdentity{Value: "local:demo"}
	var revision *string
	if provider == "github" {
		value := "0123456789abcdef0123456789abcdef01234567"
		revision = &value
		repository = RepositoryIdentity{Value: "github.com/acme/demo"}
	}
	source := ResolvedSourceReference{
		Repository:   repository,
		RequestedRef: "main",
		RevisionID:   revision,
	}
	request := SourceFileRequest{
		Repository: repository,
		RevisionID: revision,
		SourcePath: cachePath,
	}
	read := newReadFileResult(request, data, nil)
	_, canonical, err := fingerprint.CanonicalJSONSHA256(map[string]any{"name": "demo"})
	if err != nil {
		t.Fatalf("CanonicalJSONSHA256() error = %v", err)
	}
	contentType := "application/octet-stream"
	var canonicalPtr *string
	if cachePath == "project.json" || cachePath == "workflows/train.json" {
		contentType = "application/json"
		canonicalPtr = &canonical
	}
	manifest, err := BuildAdmittedSourceManifest(runID, source, []DeclaredSourceFile{
		{
			Role:                FileRoleProject,
			SourcePath:          cachePath,
			CachePath:           cachePath,
			ContentType:         contentType,
			CanonicalJSONSHA256: canonicalPtr,
		},
	}, []ReadFileResult{read})
	if err != nil {
		t.Fatalf("BuildAdmittedSourceManifest() error = %v", err)
	}
	return manifest, []ReadFileResult{read}
}
