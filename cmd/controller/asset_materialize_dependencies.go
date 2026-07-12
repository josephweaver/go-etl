package main

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"goetl/internal/model"
	"goetl/internal/persistence"
)

func (c *Controller) enqueueReadyAssetMaterializeDependents(ctx context.Context, completed persistence.WorkItemRecord, queuedAt time.Time) error {
	if c.workflowStore == nil {
		return nil
	}

	allWork, err := c.workflowStore.ListWorkItemsForRun(ctx, completed.RunID)
	if err != nil {
		return err
	}
	queued, err := c.workflowStore.ListQueuedWorkItems(ctx)
	if err != nil {
		return err
	}
	running, err := c.workflowStore.ListRunningWork(ctx)
	if err != nil {
		return err
	}
	terminal, err := c.workflowStore.ListTerminalAttemptsForRun(ctx, completed.RunID)
	if err != nil {
		return err
	}
	dependencySteps, err := c.workflowStore.ListWorkflowDependencySteps(ctx, completed.RunID)
	if err != nil {
		return err
	}

	queuedIDs := map[string]bool{}
	for _, item := range queued {
		queuedIDs[item.ID] = true
	}
	runningIDs := map[string]bool{}
	for _, item := range running {
		if item.WorkItem.RunID == completed.RunID {
			runningIDs[item.WorkItem.ID] = true
		}
	}
	completedIDs := map[string]bool{}
	failedIDs := map[string]bool{}
	for _, attempt := range terminal {
		switch attempt.TerminalState {
		case "completed":
			completedIDs[attempt.WorkItem.ID] = true
		case "failed":
			failedIDs[attempt.WorkItem.ID] = true
		}
	}

	stepsByStageAndID := make(map[string]persistence.WorkflowDependencyStepRecord, len(dependencySteps))
	for _, step := range dependencySteps {
		key := dependencyStepKey(step.StageIndex, step.StepID)
		stepsByStageAndID[key] = step
	}

	toQueue := []persistence.QueuedWorkRecord{}
	memberships := []compiledStageWorkItemMembership{}
	timestamp := queuedAt.UTC().Format(time.RFC3339)
	for _, record := range allWork {
		if record.ID == completed.ID || queuedIDs[record.ID] || runningIDs[record.ID] || completedIDs[record.ID] || failedIDs[record.ID] {
			continue
		}
		item, err := workItemPayloadFromRecord(record)
		if err != nil {
			return err
		}
		if len(item.DependsOn) == 0 || !containsString(item.DependsOn, completed.ID) {
			continue
		}
		ready := true
		for _, dependencyID := range item.DependsOn {
			if !completedIDs[dependencyID] {
				ready = false
				break
			}
		}
		if !ready {
			continue
		}

		step, ok := stepsByStageAndID[dependencyStepKey(record.StageIndex, item.StepDefinitionID)]
		if !ok {
			return fmt.Errorf("dependency step not found for asset_materialize dependent %s at stage %d step %s", record.ID, record.StageIndex, item.StepDefinitionID)
		}
		toQueue = append(toQueue, persistence.QueuedWorkRecord{
			WorkItemRecord: record,
			QueuedAt:       timestamp,
		})
		memberships = append(memberships, compiledStageWorkItemMembership{
			stageIndex:    record.StageIndex,
			stepIndex:     step.StepIndex,
			workItemID:    record.ID,
			workItemIndex: record.WorkItemIndex,
		})
	}

	if len(toQueue) == 0 {
		return nil
	}
	sort.Slice(toQueue, func(i, j int) bool {
		if toQueue[i].WorkItemIndex == toQueue[j].WorkItemIndex {
			return toQueue[i].ID < toQueue[j].ID
		}
		return toQueue[i].WorkItemIndex < toQueue[j].WorkItemIndex
	})
	if err := c.workflowStore.EnqueueWorkItems(ctx, toQueue); err != nil {
		return fmt.Errorf("enqueue asset_materialize dependent work: %w", err)
	}
	for _, membership := range memberships {
		if err := c.RecordCompiledWorkItemMembership(ctx, completed.RunID, membership.stageIndex, membership.stepIndex, membership.workItemID, membership.workItemIndex); err != nil {
			return fmt.Errorf("record asset_materialize dependent membership: %w", err)
		}
	}
	return nil
}

func (c *Controller) failAssetMaterializeDependents(ctx context.Context, failed persistence.WorkItemRecord, reason string) error {
	item, err := workItemPayloadFromRecord(failed)
	if err != nil {
		return err
	}
	if item.Type != model.WorkItemTypeAssetMaterialize {
		return nil
	}
	if reason == "" {
		reason = "asset_materialize failed"
	}
	return c.markWorkflowStageActivationFailed(ctx, failed.RunID, failed.StageIndex, reason)
}

func workItemPayloadFromRecord(record persistence.WorkItemRecord) (model.WorkItem, error) {
	var item model.WorkItem
	if err := json.Unmarshal([]byte(record.WorkerPayloadJSON), &item); err != nil {
		return model.WorkItem{}, fmt.Errorf("decode persisted work item %s: %w", record.ID, err)
	}
	return item, nil
}

func dependencyStepKey(stageIndex int, stepID string) string {
	return fmt.Sprintf("%d:%s", stageIndex, stepID)
}

func containsString(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}
