package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"goetl/internal/model"
	"goetl/internal/persistence"
	"goetl/internal/variable"
	"goetl/internal/workflow"
)

func TestCreateWorkflowDependencyPlanPersistsDependencyFactsWithoutMutatingSubmissionContext(t *testing.T) {
	store := openTestWorkflowExecutionStore(t)
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Fatalf("close workflow execution store: %v", err)
		}
	})
	run := insertTestPersistenceRunWithStage(t, context.Background(), store)
	originalSubmissionContextJSON := run.SubmissionContextJSON
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

	ctx := context.Background()
	if err := controller.CreateWorkflowDependencyPlan(ctx, run.ID, run.WorkflowID, stages); err != nil {
		t.Fatalf("CreateWorkflowDependencyPlan() error = %v", err)
	}

	orderedStages, err := controller.ListWorkflowStages(ctx, run.ID)
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

	orderedSteps, err := controller.ListWorkflowSteps(ctx, run.ID)
	if err != nil {
		t.Fatalf("ListWorkflowSteps() error = %v", err)
	}
	if len(orderedSteps) != 3 {
		t.Fatalf("step count = %d, want 3", len(orderedSteps))
	}
	if orderedSteps[0].StepIndex != 0 || orderedSteps[1].StepIndex != 2 || orderedSteps[2].StepIndex != 1 {
		t.Fatalf("unordered steps = %+v", orderedSteps)
	}

	dependencySteps, err := queryDependencyStepsForRun(ctx, store, run.ID)
	if err != nil {
		t.Fatalf("query dependency steps: %v", err)
	}
	if len(dependencySteps) != 3 {
		t.Fatalf("dependency steps len = %d, want 3", len(dependencySteps))
	}
	if dependencySteps[0].StageIndex != 0 || dependencySteps[0].StepIndex != 0 || dependencySteps[0].StepID != "write-a2" {
		t.Fatalf("ordered dependency step[0] = %+v, want stage 0 step 0 write-a2", dependencySteps[0])
	}
	if dependencySteps[1].StageIndex != 0 || dependencySteps[1].StepIndex != 2 || dependencySteps[1].StepID != "write-a" {
		t.Fatalf("ordered dependency step[1] = %+v, want stage 0 step 2 write-a", dependencySteps[1])
	}
	if dependencySteps[2].StageIndex != 1 || dependencySteps[2].StepIndex != 1 || dependencySteps[2].StepID != "write-b" {
		t.Fatalf("ordered dependency step[2] = %+v, want stage 1 step 1 write-b", dependencySteps[2])
	}

	runRecord, found, err := store.GetWorkflowRun(ctx, run.ID)
	if err != nil || !found {
		t.Fatalf("GetWorkflowRun() found=%v error=%v", found, err)
	}
	if runRecord.SubmissionContextJSON != originalSubmissionContextJSON {
		t.Fatalf("submission context changed to %s, want unchanged %s", runRecord.SubmissionContextJSON, originalSubmissionContextJSON)
	}
}

func TestDependencyTransitionsAreReadableThroughSubmissionLogs(t *testing.T) {
	store := openTestWorkflowExecutionStore(t)
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Fatalf("close workflow execution store: %v", err)
		}
	})
	run := insertTestPersistenceRunWithStage(t, context.Background(), store)
	logRoot := t.TempDir()
	logSink, err := newFilesystemLogSink(logRoot, string(model.LogLevelDebug))
	if err != nil {
		t.Fatalf("newFilesystemLogSink() error = %v", err)
	}
	controller := newController()
	controller.workflowStore = store
	controller.logRootPath = logRoot
	controller.logSink = logSink
	controller.logReadDefaultTail = 20
	controller.logReadMaxTail = 20
	ctx := context.Background()

	stages := []workflow.WorkflowStage{
		{
			Index: 0,
			Steps: []workflow.WorkflowStageStep{{
				StageIndex: 0,
				StepIndex:  0,
				StepID:     "download",
			}},
		},
	}
	if err := controller.CreateWorkflowDependencyPlan(ctx, run.ID, run.WorkflowID, stages); err != nil {
		t.Fatalf("CreateWorkflowDependencyPlan() error = %v", err)
	}
	if err := controller.RecordCompiledWorkItemMembership(ctx, run.ID, 0, 0, "download-001", 0); err != nil {
		t.Fatalf("RecordCompiledWorkItemMembership() error = %v", err)
	}
	if err := controller.RecordWorkItemTerminalFailure(context.Background(), run.ID, "download-001", "boom"); err != nil {
		t.Fatalf("RecordWorkItemTerminalFailure() error = %v", err)
	}

	request := httptest.NewRequest(http.MethodGet, "/submissions/"+run.ID+"/logs", nil)
	response := httptest.NewRecorder()
	controller.submissionLogsHandler(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status code = %d, want 200: %s", response.Code, response.Body.String())
	}
	var got submissionLogResponse
	if err := json.NewDecoder(response.Body).Decode(&got); err != nil {
		t.Fatalf("decode logs response: %v", err)
	}
	messages := make([]string, 0, len(got.Entries))
	for _, entry := range got.Entries {
		messages = append(messages, entry.Message)
	}
	joined := strings.Join(messages, "\n")
	if !strings.Contains(joined, "normalized workflow into 1 stages") {
		t.Fatalf("log messages = %q, want normalization observation", joined)
	}
	if !strings.Contains(joined, "failed workflow at stage 0 step 0: boom") {
		t.Fatalf("log messages = %q, want failure observation", joined)
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

	dependencyWorkItems, err := queryDependencyWorkItemsForStep(ctx, store, run.ID, 0, 0)
	if err != nil {
		t.Fatalf("query dependency work items: %v", err)
	}
	if len(dependencyWorkItems) != 1 {
		t.Fatalf("dependency work item count = %d, want 1", len(dependencyWorkItems))
	}
	if dependencyWorkItems[0].WorkItemID != "workitem-001" || dependencyWorkItems[0].WorkItemIndex != 7 {
		t.Fatalf("dependency work item = %+v, want id=workitem-001 index=7", dependencyWorkItems[0])
	}
}

func TestRecordCompiledWorkItemMembershipPersistsOrderedFactRows(t *testing.T) {
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
				StepIndex:  1,
				StepID:     "write-a",
			}},
		},
	}
	ctx := context.Background()
	if err := controller.CreateWorkflowDependencyPlan(ctx, run.ID, run.WorkflowID, stages); err != nil {
		t.Fatalf("CreateWorkflowDependencyPlan() error = %v", err)
	}
	if err := controller.RecordCompiledWorkItemMembership(ctx, run.ID, 0, 1, "workitem-b", 2); err != nil {
		t.Fatalf("RecordCompiledWorkItemMembership(b) error = %v", err)
	}
	if err := controller.RecordCompiledWorkItemMembership(ctx, run.ID, 0, 1, "workitem-a", 0); err != nil {
		t.Fatalf("RecordCompiledWorkItemMembership(a) error = %v", err)
	}

	dependencyWorkItems, err := queryDependencyWorkItemsForStep(ctx, store, run.ID, 0, 1)
	if err != nil {
		t.Fatalf("query dependency work items: %v", err)
	}
	if len(dependencyWorkItems) != 2 {
		t.Fatalf("dependency work item count = %d, want 2", len(dependencyWorkItems))
	}
	if dependencyWorkItems[0].WorkItemID != "workitem-a" || dependencyWorkItems[0].WorkItemIndex != 0 {
		t.Fatalf("ordered work item[0] = %+v, want workitem-a index 0", dependencyWorkItems[0])
	}
	if dependencyWorkItems[1].WorkItemID != "workitem-b" || dependencyWorkItems[1].WorkItemIndex != 2 {
		t.Fatalf("ordered work item[1] = %+v, want workitem-b index 2", dependencyWorkItems[1])
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
	if err := controller.RecordCompiledWorkItemMembership(ctx, run.ID, 0, 0, "workitem-a", 0); err != nil {
		t.Fatalf("RecordCompiledWorkItemMembership(a) error = %v", err)
	}
	if err := controller.RecordCompiledWorkItemMembership(ctx, run.ID, 0, 1, "workitem-b", 1); err != nil {
		t.Fatalf("RecordCompiledWorkItemMembership(b) error = %v", err)
	}
	completeDependencyWorkItemForTest(t, ctx, store, controller, run.ID, "workitem-a", 0, 0, `{"value":"a"}`)

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

	completeDependencyWorkItemForTest(t, ctx, store, controller, run.ID, "workitem-b", 0, 1, `{"value":"b"}`)
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
	if context.DependencyState != nil {
		t.Fatalf("dependency state = %+v, want omitted from submission context", context.DependencyState)
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
	if err := controller.RecordCompiledWorkItemMembership(ctx, run.ID, 0, 0, "workitem-a", 0); err != nil {
		t.Fatalf("RecordCompiledWorkItemMembership(a) error = %v", err)
	}
	if err := controller.RecordCompiledWorkItemMembership(ctx, run.ID, 0, 0, "workitem-b", 1); err != nil {
		t.Fatalf("RecordCompiledWorkItemMembership(b) error = %v", err)
	}
	completeDependencyWorkItemForTest(t, ctx, store, controller, run.ID, "workitem-a", 0, 0, `{"value":"a"}`)
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
	if err := controller.RecordWorkItemTerminalFailure(ctx, run.ID, "workitem-failed", "worker reported boom"); err != nil {
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
	if step.FailureReason != "worker reported boom" {
		t.Fatalf("step failure reason = %q, want worker error", step.FailureReason)
	}
	if step.WorkItems[0].State != model.WorkItemMembershipStateFailed {
		t.Fatalf("workitem state = %q, want %q", step.WorkItems[0].State, model.WorkItemMembershipStateFailed)
	}
	if step.WorkItems[0].FailureReason != "worker reported boom" {
		t.Fatalf("workitem failure reason = %q, want worker error", step.WorkItems[0].FailureReason)
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
	if stage.FailureReason != "worker reported boom" {
		t.Fatalf("stage failure reason = %q, want worker error", stage.FailureReason)
	}
}

func TestRecordWorkItemTerminalStateKeepsFailureTerminalAfterConflictingCompletion(t *testing.T) {
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
	createSingleStepDependencyPlan(t, ctx, controller, run.ID, run.WorkflowID, "work-0")

	if err := controller.RecordWorkItemTerminalFailure(ctx, run.ID, "work-0", "first failure wins"); err != nil {
		t.Fatalf("RecordWorkItemTerminalFailure() error = %v", err)
	}
	if err := controller.RecordWorkItemTerminalState(ctx, run.ID, "work-0", model.WorkItemMembershipStateCompleted); err != nil {
		t.Fatalf("RecordWorkItemTerminalState(completed) error = %v", err)
	}
	if err := controller.RecordCompletedWorkItemOutput(ctx, RecordCompletedWorkItemOutputRequest{
		SubmissionID: run.ID,
		WorkItemID:   "work-0",
		OutputJSON:   `{"value":"late"}`,
	}); err != nil {
		t.Fatalf("RecordCompletedWorkItemOutput(late) error = %v", err)
	}

	step, found, err := controller.ReadStepState(ctx, run.ID, 0)
	if err != nil || !found {
		t.Fatalf("ReadStepState() found=%v error=%v", found, err)
	}
	if step.State != model.WorkflowStepStateFailed {
		t.Fatalf("step state = %q, want failed", step.State)
	}
	if step.WorkItems[0].State != model.WorkItemMembershipStateFailed {
		t.Fatalf("membership state = %q, want failed", step.WorkItems[0].State)
	}
	if step.WorkItems[0].OutputJSON != "" {
		t.Fatalf("failed membership output = %q, want unchanged", step.WorkItems[0].OutputJSON)
	}

	runRecord, found, err := store.GetWorkflowRun(ctx, run.ID)
	if err != nil || !found {
		t.Fatalf("GetWorkflowRun() found=%v error=%v", found, err)
	}
	var context workflowRunSubmissionContext
	if err := json.Unmarshal([]byte(runRecord.SubmissionContextJSON), &context); err != nil {
		t.Fatalf("decode submission context: %v", err)
	}
	if context.DependencyState == nil || context.DependencyState.State != model.WorkflowStateFailed {
		t.Fatalf("dependency workflow state = %+v, want failed", context.DependencyState)
	}
	if context.DependencyState.FailureReason != "first failure wins" {
		t.Fatalf("workflow failure reason = %q, want first failure", context.DependencyState.FailureReason)
	}
}

func TestLateSiblingCompletionDoesNotChangeFailedWorkflowTerminalState(t *testing.T) {
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

	if err := controller.CreateWorkflowDependencyPlan(ctx, run.ID, run.WorkflowID, []workflow.WorkflowStage{
		{
			Index: 0,
			Steps: []workflow.WorkflowStageStep{
				{StageIndex: 0, StepIndex: 0, StepID: "left"},
				{StageIndex: 0, StepIndex: 1, StepID: "right"},
			},
		},
		{
			Index: 1,
			Steps: []workflow.WorkflowStageStep{
				{StageIndex: 1, StepIndex: 2, StepID: "downstream"},
			},
		},
	}); err != nil {
		t.Fatalf("CreateWorkflowDependencyPlan() error = %v", err)
	}
	if err := controller.RecordCompiledWorkItemMembership(ctx, run.ID, 0, 0, "work-left", 0); err != nil {
		t.Fatalf("RecordCompiledWorkItemMembership(left) error = %v", err)
	}
	if err := controller.RecordCompiledWorkItemMembership(ctx, run.ID, 0, 1, "work-right", 0); err != nil {
		t.Fatalf("RecordCompiledWorkItemMembership(right) error = %v", err)
	}

	if err := controller.RecordWorkItemTerminalFailure(ctx, run.ID, "work-left", "left failed"); err != nil {
		t.Fatalf("RecordWorkItemTerminalFailure(left) error = %v", err)
	}
	if err := controller.RecordCompletedWorkItemOutput(ctx, RecordCompletedWorkItemOutputRequest{
		SubmissionID: run.ID,
		WorkItemID:   "work-right",
		OutputJSON:   `{"value":"late sibling"}`,
	}); err != nil {
		t.Fatalf("RecordCompletedWorkItemOutput(right) error = %v", err)
	}
	if err := controller.RecordWorkItemTerminalState(ctx, run.ID, "work-right", model.WorkItemMembershipStateCompleted); err != nil {
		t.Fatalf("RecordWorkItemTerminalState(right) error = %v", err)
	}

	stage0, found, err := controller.ReadStageState(ctx, run.ID, 0)
	if err != nil || !found {
		t.Fatalf("ReadStageState(0) found=%v error=%v", found, err)
	}
	if stage0.State != model.WorkflowStageStateFailed {
		t.Fatalf("stage 0 state = %q, want failed", stage0.State)
	}
	stage1, found, err := controller.ReadStageState(ctx, run.ID, 1)
	if err != nil || !found {
		t.Fatalf("ReadStageState(1) found=%v error=%v", found, err)
	}
	if stage1.State != model.WorkflowStageStateBlocked {
		t.Fatalf("stage 1 state = %q, want blocked", stage1.State)
	}
	right := stage0.Steps[1]
	if right.State != model.WorkflowStepStateCompleted || right.WorkItems[0].State != model.WorkItemMembershipStateCompleted {
		t.Fatalf("late sibling state = %+v, want completed membership without workflow revival", right)
	}

	runRecord, found, err := store.GetWorkflowRun(ctx, run.ID)
	if err != nil || !found {
		t.Fatalf("GetWorkflowRun() found=%v error=%v", found, err)
	}
	var context workflowRunSubmissionContext
	if err := json.Unmarshal([]byte(runRecord.SubmissionContextJSON), &context); err != nil {
		t.Fatalf("decode submission context: %v", err)
	}
	if context.DependencyState == nil || context.DependencyState.State != model.WorkflowStateFailed {
		t.Fatalf("dependency workflow state = %+v, want failed", context.DependencyState)
	}
	if context.DependencyState.FailureReason != "left failed" {
		t.Fatalf("workflow failure reason = %q, want original failure", context.DependencyState.FailureReason)
	}
}

func TestCreateWorkflowDependencyPlanDoesNotWriteDependencyStateJSON(t *testing.T) {
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
		Dependency json.RawMessage `json:"dependency_state"`
	}
	if err := json.Unmarshal([]byte(runRecord.SubmissionContextJSON), &contextJSON); err != nil {
		t.Fatalf("unmarshal run submission context: %v", err)
	}
	if len(contextJSON.Dependency) != 0 {
		t.Fatalf("dependency_state JSON = %s, want omitted", string(contextJSON.Dependency))
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

	completeDependencyWorkItemForTest(t, ctx, store, controller, run.ID, "work-0", 0, 0, `{"label":"done","answer":42}`)

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
	fact, found, err := store.GetWorkflowStepOutputFact(ctx, run.ID, 0)
	if err != nil {
		t.Fatalf("GetWorkflowStepOutputFact() error = %v", err)
	}
	if !found {
		t.Fatal("aggregate output fact missing")
	}
	if fact.OutputJSON != `{"value":"a"}` || fact.OutputJSONSHA256 != step.OutputJSONSHA256 || fact.OutputJSONBytes != len([]byte(`{"value":"a"}`)) || fact.OutputJSONPruned || fact.OutputKind != "aggregate" {
		t.Fatalf("aggregate output fact = %+v, want retained aggregate metadata", fact)
	}
}

func TestFreshControllerReconstructsDependencyStateWithoutDependencyStateJSON(t *testing.T) {
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

	work := testPersistenceWorkItem("work-0", run.ID, 0, 0)
	if err := store.InsertWorkItems(ctx, []persistence.WorkItemRecord{work}); err != nil {
		t.Fatalf("InsertWorkItems() error = %v", err)
	}
	if err := store.EnqueueWorkItems(ctx, []persistence.QueuedWorkRecord{{WorkItemRecord: work, QueuedAt: "2026-07-03T00:00:00Z"}}); err != nil {
		t.Fatalf("EnqueueWorkItems() error = %v", err)
	}
	if _, found, err := store.ClaimNextWork(ctx, testWorkerClaimRequest(t, store, "attempt-0", "2026-07-03T00:00:01Z")); err != nil || !found {
		t.Fatalf("ClaimNextWork() found=%v error=%v, want success", found, err)
	}
	resolvedOutput, err := resolvedOutputFromJSON(`{"value":"a"}`)
	if err != nil {
		t.Fatalf("resolvedOutputFromJSON() error = %v", err)
	}
	outputJSON, outputJSONSHA256, err := canonicalOutputJSONFromResolved(resolvedOutput)
	if err != nil {
		t.Fatalf("canonicalOutputJSONFromResolved() error = %v", err)
	}
	completed, found, err := store.CompleteAttempt(ctx, persistence.CompleteAttemptRequest{
		AttemptID:        "attempt-0",
		OutputJSON:       outputJSON,
		OutputJSONSHA256: outputJSONSHA256,
		PreStateSHA256:   strings.Repeat("e", 64),
		PostStateSHA256:  strings.Repeat("f", 64),
		CompletedAt:      "2026-07-03T00:00:02Z",
	})
	if err != nil || !found {
		t.Fatalf("CompleteAttempt() found=%v error=%v, want success", found, err)
	}
	if err := controller.RecordCompletedWorkItemOutput(ctx, RecordCompletedWorkItemOutputRequest{
		SubmissionID:     run.ID,
		WorkItemID:       "work-0",
		OutputJSON:       completed.OutputJSON,
		OutputJSONSHA256: completed.OutputJSONSHA256,
	}); err != nil {
		t.Fatalf("RecordCompletedWorkItemOutput() error = %v", err)
	}
	if err := store.UpdateWorkflowRunSubmissionContext(ctx, run.ID, `{"variables":[]}`); err != nil {
		t.Fatalf("UpdateWorkflowRunSubmissionContext() error = %v", err)
	}

	recovered := newController()
	recovered.workflowStore = store
	stages, err := recovered.ListWorkflowStages(ctx, run.ID)
	if err != nil {
		t.Fatalf("ListWorkflowStages() error = %v", err)
	}
	if len(stages) != 2 || stages[0].StageIndex != 0 || stages[1].StageIndex != 1 {
		t.Fatalf("stages = %+v, want deterministic stage 0 then 1", stages)
	}
	if stages[0].State != model.WorkflowStageStateCompleted || stages[1].State != model.WorkflowStageStateBlocked {
		t.Fatalf("stage states = %+v, want completed then blocked", stages)
	}
	plan, found, err := recovered.getWorkflowDependencyState(ctx, run.ID)
	if err != nil || !found {
		t.Fatalf("getWorkflowDependencyState() found=%v error=%v", found, err)
	}
	scope, err := workflowStepScope(*plan, 1)
	if err != nil {
		t.Fatalf("workflowStepScope() error = %v", err)
	}
	resolver := variable.NewResolver(variable.NewSet(scope), variable.ResolverConfig{})
	stepValue, err := resolver.Resolve(variable.Reference{Name: variable.Name{Namespace: variable.NamespaceWorkflow, Key: "step"}, Qualified: true})
	if err != nil {
		t.Fatalf("Resolve(workflow.step) error = %v", err)
	}
	value, err := variable.ApplyAccessor(stepValue, "[0].value")
	if err != nil {
		t.Fatalf("ApplyAccessor([0].value) error = %v", err)
	}
	if value.Type != variable.TypeString || value.Value != "a" {
		t.Fatalf("workflow.step[0].value = %#v, want string a", value)
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
		index  int
		output string
	}{
		{id: "work-2", index: 2, output: `{"value":"c"}`},
		{id: "work-0", index: 0, output: `{"value":"a"}`},
		{id: "work-1", index: 1, output: `{"value":"b"}`},
	} {
		completeDependencyWorkItemForTest(t, ctx, store, controller, run.ID, item.id, 0, item.index, item.output)
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

func TestRecordCompletedWorkItemOutputPersistsCollectionDescriptorFact(t *testing.T) {
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
	workIDs := []string{"materialize-cdl--year-2008", "materialize-cdl--year-2009", "materialize-cdl--year-2010"}
	createTwoStageFanoutDependencyPlan(t, ctx, controller, run.ID, run.WorkflowID, workIDs)
	outputs := collectionStep(2008, 2009, 2010).WorkItems

	for index, workID := range workIDs {
		completeDependencyWorkItemForTest(t, ctx, store, controller, run.ID, workID, 0, index, outputs[index].OutputJSON)
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
	var manifest model.MaterializedAssetCollectionManifest
	if err := json.Unmarshal([]byte(step.OutputJSON), &manifest); err != nil {
		t.Fatalf("decode step output: %v", err)
	}
	if err := manifest.Validate(); err != nil {
		t.Fatalf("step collection manifest Validate() error = %v", err)
	}
	if manifest.Path != "/mnt/cache/assets/cdl/${year}.tif" || manifest.MemberCount != 3 {
		t.Fatalf("collection descriptor = %+v", manifest)
	}
	fact, found, err := store.GetWorkflowStepOutputFact(ctx, run.ID, 0)
	if err != nil {
		t.Fatalf("GetWorkflowStepOutputFact() error = %v", err)
	}
	if !found {
		t.Fatal("collection output fact missing")
	}
	if fact.OutputJSON != step.OutputJSON || fact.OutputKind != "aggregate" {
		t.Fatalf("collection output fact = %+v, want retained aggregate descriptor", fact)
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

	completeDependencyWorkItemForTest(t, ctx, store, controller, run.ID, "work-0", 0, 0, `{"value":"terminal"}`)

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
	fact, found, err := store.GetWorkflowStepOutputFact(ctx, run.ID, 0)
	if err != nil {
		t.Fatalf("GetWorkflowStepOutputFact() error = %v", err)
	}
	if !found {
		t.Fatal("terminal output fact missing")
	}
	if fact.OutputJSON != "" || !fact.OutputJSONPruned || fact.OutputJSONSHA256 != step.OutputJSONSHA256 || fact.OutputJSONBytes != len([]byte(`{"value":"terminal"}`)) || fact.OutputKind != "aggregate" {
		t.Fatalf("terminal output fact = %+v, want pruned aggregate metadata", fact)
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
	fact, found, err := store.GetWorkflowStepOutputFact(ctx, run.ID, 0)
	if err != nil {
		t.Fatalf("GetWorkflowStepOutputFact() error = %v", err)
	}
	if !found {
		t.Fatal("step 0 output fact missing after failure")
	}
	if fact.OutputJSON != "" || !fact.OutputJSONPruned || fact.OutputJSONSHA256 != step0.OutputJSONSHA256 || fact.OutputKind != "aggregate" {
		t.Fatalf("step 0 output fact after failure = %+v, want pruned aggregate fact", fact)
	}
}

func TestMarkCompiledStageEmptyStepsCompletedPersistsEmptyOutputFact(t *testing.T) {
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

	if err := controller.CreateWorkflowDependencyPlan(ctx, run.ID, run.WorkflowID, []workflow.WorkflowStage{{
		Index: 0,
		Steps: []workflow.WorkflowStageStep{{
			StageIndex: 0,
			StepIndex:  0,
			StepID:     "empty-step",
		}},
	}}); err != nil {
		t.Fatalf("CreateWorkflowDependencyPlan() error = %v", err)
	}

	allCompleted, err := controller.markCompiledStageEmptyStepsCompleted(ctx, run.ID, workflow.CompileStageResult{
		WorkflowID: run.WorkflowID,
		StageIndex: 0,
	})
	if err != nil {
		t.Fatalf("markCompiledStageEmptyStepsCompleted() error = %v", err)
	}
	if !allCompleted {
		t.Fatal("markCompiledStageEmptyStepsCompleted() allCompleted = false, want true")
	}

	step, found, err := controller.ReadStepState(ctx, run.ID, 0)
	if err != nil || !found {
		t.Fatalf("ReadStepState() found=%v error=%v", found, err)
	}
	if step.OutputJSON != "[]" || step.OutputJSONSHA256 == "" || step.OutputJSONBytes != 2 || step.OutputJSONPruned || len(step.WorkItems) != 0 {
		t.Fatalf("empty step = %+v, want unpruned [] with no memberships", step)
	}
	fact, found, err := store.GetWorkflowStepOutputFact(ctx, run.ID, 0)
	if err != nil {
		t.Fatalf("GetWorkflowStepOutputFact() error = %v", err)
	}
	if !found {
		t.Fatal("empty fan-out output fact missing")
	}
	if fact.OutputJSON != "[]" || fact.OutputJSONSHA256 != step.OutputJSONSHA256 || fact.OutputJSONBytes != 2 || fact.OutputJSONPruned || fact.OutputKind != "empty_fanout" {
		t.Fatalf("empty output fact = %+v, want empty_fanout [] metadata", fact)
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

func queryDependencyStepsForRun(ctx context.Context, store *persistence.Store, runID string) ([]persistence.WorkflowDependencyStepRecord, error) {
	return store.ListWorkflowDependencySteps(ctx, runID)
}

func queryDependencyWorkItemsForStep(ctx context.Context, store *persistence.Store, runID string, stageIndex int, stepIndex int) ([]persistence.WorkflowDependencyWorkItemRecord, error) {
	return store.ListWorkflowDependencyWorkItems(ctx, runID, stageIndex, stepIndex)
}

func completeDependencyWorkItemForTest(t *testing.T, ctx context.Context, store *persistence.Store, controller *Controller, runID string, workItemID string, stageIndex int, workItemIndex int, outputJSON string) {
	t.Helper()

	work := testPersistenceWorkItem(workItemID, runID, stageIndex, workItemIndex)
	if err := store.InsertWorkItems(ctx, []persistence.WorkItemRecord{work}); err != nil {
		t.Fatalf("InsertWorkItems(%s) error = %v", workItemID, err)
	}
	if err := store.EnqueueWorkItems(ctx, []persistence.QueuedWorkRecord{{WorkItemRecord: work, QueuedAt: "2026-07-03T00:00:00Z"}}); err != nil {
		t.Fatalf("EnqueueWorkItems(%s) error = %v", workItemID, err)
	}
	attemptID := "attempt-" + strings.ReplaceAll(workItemID, ":", "-")
	if _, found, err := store.ClaimNextWork(ctx, testWorkerClaimRequest(t, store, attemptID, "2026-07-03T00:00:01Z")); err != nil || !found {
		t.Fatalf("ClaimNextWork(%s) found=%v error=%v, want success", workItemID, found, err)
	}
	resolvedOutput, err := resolvedOutputFromJSON(outputJSON)
	if err != nil {
		t.Fatalf("resolvedOutputFromJSON(%s) error = %v", workItemID, err)
	}
	canonicalOutputJSON, canonicalOutputJSONSHA256, err := canonicalOutputJSONFromResolved(resolvedOutput)
	if err != nil {
		t.Fatalf("canonicalOutputJSONFromResolved(%s) error = %v", workItemID, err)
	}
	completed, found, err := store.CompleteAttempt(ctx, persistence.CompleteAttemptRequest{
		AttemptID:        attemptID,
		OutputJSON:       canonicalOutputJSON,
		OutputJSONSHA256: canonicalOutputJSONSHA256,
		PreStateSHA256:   strings.Repeat("e", 64),
		PostStateSHA256:  strings.Repeat("f", 64),
		CompletedAt:      "2026-07-03T00:00:02Z",
	})
	if err != nil || !found {
		t.Fatalf("CompleteAttempt(%s) found=%v error=%v, want success", workItemID, found, err)
	}
	if err := controller.RecordCompletedWorkItemOutput(ctx, RecordCompletedWorkItemOutputRequest{
		SubmissionID:     runID,
		WorkItemID:       workItemID,
		OutputJSON:       completed.OutputJSON,
		OutputJSONSHA256: completed.OutputJSONSHA256,
	}); err != nil {
		t.Fatalf("RecordCompletedWorkItemOutput(%s) error = %v", workItemID, err)
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
