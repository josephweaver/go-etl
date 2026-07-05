package main

import (
	"encoding/json"
	"testing"
	"time"

	"goetl/internal/model"
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

	records, queued, memberships, err := persistenceRecordsFromCompiledStageResults("run-001", []workflow.CompileStageResult{stageResult}, "v1", time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC))
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

	expectedIndexes := []int{1, 0, 0}
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

	recordsAgain, _, _, err := persistenceRecordsFromCompiledStageResults("run-002", []workflow.CompileStageResult{stageResult}, "v1", time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC))
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
