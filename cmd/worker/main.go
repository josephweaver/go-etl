package main

import (
	"fmt"
	"os"
	"time"
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

	for {
		item, hasWork, err := controller.FetchWorkItem()
		if err != nil {
			return fmt.Errorf("fetch work item: %w", err)
		}

		if !hasWork {
			fmt.Println("no work available")
			return nil
		}

		startedAt := time.Now().UTC()
		evidence, err := worker.Run(item)
		if err != nil {
			if reportErr := controller.ReportWorkFailed(item, err); reportErr != nil {
				return fmt.Errorf("run work item: %v; report failure: %w", err, reportErr)
			}
			return err
		}

		if err := controller.ReportWorkComplete(item, startedAt, evidence); err != nil {
			return fmt.Errorf("report completion: %w", err)
		}
	}
}
