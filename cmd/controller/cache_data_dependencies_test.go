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

func TestHydrateCommitDataWorkItemUsesCompletedProducerArtifactManifest(t *testing.T) {
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

	computeRecord := persistence.WorkItemRecord{
		ID:                   run.ID + ":compute-a",
		RunID:                run.ID,
		StageIndex:           0,
		WorkItemIndex:        0,
		WorkerPayloadJSON:    mustWorkItemJSON(t, model.WorkItem{ID: "compute-a", Type: model.WorkItemTypeWriteDemoOutput, OutputFilename: "compute-a.txt", StepDefinitionID: "compute"}),
		ResolvedInputsSHA256: strings.Repeat("c", 64),
		CreatedAt:            "2026-07-07T00:00:00Z",
	}
	commitPayload := model.CommitDataWorkItemPayload{
		Operator:            string(model.WorkItemTypeCommitData),
		TargetEnvironmentID: "target-local",
		Source:              model.CommitDataSource{FromWorkItemID: "compute-a", FromArtifact: "summary"},
		PublishTarget: model.BoundPublishTarget{
			Name:            "publish_summary",
			FromArtifact:    "summary",
			TargetName:      "publish_summary",
			Location:        model.DataAssetLocation{Type: model.DataProviderRegisteredLocation, LocationName: "published_data", Path: "reports/summary.csv"},
			OverwritePolicy: model.PublishedDataAssetOverwriteFailIfExists,
		},
	}
	commitRecord := persistence.WorkItemRecord{
		ID:         run.ID + ":commit-a",
		RunID:      run.ID,
		StageIndex: 0,
		WorkerPayloadJSON: mustWorkItemJSON(t, model.WorkItem{
			ID:               "commit-a",
			Type:             model.WorkItemTypeCommitData,
			OutputFilename:   "commit-a.json",
			StepDefinitionID: "compute",
			DependsOn:        []string{computeRecord.ID},
			Parameters: model.Parameters{
				"commit_data": {Type: "commit_data", Value: commitPayload},
			},
		}),
		WorkItemIndex:        1,
		ResolvedInputsSHA256: strings.Repeat("d", 64),
		CreatedAt:            "2026-07-07T00:00:00Z",
	}
	if err := store.QueueWorkItems(ctx, persistence.QueueWorkItemsRequest{
		WorkItems: []persistence.WorkItemRecord{computeRecord, commitRecord},
		QueuedWork: []persistence.QueuedWorkRecord{
			{WorkItemRecord: computeRecord, QueuedAt: "2026-07-07T00:00:00Z"},
		},
	}); err != nil {
		t.Fatalf("QueueWorkItems() error = %v", err)
	}
	claimedCompute, found, err := store.ClaimNextWork(ctx, persistence.ClaimWorkRequest{AttemptID: "attempt-compute", ExecutorType: persistence.ExecutorTypeWorker, StartedAt: "2026-07-07T00:00:01Z"})
	if err != nil || !found || claimedCompute.WorkItem.ID != computeRecord.ID {
		t.Fatalf("ClaimNextWork() = %+v found=%v err=%v, want compute", claimedCompute, found, err)
	}
	completed, found, err := store.CompleteAttempt(ctx, persistence.CompleteAttemptRequest{
		AttemptID:        "attempt-compute",
		OutputJSON:       `{"schema":"goet/artifact-manifest/v1","work_item_id":"compute-a","storage_scope":"worker_data","artifacts":[{"name":"summary","kind":"file","path":"artifacts/raw/compute-a/reports/summary.csv"}]}`,
		OutputJSONSHA256: strings.Repeat("e", 64),
		PreStateSHA256:   strings.Repeat("a", 64),
		PostStateSHA256:  strings.Repeat("b", 64),
		CompletedAt:      "2026-07-07T00:00:02Z",
	})
	if err != nil || !found {
		t.Fatalf("CompleteAttempt() found=%v err=%v", found, err)
	}
	if err := controller.enqueueReadyCacheDataDependents(ctx, computeRecord, time.Date(2026, 7, 7, 0, 0, 3, 0, time.UTC)); err != nil {
		t.Fatalf("enqueueReadyCacheDataDependents() error = %v", err)
	}
	claimedCommit, found, err := store.ClaimNextWork(ctx, persistence.ClaimWorkRequest{AttemptID: "attempt-commit", ExecutorType: persistence.ExecutorTypeWorker, StartedAt: "2026-07-07T00:00:04Z"})
	if err != nil || !found || claimedCommit.WorkItem.ID != commitRecord.ID {
		t.Fatalf("ClaimNextWork(commit) = %+v found=%v err=%v", claimedCommit, found, err)
	}

	var commitItem model.WorkItem
	if err := json.Unmarshal([]byte(claimedCommit.WorkItem.WorkerPayloadJSON), &commitItem); err != nil {
		t.Fatalf("decode commit payload: %v", err)
	}
	commitItem.AttemptID = claimedCommit.AttemptID
	hydrated, err := controller.hydrateCommitDataWorkItem(ctx, claimedCommit, commitItem)
	if err != nil {
		t.Fatalf("hydrateCommitDataWorkItem() error = %v", err)
	}
	if _, ok := hydrated.Parameters["artifact_manifest"]; !ok {
		t.Fatal("artifact_manifest parameter missing")
	}
	if completed.WorkItemID != computeRecord.ID {
		t.Fatalf("completed work item = %s, want compute", completed.WorkItemID)
	}
}

func TestHydrateCacheDataDependentWorkItemUsesCompletedMaterializedManifest(t *testing.T) {
	ctx := context.Background()
	store := openTestWorkflowExecutionStore(t)
	defer store.Close()
	controller := newController()
	controller.workflowStore = store
	run := insertTestPersistenceRunWithStage(t, ctx, store)

	cacheRecord := persistence.WorkItemRecord{
		ID:                   run.ID + ":cache-data-field-tile",
		RunID:                run.ID,
		StageIndex:           0,
		WorkItemIndex:        0,
		WorkerPayloadJSON:    mustWorkItemJSON(t, model.WorkItem{ID: "cache-data-field-tile", Type: model.WorkItemTypeCacheData, OutputFilename: "cache-data-field-tile.json"}),
		ResolvedInputsSHA256: strings.Repeat("c", 64),
		CreatedAt:            "2026-07-07T00:00:00Z",
	}
	computeRecord := persistence.WorkItemRecord{
		ID:         run.ID + ":compute-field-cdl-composition",
		RunID:      run.ID,
		StageIndex: 0,
		WorkerPayloadJSON: mustWorkItemJSON(t, model.WorkItem{
			ID:             "compute-field-cdl-composition",
			Type:           model.WorkItemTypePythonScript,
			OutputFilename: "compute-field-cdl-composition.json",
			DependsOn:      []string{cacheRecord.ID},
			Parameters: model.Parameters{
				"python_entrypoint": {Type: "path", Value: "scripts/field_cdl.py"},
				"data_assets": {Type: "data_assets", Value: []model.BoundDataAsset{
					{
						BindingName:  "field_tile_fixture",
						ProviderName: "field_tile_provider",
						Kind:         "fixture_matrix",
						Provider:     model.DataProviderLocalFile,
						Location: model.DataAssetLocation{
							Type:         model.DataProviderLocalFile,
							LocationName: "fixture_data",
							Path:         "field_tile.csv",
						},
					},
				}},
			},
		}),
		WorkItemIndex:        1,
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
	claimedCache, found, err := store.ClaimNextWork(ctx, persistence.ClaimWorkRequest{AttemptID: "attempt-cache-field-tile", ExecutorType: persistence.ExecutorTypeWorker, StartedAt: "2026-07-07T00:00:01Z"})
	if err != nil || !found || claimedCache.WorkItem.ID != cacheRecord.ID {
		t.Fatalf("ClaimNextWork(cache) = %+v found=%v err=%v, want cache", claimedCache, found, err)
	}
	if _, found, err := store.CompleteAttempt(ctx, persistence.CompleteAttemptRequest{
		AttemptID:        claimedCache.AttemptID,
		OutputJSON:       `{"schema":"goet/materialized-data-assets/v1","asset_key":"sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef","target_environment_id":"target-local","assets":[{"binding_name":"field_tile_fixture","provider_name":"field_tile_provider","provider_type":"local_file","kind":"fixture_matrix","format":"csv","local_path":"/target/cache/field_tile.csv","cache_key":"fixtures/field_tile.csv","source_sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}]}`,
		OutputJSONSHA256: strings.Repeat("e", 64),
		PreStateSHA256:   strings.Repeat("a", 64),
		PostStateSHA256:  strings.Repeat("b", 64),
		CompletedAt:      "2026-07-07T00:00:02Z",
	}); err != nil || !found {
		t.Fatalf("CompleteAttempt(cache) found=%v err=%v", found, err)
	}
	if err := store.EnqueueWorkItems(ctx, []persistence.QueuedWorkRecord{{WorkItemRecord: computeRecord, QueuedAt: "2026-07-07T00:00:03Z"}}); err != nil {
		t.Fatalf("EnqueueWorkItems(compute) error = %v", err)
	}
	claimedCompute, found, err := store.ClaimNextWork(ctx, persistence.ClaimWorkRequest{AttemptID: "attempt-compute-field-cdl", ExecutorType: persistence.ExecutorTypeWorker, StartedAt: "2026-07-07T00:00:04Z"})
	if err != nil || !found || claimedCompute.WorkItem.ID != computeRecord.ID {
		t.Fatalf("ClaimNextWork(compute) = %+v found=%v err=%v, want compute", claimedCompute, found, err)
	}

	var computeItem model.WorkItem
	if err := json.Unmarshal([]byte(claimedCompute.WorkItem.WorkerPayloadJSON), &computeItem); err != nil {
		t.Fatalf("decode compute payload: %v", err)
	}
	hydrated, err := controller.hydrateCacheDataDependentWorkItem(ctx, claimedCompute, computeItem)
	if err != nil {
		t.Fatalf("hydrateCacheDataDependentWorkItem() error = %v", err)
	}
	parameter, ok := hydrated.Parameters["materialized_data_assets"]
	if !ok {
		t.Fatal("materialized_data_assets parameter missing")
	}
	data, err := json.Marshal(parameter.Value)
	if err != nil {
		t.Fatalf("marshal materialized_data_assets: %v", err)
	}
	var manifest model.MaterializedDataAssetManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatalf("decode materialized_data_assets: %v", err)
	}
	if manifest.TargetEnvironmentID != "target-local" || len(manifest.Assets) != 1 {
		t.Fatalf("manifest = %+v, want one target-local asset", manifest)
	}
	if manifest.Assets[0].BindingName != "field_tile_fixture" || manifest.Assets[0].LocalPath != "/target/cache/field_tile.csv" {
		t.Fatalf("manifest asset = %+v, want completed cache_data local path", manifest.Assets[0])
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
