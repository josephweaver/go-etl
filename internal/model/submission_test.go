package model

import (
	"encoding/json"
	"strings"
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

func TestSubmissionStatusJSONShapeIncludesDependencySummary(t *testing.T) {
	currentStage := 0
	status := SubmissionStatus{
		SubmissionID:   "run-status-001",
		WorkflowID:     "annual-report",
		Status:         "running",
		KnownWorkItems: 1,
		Queued:         1,
		Dependency: &SubmissionDependencyStatus{
			WorkflowState:     "running",
			CurrentStageIndex: &currentStage,
			StageCount:        2,
			Counts: SubmissionDependencyCounts{
				AssignablePending: 1,
				BlockedFuture:     1,
			},
			Stages: []SubmissionDependencyStageStatus{
				{
					StageIndex: 0,
					State:      "ready",
					StepCount:  1,
					Counts: SubmissionDependencyCounts{
						AssignablePending: 1,
					},
					Steps: []SubmissionDependencyStepStatus{
						{
							StageIndex: 0,
							StepIndex:  0,
							StepID:     "download",
							State:      "ready",
							Counts: SubmissionDependencyCounts{
								AssignablePending: 1,
							},
						},
					},
				},
				{
					StageIndex: 1,
					State:      "blocked",
					StepCount:  1,
					Counts: SubmissionDependencyCounts{
						BlockedFuture: 1,
					},
					Steps: []SubmissionDependencyStepStatus{
						{
							StageIndex: 1,
							StepIndex:  1,
							StepID:     "summarize",
							State:      "blocked",
							Counts: SubmissionDependencyCounts{
								BlockedFuture: 1,
							},
						},
					},
				},
			},
		},
	}

	data, err := json.Marshal(status)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if decoded["dependency"] == nil {
		t.Fatalf("JSON = %s, want dependency summary", data)
	}
	if strings.Contains(string(data), "output_json") {
		t.Fatalf("JSON = %s, must not expose dependency output JSON fields", data)
	}
}
