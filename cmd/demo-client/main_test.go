package main

import "testing"

func TestDemoWorkflowPath(t *testing.T) {
	if got := demoWorkflowPath([]string{"demo-client"}); got != "demo-workflow.json" {
		t.Fatalf("unexpected default workflow path: %s", got)
	}

	if got := demoWorkflowPath([]string{"demo-client", "custom-workflow.json"}); got != "custom-workflow.json" {
		t.Fatalf("unexpected custom workflow path: %s", got)
	}
}
