package workflow

import (
	"fmt"

	"goetl/internal/model"
	"goetl/internal/variable"
)

type Workflow struct {
	ID        string
	Variables []variable.Variable
	Steps     []Step
}

type CompiledWorkItem struct {
	WorkflowID          string
	StepID              string
	WorkItem            model.WorkItem
	ResourceConstraints []model.WorkItemResourceConstraint
}

type CompileResult struct {
	WorkflowID string
	StepCount  int
	WorkItems  []CompiledWorkItem
}

func CompileWorkflow(resolver variable.Resolver, workflow Workflow) ([]model.WorkItem, error) {
	compiled, err := CompileWorkflowItems(resolver, workflow)
	if err != nil {
		return nil, err
	}

	items := make([]model.WorkItem, 0, len(compiled))
	for _, item := range compiled {
		items = append(items, item.WorkItem)
	}

	return items, nil
}

func CompileWorkflowItems(resolver variable.Resolver, workflow Workflow) ([]CompiledWorkItem, error) {
	result, err := CompileWorkflowResult(resolver, workflow)
	if err != nil {
		return nil, err
	}

	return result.WorkItems, nil
}

func CompileWorkflowResult(resolver variable.Resolver, workflow Workflow) (CompileResult, error) {
	if workflow.ID == "" {
		return CompileResult{}, fmt.Errorf("workflow id is required")
	}

	seenSteps := map[string]bool{}
	seenWorkItems := map[string]bool{}
	items := []CompiledWorkItem{}
	for index, step := range workflow.Steps {
		if seenSteps[step.ID] {
			return CompileResult{}, fmt.Errorf("duplicate workflow step id: %s", step.ID)
		}
		seenSteps[step.ID] = true

		compiled, err := CompileStepItems(resolver, step)
		if err != nil {
			return CompileResult{}, fmt.Errorf("compile workflow step %d: %w", index, err)
		}

		for _, item := range compiled {
			if seenWorkItems[item.WorkItem.ID] {
				return CompileResult{}, fmt.Errorf("duplicate generated work item id: %s", item.WorkItem.ID)
			}
			seenWorkItems[item.WorkItem.ID] = true

			items = append(items, CompiledWorkItem{
				WorkflowID:          workflow.ID,
				StepID:              step.ID,
				WorkItem:            item.WorkItem,
				ResourceConstraints: item.ResourceConstraints,
			})
		}
	}

	return CompileResult{
		WorkflowID: workflow.ID,
		StepCount:  len(workflow.Steps),
		WorkItems:  items,
	}, nil
}
