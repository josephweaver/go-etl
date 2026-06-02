package main

import (
	"fmt"
)

func main() {
	cfg, err := loadConfig("demo-config.json")
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

		if err := worker.Run(item); err != nil {
			if reportErr := reportWorkFailed(worker.Config.ControllerURL, item.ID, err); reportErr != nil {
				return fmt.Errorf("run work item: %v; report failure: %w", err, reportErr)
			}
			return err
		}

		if err := reportWorkComplete(worker.Config.ControllerURL, item.ID); err != nil {
			return fmt.Errorf("report completion: %w", err)
		}
	}
}
