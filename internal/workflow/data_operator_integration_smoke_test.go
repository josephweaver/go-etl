package workflow

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	fp "goetl/internal/fingerprint"
	"goetl/internal/model"
	"goetl/internal/persistence"
)

func TestDataOperatorIntegrationSmokePlansQueuesAndRecordsEvidence(t *testing.T) {
	ctx := context.Background()
	store := openDataOperatorSmokeStore(t, ctx)
	defer store.Close()

	workflowDef := dataOperatorSmokeWorkflow()
	plan, err := NormalizeStages(workflowDef)
	if err != nil {
		t.Fatalf("NormalizeStages() error = %v", err)
	}
	compiled, err := CompileWorkflowStage(testWorkflowResolver(t, 2024), workflowDef, plan, 0)
	if err != nil {
		t.Fatalf("CompileWorkflowStage() error = %v", err)
	}

	cacheItems := cacheDataItems(compiled)
	computeItems := computeDataItems(compiled)
	commitItems := commitDataItems(compiled)
	if len(cacheItems) != 3 {
		t.Fatalf("cache_data count = %d, want 3", len(cacheItems))
	}
	if len(computeItems) != 1 {
		t.Fatalf("compute count = %d, want 1", len(computeItems))
	}
	if len(commitItems) != 2 {
		t.Fatalf("commit_data count = %d, want 2", len(commitItems))
	}
	compute := computeItems[0]
	if _, ok := compute.WorkItem.Parameters["publish"]; ok {
		t.Fatal("compute item still has publish parameter")
	}
	if len(compute.WorkItem.DependsOn) != len(cacheItems) {
		t.Fatalf("compute depends_on = %+v, want all cache_data items", compute.WorkItem.DependsOn)
	}
	for _, commit := range commitItems {
		if len(commit.WorkItem.DependsOn) != 1 || commit.WorkItem.DependsOn[0] != compute.WorkItem.ID {
			t.Fatalf("commit depends_on = %+v, want compute %s", commit.WorkItem.DependsOn, compute.WorkItem.ID)
		}
	}

	runID := "run-data-operator-smoke"
	createdAt := "2026-07-07T00:00:00Z"
	insertDataOperatorSmokeRun(t, ctx, store, runID, createdAt)
	records, queued, constraints := dataOperatorSmokePersistenceRecords(t, runID, compiled, createdAt)
	if err := store.QueueWorkItems(ctx, persistence.QueueWorkItemsRequest{
		WorkItems:           records,
		ResourceConstraints: constraints,
		QueuedWork:          queued,
	}); err != nil {
		t.Fatalf("QueueWorkItems() error = %v", err)
	}

	for i := range cacheItems {
		claim := claimSmokeWork(t, ctx, store, "attempt-cache-"+string(rune('a'+i)), model.WorkItemTypeCacheData)
		if i == 0 {
			if blocked, found, err := store.ClaimNextWork(ctx, persistence.ClaimWorkRequest{AttemptID: "attempt-cache-blocked", ExecutorType: persistence.ExecutorTypeWorker, StartedAt: "2026-07-07T00:00:01Z"}); err != nil {
				t.Fatalf("blocked ClaimNextWork() error = %v", err)
			} else if found {
				t.Fatalf("blocked ClaimNextWork() = %+v, want no work while source mutex is held", blocked)
			}
		}
		completeSmokeAttempt(t, ctx, store, claim, materializedSmokeManifest(t, claim), "2026-07-07T00:00:02Z")
	}

	computeRecord := dataOperatorSmokeRecordByType(t, records, model.WorkItemTypePythonScript)
	if err := store.EnqueueWorkItems(ctx, []persistence.QueuedWorkRecord{{WorkItemRecord: computeRecord, QueuedAt: "2026-07-07T00:00:03Z"}}); err != nil {
		t.Fatalf("EnqueueWorkItems(compute) error = %v", err)
	}
	computeClaim := claimSmokeWork(t, ctx, store, "attempt-compute", model.WorkItemTypePythonScript)
	completeSmokeAttempt(t, ctx, store, computeClaim, artifactSmokeManifest(t, computeClaim.WorkItem.ID), "2026-07-07T00:00:04Z")

	commitRecords := dataOperatorSmokeRecordsByType(t, records, model.WorkItemTypeCommitData)
	commitQueued := make([]persistence.QueuedWorkRecord, 0, len(commitRecords))
	for _, record := range commitRecords {
		commitQueued = append(commitQueued, persistence.QueuedWorkRecord{WorkItemRecord: record, QueuedAt: "2026-07-07T00:00:05Z"})
	}
	if err := store.EnqueueWorkItems(ctx, commitQueued); err != nil {
		t.Fatalf("EnqueueWorkItems(commit_data) error = %v", err)
	}
	firstCommit := claimSmokeWork(t, ctx, store, "attempt-commit-a", model.WorkItemTypeCommitData)
	if blocked, found, err := store.ClaimNextWork(ctx, persistence.ClaimWorkRequest{AttemptID: "attempt-commit-blocked", ExecutorType: persistence.ExecutorTypeWorker, StartedAt: "2026-07-07T00:00:06Z"}); err != nil {
		t.Fatalf("blocked commit ClaimNextWork() error = %v", err)
	} else if found {
		t.Fatalf("blocked commit ClaimNextWork() = %+v, want no work while publish mutex is held", blocked)
	}
	completeSmokeAttempt(t, ctx, store, firstCommit, publishedSmokeManifest(t, "publish_composition_csv"), "2026-07-07T00:00:07Z")
	secondCommit := claimSmokeWork(t, ctx, store, "attempt-commit-b", model.WorkItemTypeCommitData)
	completeSmokeAttempt(t, ctx, store, secondCommit, publishedSmokeManifest(t, "publish_composition_audit"), "2026-07-07T00:00:08Z")

	terminal, err := store.ListTerminalAttemptsForRun(ctx, runID)
	if err != nil {
		t.Fatalf("ListTerminalAttemptsForRun() error = %v", err)
	}
	counts := map[model.WorkItemType]int{}
	for _, attempt := range terminal {
		if attempt.TerminalState != "completed" {
			t.Fatalf("terminal attempt %s state = %s, want completed", attempt.AttemptID, attempt.TerminalState)
		}
		var item model.WorkItem
		if err := json.Unmarshal([]byte(attempt.WorkItem.WorkerPayloadJSON), &item); err != nil {
			t.Fatalf("decode terminal payload: %v", err)
		}
		counts[item.Type]++
	}
	if counts[model.WorkItemTypeCacheData] != 3 || counts[model.WorkItemTypePythonScript] != 1 || counts[model.WorkItemTypeCommitData] != 2 {
		t.Fatalf("terminal type counts = %+v, want 3 cache_data, 1 compute, 2 commit_data", counts)
	}
}

func dataOperatorSmokeWorkflow() Workflow {
	return Workflow{
		ID: "field-cdl-composition-fixture",
		Steps: []Step{
			{
				ID: "field_cdl_composition_fixture",
				FanOut: &FanOutStep{
					WorkItem: FanOutWorkItemTemplate{
						FanOutExpression: "${years[*]}",
						Type:             model.WorkItemTypePythonScript,
						IDPrefix:         "field-cdl-composition",
						OutputPrefix:     "field-cdl-composition",
						OutputExtension:  ".json",
						Parameters: model.Parameters{
							"python_entrypoint":     {Type: "path", Value: "scripts/field_cdl_composition.py"},
							"target_environment_id": {Type: "string", Value: "target-local"},
							"data_assets":           {Type: "data_assets", Value: dataOperatorSmokeAssets()},
							"publish":               {Type: "publish_targets", Value: dataOperatorSmokePublishTargets()},
						},
					},
				},
			},
		},
	}
}

func dataOperatorSmokeAssets() []model.BoundDataAsset {
	return []model.BoundDataAsset{
		dataOperatorSmokeAsset("crop_lookup_fixture", "crop_lookup_provider", "crop_lookup.csv"),
		dataOperatorSmokeAsset("field_tile_fixture", "field_tile_provider", "field_tile.csv"),
		dataOperatorSmokeAsset("cdl_tile_fixture", "cdl_tile_provider", "cdl_tile.csv"),
	}
}

func dataOperatorSmokeAsset(bindingName, providerName, path string) model.BoundDataAsset {
	return model.BoundDataAsset{
		BindingName:  bindingName,
		ProviderName: providerName,
		Kind:         "fixture_matrix",
		Format:       "csv",
		Provider:     model.DataProviderLocalFile,
		Location: model.DataAssetLocation{
			Type:         model.DataProviderLocalFile,
			LocationName: "fixture_data",
			Path:         path,
		},
		Cache: model.DataAssetCache{
			Strategy: model.DataAssetCacheStrategyWorkerCache,
			CacheKey: "fixture-cache/" + path,
		},
		TransferPolicy: model.DataAssetTransferPolicy{
			MaxConcurrentSourceTransfers: 1,
		},
	}
}

func dataOperatorSmokePublishTargets() []model.BoundPublishTarget {
	return []model.BoundPublishTarget{
		dataOperatorSmokePublishTarget("publish_composition_csv", "field_cdl_composition.csv"),
		dataOperatorSmokePublishTarget("publish_composition_audit", "field_cdl_composition.audit.csv"),
	}
}

func dataOperatorSmokePublishTarget(name, path string) model.BoundPublishTarget {
	return model.BoundPublishTarget{
		Name:            name,
		FromArtifact:    "field_cdl_composition",
		TargetName:      name,
		Location:        model.DataAssetLocation{Type: model.DataProviderRegisteredLocation, LocationName: "published_data", Path: "field_cdl_composition/year=2024/tile=fixture_tile_001/" + path},
		OverwritePolicy: model.PublishedDataAssetOverwriteFailIfExists,
	}
}

func openDataOperatorSmokeStore(t *testing.T, ctx context.Context) *persistence.Store {
	t.Helper()
	store, err := persistence.OpenStore(ctx, persistence.Config{Driver: persistence.DriverSQLite, ConnectionString: filepath.Join(t.TempDir(), "workflow.sqlite")})
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	return store
}

func insertDataOperatorSmokeRun(t *testing.T, ctx context.Context, store *persistence.Store, runID, createdAt string) {
	t.Helper()
	project := persistence.ProjectRecord{ID: "project-data-operator-smoke", Name: "data operator smoke", RepositoryIdentity: "local:fixture", ConfigPath: "project.json", ConfigSHA256: strings.Repeat("a", 64), CreatedAt: createdAt}
	if err := store.UpsertProject(ctx, project); err != nil {
		t.Fatalf("UpsertProject() error = %v", err)
	}
	workflowRecord := persistence.WorkflowRecord{ID: "workflow-data-operator-smoke", ProjectID: project.ID, Name: "data operator smoke", RepositoryIdentity: "local:fixture", WorkflowPath: "workflow.json", WorkflowSHA256: strings.Repeat("b", 64), CreatedAt: createdAt}
	if err := store.UpsertWorkflow(ctx, workflowRecord); err != nil {
		t.Fatalf("UpsertWorkflow() error = %v", err)
	}
	if err := store.CreateWorkflowRun(ctx, persistence.WorkflowRunRecord{ID: runID, ProjectID: project.ID, WorkflowID: workflowRecord.ID, SubmissionContextJSON: `{}`, CreatedAt: createdAt}); err != nil {
		t.Fatalf("CreateWorkflowRun() error = %v", err)
	}
	if err := store.InsertStagePlan(ctx, runID, []persistence.WorkflowStageRecord{
		{RunID: runID, StageIndex: 0, StepID: "field_cdl_composition_fixture", StageSourceReference: "fixture-stage-0", State: "ready", CreatedAt: createdAt, ReadyAt: createdAt},
	}); err != nil {
		t.Fatalf("InsertStagePlan() error = %v", err)
	}
}

func dataOperatorSmokePersistenceRecords(t *testing.T, runID string, compiled CompileStageResult, createdAt string) ([]persistence.WorkItemRecord, []persistence.QueuedWorkRecord, []persistence.WorkItemResourceConstraintRecord) {
	t.Helper()
	records := make([]persistence.WorkItemRecord, 0, len(compiled.WorkItems))
	queued := []persistence.QueuedWorkRecord{}
	constraints := []persistence.WorkItemResourceConstraintRecord{}
	for index, item := range compiled.WorkItems {
		payload, err := json.Marshal(item.WorkItem)
		if err != nil {
			t.Fatalf("marshal work item: %v", err)
		}
		payloadValue, err := dataOperatorSmokeDecodeJSON(payload)
		if err != nil {
			t.Fatalf("decode work item payload for hash: %v", err)
		}
		_, hash, err := fp.CanonicalJSONSHA256(payloadValue)
		if err != nil {
			t.Fatalf("hash work item: %v", err)
		}
		record := persistence.WorkItemRecord{
			ID:                   runID + ":" + item.WorkItem.ID,
			RunID:                runID,
			StageIndex:           item.StageIndex,
			WorkItemIndex:        index,
			WorkerPayloadJSON:    string(payload),
			ResolvedInputsSHA256: hash,
			CreatedAt:            createdAt,
		}
		records = append(records, record)
		if item.WorkItem.Type == model.WorkItemTypeCacheData {
			queued = append(queued, persistence.QueuedWorkRecord{WorkItemRecord: record, QueuedAt: createdAt})
		}
		for _, constraint := range item.ResourceConstraints {
			constraints = append(constraints, persistence.WorkItemResourceConstraintRecord{
				WorkItemID:      record.ID,
				ConstraintIndex: constraint.ConstraintIndex,
				ResourceKey:     constraint.ResourceKey,
				RequestedUnits:  constraint.RequestedUnits,
				Operator:        string(constraint.Operator),
				TargetUnits:     constraint.TargetUnits,
				CreatedAt:       createdAt,
			})
		}
	}
	return records, queued, constraints
}

func claimSmokeWork(t *testing.T, ctx context.Context, store *persistence.Store, attemptID string, want model.WorkItemType) persistence.ClaimedWorkRecord {
	t.Helper()
	claim, found, err := store.ClaimNextWork(ctx, persistence.ClaimWorkRequest{AttemptID: attemptID, ExecutorType: persistence.ExecutorTypeWorker, StartedAt: "2026-07-07T00:00:01Z"})
	if err != nil {
		t.Fatalf("ClaimNextWork() error = %v", err)
	}
	if !found {
		t.Fatalf("ClaimNextWork() found = false, want %s", want)
	}
	var item model.WorkItem
	if err := json.Unmarshal([]byte(claim.WorkItem.WorkerPayloadJSON), &item); err != nil {
		t.Fatalf("decode claimed work item: %v", err)
	}
	if item.Type != want {
		t.Fatalf("claimed type = %s, want %s", item.Type, want)
	}
	return claim
}

func completeSmokeAttempt(t *testing.T, ctx context.Context, store *persistence.Store, claim persistence.ClaimedWorkRecord, output any, completedAt string) {
	t.Helper()
	outputJSON, outputHash, err := dataOperatorSmokeCanonicalJSON(output)
	if err != nil {
		t.Fatalf("canonical output: %v", err)
	}
	preJSON, preHash, err := dataOperatorSmokeCanonicalJSON(map[string]any{"attempt_id": claim.AttemptID, "phase": "pre"})
	if err != nil {
		t.Fatalf("canonical pre-state: %v", err)
	}
	postJSON, postHash, err := dataOperatorSmokeCanonicalJSON(map[string]any{"attempt_id": claim.AttemptID, "phase": "post"})
	if err != nil {
		t.Fatalf("canonical post-state: %v", err)
	}
	if preJSON == "" || postJSON == "" {
		t.Fatal("pre/post canonical JSON unexpectedly empty")
	}
	if _, found, err := store.CompleteAttempt(ctx, persistence.CompleteAttemptRequest{
		AttemptID:        claim.AttemptID,
		OutputJSON:       outputJSON,
		OutputJSONSHA256: outputHash,
		PreStateSHA256:   preHash,
		PostStateSHA256:  postHash,
		CompletedAt:      completedAt,
	}); err != nil || !found {
		t.Fatalf("CompleteAttempt(%s) found=%v err=%v", claim.AttemptID, found, err)
	}
}

func dataOperatorSmokeCanonicalJSON(value any) (string, string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", "", err
	}
	decoded, err := dataOperatorSmokeDecodeJSON(data)
	if err != nil {
		return "", "", err
	}
	canonical, hash, err := fp.CanonicalJSONSHA256(decoded)
	return string(canonical), hash, err
}

func dataOperatorSmokeDecodeJSON(data []byte) (any, error) {
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.UseNumber()
	var decoded any
	if err := decoder.Decode(&decoded); err != nil {
		return nil, err
	}
	return decoded, nil
}

func materializedSmokeManifest(t *testing.T, claim persistence.ClaimedWorkRecord) model.MaterializedDataAssetManifest {
	t.Helper()
	var item model.WorkItem
	if err := json.Unmarshal([]byte(claim.WorkItem.WorkerPayloadJSON), &item); err != nil {
		t.Fatalf("decode cache_data payload: %v", err)
	}
	parameter := item.Parameters["cache_data"]
	data, err := json.Marshal(parameter.Value)
	if err != nil {
		t.Fatalf("marshal cache_data parameter: %v", err)
	}
	var payload model.CacheDataWorkItemPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("decode cache_data parameter: %v", err)
	}
	size := int64(21)
	return model.MaterializedDataAssetManifest{
		Schema:              model.MaterializedDataAssetManifestSchemaV1,
		AssetKey:            payload.AssetKey,
		TargetEnvironmentID: payload.TargetEnvironmentID,
		Assets: []model.MaterializedDataAsset{
			{
				BindingName:             payload.BindingName,
				ProviderName:            payload.ProviderName,
				ProviderType:            payload.ProviderType,
				Kind:                    payload.Kind,
				Format:                  payload.Format,
				LocalPath:               "/target/cache/" + payload.BindingName + ".csv",
				MaterializationStrategy: model.DataAssetCacheStrategyWorkerCache,
				CacheKey:                payload.Cache.CacheKey,
				SourceSizeBytes:         &size,
				SourceSHA256:            strings.Repeat("c", 64),
			},
		},
	}
}

func artifactSmokeManifest(t *testing.T, workItemID string) model.ArtifactManifest {
	t.Helper()
	size := int64(64)
	return model.ArtifactManifest{
		Schema:       model.ArtifactManifestSchemaV1,
		WorkItemID:   strings.TrimPrefix(workItemID, "run-data-operator-smoke:"),
		StorageScope: "worker_data",
		Artifacts: []model.ArtifactDescriptor{
			{
				Name:        "field_cdl_composition",
				Kind:        model.ArtifactKindFile,
				Format:      "csv",
				Path:        "artifacts/raw/field-cdl-composition/field_cdl_composition.csv",
				ContentType: "text/csv",
				SizeBytes:   &size,
				SHA256:      strings.Repeat("d", 64),
			},
		},
	}
}

func publishedSmokeManifest(t *testing.T, name string) model.PublishedDataAssetManifest {
	t.Helper()
	size := int64(64)
	return model.PublishedDataAssetManifest{
		Schema:              model.PublishedDataAssetManifestSchemaV1,
		TargetEnvironmentID: "target-local",
		PublishedAssets: []model.PublishedDataAsset{
			{
				Name:            name,
				FromWorkItemID:  "field-cdl-composition-2024",
				FromArtifact:    "field_cdl_composition",
				ContentType:     "text/csv",
				StorageScope:    model.DataLocationTypeRegistered,
				LocationName:    "published_data",
				Path:            "field_cdl_composition/year=2024/tile=fixture_tile_001/" + name + ".csv",
				SizeBytes:       &size,
				SHA256:          strings.Repeat("e", 64),
				OverwritePolicy: model.PublishedDataAssetOverwriteFailIfExists,
			},
		},
	}
}

func dataOperatorSmokeRecordByType(t *testing.T, records []persistence.WorkItemRecord, itemType model.WorkItemType) persistence.WorkItemRecord {
	t.Helper()
	items := dataOperatorSmokeRecordsByType(t, records, itemType)
	if len(items) != 1 {
		t.Fatalf("record count for %s = %d, want 1", itemType, len(items))
	}
	return items[0]
}

func dataOperatorSmokeRecordsByType(t *testing.T, records []persistence.WorkItemRecord, itemType model.WorkItemType) []persistence.WorkItemRecord {
	t.Helper()
	var matches []persistence.WorkItemRecord
	for _, record := range records {
		var item model.WorkItem
		if err := json.Unmarshal([]byte(record.WorkerPayloadJSON), &item); err != nil {
			t.Fatalf("decode record payload: %v", err)
		}
		if item.Type == itemType {
			matches = append(matches, record)
		}
	}
	return matches
}
