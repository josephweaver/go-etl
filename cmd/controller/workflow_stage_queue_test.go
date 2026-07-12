package main

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"goetl/internal/model"
	"goetl/internal/variable"
	"goetl/internal/workflow"
)

func TestSplitCompiledWorkflowByStageMapsWorkItemIndexesByStep(t *testing.T) {
	compileResult := workflow.CompileResult{
		WorkflowID: "cdl",
		WorkItems: []workflow.CompiledWorkItem{
			{
				WorkflowID: "cdl",
				StepID:     "extract",
				WorkItem:   model.WorkItem{ID: "extract-a"},
			},
			{
				WorkflowID: "cdl",
				StepID:     "extract",
				WorkItem:   model.WorkItem{ID: "extract-b"},
			},
			{
				WorkflowID: "cdl",
				StepID:     "transform",
				WorkItem:   model.WorkItem{ID: "transform-a"},
			},
			{
				WorkflowID: "cdl",
				StepID:     "transform",
				WorkItem:   model.WorkItem{ID: "transform-b"},
			},
			{
				WorkflowID: "cdl",
				StepID:     "load",
				WorkItem:   model.WorkItem{ID: "load-a"},
			},
		},
	}
	plan := workflow.WorkflowPlan{
		WorkflowID: "cdl",
		StepCount:  3,
		Stages: []workflow.WorkflowStage{
			{
				Index: 0,
				Steps: []workflow.WorkflowStageStep{
					{StageIndex: 0, StepIndex: 0, StepID: "extract"},
				},
			},
			{
				Index: 1,
				Steps: []workflow.WorkflowStageStep{
					{StageIndex: 1, StepIndex: 1, StepID: "transform"},
					{StageIndex: 1, StepIndex: 2, StepID: "load"},
				},
			},
		},
	}

	stages, err := splitCompiledWorkflowByStage(compileResult, plan)
	if err != nil {
		t.Fatalf("splitCompiledWorkflowByStage() error = %v", err)
	}
	if len(stages) != 2 {
		t.Fatalf("len(stages) = %d, want 2", len(stages))
	}
	if len(stages[1].WorkItems) != 3 {
		t.Fatalf("stage 1 work-item count = %d, want 3", len(stages[1].WorkItems))
	}

	expectedItems := []struct {
		stepIndex     int
		workItemIndex int
		stepID        string
		id            string
		stageIndex    int
		indexInStage  int
	}{
		{stepIndex: 0, workItemIndex: 0, stepID: "extract", id: "extract-a", stageIndex: 0, indexInStage: 0},
		{stepIndex: 0, workItemIndex: 1, stepID: "extract", id: "extract-b", stageIndex: 0, indexInStage: 1},
		{stepIndex: 1, workItemIndex: 0, stepID: "transform", id: "transform-a", stageIndex: 1, indexInStage: 0},
		{stepIndex: 1, workItemIndex: 1, stepID: "transform", id: "transform-b", stageIndex: 1, indexInStage: 1},
		{stepIndex: 2, workItemIndex: 0, stepID: "load", id: "load-a", stageIndex: 1, indexInStage: 2},
	}
	for _, item := range expectedItems {
		got := stages[item.stageIndex].WorkItems[item.indexInStage]
		if got.WorkItemIndex != item.workItemIndex {
			t.Fatalf("stages[%d].WorkItems[%d].WorkItemIndex = %d, want %d", item.stageIndex, item.indexInStage, got.WorkItemIndex, item.workItemIndex)
		}
		if got.StepIndex != item.stepIndex {
			t.Fatalf("stages[%d].WorkItems[%d].StepIndex = %d, want %d", item.stageIndex, item.indexInStage, got.StepIndex, item.stepIndex)
		}
		if got.StepID != item.stepID {
			t.Fatalf("stages[%d].WorkItems[%d].StepID = %q, want %q", item.stageIndex, item.indexInStage, got.StepID, item.stepID)
		}
		if got.WorkItem.ID != item.id {
			t.Fatalf("stages[%d].WorkItems[%d].WorkItem.ID = %q, want %q", item.stageIndex, item.indexInStage, got.WorkItem.ID, item.id)
		}
	}
}

func TestPersistenceRecordsFromCompiledStageResultsIncludesDeterministicMetadata(t *testing.T) {
	stageResult := workflow.CompileStageResult{
		WorkflowID: "cdl",
		StageIndex: 1,
		WorkItems: []workflow.CompileStageWorkItem{
			{
				WorkflowID:    "cdl",
				StageIndex:    1,
				StepIndex:     0,
				StepID:        "download",
				WorkItemIndex: 1,
				WorkItem: model.WorkItem{
					ID:             "download-002",
					Type:           model.WorkItemTypeWriteDemoOutput,
					OutputFilename: "download-002.txt",
					Parameters: model.Parameters{
						"item_index": {Type: "int", Value: 2},
					},
				},
			},
			{
				WorkflowID:    "cdl",
				StageIndex:    1,
				StepIndex:     0,
				StepID:        "download",
				WorkItemIndex: 0,
				WorkItem: model.WorkItem{
					ID:             "download-001",
					Type:           model.WorkItemTypeWriteDemoOutput,
					OutputFilename: "download-001.txt",
					Parameters: model.Parameters{
						"item_index": {Type: "int", Value: 1},
					},
				},
			},
			{
				WorkflowID:    "cdl",
				StageIndex:    1,
				StepIndex:     2,
				StepID:        "summarize",
				WorkItemIndex: 0,
				WorkItem: model.WorkItem{
					ID:             "summarize-001",
					Type:           model.WorkItemTypeSummarizeInputFile,
					OutputFilename: "summarize-001.txt",
					Parameters: model.Parameters{
						"input_path": {Type: "path", Value: "input.txt"},
					},
				},
			},
		},
	}

	records, queued, memberships, _, err := persistenceRecordsFromCompiledStageResults("run-001", []workflow.CompileStageResult{stageResult}, "v1", time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("persistenceRecordsFromCompiledStageResults() error = %v", err)
	}
	if len(records) != 3 {
		t.Fatalf("records count = %d, want 3", len(records))
	}
	if len(queued) != 3 {
		t.Fatalf("queued count = %d, want 3", len(queued))
	}
	if len(memberships) != 3 {
		t.Fatalf("membership count = %d, want 3", len(memberships))
	}

	expectedWorkItemIDs := []string{"run-001:download-002", "run-001:download-001", "run-001:summarize-001"}
	for index, expected := range expectedWorkItemIDs {
		if records[index].ID != expected {
			t.Fatalf("record[%d].ID = %q, want %q", index, records[index].ID, expected)
		}
		if memberships[index].workItemID != expected {
			t.Fatalf("membership[%d].workItemID = %q, want %q", index, memberships[index].workItemID, expected)
		}
	}

	expectedIndexes := []int{0, 1, 2}
	for index, expected := range expectedIndexes {
		if records[index].WorkItemIndex != expected {
			t.Fatalf("records[%d].WorkItemIndex = %d, want %d", index, records[index].WorkItemIndex, expected)
		}
	}

	var firstPayload model.WorkItem
	if err := json.Unmarshal([]byte(records[0].WorkerPayloadJSON), &firstPayload); err != nil {
		t.Fatalf("decode first worker payload: %v", err)
	}
	if firstPayload.WorkflowInstanceID != "run-001" {
		t.Fatalf("firstPayload.WorkflowInstanceID = %q, want run-001", firstPayload.WorkflowInstanceID)
	}
	if firstPayload.StepInstanceID != "run-001:step:0" {
		t.Fatalf("firstPayload.StepInstanceID = %q, want run-001:step:0", firstPayload.StepInstanceID)
	}
	if firstPayload.WorkflowDefinitionID != "cdl" {
		t.Fatalf("firstPayload.WorkflowDefinitionID = %q, want cdl", firstPayload.WorkflowDefinitionID)
	}
	if firstPayload.StepDefinitionID != "download" {
		t.Fatalf("firstPayload.StepDefinitionID = %q, want download", firstPayload.StepDefinitionID)
	}
	if firstPayload.CodeVersion != "v1" {
		t.Fatalf("firstPayload.CodeVersion = %q, want v1", firstPayload.CodeVersion)
	}

	recordsAgain, _, _, _, err := persistenceRecordsFromCompiledStageResults("run-002", []workflow.CompileStageResult{stageResult}, "v1", time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("persistenceRecordsFromCompiledStageResults(second run) error = %v", err)
	}
	var secondPayload model.WorkItem
	if err := json.Unmarshal([]byte(recordsAgain[0].WorkerPayloadJSON), &secondPayload); err != nil {
		t.Fatalf("decode second worker payload: %v", err)
	}
	if secondPayload.StepInstanceID == firstPayload.StepInstanceID {
		t.Fatal("step instance id must differ across submissions")
	}
}

func TestPersistenceRecordsFromCompiledStageResultsStampsResourceConstraints(t *testing.T) {
	stageResult := workflow.CompileStageResult{
		WorkflowID: "cdl",
		StageIndex: 0,
		WorkItems: []workflow.CompileStageWorkItem{
			{
				WorkflowID:    "cdl",
				StageIndex:    0,
				StepIndex:     0,
				StepID:        "download",
				WorkItemIndex: 0,
				WorkItem: model.WorkItem{
					ID:             "download-001",
					Type:           model.WorkItemTypeWriteDemoOutput,
					OutputFilename: "download-001.txt",
				},
				ResourceConstraints: []model.WorkItemResourceConstraint{
					{
						WorkItemID:      "download-001",
						ConstraintIndex: 0,
						ResourceKey:     "target:local/memory-mib",
						RequestedUnits:  512,
						Operator:        model.WorkItemResourceConstraintOperatorLessEq,
						TargetUnits:     2048,
					},
				},
			},
		},
	}

	_, _, _, constraints, err := persistenceRecordsFromCompiledStageResults("run-001", []workflow.CompileStageResult{stageResult}, "v1", time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("persistenceRecordsFromCompiledStageResults() error = %v", err)
	}
	if len(constraints) != 1 {
		t.Fatalf("constraint count = %d, want 1", len(constraints))
	}
	if constraints[0].WorkItemID != "run-001:download-001" {
		t.Fatalf("constraint work item id = %q, want run-scoped id", constraints[0].WorkItemID)
	}
	if constraints[0].CreatedAt != "2026-07-05T12:00:00Z" {
		t.Fatalf("constraint created_at = %q, want submitted timestamp", constraints[0].CreatedAt)
	}
	if constraints[0].RequestedUnits != 512 {
		t.Fatalf("constraint requested units = %d, want 512", constraints[0].RequestedUnits)
	}
}

func TestPersistenceRecordsFromCompiledStageResultsPersistsProtectedRefsWithoutPlaintext(t *testing.T) {
	const sentinel = "goet-controller-should-not-store-this-secret-003"
	stageResult := workflow.CompileStageResult{
		WorkflowID: "cdl",
		StageIndex: 0,
		WorkItems: []workflow.CompileStageWorkItem{
			{
				WorkflowID:    "cdl",
				StageIndex:    0,
				StepIndex:     0,
				StepID:        "download",
				WorkItemIndex: 0,
				WorkItem: model.WorkItem{
					ID:             "download-private",
					Type:           model.WorkItemTypePythonScript,
					OutputFilename: "download-private.json",
					Parameters: model.Parameters{
						"year": {Type: "int", Value: 2026},
						"gdrive_token": {
							Type:         "string",
							ProtectedRef: &variable.ProtectedRef{Provider: "worker_env", Key: "GOET_GDRIVE_TOKEN"},
							Materialize:  &model.ParameterMaterialization{Mode: "env", Target: "GDRIVE_TOKEN"},
						},
					},
				},
			},
		},
	}

	records, queued, _, _, err := persistenceRecordsFromCompiledStageResults("run-001", []workflow.CompileStageResult{stageResult}, "v1", time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("persistenceRecordsFromCompiledStageResults() error = %v", err)
	}
	if len(records) != 1 || len(queued) != 1 {
		t.Fatalf("records=%d queued=%d, want one persisted queued work item", len(records), len(queued))
	}
	if strings.Contains(records[0].WorkerPayloadJSON, sentinel) {
		t.Fatalf("worker payload leaked sentinel: %s", records[0].WorkerPayloadJSON)
	}
	if strings.Contains(records[0].ResolvedInputsSHA256, sentinel) {
		t.Fatalf("resolved input hash leaked sentinel: %s", records[0].ResolvedInputsSHA256)
	}

	var payload model.WorkItem
	if err := json.Unmarshal([]byte(records[0].WorkerPayloadJSON), &payload); err != nil {
		t.Fatalf("decode worker payload: %v", err)
	}
	if payload.ExecutionEnvelope == nil {
		t.Fatal("execution envelope = nil")
	}
	if got := payload.ExecutionEnvelope.Variables.Public["year"].Value; got != float64(2026) {
		t.Fatalf("public year = %#v, want 2026", got)
	}
	ref := payload.ExecutionEnvelope.Variables.ProtectedRefs["gdrive_token"]
	if ref.Provider != "worker_env" || ref.Key != "GOET_GDRIVE_TOKEN" {
		t.Fatalf("protected ref = %+v", ref)
	}
	if ref.RedactionLabel != "${worker_env.GOET_GDRIVE_TOKEN}" {
		t.Fatalf("redaction label = %q", ref.RedactionLabel)
	}
	if payload.Parameters["gdrive_token"].Value != nil {
		t.Fatalf("parameter plaintext value = %#v, want nil", payload.Parameters["gdrive_token"].Value)
	}
}

func TestPersistenceRecordsFromCompiledStageResultsRejectsSensitivePlaintext(t *testing.T) {
	stageResult := workflow.CompileStageResult{
		WorkflowID: "cdl",
		StageIndex: 0,
		WorkItems: []workflow.CompileStageWorkItem{
			{
				WorkflowID:    "cdl",
				StageIndex:    0,
				StepIndex:     0,
				StepID:        "download",
				WorkItemIndex: 0,
				WorkItem: model.WorkItem{
					ID:             "download-private",
					Type:           model.WorkItemTypePythonScript,
					OutputFilename: "download-private.json",
					Parameters: model.Parameters{
						"gdrive_token": {
							Type:           "string",
							Value:          "goet-controller-should-not-store-this-secret-003",
							Sensitive:      true,
							RedactionLabel: "[REDACTED:workflow.gdrive_token]",
						},
					},
				},
			},
		},
	}

	_, _, _, _, err := persistenceRecordsFromCompiledStageResults("run-001", []workflow.CompileStageResult{stageResult}, "v1", time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC))
	if err == nil {
		t.Fatal("expected sensitive plaintext parameter to be rejected")
	}
}

func TestPersistenceRecordsFromCompiledStageResultsQueuesAssetMaterializeBeforeCompute(t *testing.T) {
	stageResult := workflow.CompileStageResult{
		WorkflowID: "cdl",
		StageIndex: 0,
		WorkItems: []workflow.CompileStageWorkItem{
			{
				WorkflowID:    "cdl",
				StageIndex:    0,
				StepIndex:     0,
				StepID:        "materialize",
				WorkItemIndex: 0,
				WorkItem: model.WorkItem{
					ID:             "asset-materialize-001",
					Type:           model.WorkItemTypeAssetMaterialize,
					OutputFilename: "asset-materialize-001.json",
					Parameters: model.Parameters{
						"target_environment_id": {Type: "string", Value: "target-local"},
					},
				},
			},
			{
				WorkflowID:    "cdl",
				StageIndex:    0,
				StepIndex:     1,
				StepID:        "compute",
				WorkItemIndex: 0,
				WorkItem: model.WorkItem{
					ID:             "compute-001",
					Type:           model.WorkItemTypePythonScript,
					OutputFilename: "compute-001.json",
					DependsOn:      []string{"asset-materialize-001"},
					Parameters: model.Parameters{
						"python_entrypoint":     {Type: "path", Value: "scripts/run.py"},
						"target_environment_id": {Type: "string", Value: "target-local"},
					},
				},
			},
		},
	}

	records, queued, memberships, _, err := persistenceRecordsFromCompiledStageResults("run-001", []workflow.CompileStageResult{stageResult}, "v1", time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("persistenceRecordsFromCompiledStageResults() error = %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("records count = %d, want asset_materialize and compute", len(records))
	}
	if len(queued) != 1 {
		t.Fatalf("queued count = %d, want only asset_materialize queued", len(queued))
	}
	var queuedPayload model.WorkItem
	if err := json.Unmarshal([]byte(queued[0].WorkerPayloadJSON), &queuedPayload); err != nil {
		t.Fatalf("decode queued payload: %v", err)
	}
	if queuedPayload.Type != model.WorkItemTypeAssetMaterialize {
		t.Fatalf("queued payload type = %s, want asset_materialize", queuedPayload.Type)
	}
	if len(memberships) != 0 {
		t.Fatalf("memberships = %+v, want compute membership deferred until dependency is queued", memberships)
	}

	var computePayload model.WorkItem
	for _, record := range records {
		var payload model.WorkItem
		if err := json.Unmarshal([]byte(record.WorkerPayloadJSON), &payload); err != nil {
			t.Fatalf("decode record payload: %v", err)
		}
		if payload.Type == model.WorkItemTypePythonScript {
			computePayload = payload
		}
	}
	if len(computePayload.DependsOn) != 1 || computePayload.DependsOn[0] != queued[0].ID {
		t.Fatalf("compute depends_on = %+v, want queued asset_materialize id %s", computePayload.DependsOn, queued[0].ID)
	}
}

func TestPersistenceRecordsFromCompiledStageResultsQueuesPriorStageDependency(t *testing.T) {
	stageResult := workflow.CompileStageResult{
		WorkflowID: "cdl",
		StageIndex: 5,
		WorkItems: []workflow.CompileStageWorkItem{
			{
				WorkflowID:    "cdl",
				StageIndex:    5,
				StepIndex:     5,
				StepID:        "publish-delivery",
				WorkItemIndex: 0,
				WorkItem: model.WorkItem{
					ID:             "publish-delivery-delivery",
					Type:           model.WorkItemTypeCommitData,
					OutputFilename: "publish-delivery-delivery.json",
					DependsOn:      []string{"package-delivery-delivery"},
					Parameters: model.Parameters{
						"target_environment_id": {Type: "string", Value: "target-local"},
						"commit_data": {
							Type: "commit_data",
							Value: model.CommitDataWorkItemPayload{
								Operator:            string(model.WorkItemTypeCommitData),
								TargetEnvironmentID: "target-local",
								Source: model.CommitDataSource{
									FromWorkItemID: "package-delivery-delivery",
									FromArtifact:   "delivery_package",
								},
								PublishTarget: model.BoundPublishTarget{
									Name:         "delivery_package",
									FromArtifact: "delivery_package",
									TargetName:   "delivery_package",
									Location: model.DataAssetLocation{
										Type:         model.DataProviderRegisteredLocation,
										LocationName: "published_data",
										Path:         "delivery/package.zip",
									},
								},
							},
						},
					},
				},
			},
		},
	}

	records, queued, memberships, _, err := persistenceRecordsFromCompiledStageResults("run-001", []workflow.CompileStageResult{stageResult}, "v1", time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("persistenceRecordsFromCompiledStageResults() error = %v", err)
	}
	if len(records) != 1 || len(queued) != 1 {
		t.Fatalf("records=%d queued=%d, want prior-stage dependent work queued", len(records), len(queued))
	}
	if len(memberships) != 1 {
		t.Fatalf("membership count = %d, want 1", len(memberships))
	}
	var queuedPayload model.WorkItem
	if err := json.Unmarshal([]byte(queued[0].WorkerPayloadJSON), &queuedPayload); err != nil {
		t.Fatalf("decode queued payload: %v", err)
	}
	if got, want := queuedPayload.DependsOn, []string{"run-001:package-delivery-delivery"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("depends_on = %+v, want %+v", got, want)
	}
}
