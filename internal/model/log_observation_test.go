package model

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestLogObservationValidate(t *testing.T) {
	t.Run("valid minimal controller observation", func(t *testing.T) {
		observation := LogObservation{
			Component: "controller",
			Level:     LogLevelInfo,
			Timestamp: "2026-07-05T12:00:00Z",
			Message:   "controller accepted request",
		}

		if err := observation.Validate(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("valid worker stdout observation with routing metadata", func(t *testing.T) {
		observation := LogObservation{
			ObservationID: "obs-001",
			SubmissionID:  "sub-001",
			WorkflowID:    "python-hello",
			WorkflowName:  "python hello",
			RunID:         "run-001",
			StepID:        "step-hello",
			StepName:      "hello",
			WorkItemID:    "work-item-001",
			AttemptID:     "attempt-001",
			WorkerID:      "worker-001",
			Component:     "worker",
			Stream:        "stdout",
			Level:         LogLevelInfo,
			Timestamp:     "2026-07-05T12:00:00.123456789Z",
			Message:       "hello from python",
		}

		if err := observation.Validate(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("valid worker stderr observation", func(t *testing.T) {
		observation := LogObservation{
			ObservationID: "obs-002",
			Component:     "worker",
			Stream:        "stderr",
			Level:         LogLevelWarn,
			Timestamp:     "2026-07-05T12:00:00.000Z",
			Message:       "possible issue",
		}

		if err := observation.Validate(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestLogObservationValidateRejectsInvalidFields(t *testing.T) {
	tests := []struct {
		name        string
		observation LogObservation
		wantErrPart string
	}{
		{
			name: "missing component",
			observation: LogObservation{
				Level:     LogLevelInfo,
				Timestamp: "2026-07-05T12:00:00Z",
				Message:   "ok",
			},
			wantErrPart: "component is required",
		},
		{
			name: "missing timestamp",
			observation: LogObservation{
				Component: "worker",
				Level:     LogLevelInfo,
				Message:   "ok",
			},
			wantErrPart: "timestamp is required",
		},
		{
			name: "invalid timestamp",
			observation: LogObservation{
				Component: "worker",
				Level:     LogLevelInfo,
				Timestamp: "2026-07-05 12:00:00",
				Message:   "ok",
			},
			wantErrPart: "invalid timestamp",
		},
		{
			name: "missing message",
			observation: LogObservation{
				Component: "worker",
				Level:     LogLevelInfo,
				Timestamp: "2026-07-05T12:00:00Z",
			},
			wantErrPart: "message is required",
		},
		{
			name: "unknown level",
			observation: LogObservation{
				Component: "worker",
				Level:     "verbose",
				Timestamp: "2026-07-05T12:00:00Z",
				Message:   "ok",
			},
			wantErrPart: "invalid level",
		},
		{
			name: "unknown stream",
			observation: LogObservation{
				Component: "worker",
				Stream:    "debug",
				Level:     LogLevelInfo,
				Timestamp: "2026-07-05T12:00:00Z",
				Message:   "ok",
			},
			wantErrPart: "invalid stream",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.observation.Validate(); err == nil {
				t.Fatal("expected validation error")
			} else if errText := err.Error(); !contains(errText, test.wantErrPart) {
				t.Fatalf("unexpected error = %q, want substring %q", err, test.wantErrPart)
			}
		})
	}
}

func TestLogObservationValidateRejectsUnsafeIDs(t *testing.T) {
	tests := []struct {
		name        string
		unsafeValue string
	}{
		{name: "submission_id contains slash", unsafeValue: "sub/../001"},
		{name: "attempt_id contains backslash", unsafeValue: "attempt\\001"},
		{name: "work_item_id includes traversal", unsafeValue: "../attempt-001"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			observation := LogObservation{
				Component:    "worker",
				Level:        LogLevelInfo,
				Stream:       "system",
				Timestamp:    "2026-07-05T12:00:00Z",
				Message:      "ok",
				SubmissionID: test.unsafeValue,
			}

			if err := observation.Validate(); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestLogObservationLevelHelpers(t *testing.T) {
	if v, err := CompareLogLevel("info", "warn"); err != nil || v >= 0 {
		t.Fatalf("expected info < warn, got %v, %v", v, err)
	}

	if !IsAtLeastLogLevel("error", "warn") {
		t.Fatalf("expected error >= warn")
	}

	if IsAtLeastLogLevel("info", "warn") {
		t.Fatalf("expected info < warn")
	}
}

func TestLogObservationJSONIncludesRoutingMetadata(t *testing.T) {
	observation := LogObservation{
		SubmissionID: "sub-001",
		WorkflowID:   "workflow-001",
		StepID:       "step-001",
		WorkItemID:   "work-item-001",
		AttemptID:    "attempt-001",
		WorkerID:     "worker-001",
		Component:    "worker",
		Level:        LogLevelInfo,
		Stream:       "stdout",
		Timestamp:    "2026-07-05T12:00:00Z",
		Message:      "message",
	}

	data, err := json.Marshal(observation)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded LogObservation
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.SubmissionID != observation.SubmissionID {
		t.Fatalf("submission_id = %q, want %q", decoded.SubmissionID, observation.SubmissionID)
	}
	if decoded.WorkItemID != observation.WorkItemID {
		t.Fatalf("work_item_id = %q, want %q", decoded.WorkItemID, observation.WorkItemID)
	}
}

func contains(haystack string, needle string) bool {
	return strings.Contains(haystack, needle)
}
