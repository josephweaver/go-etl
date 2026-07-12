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
	stopWorker := func(reason string) error {
		if err := stopHeartbeat(); err != nil {
			return err
		}
		if err := controller.StopWorker(context.Background(), session, reason); err != nil {
			fmt.Println("worker stop failed:", err)
		}
		return nil
	}
	waitForIdlePoll := func(duration time.Duration) error {
		ticker := lifecycleClock.NewTicker(duration)
		defer ticker.Stop()
		select {
		case <-ticker.C():
			return heartbeatStatus()
		case err := <-heartbeatDone:
			heartbeatStopped = true
			if err != nil {
				return fmt.Errorf("worker heartbeat: %w", err)
			}
			return fmt.Errorf("worker heartbeat stopped unexpectedly")
		}
	}

	var idleStarted time.Time
	for {
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
				if err := waitForIdlePoll(wait); err != nil {
					return err
				}
			}
			continue
		}
		idleStarted = time.Time{}

		startedAt := time.Now().UTC()
		evidence, err := worker.Run(item)
		if heartbeatErr := heartbeatStatus(); heartbeatErr != nil {
			return heartbeatErr
		}
		if err != nil {
			if reportErr := controller.ReportWorkFailed(item, err, session); reportErr != nil {
				return fmt.Errorf("run work item: %v; report failure: %w", err, reportErr)
			}
			if stopErr := stopWorker("worker_error"); stopErr != nil {
				return fmt.Errorf("run work item: %v; stop worker: %w", err, stopErr)
			}
			return err
		}

		if err := controller.ReportWorkComplete(item, startedAt, evidence, session); err != nil {
			return fmt.Errorf("report completion: %w", err)
		}
	}
}

func workerRegistrationRequest() model.WorkerRegistrationRequest {
	return model.WorkerRegistrationRequest{
		ExecutionHandle:      fmt.Sprintf("pid-%d", os.Getpid()),
		ExecutionEnvironment: runtime.GOOS + "/" + runtime.GOARCH,
	}
}
