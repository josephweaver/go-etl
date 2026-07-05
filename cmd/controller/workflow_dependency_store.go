package main

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"goetl/internal/model"
	"goetl/internal/workflow"
)

func (c *Controller) CreateWorkflowDependencyPlan(ctx context.Context, submissionID string, workflowID string, stages []workflow.WorkflowStage) error {
	if c.workflowStore == nil {
		return fmt.Errorf("workflow store required")
	}
	if submissionID == "" {
		return fmt.Errorf("submission id is required")
	}
	if workflowID == "" {
		return fmt.Errorf("workflow id is required")
	}
	if len(stages) == 0 {
		return fmt.Errorf("normalized stages are required")
	}

	if err := c.requireDependencyRunExists(ctx, submissionID); err != nil {
		return err
	}

	plan, _, err := c.getWorkflowDependencyState(ctx, submissionID)
	if err != nil {
		return err
	}
	if plan != nil {
		return fmt.Errorf("dependency plan already exists for submission %s", submissionID)
	}

	dependencyState := model.WorkflowDependencyPlan{
		RunID:      submissionID,
		WorkflowID: workflowID,
		State:      model.WorkflowStateRunning,
		Stages:     make([]model.WorkflowDependencyStage, 0, len(stages)),
	}

	seenStageIndex := map[int]bool{}
	seenStepIndex := map[int]bool{}
	earliestStageIndex := -1
	for _, stage := range stages {
		if earliestStageIndex == -1 || stage.Index < earliestStageIndex {
			earliestStageIndex = stage.Index
		}
	}

	for _, stage := range stages {
		if stage.Index < 0 {
			return fmt.Errorf("stage index must be non-negative")
		}
		if seenStageIndex[stage.Index] {
			return fmt.Errorf("duplicate stage index %d", stage.Index)
		}
		seenStageIndex[stage.Index] = true

		isReady := stage.Index == earliestStageIndex
		stepState := model.WorkflowStageStateBlocked
		if isReady {
			stepState = model.WorkflowStageStateReady
		}

		dependencyStage := model.WorkflowDependencyStage{
			StageIndex:   stage.Index,
			State:        stepState,
			ParallelWith: stage.ParallelWith,
			Steps:        make([]model.WorkflowDependencyStep, 0, len(stage.Steps)),
		}

		for _, stageStep := range stage.Steps {
			if stageStep.StageIndex != stage.Index {
				return fmt.Errorf("step %s has mismatched stage index %d", stageStep.StepID, stageStep.StageIndex)
			}
			if stageStep.StepIndex < 0 {
				return fmt.Errorf("step index must be non-negative")
			}
			if stageStep.StepID == "" {
				return fmt.Errorf("step id is required")
			}
			if seenStepIndex[stageStep.StepIndex] {
				return fmt.Errorf("duplicate step index %d", stageStep.StepIndex)
			}
			seenStepIndex[stageStep.StepIndex] = true

			stepState := model.WorkflowStepStateBlocked
			if isReady {
				stepState = model.WorkflowStepStateReady
			}

			dependencyStage.Steps = append(dependencyStage.Steps, model.WorkflowDependencyStep{
				StageIndex: stageStep.StageIndex,
				StepIndex:  stageStep.StepIndex,
				StepID:     stageStep.StepID,
				State:      stepState,
				WorkItems:  []model.WorkflowDependencyWorkItemMembership{},
			})
		}

		dependencyState.Stages = append(dependencyState.Stages, dependencyStage)
	}

	if err := validateDependencyPlan(dependencyState); err != nil {
		return err
	}
	if err := c.setWorkflowDependencyState(ctx, submissionID, dependencyState); err != nil {
		return err
	}
	return nil
}

func (c *Controller) ListWorkflowStages(ctx context.Context, submissionID string) ([]model.WorkflowDependencyStage, error) {
	plan, found, err := c.getWorkflowDependencyState(ctx, submissionID)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, fmt.Errorf("dependency plan not found for submission %s", submissionID)
	}

	ordered := make([]model.WorkflowDependencyStage, 0, len(plan.Stages))
	for _, stage := range plan.Stages {
		ordered = append(ordered, cloneDependencyStage(stage))
	}
	sortStagesByIndex(ordered)
	return ordered, nil
}

func (c *Controller) ListWorkflowSteps(ctx context.Context, submissionID string) ([]model.WorkflowDependencyStep, error) {
	plan, found, err := c.getWorkflowDependencyState(ctx, submissionID)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, fmt.Errorf("dependency plan not found for submission %s", submissionID)
	}

	steps := make([]model.WorkflowDependencyStep, 0)
	for _, stage := range plan.Stages {
		for _, step := range stage.Steps {
			steps = append(steps, cloneDependencyStep(step))
		}
	}
	sortStepsByIndex(steps)
	return steps, nil
}

func (c *Controller) RecordCompiledWorkItemMembership(ctx context.Context, submissionID string, stageIndex int, stepIndex int, workItemID string, workItemIndex int) error {
	if c.workflowStore == nil {
		return fmt.Errorf("workflow store required")
	}
	if workItemID == "" {
		return fmt.Errorf("work item id is required")
	}
	if workItemIndex < 0 {
		return fmt.Errorf("work item index must be non-negative")
	}
	if stageIndex < 0 {
		return fmt.Errorf("stage index must be non-negative")
	}
	if stepIndex < 0 {
		return fmt.Errorf("step index must be non-negative")
	}

	plan, found, err := c.getWorkflowDependencyState(ctx, submissionID)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("dependency plan not found for submission %s", submissionID)
	}

	step, found := findDependencyStep(plan, stageIndex, stepIndex)
	if !found {
		return fmt.Errorf("step not found for submission %s at stage %d step %d", submissionID, stageIndex, stepIndex)
	}
	if err := ensureNoDuplicateWorkItemMembership(step, workItemID, workItemIndex); err != nil {
		return err
	}

	step.WorkItems = append(step.WorkItems, model.WorkflowDependencyWorkItemMembership{
		WorkItemID:    workItemID,
		WorkItemIndex: workItemIndex,
		State:         model.WorkItemMembershipStateQueued,
	})
	sortWorkItemsByIndex(step.WorkItems)

	if err := c.setWorkflowDependencyState(ctx, submissionID, *plan); err != nil {
		return err
	}
	return nil
}

func (c *Controller) ReadStepState(ctx context.Context, submissionID string, stepIndex int) (model.WorkflowDependencyStep, bool, error) {
	plan, found, err := c.getWorkflowDependencyState(ctx, submissionID)
	if err != nil {
		return model.WorkflowDependencyStep{}, false, err
	}
	if !found {
		return model.WorkflowDependencyStep{}, false, nil
	}
	for _, stage := range plan.Stages {
		for _, step := range stage.Steps {
			if step.StepIndex == stepIndex {
				return cloneDependencyStep(step), true, nil
			}
		}
	}
	return model.WorkflowDependencyStep{}, false, nil
}

func (c *Controller) ReadStageState(ctx context.Context, submissionID string, stageIndex int) (model.WorkflowDependencyStage, bool, error) {
	plan, found, err := c.getWorkflowDependencyState(ctx, submissionID)
	if err != nil {
		return model.WorkflowDependencyStage{}, false, err
	}
	if !found {
		return model.WorkflowDependencyStage{}, false, nil
	}
	for _, stage := range plan.Stages {
		if stage.StageIndex == stageIndex {
			return cloneDependencyStage(stage), true, nil
		}
	}
	return model.WorkflowDependencyStage{}, false, nil
}

func (c *Controller) getWorkflowDependencyState(ctx context.Context, submissionID string) (*model.WorkflowDependencyPlan, bool, error) {
	if c.workflowStore == nil {
		return nil, false, fmt.Errorf("workflow store required")
	}
	run, found, err := c.workflowStore.GetWorkflowRun(ctx, submissionID)
	if err != nil {
		return nil, false, fmt.Errorf("get workflow run %s: %w", submissionID, err)
	}
	if !found {
		return nil, false, nil
	}

	var context workflowRunSubmissionContext
	if err := json.Unmarshal([]byte(run.SubmissionContextJSON), &context); err != nil {
		return nil, false, fmt.Errorf("decode workflow submission context: %w", err)
	}
	if context.DependencyState == nil {
		return nil, false, nil
	}
	if err := validateDependencyPlan(*context.DependencyState); err != nil {
		return nil, false, fmt.Errorf("invalid dependency state for submission %s: %w", submissionID, err)
	}
	return context.DependencyState, true, nil
}

func (c *Controller) setWorkflowDependencyState(ctx context.Context, submissionID string, plan model.WorkflowDependencyPlan) error {
	if err := validateDependencyPlan(plan); err != nil {
		return err
	}

	run, found, err := c.workflowStore.GetWorkflowRun(ctx, submissionID)
	if err != nil {
		return fmt.Errorf("get workflow run %s: %w", submissionID, err)
	}
	if !found {
		return fmt.Errorf("workflow run %s not found", submissionID)
	}

	var submissionContext workflowRunSubmissionContext
	if err := json.Unmarshal([]byte(run.SubmissionContextJSON), &submissionContext); err != nil {
		return fmt.Errorf("decode workflow submission context: %w", err)
	}
	submissionContext.DependencyState = &plan

	submissionContextJSON, err := json.Marshal(submissionContext)
	if err != nil {
		return fmt.Errorf("encode workflow dependency state: %w", err)
	}
	if err := c.workflowStore.UpdateWorkflowRunSubmissionContext(ctx, submissionID, string(submissionContextJSON)); err != nil {
		return fmt.Errorf("persist workflow dependency state: %w", err)
	}
	return nil
}

func (c *Controller) requireDependencyRunExists(ctx context.Context, submissionID string) error {
	if c.workflowStore == nil {
		return fmt.Errorf("workflow store required")
	}
	_, found, err := c.workflowStore.GetWorkflowRun(ctx, submissionID)
	if err != nil {
		return fmt.Errorf("get workflow run %s: %w", submissionID, err)
	}
	if !found {
		return fmt.Errorf("workflow run %s not found", submissionID)
	}
	return nil
}

func validateDependencyPlan(plan model.WorkflowDependencyPlan) error {
	if err := plan.Validate(); err != nil {
		return err
	}
	seenStage := map[int]bool{}
	seenStep := map[int]bool{}
	seenWorkItem := map[string]bool{}
	for _, stage := range plan.Stages {
		if seenStage[stage.StageIndex] {
			return fmt.Errorf("duplicate stage index %d", stage.StageIndex)
		}
		seenStage[stage.StageIndex] = true
		if err := stage.Validate(); err != nil {
			return fmt.Errorf("stage %d: %w", stage.StageIndex, err)
		}
		for _, step := range stage.Steps {
			if seenStep[step.StepIndex] {
				return fmt.Errorf("duplicate step index %d", step.StepIndex)
			}
			seenStep[step.StepIndex] = true
			if step.StageIndex != stage.StageIndex {
				return fmt.Errorf("step %d has unexpected stage %d for stage %d", step.StepIndex, step.StageIndex, stage.StageIndex)
			}
			for _, workItem := range step.WorkItems {
				if seenWorkItem[workItem.WorkItemID] {
					return fmt.Errorf("duplicate work item id %s", workItem.WorkItemID)
				}
				if err := workItem.Validate(); err != nil {
					return fmt.Errorf("work item %s: %w", workItem.WorkItemID, err)
				}
				seenWorkItem[workItem.WorkItemID] = true
			}
		}
	}
	return nil
}

func findDependencyStep(plan *model.WorkflowDependencyPlan, stageIndex, stepIndex int) (*model.WorkflowDependencyStep, bool) {
	for stageIdx := range plan.Stages {
		if plan.Stages[stageIdx].StageIndex != stageIndex {
			continue
		}
		for stepIdx := range plan.Stages[stageIdx].Steps {
			if plan.Stages[stageIdx].Steps[stepIdx].StepIndex != stepIndex {
				continue
			}
			return &plan.Stages[stageIdx].Steps[stepIdx], true
		}
	}
	return nil, false
}

func ensureNoDuplicateWorkItemMembership(step *model.WorkflowDependencyStep, workItemID string, workItemIndex int) error {
	for _, existing := range step.WorkItems {
		if existing.WorkItemID == workItemID {
			return fmt.Errorf("work item %s already recorded", workItemID)
		}
		if existing.WorkItemIndex == workItemIndex {
			return fmt.Errorf("work item index %d already recorded", workItemIndex)
		}
	}
	return nil
}

func sortStagesByIndex(stages []model.WorkflowDependencyStage) {
	sort.Slice(stages, func(i, j int) bool {
		return stages[i].StageIndex < stages[j].StageIndex
	})
	for index := range stages {
		sortStepsByIndex(stages[index].Steps)
	}
}

func sortStepsByIndex(steps []model.WorkflowDependencyStep) {
	sort.Slice(steps, func(i, j int) bool {
		if steps[i].StageIndex == steps[j].StageIndex {
			return steps[i].StepIndex < steps[j].StepIndex
		}
		return steps[i].StageIndex < steps[j].StageIndex
	})
	for index := range steps {
		sortWorkItemsByIndex(steps[index].WorkItems)
	}
}

func sortWorkItemsByIndex(workItems []model.WorkflowDependencyWorkItemMembership) {
	sort.Slice(workItems, func(i, j int) bool {
		if workItems[i].WorkItemIndex == workItems[j].WorkItemIndex {
			return workItems[i].WorkItemID < workItems[j].WorkItemID
		}
		return workItems[i].WorkItemIndex < workItems[j].WorkItemIndex
	})
}

func cloneDependencyStage(stage model.WorkflowDependencyStage) model.WorkflowDependencyStage {
	clone := model.WorkflowDependencyStage{
		StageIndex:   stage.StageIndex,
		State:        stage.State,
		ParallelWith: stage.ParallelWith,
		Steps:        make([]model.WorkflowDependencyStep, 0, len(stage.Steps)),
	}
	for _, step := range stage.Steps {
		clone.Steps = append(clone.Steps, cloneDependencyStep(step))
	}
	return clone
}

func cloneDependencyStep(step model.WorkflowDependencyStep) model.WorkflowDependencyStep {
	clone := model.WorkflowDependencyStep{
		StageIndex: step.StageIndex,
		StepIndex:  step.StepIndex,
		StepID:     step.StepID,
		State:      step.State,
		WorkItems:  make([]model.WorkflowDependencyWorkItemMembership, 0, len(step.WorkItems)),
	}
	for _, item := range step.WorkItems {
		clone.WorkItems = append(clone.WorkItems, model.WorkflowDependencyWorkItemMembership{
			WorkItemID:    item.WorkItemID,
			WorkItemIndex: item.WorkItemIndex,
			State:         item.State,
		})
	}
	return clone
}
