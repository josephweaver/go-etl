package model

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path"
	"strings"
	"time"
)

const WorkItemResumeAssignmentSchemaV1 = "goet/work-item-resume-assignment/v1"

type CheckpointCaptureKind string

const (
	CheckpointCaptureKindPeriodic CheckpointCaptureKind = "periodic"
	CheckpointCaptureKindQuantum  CheckpointCaptureKind = "quantum"
	CheckpointCaptureKindFinal    CheckpointCaptureKind = "final"
)

type CheckpointDisposition string

const (
	CheckpointDispositionContinue CheckpointDisposition = "continue"
	CheckpointDispositionSuspend  CheckpointDisposition = "suspend"
)

type CheckpointSuspendReason string

const (
	CheckpointSuspendReasonQuantum  CheckpointSuspendReason = "quantum"
	CheckpointSuspendReasonShutdown CheckpointSuspendReason = "shutdown"
)

type CheckpointOperation string

const (
	CheckpointOperationConfirmation  CheckpointOperation = "checkpoint_confirmation"
	CheckpointOperationSuspendLatest CheckpointOperation = "suspend_latest"
)

type WorkCheckpointConfirmation struct {
	AttemptID    string                  `json:"attempt_id"`
	ManifestJSON string                  `json:"manifest_json"`
	Reference    ResumeArtifactReference `json:"reference"`
	CaptureKind  CheckpointCaptureKind   `json:"capture_kind"`
	Disposition  CheckpointDisposition   `json:"disposition"`
	SuspendedAt  string                  `json:"suspended_at,omitempty"`
}

type WorkCheckpointSuspendLatest struct {
	AttemptID     string                  `json:"attempt_id"`
	SuspendedAt   string                  `json:"suspended_at"`
	SuspendReason CheckpointSuspendReason `json:"suspend_reason"`
}

type WorkCheckpointAcknowledgement struct {
	Operation          CheckpointOperation     `json:"operation"`
	ResumeArtifactID   string                  `json:"resume_artifact_id"`
	ExecutionLineageID string                  `json:"execution_lineage_id"`
	ResumeGeneration   int                     `json:"resume_generation"`
	Reference          ResumeArtifactReference `json:"reference"`
	CaptureKind        CheckpointCaptureKind   `json:"capture_kind"`
	AcceptedAt         string                  `json:"accepted_at"`
	Disposition        CheckpointDisposition   `json:"disposition"`
	Suspended          bool                    `json:"suspended"`
	SuspendedAt        string                  `json:"suspended_at,omitempty"`
}

type WorkItemResumeAssignment struct {
	Schema               string                  `json:"schema"`
	ResumedFromAttemptID string                  `json:"resumed_from_attempt_id"`
	ExecutionLineageID   string                  `json:"execution_lineage_id"`
	ResumeAttemptNumber  int                     `json:"resume_attempt_number"`
	ManifestJSON         string                  `json:"manifest_json"`
	Reference            ResumeArtifactReference `json:"reference"`
}

func (confirmation WorkCheckpointConfirmation) Validate() error {
	if err := validateRequiredResumeValue("attempt_id", confirmation.AttemptID); err != nil {
		return err
	}
	if err := validateCheckpointCaptureDisposition(confirmation.CaptureKind, confirmation.Disposition); err != nil {
		return err
	}
	if confirmation.Disposition == CheckpointDispositionSuspend {
		if err := validateCheckpointTimestamp("suspended_at", confirmation.SuspendedAt); err != nil {
			return err
		}
	} else if confirmation.SuspendedAt != "" {
		return fmt.Errorf("continue checkpoint must not include suspended_at")
	}

	manifest, err := validateCheckpointManifestJSON(confirmation.ManifestJSON, confirmation.Reference)
	if err != nil {
		return err
	}
	if manifest.ProducingAttemptID != confirmation.AttemptID {
		return fmt.Errorf("manifest producing_attempt_id must match attempt_id")
	}
	return nil
}

func (request WorkCheckpointSuspendLatest) Validate() error {
	if err := validateRequiredResumeValue("attempt_id", request.AttemptID); err != nil {
		return err
	}
	if err := validateCheckpointTimestamp("suspended_at", request.SuspendedAt); err != nil {
		return err
	}
	switch request.SuspendReason {
	case CheckpointSuspendReasonQuantum, CheckpointSuspendReasonShutdown:
		return nil
	default:
		return fmt.Errorf("unsupported suspend_reason %q", request.SuspendReason)
	}
}

func (acknowledgement WorkCheckpointAcknowledgement) Validate() error {
	if err := validateResumeArtifactID(acknowledgement.ResumeArtifactID); err != nil {
		return err
	}
	if err := validateRequiredResumeValue("execution_lineage_id", acknowledgement.ExecutionLineageID); err != nil {
		return err
	}
	if acknowledgement.ResumeGeneration < 1 {
		return fmt.Errorf("resume_generation must be at least 1")
	}
	if err := acknowledgement.Reference.Validate(); err != nil {
		return fmt.Errorf("reference: %w", err)
	}
	if acknowledgement.Reference.ResumeArtifactID != acknowledgement.ResumeArtifactID {
		return fmt.Errorf("reference resume_artifact_id must match acknowledgement")
	}
	if err := validateCheckpointCaptureKind(acknowledgement.CaptureKind); err != nil {
		return err
	}
	if err := validateCheckpointTimestamp("accepted_at", acknowledgement.AcceptedAt); err != nil {
		return err
	}

	switch acknowledgement.Operation {
	case CheckpointOperationConfirmation:
		if err := validateCheckpointCaptureDisposition(acknowledgement.CaptureKind, acknowledgement.Disposition); err != nil {
			return err
		}
		wantSuspended := acknowledgement.Disposition == CheckpointDispositionSuspend
		if acknowledgement.Suspended != wantSuspended {
			return fmt.Errorf("suspended must match confirmation disposition")
		}
	case CheckpointOperationSuspendLatest:
		if acknowledgement.Disposition != CheckpointDispositionSuspend {
			return fmt.Errorf("suspend_latest acknowledgement requires suspend disposition")
		}
		if !acknowledgement.Suspended {
			return fmt.Errorf("suspend_latest acknowledgement must be suspended")
		}
	default:
		return fmt.Errorf("unsupported checkpoint operation %q", acknowledgement.Operation)
	}

	if acknowledgement.Suspended {
		if err := validateCheckpointTimestamp("suspended_at", acknowledgement.SuspendedAt); err != nil {
			return err
		}
	} else if acknowledgement.SuspendedAt != "" {
		return fmt.Errorf("unsuspended acknowledgement must not include suspended_at")
	}
	return nil
}

func (assignment WorkItemResumeAssignment) Validate(workItemID string, workItemType WorkItemType) error {
	if assignment.Schema != WorkItemResumeAssignmentSchemaV1 {
		return fmt.Errorf("unsupported work-item resume assignment schema %q", assignment.Schema)
	}
	if err := validateRequiredResumeValue("resumed_from_attempt_id", assignment.ResumedFromAttemptID); err != nil {
		return err
	}
	if err := validateRequiredResumeValue("execution_lineage_id", assignment.ExecutionLineageID); err != nil {
		return err
	}
	if assignment.ResumeAttemptNumber < 1 {
		return fmt.Errorf("resume_attempt_number must be at least 1")
	}
	if err := validateRequiredResumeValue("work_item_id", workItemID); err != nil {
		return err
	}
	if err := validateRequiredResumeValue("work_item_type", string(workItemType)); err != nil {
		return err
	}

	manifest, err := validateCheckpointManifestJSON(assignment.ManifestJSON, assignment.Reference)
	if err != nil {
		return err
	}
	if manifest.WorkItemID != workItemID {
		return fmt.Errorf("manifest work_item_id must match assigned work item")
	}
	if manifest.WorkItemType != workItemType {
		return fmt.Errorf("manifest work_item_type must match assigned work item")
	}
	if manifest.ProducingAttemptID != assignment.ResumedFromAttemptID {
		return fmt.Errorf("manifest producing_attempt_id must match resumed_from_attempt_id")
	}
	if manifest.ExecutionLineageID != assignment.ExecutionLineageID {
		return fmt.Errorf("manifest execution_lineage_id must match resume assignment")
	}
	return nil
}

func validateCheckpointManifestJSON(manifestJSON string, reference ResumeArtifactReference) (ResumeArtifactManifest, error) {
	if manifestJSON == "" {
		return ResumeArtifactManifest{}, fmt.Errorf("manifest_json is required")
	}
	if err := reference.Validate(); err != nil {
		return ResumeArtifactManifest{}, fmt.Errorf("reference: %w", err)
	}

	var manifest ResumeArtifactManifest
	if err := json.Unmarshal([]byte(manifestJSON), &manifest); err != nil {
		return ResumeArtifactManifest{}, fmt.Errorf("manifest_json is invalid")
	}
	if err := manifest.Validate(); err != nil {
		return ResumeArtifactManifest{}, fmt.Errorf("manifest_json: %w", err)
	}
	if manifest.ResumeArtifactID != reference.ResumeArtifactID {
		return ResumeArtifactManifest{}, fmt.Errorf("manifest and reference resume_artifact_id do not match")
	}
	if manifest.StorageScope != reference.StorageScope {
		return ResumeArtifactManifest{}, fmt.Errorf("manifest and reference storage_scope do not match")
	}
	if !checkpointPathInside(reference.ManifestRelativePath, manifest.StorageRelativePath) {
		return ResumeArtifactManifest{}, fmt.Errorf("reference manifest path is outside artifact storage directory")
	}

	digest := sha256.Sum256([]byte(manifestJSON))
	if hex.EncodeToString(digest[:]) != reference.ManifestSHA256 {
		return ResumeArtifactManifest{}, fmt.Errorf("reference manifest_sha256 does not match exact manifest_json")
	}
	return manifest, nil
}

func validateCheckpointCaptureDisposition(capture CheckpointCaptureKind, disposition CheckpointDisposition) error {
	switch {
	case capture == CheckpointCaptureKindPeriodic && disposition == CheckpointDispositionContinue:
		return nil
	case capture == CheckpointCaptureKindQuantum && disposition == CheckpointDispositionSuspend:
		return nil
	case capture == CheckpointCaptureKindFinal && disposition == CheckpointDispositionSuspend:
		return nil
	default:
		return fmt.Errorf("unsupported checkpoint capture/disposition combination %q/%q", capture, disposition)
	}
}

func validateCheckpointCaptureKind(capture CheckpointCaptureKind) error {
	switch capture {
	case CheckpointCaptureKindPeriodic, CheckpointCaptureKindQuantum, CheckpointCaptureKindFinal:
		return nil
	default:
		return fmt.Errorf("unsupported checkpoint capture_kind %q", capture)
	}
}

func validateCheckpointTimestamp(field, value string) error {
	if err := validateRequiredResumeValue(field, value); err != nil {
		return err
	}
	if _, err := time.Parse(time.RFC3339, value); err != nil {
		return fmt.Errorf("%s must be RFC 3339: %w", field, err)
	}
	return nil
}

func checkpointPathInside(candidate, directory string) bool {
	return path.Dir(candidate) == directory || strings.HasPrefix(candidate, directory+"/")
}
