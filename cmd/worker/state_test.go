package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"goetl/internal/model"
)

func TestReportWorkFailed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/work/fail" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}

		var failure model.WorkFailure
		if err := json.NewDecoder(r.Body).Decode(&failure); err != nil {
			t.Fatalf("decode failure: %v", err)
		}

		if failure.ID != "test-001" || failure.Error != "failed" {
			t.Fatalf("unexpected failure: %+v", failure)
		}

		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	if err := reportWorkFailed(server.URL, "test-001", errors.New("failed")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestReportWorkComplete(t *testing.T) {
	startedAt := time.Now().UTC().Add(-time.Minute)
	item := model.WorkItem{
		ID:                  "test-001",
		Type:                model.WorkItemTypeWriteDemoOutput,
		OutputFilename:      "result.txt",
		WorkflowInstanceID:  "workflow-instance-001",
		StepInstanceID:      "step-instance-001",
		WorkItemFingerprint: "work-item-fingerprint",
		InputFingerprint:    "input-fingerprint",
		OutputFingerprint:   "output-fingerprint",
		CodeVersion:         "code-version",
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/work/complete" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}

		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method: %s", r.Method)
		}

		var completion model.WorkCompletion
		if err := json.NewDecoder(r.Body).Decode(&completion); err != nil {
			t.Fatalf("decode completion: %v", err)
		}

		if completion.ID != "test-001" {
			t.Fatalf("unexpected id: %q", completion.ID)
		}

		if !strings.HasPrefix(completion.AttemptID, "test-001-attempt-") {
			t.Fatalf("unexpected attempt id: %q", completion.AttemptID)
		}

		if completion.WorkflowInstanceID != item.WorkflowInstanceID {
			t.Fatalf("unexpected workflow instance id: %q", completion.WorkflowInstanceID)
		}

		if completion.StepInstanceID != item.StepInstanceID {
			t.Fatalf("unexpected step instance id: %q", completion.StepInstanceID)
		}

		if completion.WorkItemFingerprint != item.WorkItemFingerprint {
			t.Fatalf("unexpected work item fingerprint: %q", completion.WorkItemFingerprint)
		}

		if completion.CodeVersion != item.CodeVersion {
			t.Fatalf("unexpected code version: %q", completion.CodeVersion)
		}

		if completion.StartedAt != startedAt.Format(time.RFC3339) {
			t.Fatalf("started_at = %q, want %q", completion.StartedAt, startedAt.Format(time.RFC3339))
		}

		completedAt, err := time.Parse(time.RFC3339, completion.CompletedAt)
		if err != nil {
			t.Fatalf("parse completed_at: %v", err)
		}

		if completedAt.Before(startedAt) {
			t.Fatalf("completed_at = %q before started_at = %q", completion.CompletedAt, completion.StartedAt)
		}

		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	if err := reportWorkComplete(server.URL, item, startedAt); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestReportWorkCompleteRejectsServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	if err := reportWorkComplete(server.URL, model.WorkItem{ID: "test-001"}, time.Now()); err == nil {
		t.Fatal("expected an error")
	}
}

func TestFetchWorkItem(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/work/next" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}

		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"id": "test-001",
			"type": "write_demo_output",
			"output_filename": "result.txt"
		}`))
	}))
	defer server.Close()

	item, hasWork, err := fetchWorkItem(server.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !hasWork {
		t.Fatal("expected work item")
	}

	if item.ID != "test-001" {
		t.Fatalf("unexpected id: %q", item.ID)
	}

	if item.Type != model.WorkItemTypeWriteDemoOutput {
		t.Fatalf("unexpected type: %q", item.Type)
	}

	if item.OutputFilename != "result.txt" {
		t.Fatalf("unexpected output filename: %q", item.OutputFilename)
	}
}

func TestFetchWorkItemRejectsServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	if _, _, err := fetchWorkItem(server.URL); err == nil {
		t.Fatal("expected an error")
	}
}

func TestFetchWorkItemRejectsMalformedJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"id":`))
	}))
	defer server.Close()

	if _, _, err := fetchWorkItem(server.URL); err == nil {
		t.Fatal("expected an error")
	}
}

func TestFetchWorkItemRejectsInvalidItem(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{
			"id": "test-001",
			"type": "write_demo_output",
			"output_filename": "../outside.txt"
		}`))
	}))
	defer server.Close()

	if _, _, err := fetchWorkItem(server.URL); err == nil {
		t.Fatal("expected an error")
	}
}

func TestFetchWorkItemReturnsNoWork(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	item, hasWork, err := fetchWorkItem(server.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if hasWork {
		t.Fatalf("unexpected work item: %+v", item)
	}
}
