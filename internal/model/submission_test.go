package model

import (
	"encoding/json"
	"testing"
)

func TestSubmissionAcknowledgementJSONShape(t *testing.T) {
	acknowledgement := SubmissionAcknowledgement{
		SubmissionID:         "run-ack-001",
		WorkflowID:           "cdl-demo",
		InitialWorkItemCount: 2,
	}

	data, err := json.Marshal(acknowledgement)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	want := `{"submission_id":"run-ack-001","workflow_id":"cdl-demo","initial_work_item_count":2}`
	if string(data) != want {
		t.Fatalf("JSON = %s, want %s", data, want)
	}
}

func TestSubmissionStatusJSONShape(t *testing.T) {
	status := SubmissionStatus{
		SubmissionID:   "run-status-001",
		WorkflowID:     "annual-report",
		Status:         "running",
		KnownWorkItems: 47,
		Queued:         20,
		Running:        4,
		Completed:      23,
		Failed:         0,
		Skipped:        0,
	}

	data, err := json.Marshal(status)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	want := `{"submission_id":"run-status-001","workflow_id":"annual-report","status":"running","known_work_items":47,"queued":20,"running":4,"completed":23,"failed":0,"skipped":0}`
	if string(data) != want {
		t.Fatalf("JSON = %s, want %s", data, want)
	}
}
