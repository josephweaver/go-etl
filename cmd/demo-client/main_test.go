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

func TestParseLogsCommand(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want cliCommand
	}{
		{
			name: "default controller URL",
			args: []string{"goet", "logs", "sub_1234"},
			want: cliCommand{
				Kind:          commandLogs,
				ControllerURL: defaultControllerURL,
				SubmissionID:  "sub_1234",
			},
		},
		{
			name: "controller URL with filters and JSON",
			args: []string{"goet", "logs", "sub_1234", "--controller-url", "http://controller:8080", "--tail", "50", "--level", "warn", "--stream", "stderr", "--attempt-id", "att_42", "--json"},
			want: cliCommand{
				Kind:          commandLogs,
				ControllerURL: "http://controller:8080",
				SubmissionID:  "sub_1234",
				Tail:          50,
				TailSet:       true,
				Level:         "warn",
				Stream:        "stderr",
				AttemptID:     "att_42",
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

func TestParseLogsCommandValidation(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{
			name:    "requires submission ID",
			args:    []string{"goet", "logs"},
			wantErr: "submission_id is required",
		},
		{
			name:    "rejects extra positional",
			args:    []string{"goet", "logs", "sub_1234", "extra"},
			wantErr: "unexpected positional argument",
		},
		{
			name:    "rejects non-positive tail",
			args:    []string{"goet", "logs", "sub_1234", "--tail", "0"},
			wantErr: "tail must be a positive integer",
		},
		{
			name:    "rejects bad tail",
			args:    []string{"goet", "logs", "sub_1234", "--tail", "zero"},
			wantErr: "tail must be a positive integer",
		},
		{
			name:    "rejects watch",
			args:    []string{"goet", "logs", "sub_1234", "--watch"},
			wantErr: "flag provided but not defined: -watch",
		},
		{
			name:    "rejects follow",
			args:    []string{"goet", "logs", "sub_1234", "--follow"},
			wantErr: "flag provided but not defined: -follow",
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

func TestExecuteSubmitCommandWaitsForCompletedSubmission(t *testing.T) {
	var received client.WorkflowSubmission
	statusChecks := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/status":
			w.WriteHeader(http.StatusOK)
		case "/workflow":
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
		case "/submissions/run-ack-001/status":
			statusChecks++
			status := model.SubmissionStatus{
				SubmissionID:   "run-ack-001",
				WorkflowID:     "cdl-demo",
				KnownWorkItems: 1,
			}
			switch statusChecks {
			case 1:
				status.Status = "queued"
				status.Queued = 1
			case 2:
				status.Status = "running"
				status.Running = 1
			default:
				status.Status = "completed"
				status.Completed = 1
			}
			if err := json.NewEncoder(w).Encode(status); err != nil {
				t.Fatalf("encode submission status: %v", err)
			}
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	projectPath, workflowPath := writeSubmitCommandInputs(t)

	output := captureMainTestOutput(t, func() {
		err := executeCommand(cliCommand{
			Kind:          commandSubmit,
			ControllerURL: server.URL,
			ProjectPath:   projectPath,
			WorkflowPath:  workflowPath,
			Wait:          true,
		}, server.Client())
		if err != nil {
			t.Fatalf("executeCommand() error = %v", err)
		}
	})

	if statusChecks != 3 {
		t.Fatalf("status check count = %d, want 3", statusChecks)
	}

	want := strings.Join([]string{
		"Submission: run-ack-001",
		"Workflow: cdl-demo",
		"Initial work items: 0",
		"Submission: run-ack-001",
		"Workflow: cdl-demo",
		"Status: completed",
		"Known work items: 1",
		"Queued: 0",
		"Running: 0",
		"Completed: 1",
		"Failed: 0",
		"Skipped: 0",
	}, "\n")
	if got := strings.TrimSpace(output); got != want {
		t.Fatalf("wait output = %q, want %q", got, want)
	}
	if received.Workflow.ID != "cdl-demo" {
		t.Fatalf("received workflow ID = %q, want cdl-demo", received.Workflow.ID)
	}
}

func TestExecuteSubmitCommandReturnsErrorForFailedSubmission(t *testing.T) {
	statusChecks := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/status":
			w.WriteHeader(http.StatusOK)
		case "/workflow":
			w.WriteHeader(http.StatusAccepted)
			if err := json.NewEncoder(w).Encode(model.SubmissionAcknowledgement{
				SubmissionID:         "run-ack-001",
				WorkflowID:           "cdl-demo",
				InitialWorkItemCount: 0,
			}); err != nil {
				t.Fatalf("encode submission acknowledgement: %v", err)
			}
		case "/submissions/run-ack-001/status":
			statusChecks++
			status := model.SubmissionStatus{
				SubmissionID:   "run-ack-001",
				WorkflowID:     "cdl-demo",
				KnownWorkItems: 1,
				Status:         "failed",
				Failed:         1,
			}
			if statusChecks == 1 {
				status.Status = "running"
				status.Running = 1
				status.Failed = 0
			}
			if err := json.NewEncoder(w).Encode(status); err != nil {
				t.Fatalf("encode submission status: %v", err)
			}
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	projectPath, workflowPath := writeSubmitCommandInputs(t)

	var execErr error
	output := captureMainTestOutput(t, func() {
		execErr = executeCommand(cliCommand{
			Kind:          commandSubmit,
			ControllerURL: server.URL,
			ProjectPath:   projectPath,
			WorkflowPath:  workflowPath,
			Wait:          true,
		}, server.Client())
	})

	if execErr == nil {
		t.Fatal("executeCommand() error = nil, want error")
	}
	if !strings.Contains(execErr.Error(), "failed") {
		t.Fatalf("executeCommand() error = %q, want failed message", execErr.Error())
	}
	if statusChecks != 2 {
		t.Fatalf("status check count = %d, want 2", statusChecks)
	}
	if !strings.Contains(output, "Status: failed") {
		t.Fatalf("wait output = %q, want failed status", strings.TrimSpace(output))
	}
}

func TestExecuteSubmitCommandReturnsErrorForStatusCommunicationFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/status":
			w.WriteHeader(http.StatusOK)
		case "/workflow":
			w.WriteHeader(http.StatusAccepted)
			if err := json.NewEncoder(w).Encode(model.SubmissionAcknowledgement{
				SubmissionID:         "run-ack-001",
				WorkflowID:           "cdl-demo",
				InitialWorkItemCount: 0,
			}); err != nil {
				t.Fatalf("encode submission acknowledgement: %v", err)
			}
		case "/submissions/run-ack-001/status":
			http.Error(w, "temporarily unavailable", http.StatusServiceUnavailable)
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	projectPath, workflowPath := writeSubmitCommandInputs(t)

	var execErr error
	output := captureMainTestOutput(t, func() {
		execErr = executeCommand(cliCommand{
			Kind:          commandSubmit,
			ControllerURL: server.URL,
			ProjectPath:   projectPath,
			WorkflowPath:  workflowPath,
			Wait:          true,
		}, server.Client())
	})

	if execErr == nil {
		t.Fatal("executeCommand() error = nil, want error")
	}
	if !strings.Contains(execErr.Error(), "unexpected status 503") {
		t.Fatalf("executeCommand() error = %q, want status error", execErr.Error())
	}
	if !strings.Contains(output, "Submission: run-ack-001") {
		t.Fatalf("wait output = %q, want acknowledgement", strings.TrimSpace(output))
	}
	if strings.Contains(output, "Status:") {
		t.Fatalf("wait output = %q, want no final status after communication failure", strings.TrimSpace(output))
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

func TestExecuteStatusCommandPrintsSubmissionStatusJSON(t *testing.T) {
	currentStage := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/submissions/sub_1234/status":
			if err := json.NewEncoder(w).Encode(model.SubmissionStatus{
				SubmissionID:   "sub_1234",
				WorkflowID:     "annual-report",
				Status:         "running",
				KnownWorkItems: 1,
				Queued:         1,
				Dependency: &model.SubmissionDependencyStatus{
					WorkflowState:     "running",
					CurrentStageIndex: &currentStage,
					StageCount:        1,
				},
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
			JSON:          true,
		}, server.Client())
		if err != nil {
			t.Fatalf("executeCommand() error = %v", err)
		}
	})

	var got model.SubmissionStatus
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &got); err != nil {
		t.Fatalf("unmarshal status JSON = %v; output=%q", err, output)
	}
	if got.Dependency == nil || got.Dependency.WorkflowState != "running" {
		t.Fatalf("dependency status = %+v, want running dependency", got.Dependency)
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

func TestExecuteLogsCommandPrintsSubmissionLogs(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/submissions/sub_1234/logs" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("tail") != "2" {
			t.Fatalf("unexpected tail query: %s", r.URL.Query().Get("tail"))
		}

		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(client.SubmissionLogsResponse{
			SubmissionID: "sub_1234",
			Tail:         2,
			Truncated:    false,
			Entries: []model.LogObservation{
				{
					Timestamp: "2026-07-05T11:00:00Z",
					Level:     model.LogLevelInfo,
					Component: "worker",
					Stream:    "stdout",
					AttemptID: "att_1",
					Message:   "started item",
				},
				{
					Timestamp: "2026-07-05T11:00:01Z",
					Level:     model.LogLevelWarn,
					Component: "worker",
					Stream:    "stderr",
					Message:   "minor warning",
				},
			},
		}); err != nil {
			t.Fatalf("encode submission logs: %v", err)
		}
	}))
	defer server.Close()

	output := captureMainTestOutput(t, func() {
		err := executeCommand(cliCommand{
			Kind:          commandLogs,
			ControllerURL: server.URL,
			SubmissionID:  "sub_1234",
			Tail:          2,
			TailSet:       true,
		}, server.Client())
		if err != nil {
			t.Fatalf("executeCommand() error = %v", err)
		}
	})

	want := strings.Join([]string{
		"2026-07-05T11:00:00Z info worker stdout attempt=att_1 started item",
		"2026-07-05T11:00:01Z warn worker stderr minor warning",
	}, "\n")
	if got := strings.TrimSpace(output); got != want {
		t.Fatalf("logs output = %q, want %q", got, want)
	}
}

func TestExecuteLogsCommandPrintsJsonPayload(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/submissions/sub_1234/logs" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}

		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(client.SubmissionLogsResponse{
			SubmissionID: "sub_1234",
			Tail:         1,
			Truncated:    false,
			Entries: []model.LogObservation{
				{
					Timestamp: "2026-07-05T11:00:00Z",
					Level:     model.LogLevelInfo,
					Component: "worker",
					Message:   "hello",
				},
			},
		}); err != nil {
			t.Fatalf("encode submission logs: %v", err)
		}
	}))
	defer server.Close()

	output := captureMainTestOutput(t, func() {
		err := executeCommand(cliCommand{
			Kind:          commandLogs,
			ControllerURL: server.URL,
			SubmissionID:  "sub_1234",
			JSON:          true,
		}, server.Client())
		if err != nil {
			t.Fatalf("executeCommand() error = %v", err)
		}
	})

	var got client.SubmissionLogsResponse
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &got); err != nil {
		t.Fatalf("unmarshal logs JSON = %v", err)
	}
	if got.SubmissionID != "sub_1234" {
		t.Fatalf("submission_id = %q, want sub_1234", got.SubmissionID)
	}
	if got.Tail != 1 {
		t.Fatalf("tail = %d, want 1", got.Tail)
	}
	if got.Truncated {
		t.Fatal("truncated = true, want false")
	}
	if len(got.Entries) != 1 {
		t.Fatalf("entries length = %d, want 1", len(got.Entries))
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

func TestFormatSubmissionLog(t *testing.T) {
	got := formatSubmissionLog(model.LogObservation{
		Timestamp: "2026-07-05T11:00:00Z",
		Level:     model.LogLevelInfo,
		Component: "worker",
		Stream:    "stdout",
		AttemptID: "att_1",
		Message:   "hello",
	})
	want := "2026-07-05T11:00:00Z info worker stdout attempt=att_1 hello"
	if got != want {
		t.Fatalf("formatSubmissionLog() = %q, want %q", got, want)
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

func TestFormatSubmissionStatusIncludesCompactDependencySummary(t *testing.T) {
	currentStage := 0
	got := formatSubmissionStatus(model.SubmissionStatus{
		SubmissionID:   "sub_1234",
		WorkflowID:     "annual-report",
		Status:         "running",
		KnownWorkItems: 1,
		Queued:         1,
		Dependency: &model.SubmissionDependencyStatus{
			WorkflowState:     "running",
			CurrentStageIndex: &currentStage,
			StageCount:        2,
			Stages: []model.SubmissionDependencyStageStatus{
				{
					StageIndex: 0,
					State:      "ready",
					StepCount:  1,
					Counts: model.SubmissionDependencyCounts{
						AssignablePending: 1,
					},
				},
				{
					StageIndex: 1,
					State:      "blocked",
					StepCount:  1,
					Counts: model.SubmissionDependencyCounts{
						BlockedFuture: 1,
					},
				},
			},
		},
	})
	if !strings.Contains(got, "Dependency workflow: running") {
		t.Fatalf("formatSubmissionStatus() = %q, want dependency workflow line", got)
	}
	if !strings.Contains(got, "Stage 0: ready steps=1 assignable_pending=1 blocked_future=0") {
		t.Fatalf("formatSubmissionStatus() = %q, want compact stage 0 summary", got)
	}
	if !strings.Contains(got, "Stage 1: blocked steps=1 assignable_pending=0 blocked_future=1") {
		t.Fatalf("formatSubmissionStatus() = %q, want compact stage 1 summary", got)
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

func writeSubmitCommandInputs(t *testing.T) (string, string) {
	t.Helper()

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

	return projectPath, workflowPath
}

func writeMainTestFile(t *testing.T, dir, name, content string) string {
	t.Helper()

	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", name, err)
	}
	return path
}
