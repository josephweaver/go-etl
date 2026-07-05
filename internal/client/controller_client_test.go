package client

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"goetl/internal/model"
	"goetl/internal/reposource"
	"goetl/internal/variable"
	"goetl/internal/workflow"
)

func TestControllerClientSubmitWorkflow(t *testing.T) {
	var received WorkflowSubmission
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/status" {
			w.WriteHeader(http.StatusOK)
			return
		}

		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method: %s", r.Method)
		}

		if r.URL.Path != "/workflow" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}

		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Fatalf("decode request: %v", err)
		}

		writeTestSubmissionAcknowledgement(t, w)
	}))
	defer server.Close()

	client := NewControllerClient(server.Client(), testResolver(t, server.URL))
	err := client.SubmitWorkflow(WorkflowSubmission{
		Workflow: workflow.Workflow{
			ID: "cdl",
			Variables: []variable.Variable{
				{
					Name: variable.Name{Namespace: variable.NamespaceWorkflow, Key: "years"},
					TypedExpression: variable.TypedExpression{Type: variable.TypeList, Expression: []variable.TypedExpression{
						{Type: variable.TypeInt, Expression: 2024},
					}},
				},
			},
			Steps: []workflow.Step{
				{
					ID: "download",
					FanOut: &workflow.FanOutStep{
						WorkItem: workflow.FanOutWorkItemTemplate{
							FanOutExpression: "${years[*]}",
							Type:             model.WorkItemTypeWriteDemoOutput,
							OutputPrefix:     "cdl",
							OutputExtension:  ".txt",
						},
					},
				},
			},
		},
		SourceManifest: reposource.SourceManifestDeclaration{
			Files: []reposource.SourceManifestFileDeclaration{
				{Role: reposource.FileRolePythonEntrypoint, Path: "scripts/train.py", ContentType: "text/x-python"},
			},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if received.Workflow.ID != "cdl" {
		t.Fatalf("unexpected workflow id: %s", received.Workflow.ID)
	}

	if len(received.Workflow.Variables) != 1 {
		t.Fatalf("unexpected workflow variable count: %d", len(received.Workflow.Variables))
	}
	if len(received.SourceManifest.Files) != 1 {
		t.Fatalf("source manifest file count = %d, want 1", len(received.SourceManifest.Files))
	}
}

func TestControllerClientSubmitWorkflowAcknowledgement(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/status":
			w.WriteHeader(http.StatusOK)
		case "/workflow":
			writeTestSubmissionAcknowledgement(t, w)
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client := NewControllerClient(server.Client(), testResolver(t, server.URL))
	acknowledgement, err := client.SubmitWorkflowAcknowledgement(WorkflowSubmission{})
	if err != nil {
		t.Fatalf("SubmitWorkflowAcknowledgement() error = %v", err)
	}

	if acknowledgement.SubmissionID != "run-ack-001" {
		t.Fatalf("submission id = %q, want run-ack-001", acknowledgement.SubmissionID)
	}
	if acknowledgement.WorkflowID != "cdl" {
		t.Fatalf("workflow id = %q, want cdl", acknowledgement.WorkflowID)
	}
	if acknowledgement.InitialWorkItemCount != 2 {
		t.Fatalf("initial work item count = %d, want 2", acknowledgement.InitialWorkItemCount)
	}
}

func TestControllerClientSubmissionStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/submissions/sub_1234/status":
			if r.Method != http.MethodGet {
				t.Fatalf("unexpected method: %s", r.Method)
			}
			if err := json.NewEncoder(w).Encode(model.SubmissionStatus{
				SubmissionID:   "sub_1234",
				WorkflowID:     "annual-report",
				Status:         "running",
				KnownWorkItems: 47,
				Queued:         20,
				Running:        4,
				Completed:      23,
				Failed:         0,
				Skipped:        0,
			}); err != nil {
				t.Fatalf("encode status: %v", err)
			}
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client := NewControllerClient(server.Client(), testResolver(t, server.URL))
	status, err := client.SubmissionStatus("sub_1234")
	if err != nil {
		t.Fatalf("SubmissionStatus() error = %v", err)
	}

	if status.SubmissionID != "sub_1234" {
		t.Fatalf("submission id = %q, want sub_1234", status.SubmissionID)
	}
	if status.WorkflowID != "annual-report" {
		t.Fatalf("workflow id = %q, want annual-report", status.WorkflowID)
	}
	if status.Running != 4 || status.Queued != 20 || status.Completed != 23 {
		t.Fatalf("unexpected status counts: %+v", status)
	}
}

func TestControllerClientSubmissionStatusReturnsUsefulErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/submissions/missing-submission/status":
			http.Error(w, "submission not found", http.StatusNotFound)
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client := NewControllerClient(server.Client(), testResolver(t, server.URL))
	_, err := client.SubmissionStatus("missing-submission")
	if err == nil {
		t.Fatal("SubmissionStatus() error = nil, want error")
	}
	if !strings.Contains(err.Error(), `submission "missing-submission" not found`) {
		t.Fatalf("SubmissionStatus() error = %q, want unknown submission message", err.Error())
	}

	failingClient := NewControllerClient(&http.Client{
		Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
			return nil, errors.New("controller unavailable")
		}),
	}, testResolver(t, server.URL))

	_, err = failingClient.SubmissionStatus("sub_1234")
	if err == nil {
		t.Fatal("SubmissionStatus() transport error = nil, want error")
	}
	if !strings.Contains(err.Error(), "controller unavailable") {
		t.Fatalf("SubmissionStatus() transport error = %q, want wrapped transport message", err.Error())
	}
}

func TestControllerClientSubmitWorkflowRun(t *testing.T) {
	var received WorkflowRunSubmission
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/status" {
			w.WriteHeader(http.StatusOK)
			return
		}

		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method: %s", r.Method)
		}

		if r.URL.Path != "/workflow" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}

		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Fatalf("decode request: %v", err)
		}

		writeTestSubmissionAcknowledgement(t, w)
	}))
	defer server.Close()

	client := NewControllerClient(server.Client(), testResolver(t, server.URL))
	err := client.SubmitWorkflowRun(WorkflowRunSubmission{
		Project: SourceDocumentReference{
			Repository: "local:demo",
			Ref:        "main",
			Path:       "project.json",
		},
		Workflow: SourceDocumentReference{
			Repository: "local:demo",
			Ref:        "main",
			Path:       "workflows/demo-workflow.json",
		},
		Variables: []variable.Variable{
			{
				Name:            variable.Name{Namespace: variable.NamespaceOverride, Key: "code_version"},
				TypedExpression: variable.TypedExpression{Type: variable.TypeString, Expression: "test-version"},
			},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if received.Workflow.Path != "workflows/demo-workflow.json" {
		t.Fatalf("workflow path = %q, want workflows/demo-workflow.json", received.Workflow.Path)
	}
	if received.Project.Repository != "local:demo" {
		t.Fatalf("project repository = %q, want local:demo", received.Project.Repository)
	}
	if len(received.Variables) != 1 {
		t.Fatalf("variables count = %d, want 1", len(received.Variables))
	}
}

func TestControllerClientLoadsWorkflowSubmissionFile(t *testing.T) {
	path := demoProjectPath("workflows", "demo-workflow.json")

	submission, err := LoadWorkflowSubmissionFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if submission.Workflow.ID != "cdl-demo" {
		t.Fatalf("unexpected workflow id: %s", submission.Workflow.ID)
	}

	if len(submission.Workflow.Variables) != 1 {
		t.Fatalf("unexpected workflow variable count: %d", len(submission.Workflow.Variables))
	}

	if len(submission.Workflow.Steps) != 1 {
		t.Fatalf("unexpected workflow step count: %d", len(submission.Workflow.Steps))
	}
}

func TestControllerClientLoadsWorkflowSubmissionFileWithSourceManifest(t *testing.T) {
	path := filepath.Join(t.TempDir(), "workflow.json")
	content := []byte(`{
		"workflow": {"ID": "python-demo", "Steps": []},
		"source_manifest": {
			"files": [
				{"role": "python_entrypoint", "path": "scripts/train.py", "content_type": "text/x-python"},
				{"role": "python_environment", "path": "environments/python.json", "content_type": "application/json"}
			]
		},
		"variables": []
	}`)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	submission, err := LoadWorkflowSubmissionFile(path)
	if err != nil {
		t.Fatalf("LoadWorkflowSubmissionFile() error = %v", err)
	}
	if len(submission.SourceManifest.Files) != 2 {
		t.Fatalf("source manifest file count = %d, want 2", len(submission.SourceManifest.Files))
	}
}

func TestControllerClientRejectsInvalidWorkflowSourceManifest(t *testing.T) {
	path := filepath.Join(t.TempDir(), "workflow.json")
	content := []byte(`{
		"workflow": {"ID": "python-demo", "Steps": []},
		"source_manifest": {
			"files": [
				{"role": "project", "path": "project.json"}
			]
		},
		"variables": []
	}`)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	if _, err := LoadWorkflowSubmissionFile(path); err == nil {
		t.Fatal("expected source manifest validation error")
	}
}

func TestControllerClientLoadsWorkflowRunSubmissionFile(t *testing.T) {
	path := demoProjectPath("submissions", "demo-workflow-run.json")

	submission, err := LoadWorkflowRunSubmissionFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if submission.Project.Repository != "local:demo" {
		t.Fatalf("project repository = %q, want local:demo", submission.Project.Repository)
	}
	if submission.Project.Path != "project.json" {
		t.Fatalf("project path = %q, want project.json", submission.Project.Path)
	}
	if submission.Workflow.Path != "workflows/demo-workflow.json" {
		t.Fatalf("workflow path = %q, want workflows/demo-workflow.json", submission.Workflow.Path)
	}
}

func TestControllerClientLoadsSummaryWorkflowSubmissionFile(t *testing.T) {
	path := demoProjectPath("workflows", "demo-summary-workflow.json")

	submission, err := LoadWorkflowSubmissionFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if submission.Workflow.ID != "summary-demo" {
		t.Fatalf("unexpected workflow id: %s", submission.Workflow.ID)
	}

	template := submission.Workflow.Steps[0].FanOut.WorkItem
	if template.Type != model.WorkItemTypeSummarizeInputFile {
		t.Fatalf("unexpected work item type: %s", template.Type)
	}

	items, ok := submission.Workflow.Variables[0].Expression.([]variable.TypedExpression)
	if !ok || len(items) != 2 {
		t.Fatalf("unexpected summary items expression: %#v", submission.Workflow.Variables[0].Expression)
	}

	if template.Parameters["input_path"].Value != "unset" {
		t.Fatalf("unexpected input_path template parameter: %+v", template.Parameters["input_path"])
	}

	if template.ParameterAccessors["input_path"] != ".input_path" {
		t.Fatalf("unexpected input_path parameter accessor: %s", template.ParameterAccessors["input_path"])
	}
}

func TestControllerClientSubmitWorkflowRunFile(t *testing.T) {
	var received WorkflowRunSubmission
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/status":
			w.WriteHeader(http.StatusOK)
		case "/workflow":
			if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
				t.Fatalf("decode request: %v", err)
			}
			writeTestSubmissionAcknowledgement(t, w)
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client := NewControllerClient(server.Client(), testResolver(t, server.URL))
	err := client.SubmitWorkflowRunFile(demoProjectPath("submissions", "demo-workflow-run.json"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if received.Workflow.Path != "workflows/demo-workflow.json" {
		t.Fatalf("workflow path = %q, want workflows/demo-workflow.json", received.Workflow.Path)
	}
}

func TestControllerClientSubmitWorkflowFile(t *testing.T) {
	var received WorkflowSubmission
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/status":
			w.WriteHeader(http.StatusOK)
		case "/workflow":
			if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
				t.Fatalf("decode request: %v", err)
			}
			writeTestSubmissionAcknowledgement(t, w)
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client := NewControllerClient(server.Client(), testResolver(t, server.URL))
	err := client.SubmitWorkflowFile(demoProjectPath("workflows", "demo-workflow.json"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if received.Workflow.ID != "cdl-demo" {
		t.Fatalf("unexpected workflow id: %s", received.Workflow.ID)
	}
}

func demoProjectPath(parts ...string) string {
	allParts := append([]string{"..", "..", "..", "go-etl-demo-project"}, parts...)
	return filepath.Join(allParts...)
}

func TestControllerClientRejectsMissingControllerURL(t *testing.T) {
	client := NewControllerClient(nil, variable.NewResolver(variable.NewSet(), variable.ResolverConfig{}))

	err := client.SubmitWorkflow(WorkflowSubmission{})
	if err == nil {
		t.Fatal("expected an error")
	}
}

func TestControllerClientChecksControllerBeforeSubmit(t *testing.T) {
	submitted := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/status" {
			http.Error(w, "not ready", http.StatusServiceUnavailable)
			return
		}

		if r.URL.Path == "/workflow" {
			submitted = true
			writeTestSubmissionAcknowledgement(t, w)
			return
		}
	}))
	defer server.Close()

	client := NewControllerClient(server.Client(), testResolver(t, server.URL))

	err := client.SubmitWorkflow(WorkflowSubmission{})
	if err == nil {
		t.Fatal("expected an error")
	}

	if submitted {
		t.Fatal("workflow was submitted after failed controller check")
	}
}

func TestControllerClientStartsControllerWhenUnavailable(t *testing.T) {
	statusChecks := 0
	submitted := false
	starter := &testControllerStarter{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/status":
			statusChecks++
			if statusChecks < 3 {
				http.Error(w, "not ready", http.StatusServiceUnavailable)
				return
			}
			w.WriteHeader(http.StatusOK)
		case "/workflow":
			submitted = true
			writeTestSubmissionAcknowledgement(t, w)
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client := NewControllerClientWithStarter(server.Client(), testResolverWithPollInterval(t, server.URL, "0s"), starter)

	err := client.SubmitWorkflow(WorkflowSubmission{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if starter.calls != 1 {
		t.Fatalf("unexpected starter calls: %d", starter.calls)
	}

	if statusChecks != 3 {
		t.Fatalf("unexpected status checks: %d", statusChecks)
	}

	if !submitted {
		t.Fatal("expected workflow submission")
	}
}

func TestControllerClientRejectsControllerStartupTimeout(t *testing.T) {
	starter := &testControllerStarter{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/status" {
			http.Error(w, "not ready", http.StatusServiceUnavailable)
			return
		}

		if r.URL.Path == "/workflow" {
			t.Fatal("did not expect workflow submission")
		}
	}))
	defer server.Close()

	client := NewControllerClientWithStarter(server.Client(), testResolverWithPollInterval(t, server.URL, "0s"), starter)

	err := client.SubmitWorkflow(WorkflowSubmission{})
	if err == nil {
		t.Fatal("expected an error")
	}

	if starter.calls != 1 {
		t.Fatalf("unexpected starter calls: %d", starter.calls)
	}
}

func TestControllerClientRejectsFailedSubmission(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/status" {
			w.WriteHeader(http.StatusOK)
			return
		}

		http.Error(w, "failed", http.StatusBadRequest)
	}))
	defer server.Close()

	client := NewControllerClient(server.Client(), testResolver(t, server.URL))

	err := client.SubmitWorkflow(WorkflowSubmission{})
	if err == nil {
		t.Fatal("expected an error")
	}
}

func TestControllerClientShutdownWhenIdle(t *testing.T) {
	shutdown := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/status":
			if err := json.NewEncoder(w).Encode(model.ControllerStatus{}); err != nil {
				t.Fatalf("encode status: %v", err)
			}
		case "/shutdown":
			if r.Method != http.MethodPost {
				t.Fatalf("unexpected method: %s", r.Method)
			}
			shutdown = true
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client := NewControllerClient(server.Client(), testResolver(t, server.URL))

	status, err := client.ShutdownWhenIdle(1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if status.Pending != 0 || status.Assigned != 0 {
		t.Fatalf("unexpected status: %+v", status)
	}

	if !shutdown {
		t.Fatal("expected shutdown request")
	}
}

func TestControllerClientDoesNotShutdownWhenBusy(t *testing.T) {
	shutdown := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/status":
			status := model.ControllerStatus{Pending: 1}
			if err := json.NewEncoder(w).Encode(status); err != nil {
				t.Fatalf("encode status: %v", err)
			}
		case "/shutdown":
			shutdown = true
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client := NewControllerClient(server.Client(), testResolver(t, server.URL))

	_, err := client.ShutdownWhenIdle(1)
	if err == nil {
		t.Fatal("expected an error")
	}

	if shutdown {
		t.Fatal("did not expect shutdown request")
	}
}

func TestControllerClientUsesStatusPollInterval(t *testing.T) {
	statusChecks := 0
	shutdown := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/status":
			statusChecks++
			status := model.ControllerStatus{Pending: 1}
			if statusChecks == 2 {
				status = model.ControllerStatus{Attempts: 2, AttemptVariables: 20}
			}
			if err := json.NewEncoder(w).Encode(status); err != nil {
				t.Fatalf("encode status: %v", err)
			}
		case "/shutdown":
			shutdown = true
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client := NewControllerClient(server.Client(), testResolverWithPollInterval(t, server.URL, "0s"))

	status, err := client.ShutdownWhenIdle(2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if status.Pending != 0 || status.Attempts != 2 || status.AttemptVariables != 20 {
		t.Fatalf("unexpected status: %+v", status)
	}

	if statusChecks != 2 {
		t.Fatalf("unexpected status check count: %d", statusChecks)
	}

	if !shutdown {
		t.Fatal("expected shutdown request")
	}
}

type testControllerStarter struct {
	calls int
}

func (s *testControllerStarter) StartController() error {
	s.calls++
	return nil
}

func writeTestSubmissionAcknowledgement(t *testing.T, w http.ResponseWriter) {
	t.Helper()

	w.WriteHeader(http.StatusAccepted)
	if err := json.NewEncoder(w).Encode(model.SubmissionAcknowledgement{
		SubmissionID:         "run-ack-001",
		WorkflowID:           "cdl",
		InitialWorkItemCount: 2,
	}); err != nil {
		t.Fatalf("encode submission acknowledgement: %v", err)
	}
}

func testResolver(t *testing.T, controllerURL string) variable.Resolver {
	t.Helper()

	return testResolverWithVariables(t, variable.Variable{Name: variable.Name{Namespace: variable.NamespaceControllerConfig, Key: "controller_url"}, TypedExpression: variable.TypedExpression{Type: variable.TypeString, Expression: controllerURL}})
}

func testResolverWithPollInterval(t *testing.T, controllerURL string, interval string) variable.Resolver {
	t.Helper()

	return testResolverWithVariables(t,
		variable.Variable{Name: variable.Name{Namespace: variable.NamespaceControllerConfig, Key: "controller_url"}, TypedExpression: variable.TypedExpression{Type: variable.TypeString, Expression: controllerURL}},

		variable.Variable{Name: variable.Name{Namespace: variable.NamespaceControllerConfig, Key: "client_status_poll_interval"}, TypedExpression: variable.TypedExpression{Type: variable.TypeString, Expression: interval}},
	)
}

func testResolverWithVariables(t *testing.T, variables ...variable.Variable) variable.Resolver {
	t.Helper()

	scope, err := variable.NewScope(variables...)
	if err != nil {
		t.Fatal(err)
	}

	return variable.NewResolver(variable.NewSet(scope), variable.ResolverConfig{})
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (fn roundTripperFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}
