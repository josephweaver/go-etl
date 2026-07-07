package model

import (
	"fmt"
	"strings"
)

const ArtifactManifestSchemaV1 = "goet/artifact-manifest/v1"

const (
	ArtifactKindFile      = "file"
	ArtifactKindDirectory = "directory"
)

type ArtifactManifest struct {
	Schema       string               `json:"schema"`
	RunID        string               `json:"run_id,omitempty"`
	StageIndex   *int                 `json:"stage_index,omitempty"`
	StepIndex    *int                 `json:"step_index,omitempty"`
	WorkItemID   string               `json:"work_item_id,omitempty"`
	AttemptID    string               `json:"attempt_id,omitempty"`
	StorageScope string               `json:"storage_scope"`
	Artifacts    []ArtifactDescriptor `json:"artifacts"`
	ScriptOutput any                  `json:"script_output,omitempty"`
}

type ArtifactDescriptor struct {
	Name           string         `json:"name"`
	Kind           string         `json:"kind"`
	Format         string         `json:"format,omitempty"`
	Path           string         `json:"path"`
	ContentType    string         `json:"content_type,omitempty"`
	SizeBytes      *int64         `json:"size_bytes,omitempty"`
	SHA256         string         `json:"sha256,omitempty"`
	ManifestSHA256 string         `json:"manifest_sha256,omitempty"`
	RecordCount    *int64         `json:"record_count,omitempty"`
	SchemaRef      string         `json:"schema_ref,omitempty"`
	Metadata       map[string]any `json:"metadata,omitempty"`
}

func (manifest ArtifactManifest) EffectiveSchema() string {
	if strings.TrimSpace(manifest.Schema) == "" {
		return ArtifactManifestSchemaV1
	}
	return manifest.Schema
}

func (manifest ArtifactManifest) Validate() error {
	schema := manifest.EffectiveSchema()
	if schema != ArtifactManifestSchemaV1 {
		return fmt.Errorf("unsupported artifact manifest schema: %s", manifest.Schema)
	}
	if strings.TrimSpace(manifest.StorageScope) == "" {
		return fmt.Errorf("storage scope is required")
	}
	if manifest.StageIndex != nil && *manifest.StageIndex < 0 {
		return fmt.Errorf("stage index must be non-negative")
	}
	if manifest.StepIndex != nil && *manifest.StepIndex < 0 {
		return fmt.Errorf("step index must be non-negative")
	}
	if len(manifest.Artifacts) == 0 {
		return fmt.Errorf("at least one artifact is required")
	}
	for i, artifact := range manifest.Artifacts {
		if err := artifact.Validate(); err != nil {
			return fmt.Errorf("artifact %d: %w", i, err)
		}
	}
	return nil
}

func (artifact ArtifactDescriptor) Validate() error {
	if strings.TrimSpace(artifact.Name) == "" {
		return fmt.Errorf("artifact name is required")
	}
	if strings.TrimSpace(artifact.Kind) == "" {
		return fmt.Errorf("artifact kind is required")
	}
	if strings.TrimSpace(artifact.Path) == "" {
		return fmt.Errorf("artifact path is required")
	}
	if _, err := ValidateArtifactRelativePath(artifact.Path); err != nil {
		return err
	}
	if artifact.SizeBytes != nil && *artifact.SizeBytes < 0 {
		return fmt.Errorf("artifact size bytes must be non-negative")
	}
	if artifact.RecordCount != nil && *artifact.RecordCount < 0 {
		return fmt.Errorf("artifact record count must be non-negative")
	}

	switch artifact.Kind {
	case ArtifactKindFile, ArtifactKindDirectory:
		return nil
	default:
		return fmt.Errorf("unsupported artifact kind: %s", artifact.Kind)
	}
}
