package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"goetl/internal/controllerhttp"
	"goetl/internal/model"
)

func TestExecutionSupervisorReturnsOneExecutionResult(t *testing.T) {
	for _, test := range []struct {
		name     string
		result   ExecutionResult
		wantKind ExecutionSupervisorOutcomeKind
	}{
		{
			name:     "completed",
			result:   ExecutionResult{Evidence: WorkEvidence{OutputSHA256: strings.Repeat("a", 64)}},
			wantKind: ExecutionSupervisorCompleted,
		},
		{
			name:     "failed",
			result:   ExecutionResult{Err: errors.New("causal failure")},
			wantKind: ExecutionSupervisorFailed,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			execution := newSupervisorTestExecution()
			supervisor, _ := newTestExecutionSupervisor(t, CheckpointModePeriodic, execution)
			result := runSupervisorAsync(supervisor)
			execution.result <- test.result

			outcome := receiveSupervisorOutcome(t, result)
			if outcome.err != nil {
				t.Fatalf("Run() error = %v", outcome.err)
			}
			if outcome.outcome.Kind != test.wantKind {
				t.Fatalf("outcome kind = %q, want %q", outcome.outcome.Kind, test.wantKind)
			}
			if test.wantKind == ExecutionSupervisorCompleted && outcome.outcome.Evidence.OutputSHA256 != test.result.Evidence.OutputSHA256 {
				t.Fatalf("completion evidence = %+v", outcome.outcome.Evidence)
			}
			if test.wantKind == ExecutionSupervisorFailed && !errors.Is(outcome.outcome.Err, test.result.Err) {
				t.Fatalf("failure = %v, want %v", outcome.outcome.Err, test.result.Err)
			}
			if execution.terminationCount() != 0 {
				t.Fatalf("termination calls = %d, want 0", execution.terminationCount())
			}
		})
	}
}

func TestExecutionSupervisorConfirmsPeriodicCheckpointAndContinues(t *testing.T) {
	execution := newSupervisorTestExecution()
	supervisor, clock := newTestExecutionSupervisor(t, CheckpointModePeriodic, execution)
	registration := supervisor.Registration
	execution.periodic = func(_ context.Context, request CheckpointCaptureRequest) (PreparedCheckpoint, error) {
		return preparedSupervisorCheckpoint(t, request, registration), nil
	}

	confirmed := make(chan model.WorkCheckpointConfirmation, 1)
	client := &supervisorTestCheckpointClient{}
	client.confirm = func(_ context.Context, _ WorkerSession, request model.WorkCheckpointConfirmation) (model.WorkCheckpointAcknowledgement, error) {
		confirmed <- request
		return supervisorAcknowledgement(t, request), nil
	}
	supervisor.Checkpoints = client

	result := runSupervisorAsync(supervisor)
	clock.waitForTimer(t, 5*time.Second).fire()
	confirmation := receiveSupervisorConfirmation(t, confirmed)
	if confirmation.CaptureKind != model.CheckpointCaptureKindPeriodic || confirmation.Disposition != model.CheckpointDispositionContinue {
		t.Fatalf("confirmation = %+v", confirmation)
	}
	manifest := decodeSupervisorManifest(t, confirmation.ManifestJSON)
	if manifest.ResumeGeneration != 1 || manifest.ExecutionLineageID != "lineage-001" {
		t.Fatalf("manifest generation/lineage = %d/%q", manifest.ResumeGeneration, manifest.ExecutionLineageID)
	}

	execution.result <- ExecutionResult{Evidence: WorkEvidence{OutputSHA256: strings.Repeat("b", 64)}}
	outcome := receiveSupervisorOutcome(t, result)
	if outcome.err != nil || outcome.outcome.Kind != ExecutionSupervisorCompleted {
		t.Fatalf("Run() = %+v, %v", outcome.outcome, outcome.err)
	}
	if execution.periodicCount() != 1 || execution.suspendCount() != 0 || execution.terminationCount() != 0 {
		t.Fatalf("execution calls periodic=%d suspend=%d terminate=%d", execution.periodicCount(), execution.suspendCount(), execution.terminationCount())
	}
}

func TestExecutionSupervisorRetriesExactPeriodicConfirmation(t *testing.T) {
	execution := newSupervisorTestExecution()
	supervisor, clock := newTestExecutionSupervisor(t, CheckpointModePeriodic, execution)
	registration := supervisor.Registration
	execution.periodic = func(_ context.Context, request CheckpointCaptureRequest) (PreparedCheckpoint, error) {
		return preparedSupervisorCheckpoint(t, request, registration), nil
	}

	requests := make(chan model.WorkCheckpointConfirmation, 2)
	confirmCalls := 0
	client := &supervisorTestCheckpointClient{}
	client.confirm = func(_ context.Context, _ WorkerSession, request model.WorkCheckpointConfirmation) (model.WorkCheckpointAcknowledgement, error) {
		confirmCalls++
		requests <- request
		if confirmCalls == 1 {
			return model.WorkCheckpointAcknowledgement{}, errors.New("ambiguous transport failure")
		}
		return supervisorAcknowledgement(t, request), nil
	}
	supervisor.Checkpoints = client

	result := runSupervisorAsync(supervisor)
	clock.waitForTimer(t, 5*time.Second).fire()
	first := receiveSupervisorConfirmation(t, requests)
	clock.waitForTimer(t, time.Second).fire()
	second := receiveSupervisorConfirmation(t, requests)
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("confirmation replay changed\nfirst:  %+v\nsecond: %+v", first, second)
	}

	execution.result <- ExecutionResult{}
	outcome := receiveSupervisorOutcome(t, result)
	if outcome.err != nil || outcome.outcome.Kind != ExecutionSupervisorCompleted {
		t.Fatalf("Run() = %+v, %v", outcome.outcome, outcome.err)
	}
}

func TestExecutionSupervisorPeriodicCaptureFailureKeepsGeneration(t *testing.T) {
	execution := newSupervisorTestExecution()
	supervisor, clock := newTestExecutionSupervisor(t, CheckpointModePeriodic, execution)
	registration := supervisor.Registration
	captures := 0
	execution.periodic = func(_ context.Context, request CheckpointCaptureRequest) (PreparedCheckpoint, error) {
		captures++
		if captures == 1 {
			return PreparedCheckpoint{}, errors.New("capture failed")
		}
		return preparedSupervisorCheckpoint(t, request, registration), nil
	}
	confirmed := make(chan model.WorkCheckpointConfirmation, 1)
	client := &supervisorTestCheckpointClient{}
	client.confirm = func(_ context.Context, _ WorkerSession, request model.WorkCheckpointConfirmation) (model.WorkCheckpointAcknowledgement, error) {
		confirmed <- request
		return supervisorAcknowledgement(t, request), nil
	}
	supervisor.Checkpoints = client

	result := runSupervisorAsync(supervisor)
	clock.waitForTimer(t, 5*time.Second).fire()
	waitForSupervisorCallCount(t, execution.periodicCount, 1)
	clock.waitForTimer(t, 5*time.Second).fire()
	confirmation := receiveSupervisorConfirmation(t, confirmed)
	if generation := decodeSupervisorManifest(t, confirmation.ManifestJSON).ResumeGeneration; generation != 1 {
		t.Fatalf("generation after failed capture = %d, want 1", generation)
	}

	execution.result <- ExecutionResult{}
	outcome := receiveSupervisorOutcome(t, result)
	if outcome.err != nil || outcome.outcome.Kind != ExecutionSupervisorCompleted {
		t.Fatalf("Run() = %+v, %v", outcome.outcome, outcome.err)
	}
}

func TestExecutionSupervisorPeriodicOwnershipConflictAbandons(t *testing.T) {
	execution := newSupervisorTestExecution()
	supervisor, clock := newTestExecutionSupervisor(t, CheckpointModePeriodic, execution)
	registration := supervisor.Registration
	execution.periodic = func(_ context.Context, request CheckpointCaptureRequest) (PreparedCheckpoint, error) {
		return preparedSupervisorCheckpoint(t, request, registration), nil
	}
	client := &supervisorTestCheckpointClient{}
	client.confirm = func(context.Context, WorkerSession, model.WorkCheckpointConfirmation) (model.WorkCheckpointAcknowledgement, error) {
		return model.WorkCheckpointAcknowledgement{}, controllerhttp.StatusError{StatusCode: 409, Body: "assignment_no_longer_owned"}
	}
	supervisor.Checkpoints = client

	result := runSupervisorAsync(supervisor)
	clock.waitForTimer(t, 5*time.Second).fire()
	outcome := receiveSupervisorOutcome(t, result)
	if outcome.err != nil {
		t.Fatalf("Run() setup error = %v", outcome.err)
	}
	if outcome.outcome.Kind != ExecutionSupervisorAbandonWithoutTerminalReport ||
		!strings.Contains(outcome.outcome.Err.Error(), "lost assignment ownership") ||
		execution.terminationCount() != 1 {
		t.Fatalf("outcome/termination = %+v/%d", outcome.outcome, execution.terminationCount())
	}
}

func TestExecutionSupervisorResumeContinuesLineageAndGeneration(t *testing.T) {
	execution := newSupervisorTestExecution()
	supervisor, clock := newTestExecutionSupervisor(t, CheckpointModePeriodic, execution)
	registration := supervisor.Registration
	consumedRequest := CheckpointCaptureRequest{
		WorkItemID:                  supervisor.Item.ID,
		WorkItemType:                supervisor.Item.Type,
		AttemptID:                   "attempt-previous",
		ExecutionLineageID:          "lineage-resumed",
		ResumeGeneration:            3,
		ResumeArtifactID:            "artifact-consumed",
		ArtifactStorageRelativePath: "goetl/resume/artifact-consumed",
		CaptureKind:                 model.CheckpointCaptureKindPeriodic,
	}
	consumed := preparedSupervisorCheckpoint(t, consumedRequest, registration)
	supervisor.Item.Resume = &model.WorkItemResumeAssignment{
		Schema:               model.WorkItemResumeAssignmentSchemaV1,
		ResumedFromAttemptID: consumedRequest.AttemptID,
		ExecutionLineageID:   consumedRequest.ExecutionLineageID,
		ResumeAttemptNumber:  1,
		ManifestJSON:         consumed.ManifestJSON,
		Reference:            consumed.Reference,
	}
	supervisor.NewID = func(kind string) (string, error) {
		if kind != "artifact" {
			return "", fmt.Errorf("unexpected id kind %q", kind)
		}
		return "artifact-next", nil
	}
	execution.periodic = func(_ context.Context, request CheckpointCaptureRequest) (PreparedCheckpoint, error) {
		return preparedSupervisorCheckpoint(t, request, registration), nil
	}
	confirmed := make(chan model.WorkCheckpointConfirmation, 1)
	client := &supervisorTestCheckpointClient{}
	client.confirm = func(_ context.Context, _ WorkerSession, request model.WorkCheckpointConfirmation) (model.WorkCheckpointAcknowledgement, error) {
		confirmed <- request
		return supervisorAcknowledgement(t, request), nil
	}
	supervisor.Checkpoints = client

	result := runSupervisorAsync(supervisor)
	clock.waitForTimer(t, 5*time.Second).fire()
	manifest := decodeSupervisorManifest(t, receiveSupervisorConfirmation(t, confirmed).ManifestJSON)
	if manifest.ResumeGeneration != 4 || manifest.ExecutionLineageID != consumedRequest.ExecutionLineageID {
		t.Fatalf("resumed manifest generation/lineage = %d/%q", manifest.ResumeGeneration, manifest.ExecutionLineageID)
	}
	execution.result <- ExecutionResult{}
	if outcome := receiveSupervisorOutcome(t, result); outcome.err != nil || outcome.outcome.Kind != ExecutionSupervisorCompleted {
		t.Fatalf("Run() = %+v, %v", outcome.outcome, outcome.err)
	}
}

func TestExecutionSupervisorYieldSuspendsAndTerminates(t *testing.T) {
	execution := newSupervisorTestExecution()
	supervisor, clock := newTestExecutionSupervisor(t, CheckpointModeYield, execution)
	registration := supervisor.Registration
	execution.suspend = func(_ context.Context, request CheckpointCaptureRequest) (PreparedCheckpoint, error) {
		return preparedSupervisorCheckpoint(t, request, registration), nil
	}

	confirmed := make(chan model.WorkCheckpointConfirmation, 1)
	client := &supervisorTestCheckpointClient{}
	client.confirm = func(_ context.Context, _ WorkerSession, request model.WorkCheckpointConfirmation) (model.WorkCheckpointAcknowledgement, error) {
		confirmed <- request
		return supervisorAcknowledgement(t, request), nil
	}
	supervisor.Checkpoints = client

	result := runSupervisorAsync(supervisor)
	clock.waitForTimer(t, 5*time.Second).fire()
	outcome := receiveSupervisorOutcome(t, result)
	confirmation := receiveSupervisorConfirmation(t, confirmed)
	if confirmation.CaptureKind != model.CheckpointCaptureKindQuantum || confirmation.Disposition != model.CheckpointDispositionSuspend {
		t.Fatalf("confirmation = %+v", confirmation)
	}
	if outcome.err != nil || outcome.outcome.Kind != ExecutionSupervisorSuspended {
		t.Fatalf("Run() = %+v, %v", outcome.outcome, outcome.err)
	}
	if !outcome.outcome.Acknowledgement.Suspended || execution.terminationCount() != 1 {
		t.Fatalf("acknowledgement/termination = %+v/%d", outcome.outcome.Acknowledgement, execution.terminationCount())
	}
}

func TestExecutionSupervisorFinalCaptureFallsBackToAcceptedPeriodicCheckpoint(t *testing.T) {
	execution := newSupervisorTestExecution()
	supervisor, clock := newTestExecutionSupervisor(t, CheckpointModePeriodic, execution)
	registration := supervisor.Registration
	execution.periodic = func(_ context.Context, request CheckpointCaptureRequest) (PreparedCheckpoint, error) {
		return preparedSupervisorCheckpoint(t, request, registration), nil
	}
	execution.suspend = func(context.Context, CheckpointCaptureRequest) (PreparedCheckpoint, error) {
		return PreparedCheckpoint{}, errors.New("final capture failed")
	}

	periodicAccepted := make(chan model.WorkCheckpointAcknowledgement, 1)
	fallbackRequests := make(chan model.WorkCheckpointSuspendLatest, 1)
	client := &supervisorTestCheckpointClient{}
	client.confirm = func(_ context.Context, _ WorkerSession, request model.WorkCheckpointConfirmation) (model.WorkCheckpointAcknowledgement, error) {
		acknowledgement := supervisorAcknowledgement(t, request)
		periodicAccepted <- acknowledgement
		return acknowledgement, nil
	}
	client.suspendLatest = func(_ context.Context, _ WorkerSession, request model.WorkCheckpointSuspendLatest) (model.WorkCheckpointAcknowledgement, error) {
		fallbackRequests <- request
		accepted := <-periodicAccepted
		accepted.Operation = model.CheckpointOperationSuspendLatest
		accepted.Disposition = model.CheckpointDispositionSuspend
		accepted.Suspended = true
		accepted.SuspendedAt = request.SuspendedAt
		return accepted, nil
	}
	supervisor.Checkpoints = client
	drains := NewWorkerDrainRequests()
	supervisor.Drain = drains

	result := runSupervisorAsync(supervisor)
	clock.waitForTimer(t, 5*time.Second).fire()
	waitForSupervisorCallCount(t, execution.periodicCount, 1)
	if _, err := drains.Request(WorkerDrainRequest{Reason: WorkerDrainReasonSlurmSignal, RequestedAt: clock.Now()}); err != nil {
		t.Fatal(err)
	}
	clock.waitForTimer(t, 10*time.Second).fire()

	outcome := receiveSupervisorOutcome(t, result)
	fallback := receiveSupervisorFallback(t, fallbackRequests)
	if fallback.SuspendReason != model.CheckpointSuspendReasonShutdown {
		t.Fatalf("fallback reason = %q", fallback.SuspendReason)
	}
	if outcome.err != nil || outcome.outcome.Kind != ExecutionSupervisorSuspended || execution.terminationCount() != 1 {
		t.Fatalf("Run() = %+v, %v; terminations=%d", outcome.outcome, outcome.err, execution.terminationCount())
	}
}

func TestExecutionSupervisorDrainAllowsCompletionBeforeEscalation(t *testing.T) {
	execution := newSupervisorTestExecution()
	supervisor, clock := newTestExecutionSupervisor(t, CheckpointModeShutdown, execution)
	drains := NewWorkerDrainRequests()
	supervisor.Drain = drains
	result := runSupervisorAsync(supervisor)

	if _, err := drains.Request(WorkerDrainRequest{Reason: WorkerDrainReasonSlurmSignal, RequestedAt: clock.Now()}); err != nil {
		t.Fatal(err)
	}
	clock.waitForTimer(t, 10*time.Second)
	execution.result <- ExecutionResult{Evidence: WorkEvidence{OutputSHA256: strings.Repeat("c", 64)}}

	outcome := receiveSupervisorOutcome(t, result)
	if outcome.err != nil || outcome.outcome.Kind != ExecutionSupervisorCompleted {
		t.Fatalf("Run() = %+v, %v", outcome.outcome, outcome.err)
	}
	if execution.suspendCount() != 0 || execution.terminationCount() != 0 {
		t.Fatalf("execution calls suspend=%d terminate=%d", execution.suspendCount(), execution.terminationCount())
	}
}

func TestExecutionSupervisorAbandonsWhenSuspensionHasNoFallback(t *testing.T) {
	execution := newSupervisorTestExecution()
	supervisor, clock := newTestExecutionSupervisor(t, CheckpointModeYield, execution)
	execution.suspend = func(context.Context, CheckpointCaptureRequest) (PreparedCheckpoint, error) {
		return PreparedCheckpoint{}, errors.New("capture failed")
	}
	result := runSupervisorAsync(supervisor)
	clock.waitForTimer(t, 5*time.Second).fire()

	outcome := receiveSupervisorOutcome(t, result)
	if outcome.err != nil {
		t.Fatalf("Run() setup error = %v", outcome.err)
	}
	if outcome.outcome.Kind != ExecutionSupervisorAbandonWithoutTerminalReport || outcome.outcome.Err == nil {
		t.Fatalf("outcome = %+v", outcome.outcome)
	}
	if execution.terminationCount() != 1 {
		t.Fatalf("termination calls = %d, want 1", execution.terminationCount())
	}
}

func TestExecutionSupervisorTerminationTimeoutCannotReportSuspended(t *testing.T) {
	execution := newSupervisorTestExecution()
	supervisor, clock := newTestExecutionSupervisor(t, CheckpointModeYield, execution)
	registration := supervisor.Registration
	execution.suspend = func(_ context.Context, request CheckpointCaptureRequest) (PreparedCheckpoint, error) {
		return preparedSupervisorCheckpoint(t, request, registration), nil
	}
	execution.terminate = func(ctx context.Context) error {
		<-ctx.Done()
		return ctx.Err()
	}
	client := &supervisorTestCheckpointClient{}
	client.confirm = func(_ context.Context, _ WorkerSession, request model.WorkCheckpointConfirmation) (model.WorkCheckpointAcknowledgement, error) {
		return supervisorAcknowledgement(t, request), nil
	}
	supervisor.Checkpoints = client

	result := runSupervisorAsync(supervisor)
	clock.waitForTimer(t, 5*time.Second).fire()
	clock.waitForTimer(t, time.Second).fire()
	outcome := receiveSupervisorOutcome(t, result)
	if outcome.err != nil {
		t.Fatalf("Run() setup error = %v", outcome.err)
	}
	if outcome.outcome.Kind != ExecutionSupervisorAbandonWithoutTerminalReport ||
		!strings.Contains(outcome.outcome.Err.Error(), "termination failed") {
		t.Fatalf("outcome = %+v", outcome.outcome)
	}
}

func TestExecutionSupervisorAmbiguousSuspendReportAbandonsQuiescedExecution(t *testing.T) {
	execution := newSupervisorTestExecution()
	supervisor, clock := newTestExecutionSupervisor(t, CheckpointModeYield, execution)
	registration := supervisor.Registration
	execution.suspend = func(_ context.Context, request CheckpointCaptureRequest) (PreparedCheckpoint, error) {
		return preparedSupervisorCheckpoint(t, request, registration), nil
	}
	called := make(chan struct{}, 1)
	client := &supervisorTestCheckpointClient{}
	client.confirm = func(context.Context, WorkerSession, model.WorkCheckpointConfirmation) (model.WorkCheckpointAcknowledgement, error) {
		called <- struct{}{}
		return model.WorkCheckpointAcknowledgement{}, errors.New("ambiguous transport failure")
	}
	supervisor.Checkpoints = client

	result := runSupervisorAsync(supervisor)
	clock.waitForTimer(t, 5*time.Second).fire()
	select {
	case <-called:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for suspending checkpoint report")
	}
	clock.waitForTimer(t, 2*time.Second).fire()
	outcome := receiveSupervisorOutcome(t, result)
	if outcome.err != nil {
		t.Fatalf("Run() setup error = %v", outcome.err)
	}
	if outcome.outcome.Kind != ExecutionSupervisorAbandonWithoutTerminalReport ||
		!strings.Contains(outcome.outcome.Err.Error(), "remained ambiguous") ||
		execution.terminationCount() != 1 {
		t.Fatalf("outcome/termination = %+v/%d", outcome.outcome, execution.terminationCount())
	}
}

func TestExecutionSupervisorCancellationTerminatesWithoutTerminalReport(t *testing.T) {
	execution := newSupervisorTestExecution()
	supervisor, _ := newTestExecutionSupervisor(t, CheckpointModeShutdown, execution)
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan supervisorRunResult, 1)
	go func() {
		outcome, err := supervisor.Run(ctx)
		result <- supervisorRunResult{outcome: outcome, err: err}
	}()
	cancel()

	outcome := receiveSupervisorOutcome(t, result)
	if outcome.err != nil {
		t.Fatalf("Run() setup error = %v", outcome.err)
	}
	if outcome.outcome.Kind != ExecutionSupervisorAbandonWithoutTerminalReport || execution.terminationCount() != 1 {
		t.Fatalf("outcome/termination = %+v/%d", outcome.outcome, execution.terminationCount())
	}
}

type supervisorRunResult struct {
	outcome ExecutionSupervisorOutcome
	err     error
}

func runSupervisorAsync(supervisor ExecutionSupervisor) <-chan supervisorRunResult {
	result := make(chan supervisorRunResult, 1)
	go func() {
		outcome, err := supervisor.Run(context.Background())
		result <- supervisorRunResult{outcome: outcome, err: err}
	}()
	return result
}

func receiveSupervisorOutcome(t *testing.T, result <-chan supervisorRunResult) supervisorRunResult {
	t.Helper()
	select {
	case outcome := <-result:
		return outcome
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for execution supervisor")
		return supervisorRunResult{}
	}
}

func receiveSupervisorConfirmation(t *testing.T, requests <-chan model.WorkCheckpointConfirmation) model.WorkCheckpointConfirmation {
	t.Helper()
	select {
	case request := <-requests:
		return request
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for checkpoint confirmation")
		return model.WorkCheckpointConfirmation{}
	}
}

func receiveSupervisorFallback(t *testing.T, requests <-chan model.WorkCheckpointSuspendLatest) model.WorkCheckpointSuspendLatest {
	t.Helper()
	select {
	case request := <-requests:
		return request
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for checkpoint fallback")
		return model.WorkCheckpointSuspendLatest{}
	}
}

func waitForSupervisorCallCount(t *testing.T, count func() int, want int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if count() >= want {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("call count = %d, want at least %d", count(), want)
}

func newTestExecutionSupervisor(
	t *testing.T,
	mode CheckpointMode,
	execution *supervisorTestExecution,
) (ExecutionSupervisor, *supervisorTestClock) {
	t.Helper()
	clock := newSupervisorTestClock()
	adapter := &supervisorTestAdapter{execution: execution}
	registration := validPauseAdapterRegistration(adapter)
	config := Config{
		CheckpointMode:                   mode,
		DrainPauseDelaySeconds:           10,
		CheckpointCaptureTimeoutSeconds:  2,
		CheckpointReportTimeoutSeconds:   2,
		ExecutionTerminationGraceSeconds: 1,
	}
	switch mode {
	case CheckpointModePeriodic:
		config.CheckpointIntervalSeconds = 5
	case CheckpointModeYield:
		config.WorkItemExecutionQuantumSeconds = 5
	}
	ids := []string{"lineage-001", "artifact-001", "artifact-002", "artifact-003"}
	var idMu sync.Mutex
	return ExecutionSupervisor{
		Config:       config,
		Item:         model.WorkItem{ID: "work-001", AttemptID: "attempt-001", Type: model.WorkItemTypeWriteDemoOutput, OutputFilename: "output.json"},
		Registration: registration,
		Session:      WorkerSession{WorkerID: "worker-001", WorkerSessionID: "session-001"},
		Checkpoints:  &supervisorTestCheckpointClient{},
		Clock:        clock,
		NewID: func(string) (string, error) {
			idMu.Lock()
			defer idMu.Unlock()
			if len(ids) == 0 {
				return "", errors.New("test id source exhausted")
			}
			id := ids[0]
			ids = ids[1:]
			return id, nil
		},
	}, clock
}

type supervisorTestAdapter struct {
	execution SupervisedExecution
}

func (adapter *supervisorTestAdapter) StartFresh(context.Context, model.WorkItem) (SupervisedExecution, error) {
	return adapter.execution, nil
}

func (adapter *supervisorTestAdapter) StartResume(context.Context, model.WorkItem, model.WorkItemResumeAssignment) (SupervisedExecution, error) {
	return adapter.execution, nil
}

type supervisorTestExecution struct {
	result     chan ExecutionResult
	periodic   func(context.Context, CheckpointCaptureRequest) (PreparedCheckpoint, error)
	suspend    func(context.Context, CheckpointCaptureRequest) (PreparedCheckpoint, error)
	terminate  func(context.Context) error
	mu         sync.Mutex
	periodics  int
	suspends   int
	terminates int
}

func newSupervisorTestExecution() *supervisorTestExecution {
	return &supervisorTestExecution{result: make(chan ExecutionResult, 1)}
}

func (execution *supervisorTestExecution) Result() <-chan ExecutionResult {
	return execution.result
}

func (execution *supervisorTestExecution) CapturePeriodic(ctx context.Context, request CheckpointCaptureRequest) (PreparedCheckpoint, error) {
	execution.mu.Lock()
	execution.periodics++
	callback := execution.periodic
	execution.mu.Unlock()
	if callback == nil {
		return PreparedCheckpoint{}, errors.New("unexpected periodic capture")
	}
	return callback(ctx, request)
}

func (execution *supervisorTestExecution) CaptureForSuspend(ctx context.Context, request CheckpointCaptureRequest) (PreparedCheckpoint, error) {
	execution.mu.Lock()
	execution.suspends++
	callback := execution.suspend
	execution.mu.Unlock()
	if callback == nil {
		return PreparedCheckpoint{}, errors.New("unexpected suspending capture")
	}
	return callback(ctx, request)
}

func (execution *supervisorTestExecution) Continue(context.Context) error {
	return nil
}

func (execution *supervisorTestExecution) Terminate(ctx context.Context) error {
	execution.mu.Lock()
	execution.terminates++
	callback := execution.terminate
	execution.mu.Unlock()
	if callback == nil {
		return nil
	}
	return callback(ctx)
}

func (execution *supervisorTestExecution) periodicCount() int {
	execution.mu.Lock()
	defer execution.mu.Unlock()
	return execution.periodics
}

func (execution *supervisorTestExecution) suspendCount() int {
	execution.mu.Lock()
	defer execution.mu.Unlock()
	return execution.suspends
}

func (execution *supervisorTestExecution) terminationCount() int {
	execution.mu.Lock()
	defer execution.mu.Unlock()
	return execution.terminates
}

type supervisorTestCheckpointClient struct {
	confirm       func(context.Context, WorkerSession, model.WorkCheckpointConfirmation) (model.WorkCheckpointAcknowledgement, error)
	suspendLatest func(context.Context, WorkerSession, model.WorkCheckpointSuspendLatest) (model.WorkCheckpointAcknowledgement, error)
}

func (client *supervisorTestCheckpointClient) ConfirmCheckpoint(
	ctx context.Context,
	session WorkerSession,
	request model.WorkCheckpointConfirmation,
) (model.WorkCheckpointAcknowledgement, error) {
	if client.confirm == nil {
		return model.WorkCheckpointAcknowledgement{}, errors.New("unexpected checkpoint confirmation")
	}
	return client.confirm(ctx, session, request)
}

func (client *supervisorTestCheckpointClient) SuspendLatestCheckpoint(
	ctx context.Context,
	session WorkerSession,
	request model.WorkCheckpointSuspendLatest,
) (model.WorkCheckpointAcknowledgement, error) {
	if client.suspendLatest == nil {
		return model.WorkCheckpointAcknowledgement{}, errors.New("unexpected suspend-latest request")
	}
	return client.suspendLatest(ctx, session, request)
}

type supervisorTestClock struct {
	mu      sync.Mutex
	now     time.Time
	created chan *supervisorTestTimer
}

func newSupervisorTestClock() *supervisorTestClock {
	return &supervisorTestClock{
		now:     time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC),
		created: make(chan *supervisorTestTimer, 32),
	}
}

func (clock *supervisorTestClock) Now() time.Time {
	clock.mu.Lock()
	defer clock.mu.Unlock()
	return clock.now
}

func (clock *supervisorTestClock) NewTicker(time.Duration) WorkerLifecycleTicker {
	panic("execution supervisor must not create tickers")
}

func (clock *supervisorTestClock) NewTimer(duration time.Duration) WorkerLifecycleTimer {
	timer := &supervisorTestTimer{
		clock:    clock,
		channel:  make(chan time.Time, 1),
		duration: duration,
		active:   true,
	}
	clock.created <- timer
	return timer
}

func (clock *supervisorTestClock) waitForTimer(t *testing.T, duration time.Duration) *supervisorTestTimer {
	t.Helper()
	deadline := time.After(2 * time.Second)
	for {
		select {
		case timer := <-clock.created:
			if timer.currentDuration() == duration && timer.isActive() {
				return timer
			}
		case <-deadline:
			t.Fatalf("timed out waiting for %s timer", duration)
			return nil
		}
	}
}

type supervisorTestTimer struct {
	clock    *supervisorTestClock
	channel  chan time.Time
	mu       sync.Mutex
	duration time.Duration
	active   bool
}

func (timer *supervisorTestTimer) C() <-chan time.Time {
	return timer.channel
}

func (timer *supervisorTestTimer) Stop() bool {
	timer.mu.Lock()
	defer timer.mu.Unlock()
	wasActive := timer.active
	timer.active = false
	return wasActive
}

func (timer *supervisorTestTimer) Reset(duration time.Duration) bool {
	timer.mu.Lock()
	wasActive := timer.active
	timer.duration = duration
	timer.active = true
	timer.mu.Unlock()
	timer.clock.created <- timer
	return wasActive
}

func (timer *supervisorTestTimer) fire() {
	timer.mu.Lock()
	if !timer.active {
		timer.mu.Unlock()
		return
	}
	timer.active = false
	duration := timer.duration
	timer.mu.Unlock()

	timer.clock.mu.Lock()
	timer.clock.now = timer.clock.now.Add(duration)
	now := timer.clock.now
	timer.clock.mu.Unlock()
	timer.channel <- now
}

func (timer *supervisorTestTimer) currentDuration() time.Duration {
	timer.mu.Lock()
	defer timer.mu.Unlock()
	return timer.duration
}

func (timer *supervisorTestTimer) isActive() bool {
	timer.mu.Lock()
	defer timer.mu.Unlock()
	return timer.active
}

func preparedSupervisorCheckpoint(
	t *testing.T,
	request CheckpointCaptureRequest,
	registration PauseAdapterRegistration,
) PreparedCheckpoint {
	t.Helper()
	statePath := "state/checkpoint.json"
	manifest := model.ResumeArtifactManifest{
		Schema:              model.ResumeArtifactSchemaV1,
		ResumeArtifactID:    request.ResumeArtifactID,
		ResumeGeneration:    request.ResumeGeneration,
		PauseStrategy:       registration.Strategy,
		WorkItemID:          request.WorkItemID,
		WorkItemType:        request.WorkItemType,
		ProducingAttemptID:  request.AttemptID,
		ExecutionLineageID:  request.ExecutionLineageID,
		InputFingerprint:    "input-v1",
		SourceVersion:       "source-v1",
		CodeVersion:         "code-v1",
		CreatedAt:           "2026-08-04T12:00:00Z",
		StorageScope:        model.ResumeArtifactStorageScopeSharedTmp,
		StorageRelativePath: request.ArtifactStorageRelativePath,
		RetentionPolicy:     model.ResumeArtifactRetentionWhileReferenced,
		Compatibility: model.ResumeArtifactCompatibility{
			AdapterID:                      registration.AdapterID,
			AdapterVersion:                 registration.AdapterVersion,
			WorkerExecutionContractVersion: "1",
			WorkerVersion:                  "test",
			ContainerImageIdentity:         "sha256:test",
			OperatingSystem:                "linux",
			Architecture:                   "amd64",
			ContainerRuntime:               "test",
		},
		Files: []model.ResumeArtifactFile{{
			Path:      statePath,
			SizeBytes: 1,
			SHA256:    strings.Repeat("a", 64),
		}},
		Manual: &model.ManualResumePayload{
			HandlerID:      "test",
			HandlerVersion: "1",
			StateSchema:    "test/v1",
			StateFilePath:  statePath,
		},
	}
	manifestJSON, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	return PreparedCheckpoint{
		ManifestJSON: string(manifestJSON),
		Reference: model.ResumeArtifactReference{
			Schema:               model.ResumeArtifactSchemaV1,
			ResumeArtifactID:     request.ResumeArtifactID,
			StorageScope:         model.ResumeArtifactStorageScopeSharedTmp,
			ManifestRelativePath: request.ArtifactStorageRelativePath + "/manifest.json",
			ManifestSHA256:       fmt.Sprintf("%x", sha256.Sum256(manifestJSON)),
		},
	}
}

func supervisorAcknowledgement(
	t *testing.T,
	request model.WorkCheckpointConfirmation,
) model.WorkCheckpointAcknowledgement {
	t.Helper()
	manifest := decodeSupervisorManifest(t, request.ManifestJSON)
	return model.WorkCheckpointAcknowledgement{
		Operation:          model.CheckpointOperationConfirmation,
		ResumeArtifactID:   manifest.ResumeArtifactID,
		ExecutionLineageID: manifest.ExecutionLineageID,
		ResumeGeneration:   manifest.ResumeGeneration,
		Reference:          request.Reference,
		CaptureKind:        request.CaptureKind,
		AcceptedAt:         "2026-08-04T12:00:01Z",
		Disposition:        request.Disposition,
		Suspended:          request.Disposition == model.CheckpointDispositionSuspend,
		SuspendedAt:        request.SuspendedAt,
	}
}

func decodeSupervisorManifest(t *testing.T, value string) model.ResumeArtifactManifest {
	t.Helper()
	var manifest model.ResumeArtifactManifest
	if err := json.Unmarshal([]byte(value), &manifest); err != nil {
		t.Fatal(err)
	}
	return manifest
}
