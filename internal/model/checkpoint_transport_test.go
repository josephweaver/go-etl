package model

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestWorkCheckpointConfirmationValidationAndRoundTrip(t *testing.T) {
	tests := []struct {
		name        string
		capture     CheckpointCaptureKind
		disposition CheckpointDisposition
		suspendedAt string
	}{
		{name: "periodic continue", capture: CheckpointCaptureKindPeriodic, disposition: CheckpointDispositionContinue},
		{name: "quantum suspend", capture: CheckpointCaptureKindQuantum, disposition: CheckpointDispositionSuspend, suspendedAt: "2026-08-01T10:05:00Z"},
		{name: "final suspend", capture: CheckpointCaptureKindFinal, disposition: CheckpointDispositionSuspend, suspendedAt: "2026-08-01T10:05:00Z"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			manifestJSON, reference := testCheckpointTransportArtifact(t)
			confirmation := WorkCheckpointConfirmation{
				AttemptID:    "attempt-001",
				ManifestJSON: manifestJSON,
				Reference:    reference,
				CaptureKind:  test.capture,
				Disposition:  test.disposition,
				SuspendedAt:  test.suspendedAt,
			}
			if err := confirmation.Validate(); err != nil {
				t.Fatalf("Validate() error = %v", err)
			}

			data, err := json.Marshal(confirmation)
			if err != nil {
				t.Fatalf("Marshal() error = %v", err)
			}
			var decoded WorkCheckpointConfirmation
			if err := json.Unmarshal(data, &decoded); err != nil {
				t.Fatalf("Unmarshal() error = %v", err)
			}
			if !reflect.DeepEqual(decoded, confirmation) {
				t.Fatalf("round trip mismatch: got %#v, want %#v", decoded, confirmation)
			}
			if decoded.ManifestJSON != manifestJSON {
				t.Fatal("round trip changed exact manifest_json")
			}
		})
	}
}

func TestWorkCheckpointConfirmationRejectsInvalidState(t *testing.T) {
	manifestJSON, reference := testCheckpointTransportArtifact(t)
	valid := WorkCheckpointConfirmation{
		AttemptID:    "attempt-001",
		ManifestJSON: manifestJSON,
		Reference:    reference,
		CaptureKind:  CheckpointCaptureKindPeriodic,
		Disposition:  CheckpointDispositionContinue,
	}
	tests := []struct {
		name    string
		mutate  func(*WorkCheckpointConfirmation)
		wantErr string
	}{
		{name: "missing attempt", mutate: func(value *WorkCheckpointConfirmation) { value.AttemptID = "" }, wantErr: "attempt_id is required"},
		{name: "unsupported pair", mutate: func(value *WorkCheckpointConfirmation) { value.Disposition = CheckpointDispositionSuspend }, wantErr: "unsupported checkpoint capture/disposition"},
		{name: "continue with suspension time", mutate: func(value *WorkCheckpointConfirmation) { value.SuspendedAt = "2026-08-01T10:05:00Z" }, wantErr: "must not include suspended_at"},
		{name: "suspend missing time", mutate: func(value *WorkCheckpointConfirmation) {
			value.CaptureKind = CheckpointCaptureKindFinal
			value.Disposition = CheckpointDispositionSuspend
		}, wantErr: "suspended_at is required"},
		{name: "bad suspend time", mutate: func(value *WorkCheckpointConfirmation) {
			value.CaptureKind = CheckpointCaptureKindFinal
			value.Disposition = CheckpointDispositionSuspend
			value.SuspendedAt = "today"
		}, wantErr: "must be RFC 3339"},
		{name: "invalid manifest json", mutate: func(value *WorkCheckpointConfirmation) { value.ManifestJSON = "{" }, wantErr: "manifest_json is invalid"},
		{name: "wrong exact digest", mutate: func(value *WorkCheckpointConfirmation) { value.Reference.ManifestSHA256 = strings.Repeat("f", 64) }, wantErr: "does not match exact manifest_json"},
		{name: "wrong artifact id", mutate: func(value *WorkCheckpointConfirmation) { value.Reference.ResumeArtifactID = "resume-002" }, wantErr: "resume_artifact_id do not match"},
		{name: "manifest outside directory", mutate: func(value *WorkCheckpointConfirmation) {
			value.Reference.ManifestRelativePath = "goetl/resume/other/manifest.json"
		}, wantErr: "outside artifact storage directory"},
		{name: "wrong producing attempt", mutate: func(value *WorkCheckpointConfirmation) { value.AttemptID = "attempt-002" }, wantErr: "producing_attempt_id must match"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate := valid
			test.mutate(&candidate)
			assertCheckpointTransportError(t, candidate.Validate(), test.wantErr)
		})
	}
}

func TestWorkCheckpointSuspendLatestValidation(t *testing.T) {
	valid := WorkCheckpointSuspendLatest{
		AttemptID:     "attempt-001",
		SuspendedAt:   "2026-08-01T10:05:00Z",
		SuspendReason: CheckpointSuspendReasonShutdown,
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}

	for _, test := range []struct {
		name    string
		mutate  func(*WorkCheckpointSuspendLatest)
		wantErr string
	}{
		{name: "missing attempt", mutate: func(value *WorkCheckpointSuspendLatest) { value.AttemptID = "" }, wantErr: "attempt_id is required"},
		{name: "invalid time", mutate: func(value *WorkCheckpointSuspendLatest) { value.SuspendedAt = "2026-08-01" }, wantErr: "must be RFC 3339"},
		{name: "unknown reason", mutate: func(value *WorkCheckpointSuspendLatest) { value.SuspendReason = "future" }, wantErr: "unsupported suspend_reason"},
	} {
		t.Run(test.name, func(t *testing.T) {
			candidate := valid
			test.mutate(&candidate)
			assertCheckpointTransportError(t, candidate.Validate(), test.wantErr)
		})
	}
}

func TestWorkCheckpointAcknowledgementValidation(t *testing.T) {
	_, reference := testCheckpointTransportArtifact(t)
	confirmation := WorkCheckpointAcknowledgement{
		Operation:          CheckpointOperationConfirmation,
		ResumeArtifactID:   "resume-001",
		ExecutionLineageID: "lineage-001",
		ResumeGeneration:   1,
		Reference:          reference,
		CaptureKind:        CheckpointCaptureKindPeriodic,
		AcceptedAt:         "2026-08-01T10:04:00Z",
		Disposition:        CheckpointDispositionContinue,
	}
	if err := confirmation.Validate(); err != nil {
		t.Fatalf("periodic Validate() error = %v", err)
	}

	fallback := confirmation
	fallback.Operation = CheckpointOperationSuspendLatest
	fallback.Disposition = CheckpointDispositionSuspend
	fallback.Suspended = true
	fallback.SuspendedAt = "2026-08-01T10:05:00Z"
	if err := fallback.Validate(); err != nil {
		t.Fatalf("fallback Validate() error = %v", err)
	}

	for _, test := range []struct {
		name    string
		mutate  func(*WorkCheckpointAcknowledgement)
		wantErr string
	}{
		{name: "unknown operation", mutate: func(value *WorkCheckpointAcknowledgement) { value.Operation = "future" }, wantErr: "unsupported checkpoint operation"},
		{name: "reference mismatch", mutate: func(value *WorkCheckpointAcknowledgement) { value.Reference.ResumeArtifactID = "resume-002" }, wantErr: "must match acknowledgement"},
		{name: "generation zero", mutate: func(value *WorkCheckpointAcknowledgement) { value.ResumeGeneration = 0 }, wantErr: "resume_generation"},
		{name: "bad accepted time", mutate: func(value *WorkCheckpointAcknowledgement) { value.AcceptedAt = "today" }, wantErr: "accepted_at must be RFC 3339"},
		{name: "confirmation suspend flag mismatch", mutate: func(value *WorkCheckpointAcknowledgement) {
			value.Suspended = true
			value.SuspendedAt = "2026-08-01T10:05:00Z"
		}, wantErr: "must match confirmation disposition"},
	} {
		t.Run(test.name, func(t *testing.T) {
			candidate := confirmation
			test.mutate(&candidate)
			assertCheckpointTransportError(t, candidate.Validate(), test.wantErr)
		})
	}
}

func TestWorkItemResumeAssignmentValidation(t *testing.T) {
	manifestJSON, reference := testCheckpointTransportArtifact(t)
	assignment := WorkItemResumeAssignment{
		Schema:               WorkItemResumeAssignmentSchemaV1,
		ResumedFromAttemptID: "attempt-001",
		ExecutionLineageID:   "lineage-001",
		ResumeAttemptNumber:  1,
		ManifestJSON:         manifestJSON,
		Reference:            reference,
	}
	if err := assignment.Validate("work-001", WorkItemTypePythonScript); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}

	for _, test := range []struct {
		name     string
		mutate   func(*WorkItemResumeAssignment)
		workID   string
		workType WorkItemType
		wantErr  string
	}{
		{name: "unknown schema", mutate: func(value *WorkItemResumeAssignment) { value.Schema = "goet/work-item-resume-assignment/v2" }, workID: "work-001", workType: WorkItemTypePythonScript, wantErr: "unsupported work-item resume assignment schema"},
		{name: "missing predecessor", mutate: func(value *WorkItemResumeAssignment) { value.ResumedFromAttemptID = "" }, workID: "work-001", workType: WorkItemTypePythonScript, wantErr: "resumed_from_attempt_id is required"},
		{name: "attempt number zero", mutate: func(value *WorkItemResumeAssignment) { value.ResumeAttemptNumber = 0 }, workID: "work-001", workType: WorkItemTypePythonScript, wantErr: "resume_attempt_number"},
		{name: "wrong work item", workID: "work-002", workType: WorkItemTypePythonScript, wantErr: "work_item_id must match"},
		{name: "wrong work type", workID: "work-001", workType: WorkItemTypeAssetMaterialize, wantErr: "work_item_type must match"},
		{name: "wrong predecessor", mutate: func(value *WorkItemResumeAssignment) { value.ResumedFromAttemptID = "attempt-002" }, workID: "work-001", workType: WorkItemTypePythonScript, wantErr: "producing_attempt_id must match"},
		{name: "wrong lineage", mutate: func(value *WorkItemResumeAssignment) { value.ExecutionLineageID = "lineage-002" }, workID: "work-001", workType: WorkItemTypePythonScript, wantErr: "execution_lineage_id must match"},
	} {
		t.Run(test.name, func(t *testing.T) {
			candidate := assignment
			if test.mutate != nil {
				test.mutate(&candidate)
			}
			assertCheckpointTransportError(t, candidate.Validate(test.workID, test.workType), test.wantErr)
		})
	}
}

func testCheckpointTransportArtifact(t *testing.T) (string, ResumeArtifactReference) {
	t.Helper()
	manifest := testResumeArtifactManifest(PauseStrategyDMTCP)
	manifestJSON, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("Marshal(manifest) error = %v", err)
	}
	digest := sha256.Sum256(manifestJSON)
	return string(manifestJSON), ResumeArtifactReference{
		Schema:               ResumeArtifactSchemaV1,
		ResumeArtifactID:     manifest.ResumeArtifactID,
		StorageScope:         manifest.StorageScope,
		ManifestRelativePath: manifest.StorageRelativePath + "/manifest.json",
		ManifestSHA256:       fmt.Sprintf("%x", digest),
	}
}

func assertCheckpointTransportError(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil {
		t.Fatalf("Validate() error = nil, want containing %q", want)
	}
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("Validate() error = %q, want containing %q", err, want)
	}
}
