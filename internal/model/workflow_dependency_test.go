package model

import "testing"

func TestWorkflowStateValidation(t *testing.T) {
	validStates := []WorkflowState{WorkflowStateRunning, WorkflowStateCompleted, WorkflowStateFailed}
	for _, state := range validStates {
		if err := state.Validate(); err != nil {
			t.Fatalf("workflow state %q validation error = %v", state, err)
		}
	}

	if err := WorkflowState("bad").Validate(); err == nil {
		t.Fatal("invalid workflow state should error")
	}
}

func TestWorkflowDependencyStateValidation(t *testing.T) {
	validStates := []WorkflowStageState{WorkflowStageStateBlocked, WorkflowStageStateReady, WorkflowStageStateActive, WorkflowStageStateCompleted, WorkflowStageStateFailed}
	for _, state := range validStates {
		if err := state.Validate(); err != nil {
			t.Fatalf("stage state %q validation error = %v", state, err)
		}
	}

	validStepStates := []WorkflowStepState{WorkflowStepStateBlocked, WorkflowStepStateReady, WorkflowStepStateActive, WorkflowStepStateCompleted, WorkflowStepStateFailed}
	for _, state := range validStepStates {
		if err := state.Validate(); err != nil {
			t.Fatalf("step state %q validation error = %v", state, err)
		}
	}

	validMembershipStates := []WorkItemMembershipState{
		WorkItemMembershipStateQueued,
		WorkItemMembershipStateRunning,
		WorkItemMembershipStateCompleted,
		WorkItemMembershipStateFailed,
		WorkItemMembershipStateSkipped,
	}
	for _, state := range validMembershipStates {
		if err := state.Validate(); err != nil {
			t.Fatalf("membership state %q validation error = %v", state, err)
		}
	}

	if err := WorkflowStageState("bad").Validate(); err == nil {
		t.Fatal("invalid stage state should error")
	}
	if err := WorkflowStepState("bad").Validate(); err == nil {
		t.Fatal("invalid step state should error")
	}
	if err := WorkItemMembershipState("bad").Validate(); err == nil {
		t.Fatal("invalid membership state should error")
	}
}

func TestWorkflowDependencyPlanValidateRejectsMissingRunOrWorkflowID(t *testing.T) {
	plan := WorkflowDependencyPlan{
		RunID:      "",
		WorkflowID: "demo",
		State:      WorkflowStateRunning,
	}
	if err := plan.Validate(); err == nil {
		t.Fatal("plan validation should require run id")
	}

	plan.RunID = "run-1"
	plan.WorkflowID = ""
	if err := plan.Validate(); err == nil {
		t.Fatal("plan validation should require workflow id")
	}
}

func TestWorkflowDependencyStepValidateRejectsBadData(t *testing.T) {
	step := WorkflowDependencyStep{
		StageIndex: 0,
		StepIndex:  0,
		StepID:     "write-demo",
		State:      WorkflowStepStateBlocked,
	}
	if err := step.Validate(); err != nil {
		t.Fatal("valid step should not error")
	}

	step.StepIndex = 0
	step.WorkItems = []WorkflowDependencyWorkItemMembership{
		{
			WorkItemID:    "item-1",
			WorkItemIndex: -1,
			State:         WorkItemMembershipStateQueued,
		},
	}
	if err := step.Validate(); err == nil {
		t.Fatal("negative work item index should be rejected")
	}
}
