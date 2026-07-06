package main

import (
	"context"
	"encoding/json"
	"testing"

	"goetl/internal/model"
	"goetl/internal/persistence"
	"goetl/internal/workflow"
)

func TestCreateWorkflowDependencyPlanPersistsDependencyState(t *testing.T) {
	store := openTestWorkflowExecutionStore(t)
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Fatalf("close workflow execution store: %v", err)
		}
	})
	run := insertTestPersistenceRunWithStage(t, context.Background(), store)
	controller := newController()
	controller.workflowStore = store

	stages := []workflow.WorkflowStage{
		{
			Index:        1,
			ParallelWith: "group-b",
			Steps: []workflow.WorkflowStageStep{
				{
					StageIndex: 1,
					StepIndex:  1,
					StepID:     "write-b",
				},
			},
		},
		{
			Index: 0,
			Steps: []workflow.WorkflowStageStep{
				{
					StageIndex: 0,
					StepIndex:  2,
					StepID:     "write-a",
				},
				{
					StageIndex: 0,
					StepIndex:  0,
					StepID:     "write-a2",
				},
			},
		},
	}

	if err := controller.CreateWorkflowDependencyPlan(context.Background(), run.ID, run.WorkflowID, stages); err != nil {
		t.Fatalf("CreateWorkflowDependencyPlan() error = %v", err)
	}

	orderedStages, err := controller.ListWorkflowStages(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("ListWorkflowStages() error = %v", err)
	}
	if len(orderedStages) != 2 {
		t.Fatalf("stage count = %d, want 2", len(orderedStages))
	}
	if orderedStages[0].StageIndex != 0 || orderedStages[1].StageIndex != 1 {
		t.Fatalf("unordered stages = %+v", orderedStages)
	}
	if orderedStages[0].State != model.WorkflowStageStateReady {
		t.Fatalf("first stage state = %q, want %q", orderedStages[0].State, model.WorkflowStageStateReady)
	}
	if orderedStages[1].State != model.WorkflowStageStateBlocked {
		t.Fatalf("second stage state = %q, want %q", orderedStages[1].State, model.WorkflowStageStateBlocked)
	}

	orderedSteps, err := controller.ListWorkflowSteps(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("ListWorkflowSteps() error = %v", err)
	}
	if len(orderedSteps) != 3 {
		t.Fatalf("step count = %d, want 3", len(orderedSteps))
	}
	if orderedSteps[0].StepIndex != 0 || orderedSteps[1].StepIndex != 2 || orderedSteps[2].StepIndex != 1 {
		t.Fatalf("unordered steps = %+v", orderedSteps)
	}
}

func TestCreateWorkflowDependencyPlanRejectsDuplicatePlan(t *testing.T) {
	store := openTestWorkflowExecutionStore(t)
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Fatalf("close workflow execution store: %v", err)
		}
	})
	run := insertTestPersistenceRunWithStage(t, context.Background(), store)
	controller := newController()
	controller.workflowStore = store

	stages := []workflow.WorkflowStage{
		{
			Index: 0,
			Steps: []workflow.WorkflowStageStep{{
				StageIndex: 0,
				StepIndex:  0,
				StepID:     "write-a",
			}},
		},
	}

	if err := controller.CreateWorkflowDependencyPlan(context.Background(), run.ID, run.WorkflowID, stages); err != nil {
		t.Fatalf("CreateWorkflowDependencyPlan() error = %v", err)
	}
	if err := controller.CreateWorkflowDependencyPlan(context.Background(), run.ID, run.WorkflowID, stages); err == nil {
		t.Fatal("CreateWorkflowDependencyPlan() expected duplicate error, got nil")
	}
}

func TestRecordCompiledWorkItemMembershipAndReadState(t *testing.T) {
	store := openTestWorkflowExecutionStore(t)
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Fatalf("close workflow execution store: %v", err)
		}
	})
	run := insertTestPersistenceRunWithStage(t, context.Background(), store)
	controller := newController()
	controller.workflowStore = store

	stages := []workflow.WorkflowStage{
		{
			Index: 0,
			Steps: []workflow.WorkflowStageStep{
				{StageIndex: 0, StepIndex: 0, StepID: "write-a"},
			},
		},
	}
	ctx := context.Background()
	if err := controller.CreateWorkflowDependencyPlan(ctx, run.ID, run.WorkflowID, stages); err != nil {
		t.Fatalf("CreateWorkflowDependencyPlan() error = %v", err)
	}

	if err := controller.RecordCompiledWorkItemMembership(ctx, run.ID, 0, 0, "workitem-001", 7); err != nil {
		t.Fatalf("RecordCompiledWorkItemMembership() error = %v", err)
	}
	if err := controller.RecordCompiledWorkItemMembership(ctx, run.ID, 0, 0, "workitem-001", 3); err == nil {
		t.Fatal("RecordCompiledWorkItemMembership() expected duplicate work item error, got nil")
	}
	if err := controller.RecordCompiledWorkItemMembership(ctx, run.ID, 0, 0, "workitem-002", 7); err == nil {
		t.Fatal("RecordCompiledWorkItemMembership() expected duplicate work item index error, got nil")
	}

	step, found, err := controller.ReadStepState(ctx, run.ID, 0)
	if err != nil {
		t.Fatalf("ReadStepState() error = %v", err)
	}
	if !found {
		t.Fatal("step state missing for existing plan")
	}
	if len(step.WorkItems) != 1 {
		t.Fatalf("work item membership count = %d, want 1", len(step.WorkItems))
	}
	if step.WorkItems[0].WorkItemID != "workitem-001" || step.WorkItems[0].WorkItemIndex != 7 || step.WorkItems[0].State != model.WorkItemMembershipStateQueued {
		t.Fatalf("work item membership = %+v, want id=workitem-001 index=7 state=queued", step.WorkItems[0])
	}

	stage, found, err := controller.ReadStageState(ctx, run.ID, 0)
	if err != nil {
		t.Fatalf("ReadStageState() error = %v", err)
	}
	if !found {
		t.Fatal("stage state missing for existing plan")
	}
	if stage.State != model.WorkflowStageStateReady {
		t.Fatalf("stage state = %q, want %q", stage.State, model.WorkflowStageStateReady)
	}
}

func TestListWorkflowStateMethodsReturnNotFoundForMissingPlan(t *testing.T) {
	store := openTestWorkflowExecutionStore(t)
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Fatalf("close workflow execution store: %v", err)
		}
	})
	run := insertTestPersistenceRunWithStage(t, context.Background(), store)
	controller := newController()
	controller.workflowStore = store

	if _, err := controller.ListWorkflowStages(context.Background(), run.ID); err == nil {
		t.Fatal("ListWorkflowStages() expected dependency plan error, got nil")
	}
	if _, err := controller.ListWorkflowSteps(context.Background(), run.ID); err == nil {
		t.Fatal("ListWorkflowSteps() expected dependency plan error, got nil")
	}
	if _, _, err := controller.ReadStepState(context.Background(), run.ID, 0); err != nil {
		t.Fatal("ReadStepState() should not error for missing plan")
	}
	if _, _, err := controller.ReadStageState(context.Background(), run.ID, 0); err != nil {
		t.Fatal("ReadStageState() should not error for missing plan")
	}
}

func TestCreatePlanRejectsInvalidStateInStoredContext(t *testing.T) {
	store := openTestWorkflowExecutionStore(t)
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Fatalf("close workflow execution store: %v", err)
		}
	})
	run := insertTestPersistenceRunWithStage(t, context.Background(), store)

	invalid := `{"schema":"goet/workflow-run-submission-context/v1","variables":[],"dependency_state":{"run_id":"` + run.ID + `","workflow_id":"` + run.WorkflowID + `","state":"bad","stages":[]}}`
	if err := store.UpdateWorkflowRunSubmissionContext(context.Background(), run.ID, invalid); err != nil {
		t.Fatalf("UpdateWorkflowRunSubmissionContext() error = %v", err)
	}

	controller := newController()
	controller.workflowStore = store
	if _, err := controller.ListWorkflowStages(context.Background(), run.ID); err == nil {
		t.Fatal("ListWorkflowStages() should reject invalid stored state")
	}
}

func TestRecordWorkItemTerminalStateAdvancesStepAndStageCompletion(t *testing.T) {
	store := openTestWorkflowExecutionStore(t)
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Fatalf("close workflow execution store: %v", err)
		}
	})
	run := insertTestPersistenceRunWithStage(t, context.Background(), store)
	controller := newController()
	controller.workflowStore = store
	ctx := context.Background()

	stages := []workflow.WorkflowStage{
		{
			Index: 0,
			Steps: []workflow.WorkflowStageStep{
				{
					StageIndex: 0,
					StepIndex:  0,
					StepID:     "write-a",
				},
				{
					StageIndex: 0,
					StepIndex:  1,
					StepID:     "write-b",
				},
			},
		},
	}
	if err := controller.CreateWorkflowDependencyPlan(ctx, run.ID, run.WorkflowID, stages); err != nil {
		t.Fatalf("CreateWorkflowDependencyPlan() error = %v", err)
	}
	workItems := []persistence.WorkItemRecord{
		testPersistenceWorkItem("workitem-a", run.ID, 0, 0),
		testPersistenceWorkItem("workitem-b", run.ID, 0, 1),
	}
	if err := store.InsertWorkItems(ctx, workItems); err != nil {
		t.Fatalf("InsertWorkItems() error = %v", err)
	}
	if err := controller.RecordCompiledWorkItemMembership(ctx, run.ID, 0, 0, "workitem-a", 0); err != nil {
		t.Fatalf("RecordCompiledWorkItemMembership(a) error = %v", err)
	}
	if err := controller.RecordCompiledWorkItemMembership(ctx, run.ID, 0, 1, "workitem-b", 1); err != nil {
		t.Fatalf("RecordCompiledWorkItemMembership(b) error = %v", err)
	}
	if err := controller.RecordWorkItemTerminalState(ctx, run.ID, "workitem-a", model.WorkItemMembershipStateCompleted); err != nil {
		t.Fatalf("RecordWorkItemTerminalState(a) error = %v", err)
	}

	step0, found, err := controller.ReadStepState(ctx, run.ID, 0)
	if err != nil {
		t.Fatalf("ReadStepState(0) error = %v", err)
	}
	if !found {
		t.Fatal("step 0 not found after terminal update")
	}
	if step0.State != model.WorkflowStepStateCompleted {
		t.Fatalf("step 0 state = %q, want %q", step0.State, model.WorkflowStepStateCompleted)
	}
	if step0.WorkItems[0].State != model.WorkItemMembershipStateCompleted {
		t.Fatalf("workitem-a state = %q, want %q", step0.WorkItems[0].State, model.WorkItemMembershipStateCompleted)
	}

	stage0, found, err := controller.ReadStageState(ctx, run.ID, 0)
	if err != nil {
		t.Fatalf("ReadStageState(0) error = %v", err)
	}
	if !found {
		t.Fatal("stage 0 not found after terminal update")
	}
	if stage0.State != model.WorkflowStageStateActive {
		t.Fatalf("stage 0 state = %q, want %q", stage0.State, model.WorkflowStageStateActive)
	}

	if err := controller.RecordWorkItemTerminalState(ctx, run.ID, "workitem-b", model.WorkItemMembershipStateCompleted); err != nil {
		t.Fatalf("RecordWorkItemTerminalState(b) error = %v", err)
	}
	stage0, found, err = controller.ReadStageState(ctx, run.ID, 0)
	if err != nil {
		t.Fatalf("ReadStageState(0) error = %v", err)
	}
	if !found {
		t.Fatal("stage 0 not found after second terminal update")
	}
	if stage0.State != model.WorkflowStageStateCompleted {
		t.Fatalf("stage 0 state = %q, want %q", stage0.State, model.WorkflowStageStateCompleted)
	}

	runRecord, found, err := store.GetWorkflowRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetWorkflowRun() error = %v", err)
	}
	if !found {
		t.Fatal("workflow run missing after terminal updates")
	}
	var context workflowRunSubmissionContext
	if err := json.Unmarshal([]byte(runRecord.SubmissionContextJSON), &context); err != nil {
		t.Fatalf("decode submission context: %v", err)
	}
	if context.DependencyState == nil {
		t.Fatal("dependency state missing from submission context")
	}
	if context.DependencyState.State != model.WorkflowStateCompleted {
		t.Fatalf("dependency workflow state = %q, want %q", context.DependencyState.State, model.WorkflowStateCompleted)
	}
}

func TestRecordWorkItemTerminalStateIsIdempotent(t *testing.T) {
	store := openTestWorkflowExecutionStore(t)
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Fatalf("close workflow execution store: %v", err)
		}
	})
	run := insertTestPersistenceRunWithStage(t, context.Background(), store)
	controller := newController()
	controller.workflowStore = store
	ctx := context.Background()

	stages := []workflow.WorkflowStage{
		{
			Index: 0,
			Steps: []workflow.WorkflowStageStep{
				{
					StageIndex: 0,
					StepIndex:  0,
					StepID:     "write-a",
				},
			},
		},
	}
	if err := controller.CreateWorkflowDependencyPlan(ctx, run.ID, run.WorkflowID, stages); err != nil {
		t.Fatalf("CreateWorkflowDependencyPlan() error = %v", err)
	}
	if err := store.InsertWorkItems(ctx, []persistence.WorkItemRecord{
		testPersistenceWorkItem("workitem-a", run.ID, 0, 0),
		testPersistenceWorkItem("workitem-b", run.ID, 0, 1),
	}); err != nil {
		t.Fatalf("InsertWorkItems() error = %v", err)
	}
	if err := controller.RecordCompiledWorkItemMembership(ctx, run.ID, 0, 0, "workitem-a", 0); err != nil {
		t.Fatalf("RecordCompiledWorkItemMembership(a) error = %v", err)
	}
	if err := controller.RecordCompiledWorkItemMembership(ctx, run.ID, 0, 0, "workitem-b", 1); err != nil {
		t.Fatalf("RecordCompiledWorkItemMembership(b) error = %v", err)
	}
	if err := controller.RecordWorkItemTerminalState(ctx, run.ID, "workitem-a", model.WorkItemMembershipStateCompleted); err != nil {
		t.Fatalf("RecordWorkItemTerminalState(first) error = %v", err)
	}
	step, found, err := controller.ReadStepState(ctx, run.ID, 0)
	if err != nil {
		t.Fatalf("ReadStepState() error = %v", err)
	}
	if !found {
		t.Fatal("step not found after first terminal update")
	}
	if step.State != model.WorkflowStepStateActive {
		t.Fatalf("step state after first terminal = %q, want %q", step.State, model.WorkflowStepStateActive)
	}

	if err := controller.RecordWorkItemTerminalState(ctx, run.ID, "workitem-a", model.WorkItemMembershipStateCompleted); err != nil {
		t.Fatalf("RecordWorkItemTerminalState(duplicate) error = %v", err)
	}
	stepAfterDuplicate, found, err := controller.ReadStepState(ctx, run.ID, 0)
	if err != nil {
		t.Fatalf("ReadStepState() error = %v", err)
	}
	if !found {
		t.Fatal("step not found after duplicate terminal update")
	}
	if stepAfterDuplicate.State != model.WorkflowStepStateActive {
		t.Fatalf("step state after duplicate terminal = %q, want %q", stepAfterDuplicate.State, model.WorkflowStepStateActive)
	}
}

func TestRecordWorkItemTerminalStateMarksStepAndStageFailed(t *testing.T) {
	store := openTestWorkflowExecutionStore(t)
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Fatalf("close workflow execution store: %v", err)
		}
	})
	run := insertTestPersistenceRunWithStage(t, context.Background(), store)
	controller := newController()
	controller.workflowStore = store
	ctx := context.Background()

	stages := []workflow.WorkflowStage{
		{
			Index: 0,
			Steps: []workflow.WorkflowStageStep{
				{
					StageIndex: 0,
					StepIndex:  0,
					StepID:     "write-a",
				},
			},
		},
	}
	if err := controller.CreateWorkflowDependencyPlan(ctx, run.ID, run.WorkflowID, stages); err != nil {
		t.Fatalf("CreateWorkflowDependencyPlan() error = %v", err)
	}
	if err := store.InsertWorkItems(ctx, []persistence.WorkItemRecord{
		testPersistenceWorkItem("workitem-failed", run.ID, 0, 0),
	}); err != nil {
		t.Fatalf("InsertWorkItems() error = %v", err)
	}
	if err := controller.RecordCompiledWorkItemMembership(ctx, run.ID, 0, 0, "workitem-failed", 0); err != nil {
		t.Fatalf("RecordCompiledWorkItemMembership() error = %v", err)
	}
	if err := controller.RecordWorkItemTerminalState(ctx, run.ID, "workitem-failed", model.WorkItemMembershipStateFailed); err != nil {
		t.Fatalf("RecordWorkItemTerminalState() error = %v", err)
	}

	step, found, err := controller.ReadStepState(ctx, run.ID, 0)
	if err != nil {
		t.Fatalf("ReadStepState() error = %v", err)
	}
	if !found {
		t.Fatal("step not found after failure")
	}
	if step.State != model.WorkflowStepStateFailed {
		t.Fatalf("step state = %q, want %q", step.State, model.WorkflowStepStateFailed)
	}
	if step.WorkItems[0].State != model.WorkItemMembershipStateFailed {
		t.Fatalf("workitem state = %q, want %q", step.WorkItems[0].State, model.WorkItemMembershipStateFailed)
	}

	stage, found, err := controller.ReadStageState(ctx, run.ID, 0)
	if err != nil {
		t.Fatalf("ReadStageState() error = %v", err)
	}
	if !found {
		t.Fatal("stage not found after failure")
	}
	if stage.State != model.WorkflowStageStateFailed {
		t.Fatalf("stage state = %q, want %q", stage.State, model.WorkflowStageStateFailed)
	}
}

func TestReadWorkflowRunSubmissionContextKeepsDependencyStateJSONShape(t *testing.T) {
	store := openTestWorkflowExecutionStore(t)
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Fatalf("close workflow execution store: %v", err)
		}
	})
	run := insertTestPersistenceRunWithStage(t, context.Background(), store)
	controller := newController()
	controller.workflowStore = store

	stages := []workflow.WorkflowStage{
		{
			Index: 0,
			Steps: []workflow.WorkflowStageStep{
				{StageIndex: 0, StepIndex: 0, StepID: "write-a"},
			},
		},
	}
	ctx := context.Background()
	if err := controller.CreateWorkflowDependencyPlan(ctx, run.ID, run.WorkflowID, stages); err != nil {
		t.Fatalf("CreateWorkflowDependencyPlan() error = %v", err)
	}

	runRecord, found, err := store.GetWorkflowRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetWorkflowRun() error = %v", err)
	}
	if !found {
		t.Fatal("created run missing")
	}

	var contextJSON struct {
		Schema     string `json:"schema"`
		Dependency *struct {
			RunID      string `json:"run_id"`
			WorkflowID string `json:"workflow_id"`
		} `json:"dependency_state"`
	}
	if err := json.Unmarshal([]byte(runRecord.SubmissionContextJSON), &contextJSON); err != nil {
		t.Fatalf("unmarshal run submission context: %v", err)
	}
	if contextJSON.Dependency == nil {
		t.Fatal("dependency state not found in submission context")
	}
	if contextJSON.Dependency.RunID != run.ID {
		t.Fatalf("dependency run id = %q, want %q", contextJSON.Dependency.RunID, run.ID)
	}
}
