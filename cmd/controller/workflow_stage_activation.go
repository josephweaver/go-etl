package main

import (
	"context"
	"fmt"
	"time"

	"goetl/internal/model"
	"goetl/internal/persistence"
	"goetl/internal/reposource"
	"goetl/internal/variable"
	"goetl/internal/workflow"
)

func (c *Controller) activateNextReadyWorkflowStage(ctx context.Context, runID string, completedStageIndex int, activatedAt time.Time) error {
	if c.workflowStore == nil {
		return nil
	}

	plan, found, err := c.getWorkflowDependencyState(ctx, runID)
	if err != nil {
		return err
	}
	if !found || plan.State != model.WorkflowStateRunning {
		return nil
	}

	completedStage, found := findDependencyStage(plan, completedStageIndex)
	if !found || completedStage.State != model.WorkflowStageStateCompleted {
		return nil
	}

	nextStageIndex := completedStageIndex
	for {
		nextStageIndex++
		nextStage, found := findDependencyStage(plan, nextStageIndex)
		if !found || nextStage.State != model.WorkflowStageStateBlocked {
			return nil
		}

		stageResult, _, codeVersion, err := c.compileActivationStage(ctx, runID, *plan, nextStageIndex)
		if err != nil {
			return c.failWorkflowStageActivation(ctx, runID, nextStageIndex, err)
		}
		items, queued, memberships, resourceConstraints, err := persistenceRecordsFromCompiledStageResults(runID, []workflow.CompileStageResult{stageResult}, codeVersion, activatedAt)
		if err != nil {
			return c.failWorkflowStageActivation(ctx, runID, nextStageIndex, err)
		}
		nextStageCompleted, err := c.markCompiledStageEmptyStepsCompleted(ctx, runID, stageResult)
		if err != nil {
			return c.failWorkflowStageActivation(ctx, runID, nextStageIndex, err)
		}

		if nextStageCompleted {
			nextPlan, found, err := c.getWorkflowDependencyState(ctx, runID)
			if err != nil {
				return err
			}
			if !found {
				return nil
			}
			plan = nextPlan
			continue
		}

		if len(items) == 0 {
			return nil
		}
		if len(items) != 0 {
			if err := c.workflowStore.QueueWorkItems(ctx, persistence.QueueWorkItemsRequest{
				WorkItems:           items,
				ResourceConstraints: resourceConstraints,
				QueuedWork:          queued,
			}); err != nil {
				return c.failWorkflowStageActivation(ctx, runID, nextStageIndex, fmt.Errorf("queue activated stage work items: %w", err))
			}
		}
		for _, membership := range memberships {
			if err := c.RecordCompiledWorkItemMembership(ctx, runID, membership.stageIndex, membership.stepIndex, membership.workItemID, membership.workItemIndex); err != nil {
				return c.failWorkflowStageActivation(ctx, runID, nextStageIndex, fmt.Errorf("record activated stage membership: %w", err))
			}
		}
		if err := c.MarkWorkflowStageReady(ctx, runID, nextStageIndex); err != nil {
			return c.failWorkflowStageActivation(ctx, runID, nextStageIndex, err)
		}
		c.signalCareTaker("workflow_stage_activated")
		return nil
	}
}

func (c *Controller) failWorkflowStageActivation(ctx context.Context, runID string, stageIndex int, cause error) error {
	if cause == nil {
		cause = fmt.Errorf("workflow stage activation failed")
	}
	if failErr := c.markWorkflowStageActivationFailed(ctx, runID, stageIndex, cause.Error()); failErr != nil {
		return fmt.Errorf("%w; additionally failed to mark workflow failed: %v", cause, failErr)
	}
	return nil
}

func (c *Controller) compileActivationStage(ctx context.Context, runID string, plan model.WorkflowDependencyPlan, stageIndex int) (workflow.CompileStageResult, variable.Resolver, string, error) {
	run, found, err := c.workflowStore.GetWorkflowRun(ctx, runID)
	if err != nil {
		return workflow.CompileStageResult{}, variable.Resolver{}, "", fmt.Errorf("get workflow run %s: %w", runID, err)
	}
	if !found {
		return workflow.CompileStageResult{}, variable.Resolver{}, "", fmt.Errorf("workflow run %s not found", runID)
	}

	context, ok, err := workflowRunSourceContext(run)
	if err != nil {
		return workflow.CompileStageResult{}, variable.Resolver{}, "", err
	}
	if !ok {
		return workflow.CompileStageResult{}, variable.Resolver{}, "", fmt.Errorf("workflow run %s missing source-admission context", runID)
	}
	manifest, err := readAdmittedSourceManifest(context.SourceAdmission.ManifestRef)
	if err != nil {
		return workflow.CompileStageResult{}, variable.Resolver{}, "", err
	}
	access, err := reposource.NewCacheAccess(c.repoCacheLayout, manifest)
	if err != nil {
		return workflow.CompileStageResult{}, variable.Resolver{}, "", err
	}
	workflowFile, err := manifestFileByRole(manifest, reposource.FileRoleWorkflow)
	if err != nil {
		return workflow.CompileStageResult{}, variable.Resolver{}, "", err
	}
	workflowData, err := access.ReadFile(workflowFile.CachePath)
	if err != nil {
		return workflow.CompileStageResult{}, variable.Resolver{}, "", err
	}
	workflowSubmission, err := decodeWorkflowSourceSubmission(workflowData)
	if err != nil {
		return workflow.CompileStageResult{}, variable.Resolver{}, "", fmt.Errorf("decode cached workflow source: %w", err)
	}

	nextStage, found := findDependencyStage(&plan, stageIndex)
	if !found {
		return workflow.CompileStageResult{}, variable.Resolver{}, "", fmt.Errorf("dependency stage %d not found", stageIndex)
	}
	beforeStepIndex := firstDependencyStepIndex(*nextStage)
	generatedWorkflowScope, err := workflowStepScope(plan, beforeStepIndex)
	if err != nil {
		return workflow.CompileStageResult{}, variable.Resolver{}, "", err
	}
	workflowScope, err := variable.NewScope(workflowSubmission.Workflow.Variables...)
	if err != nil {
		return workflow.CompileStageResult{}, variable.Resolver{}, "", err
	}
	sourceSubmissionScope, err := variable.NewScope(workflowSubmission.Variables...)
	if err != nil {
		return workflow.CompileStageResult{}, variable.Resolver{}, "", err
	}
	runSubmissionScope, err := variable.NewScope(context.Variables...)
	if err != nil {
		return workflow.CompileStageResult{}, variable.Resolver{}, "", err
	}
	resolver := variable.NewResolver(variable.NewSet(workflowScope, sourceSubmissionScope, runSubmissionScope, generatedWorkflowScope), variable.ResolverConfig{})
	codeVersion, err := controllerCodeVersion(resolver)
	if err != nil {
		return workflow.CompileStageResult{}, variable.Resolver{}, "", err
	}
	workflowPlan, err := workflow.NormalizeStages(workflowSubmission.Workflow)
	if err != nil {
		return workflow.CompileStageResult{}, variable.Resolver{}, "", err
	}
	result, err := workflow.CompileWorkflowStage(resolver, workflowSubmission.Workflow, workflowPlan, stageIndex)
	if err != nil {
		return workflow.CompileStageResult{}, variable.Resolver{}, "", err
	}
	compileResult := workflow.CompileResult{
		WorkflowID: result.WorkflowID,
		StepCount:  len(workflowSubmission.Workflow.Steps),
		WorkItems:  make([]workflow.CompiledWorkItem, 0, len(result.WorkItems)),
	}
	for _, item := range result.WorkItems {
		compileResult.WorkItems = append(compileResult.WorkItems, workflow.CompiledWorkItem{
			WorkflowID:          result.WorkflowID,
			StepID:              item.StepID,
			WorkItem:            item.WorkItem,
			ResourceConstraints: item.ResourceConstraints,
		})
	}
	compileResult, err = prepareCompiledWorkflowForAdmission(c.repoCacheLayout, manifest, compileResult)
	if err != nil {
		return workflow.CompileStageResult{}, variable.Resolver{}, "", err
	}
	if len(result.WorkItems) != len(compileResult.WorkItems) {
		return workflow.CompileStageResult{}, variable.Resolver{}, "", fmt.Errorf("compile result mismatch: expected %d stage items, got %d", len(result.WorkItems), len(compileResult.WorkItems))
	}
	for index := range result.WorkItems {
		result.WorkItems[index].WorkItem = compileResult.WorkItems[index].WorkItem
		result.WorkItems[index].ResourceConstraints = compileResult.WorkItems[index].ResourceConstraints
	}
	return result, resolver, codeVersion, nil
}

func activationTimeFromCompletedWork(completed persistence.CompletedWorkRecord) time.Time {
	parsed, err := time.Parse(time.RFC3339, completed.CompletedAt)
	if err != nil {
		return time.Now().UTC()
	}
	return parsed.UTC()
}
