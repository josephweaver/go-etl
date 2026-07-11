package main

import (
	"crypto/tls"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"goetl/internal/controllerhttp"
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

		if failure.ID != "test-001" || failure.AttemptID != "attempt-001" || failure.Error != "failed" {
			t.Fatalf("unexpected failure: %+v", failure)
		}
		if _, err := time.Parse(time.RFC3339, failure.FailedAt); err != nil {
			t.Fatalf("failed_at is not RFC3339: %q", failure.FailedAt)
		}

		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	item := model.WorkItem{ID: "test-001", AttemptID: "attempt-001"}
	if err := reportWorkFailed(server.URL, item, errors.New("failed")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestReportWorkComplete(t *testing.T) {
	startedAt := time.Now().UTC().Add(-time.Minute)
	item := model.WorkItem{
		ID:                   "test-001",
		AttemptID:            "attempt-001",
		Type:                 model.WorkItemTypeWriteDemoOutput,
		OutputFilename:       "result.txt",
		WorkflowDefinitionID: "workflow-definition-001",
		WorkflowFingerprint:  "workflow-fingerprint",
		WorkflowInstanceID:   "workflow-instance-001",
		StepDefinitionID:     "step-definition-001",
		StepFingerprint:      "step-fingerprint",
		StepInstanceID:       "step-instance-001",
		WorkItemFingerprint:  "work-item-fingerprint",
		InputFingerprint:     "input-fingerprint",
		OutputFingerprint:    "output-fingerprint",
		CodeVersion:          "code-version",
		Parameters: model.Parameters{
			"input_path": {Type: "path", Value: "demo-summary-input.txt"},
		},
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

		if completion.AttemptID != item.AttemptID {
			t.Fatalf("unexpected attempt id: %q", completion.AttemptID)
		}

		if completion.WorkflowInstanceID != item.WorkflowInstanceID {
			t.Fatalf("unexpected workflow instance id: %q", completion.WorkflowInstanceID)
		}

		if completion.WorkflowDefinitionID != item.WorkflowDefinitionID {
			t.Fatalf("unexpected workflow definition id: %q", completion.WorkflowDefinitionID)
		}

		if completion.WorkflowFingerprint != item.WorkflowFingerprint {
			t.Fatalf("unexpected workflow fingerprint: %q", completion.WorkflowFingerprint)
		}

		if completion.StepDefinitionID != item.StepDefinitionID {
			t.Fatalf("unexpected step definition id: %q", completion.StepDefinitionID)
		}

		if completion.StepFingerprint != item.StepFingerprint {
			t.Fatalf("unexpected step fingerprint: %q", completion.StepFingerprint)
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

		if completion.Parameters["input_path"].Value != "demo-summary-input.txt" {
			t.Fatalf("unexpected input_path parameter: %+v", completion.Parameters["input_path"])
		}

		if completion.OutputJSON == "" || completion.PreStateJSON == "" || completion.PostStateJSON == "" {
			t.Fatalf("completion evidence is incomplete: %+v", completion)
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

	evidence := WorkEvidence{
		OutputJSON:    `{"work_item_id":"test-001"}`,
		PreStateJSON:  `{"output_exists":false}`,
		PostStateJSON: `{"output_exists":true}`,
	}
	if err := reportWorkComplete(server.URL, item, startedAt, evidence); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestReportWorkCompleteRejectsServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	if err := reportWorkComplete(server.URL, model.WorkItem{ID: "test-001"}, time.Now(), WorkEvidence{}); err == nil {
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

func TestWorkerControllerClientUsesTokenFileForWorkRequests(t *testing.T) {
	const sentinel = "goetl-worker-controller-token-sentinel-006"
	tokenFile := filepath.Join(t.TempDir(), "controller-worker-token")
	if err := os.WriteFile(tokenFile, []byte(sentinel+"\n"), 0o600); err != nil {
		t.Fatalf("write token file: %v", err)
	}

	var sawClaimAuth bool
	var sawCompleteAuth bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/work/next":
			sawClaimAuth = r.Header.Get("Authorization") == "Bearer "+sentinel
			_ = json.NewEncoder(w).Encode(model.WorkItem{
				ID:             "test-001",
				Type:           model.WorkItemTypeWriteDemoOutput,
				OutputFilename: "result.txt",
			})
		case r.Method == http.MethodPost && r.URL.Path == "/work/complete":
			sawCompleteAuth = r.Header.Get("Authorization") == "Bearer "+sentinel
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client, err := NewWorkerControllerClient(Config{
		ControllerURL:       server.URL,
		ControllerTokenFile: tokenFile,
	})
	if err != nil {
		t.Fatalf("NewWorkerControllerClient() error = %v", err)
	}
	if err := os.Remove(tokenFile); err != nil {
		t.Fatalf("remove token file after client creation: %v", err)
	}

	item, hasWork, err := client.FetchWorkItem()
	if err != nil {
		t.Fatalf("FetchWorkItem() error = %v", err)
	}
	if !hasWork {
		t.Fatal("expected work item")
	}
	if err := client.ReportWorkComplete(item, time.Now().UTC(), WorkEvidence{}); err != nil {
		t.Fatalf("ReportWorkComplete() error = %v", err)
	}
	if !sawClaimAuth || !sawCompleteAuth {
		t.Fatalf("authorization headers claim=%v complete=%v, want both true", sawClaimAuth, sawCompleteAuth)
	}
}

func TestWorkerControllerClientSendsSessionHeaders(t *testing.T) {
	session := testWorkerSession()
	seen := map[string]bool{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get(workerIDHeader) != session.WorkerID {
			t.Fatalf("%s = %q, want %q", workerIDHeader, r.Header.Get(workerIDHeader), session.WorkerID)
		}
		if r.Header.Get(workerSessionIDHeader) != session.WorkerSessionID {
			t.Fatalf("%s = %q, want %q", workerSessionIDHeader, r.Header.Get(workerSessionIDHeader), session.WorkerSessionID)
		}
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/work/next":
			seen["claim"] = true
			_ = json.NewEncoder(w).Encode(model.WorkItem{
				ID:             "test-001",
				Type:           model.WorkItemTypeWriteDemoOutput,
				OutputFilename: "result.txt",
			})
		case r.Method == http.MethodPost && r.URL.Path == "/work/complete":
			seen["complete"] = true
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodPost && r.URL.Path == "/work/fail":
			seen["fail"] = true
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client, err := newUnauthenticatedWorkerControllerClient(server.URL)
	if err != nil {
		t.Fatalf("newUnauthenticatedWorkerControllerClient() error = %v", err)
	}
	item, hasWork, err := client.FetchWorkItem(session)
	if err != nil {
		t.Fatalf("FetchWorkItem() error = %v", err)
	}
	if !hasWork {
		t.Fatal("expected work item")
	}
	if err := client.ReportWorkComplete(item, time.Now().UTC(), WorkEvidence{}, session); err != nil {
		t.Fatalf("ReportWorkComplete() error = %v", err)
	}
	if err := client.ReportWorkFailed(model.WorkItem{ID: "test-002", AttemptID: "attempt-002"}, errors.New("failed"), session); err != nil {
		t.Fatalf("ReportWorkFailed() error = %v", err)
	}
	for _, name := range []string{"claim", "complete", "fail"} {
		if !seen[name] {
			t.Fatalf("request %q was not observed", name)
		}
	}
}

func TestWorkerControllerClientRejectsMissingExternalTokenFile(t *testing.T) {
	_, err := NewWorkerControllerClient(Config{ControllerURL: "https://controller.example.org"})
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "controller token file is required") {
		t.Fatalf("error = %v, want missing token file", err)
	}
}

func TestWorkerControllerClientRejectsExternalHTTPByDefault(t *testing.T) {
	tokenFile := filepath.Join(t.TempDir(), "controller-worker-token")
	if err := os.WriteFile(tokenFile, []byte("worker-token"), 0o600); err != nil {
		t.Fatalf("write token file: %v", err)
	}

	_, err := NewWorkerControllerClient(Config{
		ControllerURL:       "http://dev-node.example.test:39281",
		ControllerTokenFile: tokenFile,
	})
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "plain HTTP with a non-loopback host") {
		t.Fatalf("error = %v, want external HTTP rejection", err)
	}
}

func TestWorkerControllerClientAllowsExternalHTTPWhenConfigured(t *testing.T) {
	tokenFile := filepath.Join(t.TempDir(), "controller-worker-token")
	if err := os.WriteFile(tokenFile, []byte("worker-token"), 0o600); err != nil {
		t.Fatalf("write token file: %v", err)
	}

	client, err := NewWorkerControllerClient(Config{
		ControllerURL:                         "http://dev-node.example.test:39281",
		ControllerTokenFile:                   tokenFile,
		ControllerInsecureExternalHTTPAllowed: true,
	})
	if err != nil {
		t.Fatalf("NewWorkerControllerClient() error = %v", err)
	}
	if !client.Initialized() {
		t.Fatal("expected initialized client")
	}
}

func TestWorkerControllerClientErrorsDoNotExposeToken(t *testing.T) {
	const sentinel = "goetl-worker-controller-token-sentinel-006"
	tokenFile := filepath.Join(t.TempDir(), "controller-worker-token")
	if err := os.WriteFile(tokenFile, []byte(sentinel), 0o600); err != nil {
		t.Fatalf("write token file: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "forbidden", http.StatusForbidden)
	}))
	defer server.Close()

	client, err := NewWorkerControllerClient(Config{
		ControllerURL:       server.URL,
		ControllerTokenFile: tokenFile,
	})
	if err != nil {
		t.Fatalf("NewWorkerControllerClient() error = %v", err)
	}

	_, _, err = client.FetchWorkItem()
	if err == nil {
		t.Fatal("expected an error")
	}
	if strings.Contains(err.Error(), sentinel) {
		t.Fatalf("error leaked token sentinel: %v", err)
	}
}

func TestWorkerControllerClientSupportsHTTPS(t *testing.T) {
	const sentinel = "goetl-worker-controller-token-sentinel-006"
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+sentinel {
			t.Fatalf("authorization = %q", r.Header.Get("Authorization"))
		}
		_ = json.NewEncoder(w).Encode(model.WorkItem{
			ID:             "test-001",
			Type:           model.WorkItemTypeWriteDemoOutput,
			OutputFilename: "result.txt",
		})
	}))
	defer server.Close()

	token, err := controllerhttp.NewSensitiveToken(sentinel)
	if err != nil {
		t.Fatalf("NewSensitiveToken() error = %v", err)
	}
	baseClient, err := controllerhttp.New(controllerhttp.Config{
		BaseURL: server.URL,
		HTTP: &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
		}}},
		Token:  controllerhttp.NewStaticTokenProvider(token),
		Caller: "goetl-worker/1",
	})
	if err != nil {
		t.Fatalf("controllerhttp.New() error = %v", err)
	}

	client := WorkerControllerClient{client: baseClient, authenticated: true, initialized: true}
	if _, hasWork, err := client.FetchWorkItem(); err != nil {
		t.Fatalf("FetchWorkItem() error = %v", err)
	} else if !hasWork {
		t.Fatal("expected work item")
	}
}
