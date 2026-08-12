package main

import (
	"context"
	"encoding/json"
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
	PauseAdapters  PauseAdapterRegistry
	LocalOnly      bool
}

func (w Worker) Run(item model.WorkItem) (WorkEvidence, error) {
	if item.Resume != nil {
		return WorkEvidence{}, fmt.Errorf("resume assignment requires supervised execution")
	}
	fmt.Println("worker starting")
	fmt.Println("log dir:", w.Config.LogDir)

	if err := w.log("worker starting"); err != nil {
		return WorkEvidence{}, err
	}

	return w.runWorkItem(item)
}

func (w Worker) Validate() error {
	if err := w.Config.validateCheckpointPolicy(); err != nil {
		return err
	}
	if err := w.Config.validatePauseAdapterProfile(); err != nil {
		return err
	}
	if err := w.validatePauseAdapters(); err != nil {
		return err
	}

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

func (w Worker) validatePauseAdapters() error {
	if w.Config.CheckpointMode == CheckpointModeDisabled {
		return nil
	}
	registry, err := w.pauseAdapterRegistry()
	if err != nil {
		return err
	}
	if registry.Len() == 0 {
		return fmt.Errorf("checkpoint mode %q requires at least one pause adapter", w.Config.CheckpointMode)
	}
	for workItemType, registration := range registry.byWorkItemType {
		if err := registration.Validate(); err != nil {
			return fmt.Errorf("pause adapter for work item type %q: %w", workItemType, err)
		}
		if registration.WorkItemType != workItemType {
			return fmt.Errorf("pause adapter registry key %q does not match registration work item type %q", workItemType, registration.WorkItemType)
		}
		if !registration.Capabilities.Supports(w.Config.CheckpointMode) {
			return fmt.Errorf("pause adapter for work item type %q does not support checkpoint mode %q", workItemType, w.Config.CheckpointMode)
		}
	}
	return nil
}

func (w Worker) pauseAdapterRegistry() (PauseAdapterRegistry, error) {
	if w.PauseAdapters.byWorkItemType != nil {
		return w.PauseAdapters, nil
	}
	if w.Config.PauseAdapterProfile == "" {
		return NewPauseAdapterRegistry()
	}
	if w.Config.PauseAdapterProfile != PauseAdapterProfileDirectPythonDMTCP42 {
		return PauseAdapterRegistry{}, fmt.Errorf("unsupported pause adapter profile %q", w.Config.PauseAdapterProfile)
	}
	if w.Config.DMTCPProfile == nil {
		return PauseAdapterRegistry{}, fmt.Errorf("pause adapter profile %q requires dmtcp_profile", w.Config.PauseAdapterProfile)
	}
	profile := w.Config.DMTCPProfile.launchProfile()
	if err := profile.validate(); err != nil {
		return PauseAdapterRegistry{}, fmt.Errorf("DMTCP pause adapter profile: %w", err)
	}
	adapter := &DMTCPAdapter{Worker: w, Profile: profile}
	return NewPauseAdapterRegistry(PauseAdapterRegistration{
		WorkItemType:   model.WorkItemTypePythonScript,
		Strategy:       model.PauseStrategyDMTCP,
		AdapterID:      profile.AdapterID,
		AdapterVersion: profile.AdapterVersion,
		Capabilities: PauseAdapterCapabilities{
			Shutdown: true,
			Periodic: true,
			Yield:    true,
		},
		Adapter: adapter,
	})
}

func (w Worker) RunSupervised(
	ctx context.Context,
	item model.WorkItem,
	session WorkerSession,
	checkpoints ExecutionSupervisorCheckpointClient,
	drain WorkerDrainSource,
) (ExecutionSupervisorOutcome, error) {
	supervisor, err := w.executionSupervisor(item, session, checkpoints, drain)
	if err != nil {
		return ExecutionSupervisorOutcome{}, err
	}
	return supervisor.Run(ctx)
}

func (w Worker) executionSupervisor(
	item model.WorkItem,
	session WorkerSession,
	checkpoints ExecutionSupervisorCheckpointClient,
	drain WorkerDrainSource,
) (ExecutionSupervisor, error) {
	if err := item.Validate(); err != nil {
		return ExecutionSupervisor{}, fmt.Errorf("supervised work item: %w", err)
	}
	if w.Config.CheckpointMode == CheckpointModeDisabled {
		return ExecutionSupervisor{}, fmt.Errorf("supervised execution requires an enabled checkpoint mode")
	}
	if err := w.validatePauseAdapters(); err != nil {
		return ExecutionSupervisor{}, err
	}
	registry, err := w.pauseAdapterRegistry()
	if err != nil {
		return ExecutionSupervisor{}, err
	}
	registration, found := registry.AdapterFor(item.Type)
	if !found {
		return ExecutionSupervisor{}, fmt.Errorf("no pause adapter is registered for work item type %q", item.Type)
	}
	if item.Resume != nil {
		if err := validateResumeAdapter(item, registration); err != nil {
			return ExecutionSupervisor{}, err
		}
	}
	return ExecutionSupervisor{
		Config:       w.Config,
		Item:         item,
		Registration: registration,
		Session:      session,
		Checkpoints:  checkpoints,
		Drain:        drain,
		Clock:        w.LifecycleClock,
	}, nil
}

func validateResumeAdapter(item model.WorkItem, registration PauseAdapterRegistration) error {
	var manifest model.ResumeArtifactManifest
	if err := json.Unmarshal([]byte(item.Resume.ManifestJSON), &manifest); err != nil {
		return fmt.Errorf("decode resume manifest for adapter selection: %w", err)
	}
	if manifest.PauseStrategy != registration.Strategy {
		return fmt.Errorf("resume pause strategy %q does not match registered adapter strategy %q", manifest.PauseStrategy, registration.Strategy)
	}
	if manifest.Compatibility.AdapterID != registration.AdapterID {
		return fmt.Errorf("resume adapter id does not match registered adapter")
	}
	if manifest.Compatibility.AdapterVersion != registration.AdapterVersion {
		return fmt.Errorf("resume adapter version does not match registered adapter")
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
