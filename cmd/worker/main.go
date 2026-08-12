package main

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"time"

	"goetl/internal/model"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "execute" {
		os.Exit(runDirectCommand(os.Args[2:], os.Stdout, os.Stderr))
	}

	cfg, err := loadConfig(workerConfigPath(os.Args))
	if err != nil {
		fmt.Println("invalid config:", err)
		return
	}

	controller, err := NewWorkerControllerClient(cfg)
	if err != nil {
		fmt.Println("invalid controller client:", err)
		return
	}

	worker := Worker{
		Config:        cfg,
		Controller:    controller,
		SourceBundles: ControllerSourceBundleProvider{Controller: controller},
	}

	if err := worker.Validate(); err != nil {
		fmt.Println("invalid worker:", err)
		return
	}

	if err := runWorkerLoop(worker); err != nil {
		fmt.Println("worker failed:", err)
		return
	}

}

func workerConfigPath(args []string) string {
	if len(args) > 1 {
		return args[1]
	}

	return "demo-config.json"
}

func runWorkerLoop(worker Worker) error {
	if err := worker.Validate(); err != nil {
		return fmt.Errorf("validate worker: %w", err)
	}
	drain, stopDrain := newPlatformWorkerDrainSource(worker.LifecycleClock)
	defer stopDrain()
	return runWorkerLoopWithDrain(worker, drain)
}

type claimedWorkResult struct {
	outcome ExecutionSupervisorOutcome
	err     error
}

type currentWorkerDrainSource interface {
	Current() (WorkerDrainRequest, bool)
}

func runWorkerLoopWithDrain(worker Worker, drain WorkerDrainSource) error {
	controller, err := worker.controllerClient()
	if err != nil {
		return fmt.Errorf("controller client: %w", err)
	}
	lifecycleClock := worker.LifecycleClock
	if lifecycleClock == nil {
		lifecycleClock = realWorkerLifecycleClock{}
	}
	session, err := controller.RegisterWorker(context.Background(), workerRegistrationRequest())
	if err != nil {
		return fmt.Errorf("register worker: %w", err)
	}

	heartbeatCtx, cancelHeartbeat := context.WithCancel(context.Background())
	heartbeatDone := make(chan error, 1)
	go func() {
		heartbeatDone <- RunHeartbeat(heartbeatCtx, session, controller.HeartbeatWorker, lifecycleClock)
	}()
	heartbeatStopped := false
	stopHeartbeat := func() error {
		if heartbeatStopped {
			return nil
		}
		cancelHeartbeat()
		err := <-heartbeatDone
		heartbeatStopped = true
		if err != nil {
			return fmt.Errorf("worker heartbeat: %w", err)
		}
		return nil
	}
	defer func() {
		_ = stopHeartbeat()
	}()
	heartbeatStatus := func() error {
		if heartbeatStopped {
			return nil
		}
		select {
		case err := <-heartbeatDone:
			heartbeatStopped = true
			if err != nil {
				return fmt.Errorf("worker heartbeat: %w", err)
			}
			return fmt.Errorf("worker heartbeat stopped unexpectedly")
		default:
			return nil
		}
	}
	heartbeatResult := func(err error) error {
		if err != nil {
			return fmt.Errorf("worker heartbeat: %w", err)
		}
		return fmt.Errorf("worker heartbeat stopped unexpectedly")
	}
	confirmTerminalOwnership := func() error {
		if err := heartbeatStatus(); err != nil {
			return err
		}
		if err := controller.HeartbeatWorker(context.Background(), session); err != nil {
			return fmt.Errorf("worker heartbeat: %w", err)
		}
		return heartbeatStatus()
	}
	stopWorker := func(reason string) error {
		if err := stopHeartbeat(); err != nil {
			return err
		}
		if err := controller.StopWorker(context.Background(), session, reason); err != nil {
			fmt.Println("worker stop failed:", err)
		}
		return nil
	}
	drainChannel := (<-chan WorkerDrainRequest)(nil)
	if drain != nil {
		drainChannel = drain.C()
	}
	drainRequested := func() bool {
		state, ok := drain.(currentWorkerDrainSource)
		if !ok {
			return false
		}
		_, requested := state.Current()
		return requested
	}
	takeDrain := func() bool {
		if drainRequested() {
			return true
		}
		select {
		case _, open := <-drainChannel:
			if !open {
				drainChannel = nil
				return false
			}
			return true
		default:
			return false
		}
	}
	waitForIdlePoll := func(duration time.Duration) (bool, error) {
		ticker := lifecycleClock.NewTicker(duration)
		defer ticker.Stop()
		if drainRequested() {
			return true, nil
		}
		select {
		case <-ticker.C():
			return false, heartbeatStatus()
		case _, open := <-drainChannel:
			if !open {
				drainChannel = nil
				return false, heartbeatStatus()
			}
			return true, nil
		case err := <-heartbeatDone:
			heartbeatStopped = true
			return false, heartbeatResult(err)
		}
	}

	var idleStarted time.Time
	for {
		if takeDrain() {
			if err := stopWorker("slurm_drain_idle"); err != nil {
				return err
			}
			return nil
		}
		if err := heartbeatStatus(); err != nil {
			return err
		}
		item, hasWork, err := controller.FetchWorkItem(session)
		if err != nil {
			return fmt.Errorf("fetch work item: %w", err)
		}
		if err := heartbeatStatus(); err != nil {
			return err
		}
		claimedUnderDrain := takeDrain()
		if claimedUnderDrain && !hasWork {
			if err := stopWorker("slurm_drain_idle"); err != nil {
				return err
			}
			return nil
		}

		if !hasWork {
			idleTimeout := worker.Config.effectiveIdleTimeout()
			if idleTimeout <= 0 {
				fmt.Println("no work available")
				if err := stopWorker("no_work"); err != nil {
					return err
				}
				return nil
			}
			now := lifecycleClock.Now()
			if idleStarted.IsZero() {
				idleStarted = now
				fmt.Printf("no work available; polling for up to %s\n", idleTimeout)
			}
			elapsed := now.Sub(idleStarted)
			if elapsed >= idleTimeout {
				fmt.Println("no work available; idle timeout reached")
				if err := stopWorker("no_work"); err != nil {
					return err
				}
				return nil
			}
			wait := worker.Config.effectiveIdlePollInterval()
			if remaining := idleTimeout - elapsed; remaining < wait {
				wait = remaining
			}
			if wait > 0 {
				drained, err := waitForIdlePoll(wait)
				if err != nil {
					return err
				}
				if drained {
					if err := stopWorker("slurm_drain_idle"); err != nil {
						return err
					}
					return nil
				}
			}
			continue
		}
		idleStarted = time.Time{}

		startedAt := lifecycleClock.Now().UTC()
		executionCtx, cancelExecution := context.WithCancel(context.Background())
		executionDone := make(chan claimedWorkResult, 1)
		supervised := worker.Config.CheckpointMode != CheckpointModeDisabled || item.Resume != nil
		go func() {
			if supervised {
				outcome, runErr := worker.RunSupervised(executionCtx, item, session, controller, drain)
				executionDone <- claimedWorkResult{outcome: outcome, err: runErr}
				return
			}
			evidence, runErr := worker.Run(item)
			outcome := ExecutionSupervisorOutcome{Kind: ExecutionSupervisorCompleted, Evidence: evidence}
			if runErr != nil {
				outcome = ExecutionSupervisorOutcome{Kind: ExecutionSupervisorFailed, Err: runErr}
			}
			executionDone <- claimedWorkResult{outcome: outcome}
		}()

		draining := claimedUnderDrain || drainRequested()
		var execution claimedWorkResult
		for {
			if supervised {
				select {
				case execution = <-executionDone:
					cancelExecution()
					goto executionFinished
				case heartbeatErr := <-heartbeatDone:
					heartbeatStopped = true
					cancelExecution()
					<-executionDone
					return heartbeatResult(heartbeatErr)
				}
			}

			select {
			case execution = <-executionDone:
				cancelExecution()
				goto executionFinished
			case _, open := <-drainChannel:
				if !open {
					drainChannel = nil
					continue
				}
				draining = true
			case heartbeatErr := <-heartbeatDone:
				heartbeatStopped = true
				cancelExecution()
				return heartbeatResult(heartbeatErr)
			}
		}

	executionFinished:
		if drainRequested() {
			draining = true
		}
		if heartbeatErr := heartbeatStatus(); heartbeatErr != nil {
			return heartbeatErr
		}
		if execution.err != nil {
			reason := "checkpoint_launch_rejected"
			if item.Resume != nil {
				reason = "resume_launch_rejected"
			}
			if stopErr := stopWorker(reason); stopErr != nil {
				return fmt.Errorf("supervised work item setup: %v; stop worker: %w", execution.err, stopErr)
			}
			return fmt.Errorf("supervised work item setup: %w", execution.err)
		}

		switch execution.outcome.Kind {
		case ExecutionSupervisorCompleted:
			if err := confirmTerminalOwnership(); err != nil {
				return err
			}
			if err := controller.ReportWorkComplete(item, startedAt, execution.outcome.Evidence, session); err != nil {
				return fmt.Errorf("report completion: %w", err)
			}
			if draining {
				if err := stopWorker("slurm_drain_finished"); err != nil {
					return err
				}
				return nil
			}

		case ExecutionSupervisorFailed:
			workErr := execution.outcome.Err
			if workErr == nil {
				workErr = fmt.Errorf("supervised execution failed without an error")
			}
			if err := confirmTerminalOwnership(); err != nil {
				return err
			}
			if reportErr := controller.ReportWorkFailed(item, workErr, session); reportErr != nil {
				return fmt.Errorf("run work item: %v; report failure: %w", workErr, reportErr)
			}
			reason := "worker_error"
			if draining {
				reason = "slurm_drain_finished"
			}
			if stopErr := stopWorker(reason); stopErr != nil {
				return fmt.Errorf("run work item: %v; stop worker: %w", workErr, stopErr)
			}
			return workErr

		case ExecutionSupervisorSuspended:
			if err := stopWorker("checkpoint_suspended"); err != nil {
				return err
			}
			return nil

		case ExecutionSupervisorAbandonWithoutTerminalReport:
			if err := stopWorker("checkpoint_abandoned"); err != nil {
				return err
			}
			if execution.outcome.Err != nil {
				return execution.outcome.Err
			}
			return fmt.Errorf("supervised execution abandoned without a terminal report")

		default:
			if err := stopWorker("checkpoint_abandoned"); err != nil {
				return err
			}
			return fmt.Errorf("unsupported supervised execution outcome %q", execution.outcome.Kind)
		}
	}
}

func workerRegistrationRequest() model.WorkerRegistrationRequest {
	return model.WorkerRegistrationRequest{
		ExecutionHandle:      fmt.Sprintf("pid-%d", os.Getpid()),
		ExecutionEnvironment: runtime.GOOS + "/" + runtime.GOARCH,
	}
}
