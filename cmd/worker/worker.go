package main

import (
	"fmt"
	"os"
	"path/filepath"
)

import "goetl/internal/model"

type Worker struct {
	Config Config
}

func (w Worker) Run(item model.WorkItem) (WorkEvidence, error) {
	fmt.Println("worker starting")
	fmt.Println("log dir:", w.Config.LogDir)

	if err := w.log("worker starting"); err != nil {
		return WorkEvidence{}, err
	}

	return w.runWorkItem(item)
}

func (w Worker) Validate() error {
	if err := requireDir(w.Config.LogDir); err != nil {
		return err
	}

	if err := requireDir(w.Config.TmpDir); err != nil {
		return err
	}

	if err := requireDir(w.Config.DataDir); err != nil {
		return err
	}

	return nil
}

func (w Worker) log(message string) error {
	path := filepath.Join(w.Config.LogDir, "worker.log")

	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("open log file %s: %w", path, err)
	}
	defer file.Close()

	if _, err := fmt.Fprintln(file, message); err != nil {
		return fmt.Errorf("write log file %s: %w", path, err)
	}
	return nil
}

func requireDir(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("check directory %s: %w", path, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("%s is not a directory", path)
	}
	return nil
}

func (w Worker) runWorkItem(item model.WorkItem) (WorkEvidence, error) {
	if err := item.Validate(); err != nil {
		return WorkEvidence{}, fmt.Errorf("invalid work item: %w", err)
	}

	switch item.Type {
	case model.WorkItemTypeWriteDemoOutput:
		return w.writeDemoOutput(item)
	case model.WorkItemTypeSummarizeInputFile:
		return w.summarizeInputFile(item)
	case model.WorkItemTypePythonScript:
		return w.runPythonScript(item)
	case model.WorkItemTypeCacheData:
		return w.cacheData(item)
	default:
		return WorkEvidence{}, fmt.Errorf("unsupported work item type: %s", item.Type)
	}
}
