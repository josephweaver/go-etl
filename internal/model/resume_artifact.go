package model

import (
	"fmt"
	"strings"
	"time"
	"unicode"
)

const ResumeArtifactSchemaV1 = "goet/resume-artifact/v1"

const (
	ResumeArtifactStorageScopeSharedTmp    = "shared_tmp"
	ResumeArtifactRetentionWhileReferenced = "while_referenced"
)

type PauseStrategy string

const (
	PauseStrategyDMTCP  PauseStrategy = "dmtcp"
	PauseStrategyNative PauseStrategy = "native"
	PauseStrategyManual PauseStrategy = "manual"
)

type ResumeArtifactManifest struct {
	Schema              string                      `json:"schema"`
	ResumeArtifactID    string                      `json:"resume_artifact_id"`
	ResumeGeneration    int                         `json:"resume_generation"`
	PauseStrategy       PauseStrategy               `json:"pause_strategy"`
	WorkItemID          string                      `json:"work_item_id"`
	WorkItemType        WorkItemType                `json:"work_item_type"`
	ProducingAttemptID  string                      `json:"producing_attempt_id"`
	ExecutionLineageID  string                      `json:"execution_lineage_id"`
	InputFingerprint    string                      `json:"input_fingerprint"`
	SourceVersion       string                      `json:"source_version"`
	CodeVersion         string                      `json:"code_version"`
	CreatedAt           string                      `json:"created_at"`
	StorageScope        string                      `json:"storage_scope"`
	StorageRelativePath string                      `json:"storage_relative_path"`
	RetentionPolicy     string                      `json:"retention_policy"`
	Compatibility       ResumeArtifactCompatibility `json:"compatibility"`
	Files               []ResumeArtifactFile        `json:"files"`
	DMTCP               *DMTCPResumePayload         `json:"dmtcp,omitempty"`
	Native              *NativeResumePayload        `json:"native,omitempty"`
	Manual              *ManualResumePayload        `json:"manual,omitempty"`
}

type ResumeArtifactReference struct {
	Schema               string `json:"schema"`
	ResumeArtifactID     string `json:"resume_artifact_id"`
	StorageScope         string `json:"storage_scope"`
	ManifestRelativePath string `json:"manifest_relative_path"`
	ManifestSHA256       string `json:"manifest_sha256"`
}

type ResumeArtifactCompatibility struct {
	AdapterID                      string `json:"adapter_id"`
	AdapterVersion                 string `json:"adapter_version"`
	WorkerExecutionContractVersion string `json:"worker_execution_contract_version"`
	WorkerVersion                  string `json:"worker_version"`
	ContainerImageIdentity         string `json:"container_image_identity"`
	OperatingSystem                string `json:"operating_system"`
	Architecture                   string `json:"architecture"`
	ContainerRuntime               string `json:"container_runtime"`
}

type ResumeArtifactFile struct {
	Path      string `json:"path"`
	SizeBytes int64  `json:"size_bytes"`
	SHA256    string `json:"sha256"`
}

type DMTCPResumePayload struct {
	BuildIdentity   string   `json:"build_identity"`
	CheckpointPaths []string `json:"checkpoint_paths"`
}

type NativeResumePayload struct {
	Operation       string   `json:"operation"`
	AdapterVersion  string   `json:"adapter_version"`
	BackendIdentity string   `json:"backend_identity"`
	StateFilePaths  []string `json:"state_file_paths"`
}

type ManualResumePayload struct {
	HandlerID      string `json:"handler_id"`
	HandlerVersion string `json:"handler_version"`
	StateSchema    string `json:"state_schema"`
	StateFilePath  string `json:"state_file_path"`
}

func (manifest ResumeArtifactManifest) Validate() error {
	if manifest.Schema != ResumeArtifactSchemaV1 {
		return fmt.Errorf("unsupported resume artifact schema %q", manifest.Schema)
	}
	if err := validateResumeArtifactID(manifest.ResumeArtifactID); err != nil {
		return err
	}
	if manifest.ResumeGeneration < 1 {
		return fmt.Errorf("resume_generation must be at least 1")
	}
	if err := validatePauseStrategy(manifest.PauseStrategy); err != nil {
		return err
	}
	for _, field := range []struct {
		name  string
		value string
	}{
		{name: "work_item_id", value: manifest.WorkItemID},
		{name: "work_item_type", value: string(manifest.WorkItemType)},
		{name: "producing_attempt_id", value: manifest.ProducingAttemptID},
		{name: "execution_lineage_id", value: manifest.ExecutionLineageID},
		{name: "input_fingerprint", value: manifest.InputFingerprint},
		{name: "source_version", value: manifest.SourceVersion},
		{name: "code_version", value: manifest.CodeVersion},
		{name: "created_at", value: manifest.CreatedAt},
	} {
		if err := validateRequiredResumeValue(field.name, field.value); err != nil {
			return err
		}
	}
	if _, err := time.Parse(time.RFC3339, manifest.CreatedAt); err != nil {
		return fmt.Errorf("created_at must be RFC 3339: %w", err)
	}
	if manifest.StorageScope != ResumeArtifactStorageScopeSharedTmp {
		return fmt.Errorf("unsupported resume artifact storage_scope %q", manifest.StorageScope)
	}
	if err := validateResumeRelativePath("storage_relative_path", manifest.StorageRelativePath); err != nil {
		return err
	}
	if manifest.RetentionPolicy != ResumeArtifactRetentionWhileReferenced {
		return fmt.Errorf("unsupported resume artifact retention_policy %q", manifest.RetentionPolicy)
	}
	if err := manifest.Compatibility.Validate(); err != nil {
		return fmt.Errorf("compatibility: %w", err)
	}

	filePaths, err := validateResumeArtifactFiles(manifest.Files)
	if err != nil {
		return err
	}

	payloadCount := 0
	if manifest.DMTCP != nil {
		payloadCount++
	}
	if manifest.Native != nil {
		payloadCount++
	}
	if manifest.Manual != nil {
		payloadCount++
	}
	if payloadCount != 1 {
		return fmt.Errorf("exactly one resume strategy payload is required")
	}

	switch manifest.PauseStrategy {
	case PauseStrategyDMTCP:
		if manifest.DMTCP == nil {
			return fmt.Errorf("pause_strategy %q requires dmtcp payload", manifest.PauseStrategy)
		}
		if err := manifest.DMTCP.validate(filePaths); err != nil {
			return fmt.Errorf("dmtcp: %w", err)
		}
	case PauseStrategyNative:
		if manifest.Native == nil {
			return fmt.Errorf("pause_strategy %q requires native payload", manifest.PauseStrategy)
		}
		if err := manifest.Native.validate(filePaths); err != nil {
			return fmt.Errorf("native: %w", err)
		}
		if manifest.Native.AdapterVersion != manifest.Compatibility.AdapterVersion {
			return fmt.Errorf("native adapter_version must match compatibility adapter_version")
		}
	case PauseStrategyManual:
		if manifest.Manual == nil {
			return fmt.Errorf("pause_strategy %q requires manual payload", manifest.PauseStrategy)
		}
		if err := manifest.Manual.validate(filePaths); err != nil {
			return fmt.Errorf("manual: %w", err)
		}
	}

	return nil
}

func (reference ResumeArtifactReference) Validate() error {
	if reference.Schema != ResumeArtifactSchemaV1 {
		return fmt.Errorf("unsupported resume artifact schema %q", reference.Schema)
	}
	if err := validateResumeArtifactID(reference.ResumeArtifactID); err != nil {
		return err
	}
	if reference.StorageScope != ResumeArtifactStorageScopeSharedTmp {
		return fmt.Errorf("unsupported resume artifact storage_scope %q", reference.StorageScope)
	}
	if err := validateResumeRelativePath("manifest_relative_path", reference.ManifestRelativePath); err != nil {
		return err
	}
	if reference.ManifestSHA256 == "" {
		return fmt.Errorf("manifest_sha256 is required")
	}
	if err := validateOptionalSHA256("manifest_sha256", reference.ManifestSHA256); err != nil {
		return err
	}
	return nil
}

func (compatibility ResumeArtifactCompatibility) Validate() error {
	for _, field := range []struct {
		name  string
		value string
	}{
		{name: "adapter_id", value: compatibility.AdapterID},
		{name: "adapter_version", value: compatibility.AdapterVersion},
		{name: "worker_execution_contract_version", value: compatibility.WorkerExecutionContractVersion},
		{name: "worker_version", value: compatibility.WorkerVersion},
		{name: "container_image_identity", value: compatibility.ContainerImageIdentity},
		{name: "operating_system", value: compatibility.OperatingSystem},
		{name: "architecture", value: compatibility.Architecture},
		{name: "container_runtime", value: compatibility.ContainerRuntime},
	} {
		if err := validateRequiredResumeValue(field.name, field.value); err != nil {
			return err
		}
	}
	return nil
}

func (file ResumeArtifactFile) Validate() error {
	if err := validateResumeRelativePath("file path", file.Path); err != nil {
		return err
	}
	if file.SizeBytes < 0 {
		return fmt.Errorf("file size_bytes must be non-negative")
	}
	if file.SHA256 == "" {
		return fmt.Errorf("file sha256 is required")
	}
	if err := validateOptionalSHA256("file sha256", file.SHA256); err != nil {
		return err
	}
	return nil
}

func (payload DMTCPResumePayload) validate(filePaths map[string]struct{}) error {
	if err := validateRequiredResumeValue("build_identity", payload.BuildIdentity); err != nil {
		return err
	}
	return validateResumePayloadPaths("checkpoint_paths", payload.CheckpointPaths, filePaths)
}

func (payload NativeResumePayload) validate(filePaths map[string]struct{}) error {
	if err := validateRequiredResumeValue("operation", payload.Operation); err != nil {
		return err
	}
	if err := validateRequiredResumeValue("adapter_version", payload.AdapterVersion); err != nil {
		return err
	}
	if err := validateRequiredResumeValue("backend_identity", payload.BackendIdentity); err != nil {
		return err
	}
	return validateResumePayloadPaths("state_file_paths", payload.StateFilePaths, filePaths)
}

func (payload ManualResumePayload) validate(filePaths map[string]struct{}) error {
	for _, field := range []struct {
		name  string
		value string
	}{
		{name: "handler_id", value: payload.HandlerID},
		{name: "handler_version", value: payload.HandlerVersion},
		{name: "state_schema", value: payload.StateSchema},
	} {
		if err := validateRequiredResumeValue(field.name, field.value); err != nil {
			return err
		}
	}
	if err := validateResumeRelativePath("state_file_path", payload.StateFilePath); err != nil {
		return err
	}
	if _, ok := filePaths[payload.StateFilePath]; !ok {
		return fmt.Errorf("state_file_path %q is not present in files", payload.StateFilePath)
	}
	return nil
}

func validatePauseStrategy(strategy PauseStrategy) error {
	switch strategy {
	case PauseStrategyDMTCP, PauseStrategyNative, PauseStrategyManual:
		return nil
	default:
		return fmt.Errorf("unsupported pause_strategy %q", strategy)
	}
}

func validateResumeArtifactID(value string) error {
	if err := validateRequiredResumeValue("resume_artifact_id", value); err != nil {
		return err
	}
	if value == "." || value == ".." {
		return fmt.Errorf("resume_artifact_id must be a safe path segment")
	}
	for _, r := range value {
		if (r >= 'a' && r <= 'z') ||
			(r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') ||
			r == '.' || r == '_' || r == '-' {
			continue
		}
		return fmt.Errorf("resume_artifact_id must contain only letters, digits, dot, underscore, or hyphen")
	}
	return nil
}

func validateRequiredResumeValue(field, value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("%s is required", field)
	}
	if strings.TrimSpace(value) != value {
		return fmt.Errorf("%s must not contain leading or trailing whitespace", field)
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return fmt.Errorf("%s must not contain control characters", field)
		}
	}
	return nil
}

func validateResumeRelativePath(field, value string) error {
	if _, err := ValidateArtifactRelativePath(value); err != nil {
		return fmt.Errorf("%s: %w", field, err)
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return fmt.Errorf("%s must not contain control characters", field)
		}
	}
	return nil
}

func validateResumeArtifactFiles(files []ResumeArtifactFile) (map[string]struct{}, error) {
	if len(files) == 0 {
		return nil, fmt.Errorf("at least one resume artifact file is required")
	}
	paths := make(map[string]struct{}, len(files))
	for i, file := range files {
		if err := file.Validate(); err != nil {
			return nil, fmt.Errorf("file %d: %w", i, err)
		}
		if _, exists := paths[file.Path]; exists {
			return nil, fmt.Errorf("duplicate resume artifact file path %q", file.Path)
		}
		paths[file.Path] = struct{}{}
	}
	return paths, nil
}

func validateResumePayloadPaths(field string, paths []string, filePaths map[string]struct{}) error {
	if len(paths) == 0 {
		return fmt.Errorf("%s requires at least one path", field)
	}
	seen := make(map[string]struct{}, len(paths))
	for i, path := range paths {
		if err := validateResumeRelativePath(fmt.Sprintf("%s[%d]", field, i), path); err != nil {
			return err
		}
		if _, exists := seen[path]; exists {
			return fmt.Errorf("%s contains duplicate path %q", field, path)
		}
		seen[path] = struct{}{}
		if _, exists := filePaths[path]; !exists {
			return fmt.Errorf("%s path %q is not present in files", field, path)
		}
	}
	return nil
}
