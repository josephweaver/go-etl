package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path"
	"reflect"
	"strings"
	"unicode"

	"goetl/internal/model"
)

type PauseAdapterCapabilities struct {
	Shutdown bool
	Periodic bool
	Yield    bool
}

func (capabilities PauseAdapterCapabilities) Validate() error {
	if !capabilities.Shutdown {
		return fmt.Errorf("pause adapter must support shutdown capture")
	}
	return nil
}

func (capabilities PauseAdapterCapabilities) Supports(mode CheckpointMode) bool {
	switch mode {
	case CheckpointModeDisabled:
		return true
	case CheckpointModeShutdown:
		return capabilities.Shutdown
	case CheckpointModePeriodic:
		return capabilities.Shutdown && capabilities.Periodic
	case CheckpointModeYield:
		return capabilities.Shutdown && capabilities.Yield
	default:
		return false
	}
}

type PauseAdapterRegistration struct {
	WorkItemType   model.WorkItemType
	Strategy       model.PauseStrategy
	AdapterID      string
	AdapterVersion string
	Capabilities   PauseAdapterCapabilities
	Adapter        PauseAdapter
}

func (registration PauseAdapterRegistration) Validate() error {
	if err := validateAdapterContractValue("work item type", string(registration.WorkItemType)); err != nil {
		return err
	}
	switch registration.Strategy {
	case model.PauseStrategyDMTCP, model.PauseStrategyNative, model.PauseStrategyManual:
	default:
		return fmt.Errorf("unsupported pause adapter strategy %q", registration.Strategy)
	}
	if err := validateAdapterContractValue("adapter id", registration.AdapterID); err != nil {
		return err
	}
	if err := validateAdapterContractValue("adapter version", registration.AdapterVersion); err != nil {
		return err
	}
	if err := registration.Capabilities.Validate(); err != nil {
		return err
	}
	if pauseAdapterIsNil(registration.Adapter) {
		return fmt.Errorf("pause adapter is required")
	}
	return nil
}

type PauseAdapterRegistry struct {
	byWorkItemType map[model.WorkItemType]PauseAdapterRegistration
}

func NewPauseAdapterRegistry(registrations ...PauseAdapterRegistration) (PauseAdapterRegistry, error) {
	registry := PauseAdapterRegistry{
		byWorkItemType: make(map[model.WorkItemType]PauseAdapterRegistration, len(registrations)),
	}
	for i, registration := range registrations {
		if err := registration.Validate(); err != nil {
			return PauseAdapterRegistry{}, fmt.Errorf("pause adapter registration %d: %w", i, err)
		}
		if _, exists := registry.byWorkItemType[registration.WorkItemType]; exists {
			return PauseAdapterRegistry{}, fmt.Errorf("duplicate pause adapter registration for work item type %q", registration.WorkItemType)
		}
		registry.byWorkItemType[registration.WorkItemType] = registration
	}
	return registry, nil
}

func (registry PauseAdapterRegistry) AdapterFor(workItemType model.WorkItemType) (PauseAdapterRegistration, bool) {
	registration, found := registry.byWorkItemType[workItemType]
	return registration, found
}

func (registry PauseAdapterRegistry) Len() int {
	return len(registry.byWorkItemType)
}

type PauseAdapter interface {
	StartFresh(context.Context, model.WorkItem) (SupervisedExecution, error)
	StartResume(context.Context, model.WorkItem, model.WorkItemResumeAssignment) (SupervisedExecution, error)
}

type SupervisedExecution interface {
	Result() <-chan ExecutionResult
	CapturePeriodic(context.Context, CheckpointCaptureRequest) (PreparedCheckpoint, error)
	CaptureForSuspend(context.Context, CheckpointCaptureRequest) (PreparedCheckpoint, error)
	Continue(context.Context) error
	Terminate(context.Context) error
}

type ExecutionResult struct {
	Evidence WorkEvidence
	Err      error
}

type CheckpointCaptureRequest struct {
	WorkItemID                  string
	WorkItemType                model.WorkItemType
	AttemptID                   string
	ExecutionLineageID          string
	ResumeGeneration            int
	ResumeArtifactID            string
	ArtifactStorageRelativePath string
	CaptureKind                 model.CheckpointCaptureKind
}

func (request CheckpointCaptureRequest) Validate() error {
	for _, field := range []struct {
		name  string
		value string
	}{
		{name: "work item id", value: request.WorkItemID},
		{name: "work item type", value: string(request.WorkItemType)},
		{name: "attempt id", value: request.AttemptID},
		{name: "execution lineage id", value: request.ExecutionLineageID},
	} {
		if err := validateAdapterContractValue(field.name, field.value); err != nil {
			return err
		}
	}
	if request.ResumeGeneration < 1 {
		return fmt.Errorf("resume generation must be at least 1")
	}
	if err := validateAdapterArtifactID(request.ResumeArtifactID); err != nil {
		return err
	}
	if err := validateAdapterContractValue("artifact storage relative path", request.ArtifactStorageRelativePath); err != nil {
		return err
	}
	if _, err := model.ValidateArtifactRelativePath(request.ArtifactStorageRelativePath); err != nil {
		return fmt.Errorf("artifact storage relative path: %w", err)
	}
	switch request.CaptureKind {
	case model.CheckpointCaptureKindPeriodic, model.CheckpointCaptureKindQuantum, model.CheckpointCaptureKindFinal:
		return nil
	default:
		return fmt.Errorf("unsupported checkpoint capture kind %q", request.CaptureKind)
	}
}

type PreparedCheckpoint struct {
	ManifestJSON string
	Reference    model.ResumeArtifactReference
}

func (checkpoint PreparedCheckpoint) Validate(
	request CheckpointCaptureRequest,
	registration PauseAdapterRegistration,
) (model.ResumeArtifactManifest, error) {
	if err := request.Validate(); err != nil {
		return model.ResumeArtifactManifest{}, fmt.Errorf("capture request: %w", err)
	}
	if err := registration.Validate(); err != nil {
		return model.ResumeArtifactManifest{}, fmt.Errorf("adapter registration: %w", err)
	}
	if registration.WorkItemType != request.WorkItemType {
		return model.ResumeArtifactManifest{}, fmt.Errorf("adapter work item type does not match capture request")
	}
	if checkpoint.ManifestJSON == "" {
		return model.ResumeArtifactManifest{}, fmt.Errorf("checkpoint manifest JSON is required")
	}
	if err := checkpoint.Reference.Validate(); err != nil {
		return model.ResumeArtifactManifest{}, fmt.Errorf("checkpoint reference: %w", err)
	}

	var manifest model.ResumeArtifactManifest
	if err := json.Unmarshal([]byte(checkpoint.ManifestJSON), &manifest); err != nil {
		return model.ResumeArtifactManifest{}, fmt.Errorf("checkpoint manifest JSON is invalid")
	}
	if err := manifest.Validate(); err != nil {
		return model.ResumeArtifactManifest{}, fmt.Errorf("checkpoint manifest: %w", err)
	}
	digest := sha256.Sum256([]byte(checkpoint.ManifestJSON))
	if checkpoint.Reference.ManifestSHA256 != hex.EncodeToString(digest[:]) {
		return model.ResumeArtifactManifest{}, fmt.Errorf("checkpoint reference digest does not match exact manifest JSON")
	}
	if manifest.ResumeArtifactID != checkpoint.Reference.ResumeArtifactID ||
		manifest.StorageScope != checkpoint.Reference.StorageScope {
		return model.ResumeArtifactManifest{}, fmt.Errorf("checkpoint manifest and reference identity do not match")
	}
	if !adapterCheckpointPathInside(checkpoint.Reference.ManifestRelativePath, manifest.StorageRelativePath) {
		return model.ResumeArtifactManifest{}, fmt.Errorf("checkpoint reference manifest path is outside artifact storage directory")
	}

	for _, match := range []struct {
		name string
		got  string
		want string
	}{
		{name: "work item id", got: manifest.WorkItemID, want: request.WorkItemID},
		{name: "work item type", got: string(manifest.WorkItemType), want: string(request.WorkItemType)},
		{name: "producing attempt id", got: manifest.ProducingAttemptID, want: request.AttemptID},
		{name: "execution lineage id", got: manifest.ExecutionLineageID, want: request.ExecutionLineageID},
		{name: "resume artifact id", got: manifest.ResumeArtifactID, want: request.ResumeArtifactID},
		{name: "artifact storage relative path", got: manifest.StorageRelativePath, want: request.ArtifactStorageRelativePath},
		{name: "pause strategy", got: string(manifest.PauseStrategy), want: string(registration.Strategy)},
		{name: "adapter id", got: manifest.Compatibility.AdapterID, want: registration.AdapterID},
		{name: "adapter version", got: manifest.Compatibility.AdapterVersion, want: registration.AdapterVersion},
	} {
		if match.got != match.want {
			return model.ResumeArtifactManifest{}, fmt.Errorf("checkpoint manifest %s does not match capture contract", match.name)
		}
	}
	if manifest.ResumeGeneration != request.ResumeGeneration {
		return model.ResumeArtifactManifest{}, fmt.Errorf("checkpoint manifest resume generation does not match capture contract")
	}

	return manifest, nil
}

func validateAdapterContractValue(name string, value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("%s is required", name)
	}
	if strings.TrimSpace(value) != value {
		return fmt.Errorf("%s must not contain leading or trailing whitespace", name)
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return fmt.Errorf("%s must not contain control characters", name)
		}
	}
	return nil
}

func validateAdapterArtifactID(value string) error {
	if err := validateAdapterContractValue("resume artifact id", value); err != nil {
		return err
	}
	if value == "." || value == ".." || strings.ContainsAny(value, `/\\`) {
		return fmt.Errorf("resume artifact id must be a safe path segment")
	}
	for _, character := range value {
		if (character >= 'a' && character <= 'z') ||
			(character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') ||
			character == '.' || character == '_' || character == '-' {
			continue
		}
		return fmt.Errorf("resume artifact id contains an unsupported character")
	}
	return nil
}

func pauseAdapterIsNil(adapter PauseAdapter) bool {
	if adapter == nil {
		return true
	}
	value := reflect.ValueOf(adapter)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}

func adapterCheckpointPathInside(candidate string, directory string) bool {
	return path.Dir(candidate) == directory || strings.HasPrefix(candidate, directory+"/")
}
