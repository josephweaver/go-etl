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
