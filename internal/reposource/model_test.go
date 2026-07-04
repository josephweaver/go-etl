package reposource

import "testing"

func TestModelConstructionUsesNullablePointers(t *testing.T) {
	revisionID := "rev-123"
	objectID := "obj-456"
	rawSHA256 := "raw"
	canonicalJSONSHA256 := "canonical"

	manifest := AdmittedSourceManifest{
		Schema: "goet/reposource/v1alpha1",
		RunID:  "run-1",
		Source: ResolvedSourceReference{
			Repository: RepositoryIdentity{
				Value:       "github.com/acme/project",
				DisplayName: "Acme Project",
			},
			RequestedRef: "main",
			RevisionID:   &revisionID,
		},
		Files: []AdmittedSourceManifestFile{
			{
				Role:                FileRoleProject,
				SourcePath:          "project.json",
				CachePath:           "files/project.json",
				ObjectID:            &objectID,
				SizeBytes:           17,
				RawSHA256:           &rawSHA256,
				CanonicalJSONSHA256: &canonicalJSONSHA256,
				ContentType:         "application/json",
			},
		},
	}

	if manifest.Source.RevisionID == nil {
		t.Fatal("revision id is nil")
	}
	if manifest.Files[0].ObjectID == nil {
		t.Fatal("object id is nil")
	}
	if manifest.Files[0].RawSHA256 == nil {
		t.Fatal("raw sha256 is nil")
	}
	if manifest.Files[0].CanonicalJSONSHA256 == nil {
		t.Fatal("canonical json sha256 is nil")
	}
	if got := manifest.Files[0].Role; got != FileRoleProject {
		t.Fatalf("role = %q, want %q", got, FileRoleProject)
	}
}

func TestFileRoleConstants(t *testing.T) {
	cases := []struct {
		name string
		role FileRole
		want string
	}{
		{name: "project", role: FileRoleProject, want: "project"},
		{name: "workflow", role: FileRoleWorkflow, want: "workflow"},
		{name: "python_entrypoint", role: FileRolePythonEntrypoint, want: "python_entrypoint"},
		{name: "python_environment", role: FileRolePythonEnvironment, want: "python_environment"},
		{name: "support_file", role: FileRoleSupportFile, want: "support_file"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := string(tc.role); got != tc.want {
				t.Fatalf("role = %q, want %q", got, tc.want)
			}
		})
	}
}
