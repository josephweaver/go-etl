package workflow

import (
	"testing"

	"goetl/internal/model"
	"goetl/internal/variable"
)

func TestCompileWorkflow(t *testing.T) {
	resolver := testWorkflowResolver(t, 2024, 2025)

	items, err := CompileWorkflow(resolver, Workflow{
		ID: "cdl",
		Steps: []Step{
			{
				ID: "download",
				FanOut: &FanOutStep{
					WorkItem: FanOutWorkItemTemplate{
						FanOutExpression: "${years[*]}",
						Type:             model.WorkItemTypeWriteDemoOutput,
						OutputPrefix:     "cdl",
						OutputExtension:  ".txt",
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(items) != 2 {
		t.Fatalf("unexpected item count: %d", len(items))
	}

	if items[1].ID != "download-2025" {
		t.Fatalf("unexpected second id: %s", items[1].ID)
	}
}

func TestCompileWorkflowWithWorkflowVariables(t *testing.T) {
	workflow := Workflow{
		ID: "cdl",
		Variables: []variable.Variable{
			{
				Name: variable.Name{Namespace: variable.NamespaceWorkflow, Key: "years"},
				TypedExpression: variable.TypedExpression{Type: variable.TypeList, Expression: []variable.TypedExpression{
					{Type: variable.TypeInt, Expression: 2024},
					{Type: variable.TypeInt, Expression: 2025},
				}},
			},
		},
		Steps: []Step{
			{
				ID: "download",
				FanOut: &FanOutStep{
					WorkItem: FanOutWorkItemTemplate{
						FanOutExpression: "${years[*]}",
						Type:             model.WorkItemTypeWriteDemoOutput,
						OutputPrefix:     "cdl",
						OutputExtension:  ".txt",
					},
				},
			},
		},
	}

	scope, err := variable.NewScope(workflow.Variables...)
	if err != nil {
		t.Fatal(err)
	}

	resolver := variable.NewResolver(variable.NewSet(scope), variable.ResolverConfig{})

	items, err := CompileWorkflow(resolver, workflow)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if items[1].ID != "download-2025" {
		t.Fatalf("unexpected second id: %s", items[1].ID)
	}
}

func TestCompileWorkflowItemsIncludesTraceMetadata(t *testing.T) {
	resolver := testWorkflowResolver(t, 2024)

	items, err := CompileWorkflowItems(resolver, Workflow{
		ID: "cdl",
		Steps: []Step{
			{
				ID: "download",
				FanOut: &FanOutStep{
					WorkItem: FanOutWorkItemTemplate{
						FanOutExpression: "${years[*]}",
						Type:             model.WorkItemTypeWriteDemoOutput,
						OutputPrefix:     "cdl",
						OutputExtension:  ".txt",
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if items[0].WorkflowID != "cdl" {
		t.Fatalf("unexpected workflow id: %s", items[0].WorkflowID)
	}

	if items[0].StepID != "download" {
		t.Fatalf("unexpected step id: %s", items[0].StepID)
	}

	if items[0].WorkItem.ID != "download-2024" {
		t.Fatalf("unexpected work item id: %s", items[0].WorkItem.ID)
	}
}

func TestCompileWorkflowResultIncludesSummary(t *testing.T) {
	resolver := testWorkflowResolver(t, 2024, 2025)

	result, err := CompileWorkflowResult(resolver, Workflow{
		ID: "cdl",
		Steps: []Step{
			{
				ID: "download",
				FanOut: &FanOutStep{
					WorkItem: FanOutWorkItemTemplate{
						FanOutExpression: "${years[*]}",
						Type:             model.WorkItemTypeWriteDemoOutput,
						OutputPrefix:     "cdl",
						OutputExtension:  ".txt",
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.WorkflowID != "cdl" {
		t.Fatalf("unexpected workflow id: %s", result.WorkflowID)
	}

	if result.StepCount != 1 {
		t.Fatalf("unexpected step count: %d", result.StepCount)
	}

	if len(result.WorkItems) != 2 {
		t.Fatalf("unexpected work item count: %d", len(result.WorkItems))
	}
}

func TestCompileWorkflowRejectsMissingID(t *testing.T) {
	resolver := variable.NewResolver(variable.NewSet(), variable.ResolverConfig{})

	_, err := CompileWorkflow(resolver, Workflow{})
	if err == nil {
		t.Fatal("expected an error")
	}
}

func TestCompileWorkflowRejectsDuplicateStepID(t *testing.T) {
	resolver := testWorkflowResolver(t, 2024)

	_, err := CompileWorkflow(resolver, Workflow{
		ID: "cdl",
		Steps: []Step{
			{
				ID: "download",
				FanOut: &FanOutStep{
					WorkItem: FanOutWorkItemTemplate{
						FanOutExpression: "${years[*]}",
						Type:             model.WorkItemTypeWriteDemoOutput,
						OutputPrefix:     "cdl",
						OutputExtension:  ".txt",
					},
				},
			},
			{
				ID: "download",
				FanOut: &FanOutStep{
					WorkItem: FanOutWorkItemTemplate{
						FanOutExpression: "${years[*]}",
						Type:             model.WorkItemTypeWriteDemoOutput,
						OutputPrefix:     "cdl-copy",
						OutputExtension:  ".txt",
					},
				},
			},
		},
	})
	if err == nil {
		t.Fatal("expected an error")
	}
}

func TestCompileWorkflowRejectsDuplicateGeneratedWorkItemID(t *testing.T) {
	resolver := testWorkflowResolver(t, 2024)

	_, err := CompileWorkflow(resolver, Workflow{
		ID: "cdl",
		Steps: []Step{
			{
				ID: "download",
				FanOut: &FanOutStep{
					WorkItem: FanOutWorkItemTemplate{
						FanOutExpression: "${years[*]}",
						Type:             model.WorkItemTypeWriteDemoOutput,
						IDPrefix:         "cdl",
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
						IDPrefix:         "cdl",
						OutputPrefix:     "summary",
						OutputExtension:  ".txt",
					},
				},
			},
		},
	})
	if err == nil {
		t.Fatal("expected an error")
	}
}

func testWorkflowResolver(t *testing.T, years ...int) variable.Resolver {
	t.Helper()

	items := make([]variable.TypedExpression, 0, len(years))
	for _, year := range years {
		items = append(items, variable.TypedExpression{Type: variable.TypeInt, Expression: year})
	}

	scope, err := variable.NewScope(variable.Variable{Name: variable.Name{Namespace: variable.NamespaceWorkflow, Key: "years"}, TypedExpression: variable.TypedExpression{Type: variable.TypeList, Expression: items}})
	if err != nil {
		t.Fatal(err)
	}

	return variable.NewResolver(variable.NewSet(scope), variable.ResolverConfig{})
}
