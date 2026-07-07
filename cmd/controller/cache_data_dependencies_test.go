package main

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"goetl/internal/model"
	"goetl/internal/persistence"
	"goetl/internal/workflow"
)

func TestEnqueueReadyCacheDataDependentsQueuesComputeAfterCacheCompletion(t *testing.T) {
	ctx := context.Background()
	store := openTestWorkflowExecutionStore(t)
	defer store.Close()
	controller := newController()
	controller.workflowStore = store
	run := insertTestPersistenceRunWithStage(t, ctx, store)
	if err := controller.CreateWorkflowDependencyPlan(ctx, run.ID, run.WorkflowID, []workflow.WorkflowStage{
		{
			Index: 0,
			Steps: []workflow.WorkflowStageStep{
				{StageIndex: 0, StepIndex: 0, StepID: "compute"},
			},
		},
	}); err != nil {
		t.Fatalf("CreateWorkflowDependencyPlan() error = %v", err)
	}

	cachePayload := mustWorkItemJSON(t, model.WorkItem{
		ID:             "cache-data-a",
		Type:           model.WorkItemTypeCacheData,
		OutputFilename: "cache-data-a.json",
	})
	computePayload := mustWorkItemJSON(t, model.WorkItem{
		ID:               "compute-a",
		Type:             model.WorkItemTypeWriteDemoOutput,
		OutputFilename:   "compute-a.txt",
		StepDefinitionID: "compute",
		DependsOn:        []string{run.ID + ":cache-data-a"},
	})
	cacheRecord := persistence.WorkItemRecord{
		ID:                   run.ID + ":cache-data-a",
		RunID:                run.ID,
		StageIndex:           0,
		WorkItemIndex:        0,
		WorkerPayloadJSON:    cachePayload,
		ResolvedInputsSHA256: strings.Repeat("c", 64),
		CreatedAt:            "2026-07-07T00:00:00Z",
	}
	computeRecord := persistence.WorkItemRecord{
		ID:                   run.ID + ":compute-a",
		RunID:                run.ID,
		StageIndex:           0,
		WorkItemIndex:        1,
		WorkerPayloadJSON:    computePayload,
		ResolvedInputsSHA256: strings.Repeat("d", 64),
		CreatedAt:            "2026-07-07T00:00:00Z",
	}
	if err := store.QueueWorkItems(ctx, persistence.QueueWorkItemsRequest{
		WorkItems: []persistence.WorkItemRecord{cacheRecord, computeRecord},
		QueuedWork: []persistence.QueuedWorkRecord{
			{WorkItemRecord: cacheRecord, QueuedAt: "2026-07-07T00:00:00Z"},
		},
	}); err != nil {
		t.Fatalf("QueueWorkItems() error = %v", err)
	}

	claimed, found, err := store.ClaimNextWork(ctx, persistence.ClaimWorkRequest{
		AttemptID:    "attempt-cache",
		ExecutorType: persistence.ExecutorTypeWorker,
		StartedAt:    "2026-07-07T00:00:01Z",
	})
	if err != nil {
		t.Fatalf("ClaimNextWork() error = %v", err)
	}
	if !found || claimed.WorkItem.ID != cacheRecord.ID {
		t.Fatalf("claimed = %+v found=%v, want cache", claimed, found)
	}
	completed, found, err := store.CompleteAttempt(ctx, persistence.CompleteAttemptRequest{
		AttemptID:        "attempt-cache",
		OutputJSON:       `{"schema":"goet/materialized-data-assets/v1","asset_key":"sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef","target_environment_id":"target-local","assets":[{"binding_name":"cropland_year","provider_type":"local_file","kind":"fixture","local_path":"/cache/source"}]}`,
		OutputJSONSHA256: strings.Repeat("e", 64),
		PreStateSHA256:   strings.Repeat("a", 64),
		PostStateSHA256:  strings.Repeat("b", 64),
		CompletedAt:      "2026-07-07T00:00:02Z",
	})
	if err != nil {
		t.Fatalf("CompleteAttempt() error = %v", err)
	}
	if !found {
		t.Fatal("CompleteAttempt() did not find active attempt")
	}
	if completed.WorkItemID != cacheRecord.ID {
		t.Fatalf("completed work item = %s, want cache", completed.WorkItemID)
	}

	if err := controller.enqueueReadyCacheDataDependents(ctx, cacheRecord, time.Date(2026, 7, 7, 0, 0, 3, 0, time.UTC)); err != nil {
		t.Fatalf("enqueueReadyCacheDataDependents() error = %v", err)
	}
	queued, err := store.ListQueuedWorkItems(ctx)
	if err != nil {
		t.Fatalf("ListQueuedWorkItems() error = %v", err)
	}
	if len(queued) != 1 || queued[0].ID != computeRecord.ID {
		t.Fatalf("queued = %+v, want compute only", queued)
	}
	step, found, err := controller.ReadStepState(ctx, run.ID, 0)
	if err != nil {
		t.Fatalf("ReadStepState() error = %v", err)
	}
	if !found || len(step.WorkItems) != 1 || step.WorkItems[0].WorkItemID != computeRecord.ID {
		t.Fatalf("step = %+v found=%v, want compute membership", step, found)
	}
}

func mustWorkItemJSON(t *testing.T, item model.WorkItem) string {
	t.Helper()
	data, err := json.Marshal(item)
	if err != nil {
		t.Fatalf("marshal work item: %v", err)
	}
	return string(data)
}
