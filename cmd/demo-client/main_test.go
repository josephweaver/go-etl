package main

import (
	"path/filepath"
	"strings"
	"testing"

	"goetl/internal/model"
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
