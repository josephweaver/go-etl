package workflow

import (
	"reflect"
	"testing"
)

func TestNormalizeStagesRejectsEmptyWorkflow(t *testing.T) {
	_, err := NormalizeStages(Workflow{
		ID:    "cdl",
		Steps: nil,
	})
	if err == nil {
		t.Fatal("expected an error")
	}
}

func TestNormalizeStagesUntaggedStepsCreateIndependentStages(t *testing.T) {
	plan, err := NormalizeStages(Workflow{
		ID: "cdl",
		Steps: []Step{
			{ID: "download"},
			{ID: "summarize"},
			{ID: "notify"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if plan.WorkflowID != "cdl" {
		t.Fatalf("unexpected workflow id: %s", plan.WorkflowID)
	}
	if plan.StepCount != 3 {
		t.Fatalf("unexpected step count: %d", plan.StepCount)
	}
	if len(plan.Stages) != 3 {
		t.Fatalf("unexpected stage count: %d", len(plan.Stages))
	}

	for index, stage := range plan.Stages {
		if stage.Index != index {
			t.Fatalf("unexpected stage index %d: %d", index, stage.Index)
		}
		if len(stage.Steps) != 1 {
			t.Fatalf("unexpected stage step count: %d", len(stage.Steps))
		}
		if stage.ParallelWith != "" {
			t.Fatalf("unexpected parallel_with: %q", stage.ParallelWith)
		}
	}
}

func TestNormalizeStagesGroupsContiguousParallelSteps(t *testing.T) {
	plan, err := NormalizeStages(Workflow{
		ID: "cdl",
		Steps: []Step{
			{ID: "extract", ParallelWith: "batch"},
			{ID: "transform", ParallelWith: "batch"},
			{ID: "notify"},
			{ID: "archive"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(plan.Stages) != 3 {
		t.Fatalf("unexpected stage count: %d", len(plan.Stages))
	}
	if !reflect.DeepEqual(plan.Stages[0].Steps[0].StepID, "extract") ||
		!reflect.DeepEqual(plan.Stages[0].Steps[1].StepID, "transform") {
		t.Fatalf("unexpected stage 0 steps: %#v", plan.Stages[0].Steps)
	}
	if plan.Stages[0].ParallelWith != "batch" {
		t.Fatalf("unexpected stage 0 label: %q", plan.Stages[0].ParallelWith)
	}
}

func TestNormalizeStagesClosesParallelGroupAfterUnrelatedStep(t *testing.T) {
	plan, err := NormalizeStages(Workflow{
		ID: "cdl",
		Steps: []Step{
			{ID: "extract", ParallelWith: "batch"},
			{ID: "transform", ParallelWith: "batch"},
			{ID: "notify"},
			{ID: "archive", ParallelWith: "batch"},
		},
	})
	if err == nil {
		t.Fatalf("expected a closed-label error, got %#v", plan)
	}
}

func TestNormalizeStagesAllowsSecondParallelGroupWithDifferentLabel(t *testing.T) {
	plan, err := NormalizeStages(Workflow{
		ID: "cdl",
		Steps: []Step{
			{ID: "extract", ParallelWith: "batch"},
			{ID: "transform", ParallelWith: "batch"},
			{ID: "notify"},
			{ID: "archive", ParallelWith: "report"},
			{ID: "email", ParallelWith: "report"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(plan.Stages) != 3 {
		t.Fatalf("unexpected stage count: %d", len(plan.Stages))
	}
	if plan.Stages[2].ParallelWith != "report" {
		t.Fatalf("unexpected stage 2 label: %q", plan.Stages[2].ParallelWith)
	}
	if len(plan.Stages[2].Steps) != 2 {
		t.Fatalf("unexpected step count: %d", len(plan.Stages[2].Steps))
	}
}

func TestNormalizeStagesRejectsReusedClosedParallelLabel(t *testing.T) {
	_, err := NormalizeStages(Workflow{
		ID: "cdl",
		Steps: []Step{
			{ID: "extract", ParallelWith: "batch"},
			{ID: "transform", ParallelWith: "batch"},
			{ID: "notify"},
			{ID: "archive", ParallelWith: "batch"},
		},
	})
	if err == nil {
		t.Fatal("expected an error")
	}
}

func TestNormalizeStagesRejectsDuplicateStepID(t *testing.T) {
	_, err := NormalizeStages(Workflow{
		ID: "cdl",
		Steps: []Step{
			{ID: "download"},
			{ID: "download"},
		},
	})
	if err == nil {
		t.Fatal("expected an error")
	}
}

func TestNormalizeStagesTrimsParallelWithWhitespace(t *testing.T) {
	plan, err := NormalizeStages(Workflow{
		ID: "cdl",
		Steps: []Step{
			{ID: "extract", ParallelWith: "   batch "},
			{ID: "transform", ParallelWith: "batch"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(plan.Stages) != 1 {
		t.Fatalf("unexpected stage count: %d", len(plan.Stages))
	}
	if plan.Stages[0].Steps[0].Step.ParallelWith != "batch" {
		t.Fatalf("unexpected normalized parallel_with: %q", plan.Stages[0].Steps[0].Step.ParallelWith)
	}
}
