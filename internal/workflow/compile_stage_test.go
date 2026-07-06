package workflow

import (
	"strings"
	"testing"

	"goetl/internal/model"
	"goetl/internal/variable"
)

func TestCompileWorkflowStageSequentialWorkflow(t *testing.T) {
	resolver := testWorkflowResolver(t, 2024, 2025)
	workflow := Workflow{
		ID: "cdl",
		Steps: []Step{
			{
				ID: "download",
				FanOut: &FanOutStep{
					WorkItem: FanOutWorkItemTemplate{
						FanOutExpression: "${years[*]}",
						Type:             model.WorkItemTypeWriteDemoOutput,
						IDPrefix:         "download",
						OutputPrefix:     "download",
						OutputExtension:  ".txt",
					},
				},
			},
			{
				ID: "summarize",
				FanOut: &FanOutStep{
					WorkItem: FanOutWorkItemTemplate{
						FanOutExpression: "${years[*]}",
						Type:             model.WorkItemTypeWriteDemoOutput,
						IDPrefix:         "summarize",
						OutputPrefix:     "summarize",
						OutputExtension:  ".txt",
					},
				},
			},
		},
	}
	plan, err := NormalizeStages(workflow)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	stage0, err := CompileWorkflowStage(resolver, workflow, plan, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stage0.StageIndex != 0 {
		t.Fatalf("unexpected stage index: %d", stage0.StageIndex)
	}
	if len(stage0.WorkItems) != 2 {
		t.Fatalf("unexpected stage 0 item count: %d", len(stage0.WorkItems))
	}
	if stage0.WorkItems[0].StepID != "download" {
		t.Fatalf("unexpected step id: %s", stage0.WorkItems[0].StepID)
	}
	if stage0.WorkItems[0].StepIndex != 0 {
		t.Fatalf("unexpected step index: %d", stage0.WorkItems[0].StepIndex)
	}

	stage1, err := CompileWorkflowStage(resolver, workflow, plan, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stage1.StageIndex != 1 {
		t.Fatalf("unexpected stage index: %d", stage1.StageIndex)
	}
	if len(stage1.WorkItems) != 2 {
		t.Fatalf("unexpected stage 1 item count: %d", len(stage1.WorkItems))
	}
	if stage1.WorkItems[0].StepID != "summarize" {
		t.Fatalf("unexpected step id: %s", stage1.WorkItems[0].StepID)
	}
	if stage1.WorkItems[0].StepIndex != 1 {
		t.Fatalf("unexpected step index: %d", stage1.WorkItems[0].StepIndex)
	}
}

func TestCompileWorkflowStageParallelStageIsOrderedByStepThenFanOut(t *testing.T) {
	resolver := testWorkflowResolver(t, 2024, 2025, 2026)
	workflow := Workflow{
		ID: "cdl",
		Steps: []Step{
			{
				ID:           "extract",
				ParallelWith: "batch",
				FanOut: &FanOutStep{
					WorkItem: FanOutWorkItemTemplate{
						FanOutExpression: "${years[*]}",
						Type:             model.WorkItemTypeWriteDemoOutput,
						IDPrefix:         "extract",
						OutputPrefix:     "extract",
						OutputExtension:  ".txt",
					},
				},
			},
			{
				ID:           "transform",
				ParallelWith: "batch",
				FanOut: &FanOutStep{
					WorkItem: FanOutWorkItemTemplate{
						FanOutExpression: "${years[*]}",
						Type:             model.WorkItemTypeWriteDemoOutput,
						IDPrefix:         "transform",
						OutputPrefix:     "transform",
						OutputExtension:  ".txt",
					},
				},
			},
		},
	}
	plan, err := NormalizeStages(workflow)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	result, err := CompileWorkflowStage(resolver, workflow, plan, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.WorkItems) != 6 {
		t.Fatalf("unexpected item count: %d", len(result.WorkItems))
	}
	expectedIDs := []string{
		"extract-2024",
		"extract-2025",
		"extract-2026",
		"transform-2024",
		"transform-2025",
		"transform-2026",
	}
	for index, expected := range expectedIDs {
		if result.WorkItems[index].WorkItem.ID != expected {
			t.Fatalf("unexpected item %d: %s", index, result.WorkItems[index].WorkItem.ID)
		}
		if result.WorkItems[index].StepIndex != index/3 {
			t.Fatalf("unexpected work-item step index at %d: %d", index, result.WorkItems[index].StepIndex)
		}
		if result.WorkItems[index].WorkItemIndex != index%3 {
			t.Fatalf("unexpected work-item index at %d: %d", index, result.WorkItems[index].WorkItemIndex)
		}
	}
}

func TestCompileWorkflowStageIncludesMetadataAndSteps(t *testing.T) {
	resolver := testWorkflowResolver(t, 2024, 2025)
	workflow := Workflow{
		ID: "cdl",
		Steps: []Step{
			{
				ID:           "extract",
				ParallelWith: "batch",
				FanOut: &FanOutStep{
					WorkItem: FanOutWorkItemTemplate{
						FanOutExpression: "${years[*]}",
						Type:             model.WorkItemTypeWriteDemoOutput,
						IDPrefix:         "extract",
						OutputPrefix:     "extract",
						OutputExtension:  ".txt",
					},
				},
			},
			{
				ID:           "transform",
				ParallelWith: "batch",
				FanOut: &FanOutStep{
					WorkItem: FanOutWorkItemTemplate{
						FanOutExpression: "${years[*]}",
						Type:             model.WorkItemTypeWriteDemoOutput,
						IDPrefix:         "transform",
						OutputPrefix:     "transform",
						OutputExtension:  ".txt",
					},
				},
			},
		},
	}
	plan, err := NormalizeStages(workflow)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	result, err := CompileWorkflowStage(resolver, workflow, plan, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Steps) != 2 {
		t.Fatalf("unexpected compiled steps: %d", len(result.Steps))
	}
	if result.Steps[0].StepIndex != 0 || result.Steps[0].StepID != "extract" {
		t.Fatalf("unexpected first compiled step: %+v", result.Steps[0])
	}
	if result.Steps[1].StepIndex != 1 || result.Steps[1].StepID != "transform" {
		t.Fatalf("unexpected second compiled step: %+v", result.Steps[1])
	}
}

func TestCompileWorkflowStageChecksDuplicateWorkItemIDInStage(t *testing.T) {
	workflow := Workflow{
		ID: "cdl",
		Steps: []Step{
			{
				ID:           "extract",
				ParallelWith: "batch",
				FanOut: &FanOutStep{
					WorkItem: FanOutWorkItemTemplate{
						FanOutExpression: "${years[*]}",
						Type:             model.WorkItemTypeWriteDemoOutput,
				IDPrefix:         "same",
						OutputPrefix:     "same",
						OutputExtension:  ".txt",
					},
				},
			},
			{
				ID:           "transform",
				ParallelWith: "batch",
				FanOut: &FanOutStep{
					WorkItem: FanOutWorkItemTemplate{
						FanOutExpression: "${years[*]}",
						Type:             model.WorkItemTypeWriteDemoOutput,
						IDPrefix:         "same",
						OutputPrefix:     "same",
						OutputExtension:  ".txt",
					},
				},
			},
		},
	}
	workflow.Variables = []variable.Variable{
		{
			Name:            variable.Name{Namespace: variable.NamespaceWorkflow, Key: "years"},
			TypedExpression: variable.TypedExpression{
				Type:      variable.TypeList,
				Expression: []variable.TypedExpression{{Type: variable.TypeInt, Expression: 2024}},
			},
		},
	}
	scope, err := variable.NewScope(workflow.Variables...)
	if err != nil {
		t.Fatal(err)
	}
	resolver := variable.NewResolver(variable.NewSet(scope), variable.ResolverConfig{})

	if _, err := CompileWorkflowStage(resolver, workflow, WorkflowPlan{WorkflowID: "cdl", StepCount: 2, Stages: []WorkflowStage{
		{
			Index:        0,
			ParallelWith: "batch",
			Steps: []WorkflowStageStep{
				{
					StageIndex: 0,
					StepIndex:  0,
					StepID:     "extract",
					Step:       workflow.Steps[0],
				},
				{
					StageIndex: 0,
					StepIndex:  1,
					StepID:     "transform",
					Step:       workflow.Steps[1],
				},
			},
		},
	}}, 0); err == nil {
		t.Fatal("expected duplicate work-item error")
	} else if !strings.Contains(err.Error(), "duplicate generated work-item id in stage 0") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCompileWorkflowStageErrorsOnInvalidStageIndex(t *testing.T) {
	resolver := testWorkflowResolver(t, 2024)
	workflow := Workflow{
		ID: "cdl",
		Steps: []Step{
			{
				ID: "download",
				FanOut: &FanOutStep{
					WorkItem: FanOutWorkItemTemplate{
						FanOutExpression: "${years[*]}",
						Type:             model.WorkItemTypeWriteDemoOutput,
						OutputPrefix:      "download",
						OutputExtension:   ".txt",
					},
				},
			},
		},
	}
	plan, err := NormalizeStages(workflow)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := CompileWorkflowStage(resolver, workflow, plan, -1); err == nil {
		t.Fatal("expected negative stage index error")
	}
	if _, err := CompileWorkflowStage(resolver, workflow, plan, 2); err == nil {
		t.Fatal("expected out-of-range stage index error")
	}
}

func TestCompileWorkflowStageStepErrorsIncludeContext(t *testing.T) {
	workflow := Workflow{
		ID: "cdl",
		Steps: []Step{
			{
				ID:           "extract",
				ParallelWith: "batch",
				FanOut: &FanOutStep{
					WorkItem: FanOutWorkItemTemplate{
						FanOutExpression: "${years[*]}",
						Type:             model.WorkItemTypeWriteDemoOutput,
						IDPrefix:         "extract",
						OutputPrefix:     "extract",
						OutputExtension:  ".txt",
					},
				},
			},
			{
				ID: "missing-compiler",
			},
		},
	}
	workflow.Steps[1].ParallelWith = "batch"

	plan, err := NormalizeStages(workflow)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	resolver := testWorkflowResolver(t, 2024)
	if _, err := CompileWorkflowStage(resolver, workflow, plan, 0); err == nil {
		t.Fatal("expected step compile error")
	} else if !strings.Contains(err.Error(), "compile workflow stage 0 step 1 (missing-compiler)") {
		t.Fatalf("unexpected error: %v", err)
	}
}
