package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path"
	"reflect"
	"time"

	"goetl/internal/controllerhttp"
	"goetl/internal/model"
)

type ExecutionSupervisorOutcomeKind string

const (
	ExecutionSupervisorCompleted                    ExecutionSupervisorOutcomeKind = "completed"
	ExecutionSupervisorFailed                       ExecutionSupervisorOutcomeKind = "failed"
	ExecutionSupervisorSuspended                    ExecutionSupervisorOutcomeKind = "suspended"
	ExecutionSupervisorAbandonWithoutTerminalReport ExecutionSupervisorOutcomeKind = "abandon_without_terminal_report"
)

type ExecutionSupervisorOutcome struct {
	Kind            ExecutionSupervisorOutcomeKind
	Evidence        WorkEvidence
	Err             error
	Acknowledgement model.WorkCheckpointAcknowledgement
}

type ExecutionSupervisorCheckpointClient interface {
	ConfirmCheckpoint(
		context.Context,
		WorkerSession,
		model.WorkCheckpointConfirmation,
	) (model.WorkCheckpointAcknowledgement, error)
	SuspendLatestCheckpoint(
		context.Context,
		WorkerSession,
		model.WorkCheckpointSuspendLatest,
	) (model.WorkCheckpointAcknowledgement, error)
}

type ExecutionSupervisorIDFunc func(kind string) (string, error)

type ExecutionSupervisor struct {
	Config       Config
	Item         model.WorkItem
	Registration PauseAdapterRegistration
	Session      WorkerSession
	Checkpoints  ExecutionSupervisorCheckpointClient
	Drain        WorkerDrainSource
	Clock        WorkerLifecycleClock
	NewID        ExecutionSupervisorIDFunc
}

type acceptedSupervisorCheckpoint struct {
	generation int
	lineageID  string
	reference  model.ResumeArtifactReference
}

type pendingSupervisorConfirmation struct {
	request model.WorkCheckpointConfirmation
}

type supervisorReportResult struct {
	acknowledgement model.WorkCheckpointAcknowledgement
	err             error
	definitive      bool
	abandon         bool
}

type supervisorCaptureResult struct {
	checkpoint PreparedCheckpoint
	terminal   *ExecutionResult
	err        error
}

func (supervisor ExecutionSupervisor) Run(ctx context.Context) (ExecutionSupervisorOutcome, error) {
	if ctx == nil {
		return ExecutionSupervisorOutcome{}, fmt.Errorf("execution supervisor context is required")
	}
	if supervisor.Config.CheckpointMode == CheckpointModeDisabled {
		return ExecutionSupervisorOutcome{}, fmt.Errorf("execution supervisor requires an enabled checkpoint mode")
	}
	if err := supervisor.Config.validateCheckpointPolicy(); err != nil {
		return ExecutionSupervisorOutcome{}, fmt.Errorf("execution supervisor checkpoint policy: %w", err)
	}
	if err := supervisor.Item.Validate(); err != nil {
		return ExecutionSupervisorOutcome{}, fmt.Errorf("execution supervisor work item: %w", err)
	}
	if supervisor.Item.AttemptID == "" {
		return ExecutionSupervisorOutcome{}, fmt.Errorf("execution supervisor work item attempt id is required")
	}
	if err := supervisor.Registration.Validate(); err != nil {
		return ExecutionSupervisorOutcome{}, fmt.Errorf("execution supervisor adapter: %w", err)
	}
	if supervisor.Registration.WorkItemType != supervisor.Item.Type {
		return ExecutionSupervisorOutcome{}, fmt.Errorf("execution supervisor adapter does not match work item type")
	}
	if !supervisor.Registration.Capabilities.Supports(supervisor.Config.CheckpointMode) {
		return ExecutionSupervisorOutcome{}, fmt.Errorf("execution supervisor adapter does not support checkpoint mode %q", supervisor.Config.CheckpointMode)
	}
	if err := supervisor.Session.ValidateIdentity(); err != nil {
		return ExecutionSupervisorOutcome{}, fmt.Errorf("execution supervisor session: %w", err)
	}
	if executionSupervisorInterfaceIsNil(supervisor.Checkpoints) {
		return ExecutionSupervisorOutcome{}, fmt.Errorf("execution supervisor checkpoint client is required")
	}
	if supervisor.Clock == nil {
		supervisor.Clock = realWorkerLifecycleClock{}
	}
	if supervisor.NewID == nil {
		supervisor.NewID = newExecutionSupervisorID
	}

	lineageID, nextGeneration, latest, err := supervisor.initialCheckpointState()
	if err != nil {
		return ExecutionSupervisorOutcome{}, err
	}

	execution, err := supervisor.startExecution(ctx)
	if err != nil {
		return ExecutionSupervisorOutcome{}, fmt.Errorf("start supervised execution: %w", err)
	}
	if executionSupervisorInterfaceIsNil(execution) || execution.Result() == nil {
		return ExecutionSupervisorOutcome{}, fmt.Errorf("pause adapter returned an invalid supervised execution")
	}

	return supervisor.runExecution(ctx, execution, lineageID, nextGeneration, latest), nil
}

func (supervisor ExecutionSupervisor) startExecution(ctx context.Context) (SupervisedExecution, error) {
	if supervisor.Item.Resume == nil {
		return supervisor.Registration.Adapter.StartFresh(ctx, supervisor.Item)
	}
	return supervisor.Registration.Adapter.StartResume(ctx, supervisor.Item, *supervisor.Item.Resume)
}

func (supervisor ExecutionSupervisor) initialCheckpointState() (
	string,
	int,
	*acceptedSupervisorCheckpoint,
	error,
) {
	if supervisor.Item.Resume == nil {
		lineageID, err := supervisor.NewID("lineage")
		if err != nil {
			return "", 0, nil, fmt.Errorf("create execution lineage id: %w", err)
		}
		if err := validateAdapterContractValue("execution lineage id", lineageID); err != nil {
			return "", 0, nil, fmt.Errorf("create execution lineage id: %w", err)
		}
		return lineageID, 1, nil, nil
	}

	var manifest model.ResumeArtifactManifest
	if err := json.Unmarshal([]byte(supervisor.Item.Resume.ManifestJSON), &manifest); err != nil {
		return "", 0, nil, fmt.Errorf("decode consumed resume artifact: %w", err)
	}
	latest := &acceptedSupervisorCheckpoint{
		generation: manifest.ResumeGeneration,
		lineageID:  supervisor.Item.Resume.ExecutionLineageID,
		reference:  supervisor.Item.Resume.Reference,
	}
	return latest.lineageID, latest.generation + 1, latest, nil
}

func (supervisor ExecutionSupervisor) runExecution(
	ctx context.Context,
	execution SupervisedExecution,
	lineageID string,
	nextGeneration int,
	latest *acceptedSupervisorCheckpoint,
) ExecutionSupervisorOutcome {
	activeDuration := supervisor.activeExecutionDuration()
	var activeTimer WorkerLifecycleTimer
	if activeDuration > 0 {
		activeTimer = supervisor.Clock.NewTimer(activeDuration)
		defer activeTimer.Stop()
	}

	var drainTimer WorkerLifecycleTimer
	defer func() {
		if drainTimer != nil {
			drainTimer.Stop()
		}
	}()
	drainChannel := supervisor.drainChannel()
	drainRequested := false
	var pending *pendingSupervisorConfirmation

	for {
		var activeTimerChannel <-chan time.Time
		if activeTimer != nil {
			activeTimerChannel = activeTimer.C()
		}
		var drainTimerChannel <-chan time.Time
		if drainTimer != nil {
			drainTimerChannel = drainTimer.C()
		}

		select {
		case result, open := <-execution.Result():
			if !open {
				return failedSupervisorOutcome(fmt.Errorf("supervised execution result channel closed without a result"))
			}
			return supervisorOutcomeFromExecution(result)

		case <-ctx.Done():
			return supervisor.abandonAfterTermination(execution, fmt.Errorf("supervised execution canceled: %w", ctx.Err()))

		case request, open := <-drainChannel:
			if !open {
				drainChannel = nil
				continue
			}
			if drainRequested {
				continue
			}
			if err := request.Validate(); err != nil {
				continue
			}
			drainRequested = true
			delay := time.Duration(supervisor.Config.DrainPauseDelaySeconds) * time.Second
			elapsed := supervisor.Clock.Now().Sub(request.RequestedAt)
			if elapsed > 0 {
				delay -= elapsed
			}
			if delay < 0 {
				delay = 0
			}
			drainTimer = supervisor.Clock.NewTimer(delay)

		case <-activeTimerChannel:
			if pending != nil {
				report := supervisor.reportConfirmation(ctx, pending.request)
				if report.abandon {
					return supervisor.abandonAfterTermination(execution, fmt.Errorf("periodic checkpoint confirmation lost assignment ownership: %w", report.err))
				}
				if report.err == nil {
					latest = acceptedCheckpointFromAcknowledgement(report.acknowledgement)
					nextGeneration = latest.generation + 1
					pending = nil
				} else if report.definitive {
					pending = nil
				}
				activeTimer.Reset(activeDuration)
				continue
			}

			kind := model.CheckpointCaptureKindPeriodic
			if supervisor.Config.CheckpointMode == CheckpointModeYield {
				kind = model.CheckpointCaptureKindQuantum
			}
			if kind == model.CheckpointCaptureKindPeriodic {
				outcome, accepted, unresolved := supervisor.capturePeriodic(
					ctx,
					execution,
					lineageID,
					nextGeneration,
				)
				if outcome != nil {
					return *outcome
				}
				if accepted != nil {
					latest = accepted
					nextGeneration = accepted.generation + 1
				}
				pending = unresolved
				activeTimer.Reset(activeDuration)
				continue
			}

			return supervisor.captureAndSuspend(
				ctx,
				execution,
				lineageID,
				nextGeneration,
				model.CheckpointCaptureKindQuantum,
				latest,
			)

		case <-drainTimerChannel:
			if activeTimer != nil {
				activeTimer.Stop()
			}
			if pending != nil {
				report := supervisor.reportConfirmation(ctx, pending.request)
				if report.abandon {
					return supervisor.abandonAfterTermination(execution, fmt.Errorf("periodic checkpoint confirmation lost assignment ownership before final pause: %w", report.err))
				}
				if report.err == nil {
					latest = acceptedCheckpointFromAcknowledgement(report.acknowledgement)
					nextGeneration = latest.generation + 1
					pending = nil
				} else if report.definitive {
					pending = nil
				} else {
					return supervisor.abandonAfterTermination(execution, fmt.Errorf("periodic checkpoint confirmation remained ambiguous before final pause: %w", report.err))
				}
			}
			return supervisor.captureAndSuspend(
				ctx,
				execution,
				lineageID,
				nextGeneration,
				model.CheckpointCaptureKindFinal,
				latest,
			)
		}
	}
}

func (supervisor ExecutionSupervisor) capturePeriodic(
	ctx context.Context,
	execution SupervisedExecution,
	lineageID string,
	generation int,
) (*ExecutionSupervisorOutcome, *acceptedSupervisorCheckpoint, *pendingSupervisorConfirmation) {
	request, err := supervisor.newCaptureRequest(lineageID, generation, model.CheckpointCaptureKindPeriodic)
	if err != nil {
		return nil, nil, nil
	}
	capture := supervisor.capture(ctx, execution, request, false)
	if capture.terminal != nil {
		outcome := supervisorOutcomeFromExecution(*capture.terminal)
		return &outcome, nil, nil
	}
	if capture.err != nil {
		return nil, nil, nil
	}
	if _, err := capture.checkpoint.Validate(request, supervisor.Registration); err != nil {
		return nil, nil, nil
	}

	confirmation := model.WorkCheckpointConfirmation{
		AttemptID:    supervisor.Item.AttemptID,
		ManifestJSON: capture.checkpoint.ManifestJSON,
		Reference:    capture.checkpoint.Reference,
		CaptureKind:  model.CheckpointCaptureKindPeriodic,
		Disposition:  model.CheckpointDispositionContinue,
	}
	report := supervisor.reportConfirmation(ctx, confirmation)
	if report.err == nil {
		return nil, acceptedCheckpointFromAcknowledgement(report.acknowledgement), nil
	}
	if report.abandon {
		outcome := supervisor.abandonAfterTermination(execution, fmt.Errorf("periodic checkpoint confirmation lost assignment ownership: %w", report.err))
		return &outcome, nil, nil
	}
	if report.definitive {
		return nil, nil, nil
	}
	return nil, nil, &pendingSupervisorConfirmation{request: confirmation}
}

func (supervisor ExecutionSupervisor) captureAndSuspend(
	ctx context.Context,
	execution SupervisedExecution,
	lineageID string,
	generation int,
	kind model.CheckpointCaptureKind,
	latest *acceptedSupervisorCheckpoint,
) ExecutionSupervisorOutcome {
	request, err := supervisor.newCaptureRequest(lineageID, generation, kind)
	if err == nil {
		capture := supervisor.capture(ctx, execution, request, true)
		if capture.terminal != nil {
			return supervisorOutcomeFromExecution(*capture.terminal)
		}
		err = capture.err
		if err == nil {
			_, err = capture.checkpoint.Validate(request, supervisor.Registration)
		}
		if err == nil {
			suspendedAt := supervisor.Clock.Now().UTC().Format(time.RFC3339)
			confirmation := model.WorkCheckpointConfirmation{
				AttemptID:    supervisor.Item.AttemptID,
				ManifestJSON: capture.checkpoint.ManifestJSON,
				Reference:    capture.checkpoint.Reference,
				CaptureKind:  kind,
				Disposition:  model.CheckpointDispositionSuspend,
				SuspendedAt:  suspendedAt,
			}
			report := supervisor.reportConfirmation(ctx, confirmation)
			if report.err == nil {
				return supervisor.suspendedAfterTermination(execution, report.acknowledgement)
			}
			return supervisor.abandonAfterTermination(execution, fmt.Errorf("suspending checkpoint confirmation failed: %w", report.err))
		}
	}

	if latest == nil {
		return supervisor.abandonAfterTermination(execution, fmt.Errorf("suspending checkpoint failed without an accepted fallback: %w", err))
	}
	return supervisor.suspendFromLatest(ctx, execution, kind, latest, err)
}

func (supervisor ExecutionSupervisor) suspendFromLatest(
	ctx context.Context,
	execution SupervisedExecution,
	kind model.CheckpointCaptureKind,
	latest *acceptedSupervisorCheckpoint,
	captureErr error,
) ExecutionSupervisorOutcome {
	reason := model.CheckpointSuspendReasonShutdown
	if kind == model.CheckpointCaptureKindQuantum {
		reason = model.CheckpointSuspendReasonQuantum
	}
	request := model.WorkCheckpointSuspendLatest{
		AttemptID:     supervisor.Item.AttemptID,
		SuspendedAt:   supervisor.Clock.Now().UTC().Format(time.RFC3339),
		SuspendReason: reason,
	}
	report := supervisor.reportSuspendLatest(ctx, request, latest)
	if report.err != nil {
		return supervisor.abandonAfterTermination(execution, fmt.Errorf("suspend from latest checkpoint after capture failure %v: %w", captureErr, report.err))
	}
	return supervisor.suspendedAfterTermination(execution, report.acknowledgement)
}

func (supervisor ExecutionSupervisor) capture(
	ctx context.Context,
	execution SupervisedExecution,
	request CheckpointCaptureRequest,
	suspend bool,
) supervisorCaptureResult {
	captureCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	result := make(chan supervisorCaptureResult, 1)
	go func() {
		var checkpoint PreparedCheckpoint
		var err error
		if suspend {
			checkpoint, err = execution.CaptureForSuspend(captureCtx, request)
		} else {
			checkpoint, err = execution.CapturePeriodic(captureCtx, request)
		}
		result <- supervisorCaptureResult{checkpoint: checkpoint, err: err}
	}()

	timer := supervisor.Clock.NewTimer(time.Duration(supervisor.Config.CheckpointCaptureTimeoutSeconds) * time.Second)
	defer timer.Stop()
	select {
	case capture := <-result:
		return capture
	case terminal, open := <-execution.Result():
		if !open {
			terminal = ExecutionResult{Err: fmt.Errorf("supervised execution result channel closed without a result")}
		}
		return supervisorCaptureResult{terminal: &terminal}
	case <-timer.C():
		return supervisorCaptureResult{err: fmt.Errorf("checkpoint capture timed out")}
	case <-ctx.Done():
		return supervisorCaptureResult{err: ctx.Err()}
	}
}

func (supervisor ExecutionSupervisor) reportConfirmation(
	ctx context.Context,
	request model.WorkCheckpointConfirmation,
) supervisorReportResult {
	var expectedManifest model.ResumeArtifactManifest
	if err := json.Unmarshal([]byte(request.ManifestJSON), &expectedManifest); err != nil {
		return supervisorReportResult{err: fmt.Errorf("decode checkpoint confirmation manifest: %w", err), definitive: true}
	}
	return supervisor.retryCheckpointReport(ctx, func(callCtx context.Context) (model.WorkCheckpointAcknowledgement, error) {
		acknowledgement, err := supervisor.Checkpoints.ConfirmCheckpoint(callCtx, supervisor.Session, request)
		if err != nil {
			return model.WorkCheckpointAcknowledgement{}, err
		}
		if err := acknowledgement.Validate(); err != nil {
			return model.WorkCheckpointAcknowledgement{}, fmt.Errorf("validate checkpoint acknowledgement: %w", err)
		}
		if acknowledgement.Operation != model.CheckpointOperationConfirmation ||
			acknowledgement.ResumeArtifactID != request.Reference.ResumeArtifactID ||
			acknowledgement.ExecutionLineageID != expectedManifest.ExecutionLineageID ||
			acknowledgement.ResumeGeneration != expectedManifest.ResumeGeneration ||
			acknowledgement.Reference != request.Reference ||
			acknowledgement.CaptureKind != request.CaptureKind ||
			acknowledgement.Disposition != request.Disposition ||
			acknowledgement.SuspendedAt != request.SuspendedAt {
			return model.WorkCheckpointAcknowledgement{}, fmt.Errorf("checkpoint acknowledgement does not match confirmation request")
		}
		return acknowledgement, nil
	})
}

func (supervisor ExecutionSupervisor) reportSuspendLatest(
	ctx context.Context,
	request model.WorkCheckpointSuspendLatest,
	latest *acceptedSupervisorCheckpoint,
) supervisorReportResult {
	return supervisor.retryCheckpointReport(ctx, func(callCtx context.Context) (model.WorkCheckpointAcknowledgement, error) {
		acknowledgement, err := supervisor.Checkpoints.SuspendLatestCheckpoint(callCtx, supervisor.Session, request)
		if err != nil {
			return model.WorkCheckpointAcknowledgement{}, err
		}
		if err := acknowledgement.Validate(); err != nil {
			return model.WorkCheckpointAcknowledgement{}, fmt.Errorf("validate suspend-latest acknowledgement: %w", err)
		}
		if acknowledgement.Operation != model.CheckpointOperationSuspendLatest ||
			acknowledgement.SuspendedAt != request.SuspendedAt ||
			acknowledgement.ResumeArtifactID != latest.reference.ResumeArtifactID ||
			acknowledgement.Reference != latest.reference ||
			acknowledgement.ResumeGeneration != latest.generation ||
			acknowledgement.ExecutionLineageID != latest.lineageID {
			return model.WorkCheckpointAcknowledgement{}, fmt.Errorf("checkpoint acknowledgement does not match latest accepted checkpoint")
		}
		return acknowledgement, nil
	})
}

func (supervisor ExecutionSupervisor) retryCheckpointReport(
	ctx context.Context,
	call func(context.Context) (model.WorkCheckpointAcknowledgement, error),
) supervisorReportResult {
	timeout := time.Duration(supervisor.Config.CheckpointReportTimeoutSeconds) * time.Second
	deadline := supervisor.Clock.NewTimer(timeout)
	defer deadline.Stop()

	for {
		callCtx, cancel := context.WithCancel(ctx)
		result := make(chan supervisorReportResult, 1)
		go func() {
			acknowledgement, err := call(callCtx)
			result <- supervisorReportResult{acknowledgement: acknowledgement, err: err}
		}()

		select {
		case report := <-result:
			cancel()
			if report.err == nil {
				return report
			}
			if checkpointReportErrorIsDefinitive(report.err) {
				report.definitive = true
				report.abandon = checkpointReportErrorRequiresAbandon(report.err)
				return report
			}
		case <-deadline.C():
			cancel()
			return supervisorReportResult{err: fmt.Errorf("checkpoint report outcome remained ambiguous before timeout")}
		case <-ctx.Done():
			cancel()
			return supervisorReportResult{err: ctx.Err()}
		}

		retryDelay := time.Second
		if timeout < retryDelay {
			retryDelay = timeout
		}
		retry := supervisor.Clock.NewTimer(retryDelay)
		select {
		case <-retry.C():
			retry.Stop()
		case <-deadline.C():
			retry.Stop()
			return supervisorReportResult{err: fmt.Errorf("checkpoint report outcome remained ambiguous before timeout")}
		case <-ctx.Done():
			retry.Stop()
			return supervisorReportResult{err: ctx.Err()}
		}
	}
}

func (supervisor ExecutionSupervisor) suspendedAfterTermination(
	execution SupervisedExecution,
	acknowledgement model.WorkCheckpointAcknowledgement,
) ExecutionSupervisorOutcome {
	if err := supervisor.terminate(execution); err != nil {
		return ExecutionSupervisorOutcome{
			Kind: ExecutionSupervisorAbandonWithoutTerminalReport,
			Err:  fmt.Errorf("controller accepted suspension but execution termination failed: %w", err),
		}
	}
	return ExecutionSupervisorOutcome{
		Kind:            ExecutionSupervisorSuspended,
		Acknowledgement: acknowledgement,
	}
}

func (supervisor ExecutionSupervisor) abandonAfterTermination(
	execution SupervisedExecution,
	cause error,
) ExecutionSupervisorOutcome {
	if err := supervisor.terminate(execution); err != nil {
		cause = errors.Join(cause, fmt.Errorf("terminate supervised execution: %w", err))
	}
	return ExecutionSupervisorOutcome{
		Kind: ExecutionSupervisorAbandonWithoutTerminalReport,
		Err:  cause,
	}
}

func (supervisor ExecutionSupervisor) terminate(execution SupervisedExecution) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	go func() {
		result <- execution.Terminate(ctx)
	}()
	timer := supervisor.Clock.NewTimer(time.Duration(supervisor.Config.ExecutionTerminationGraceSeconds) * time.Second)
	defer timer.Stop()
	select {
	case err := <-result:
		return err
	case <-timer.C():
		return fmt.Errorf("execution termination timed out")
	}
}

func (supervisor ExecutionSupervisor) newCaptureRequest(
	lineageID string,
	generation int,
	kind model.CheckpointCaptureKind,
) (CheckpointCaptureRequest, error) {
	artifactID, err := supervisor.NewID("artifact")
	if err != nil {
		return CheckpointCaptureRequest{}, fmt.Errorf("create resume artifact id: %w", err)
	}
	request := CheckpointCaptureRequest{
		WorkItemID:                  supervisor.Item.ID,
		WorkItemType:                supervisor.Item.Type,
		AttemptID:                   supervisor.Item.AttemptID,
		ExecutionLineageID:          lineageID,
		ResumeGeneration:            generation,
		ResumeArtifactID:            artifactID,
		ArtifactStorageRelativePath: path.Join("goetl", "resume", artifactID),
		CaptureKind:                 kind,
	}
	if err := request.Validate(); err != nil {
		return CheckpointCaptureRequest{}, fmt.Errorf("create checkpoint capture request: %w", err)
	}
	return request, nil
}

func (supervisor ExecutionSupervisor) activeExecutionDuration() time.Duration {
	switch supervisor.Config.CheckpointMode {
	case CheckpointModePeriodic:
		return time.Duration(supervisor.Config.CheckpointIntervalSeconds) * time.Second
	case CheckpointModeYield:
		return time.Duration(supervisor.Config.WorkItemExecutionQuantumSeconds) * time.Second
	default:
		return 0
	}
}

func (supervisor ExecutionSupervisor) drainChannel() <-chan WorkerDrainRequest {
	if supervisor.Drain == nil {
		return nil
	}
	return supervisor.Drain.C()
}

func supervisorOutcomeFromExecution(result ExecutionResult) ExecutionSupervisorOutcome {
	if result.Err != nil {
		return failedSupervisorOutcome(result.Err)
	}
	return ExecutionSupervisorOutcome{
		Kind:     ExecutionSupervisorCompleted,
		Evidence: result.Evidence,
	}
}

func failedSupervisorOutcome(err error) ExecutionSupervisorOutcome {
	return ExecutionSupervisorOutcome{
		Kind: ExecutionSupervisorFailed,
		Err:  err,
	}
}

func acceptedCheckpointFromAcknowledgement(
	acknowledgement model.WorkCheckpointAcknowledgement,
) *acceptedSupervisorCheckpoint {
	return &acceptedSupervisorCheckpoint{
		generation: acknowledgement.ResumeGeneration,
		lineageID:  acknowledgement.ExecutionLineageID,
		reference:  acknowledgement.Reference,
	}
}

func checkpointReportErrorIsDefinitive(err error) bool {
	var statusError controllerhttp.StatusError
	if !errors.As(err, &statusError) {
		return false
	}
	return statusError.StatusCode >= http.StatusBadRequest &&
		statusError.StatusCode < http.StatusInternalServerError &&
		statusError.StatusCode != http.StatusRequestTimeout &&
		statusError.StatusCode != http.StatusTooManyRequests
}

func checkpointReportErrorRequiresAbandon(err error) bool {
	var statusError controllerhttp.StatusError
	return errors.As(err, &statusError) &&
		(statusError.StatusCode == http.StatusUnauthorized ||
			statusError.StatusCode == http.StatusForbidden ||
			statusError.StatusCode == http.StatusConflict)
}

func executionSupervisorInterfaceIsNil(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}

func newExecutionSupervisorID(kind string) (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return kind + "-" + hex.EncodeToString(bytes), nil
}

var _ ExecutionSupervisorCheckpointClient = WorkerControllerClient{}
