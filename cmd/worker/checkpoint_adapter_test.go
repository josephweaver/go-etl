package main

import (
	"context"
	"errors"
	"strings"
	"testing"

	"goetl/internal/model"
)

func TestPauseAdapterCapabilitiesSupportCheckpointModes(t *testing.T) {
	capabilities := PauseAdapterCapabilities{Shutdown: true, Periodic: true}
	if err := capabilities.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	for _, mode := range []CheckpointMode{CheckpointModeDisabled, CheckpointModeShutdown, CheckpointModePeriodic} {
		if !capabilities.Supports(mode) {
			t.Fatalf("Supports(%q) = false, want true", mode)
		}
	}
	for _, mode := range []CheckpointMode{CheckpointModeYield, "future"} {
		if capabilities.Supports(mode) {
			t.Fatalf("Supports(%q) = true, want false", mode)
		}
	}
	if err := (PauseAdapterCapabilities{}).Validate(); err == nil {
		t.Fatal("capabilities without shutdown support validated")
	}
}

func TestPauseAdapterRegistryValidatesAndSelectsByWorkItemType(t *testing.T) {
	manual := &fakePauseAdapter{}
	native := &fakePauseAdapter{}
	manualRegistration := validPauseAdapterRegistration(manual)
	nativeRegistration := validPauseAdapterRegistration(native)
	nativeRegistration.WorkItemType = model.WorkItemTypeAssetMaterialize
	nativeRegistration.Strategy = model.PauseStrategyNative
	nativeRegistration.AdapterID = "native-test"

	registry, err := NewPauseAdapterRegistry(manualRegistration, nativeRegistration)
	if err != nil {
		t.Fatalf("NewPauseAdapterRegistry() error = %v", err)
	}
	if registry.Len() != 2 {
		t.Fatalf("registry length = %d, want 2", registry.Len())
	}
	selected, found := registry.AdapterFor(model.WorkItemTypeAssetMaterialize)
	if !found || selected.Adapter != native || selected.Strategy != model.PauseStrategyNative {
		t.Fatalf("selected registration = %+v found=%v", selected, found)
	}
	if _, found := registry.AdapterFor(model.WorkItemTypePythonScript); found {
		t.Fatal("unexpected adapter registration for python_script")
	}

	_, err = NewPauseAdapterRegistry(manualRegistration, manualRegistration)
	if err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("duplicate registration error = %v", err)
	}
}

func TestPauseAdapterRegistrationRejectsInvalidContracts(t *testing.T) {
	valid := validPauseAdapterRegistration(&fakePauseAdapter{})
	var typedNil *fakePauseAdapter
	tests := []struct {
		name    string
		mutate  func(*PauseAdapterRegistration)
		wantErr string
	}{
		{name: "missing work item type", mutate: func(value *PauseAdapterRegistration) { value.WorkItemType = "" }, wantErr: "work item type is required"},
		{name: "unsupported strategy", mutate: func(value *PauseAdapterRegistration) { value.Strategy = "future" }, wantErr: "unsupported pause adapter strategy"},
		{name: "missing adapter id", mutate: func(value *PauseAdapterRegistration) { value.AdapterID = "" }, wantErr: "adapter id is required"},
		{name: "whitespace adapter version", mutate: func(value *PauseAdapterRegistration) { value.AdapterVersion = " 1" }, wantErr: "leading or trailing whitespace"},
		{name: "missing shutdown capability", mutate: func(value *PauseAdapterRegistration) { value.Capabilities.Shutdown = false }, wantErr: "must support shutdown"},
		{name: "nil adapter", mutate: func(value *PauseAdapterRegistration) { value.Adapter = nil }, wantErr: "pause adapter is required"},
		{name: "typed nil adapter", mutate: func(value *PauseAdapterRegistration) { value.Adapter = typedNil }, wantErr: "pause adapter is required"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			registration := valid
			test.mutate(&registration)
			if err := registration.Validate(); err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("Validate() error = %v, want text %q", err, test.wantErr)
			}
		})
	}
}

func TestPauseAdapterHasDistinctFreshAndResumeStarts(t *testing.T) {
	adapter := &fakePauseAdapter{execution: newFakeSupervisedExecution()}
	item := model.WorkItem{ID: "work-001", AttemptID: "attempt-002", Type: model.WorkItemTypeWriteDemoOutput}
	if _, err := adapter.StartFresh(context.Background(), item); err != nil {
		t.Fatalf("StartFresh() error = %v", err)
	}
	assignment := model.WorkItemResumeAssignment{Schema: model.WorkItemResumeAssignmentSchemaV1}
	if _, err := adapter.StartResume(context.Background(), item, assignment); err != nil {
		t.Fatalf("StartResume() error = %v", err)
	}
	if adapter.freshStarts != 1 || adapter.resumeStarts != 1 || adapter.lastResume.Schema != assignment.Schema {
		t.Fatalf("start calls fresh=%d resume=%d assignment=%+v", adapter.freshStarts, adapter.resumeStarts, adapter.lastResume)
	}
}

func TestCheckpointCaptureRequestValidation(t *testing.T) {
	valid := validCheckpointCaptureRequest()
	tests := []struct {
		name    string
		mutate  func(*CheckpointCaptureRequest)
		wantErr string
	}{
		{name: "valid"},
		{name: "missing work item", mutate: func(value *CheckpointCaptureRequest) { value.WorkItemID = "" }, wantErr: "work item id is required"},
		{name: "missing work item type", mutate: func(value *CheckpointCaptureRequest) { value.WorkItemType = "" }, wantErr: "work item type is required"},
		{name: "missing attempt", mutate: func(value *CheckpointCaptureRequest) { value.AttemptID = "" }, wantErr: "attempt id is required"},
		{name: "missing lineage", mutate: func(value *CheckpointCaptureRequest) { value.ExecutionLineageID = "" }, wantErr: "execution lineage id is required"},
		{name: "generation zero", mutate: func(value *CheckpointCaptureRequest) { value.ResumeGeneration = 0 }, wantErr: "resume generation"},
		{name: "unsafe artifact id", mutate: func(value *CheckpointCaptureRequest) { value.ResumeArtifactID = "../artifact" }, wantErr: "safe path segment"},
		{name: "unsafe storage path", mutate: func(value *CheckpointCaptureRequest) { value.ArtifactStorageRelativePath = "../artifact" }, wantErr: "artifact storage relative path"},
		{name: "control character storage path", mutate: func(value *CheckpointCaptureRequest) { value.ArtifactStorageRelativePath = "goetl/resume/\nartifact" }, wantErr: "control characters"},
		{name: "unsupported capture kind", mutate: func(value *CheckpointCaptureRequest) { value.CaptureKind = "future" }, wantErr: "unsupported checkpoint capture kind"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := valid
			if test.mutate != nil {
				test.mutate(&request)
			}
			err := request.Validate()
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

func TestPreparedCheckpointValidation(t *testing.T) {
	confirmation := testWorkerCheckpointConfirmation(t, "artifact-001", "source-v1")
	checkpoint := PreparedCheckpoint{ManifestJSON: confirmation.ManifestJSON, Reference: confirmation.Reference}
	request := validCheckpointCaptureRequest()
	registration := validPauseAdapterRegistration(&fakePauseAdapter{})

	manifest, err := checkpoint.Validate(request, registration)
	if err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if manifest.ResumeArtifactID != request.ResumeArtifactID || manifest.ResumeGeneration != request.ResumeGeneration {
		t.Fatalf("manifest = %+v, want request identity", manifest)
	}

	tests := []struct {
		name    string
		mutate  func(*PreparedCheckpoint, *CheckpointCaptureRequest, *PauseAdapterRegistration)
		wantErr string
	}{
		{name: "empty manifest", mutate: func(value *PreparedCheckpoint, _ *CheckpointCaptureRequest, _ *PauseAdapterRegistration) {
			value.ManifestJSON = ""
		}, wantErr: "manifest JSON is required"},
		{name: "bad digest", mutate: func(value *PreparedCheckpoint, _ *CheckpointCaptureRequest, _ *PauseAdapterRegistration) {
			value.Reference.ManifestSHA256 = strings.Repeat("b", 64)
		}, wantErr: "digest does not match"},
		{name: "wrong work item", mutate: func(_ *PreparedCheckpoint, value *CheckpointCaptureRequest, _ *PauseAdapterRegistration) {
			value.WorkItemID = "work-002"
		}, wantErr: "work item id does not match"},
		{name: "wrong work item type", mutate: func(_ *PreparedCheckpoint, value *CheckpointCaptureRequest, registration *PauseAdapterRegistration) {
			value.WorkItemType = model.WorkItemTypeAssetMaterialize
			registration.WorkItemType = model.WorkItemTypeAssetMaterialize
		}, wantErr: "work item type does not match"},
		{name: "wrong attempt", mutate: func(_ *PreparedCheckpoint, value *CheckpointCaptureRequest, _ *PauseAdapterRegistration) {
			value.AttemptID = "attempt-002"
		}, wantErr: "producing attempt id does not match"},
		{name: "wrong lineage", mutate: func(_ *PreparedCheckpoint, value *CheckpointCaptureRequest, _ *PauseAdapterRegistration) {
			value.ExecutionLineageID = "lineage-002"
		}, wantErr: "execution lineage id does not match"},
		{name: "wrong generation", mutate: func(_ *PreparedCheckpoint, value *CheckpointCaptureRequest, _ *PauseAdapterRegistration) {
			value.ResumeGeneration = 2
		}, wantErr: "resume generation does not match"},
		{name: "wrong artifact", mutate: func(_ *PreparedCheckpoint, value *CheckpointCaptureRequest, _ *PauseAdapterRegistration) {
			value.ResumeArtifactID = "artifact-002"
		}, wantErr: "resume artifact id does not match"},
		{name: "wrong storage", mutate: func(_ *PreparedCheckpoint, value *CheckpointCaptureRequest, _ *PauseAdapterRegistration) {
			value.ArtifactStorageRelativePath = "goetl/resume/artifact-002"
		}, wantErr: "storage relative path does not match"},
		{name: "wrong strategy", mutate: func(_ *PreparedCheckpoint, _ *CheckpointCaptureRequest, value *PauseAdapterRegistration) {
			value.Strategy = model.PauseStrategyNative
		}, wantErr: "pause strategy does not match"},
		{name: "wrong adapter id", mutate: func(_ *PreparedCheckpoint, _ *CheckpointCaptureRequest, value *PauseAdapterRegistration) {
			value.AdapterID = "other"
		}, wantErr: "adapter id does not match"},
		{name: "wrong adapter version", mutate: func(_ *PreparedCheckpoint, _ *CheckpointCaptureRequest, value *PauseAdapterRegistration) {
			value.AdapterVersion = "2"
		}, wantErr: "adapter version does not match"},
		{name: "invalid manifest JSON", mutate: func(value *PreparedCheckpoint, _ *CheckpointCaptureRequest, _ *PauseAdapterRegistration) {
			value.ManifestJSON = "{"
		}, wantErr: "manifest JSON is invalid"},
		{name: "reference outside storage", mutate: func(value *PreparedCheckpoint, _ *CheckpointCaptureRequest, _ *PauseAdapterRegistration) {
			value.Reference.ManifestRelativePath = "other/manifest.json"
		}, wantErr: "outside artifact storage"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate := checkpoint
			candidateRequest := request
			candidateRegistration := registration
			test.mutate(&candidate, &candidateRequest, &candidateRegistration)
			if _, err := candidate.Validate(candidateRequest, candidateRegistration); err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("Validate() error = %v, want text %q", err, test.wantErr)
			}
		})
	}
}

func TestFakeSupervisedExecutionImplementsCaptureLifecycle(t *testing.T) {
	execution := newFakeSupervisedExecution()
	request := validCheckpointCaptureRequest()
	checkpoint := PreparedCheckpoint{ManifestJSON: "manifest"}
	execution.periodicCheckpoint = checkpoint
	execution.suspendCheckpoint = checkpoint
	if got, err := execution.CapturePeriodic(context.Background(), request); err != nil || got.ManifestJSON != "manifest" {
		t.Fatalf("CapturePeriodic() = %+v, %v", got, err)
	}
	request.CaptureKind = model.CheckpointCaptureKindFinal
	if got, err := execution.CaptureForSuspend(context.Background(), request); err != nil || got.ManifestJSON != "manifest" {
		t.Fatalf("CaptureForSuspend() = %+v, %v", got, err)
	}
	if err := execution.Continue(context.Background()); err != nil {
		t.Fatalf("Continue() error = %v", err)
	}
	if err := execution.Terminate(context.Background()); err != nil {
		t.Fatalf("Terminate() error = %v", err)
	}
	execution.result <- ExecutionResult{Err: errors.New("causal failure")}
	if result := <-execution.Result(); result.Err == nil {
		t.Fatal("Result() did not return terminal error")
	}
	if execution.periodicCaptures != 1 || execution.suspendCaptures != 1 || execution.continues != 1 || execution.terminations != 1 {
		t.Fatalf("execution calls periodic=%d suspend=%d continue=%d terminate=%d", execution.periodicCaptures, execution.suspendCaptures, execution.continues, execution.terminations)
	}
}

func validPauseAdapterRegistration(adapter PauseAdapter) PauseAdapterRegistration {
	return PauseAdapterRegistration{
		WorkItemType:   model.WorkItemTypeWriteDemoOutput,
		Strategy:       model.PauseStrategyManual,
		AdapterID:      "manual-test",
		AdapterVersion: "1",
		Capabilities: PauseAdapterCapabilities{
			Shutdown: true,
			Periodic: true,
			Yield:    true,
		},
		Adapter: adapter,
	}
}

func validCheckpointCaptureRequest() CheckpointCaptureRequest {
	return CheckpointCaptureRequest{
		WorkItemID:                  "work-001",
		WorkItemType:                model.WorkItemTypeWriteDemoOutput,
		AttemptID:                   "attempt-001",
		ExecutionLineageID:          "lineage-001",
		ResumeGeneration:            1,
		ResumeArtifactID:            "artifact-001",
		ArtifactStorageRelativePath: "goetl/resume/artifact-001",
		CaptureKind:                 model.CheckpointCaptureKindPeriodic,
	}
}

type fakePauseAdapter struct {
	execution    SupervisedExecution
	freshStarts  int
	resumeStarts int
	lastResume   model.WorkItemResumeAssignment
}

func (adapter *fakePauseAdapter) StartFresh(context.Context, model.WorkItem) (SupervisedExecution, error) {
	adapter.freshStarts++
	return adapter.execution, nil
}

func (adapter *fakePauseAdapter) StartResume(_ context.Context, _ model.WorkItem, assignment model.WorkItemResumeAssignment) (SupervisedExecution, error) {
	adapter.resumeStarts++
	adapter.lastResume = assignment
	return adapter.execution, nil
}

type fakeSupervisedExecution struct {
	result             chan ExecutionResult
	periodicCheckpoint PreparedCheckpoint
	suspendCheckpoint  PreparedCheckpoint
	periodicCaptures   int
	suspendCaptures    int
	continues          int
	terminations       int
}

func newFakeSupervisedExecution() *fakeSupervisedExecution {
	return &fakeSupervisedExecution{result: make(chan ExecutionResult, 1)}
}

func (execution *fakeSupervisedExecution) Result() <-chan ExecutionResult {
	return execution.result
}

func (execution *fakeSupervisedExecution) CapturePeriodic(context.Context, CheckpointCaptureRequest) (PreparedCheckpoint, error) {
	execution.periodicCaptures++
	return execution.periodicCheckpoint, nil
}

func (execution *fakeSupervisedExecution) CaptureForSuspend(context.Context, CheckpointCaptureRequest) (PreparedCheckpoint, error) {
	execution.suspendCaptures++
	return execution.suspendCheckpoint, nil
}

func (execution *fakeSupervisedExecution) Continue(context.Context) error {
	execution.continues++
	return nil
}

func (execution *fakeSupervisedExecution) Terminate(context.Context) error {
	execution.terminations++
	return nil
}
