package reposource

import (
	"encoding/json"
	"fmt"
)

// SourceManifestDeclaration names supplemental repository files required by a workflow source.
type SourceManifestDeclaration struct {
	Files []SourceManifestFileDeclaration `json:"files"`
}

type SourceManifestFileDeclaration struct {
	Role        FileRole `json:"role"`
	Path        string   `json:"path"`
	ContentType string   `json:"content_type,omitempty"`
}

func (d SourceManifestDeclaration) Validate() error {
	seen := make(map[string]struct{}, len(d.Files))
	for index, file := range d.Files {
		if err := file.Validate(); err != nil {
			return fmt.Errorf("source_manifest.files[%d]: %w", index, err)
		}
		path, err := ValidateRepositoryRelativePath(file.Path)
		if err != nil {
			return fmt.Errorf("source_manifest.files[%d].path: %w", index, err)
		}
		if _, ok := seen[path]; ok {
			return fmt.Errorf("source_manifest.files[%d].path: duplicate path %s", index, path)
		}
		seen[path] = struct{}{}
	}
	return nil
}

func (f SourceManifestFileDeclaration) Validate() error {
	switch f.Role {
	case FileRolePythonEntrypoint, FileRolePythonEnvironment, FileRoleSupportFile:
	default:
		return fmt.Errorf("unsupported role %q", f.Role)
	}
	if _, err := ValidateRepositoryRelativePath(f.Path); err != nil {
		return err
	}
	return nil
}

func (f SourceManifestFileDeclaration) DeclaredSourceFile() DeclaredSourceFile {
	path, _ := ValidateRepositoryRelativePath(f.Path)
	return DeclaredSourceFile{
		Role:        f.Role,
		SourcePath:  path,
		CachePath:   path,
		ContentType: f.ContentType,
	}
}

func (d SourceManifestDeclaration) DeclaredSourceFiles() ([]DeclaredSourceFile, error) {
	if err := d.Validate(); err != nil {
		return nil, err
	}
	files := make([]DeclaredSourceFile, 0, len(d.Files))
	for _, file := range d.Files {
		files = append(files, file.DeclaredSourceFile())
	}
	return files, nil
}

func (f *SourceManifestFileDeclaration) UnmarshalJSON(data []byte) error {
	type sourceManifestFileDeclaration SourceManifestFileDeclaration
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	if _, ok := raw["cache_path"]; ok {
		return fmt.Errorf("cache_path is controller-owned and must not be declared")
	}
	var decoded sourceManifestFileDeclaration
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*f = SourceManifestFileDeclaration(decoded)
	return nil
}
