package workflow

import (
	"fmt"

	"goetl/internal/model"
	"goetl/internal/variable"
)

type CompileStageWorkItem struct {
	WorkflowID          string
	StageIndex          int
	StepIndex           int
	StepID              string
	WorkItemIndex       int
	WorkItem            model.WorkItem
	ResourceConstraints []model.WorkItemResourceConstraint
}

type CompileStageResult struct {
	WorkflowID string
	StageIndex int
	Steps      []WorkflowStageStep
	WorkItems  []CompileStageWorkItem
}

func CompileWorkflowStage(resolver variable.Resolver, workflow Workflow, plan WorkflowPlan, stageIndex int) (CompileStageResult, error) {
	if workflow.ID == "" {
		return CompileStageResult{}, fmt.Errorf("workflow id is required")
	}

	if plan.WorkflowID != "" && plan.WorkflowID != workflow.ID {
		return CompileStageResult{}, fmt.Errorf("workflow id mismatch: plan=%s workflow=%s", plan.WorkflowID, workflow.ID)
	}

	if stageIndex < 0 || stageIndex >= len(plan.Stages) {
		return CompileStageResult{}, fmt.Errorf("invalid workflow stage index %d", stageIndex)
	}

	stage := plan.Stages[stageIndex]
	seenWorkItems := map[string]bool{}
	result := CompileStageResult{
		WorkflowID: workflow.ID,
		StageIndex: stageIndex,
		Steps:      stage.Steps,
		WorkItems:  nil,
	}

	for _, workflowStep := range stage.Steps {
		compiled, err := CompileStepItems(resolver, workflowStep.Step)
		if err != nil {
			return CompileStageResult{}, fmt.Errorf(
				"compile workflow stage %d step %d (%s): %w",
				stageIndex,
				workflowStep.StepIndex,
				workflowStep.StepID,
				err,
			)
		}

		for itemIndex, item := range compiled {
			if seenWorkItems[item.WorkItem.ID] {
				return CompileStageResult{}, fmt.Errorf(
					"duplicate generated work-item id in stage %d: %s",
					stageIndex,
					item.WorkItem.ID,
				)
			}
			seenWorkItems[item.WorkItem.ID] = true

			result.WorkItems = append(result.WorkItems, CompileStageWorkItem{
				WorkflowID:          workflow.ID,
				StageIndex:          stageIndex,
				StepIndex:           workflowStep.StepIndex,
				StepID:              workflowStep.StepID,
				WorkItemIndex:       itemIndex,
				WorkItem:            item.WorkItem,
				ResourceConstraints: item.ResourceConstraints,
			})
		}
	}

	result, err := PlanStageCacheDataWorkItems(result)
	if err != nil {
		return CompileStageResult{}, err
	}
	return PlanCommitDataWorkItems(result)
}
