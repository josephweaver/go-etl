package main

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	fp "goetl/internal/fingerprint"
	"goetl/internal/model"
	"goetl/internal/variable"
	"goetl/internal/workflow"
)

type RecordCompletedWorkItemOutputRequest struct {
	SubmissionID     string
	WorkItemID       string
	OutputJSON       string
	OutputJSONSHA256 string
}

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

func (c *Controller) RecordWorkItemTerminalState(ctx context.Context, submissionID string, workItemID string, terminalState model.WorkItemMembershipState) error {
	if c.workflowStore == nil {
		return fmt.Errorf("workflow store required")
	}
	if submissionID == "" {
		return fmt.Errorf("submission id is required")
	}
	if workItemID == "" {
		return fmt.Errorf("work item id is required")
	}
	if err := terminalState.Validate(); err != nil {
		return err
	}

	plan, found, err := c.getWorkflowDependencyState(ctx, submissionID)
	if err != nil {
		return err
	}
	if !found {
		return nil
	}

	updated, err := updateDependencyPlanForWorkItemTerminal(plan, workItemID, terminalState)
	if err != nil {
		return err
	}
	if !updated {
		return nil
	}

	if err := c.setWorkflowDependencyState(ctx, submissionID, *plan); err != nil {
		return err
	}
	return nil
}

func (c *Controller) RecordCompletedWorkItemOutput(ctx context.Context, req RecordCompletedWorkItemOutputRequest) error {
	if c.workflowStore == nil {
		return fmt.Errorf("workflow store required")
	}
	if req.SubmissionID == "" {
		return fmt.Errorf("submission id is required")
	}
	if req.WorkItemID == "" {
		return fmt.Errorf("work item id is required")
	}
	if strings.TrimSpace(req.OutputJSON) == "" {
		return fmt.Errorf("output json is required")
	}
	if err := validateCompletedWorkOutputJSONSize(req.OutputJSON); err != nil {
		return err
	}

	resolved, err := resolvedOutputFromJSON(req.OutputJSON)
	if err != nil {
		return err
	}
	outputJSON, outputJSONSHA256, err := canonicalOutputJSONFromResolved(resolved)
	if err != nil {
		return err
	}
	if err := validateCompletedWorkOutputJSONSize(outputJSON); err != nil {
		return err
	}
	if req.OutputJSONSHA256 != "" {
		if err := fp.ValidateSHA256Hex(req.OutputJSONSHA256); err != nil {
			return fmt.Errorf("output_json_sha256: %w", err)
		}
		if req.OutputJSONSHA256 != outputJSONSHA256 {
			return fmt.Errorf("output_json_sha256 does not match canonical output JSON")
		}
	}

	plan, found, err := c.getWorkflowDependencyState(ctx, req.SubmissionID)
	if err != nil {
		return err
	}
	if !found {
		return nil
	}

	step, stage, membershipIndex, found := findDependencyWorkItem(plan, req.WorkItemID)
	if !found {
		return fmt.Errorf("work item %s not found in dependency plan for submission %s", req.WorkItemID, req.SubmissionID)
	}
	membership := &step.WorkItems[membershipIndex]
	membership.State = model.WorkItemMembershipStateCompleted
	membership.OutputJSON = outputJSON
	membership.OutputJSONSHA256 = outputJSONSHA256
	membership.OutputJSONBytes = len([]byte(outputJSON))
	membership.OutputJSONPruned = false

	step.State = updateStepStateFromWorkItems(step.WorkItems, step.State)
	if dependencyStepOutputsReady(*step) {
		stepOutputJSON, stepOutputJSONSHA256, err := aggregateStepOutputJSON(*step)
		if err != nil {
			return err
		}
		if err := validateLogicalStepOutputJSONSize(stepOutputJSON); err != nil {
			return err
		}
		step.OutputJSON = stepOutputJSON
		step.OutputJSONSHA256 = stepOutputJSONSHA256
		step.OutputJSONBytes = len([]byte(stepOutputJSON))
		step.OutputJSONPruned = false
		step.State = model.WorkflowStepStateCompleted
		pruneWorkItemOutputJSON(step)
	}
	stage.State = updateStageStateFromSteps(stage.Steps, stage.State)
	updateWorkflowStateFromStages(plan)
	pruneDependencyOutputsIfTerminal(plan)

	if err := c.setWorkflowDependencyState(ctx, req.SubmissionID, *plan); err != nil {
		return err
	}
	return nil
}

func (c *Controller) markCompiledStageEmptyStepsCompleted(ctx context.Context, submissionID string, stageResult workflow.CompileStageResult) (bool, error) {
	emptyStepOutputJSON, emptyStepOutputJSONSHA256, err := canonicalOutputFromEmptyList()
	if err != nil {
		return false, err
	}
	if err := validateLogicalStepOutputJSONSize(emptyStepOutputJSON); err != nil {
		return false, err
	}

	plan, found, err := c.getWorkflowDependencyState(ctx, submissionID)
	if err != nil {
		return false, err
	}
	if !found {
		return false, nil
	}

	stepsByIndex := make(map[int]bool, len(stageResult.WorkItems))
	for _, item := range stageResult.WorkItems {
		stepsByIndex[item.StepIndex] = true
	}

	stage, found := findDependencyStage(plan, stageResult.StageIndex)
	if !found {
		return false, nil
	}

	updated := false
	for index := range stage.Steps {
		step := &stage.Steps[index]
		if stepsByIndex[step.StepIndex] {
			continue
		}
		if len(step.WorkItems) > 0 {
			continue
		}
		if step.State == model.WorkflowStepStateCompleted || step.State == model.WorkflowStepStateFailed {
			continue
		}
		step.State = model.WorkflowStepStateCompleted
		step.OutputJSON = emptyStepOutputJSON
		step.OutputJSONSHA256 = emptyStepOutputJSONSHA256
		step.OutputJSONBytes = len([]byte(emptyStepOutputJSON))
		step.OutputJSONPruned = false
		updated = true
	}
	if !updated {
		return false, nil
	}

	allCompleted := true
	for _, step := range stage.Steps {
		if step.State != model.WorkflowStepStateCompleted {
			allCompleted = false
			break
		}
	}
	if allCompleted {
		stage.State = model.WorkflowStageStateCompleted
		updateWorkflowStateFromStages(plan)
	}

	if err := c.setWorkflowDependencyState(ctx, submissionID, *plan); err != nil {
		return false, err
	}
	return allCompleted, nil
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

func (c *Controller) MarkWorkflowStageReady(ctx context.Context, submissionID string, stageIndex int) error {
	plan, found, err := c.getWorkflowDependencyState(ctx, submissionID)
	if err != nil {
		return err
	}
	if !found || plan.State != model.WorkflowStateRunning {
		return nil
	}
	stage, found := findDependencyStage(plan, stageIndex)
	if !found {
		return fmt.Errorf("stage %d not found for submission %s", stageIndex, submissionID)
	}
	if stage.State != model.WorkflowStageStateBlocked {
		return nil
	}
	stage.State = model.WorkflowStageStateReady
	for index := range stage.Steps {
		if stage.Steps[index].State == model.WorkflowStepStateBlocked {
			stage.Steps[index].State = model.WorkflowStepStateReady
		}
	}
	return c.setWorkflowDependencyState(ctx, submissionID, *plan)
}

func (c *Controller) markWorkflowStageActivationFailed(ctx context.Context, submissionID string, stageIndex int) error {
	plan, found, err := c.getWorkflowDependencyState(ctx, submissionID)
	if err != nil {
		return err
	}
	if !found {
		return nil
	}
	stage, found := findDependencyStage(plan, stageIndex)
	if found {
		stage.State = model.WorkflowStageStateFailed
		for index := range stage.Steps {
			stage.Steps[index].State = model.WorkflowStepStateFailed
		}
	}
	plan.State = model.WorkflowStateFailed
	pruneDependencyOutputsIfTerminal(plan)
	return c.setWorkflowDependencyState(ctx, submissionID, *plan)
}

func updateDependencyPlanForWorkItemTerminal(plan *model.WorkflowDependencyPlan, workItemID string, terminalState model.WorkItemMembershipState) (bool, error) {
	if plan == nil {
		return false, nil
	}

	step, stage, membershipIndex, found := findDependencyWorkItem(plan, workItemID)
	if !found {
		return false, nil
	}
	if membershipIndex < 0 || membershipIndex >= len(step.WorkItems) {
		return false, fmt.Errorf("invalid membership index for work item %s", workItemID)
	}

	if step.WorkItems[membershipIndex].State == terminalState {
		return false, nil
	}
	step.WorkItems[membershipIndex].State = terminalState

	step.State = updateStepStateFromWorkItems(step.WorkItems, step.State)
	stage.State = updateStageStateFromSteps(stage.Steps, stage.State)
	updateWorkflowStateFromStages(plan)
	pruneDependencyOutputsIfTerminal(plan)
	return true, nil
}

func findDependencyWorkItem(plan *model.WorkflowDependencyPlan, workItemID string) (*model.WorkflowDependencyStep, *model.WorkflowDependencyStage, int, bool) {
	for stageIndex := range plan.Stages {
		for stepIndex := range plan.Stages[stageIndex].Steps {
			for membershipIndex, membership := range plan.Stages[stageIndex].Steps[stepIndex].WorkItems {
				if membership.WorkItemID != workItemID {
					continue
				}
				return &plan.Stages[stageIndex].Steps[stepIndex], &plan.Stages[stageIndex], membershipIndex, true
			}
		}
	}
	return nil, nil, 0, false
}

func findDependencyMembershipByWorkItemID(plan *model.WorkflowDependencyPlan, workItemID string) (*model.WorkflowDependencyStep, *model.WorkflowDependencyWorkItemMembership, bool) {
	if plan == nil {
		return nil, nil, false
	}
	for stageIndex := range plan.Stages {
		for stepIndex := range plan.Stages[stageIndex].Steps {
			for membershipIndex := range plan.Stages[stageIndex].Steps[stepIndex].WorkItems {
				membership := &plan.Stages[stageIndex].Steps[stepIndex].WorkItems[membershipIndex]
				if membership.WorkItemID != workItemID {
					continue
				}
				return &plan.Stages[stageIndex].Steps[stepIndex], membership, true
			}
		}
	}
	return nil, nil, false
}

func findDependencyStage(plan *model.WorkflowDependencyPlan, stageIndex int) (*model.WorkflowDependencyStage, bool) {
	if plan == nil {
		return nil, false
	}
	for index := range plan.Stages {
		if plan.Stages[index].StageIndex == stageIndex {
			return &plan.Stages[index], true
		}
	}
	return nil, false
}

func dependencyStageHasWorkItems(stage model.WorkflowDependencyStage) bool {
	for _, step := range stage.Steps {
		if len(step.WorkItems) != 0 {
			return true
		}
	}
	return false
}

func firstDependencyStepIndex(stage model.WorkflowDependencyStage) int {
	first := -1
	for _, step := range stage.Steps {
		if first == -1 || step.StepIndex < first {
			first = step.StepIndex
		}
	}
	return first
}

func dependencyStepOutputsReady(step model.WorkflowDependencyStep) bool {
	if len(step.WorkItems) == 0 {
		return false
	}
	for _, item := range step.WorkItems {
		switch item.State {
		case model.WorkItemMembershipStateCompleted, model.WorkItemMembershipStateSkipped:
			if item.OutputJSON == "" {
				return false
			}
		default:
			return false
		}
	}
	return true
}

func dependencyStepHasIncompleteWork(step model.WorkflowDependencyStep) bool {
	for _, item := range step.WorkItems {
		switch item.State {
		case model.WorkItemMembershipStateCompleted, model.WorkItemMembershipStateSkipped, model.WorkItemMembershipStateFailed:
			continue
		default:
			return true
		}
	}
	return false
}

func updateStepStateFromWorkItems(workItems []model.WorkflowDependencyWorkItemMembership, currentState model.WorkflowStepState) model.WorkflowStepState {
	completed := 0
	hasTerminal := false
	for _, workItem := range workItems {
		switch workItem.State {
		case model.WorkItemMembershipStateFailed:
			return model.WorkflowStepStateFailed
		case model.WorkItemMembershipStateCompleted, model.WorkItemMembershipStateSkipped:
			completed++
			hasTerminal = true
		}
	}

	if len(workItems) == completed {
		return model.WorkflowStepStateCompleted
	}
	if hasTerminal {
		return model.WorkflowStepStateActive
	}
	return currentState
}

func updateStageStateFromSteps(steps []model.WorkflowDependencyStep, currentState model.WorkflowStageState) model.WorkflowStageState {
	allCompleted := true
	for _, step := range steps {
		if step.State == model.WorkflowStepStateFailed {
			return model.WorkflowStageStateFailed
		}
		if step.State != model.WorkflowStepStateCompleted {
			allCompleted = false
		}
	}
	if allCompleted {
		return model.WorkflowStageStateCompleted
	}
	if currentState == model.WorkflowStageStateFailed {
		return currentState
	}
	for _, step := range steps {
		if step.State == model.WorkflowStepStateActive {
			return model.WorkflowStageStateActive
		}
	}
	for _, step := range steps {
		if step.State == model.WorkflowStepStateCompleted {
			return model.WorkflowStageStateActive
		}
	}
	return currentState
}

func updateWorkflowStateFromStages(plan *model.WorkflowDependencyPlan) {
	for _, stage := range plan.Stages {
		if stage.State == model.WorkflowStageStateFailed {
			plan.State = model.WorkflowStateFailed
			return
		}
	}
	for _, stage := range plan.Stages {
		if stage.State != model.WorkflowStageStateCompleted {
			plan.State = model.WorkflowStateRunning
			return
		}
	}
	plan.State = model.WorkflowStateCompleted
}

func pruneWorkItemOutputJSON(step *model.WorkflowDependencyStep) {
	if step == nil {
		return
	}
	for index := range step.WorkItems {
		item := &step.WorkItems[index]
		if item.OutputJSON == "" {
			continue
		}
		item.OutputJSONBytes = len([]byte(item.OutputJSON))
		item.OutputJSON = ""
		item.OutputJSONPruned = true
	}
}

func pruneStepOutputJSON(step *model.WorkflowDependencyStep) {
	if step == nil || step.OutputJSON == "" {
		return
	}
	step.OutputJSONBytes = len([]byte(step.OutputJSON))
	step.OutputJSON = ""
	step.OutputJSONPruned = true
}

func pruneDependencyOutputsIfTerminal(plan *model.WorkflowDependencyPlan) {
	if plan == nil || (plan.State != model.WorkflowStateCompleted && plan.State != model.WorkflowStateFailed) {
		return
	}
	for stageIndex := range plan.Stages {
		for stepIndex := range plan.Stages[stageIndex].Steps {
			step := &plan.Stages[stageIndex].Steps[stepIndex]
			pruneWorkItemOutputJSON(step)
			pruneStepOutputJSON(step)
		}
	}
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

func canonicalOutputFromEmptyList() (string, string, error) {
	json, sha256, err := canonicalOutputJSONFromResolved(variable.ResolvedValue{
		Type: variable.TypeList,
		List: []variable.ResolvedValue{},
	})
	if err != nil {
		return "", "", err
	}
	return json, sha256, nil
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
		StageIndex:       step.StageIndex,
		StepIndex:        step.StepIndex,
		StepID:           step.StepID,
		State:            step.State,
		OutputJSON:       step.OutputJSON,
		OutputJSONSHA256: step.OutputJSONSHA256,
		OutputJSONBytes:  step.OutputJSONBytes,
		OutputJSONPruned: step.OutputJSONPruned,
		WorkItems:        make([]model.WorkflowDependencyWorkItemMembership, 0, len(step.WorkItems)),
	}
	for _, item := range step.WorkItems {
		clone.WorkItems = append(clone.WorkItems, model.WorkflowDependencyWorkItemMembership{
			WorkItemID:       item.WorkItemID,
			WorkItemIndex:    item.WorkItemIndex,
			State:            item.State,
			OutputJSON:       item.OutputJSON,
			OutputJSONSHA256: item.OutputJSONSHA256,
			OutputJSONBytes:  item.OutputJSONBytes,
			OutputJSONPruned: item.OutputJSONPruned,
		})
	}
	return clone
}
