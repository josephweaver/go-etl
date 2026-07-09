package main

import (
	"context"
	"fmt"
	"sync"
	"time"

	"goetl/internal/model"
	"goetl/internal/variable"
)

const (
	workerExecutionPatternOneByOneUntilSaturated = "one_by_one_until_saturated"
	workerExecutionPatternNull                   = "null"
)

type WorkerDemand struct {
	PendingQueued    int
	PendingClaimable int
	RunningAttempts  int
}

type WorkerStartReservation struct {
	ID        string
	CreatedAt time.Time
}

type WorkerExecutionState struct {
	InflightStarts []WorkerStartReservation
}

type WorkerExecutionConfig struct {
	Pattern              string
	MaxActiveWorkers     int
	InflightStartTimeout time.Duration
}

type WorkerStartPlan struct {
	StartCount int
	Reason     string
}

type WorkerExecutionPattern interface {
	Plan(now time.Time, demand WorkerDemand, state WorkerExecutionState, cfg WorkerExecutionConfig) WorkerStartPlan
}

type OneByOneUntilSaturatedPattern struct{}

func (p OneByOneUntilSaturatedPattern) Plan(now time.Time, demand WorkerDemand, state WorkerExecutionState, cfg WorkerExecutionConfig) WorkerStartPlan {
	inflight := state.UnexpiredInflightStarts(now, cfg.InflightStartTimeout)
	active := demand.RunningAttempts + len(inflight)

	if demand.PendingClaimable <= 0 {
		return WorkerStartPlan{Reason: "no_claimable_work"}
	}
	if len(inflight) > 0 {
		return WorkerStartPlan{Reason: "waiting_for_inflight_start_claim"}
	}
	if cfg.MaxActiveWorkers > 0 && active >= cfg.MaxActiveWorkers {
		return WorkerStartPlan{Reason: "max_active_workers_reached"}
	}
	if active >= demand.PendingClaimable {
		return WorkerStartPlan{Reason: "active_capacity_satisfies_claimable_work"}
	}

	return WorkerStartPlan{
		StartCount: 1,
		Reason:     "active_capacity_below_claimable_work",
	}
}

type NullWorkerExecutionPattern struct{}

func (p NullWorkerExecutionPattern) Plan(now time.Time, demand WorkerDemand, state WorkerExecutionState, cfg WorkerExecutionConfig) WorkerStartPlan {
	return WorkerStartPlan{Reason: "worker_scheduling_disabled"}
}

func (s WorkerExecutionState) UnexpiredInflightStarts(now time.Time, timeout time.Duration) []WorkerStartReservation {
	if timeout <= 0 {
		return append([]WorkerStartReservation(nil), s.InflightStarts...)
	}

	unexpired := make([]WorkerStartReservation, 0, len(s.InflightStarts))
	for _, reservation := range s.InflightStarts {
		if now.Sub(reservation.CreatedAt) <= timeout {
			unexpired = append(unexpired, reservation)
		}
	}
	return unexpired
}

type WorkerCapacityManager struct {
	mu      sync.Mutex
	state   WorkerExecutionState
	pattern WorkerExecutionPattern
	nextID  int
}

func NewWorkerCapacityManager(pattern WorkerExecutionPattern) *WorkerCapacityManager {
	return &WorkerCapacityManager{pattern: pattern}
}

func (m *WorkerCapacityManager) Evaluate(
	ctx context.Context,
	now time.Time,
	cfg WorkerExecutionConfig,
	demandFn func(context.Context) (WorkerDemand, error),
	launchFn func(context.Context, int) error,
) (WorkerStartPlan, error) {
	if m == nil {
		return WorkerStartPlan{}, fmt.Errorf("worker capacity manager is required")
	}
	if demandFn == nil {
		return WorkerStartPlan{}, fmt.Errorf("worker demand function is required")
	}
	if launchFn == nil {
		return WorkerStartPlan{}, fmt.Errorf("worker launch function is required")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if cfg.Pattern == "" {
		cfg.Pattern = workerExecutionPatternOneByOneUntilSaturated
	}
	pattern, err := m.patternForConfig(cfg.Pattern)
	if err != nil {
		return WorkerStartPlan{}, err
	}

	demand, err := demandFn(ctx)
	if err != nil {
		return WorkerStartPlan{}, err
	}

	m.pruneExpiredInflightStarts(now, cfg.InflightStartTimeout)
	plan := pattern.Plan(now, demand, m.state, cfg)
	fmt.Printf("worker_capacity_evaluation pending_queued=%d pending_claimable=%d running_attempts=%d inflight_starts=%d start_count=%d reason=%s pattern=%s\n",
		demand.PendingQueued,
		demand.PendingClaimable,
		demand.RunningAttempts,
		len(m.state.InflightStarts),
		plan.StartCount,
		plan.Reason,
		cfg.Pattern,
	)
	if plan.StartCount <= 0 {
		return plan, nil
	}

	reservations := m.reserveInflightStarts(now, plan.StartCount)
	fmt.Printf("worker_start_requested start_count=%d reason=%s pattern=%s\n", plan.StartCount, plan.Reason, cfg.Pattern)
	if err := launchFn(ctx, plan.StartCount); err != nil {
		m.removeInflightReservations(reservations)
		fmt.Printf("worker_start_failed start_count=%d reason=%s error=%v\n", plan.StartCount, plan.Reason, err)
		return plan, err
	}
	return plan, nil
}

func (m *WorkerCapacityManager) patternForConfig(pattern string) (WorkerExecutionPattern, error) {
	switch pattern {
	case workerExecutionPatternOneByOneUntilSaturated:
		if m.pattern != nil {
			return m.pattern, nil
		}
		return OneByOneUntilSaturatedPattern{}, nil
	case workerExecutionPatternNull:
		return NullWorkerExecutionPattern{}, nil
	default:
		return nil, fmt.Errorf("unsupported worker execution pattern %q", pattern)
	}
}

func (m *WorkerCapacityManager) ConfirmInflightStartClaimed() bool {
	if m == nil {
		return false
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if len(m.state.InflightStarts) == 0 {
		return false
	}
	confirmed := m.state.InflightStarts[0]
	m.state.InflightStarts = append([]WorkerStartReservation(nil), m.state.InflightStarts[1:]...)
	fmt.Printf("worker_start_confirmed_by_claim reservation_id=%s\n", confirmed.ID)
	return true
}

func (m *WorkerCapacityManager) Snapshot() WorkerExecutionState {
	if m == nil {
		return WorkerExecutionState{}
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	return WorkerExecutionState{
		InflightStarts: append([]WorkerStartReservation(nil), m.state.InflightStarts...),
	}
}

func (m *WorkerCapacityManager) pruneExpiredInflightStarts(now time.Time, timeout time.Duration) {
	if timeout <= 0 {
		return
	}

	unexpired := make([]WorkerStartReservation, 0, len(m.state.InflightStarts))
	for _, reservation := range m.state.InflightStarts {
		if now.Sub(reservation.CreatedAt) <= timeout {
			unexpired = append(unexpired, reservation)
			continue
		}
		fmt.Printf("worker_start_reservation_expired reservation_id=%s\n", reservation.ID)
	}
	m.state.InflightStarts = unexpired
}

func (m *WorkerCapacityManager) reserveInflightStarts(now time.Time, count int) []WorkerStartReservation {
	reservations := make([]WorkerStartReservation, 0, count)
	for range count {
		m.nextID++
		reservation := WorkerStartReservation{
			ID:        fmt.Sprintf("worker-start-%d", m.nextID),
			CreatedAt: now,
		}
		m.state.InflightStarts = append(m.state.InflightStarts, reservation)
		reservations = append(reservations, reservation)
	}
	return reservations
}

func (m *WorkerCapacityManager) removeInflightReservations(reservations []WorkerStartReservation) {
	remove := make(map[string]struct{}, len(reservations))
	for _, reservation := range reservations {
		remove[reservation.ID] = struct{}{}
	}

	kept := make([]WorkerStartReservation, 0, len(m.state.InflightStarts))
	for _, reservation := range m.state.InflightStarts {
		if _, ok := remove[reservation.ID]; !ok {
			kept = append(kept, reservation)
		}
	}
	m.state.InflightStarts = kept
}

func (c *Controller) EvaluateWorkerCapacity(ctx context.Context, now time.Time) error {
	if c.workerExecutor == nil {
		c.workerExecutor = NewWorkerCapacityManager(nil)
	}

	cfg, err := workerExecutionConfig(c.launchResolver, defaultWorkerExecutionConfig())
	if err != nil {
		return err
	}
	_, err = c.workerExecutor.Evaluate(ctx, now, cfg, c.workerDemand, func(ctx context.Context, count int) error {
		return c.startWorkers(ctx, c.launchResolver, count)
	})
	return err
}

func (c *Controller) ConfirmWorkerStartClaimed() bool {
	if c.workerExecutor == nil {
		return false
	}
	return c.workerExecutor.ConfirmInflightStartClaimed()
}

func (c *Controller) ConfirmWorkerStartClaimedAndEvaluateAsync() {
	if !c.ConfirmWorkerStartClaimed() {
		return
	}
	if !c.asyncWorkerCapacity {
		return
	}
	go func() {
		time.Sleep(50 * time.Millisecond)
		if err := c.EvaluateWorkerCapacity(context.Background(), time.Now().UTC()); err != nil {
			fmt.Println("worker capacity evaluation after claim failed:", err)
		}
	}()
}

func (c *Controller) workerDemand(ctx context.Context) (WorkerDemand, error) {
	if c.workflowStore == nil {
		return WorkerDemand{}, fmt.Errorf("workflow store required")
	}

	queued, err := c.workflowStore.ListQueuedWorkItems(ctx)
	if err != nil {
		return WorkerDemand{}, fmt.Errorf("list queued work: %w", err)
	}
	running, err := c.workflowStore.ListRunningWork(ctx)
	if err != nil {
		return WorkerDemand{}, fmt.Errorf("list running work: %w", err)
	}
	checks, err := c.workflowStore.ListQueuedResourceConstraintChecks(ctx)
	if err != nil {
		return WorkerDemand{}, fmt.Errorf("list queued resource constraint checks: %w", err)
	}

	queuedIDs := make(map[string]struct{}, len(queued))
	for _, item := range queued {
		queuedIDs[item.ID] = struct{}{}
	}
	checksByWorkItem := make(map[string][]model.ResourceConstraintCheck)
	for _, check := range checks {
		if _, ok := queuedIDs[check.WorkItemID]; !ok {
			continue
		}
		checksByWorkItem[check.WorkItemID] = append(checksByWorkItem[check.WorkItemID], model.ResourceConstraintCheck{
			TotalUnits:     check.TotalUnits,
			RequestedUnits: check.RequestedUnits,
			Operator:       model.ResourceOperator(check.Operator),
			TargetUnits:    check.TargetUnits,
		})
	}

	blocked := 0
	for workItemID, itemChecks := range checksByWorkItem {
		allowed, err := model.ResourceConstraintChecksAllow(itemChecks)
		if err != nil {
			return WorkerDemand{}, fmt.Errorf("evaluate resource constraints for %s: %w", workItemID, err)
		}
		if !allowed {
			blocked++
		}
	}

	return WorkerDemand{
		PendingQueued:    len(queued),
		PendingClaimable: len(queued) - blocked,
		RunningAttempts:  len(running),
	}, nil
}

func defaultWorkerExecutionConfig() WorkerExecutionConfig {
	return WorkerExecutionConfig{
		Pattern:              workerExecutionPatternOneByOneUntilSaturated,
		MaxActiveWorkers:     2,
		InflightStartTimeout: 5 * time.Minute,
	}
}

func workerExecutionConfig(resolver variable.Resolver, defaults WorkerExecutionConfig) (WorkerExecutionConfig, error) {
	cfg := defaults
	var err error

	if cfg.Pattern, err = optionalStringVariableWithFallback(resolver, "worker_execution_pattern", cfg.Pattern); err != nil {
		return WorkerExecutionConfig{}, err
	}
	if cfg.MaxActiveWorkers, err = optionalIntVariable(resolver, "worker_max_active", cfg.MaxActiveWorkers); err != nil {
		return WorkerExecutionConfig{}, err
	}
	if cfg.InflightStartTimeout, err = optionalDurationVariable(resolver, "worker_inflight_start_timeout", cfg.InflightStartTimeout); err != nil {
		return WorkerExecutionConfig{}, err
	}
	if cfg.InflightStartTimeout < 0 {
		return WorkerExecutionConfig{}, fmt.Errorf("worker_inflight_start_timeout must be non-negative")
	}

	return cfg, nil
}

func optionalStringVariableWithFallback(resolver variable.Resolver, name string, fallback string) (string, error) {
	reference, err := variable.ParseReference(name)
	if err != nil {
		return "", err
	}

	value, err := resolver.Resolve(reference)
	if err != nil {
		return fallback, nil
	}

	if value.Type != variable.TypeString {
		return "", fmt.Errorf("%s has type %s, want string", name, value.Type)
	}

	text, ok := value.Value.(string)
	if !ok || text == "" {
		return "", fmt.Errorf("%s is required", name)
	}

	return text, nil
}
