package main

import (
	"testing"

	"goetl/internal/model"
)

func TestDemoWorkflowRunPath(t *testing.T) {
	if got := demoWorkflowRunPath([]string{"demo-client"}); got != "demo-workflow-run.json" {
		t.Fatalf("unexpected default workflow run path: %s", got)
	}

	if got := demoWorkflowRunPath([]string{"demo-client", "custom-workflow-run.json"}); got != "custom-workflow-run.json" {
		t.Fatalf("unexpected custom workflow run path: %s", got)
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
