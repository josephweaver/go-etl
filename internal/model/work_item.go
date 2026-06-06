package model

import (
	"fmt"
	"path/filepath"
)

type WorkItemType string

const (
	WorkItemTypeWriteDemoOutput WorkItemType = "write_demo_output"
)

type WorkItem struct {
	ID             string       `json:"id"`
	Type           WorkItemType `json:"type"`
	OutputFilename string       `json:"output_filename"`
}

type WorkCompletion struct {
	ID                  string `json:"id"`
	AttemptID           string `json:"attempt_id,omitempty"`
	WorkflowInstanceID  string `json:"workflow_instance_id,omitempty"`
	StepInstanceID      string `json:"step_instance_id,omitempty"`
	WorkItemFingerprint string `json:"work_item_fingerprint,omitempty"`
	InputFingerprint    string `json:"input_fingerprint,omitempty"`
	OutputFingerprint   string `json:"output_fingerprint,omitempty"`
	CodeVersion         string `json:"code_version,omitempty"`
	StartedAt           string `json:"started_at,omitempty"`
	CompletedAt         string `json:"completed_at,omitempty"`
}

type WorkFailure struct {
	ID    string `json:"id"`
	Error string `json:"error"`
}

type ControllerStatus struct {
	Pending  int `json:"pending"`
	Assigned int `json:"assigned"`
	Failed   int `json:"failed"`
}

func (item WorkItem) Validate() error {
	if item.ID == "" {
		return fmt.Errorf("work item id is required")
	}

	if item.Type == "" {
		return fmt.Errorf("work item type is required")
	}

	if item.OutputFilename == "" {
		return fmt.Errorf("output filename is required")
	}

	if filepath.Base(item.OutputFilename) != item.OutputFilename {
		return fmt.Errorf("output filename must not contain a directory: %s", item.OutputFilename)
	}

	return nil
}
