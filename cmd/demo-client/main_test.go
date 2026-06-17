package main

import (
	"testing"

	"goetl/internal/model"
)

func TestDemoWorkflowPath(t *testing.T) {
	if got := demoWorkflowPath([]string{"demo-client"}); got != "demo-workflow.json" {
		t.Fatalf("unexpected default workflow path: %s", got)
	}

	if got := demoWorkflowPath([]string{"demo-client", "custom-workflow.json"}); got != "custom-workflow.json" {
		t.Fatalf("unexpected custom workflow path: %s", got)
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
