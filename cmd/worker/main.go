package main

import (
	"fmt"
	"os"
	"time"
)

func main() {
	cfg, err := loadConfig(workerConfigPath(os.Args))
	if err != nil {
		fmt.Println("invalid config:", err)
		return
	}

	worker := Worker{Config: cfg}

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
	for {
		item, hasWork, err := fetchWorkItem(worker.Config.ControllerURL)
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
			if reportErr := reportWorkFailed(worker.Config.ControllerURL, item, err); reportErr != nil {
				return fmt.Errorf("run work item: %v; report failure: %w", err, reportErr)
			}
			return err
		}

		if err := reportWorkComplete(worker.Config.ControllerURL, item, startedAt, evidence); err != nil {
			return fmt.Errorf("report completion: %w", err)
		}
	}
}
