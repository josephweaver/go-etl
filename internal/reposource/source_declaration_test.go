package reposource

import (
	"encoding/json"
	"testing"
)

func TestSourceManifestDeclarationAcceptsSupplementalFiles(t *testing.T) {
	var declaration SourceManifestDeclaration
	if err := json.Unmarshal([]byte(`{
		"files": [
			{"role": "python_entrypoint", "path": "scripts/train.py", "content_type": "text/x-python"},
			{"role": "python_environment", "path": "environments/python.json", "content_type": "application/json"},
			{"role": "support_file", "path": "scripts/lib/helpers.py", "content_type": "text/x-python"}
		]
	}`), &declaration); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if err := declaration.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}

	files, err := declaration.DeclaredSourceFiles()
	if err != nil {
		t.Fatalf("DeclaredSourceFiles() error = %v", err)
	}
	if len(files) != 3 {
		t.Fatalf("len(files) = %d, want 3", len(files))
	}
	if files[0].SourcePath != "scripts/train.py" || files[0].CachePath != "scripts/train.py" {
		t.Fatalf("declared paths = %q/%q", files[0].SourcePath, files[0].CachePath)
	}
}

func TestSourceManifestDeclarationAllowsAbsentManifest(t *testing.T) {
	var declaration SourceManifestDeclaration
	if err := declaration.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestSourceManifestDeclarationRejectsUnsafePaths(t *testing.T) {
	for _, path := range []string{"", ".", "../secret.py", "/secret.py", "C:/secret.py", `scripts\train.py`} {
		t.Run(path, func(t *testing.T) {
			declaration := SourceManifestDeclaration{Files: []SourceManifestFileDeclaration{
				{Role: FileRolePythonEntrypoint, Path: path},
			}}
			if err := declaration.Validate(); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestSourceManifestDeclarationRejectsDuplicatePaths(t *testing.T) {
	declaration := SourceManifestDeclaration{Files: []SourceManifestFileDeclaration{
		{Role: FileRolePythonEntrypoint, Path: "scripts/train.py"},
		{Role: FileRoleSupportFile, Path: "scripts/train.py"},
	}}
	if err := declaration.Validate(); err == nil {
		t.Fatal("expected duplicate path error")
	}
}

func TestSourceManifestDeclarationRejectsUnsupportedRoles(t *testing.T) {
	for _, role := range []FileRole{FileRoleProject, FileRoleWorkflow, FileRole("container_image")} {
		t.Run(string(role), func(t *testing.T) {
			declaration := SourceManifestDeclaration{Files: []SourceManifestFileDeclaration{
				{Role: role, Path: "project.json"},
			}}
			if err := declaration.Validate(); err == nil {
				t.Fatal("expected unsupported role error")
			}
		})
	}
}

func TestSourceManifestDeclarationRejectsCachePath(t *testing.T) {
	var declaration SourceManifestDeclaration
	err := json.Unmarshal([]byte(`{"files":[{"role":"support_file","path":"scripts/helper.py","cache_path":"files/helper.py"}]}`), &declaration)
	if err == nil {
		t.Fatal("expected cache_path decode error")
	}
}
