package workflow

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strconv"
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
	compiled, err := compileDataOperatorSmokeWorkflow(t, workflowDef, plan)
	if err != nil {
		t.Fatalf("compileDataOperatorSmokeWorkflow() error = %v", err)
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
	if _, ok := compute.WorkItem.Parameters["data_assets"]; ok {
		t.Fatal("compute item still has data_assets parameter")
	}
	if _, ok := compute.WorkItem.Parameters["publish"]; ok {
		t.Fatal("compute item still has publish parameter")
	}
	if len(compute.WorkItem.DependsOn) != 0 {
		t.Fatalf("compute depends_on = %+v, want no hidden cache_data dependencies", compute.WorkItem.DependsOn)
	}
	for _, commit := range commitItems {
		if len(commit.WorkItem.DependsOn) != 1 || commit.WorkItem.DependsOn[0] != compute.WorkItem.ID {
			t.Fatalf("commit depends_on = %+v, want compute %s", commit.WorkItem.DependsOn, compute.WorkItem.ID)
		}
	}

	runID := "run-data-operator-smoke"
	createdAt := "2026-07-07T00:00:00Z"
	insertDataOperatorSmokeRun(t, ctx, store, runID, plan, createdAt)
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
			if blocked, found, err := store.ClaimNextWork(ctx, smokeWorkerClaimRequest(t, ctx, store, "attempt-cache-blocked", "2026-07-07T00:00:01Z")); err != nil {
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
	if blocked, found, err := store.ClaimNextWork(ctx, smokeWorkerClaimRequest(t, ctx, store, "attempt-commit-blocked", "2026-07-07T00:00:06Z")); err != nil {
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

func compileDataOperatorSmokeWorkflow(t *testing.T, workflowDef Workflow, plan WorkflowPlan) (CompileStageResult, error) {
	t.Helper()

	compiled := CompileStageResult{
		WorkflowID: workflowDef.ID,
		StageIndex: -1,
	}
	resolver := testWorkflowResolver(t, 2024)
	for stageIndex := range plan.Stages {
		stage, err := CompileWorkflowStage(resolver, workflowDef, plan, stageIndex)
		if err != nil {
			return CompileStageResult{}, err
		}
		compiled.Steps = append(compiled.Steps, stage.Steps...)
		compiled.WorkItems = append(compiled.WorkItems, stage.WorkItems...)
	}
	return compiled, nil
}

func dataOperatorSmokeWorkflow() Workflow {
	definitions := dataOperatorSmokeDataDefinitions()
	return Workflow{
		ID: "field-cdl-composition-fixture",
		Steps: []Step{
			dataOperatorSmokeCacheStep("cache-crop-lookup", "crop_lookup_fixture", definitions, ""),
			dataOperatorSmokeCacheStep("cache-field-tile", "field_tile_fixture", definitions, "data-cache"),
			dataOperatorSmokeCacheStep("cache-cdl-tile", "cdl_tile_fixture", definitions, "data-cache"),
			{
				ID: "field-cdl-composition",
				FanOut: &FanOutStep{
					WorkItem: FanOutWorkItemTemplate{
						FanOutExpression: "${years[*]}",
						Type:             model.WorkItemTypePythonScript,
						OutputPrefix:     "field-cdl-composition",
						OutputExtension:  ".json",
						Parameters: model.Parameters{
							"python_entrypoint":     {Type: "path", Value: "scripts/field_cdl_composition.py"},
							"target_environment_id": {Type: "string", Value: "target-local"},
						},
					},
				},
			},
			dataOperatorSmokeCommitStep("commit-composition-csv", "publish_composition_csv", definitions, ""),
			dataOperatorSmokeCommitStep("commit-composition-audit", "publish_composition_audit", definitions, "publish-data"),
		},
	}
}

func dataOperatorSmokeCacheStep(id, asset string, definitions model.DataDefinitions, parallelWith string) Step {
	return Step{
		ID:           id,
		ParallelWith: parallelWith,
		FanOut: &FanOutStep{
			WorkItem: FanOutWorkItemTemplate{
				FanOutExpression: "${years[*]}",
				Type:             model.WorkItemTypeCacheData,
				OutputPrefix:     id,
				OutputExtension:  ".json",
				Parameters: model.Parameters{
					"target_environment_id": {Type: "string", Value: "target-local"},
				},
				ExplicitCacheData: &ExplicitCacheDataTemplate{
					Definitions: definitions,
					Alias:       asset,
					Asset:       asset,
				},
			},
		},
	}
}

func dataOperatorSmokeCommitStep(id, target string, definitions model.DataDefinitions, parallelWith string) Step {
	return Step{
		ID:           id,
		ParallelWith: parallelWith,
		FanOut: &FanOutStep{
			WorkItem: FanOutWorkItemTemplate{
				FanOutExpression: "${years[*]}",
				Type:             model.WorkItemTypeCommitData,
				OutputPrefix:     id,
				OutputExtension:  ".json",
				Parameters: model.Parameters{
					"target_environment_id": {Type: "string", Value: "target-local"},
				},
				ExplicitCommitData: &ExplicitCommitDataTemplate{
					Definitions:  definitions,
					Alias:        target,
					Target:       target,
					FromStep:     "field-cdl-composition",
					FromArtifact: "field_cdl_composition",
				},
			},
		},
	}
}

func dataOperatorSmokeDataDefinitions() model.DataDefinitions {
	return model.DataDefinitions{
		Inputs: map[string]model.DataInputDefinition{
			"crop_lookup_fixture": dataOperatorSmokeInputDefinition("crop_lookup_provider", "crop_lookup.csv"),
			"field_tile_fixture":  dataOperatorSmokeInputDefinition("field_tile_provider", "field_tile.csv"),
			"cdl_tile_fixture":    dataOperatorSmokeInputDefinition("cdl_tile_provider", "cdl_tile.csv"),
		},
		Outputs: map[string]model.DataOutputDefinition{
			"publish_composition_csv":   dataOperatorSmokeOutputDefinition("field_cdl_composition.csv"),
			"publish_composition_audit": dataOperatorSmokeOutputDefinition("field_cdl_composition.audit.csv"),
		},
	}
}

func dataOperatorSmokeInputDefinition(providerName, path string) model.DataInputDefinition {
	return model.DataInputDefinition{
		Kind:   "fixture_matrix",
		Format: "csv",
		Binding: model.DataInputBindingDefinition{
			ProviderName: providerName,
			Provider:     model.DataProviderLocalFile,
			Location: model.DataDefinitionLocation{
				Name: "fixture_data",
				Path: path,
			},
			Cache: model.DataDefinitionCache{
				Strategy: model.DataAssetCacheStrategyWorkerCache,
				CacheKey: "fixture-cache/" + path,
			},
			Materialization: model.DataDefinitionMaterialization{
				Scope:    model.DataMaterializationScopeShared,
				Strategy: model.DataAssetCacheStrategyWorkerCache,
			},
			TransferPolicy: model.DataAssetTransferPolicy{
				MaxConcurrentSourceTransfers: 1,
			},
		},
	}
}

func dataOperatorSmokeOutputDefinition(path string) model.DataOutputDefinition {
	return model.DataOutputDefinition{
		Kind:   "fixture_matrix",
		Format: "csv",
		Binding: model.DataOutputBindingDefinition{
			Provider: model.DataProviderRegisteredLocation,
			Location: model.DataDefinitionLocation{
				Name: "published_data",
				Path: "field_cdl_composition/year=2024/tile=fixture_tile_001/" + path,
			},
			OverwritePolicy: model.PublishedDataAssetOverwriteFailIfExists,
		},
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

func insertDataOperatorSmokeRun(t *testing.T, ctx context.Context, store *persistence.Store, runID string, plan WorkflowPlan, createdAt string) {
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
	stages := make([]persistence.WorkflowStageRecord, 0, len(plan.Stages))
	for _, stage := range plan.Stages {
		stepID := ""
		if len(stage.Steps) > 0 {
			stepID = stage.Steps[0].StepID
		}
		stages = append(stages, persistence.WorkflowStageRecord{RunID: runID, StageIndex: stage.Index, StepID: stepID, StageSourceReference: "fixture-stage-" + strconv.Itoa(stage.Index), State: "ready", CreatedAt: createdAt, ReadyAt: createdAt})
	}
	if err := store.InsertStagePlan(ctx, runID, stages); err != nil {
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
	claim, found, err := store.ClaimNextWork(ctx, smokeWorkerClaimRequest(t, ctx, store, attemptID, "2026-07-07T00:00:01Z"))
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

func smokeWorkerClaimRequest(t *testing.T, ctx context.Context, store *persistence.Store, attemptID string, startedAt string) persistence.ClaimWorkRequest {
	t.Helper()
	workerID := "worker-" + attemptID
	sessionID := "session-" + attemptID
	if _, err := store.RegisterWorkerSession(ctx, persistence.RegisterWorkerSessionRequest{
		WorkerID:     workerID,
		SessionID:    sessionID,
		RegisteredAt: "2999-01-01T00:00:00Z",
	}); err != nil {
		t.Fatalf("RegisterWorkerSession(%s) error = %v", sessionID, err)
	}
	return persistence.ClaimWorkRequest{
		AttemptID:       attemptID,
		WorkerID:        workerID,
		WorkerSessionID: sessionID,
		ExecutorType:    persistence.ExecutorTypeWorker,
		StartedAt:       startedAt,
	}
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
