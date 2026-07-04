package client

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"goetl/internal/model"
	"goetl/internal/variable"
	"goetl/internal/workflow"
)

func TestWorkflowClientSubmitWorkflow(t *testing.T) {
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

		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := NewWorkflowClient(server.Client(), testResolver(t, server.URL))
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
}

func TestWorkflowClientSubmitWorkflowRun(t *testing.T) {
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

		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := NewWorkflowClient(server.Client(), testResolver(t, server.URL))
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

func TestWorkflowClientLoadsWorkflowSubmissionFile(t *testing.T) {
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

func TestWorkflowClientLoadsWorkflowRunSubmissionFile(t *testing.T) {
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

func TestWorkflowClientLoadsSummaryWorkflowSubmissionFile(t *testing.T) {
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

func TestWorkflowClientSubmitWorkflowRunFile(t *testing.T) {
	var received WorkflowRunSubmission
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/status":
			w.WriteHeader(http.StatusOK)
		case "/workflow":
			if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
				t.Fatalf("decode request: %v", err)
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client := NewWorkflowClient(server.Client(), testResolver(t, server.URL))
	err := client.SubmitWorkflowRunFile(demoProjectPath("submissions", "demo-workflow-run.json"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if received.Workflow.Path != "workflows/demo-workflow.json" {
		t.Fatalf("workflow path = %q, want workflows/demo-workflow.json", received.Workflow.Path)
	}
}

func TestWorkflowClientSubmitWorkflowFile(t *testing.T) {
	var received WorkflowSubmission
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/status":
			w.WriteHeader(http.StatusOK)
		case "/workflow":
			if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
				t.Fatalf("decode request: %v", err)
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client := NewWorkflowClient(server.Client(), testResolver(t, server.URL))
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

func TestWorkflowClientRejectsMissingControllerURL(t *testing.T) {
	client := NewWorkflowClient(nil, variable.NewResolver(variable.NewSet(), variable.ResolverConfig{}))

	err := client.SubmitWorkflow(WorkflowSubmission{})
	if err == nil {
		t.Fatal("expected an error")
	}
}

func TestWorkflowClientChecksControllerBeforeSubmit(t *testing.T) {
	submitted := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/status" {
			http.Error(w, "not ready", http.StatusServiceUnavailable)
			return
		}

		if r.URL.Path == "/workflow" {
			submitted = true
			w.WriteHeader(http.StatusNoContent)
			return
		}
	}))
	defer server.Close()

	client := NewWorkflowClient(server.Client(), testResolver(t, server.URL))

	err := client.SubmitWorkflow(WorkflowSubmission{})
	if err == nil {
		t.Fatal("expected an error")
	}

	if submitted {
		t.Fatal("workflow was submitted after failed controller check")
	}
}

func TestWorkflowClientStartsControllerWhenUnavailable(t *testing.T) {
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
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client := NewWorkflowClientWithStarter(server.Client(), testResolverWithPollInterval(t, server.URL, "0s"), starter)

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

func TestWorkflowClientRejectsControllerStartupTimeout(t *testing.T) {
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

	client := NewWorkflowClientWithStarter(server.Client(), testResolverWithPollInterval(t, server.URL, "0s"), starter)

	err := client.SubmitWorkflow(WorkflowSubmission{})
	if err == nil {
		t.Fatal("expected an error")
	}

	if starter.calls != 1 {
		t.Fatalf("unexpected starter calls: %d", starter.calls)
	}
}

func TestWorkflowClientRejectsFailedSubmission(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/status" {
			w.WriteHeader(http.StatusOK)
			return
		}

		http.Error(w, "failed", http.StatusBadRequest)
	}))
	defer server.Close()

	client := NewWorkflowClient(server.Client(), testResolver(t, server.URL))

	err := client.SubmitWorkflow(WorkflowSubmission{})
	if err == nil {
		t.Fatal("expected an error")
	}
}

func TestWorkflowClientShutdownWhenIdle(t *testing.T) {
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

	client := NewWorkflowClient(server.Client(), testResolver(t, server.URL))

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

func TestWorkflowClientDoesNotShutdownWhenBusy(t *testing.T) {
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

	client := NewWorkflowClient(server.Client(), testResolver(t, server.URL))

	_, err := client.ShutdownWhenIdle(1)
	if err == nil {
		t.Fatal("expected an error")
	}

	if shutdown {
		t.Fatal("did not expect shutdown request")
	}
}

func TestWorkflowClientUsesStatusPollInterval(t *testing.T) {
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

	client := NewWorkflowClient(server.Client(), testResolverWithPollInterval(t, server.URL, "0s"))

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
