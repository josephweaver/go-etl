package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"goetl/internal/model"
)

func TestRequireDir(t *testing.T) {
	root := t.TempDir()
	filePath := filepath.Join(root, "file.txt")

	if err := os.WriteFile(filePath, []byte("test"), 0644); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{name: "existing directory", path: root},
		{name: "missing path", path: filepath.Join(root, "missing"), wantErr: true},
		{name: "regular file", path: filePath, wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := requireDir(test.path)

			if test.wantErr && err == nil {
				t.Fatal("expected an error")
			}

			if !test.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestWorkerRunWorkItemRejectsInvalidItem(t *testing.T) {
	worker := newTestWorker(t)

	item := model.WorkItem{
		ID:             "test-001",
		Type:           model.WorkItemTypeWriteDemoOutput,
		OutputFilename: "../outside.txt",
	}

	if _, err := worker.runWorkItem(item); err == nil {
		t.Fatal("expected an error")
	}
}

func TestWorkerRunRejectsResumeAssignment(t *testing.T) {
	worker := newTestWorker(t)
	_, err := worker.Run(model.WorkItem{Resume: &model.WorkItemResumeAssignment{}})
	if err == nil || !strings.Contains(err.Error(), "requires supervised execution") {
		t.Fatalf("Run(resume) error = %v", err)
	}
}

func newTestWorker(t *testing.T) Worker {
	t.Helper()

	root := t.TempDir()

	config := Config{
		LogDir:        filepath.Join(root, "logs"),
		TmpDir:        filepath.Join(root, "tmp"),
		DataDir:       filepath.Join(root, "data"),
		ControllerURL: "https://controller.local",
	}

	for _, dir := range []string{config.LogDir, config.TmpDir, config.DataDir} {
		if err := os.Mkdir(dir, 0755); err != nil {
			t.Fatalf("create directory %s: %v", dir, err)
		}
	}

	return Worker{Config: config}
}

func TestWorkerRun(t *testing.T) {
	worker := newTestWorker(t)

	item := model.WorkItem{
		ID:             "test-001",
		Type:           model.WorkItemTypeWriteDemoOutput,
		OutputFilename: "result.txt",
	}

	evidence, err := worker.Run(item)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if evidence.OutputJSON == "" || evidence.PreStateJSON == "" || evidence.PostStateJSON == "" {
		t.Fatalf("expected run evidence: %+v", evidence)
	}

	dataPath := filepath.Join(worker.Config.DataDir, item.OutputFilename)
	if _, err := os.Stat(dataPath); err != nil {
		t.Fatalf("completed output does not exist: %v", err)
	}
}

func TestWorkerRunSummarizeInputFile(t *testing.T) {
	worker := newTestWorker(t)
	inputPath := filepath.Join(t.TempDir(), "input.txt")
	if err := os.WriteFile(inputPath, []byte("hello\n"), 0644); err != nil {
		t.Fatalf("write input: %v", err)
	}

	item := model.WorkItem{
		ID:             "summary-001",
		Type:           model.WorkItemTypeSummarizeInputFile,
		OutputFilename: "summary.txt",
		Parameters: model.Parameters{
			"input_path": {Type: "path", Value: inputPath},
		},
	}

	evidence, err := worker.Run(item)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if evidence.OutputJSON == "" || evidence.PreStateJSON == "" || evidence.PostStateJSON == "" {
		t.Fatalf("expected summary evidence: %+v", evidence)
	}

	dataPath := filepath.Join(worker.Config.DataDir, item.OutputFilename)
	if _, err := os.Stat(dataPath); err != nil {
		t.Fatalf("completed output does not exist: %v", err)
	}
}

func TestWorkerRunWorkItemRejectsUnsupportedType(t *testing.T) {
	worker := newTestWorker(t)

	item := model.WorkItem{
		ID:             "test-001",
		Type:           "unknown",
		OutputFilename: "result.txt",
	}

	if _, err := worker.runWorkItem(item); err == nil {
		t.Fatal("expected an error")
	}
}

func TestWorkerValidateRequiresAdaptersForEnabledCheckpointMode(t *testing.T) {
	tests := []struct {
		name         string
		mode         CheckpointMode
		capabilities PauseAdapterCapabilities
		withAdapter  bool
		wantErr      string
	}{
		{
			name: "disabled without adapters",
			mode: CheckpointModeDisabled,
		},
		{
			name:    "enabled without adapters",
			mode:    CheckpointModeShutdown,
			wantErr: "requires at least one pause adapter",
		},
		{
			name:         "adapter lacks periodic capability",
			mode:         CheckpointModePeriodic,
			withAdapter:  true,
			capabilities: PauseAdapterCapabilities{Shutdown: true},
			wantErr:      "does not support checkpoint mode",
		},
		{
			name:         "adapter supports configured mode",
			mode:         CheckpointModeYield,
			withAdapter:  true,
			capabilities: PauseAdapterCapabilities{Shutdown: true, Yield: true},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			worker := newTestWorker(t)
			configureWorkerCheckpointMode(&worker.Config, test.mode)
			if test.withAdapter {
				registration := validPauseAdapterRegistration(&fakePauseAdapter{execution: newFakeSupervisedExecution()})
				registration.Capabilities = test.capabilities
				registry, err := NewPauseAdapterRegistry(registration)
				if err != nil {
					t.Fatal(err)
				}
				worker.PauseAdapters = registry
			}

			err := worker.Validate()
			if test.wantErr == "" {
				if err != nil {
					t.Fatalf("Validate() error = %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("Validate() error = %v, want text %q", err, test.wantErr)
			}
		})
	}
}

func TestWorkerRunSupervisedSelectsFreshAndResumeStart(t *testing.T) {
	for _, resume := range []bool{false, true} {
		name := "fresh"
		if resume {
			name = "resume"
		}
		t.Run(name, func(t *testing.T) {
			worker := newTestWorker(t)
			configureWorkerCheckpointMode(&worker.Config, CheckpointModeShutdown)
			execution := newFakeSupervisedExecution()
			execution.result <- ExecutionResult{Evidence: WorkEvidence{OutputSHA256: strings.Repeat("a", 64)}}
			adapter := &fakePauseAdapter{execution: execution}
			registration := validPauseAdapterRegistration(adapter)
			registry, err := NewPauseAdapterRegistry(registration)
			if err != nil {
				t.Fatal(err)
			}
			worker.PauseAdapters = registry

			item := model.WorkItem{
				ID:             "work-001",
				AttemptID:      "attempt-002",
				Type:           model.WorkItemTypeWriteDemoOutput,
				OutputFilename: "output.json",
			}
			if resume {
				item.Resume = workerResumeAssignment(t)
			}
			outcome, err := worker.RunSupervised(
				context.Background(),
				item,
				WorkerSession{WorkerID: "worker-001", WorkerSessionID: "session-001"},
				&supervisorTestCheckpointClient{},
				nil,
			)
			if err != nil {
				t.Fatalf("RunSupervised() error = %v", err)
			}
			if outcome.Kind != ExecutionSupervisorCompleted {
				t.Fatalf("outcome = %+v", outcome)
			}
			if resume {
				if adapter.resumeStarts != 1 || adapter.freshStarts != 0 {
					t.Fatalf("adapter starts fresh=%d resume=%d", adapter.freshStarts, adapter.resumeStarts)
				}
			} else if adapter.freshStarts != 1 || adapter.resumeStarts != 0 {
				t.Fatalf("adapter starts fresh=%d resume=%d", adapter.freshStarts, adapter.resumeStarts)
			}
		})
	}
}

func TestWorkerRunSupervisedRejectsMissingOrMismatchedResumeAdapter(t *testing.T) {
	item := model.WorkItem{
		ID:             "work-001",
		AttemptID:      "attempt-002",
		Type:           model.WorkItemTypeWriteDemoOutput,
		OutputFilename: "output.json",
		Resume:         workerResumeAssignment(t),
	}

	t.Run("missing", func(t *testing.T) {
		worker := newTestWorker(t)
		configureWorkerCheckpointMode(&worker.Config, CheckpointModeShutdown)
		adapter := &fakePauseAdapter{execution: newFakeSupervisedExecution()}
		registration := validPauseAdapterRegistration(adapter)
		registration.WorkItemType = model.WorkItemTypeAssetMaterialize
		registry, err := NewPauseAdapterRegistry(registration)
		if err != nil {
			t.Fatal(err)
		}
		worker.PauseAdapters = registry

		_, err = worker.RunSupervised(
			context.Background(),
			item,
			WorkerSession{WorkerID: "worker-001", WorkerSessionID: "session-001"},
			&supervisorTestCheckpointClient{},
			nil,
		)
		if err == nil || !strings.Contains(err.Error(), "no pause adapter") {
			t.Fatalf("RunSupervised() error = %v", err)
		}
		if adapter.freshStarts != 0 || adapter.resumeStarts != 0 {
			t.Fatalf("adapter starts fresh=%d resume=%d", adapter.freshStarts, adapter.resumeStarts)
		}
	})

	t.Run("mismatched adapter version", func(t *testing.T) {
		worker := newTestWorker(t)
		configureWorkerCheckpointMode(&worker.Config, CheckpointModeShutdown)
		adapter := &fakePauseAdapter{execution: newFakeSupervisedExecution()}
		registration := validPauseAdapterRegistration(adapter)
		registration.AdapterVersion = "2"
		registry, err := NewPauseAdapterRegistry(registration)
		if err != nil {
			t.Fatal(err)
		}
		worker.PauseAdapters = registry

		_, err = worker.RunSupervised(
			context.Background(),
			item,
			WorkerSession{WorkerID: "worker-001", WorkerSessionID: "session-001"},
			&supervisorTestCheckpointClient{},
			nil,
		)
		if err == nil || !strings.Contains(err.Error(), "adapter version") {
			t.Fatalf("RunSupervised() error = %v", err)
		}
		if adapter.freshStarts != 0 || adapter.resumeStarts != 0 {
			t.Fatalf("adapter starts fresh=%d resume=%d", adapter.freshStarts, adapter.resumeStarts)
		}
	})
}

func configureWorkerCheckpointMode(config *Config, mode CheckpointMode) {
	config.CheckpointMode = mode
	if mode == CheckpointModeDisabled {
		return
	}
	config.DrainPauseDelaySeconds = 10
	config.CheckpointCaptureTimeoutSeconds = 2
	config.CheckpointReportTimeoutSeconds = 2
	config.ExecutionTerminationGraceSeconds = 1
	if mode == CheckpointModePeriodic {
		config.CheckpointIntervalSeconds = 5
	}
	if mode == CheckpointModeYield {
		config.WorkItemExecutionQuantumSeconds = 5
	}
}

func workerResumeAssignment(t *testing.T) *model.WorkItemResumeAssignment {
	t.Helper()
	confirmation := testWorkerCheckpointConfirmation(t, "artifact-001", "source-v1")
	return &model.WorkItemResumeAssignment{
		Schema:               model.WorkItemResumeAssignmentSchemaV1,
		ResumedFromAttemptID: "attempt-001",
		ExecutionLineageID:   "lineage-001",
		ResumeAttemptNumber:  1,
		ManifestJSON:         confirmation.ManifestJSON,
		Reference:            confirmation.Reference,
	}
}
