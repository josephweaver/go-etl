package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"goetl/internal/model"
)

func TestWorkerControllerClientCheckpointMethodsPreserveContractsAndSession(t *testing.T) {
	session := WorkerSession{WorkerID: "worker-001", WorkerSessionID: "session-001"}
	confirmation := testWorkerCheckpointConfirmation(t, "artifact-001", "source-v1")
	suspension := model.WorkCheckpointSuspendLatest{
		AttemptID:     confirmation.AttemptID,
		SuspendedAt:   "2026-08-01T00:20:00Z",
		SuspendReason: model.CheckpointSuspendReasonShutdown,
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if r.Header.Get(workerIDHeader) != session.WorkerID || r.Header.Get(workerSessionIDHeader) != session.WorkerSessionID {
			t.Errorf("session headers = %q/%q", r.Header.Get(workerIDHeader), r.Header.Get(workerSessionIDHeader))
		}

		switch r.URL.Path {
		case "/work/checkpoint/confirm":
			var got model.WorkCheckpointConfirmation
			if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
				t.Errorf("decode confirmation: %v", err)
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}
			if got != confirmation {
				t.Errorf("confirmation changed in transport: %+v", got)
			}
			writeWorkerCheckpointAcknowledgement(t, w, confirmationAcknowledgement(t, confirmation))
		case "/work/checkpoint/suspend-latest":
			var got model.WorkCheckpointSuspendLatest
			if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
				t.Errorf("decode suspension: %v", err)
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}
			if got != suspension {
				t.Errorf("suspension changed in transport: %+v", got)
			}
			acknowledgement := confirmationAcknowledgement(t, confirmation)
			acknowledgement.Operation = model.CheckpointOperationSuspendLatest
			acknowledgement.Disposition = model.CheckpointDispositionSuspend
			acknowledgement.Suspended = true
			acknowledgement.SuspendedAt = suspension.SuspendedAt
			writeWorkerCheckpointAcknowledgement(t, w, acknowledgement)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client, err := newUnauthenticatedWorkerControllerClient(server.URL)
	if err != nil {
		t.Fatalf("new worker controller client: %v", err)
	}

	confirmed, err := client.ConfirmCheckpoint(context.Background(), session, confirmation)
	if err != nil {
		t.Fatalf("ConfirmCheckpoint() error = %v", err)
	}
	if err := confirmed.Validate(); err != nil {
		t.Fatalf("confirmed acknowledgement validation: %v", err)
	}
	if confirmed.ResumeArtifactID != confirmation.Reference.ResumeArtifactID {
		t.Fatalf("confirmed artifact = %q", confirmed.ResumeArtifactID)
	}

	suspended, err := client.SuspendLatestCheckpoint(context.Background(), session, suspension)
	if err != nil {
		t.Fatalf("SuspendLatestCheckpoint() error = %v", err)
	}
	if err := suspended.Validate(); err != nil {
		t.Fatalf("suspended acknowledgement validation: %v", err)
	}
	if suspended.Operation != model.CheckpointOperationSuspendLatest || suspended.SuspendedAt != suspension.SuspendedAt {
		t.Fatalf("suspended acknowledgement = %+v", suspended)
	}
}

func TestWorkerControllerClientCheckpointRejectsInvalidRequestBeforePost(t *testing.T) {
	posts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		posts++
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	client, err := newUnauthenticatedWorkerControllerClient(server.URL)
	if err != nil {
		t.Fatal(err)
	}

	_, err = client.ConfirmCheckpoint(
		context.Background(),
		WorkerSession{WorkerID: "worker-001", WorkerSessionID: "session-001"},
		model.WorkCheckpointConfirmation{},
	)
	if err == nil || !strings.Contains(err.Error(), "validate checkpoint confirmation") {
		t.Fatalf("ConfirmCheckpoint() error = %v", err)
	}
	if posts != 0 {
		t.Fatalf("controller posts = %d, want 0", posts)
	}
}

func TestWorkerControllerClientCheckpointRejectsMismatchedAcknowledgement(t *testing.T) {
	confirmation := testWorkerCheckpointConfirmation(t, "artifact-001", "source-v1")
	other := testWorkerCheckpointConfirmation(t, "artifact-other", "source-v1")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeWorkerCheckpointAcknowledgement(t, w, confirmationAcknowledgement(t, other))
	}))
	defer server.Close()
	client, err := newUnauthenticatedWorkerControllerClient(server.URL)
	if err != nil {
		t.Fatal(err)
	}

	_, err = client.ConfirmCheckpoint(
		context.Background(),
		WorkerSession{WorkerID: "worker-001", WorkerSessionID: "session-001"},
		confirmation,
	)
	if err == nil || !strings.Contains(err.Error(), "does not match request") {
		t.Fatalf("ConfirmCheckpoint() error = %v", err)
	}
}

func TestWorkerControllerClientCheckpointErrorDoesNotExposeManifest(t *testing.T) {
	const sentinel = "checkpoint-manifest-secret-sentinel"
	confirmation := testWorkerCheckpointConfirmation(t, "artifact-001", sentinel)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "checkpoint persistence failure", http.StatusInternalServerError)
	}))
	defer server.Close()
	client, err := newUnauthenticatedWorkerControllerClient(server.URL)
	if err != nil {
		t.Fatal(err)
	}

	_, err = client.ConfirmCheckpoint(
		context.Background(),
		WorkerSession{WorkerID: "worker-001", WorkerSessionID: "session-001"},
		confirmation,
	)
	if err == nil {
		t.Fatal("ConfirmCheckpoint() error = nil")
	}
	if strings.Contains(err.Error(), sentinel) || strings.Contains(err.Error(), confirmation.ManifestJSON) {
		t.Fatalf("ConfirmCheckpoint() exposed manifest data: %v", err)
	}
}

func testWorkerCheckpointConfirmation(t *testing.T, artifactID string, sourceVersion string) model.WorkCheckpointConfirmation {
	t.Helper()
	storagePath := "goetl/resume/" + artifactID
	statePath := "state/checkpoint.json"
	manifest := model.ResumeArtifactManifest{
		Schema:              model.ResumeArtifactSchemaV1,
		ResumeArtifactID:    artifactID,
		ResumeGeneration:    1,
		PauseStrategy:       model.PauseStrategyManual,
		WorkItemID:          "work-001",
		WorkItemType:        model.WorkItemTypeWriteDemoOutput,
		ProducingAttemptID:  "attempt-001",
		ExecutionLineageID:  "lineage-001",
		InputFingerprint:    "input-v1",
		SourceVersion:       sourceVersion,
		CodeVersion:         "code-v1",
		CreatedAt:           "2026-08-01T00:01:00Z",
		StorageScope:        model.ResumeArtifactStorageScopeSharedTmp,
		StorageRelativePath: storagePath,
		RetentionPolicy:     model.ResumeArtifactRetentionWhileReferenced,
		Compatibility: model.ResumeArtifactCompatibility{
			AdapterID:                      "manual-test",
			AdapterVersion:                 "1",
			WorkerExecutionContractVersion: "1",
			WorkerVersion:                  "test",
			ContainerImageIdentity:         "sha256:test",
			OperatingSystem:                "linux",
			Architecture:                   "amd64",
			ContainerRuntime:               "test",
		},
		Files: []model.ResumeArtifactFile{{
			Path:      statePath,
			SizeBytes: 10,
			SHA256:    strings.Repeat("a", 64),
		}},
		Manual: &model.ManualResumePayload{
			HandlerID:      "write-demo-output",
			HandlerVersion: "1",
			StateSchema:    "test/v1",
			StateFilePath:  statePath,
		},
	}
	manifestJSON, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	return model.WorkCheckpointConfirmation{
		AttemptID:    manifest.ProducingAttemptID,
		ManifestJSON: string(manifestJSON),
		Reference: model.ResumeArtifactReference{
			Schema:               model.ResumeArtifactSchemaV1,
			ResumeArtifactID:     artifactID,
			StorageScope:         model.ResumeArtifactStorageScopeSharedTmp,
			ManifestRelativePath: storagePath + "/manifest.json",
			ManifestSHA256:       fmt.Sprintf("%x", sha256.Sum256(manifestJSON)),
		},
		CaptureKind: model.CheckpointCaptureKindPeriodic,
		Disposition: model.CheckpointDispositionContinue,
	}
}

func confirmationAcknowledgement(t *testing.T, confirmation model.WorkCheckpointConfirmation) model.WorkCheckpointAcknowledgement {
	t.Helper()
	var manifest model.ResumeArtifactManifest
	if err := json.Unmarshal([]byte(confirmation.ManifestJSON), &manifest); err != nil {
		t.Fatal(err)
	}
	return model.WorkCheckpointAcknowledgement{
		Operation:          model.CheckpointOperationConfirmation,
		ResumeArtifactID:   manifest.ResumeArtifactID,
		ExecutionLineageID: manifest.ExecutionLineageID,
		ResumeGeneration:   manifest.ResumeGeneration,
		Reference:          confirmation.Reference,
		CaptureKind:        confirmation.CaptureKind,
		AcceptedAt:         "2026-08-01T00:02:00Z",
		Disposition:        confirmation.Disposition,
	}
}

func writeWorkerCheckpointAcknowledgement(t *testing.T, w http.ResponseWriter, acknowledgement model.WorkCheckpointAcknowledgement) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(acknowledgement); err != nil {
		t.Errorf("encode acknowledgement: %v", err)
	}
}
