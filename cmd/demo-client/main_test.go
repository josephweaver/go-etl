package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"goetl/internal/client"
	"goetl/internal/model"
	"goetl/internal/variable"
)

func TestDemoWorkflowRunPath(t *testing.T) {
	want := filepath.Join("..", "go-etl-demo-project", "submissions", "demo-workflow-run.json")
	if got := demoWorkflowRunPath([]string{"demo-client"}); got != want {
		t.Fatalf("unexpected default workflow run path: %s", got)
	}

	if got := demoWorkflowRunPath([]string{"demo-client", "custom-workflow-run.json"}); got != "custom-workflow-run.json" {
		t.Fatalf("unexpected custom workflow run path: %s", got)
	}
}

func TestParseCommandKeepsZeroArgumentDemoPath(t *testing.T) {
	command, err := parseCommand([]string{"demo-client"})
	if err != nil {
		t.Fatalf("parseCommand() unexpected error: %v", err)
	}

	if command.Kind != commandDemo {
		t.Fatalf("Kind = %q, want %q", command.Kind, commandDemo)
	}

	want := filepath.Join("..", "go-etl-demo-project", "submissions", "demo-workflow-run.json")
	if command.WorkflowRunPath != want {
		t.Fatalf("WorkflowRunPath = %q, want %q", command.WorkflowRunPath, want)
	}
}

func TestParseSubmitCommand(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want cliCommand
	}{
		{
			name: "controller file",
			args: []string{"goet", "submit", "--controller", "controller.json", "--project", "project.json", "--workflow", "workflow.json"},
			want: cliCommand{
				Kind:           commandSubmit,
				ControllerPath: "controller.json",
				ProjectPath:    "project.json",
				WorkflowPath:   "workflow.json",
			},
		},
		{
			name: "controller URL with accepted deferred flags",
			args: []string{"goet", "submit", "--controller-url", "http://controller:8080", "--project", "project.json", "--workflow", "workflow.json", "--wait", "--json"},
			want: cliCommand{
				Kind:          commandSubmit,
				ControllerURL: "http://controller:8080",
				ProjectPath:   "project.json",
				WorkflowPath:  "workflow.json",
				Wait:          true,
				JSON:          true,
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := parseCommand(test.args)
			if err != nil {
				t.Fatalf("parseCommand() unexpected error: %v", err)
			}
			if got != test.want {
				t.Fatalf("parseCommand() = %+v, want %+v", got, test.want)
			}
		})
	}
}

func TestParseSubmitCommandValidation(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{
			name:    "requires one controller selector",
			args:    []string{"goet", "submit", "--project", "project.json", "--workflow", "workflow.json"},
			wantErr: "exactly one of --controller or --controller-url is required",
		},
		{
			name:    "rejects both controller selectors",
			args:    []string{"goet", "submit", "--controller", "controller.json", "--controller-url", "http://controller:8080", "--project", "project.json", "--workflow", "workflow.json"},
			wantErr: "cannot both be supplied",
		},
		{
			name:    "requires project",
			args:    []string{"goet", "submit", "--controller", "controller.json", "--workflow", "workflow.json"},
			wantErr: "--project is required",
		},
		{
			name:    "requires workflow",
			args:    []string{"goet", "submit", "--controller", "controller.json", "--project", "project.json"},
			wantErr: "--workflow is required",
		},
		{
			name:    "rejects watch",
			args:    []string{"goet", "submit", "--controller", "controller.json", "--project", "project.json", "--workflow", "workflow.json", "--watch"},
			wantErr: "flag provided but not defined: -watch",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := parseCommand(test.args)
			if err == nil {
				t.Fatal("parseCommand() error = nil, want error")
			}
			if !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("parseCommand() error = %q, want substring %q", err.Error(), test.wantErr)
			}
		})
	}
}

func TestExecuteSubmitCommandPostsLoadedInputs(t *testing.T) {
	var received client.WorkflowSubmission
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/status":
			w.WriteHeader(http.StatusOK)
		case "/workflow":
			if r.Method != http.MethodPost {
				t.Fatalf("method = %s, want POST", r.Method)
			}
			if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
				t.Fatalf("decode workflow submission: %v", err)
			}
			w.WriteHeader(http.StatusAccepted)
			if err := json.NewEncoder(w).Encode(model.SubmissionAcknowledgement{
				SubmissionID:         "run-ack-001",
				WorkflowID:           "cdl-demo",
				InitialWorkItemCount: 0,
			}); err != nil {
				t.Fatalf("encode submission acknowledgement: %v", err)
			}
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	dir := t.TempDir()
	projectPath := writeMainTestFile(t, dir, "project.json", `{"id":"go-etl-demo"}`)
	workflowPath := writeMainTestFile(t, dir, "workflow.json", `{
		"workflow": {
			"ID": "cdl-demo",
			"Variables": [
				{"name":{"namespace":"workflow","key":"years"},"type":"list","expression":[{"type":"int","expression":2026}]}
			],
			"Steps": []
		},
		"source_manifest": {},
		"variables": [
			{"name":{"namespace":"override","key":"code_version"},"type":"string","expression":"test-version"}
		]
	}`)

	err := executeCommand(cliCommand{
		Kind:          commandSubmit,
		ControllerURL: server.URL,
		ProjectPath:   projectPath,
		WorkflowPath:  workflowPath,
	}, server.Client())
	if err != nil {
		t.Fatalf("executeCommand() error = %v", err)
	}

	if received.Workflow.ID != "cdl-demo" {
		t.Fatalf("received workflow ID = %q, want cdl-demo", received.Workflow.ID)
	}
	if len(received.Workflow.Variables) != 1 {
		t.Fatalf("workflow variable count = %d, want 1", len(received.Workflow.Variables))
	}
	if len(received.Variables) != 2 {
		t.Fatalf("submission variable count = %d, want 2", len(received.Variables))
	}
	if received.Variables[0].Name != (variable.Name{Namespace: variable.NamespaceProjectConfig, Key: "id"}) {
		t.Fatalf("first submission variable = %+v, want project_config.id", received.Variables[0].Name)
	}
}

func TestExecuteStatusCommandPrintsSubmissionStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/submissions/sub_1234/status":
			if r.Method != http.MethodGet {
				t.Fatalf("method = %s, want GET", r.Method)
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
				t.Fatalf("encode submission status: %v", err)
			}
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	output := captureMainTestOutput(t, func() {
		err := executeCommand(cliCommand{
			Kind:          commandStatus,
			ControllerURL: server.URL,
			SubmissionID:  "sub_1234",
		}, server.Client())
		if err != nil {
			t.Fatalf("executeCommand() error = %v", err)
		}
	})

	want := strings.Join([]string{
		"Submission: sub_1234",
		"Workflow: annual-report",
		"Status: running",
		"Known work items: 47",
		"Queued: 20",
		"Running: 4",
		"Completed: 23",
		"Failed: 0",
		"Skipped: 0",
	}, "\n")
	if got := strings.TrimSpace(output); got != want {
		t.Fatalf("status output = %q, want %q", got, want)
	}
}

func TestExecuteStatusCommandReturnsUsefulErrorForUnknownSubmission(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/submissions/missing-submission/status":
			http.Error(w, "submission not found", http.StatusNotFound)
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	err := executeCommand(cliCommand{
		Kind:          commandStatus,
		ControllerURL: server.URL,
		SubmissionID:  "missing-submission",
	}, server.Client())
	if err == nil {
		t.Fatal("executeCommand() error = nil, want error")
	}
	if !strings.Contains(err.Error(), `submission "missing-submission" not found`) {
		t.Fatalf("executeCommand() error = %q, want unknown submission message", err.Error())
	}
}

func TestParseStatusCommand(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want cliCommand
	}{
		{
			name: "default controller URL",
			args: []string{"goet", "status", "sub_1234"},
			want: cliCommand{
				Kind:          commandStatus,
				ControllerURL: defaultControllerURL,
				SubmissionID:  "sub_1234",
			},
		},
		{
			name: "explicit controller URL and JSON after submission ID",
			args: []string{"goet", "status", "sub_1234", "--controller-url", "http://controller:8080", "--json"},
			want: cliCommand{
				Kind:          commandStatus,
				ControllerURL: "http://controller:8080",
				SubmissionID:  "sub_1234",
				JSON:          true,
			},
		},
		{
			name: "equals controller URL flag",
			args: []string{"goet", "status", "sub_1234", "--controller-url=http://controller:8080"},
			want: cliCommand{
				Kind:          commandStatus,
				ControllerURL: "http://controller:8080",
				SubmissionID:  "sub_1234",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := parseCommand(test.args)
			if err != nil {
				t.Fatalf("parseCommand() unexpected error: %v", err)
			}
			if got != test.want {
				t.Fatalf("parseCommand() = %+v, want %+v", got, test.want)
			}
		})
	}
}

func TestParseStatusCommandValidation(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{
			name:    "requires submission ID",
			args:    []string{"goet", "status"},
			wantErr: "submission_id is required",
		},
		{
			name:    "rejects extra positional",
			args:    []string{"goet", "status", "sub_1234", "extra"},
			wantErr: "unexpected positional argument",
		},
		{
			name:    "rejects watch",
			args:    []string{"goet", "status", "sub_1234", "--watch"},
			wantErr: "flag provided but not defined: -watch",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := parseCommand(test.args)
			if err == nil {
				t.Fatal("parseCommand() error = nil, want error")
			}
			if !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("parseCommand() error = %q, want substring %q", err.Error(), test.wantErr)
			}
		})
	}
}

func TestFormatFinalStatusIncludesReuseCandidates(t *testing.T) {
	status := model.ControllerStatus{
		Pending:                1,
		Assigned:               2,
		Failed:                 3,
		PendingReuseCandidates: 4,
		Attempts:               5,
		AttemptVariables:       6,
	}

	got := formatFinalStatus(status)
	want := "final status: pending=1 assigned=2 failed=3 pending_reuse_candidates=4 attempts=5 attempt_variables=6"
	if got != want {
		t.Fatalf("formatFinalStatus() = %q, want %q", got, want)
	}
}

func TestFormatSubmissionAcknowledgement(t *testing.T) {
	got := formatSubmissionAcknowledgement(model.SubmissionAcknowledgement{
		SubmissionID:         "run-ack-001",
		WorkflowID:           "cdl-demo",
		InitialWorkItemCount: 2,
	})
	want := "Submission: run-ack-001\nWorkflow: cdl-demo\nInitial work items: 2"
	if got != want {
		t.Fatalf("formatSubmissionAcknowledgement() = %q, want %q", got, want)
	}
}

func TestFormatSubmissionStatus(t *testing.T) {
	got := formatSubmissionStatus(model.SubmissionStatus{
		SubmissionID:   "sub_1234",
		WorkflowID:     "annual-report",
		Status:         "running",
		KnownWorkItems: 47,
		Queued:         20,
		Running:        4,
		Completed:      23,
		Failed:         0,
		Skipped:        0,
	})
	want := strings.Join([]string{
		"Submission: sub_1234",
		"Workflow: annual-report",
		"Status: running",
		"Known work items: 47",
		"Queued: 20",
		"Running: 4",
		"Completed: 23",
		"Failed: 0",
		"Skipped: 0",
	}, "\n")
	if got != want {
		t.Fatalf("formatSubmissionStatus() = %q, want %q", got, want)
	}
}

func captureMainTestOutput(t *testing.T, fn func()) string {
	t.Helper()

	originalStdout := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() error = %v", err)
	}
	os.Stdout = writer

	fn()

	if err := writer.Close(); err != nil {
		t.Fatalf("close stdout pipe writer: %v", err)
	}
	os.Stdout = originalStdout

	output, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read stdout pipe: %v", err)
	}
	return string(output)
}

func writeMainTestFile(t *testing.T, dir, name, content string) string {
	t.Helper()

	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", name, err)
	}
	return path
}
