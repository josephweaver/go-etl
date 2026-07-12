package main

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"goetl/internal/model"
	"goetl/internal/persistence"
	"goetl/internal/workflow"
)

type compiledStageWorkItemMembership struct {
	stageIndex    int
	stepIndex     int
	workItemID    string
	workItemIndex int
}

func splitCompiledWorkflowByStage(compileResult workflow.CompileResult, plan workflow.WorkflowPlan) ([]workflow.CompileStageResult, error) {
	stepIndexByID := make(map[string]int, len(compileResult.WorkItems))
	stageIndexByID := make(map[string]int, len(compileResult.WorkItems))
	for _, stage := range plan.Stages {
		for _, stageStep := range stage.Steps {
			stepIndexByID[stageStep.StepID] = stageStep.StepIndex
			stageIndexByID[stageStep.StepID] = stage.Index
		}
	}

	orderedStageIndexes := make([]int, 0, len(plan.Stages))
	for _, stage := range plan.Stages {
		orderedStageIndexes = append(orderedStageIndexes, stage.Index)
	}
	sort.Ints(orderedStageIndexes)

	nextWorkItemIndex := make(map[string]int, len(plan.Stages))
	stageItemsByIndex := make(map[int][]workflow.CompileStageWorkItem, len(plan.Stages))

	for _, item := range compileResult.WorkItems {
		stepIndex, hasStepIndex := stepIndexByID[item.StepID]
		if !hasStepIndex {
			return nil, fmt.Errorf("compiled work item references unknown step: %s", item.StepID)
		}
		stageIndex, hasStageIndex := stageIndexByID[item.StepID]
		if !hasStageIndex {
			return nil, fmt.Errorf("compiled work item step has no stage: %s", item.StepID)
		}

		workItemIndex := nextWorkItemIndex[item.StepID]
		nextWorkItemIndex[item.StepID]++
		stageItemsByIndex[stageIndex] = append(stageItemsByIndex[stageIndex], workflow.CompileStageWorkItem{
			WorkflowID:          compileResult.WorkflowID,
			StageIndex:          stageIndex,
			StepIndex:           stepIndex,
			StepID:              item.StepID,
			WorkItemIndex:       workItemIndex,
			WorkItem:            item.WorkItem,
			ResourceConstraints: item.ResourceConstraints,
		})
	}

	planStepsByStage := make(map[int][]workflow.WorkflowStageStep, len(plan.Stages))
	for _, stage := range plan.Stages {
		planStepsByStage[stage.Index] = append(planStepsByStage[stage.Index], stage.Steps...)
	}

	stageResults := make([]workflow.CompileStageResult, 0, len(orderedStageIndexes))
	for _, stageIndex := range orderedStageIndexes {
		steps := planStepsByStage[stageIndex]
		workItems := stageItemsByIndex[stageIndex]
		if len(workItems) == 0 {
			continue
		}

		sort.Slice(steps, func(i, j int) bool {
			return steps[i].StepIndex < steps[j].StepIndex
		})

		stageResults = append(stageResults, workflow.CompileStageResult{
			WorkflowID: compileResult.WorkflowID,
			StageIndex: stageIndex,
			Steps:      steps,
			WorkItems:  workItems,
		})
	}

	return stageResults, nil
}

func persistenceRecordsFromCompiledStageResults(
	runID string,
	stageResults []workflow.CompileStageResult,
	codeVersion string,
	submittedAt time.Time,
) ([]persistence.WorkItemRecord, []persistence.QueuedWorkRecord, []compiledStageWorkItemMembership, []persistence.WorkItemResourceConstraintRecord, error) {
	stepInstances := make(map[string]string, len(stageResults))
	timestamp := submittedAt.UTC().Format(time.RFC3339)
	persistenceItems := make([]persistence.WorkItemRecord, 0)
	queued := make([]persistence.QueuedWorkRecord, 0)
	memberships := make([]compiledStageWorkItemMembership, 0)
	resourceConstraints := make([]persistence.WorkItemResourceConstraintRecord, 0)
	nextWorkItemIndexByStage := make(map[int]int, len(stageResults))
	currentStageWorkItemIDs := map[string]struct{}{}
	payloadsByPersistedID := map[string]model.WorkItem{}
	membershipsByPersistedID := map[string]compiledStageWorkItemMembership{}

	workflowID := ""
	if len(stageResults) != 0 {
		workflowID = stageResults[0].WorkflowID
	}

	workflowFingerprint := fingerprint("workflow", map[string]any{
		"id": workflowID,
	})

	for _, stageResult := range stageResults {
		for _, item := range stageResult.WorkItems {
			workItemIndex := nextWorkItemIndexByStage[item.StageIndex]
			nextWorkItemIndexByStage[item.StageIndex]++

			if _, ok := stepInstances[item.StepID]; !ok {
				stepInstances[item.StepID] = runID + ":step:" + strconv.Itoa(item.StepIndex)
			}

			itemPayload := item.WorkItem
			itemPayload.WorkflowDefinitionID = stageResult.WorkflowID
			itemPayload.WorkflowFingerprint = workflowFingerprint
			itemPayload.WorkflowInstanceID = runID
			itemPayload.StepDefinitionID = item.StepID
			itemPayload.StepFingerprint = fingerprint("step", map[string]any{
				"workflow_fingerprint": workflowFingerprint,
				"id":                   item.StepID,
			})
			itemPayload.StepInstanceID = stepInstances[item.StepID]
			itemPayload, err := itemPayload.WithExecutionEnvelope()
			if err != nil {
				return nil, nil, nil, nil, fmt.Errorf("build execution envelope for work item %s: %w", item.WorkItem.ID, err)
			}

			itemPayload.WorkItemFingerprint = fingerprint("work-item", map[string]any{
				"id":              itemPayload.ID,
				"type":            itemPayload.Type,
				"output_filename": itemPayload.OutputFilename,
				"variables":       itemPayload.ExecutionEnvelope.Variables,
			})
			itemPayload.InputFingerprint = fingerprint("input", itemPayload.ExecutionEnvelope.Variables)
			itemPayload.OutputFingerprint = fingerprint("output", map[string]any{
				"output_filename": itemPayload.OutputFilename,
			})
			itemPayload.CodeVersion = codeVersion

			for index, dependencyID := range itemPayload.DependsOn {
				if dependencyID != "" && !strings.HasPrefix(dependencyID, runID+":") {
					itemPayload.DependsOn[index] = runID + ":" + dependencyID
				}
			}

			payload, err := json.Marshal(itemPayload)
			if err != nil {
				return nil, nil, nil, nil, fmt.Errorf("encode workflow work item: %w", err)
			}
			_, resolvedInputsSHA256, err := canonicalSourceDocument(payload)
			if err != nil {
				return nil, nil, nil, nil, fmt.Errorf("hash workflow work item: %w", err)
			}

			id := runID + ":" + itemPayload.ID
			record := persistence.WorkItemRecord{
				ID:                   id,
				RunID:                runID,
				StageIndex:           item.StageIndex,
				WorkItemIndex:        workItemIndex,
				WorkerPayloadJSON:    string(payload),
				ResolvedInputsSHA256: resolvedInputsSHA256,
				CreatedAt:            timestamp,
			}
			persistenceItems = append(persistenceItems, record)
			currentStageWorkItemIDs[id] = struct{}{}
			payloadsByPersistedID[id] = itemPayload
			if itemPayload.Type != model.WorkItemTypeAssetMaterialize {
				membershipsByPersistedID[id] = compiledStageWorkItemMembership{
					stageIndex:    item.StageIndex,
					stepIndex:     item.StepIndex,
					workItemID:    id,
					workItemIndex: workItemIndex,
				}
			}
			for _, constraint := range item.ResourceConstraints {
				constraint.WorkItemID = id
				constraint.CreatedAt = timestamp
				if err := constraint.Validate(); err != nil {
					return nil, nil, nil, nil, fmt.Errorf("validate resource constraint for work item %s: %w", id, err)
				}
				resourceConstraints = append(resourceConstraints, persistenceResourceConstraintRecord(constraint))
			}
		}
	}

	for _, record := range persistenceItems {
		payload := payloadsByPersistedID[record.ID]
		if hasCurrentStageDependency(payload.DependsOn, currentStageWorkItemIDs) {
			continue
		}
		queued = append(queued, persistence.QueuedWorkRecord{
			WorkItemRecord: record,
			QueuedAt:       timestamp,
		})
		if membership, ok := membershipsByPersistedID[record.ID]; ok {
			memberships = append(memberships, membership)
		}
	}

	return persistenceItems, queued, memberships, resourceConstraints, nil
}

func hasCurrentStageDependency(dependsOn []string, currentStageWorkItemIDs map[string]struct{}) bool {
	for _, dependencyID := range dependsOn {
		if _, ok := currentStageWorkItemIDs[dependencyID]; ok {
			return true
		}
	}
	return false
}

func persistenceResourceConstraintRecords(constraints []model.WorkItemResourceConstraint) []persistence.WorkItemResourceConstraintRecord {
	records := make([]persistence.WorkItemResourceConstraintRecord, 0, len(constraints))
	for _, constraint := range constraints {
		records = append(records, persistenceResourceConstraintRecord(constraint))
	}
	return records
}

func persistenceResourceConstraintRecord(constraint model.WorkItemResourceConstraint) persistence.WorkItemResourceConstraintRecord {
	return persistence.WorkItemResourceConstraintRecord{
		WorkItemID:      constraint.WorkItemID,
		ConstraintIndex: constraint.ConstraintIndex,
		ResourceKey:     constraint.ResourceKey,
		RequestedUnits:  constraint.RequestedUnits,
		Operator:        string(constraint.Operator),
		TargetUnits:     constraint.TargetUnits,
		CreatedAt:       constraint.CreatedAt,
	}
}
