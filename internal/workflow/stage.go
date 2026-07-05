package workflow

import (
	"fmt"
	"strings"
)

type WorkflowPlan struct {
	WorkflowID string
	StepCount  int
	Stages     []WorkflowStage
}

type WorkflowStage struct {
	Index        int
	ParallelWith string
	Steps        []WorkflowStageStep
}

type WorkflowStageStep struct {
	StageIndex int
	StepIndex  int
	StepID     string
	Step       Step
}

func NormalizeStages(workflow Workflow) (WorkflowPlan, error) {
	if len(workflow.Steps) == 0 {
		return WorkflowPlan{}, fmt.Errorf("workflow has no steps")
	}

	seenStepID := map[string]bool{}
	closedParallelWith := map[string]bool{}
	plan := WorkflowPlan{
		WorkflowID: workflow.ID,
		StepCount:  len(workflow.Steps),
	}
	currentStage := -1
	currentLabel := ""

	for index, step := range workflow.Steps {
		normalizedLabel := strings.TrimSpace(step.ParallelWith)

		if seenStepID[step.ID] {
			return WorkflowPlan{}, fmt.Errorf("duplicate workflow step id: %s", step.ID)
		}
		seenStepID[step.ID] = true

		if normalizedLabel == "" {
			if currentLabel != "" {
				closedParallelWith[currentLabel] = true
			}

			currentLabel = ""
			currentStage = len(plan.Stages)
			step.ParallelWith = ""
			plan.Stages = append(plan.Stages, WorkflowStage{
				Index:        currentStage,
				ParallelWith: "",
				Steps: []WorkflowStageStep{
					{
						StageIndex: currentStage,
						StepIndex:  index,
						StepID:     step.ID,
						Step:       step,
					},
				},
			})
			continue
		}

		if closedParallelWith[normalizedLabel] {
			return WorkflowPlan{}, fmt.Errorf("workflow parallel_with label reused after close: %s", normalizedLabel)
		}

		if normalizedLabel == currentLabel && currentStage >= 0 {
			step.ParallelWith = normalizedLabel
			plan.Stages[currentStage].ParallelWith = normalizedLabel
			plan.Stages[currentStage].Steps = append(plan.Stages[currentStage].Steps, WorkflowStageStep{
				StageIndex: currentStage,
				StepIndex:  index,
				StepID:     step.ID,
				Step:       step,
			})
			continue
		}

		if currentLabel != "" && currentLabel != normalizedLabel {
			closedParallelWith[currentLabel] = true
		}

		step.ParallelWith = normalizedLabel
		currentLabel = normalizedLabel
		currentStage = len(plan.Stages)
		plan.Stages = append(plan.Stages, WorkflowStage{
			Index:        currentStage,
			ParallelWith: normalizedLabel,
			Steps: []WorkflowStageStep{
				{
					StageIndex: currentStage,
					StepIndex:  index,
					StepID:     step.ID,
					Step:       step,
				},
			},
		})
	}

	return plan, nil
}
