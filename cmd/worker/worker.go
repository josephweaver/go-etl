package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

import "goetl/internal/model"

type Worker struct {
	Config         Config
	Controller     WorkerControllerClient
	LifecycleClock WorkerLifecycleClock
	SourceBundles  SourceBundleProvider
	LocalOnly      bool
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

func (w Worker) controllerClient() (WorkerControllerClient, error) {
	if w.Controller.Initialized() {
		return w.Controller, nil
	}
	return NewWorkerControllerClient(w.Config)
}

func (w Worker) sourceBundleProvider() (SourceBundleProvider, error) {
	if w.SourceBundles != nil {
		return w.SourceBundles, nil
	}
	if w.LocalOnly {
		return nil, fmt.Errorf("source-bundle provider is required for local-only worker execution")
	}
	controller, err := w.controllerClient()
	if err != nil {
		return nil, fmt.Errorf("controller client: %w", err)
	}
	return ControllerSourceBundleProvider{Controller: controller}, nil
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
	case model.WorkItemTypePythonScript:
		return w.runPythonScript(item)
	case model.WorkItemTypeWriteDemoOutput,
		model.WorkItemTypeSummarizeInputFile,
		model.WorkItemTypeAssetMaterialize,
		model.WorkItemTypeArchiveExtract,
		model.WorkItemTypeCommitData:
	default:
		return WorkEvidence{}, fmt.Errorf("unsupported work item type: %s", item.Type)
	}

	operation, err := w.operationContext(context.Background(), item, trustedGoSensitiveNeeds(item.Type))
	if err != nil {
		return WorkEvidence{}, fmt.Errorf("build operation context: %w", err)
	}

	switch item.Type {
	case model.WorkItemTypeWriteDemoOutput:
		return w.writeDemoOutput(operation)
	case model.WorkItemTypeSummarizeInputFile:
		return w.summarizeInputFile(operation)
	case model.WorkItemTypeAssetMaterialize:
		return w.AssetMaterialize(operation)
	case model.WorkItemTypeArchiveExtract:
		return w.ArchiveExtract(operation)
	case model.WorkItemTypeCommitData:
		return w.commitData(operation)
	default:
		return WorkEvidence{}, fmt.Errorf("unsupported work item type: %s", item.Type)
	}
}

func trustedGoSensitiveNeeds(itemType model.WorkItemType) []string {
	switch itemType {
	case model.WorkItemTypeWriteDemoOutput:
		return []string{"demo_secret"}
	default:
		return nil
	}
}
