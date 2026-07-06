package model

import (
	"encoding/json"
	"fmt"
)

type WorkflowState string

const (
	WorkflowStateRunning   WorkflowState = "running"
	WorkflowStateCompleted WorkflowState = "completed"
	WorkflowStateFailed    WorkflowState = "failed"
)

func (state WorkflowState) Validate() error {
	switch state {
	case WorkflowStateRunning, WorkflowStateCompleted, WorkflowStateFailed:
		return nil
	default:
		return fmt.Errorf("unsupported workflow state: %s", state)
	}
}

type WorkflowStageState string

const (
	WorkflowStageStateBlocked   WorkflowStageState = "blocked"
	WorkflowStageStateReady     WorkflowStageState = "ready"
	WorkflowStageStateActive    WorkflowStageState = "active"
	WorkflowStageStateCompleted WorkflowStageState = "completed"
	WorkflowStageStateFailed    WorkflowStageState = "failed"
)

func (state WorkflowStageState) Validate() error {
	switch state {
	case WorkflowStageStateBlocked, WorkflowStageStateReady, WorkflowStageStateActive, WorkflowStageStateCompleted, WorkflowStageStateFailed:
		return nil
	default:
		return fmt.Errorf("unsupported workflow stage state: %s", state)
	}
}

type WorkflowStepState string

const (
	WorkflowStepStateBlocked   WorkflowStepState = "blocked"
	WorkflowStepStateReady     WorkflowStepState = "ready"
	WorkflowStepStateActive    WorkflowStepState = "active"
	WorkflowStepStateCompleted WorkflowStepState = "completed"
	WorkflowStepStateFailed    WorkflowStepState = "failed"
)

func (state WorkflowStepState) Validate() error {
	switch state {
	case WorkflowStepStateBlocked, WorkflowStepStateReady, WorkflowStepStateActive, WorkflowStepStateCompleted, WorkflowStepStateFailed:
		return nil
	default:
		return fmt.Errorf("unsupported workflow step state: %s", state)
	}
}

type WorkItemMembershipState string

const (
	WorkItemMembershipStateQueued    WorkItemMembershipState = "queued"
	WorkItemMembershipStateRunning   WorkItemMembershipState = "running"
	WorkItemMembershipStateCompleted WorkItemMembershipState = "completed"
	WorkItemMembershipStateFailed    WorkItemMembershipState = "failed"
	WorkItemMembershipStateSkipped   WorkItemMembershipState = "skipped"
)

func (state WorkItemMembershipState) Validate() error {
	switch state {
	case WorkItemMembershipStateQueued, WorkItemMembershipStateRunning, WorkItemMembershipStateCompleted, WorkItemMembershipStateFailed, WorkItemMembershipStateSkipped:
		return nil
	default:
		return fmt.Errorf("unsupported work item membership state: %s", state)
	}
}

type WorkflowDependencyWorkItemMembership struct {
	WorkItemID       string                  `json:"work_item_id"`
	WorkItemIndex    int                     `json:"work_item_index"`
	State            WorkItemMembershipState `json:"state"`
	OutputJSON       string                  `json:"output_json,omitempty"`
	OutputJSONSHA256 string                  `json:"output_json_sha256,omitempty"`
	OutputJSONBytes  int                     `json:"output_json_bytes,omitempty"`
	OutputJSONPruned bool                    `json:"output_json_pruned,omitempty"`
}

func (membership WorkflowDependencyWorkItemMembership) Validate() error {
	if membership.WorkItemID == "" {
		return fmt.Errorf("work item id is required")
	}
	if membership.WorkItemIndex < 0 {
		return fmt.Errorf("work item index must be non-negative")
	}
	if err := membership.State.Validate(); err != nil {
		return err
	}
	if membership.OutputJSON != "" && !json.Valid([]byte(membership.OutputJSON)) {
		return fmt.Errorf("output json must be valid JSON")
	}
	if membership.OutputJSONBytes < 0 {
		return fmt.Errorf("output json bytes must be non-negative")
	}
	return nil
}

type WorkflowDependencyStep struct {
	StageIndex       int                                    `json:"stage_index"`
	StepIndex        int                                    `json:"step_index"`
	StepID           string                                 `json:"step_id"`
	State            WorkflowStepState                      `json:"state"`
	OutputJSON       string                                 `json:"output_json,omitempty"`
	OutputJSONSHA256 string                                 `json:"output_json_sha256,omitempty"`
	OutputJSONBytes  int                                    `json:"output_json_bytes,omitempty"`
	OutputJSONPruned bool                                   `json:"output_json_pruned,omitempty"`
	WorkItems        []WorkflowDependencyWorkItemMembership `json:"work_items"`
}

func (step WorkflowDependencyStep) Validate() error {
	if step.StepID == "" {
		return fmt.Errorf("step id is required")
	}
	if step.StageIndex < 0 {
		return fmt.Errorf("stage index must be non-negative")
	}
	if step.StepIndex < 0 {
		return fmt.Errorf("step index must be non-negative")
	}
	if err := step.State.Validate(); err != nil {
		return err
	}
	if step.OutputJSON != "" && !json.Valid([]byte(step.OutputJSON)) {
		return fmt.Errorf("output json must be valid JSON")
	}
	if step.OutputJSONBytes < 0 {
		return fmt.Errorf("output json bytes must be non-negative")
	}
	for _, membership := range step.WorkItems {
		if err := membership.Validate(); err != nil {
			return fmt.Errorf("work item %s: %w", membership.WorkItemID, err)
		}
	}
	return nil
}

type WorkflowDependencyStage struct {
	StageIndex   int                      `json:"stage_index"`
	State        WorkflowStageState       `json:"state"`
	ParallelWith string                   `json:"parallel_with"`
	Steps        []WorkflowDependencyStep `json:"steps"`
}

func (stage WorkflowDependencyStage) Validate() error {
	if stage.StageIndex < 0 {
		return fmt.Errorf("stage index must be non-negative")
	}
	if err := stage.State.Validate(); err != nil {
		return err
	}
	for _, step := range stage.Steps {
		if err := step.Validate(); err != nil {
			return err
		}
	}
	return nil
}

type WorkflowDependencyPlan struct {
	RunID      string                    `json:"run_id"`
	WorkflowID string                    `json:"workflow_id"`
	State      WorkflowState             `json:"state"`
	Stages     []WorkflowDependencyStage `json:"stages"`
}

func (plan WorkflowDependencyPlan) Validate() error {
	if plan.RunID == "" {
		return fmt.Errorf("workflow run id is required")
	}
	if plan.WorkflowID == "" {
		return fmt.Errorf("workflow id is required")
	}
	if err := plan.State.Validate(); err != nil {
		return err
	}
	return nil
}
