package main

import (
	"context"
	"encoding/json"
	"strings"
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

func TestRecordCompletedWorkItemOutputPrunesMembershipOutputAfterStepAggregation(t *testing.T) {
	store := openTestWorkflowExecutionStore(t)
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Fatalf("close workflow execution store: %v", err)
		}
	})
	controller := newController()
	controller.workflowStore = store
	ctx := context.Background()
	run := insertTestPersistenceRunWithStage(t, ctx, store)
	createSingleStepDependencyPlan(t, ctx, controller, run.ID, run.WorkflowID, "work-0")

	if err := controller.RecordCompletedWorkItemOutput(ctx, RecordCompletedWorkItemOutputRequest{
		SubmissionID: run.ID,
		WorkItemID:   "work-0",
		OutputJSON:   `{"label":"done","answer":42}`,
	}); err != nil {
		t.Fatalf("RecordCompletedWorkItemOutput() error = %v", err)
	}

	step, found, err := controller.ReadStepState(ctx, run.ID, 0)
	if err != nil {
		t.Fatalf("ReadStepState() error = %v", err)
	}
	if !found {
		t.Fatal("step not found after output capture")
	}
	if step.WorkItems[0].OutputJSON != "" {
		t.Fatalf("membership output = %s, want pruned", step.WorkItems[0].OutputJSON)
	}
	if step.WorkItems[0].OutputJSONSHA256 == "" {
		t.Fatal("membership output hash was not persisted")
	}
	if step.WorkItems[0].OutputJSONBytes != len([]byte(`{"answer":42,"label":"done"}`)) || !step.WorkItems[0].OutputJSONPruned {
		t.Fatalf("membership pruning metadata = %+v, want bytes/pruned", step.WorkItems[0])
	}
}

func TestRecordCompletedWorkItemOutputPersistsAggregatedStepOutput(t *testing.T) {
	store := openTestWorkflowExecutionStore(t)
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Fatalf("close workflow execution store: %v", err)
		}
	})
	controller := newController()
	controller.workflowStore = store
	ctx := context.Background()
	run := insertTestPersistenceRunWithStage(t, ctx, store)
	createTwoStageDependencyPlan(t, ctx, controller, run.ID, run.WorkflowID, "work-0")

	if err := controller.RecordCompletedWorkItemOutput(ctx, RecordCompletedWorkItemOutputRequest{
		SubmissionID: run.ID,
		WorkItemID:   "work-0",
		OutputJSON:   `{"value":"a"}`,
	}); err != nil {
		t.Fatalf("RecordCompletedWorkItemOutput() error = %v", err)
	}

	reloaded := newController()
	reloaded.workflowStore = store
	step, found, err := reloaded.ReadStepState(ctx, run.ID, 0)
	if err != nil {
		t.Fatalf("ReadStepState() error = %v", err)
	}
	if !found {
		t.Fatal("step not found after reload")
	}
	if step.State != model.WorkflowStepStateCompleted {
		t.Fatalf("step state = %q, want completed", step.State)
	}
	if step.OutputJSON != `{"value":"a"}` || step.OutputJSONSHA256 == "" || step.OutputJSONBytes != len([]byte(`{"value":"a"}`)) || step.OutputJSONPruned {
		t.Fatalf("step output = %+v, want retained aggregate with metadata", step)
	}
}

func TestRecordCompletedWorkItemOutputLeavesWorkflowStageOutputUnused(t *testing.T) {
	store := openTestWorkflowExecutionStore(t)
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Fatalf("close workflow execution store: %v", err)
		}
	})
	controller := newController()
	controller.workflowStore = store
	ctx := context.Background()
	run := insertTestPersistenceRunWithStage(t, ctx, store)
	createTwoStageDependencyPlan(t, ctx, controller, run.ID, run.WorkflowID, "work-0")

	if err := controller.RecordCompletedWorkItemOutput(ctx, RecordCompletedWorkItemOutputRequest{
		SubmissionID: run.ID,
		WorkItemID:   "work-0",
		OutputJSON:   `{"value":"step-output"}`,
	}); err != nil {
		t.Fatalf("RecordCompletedWorkItemOutput() error = %v", err)
	}

	stage, found, err := store.GetWorkflowStage(ctx, run.ID, 0)
	if err != nil {
		t.Fatalf("GetWorkflowStage() error = %v", err)
	}
	if !found {
		t.Fatal("workflow stage missing")
	}
	if stage.OutputJSON != "" || stage.OutputJSONSHA256 != "" {
		t.Fatalf("stage output = %q hash=%q, want unused", stage.OutputJSON, stage.OutputJSONSHA256)
	}
}

func TestRecordCompletedWorkItemOutputKeepsFanoutOrderStable(t *testing.T) {
	store := openTestWorkflowExecutionStore(t)
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Fatalf("close workflow execution store: %v", err)
		}
	})
	controller := newController()
	controller.workflowStore = store
	ctx := context.Background()
	run := insertTestPersistenceRunWithStage(t, ctx, store)
	createTwoStageFanoutDependencyPlan(t, ctx, controller, run.ID, run.WorkflowID, []string{"work-0", "work-1", "work-2"})

	for _, item := range []struct {
		id     string
		output string
	}{
		{id: "work-2", output: `{"value":"c"}`},
		{id: "work-0", output: `{"value":"a"}`},
		{id: "work-1", output: `{"value":"b"}`},
	} {
		if err := controller.RecordCompletedWorkItemOutput(ctx, RecordCompletedWorkItemOutputRequest{
			SubmissionID: run.ID,
			WorkItemID:   item.id,
			OutputJSON:   item.output,
		}); err != nil {
			t.Fatalf("RecordCompletedWorkItemOutput(%s) error = %v", item.id, err)
		}
	}

	step, found, err := controller.ReadStepState(ctx, run.ID, 0)
	if err != nil {
		t.Fatalf("ReadStepState() error = %v", err)
	}
	if !found {
		t.Fatal("step not found after fanout outputs")
	}
	if step.OutputJSON != `[{"value":"a"},{"value":"b"},{"value":"c"}]` {
		t.Fatalf("step output = %s, want work-item-index order", step.OutputJSON)
	}
}

func TestRecordCompletedWorkItemOutputDoesNotLeakAcrossSubmissions(t *testing.T) {
	store := openTestWorkflowExecutionStore(t)
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Fatalf("close workflow execution store: %v", err)
		}
	})
	controller := newController()
	controller.workflowStore = store
	ctx := context.Background()
	run1 := insertTestPersistenceRunWithID(t, ctx, store, "run-1")
	run2 := insertTestPersistenceRunWithID(t, ctx, store, "run-2")
	createSingleStepDependencyPlan(t, ctx, controller, run1.ID, run1.WorkflowID, "shared-work")
	createSingleStepDependencyPlan(t, ctx, controller, run2.ID, run2.WorkflowID, "shared-work")

	if err := controller.RecordCompletedWorkItemOutput(ctx, RecordCompletedWorkItemOutputRequest{
		SubmissionID: run1.ID,
		WorkItemID:   "shared-work",
		OutputJSON:   `{"answer":1}`,
	}); err != nil {
		t.Fatalf("RecordCompletedWorkItemOutput() error = %v", err)
	}

	step1, found, err := controller.ReadStepState(ctx, run1.ID, 0)
	if err != nil || !found {
		t.Fatalf("ReadStepState(run1) found=%v error=%v", found, err)
	}
	step2, found, err := controller.ReadStepState(ctx, run2.ID, 0)
	if err != nil || !found {
		t.Fatalf("ReadStepState(run2) found=%v error=%v", found, err)
	}
	if step1.OutputJSONSHA256 == "" || !step1.OutputJSONPruned {
		t.Fatalf("run1 output metadata = %+v, want pruned recorded output", step1)
	}
	if step2.OutputJSON != "" || step2.WorkItems[0].OutputJSON != "" {
		t.Fatalf("run2 leaked output: step=%q item=%q", step2.OutputJSON, step2.WorkItems[0].OutputJSON)
	}
}

func TestRecordCompletedWorkItemOutputRejectsUnknownWorkItem(t *testing.T) {
	store := openTestWorkflowExecutionStore(t)
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Fatalf("close workflow execution store: %v", err)
		}
	})
	controller := newController()
	controller.workflowStore = store
	ctx := context.Background()
	run := insertTestPersistenceRunWithStage(t, ctx, store)
	createSingleStepDependencyPlan(t, ctx, controller, run.ID, run.WorkflowID, "work-0")

	if err := controller.RecordCompletedWorkItemOutput(ctx, RecordCompletedWorkItemOutputRequest{
		SubmissionID: run.ID,
		WorkItemID:   "missing",
		OutputJSON:   `{"value":"a"}`,
	}); err == nil {
		t.Fatal("RecordCompletedWorkItemOutput() expected unknown work-item error")
	}
}

func TestRecordCompletedWorkItemOutputRejectsInvalidOutputJSON(t *testing.T) {
	store := openTestWorkflowExecutionStore(t)
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Fatalf("close workflow execution store: %v", err)
		}
	})
	controller := newController()
	controller.workflowStore = store
	ctx := context.Background()
	run := insertTestPersistenceRunWithStage(t, ctx, store)
	createSingleStepDependencyPlan(t, ctx, controller, run.ID, run.WorkflowID, "work-0")

	if err := controller.RecordCompletedWorkItemOutput(ctx, RecordCompletedWorkItemOutputRequest{
		SubmissionID: run.ID,
		WorkItemID:   "work-0",
		OutputJSON:   `{"value": null}`,
	}); err == nil {
		t.Fatal("RecordCompletedWorkItemOutput() expected invalid output error")
	}
}

func TestRecordCompletedWorkItemOutputRejectsOversizedOutputJSON(t *testing.T) {
	store := openTestWorkflowExecutionStore(t)
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Fatalf("close workflow execution store: %v", err)
		}
	})
	controller := newController()
	controller.workflowStore = store
	ctx := context.Background()
	run := insertTestPersistenceRunWithStage(t, ctx, store)
	createSingleStepDependencyPlan(t, ctx, controller, run.ID, run.WorkflowID, "work-0")

	output := `{"log":"` + strings.Repeat("x", maxCompletedWorkOutputJSONBytes) + `"}`
	err := controller.RecordCompletedWorkItemOutput(ctx, RecordCompletedWorkItemOutputRequest{
		SubmissionID: run.ID,
		WorkItemID:   "work-0",
		OutputJSON:   output,
	})
	if err == nil {
		t.Fatal("RecordCompletedWorkItemOutput() expected oversized output error")
	}
	assertOversizedOutputError(t, err, len([]byte(output)), maxCompletedWorkOutputJSONBytes)
}

func TestTerminalWorkflowPruningClearsAllDependencyOutputJSON(t *testing.T) {
	store := openTestWorkflowExecutionStore(t)
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Fatalf("close workflow execution store: %v", err)
		}
	})
	controller := newController()
	controller.workflowStore = store
	ctx := context.Background()
	run := insertTestPersistenceRunWithStage(t, ctx, store)
	createSingleStepDependencyPlan(t, ctx, controller, run.ID, run.WorkflowID, "work-0")

	if err := controller.RecordCompletedWorkItemOutput(ctx, RecordCompletedWorkItemOutputRequest{
		SubmissionID: run.ID,
		WorkItemID:   "work-0",
		OutputJSON:   `{"value":"terminal"}`,
	}); err != nil {
		t.Fatalf("RecordCompletedWorkItemOutput() error = %v", err)
	}

	step, found, err := controller.ReadStepState(ctx, run.ID, 0)
	if err != nil {
		t.Fatalf("ReadStepState() error = %v", err)
	}
	if !found {
		t.Fatal("step not found after terminal prune")
	}
	if step.OutputJSON != "" || !step.OutputJSONPruned || step.OutputJSONSHA256 == "" || step.OutputJSONBytes != len([]byte(`{"value":"terminal"}`)) {
		t.Fatalf("terminal step pruning = %+v, want pruned output with metadata", step)
	}
	item := step.WorkItems[0]
	if item.OutputJSON != "" || !item.OutputJSONPruned || item.OutputJSONSHA256 == "" || item.OutputJSONBytes != len([]byte(`{"value":"terminal"}`)) {
		t.Fatalf("terminal membership pruning = %+v, want pruned output with metadata", item)
	}
}

func TestTerminalWorkflowPruningRunsOnFailure(t *testing.T) {
	store := openTestWorkflowExecutionStore(t)
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Fatalf("close workflow execution store: %v", err)
		}
	})
	controller := newController()
	controller.workflowStore = store
	ctx := context.Background()
	run := insertTestPersistenceRunWithStage(t, ctx, store)
	createTwoStageDependencyPlan(t, ctx, controller, run.ID, run.WorkflowID, "work-0")
	if err := controller.RecordCompiledWorkItemMembership(ctx, run.ID, 1, 1, "work-1", 0); err != nil {
		t.Fatalf("RecordCompiledWorkItemMembership(work-1) error = %v", err)
	}

	if err := controller.RecordCompletedWorkItemOutput(ctx, RecordCompletedWorkItemOutputRequest{
		SubmissionID: run.ID,
		WorkItemID:   "work-0",
		OutputJSON:   `{"value":"needed-until-failure"}`,
	}); err != nil {
		t.Fatalf("RecordCompletedWorkItemOutput() error = %v", err)
	}
	step0, found, err := controller.ReadStepState(ctx, run.ID, 0)
	if err != nil || !found {
		t.Fatalf("ReadStepState(before failure) found=%v error=%v", found, err)
	}
	if step0.OutputJSON == "" {
		t.Fatal("step 0 output should be retained before terminal failure")
	}

	if err := controller.RecordWorkItemTerminalState(ctx, run.ID, "work-1", model.WorkItemMembershipStateFailed); err != nil {
		t.Fatalf("RecordWorkItemTerminalState(failed) error = %v", err)
	}
	step0, found, err = controller.ReadStepState(ctx, run.ID, 0)
	if err != nil || !found {
		t.Fatalf("ReadStepState(after failure) found=%v error=%v", found, err)
	}
	if step0.OutputJSON != "" || !step0.OutputJSONPruned || step0.OutputJSONSHA256 == "" {
		t.Fatalf("step 0 output after failure = %+v, want pruned with hash", step0)
	}
}

func createSingleStepDependencyPlan(t *testing.T, ctx context.Context, controller *Controller, runID string, workflowID string, workItemID string) {
	t.Helper()

	if err := controller.CreateWorkflowDependencyPlan(ctx, runID, workflowID, []workflow.WorkflowStage{{
		Index: 0,
		Steps: []workflow.WorkflowStageStep{{
			StageIndex: 0,
			StepIndex:  0,
			StepID:     "step-0",
		}},
	}}); err != nil {
		t.Fatalf("CreateWorkflowDependencyPlan() error = %v", err)
	}
	if err := controller.RecordCompiledWorkItemMembership(ctx, runID, 0, 0, workItemID, 0); err != nil {
		t.Fatalf("RecordCompiledWorkItemMembership() error = %v", err)
	}
}

func createTwoStageDependencyPlan(t *testing.T, ctx context.Context, controller *Controller, runID string, workflowID string, workItemID string) {
	t.Helper()

	if err := controller.CreateWorkflowDependencyPlan(ctx, runID, workflowID, []workflow.WorkflowStage{
		{
			Index: 0,
			Steps: []workflow.WorkflowStageStep{{
				StageIndex: 0,
				StepIndex:  0,
				StepID:     "step-0",
			}},
		},
		{
			Index: 1,
			Steps: []workflow.WorkflowStageStep{{
				StageIndex: 1,
				StepIndex:  1,
				StepID:     "step-1",
			}},
		},
	}); err != nil {
		t.Fatalf("CreateWorkflowDependencyPlan() error = %v", err)
	}
	if err := controller.RecordCompiledWorkItemMembership(ctx, runID, 0, 0, workItemID, 0); err != nil {
		t.Fatalf("RecordCompiledWorkItemMembership() error = %v", err)
	}
}

func createFanoutDependencyPlan(t *testing.T, ctx context.Context, controller *Controller, runID string, workflowID string, workItemIDs []string) {
	t.Helper()

	if err := controller.CreateWorkflowDependencyPlan(ctx, runID, workflowID, []workflow.WorkflowStage{{
		Index: 0,
		Steps: []workflow.WorkflowStageStep{{
			StageIndex: 0,
			StepIndex:  0,
			StepID:     "step-0",
		}},
	}}); err != nil {
		t.Fatalf("CreateWorkflowDependencyPlan() error = %v", err)
	}
	for index, id := range workItemIDs {
		if err := controller.RecordCompiledWorkItemMembership(ctx, runID, 0, 0, id, index); err != nil {
			t.Fatalf("RecordCompiledWorkItemMembership(%s) error = %v", id, err)
		}
	}
}

func createTwoStageFanoutDependencyPlan(t *testing.T, ctx context.Context, controller *Controller, runID string, workflowID string, workItemIDs []string) {
	t.Helper()

	if err := controller.CreateWorkflowDependencyPlan(ctx, runID, workflowID, []workflow.WorkflowStage{
		{
			Index: 0,
			Steps: []workflow.WorkflowStageStep{{
				StageIndex: 0,
				StepIndex:  0,
				StepID:     "step-0",
			}},
		},
		{
			Index: 1,
			Steps: []workflow.WorkflowStageStep{{
				StageIndex: 1,
				StepIndex:  1,
				StepID:     "step-1",
			}},
		},
	}); err != nil {
		t.Fatalf("CreateWorkflowDependencyPlan() error = %v", err)
	}
	for index, id := range workItemIDs {
		if err := controller.RecordCompiledWorkItemMembership(ctx, runID, 0, 0, id, index); err != nil {
			t.Fatalf("RecordCompiledWorkItemMembership(%s) error = %v", id, err)
		}
	}
}

func insertTestPersistenceRunWithID(t *testing.T, ctx context.Context, store *persistence.Store, runID string) persistence.WorkflowRunRecord {
	t.Helper()

	project := persistence.ProjectRecord{
		ID:                 "project-" + runID,
		Name:               "Project " + runID,
		RepositoryIdentity: "repo",
		SourceRevisionID:   stringPtr("commit"),
		ConfigPath:         "project.json",
		SourceObjectID:     "object",
		ConfigSHA256:       strings.Repeat("a", 64),
		CreatedAt:          "2026-07-03T00:00:00Z",
	}
	if err := store.UpsertProject(ctx, project); err != nil {
		t.Fatalf("UpsertProject() error = %v", err)
	}
	workflowRecord := persistence.WorkflowRecord{
		ID:                 "workflow-" + runID,
		ProjectID:          project.ID,
		Name:               "Workflow " + runID,
		RepositoryIdentity: "repo",
		SourceRevisionID:   stringPtr("commit"),
		WorkflowPath:       "workflow.json",
		SourceObjectID:     "object",
		WorkflowSHA256:     strings.Repeat("b", 64),
		CreatedAt:          "2026-07-03T00:00:00Z",
	}
	if err := store.UpsertWorkflow(ctx, workflowRecord); err != nil {
		t.Fatalf("UpsertWorkflow() error = %v", err)
	}
	run := persistence.WorkflowRunRecord{
		ID:                    runID,
		ProjectID:             project.ID,
		WorkflowID:            workflowRecord.ID,
		SubmissionContextJSON: `{"variables":[]}`,
		CreatedAt:             "2026-07-03T00:00:00Z",
	}
	if err := store.CreateWorkflowRun(ctx, run); err != nil {
		t.Fatalf("CreateWorkflowRun() error = %v", err)
	}
	if err := store.InsertStagePlan(ctx, run.ID, []persistence.WorkflowStageRecord{{
		RunID:                run.ID,
		StageIndex:           0,
		StepID:               "step-001",
		StageSourceReference: "workflow.json#/steps/0",
		State:                "ready",
		CreatedAt:            "2026-07-03T00:00:00Z",
		ReadyAt:              "2026-07-03T00:00:00Z",
	}}); err != nil {
		t.Fatalf("InsertStagePlan() error = %v", err)
	}
	return run
}
