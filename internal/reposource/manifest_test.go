package reposource

import "testing"

func TestBuildAdmittedSourceManifestRecordsDeclaredFiles(t *testing.T) {
	revision := "rev-1"
	canonical := "canonical-json-sha"
	objectID := "blob-1"
	source := ResolvedSourceReference{
		Repository:   RepositoryIdentity{Value: "github.com/acme/demo"},
		RequestedRef: "main",
		RevisionID:   &revision,
	}
	request := SourceFileRequest{
		Repository: source.Repository,
		RevisionID: &revision,
		SourcePath: "project.json",
	}
	read := newReadFileResult(request, []byte(`{"name":"demo"}`), &objectID)

	manifest, err := BuildAdmittedSourceManifest("run-1", source, []DeclaredSourceFile{
		{
			Role:                FileRoleProject,
			SourcePath:          "project.json",
			CachePath:           "files/project.json",
			ContentType:         "application/json",
			CanonicalJSONSHA256: &canonical,
		},
	}, []ReadFileResult{read})
	if err != nil {
		t.Fatalf("BuildAdmittedSourceManifest() error = %v", err)
	}
	if manifest.Schema != AdmittedSourceManifestSchemaV1 {
		t.Fatalf("schema = %q, want %q", manifest.Schema, AdmittedSourceManifestSchemaV1)
	}
	if len(manifest.Files) != 1 {
		t.Fatalf("len(files) = %d, want 1", len(manifest.Files))
	}
	file := manifest.Files[0]
	if file.Role != FileRoleProject {
		t.Fatalf("role = %q, want %q", file.Role, FileRoleProject)
	}
	if file.SourcePath != "project.json" || file.CachePath != "files/project.json" {
		t.Fatalf("paths = %q/%q", file.SourcePath, file.CachePath)
	}
	if file.ObjectID == nil || *file.ObjectID != objectID {
		t.Fatalf("object id = %v, want %s", file.ObjectID, objectID)
	}
	if file.RawSHA256 == nil || *file.RawSHA256 == "" {
		t.Fatal("raw sha256 is empty")
	}
	if file.CanonicalJSONSHA256 == nil || *file.CanonicalJSONSHA256 != canonical {
		t.Fatalf("canonical json sha = %v, want %s", file.CanonicalJSONSHA256, canonical)
	}
	if file.SizeBytes != int64(len(`{"name":"demo"}`)) {
		t.Fatalf("size = %d", file.SizeBytes)
	}
}

func TestBuildAdmittedSourceManifestFailsWhenDeclaredFileWasNotRead(t *testing.T) {
	source := ResolvedSourceReference{
		Repository:   RepositoryIdentity{Value: "local:demo"},
		RequestedRef: "working-tree",
	}
	_, err := BuildAdmittedSourceManifest("run-1", source, []DeclaredSourceFile{
		{Role: FileRoleWorkflow, SourcePath: "workflow.json", CachePath: "files/workflow.json"},
	}, nil)
	if err == nil {
		t.Fatal("expected missing read error")
	}
}
