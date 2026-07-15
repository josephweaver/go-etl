package main

import (
	"context"
	"time"

	"goetl/internal/ledger"
	"goetl/internal/model"
)

func (c *Controller) recordSkippedAttempt(ctx context.Context, item model.WorkItem, skippedAt time.Time) (model.WorkSkip, bool, error) {
	decision, err := c.workReuseDecision(ctx, item)
	if err != nil {
		return model.WorkSkip{}, false, err
	}

	skip, ok, err := workSkipForReuseDecision(item, decision)
	if err != nil || !ok {
		return model.WorkSkip{}, false, err
	}

	attempt, err := skippedAttemptFromWorkSkip(item, skip, skippedAt)
	if err != nil {
		return model.WorkSkip{}, false, err
	}
	if err := c.recordAttempt(ctx, attempt); err != nil {
		return model.WorkSkip{}, false, err
	}

	return skip, true, nil
}

func (c *Controller) priorCompletedAttempt(ctx context.Context, item model.WorkItem) (ledger.Attempt, bool, error) {
	if c.ledger == nil || item.WorkItemFingerprint == "" {
		return ledger.Attempt{}, false, nil
	}

	return ledger.FindLatestCompletedAttemptByWorkItemFingerprint(ctx, c.ledger, item.WorkItemFingerprint)
}

func priorCompletedAttemptMatchesWorkItem(item model.WorkItem, attempt ledger.Attempt) bool {
	if attempt.Status != ledger.AttemptStatusCompleted {
		return false
	}
	if item.WorkItemFingerprint == "" || item.InputFingerprint == "" || item.OutputFingerprint == "" || item.CodeVersion == "" {
		return false
	}

	return item.WorkItemFingerprint == attempt.WorkItemFingerprint &&
		item.InputFingerprint == attempt.InputFingerprint &&
		item.OutputFingerprint == attempt.OutputFingerprint &&
		item.CodeVersion == attempt.CodeVersion
}

func (c *Controller) reusablePriorAttempt(ctx context.Context, item model.WorkItem) (ledger.Attempt, bool, error) {
	attempt, ok, err := c.priorCompletedAttempt(ctx, item)
	if err != nil || !ok {
		return ledger.Attempt{}, false, err
	}
	if !priorCompletedAttemptMatchesWorkItem(item, attempt) {
		return ledger.Attempt{}, false, nil
	}

	return attempt, true, nil
}

func (c *Controller) workReuseDecision(ctx context.Context, item model.WorkItem) (WorkReuseDecision, error) {
	attempt, ok, err := c.priorCompletedAttempt(ctx, item)
	if err != nil {
		return WorkReuseDecision{}, err
	}
	if !ok {
		return WorkReuseDecision{Reason: "no_prior_completed_attempt"}, nil
	}
	if !priorCompletedAttemptMatchesWorkItem(item, attempt) {
		return WorkReuseDecision{
			Reason:         "prior_attempt_mismatch",
			PriorAttemptID: attempt.ID,
		}, nil
	}

	return WorkReuseDecision{
		Reusable:       true,
		Reason:         "matched_prior_completed_attempt",
		PriorAttemptID: attempt.ID,
	}, nil
}

func workSkipForReuseDecision(item model.WorkItem, decision WorkReuseDecision) (model.WorkSkip, bool, error) {
	if !decision.Reusable {
		return model.WorkSkip{}, false, nil
	}

	skip := model.WorkSkip{
		ID:             item.ID,
		PriorAttemptID: decision.PriorAttemptID,
		Reason:         decision.Reason,
	}
	if err := skip.Validate(); err != nil {
		return model.WorkSkip{}, false, err
	}

	return skip, true, nil
}
