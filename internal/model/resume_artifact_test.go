package model

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestResumeArtifactManifestRoundTripByStrategy(t *testing.T) {
	for _, strategy := range []PauseStrategy{
		PauseStrategyDMTCP,
		PauseStrategyNative,
		PauseStrategyManual,
	} {
		t.Run(string(strategy), func(t *testing.T) {
			manifest := testResumeArtifactManifest(strategy)
			if err := manifest.Validate(); err != nil {
				t.Fatalf("Validate() error = %v", err)
			}

			data, err := json.Marshal(manifest)
			if err != nil {
				t.Fatalf("Marshal() error = %v", err)
			}
			for _, field := range []string{
				`"schema":"goet/resume-artifact/v1"`,
				`"resume_artifact_id":"resume-001"`,
				`"resume_generation":1`,
				`"pause_strategy":"` + string(strategy) + `"`,
				`"work_item_id":"work-001"`,
				`"work_item_type":`,
				`"producing_attempt_id":"attempt-001"`,
				`"execution_lineage_id":"lineage-001"`,
				`"input_fingerprint":"sha256:input-001"`,
				`"source_version":"source-001"`,
				`"code_version":"code-001"`,
				`"created_at":"2026-07-30T18:00:00Z"`,
				`"storage_scope":"shared_tmp"`,
				`"storage_relative_path":"goetl/resume/resume-001"`,
				`"retention_policy":"while_referenced"`,
				`"compatibility":`,
				`"adapter_id":"` + string(strategy) + `"`,
				`"adapter_version":"1"`,
				`"worker_execution_contract_version":"goet/worker-execution/v1"`,
				`"worker_version":"worker-build-001"`,
				`"container_image_identity":"sha256:image-001"`,
				`"operating_system":"linux"`,
				`"architecture":"amd64"`,
				`"container_runtime":"singularity-ce-4.1.2"`,
				`"files":`,
				`"path":`,
				`"size_bytes":`,
				`"sha256":`,
			} {
				if !strings.Contains(string(data), field) {
					t.Fatalf("manifest JSON %s missing %s", data, field)
				}
			}
			strategyFields := map[PauseStrategy][]string{
				PauseStrategyDMTCP: {
					`"build_identity":"dmtcp-4.2.0-f8009ce7"`,
					`"checkpoint_paths":`,
				},
				PauseStrategyNative: {
					`"operation":"asset.materialize"`,
					`"adapter_version":"1"`,
					`"backend_identity":"rclone-1.71.2:gdrive"`,
					`"state_file_paths":`,
				},
				PauseStrategyManual: {
					`"handler_id":"write_demo_output"`,
					`"handler_version":"1"`,
					`"state_schema":"goet/manual-write-demo-output/v1"`,
					`"state_file_path":"manual/state.json"`,
				},
			}
			for _, field := range strategyFields[strategy] {
				if !strings.Contains(string(data), field) {
					t.Fatalf("manifest JSON %s missing strategy field %s", data, field)
				}
			}

			var decoded ResumeArtifactManifest
			if err := json.Unmarshal(data, &decoded); err != nil {
				t.Fatalf("Unmarshal() error = %v", err)
			}
			if err := decoded.Validate(); err != nil {
				t.Fatalf("decoded Validate() error = %v", err)
			}
			if !reflect.DeepEqual(decoded, manifest) {
				t.Fatalf("round trip mismatch:\n got: %#v\nwant: %#v", decoded, manifest)
			}

			payloadFields := map[string]json.RawMessage{}
			if err := json.Unmarshal(data, &payloadFields); err != nil {
				t.Fatalf("Unmarshal(payload fields) error = %v", err)
			}
			for _, candidate := range []PauseStrategy{
				PauseStrategyDMTCP,
				PauseStrategyNative,
				PauseStrategyManual,
			} {
				_, present := payloadFields[string(candidate)]
				if present != (candidate == strategy) {
					t.Fatalf("payload %q presence = %t, want %t", candidate, present, candidate == strategy)
				}
			}
		})
	}
}

func TestResumeArtifactManifestRejectsInvalidCommonState(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*ResumeArtifactManifest)
		wantErr string
	}{
		{
			name:    "missing schema",
			mutate:  func(manifest *ResumeArtifactManifest) { manifest.Schema = "" },
			wantErr: "unsupported resume artifact schema",
		},
		{
			name:    "unknown schema",
			mutate:  func(manifest *ResumeArtifactManifest) { manifest.Schema = "goet/resume-artifact/v2" },
			wantErr: "unsupported resume artifact schema",
		},
		{
			name:    "unsafe artifact id",
			mutate:  func(manifest *ResumeArtifactManifest) { manifest.ResumeArtifactID = "../resume" },
			wantErr: "resume_artifact_id",
		},
		{
			name:    "dot artifact id",
			mutate:  func(manifest *ResumeArtifactManifest) { manifest.ResumeArtifactID = "." },
			wantErr: "safe path segment",
		},
		{
			name:    "generation zero",
			mutate:  func(manifest *ResumeArtifactManifest) { manifest.ResumeGeneration = 0 },
			wantErr: "resume_generation",
		},
		{
			name:    "unknown strategy",
			mutate:  func(manifest *ResumeArtifactManifest) { manifest.PauseStrategy = "future" },
			wantErr: "unsupported pause_strategy",
		},
		{
			name:    "missing work item id",
			mutate:  func(manifest *ResumeArtifactManifest) { manifest.WorkItemID = "" },
			wantErr: "work_item_id is required",
		},
		{
			name:    "missing work item type",
			mutate:  func(manifest *ResumeArtifactManifest) { manifest.WorkItemType = "" },
			wantErr: "work_item_type is required",
		},
		{
			name:    "missing producing attempt",
			mutate:  func(manifest *ResumeArtifactManifest) { manifest.ProducingAttemptID = "" },
			wantErr: "producing_attempt_id is required",
		},
		{
			name:    "missing execution lineage",
			mutate:  func(manifest *ResumeArtifactManifest) { manifest.ExecutionLineageID = "" },
			wantErr: "execution_lineage_id is required",
		},
		{
			name:    "missing input fingerprint",
			mutate:  func(manifest *ResumeArtifactManifest) { manifest.InputFingerprint = "" },
			wantErr: "input_fingerprint is required",
		},
		{
			name:    "missing source version",
			mutate:  func(manifest *ResumeArtifactManifest) { manifest.SourceVersion = "" },
			wantErr: "source_version is required",
		},
		{
			name:    "missing code version",
			mutate:  func(manifest *ResumeArtifactManifest) { manifest.CodeVersion = "" },
			wantErr: "code_version is required",
		},
		{
			name:    "required value has whitespace",
			mutate:  func(manifest *ResumeArtifactManifest) { manifest.WorkItemID = " work-001" },
			wantErr: "leading or trailing whitespace",
		},
		{
			name:    "required value has control character",
			mutate:  func(manifest *ResumeArtifactManifest) { manifest.InputFingerprint = "sha256:\x00bad" },
			wantErr: "control characters",
		},
		{
			name:    "invalid timestamp",
			mutate:  func(manifest *ResumeArtifactManifest) { manifest.CreatedAt = "2026-07-30" },
			wantErr: "created_at must be RFC 3339",
		},
		{
			name:    "unknown storage scope",
			mutate:  func(manifest *ResumeArtifactManifest) { manifest.StorageScope = "node_local" },
			wantErr: "unsupported resume artifact storage_scope",
		},
		{
			name:    "absolute storage path",
			mutate:  func(manifest *ResumeArtifactManifest) { manifest.StorageRelativePath = "/goetl/resume/resume-001" },
			wantErr: "storage_relative_path",
		},
		{
			name:    "storage path has parent segment",
			mutate:  func(manifest *ResumeArtifactManifest) { manifest.StorageRelativePath = "goetl/../resume-001" },
			wantErr: "storage_relative_path",
		},
		{
			name:    "storage path has control character",
			mutate:  func(manifest *ResumeArtifactManifest) { manifest.StorageRelativePath = "goetl/resume/\x00bad" },
			wantErr: "control characters",
		},
		{
			name:    "unknown retention policy",
			mutate:  func(manifest *ResumeArtifactManifest) { manifest.RetentionPolicy = "forever" },
			wantErr: "unsupported resume artifact retention_policy",
		},
		{
			name:    "missing adapter id",
			mutate:  func(manifest *ResumeArtifactManifest) { manifest.Compatibility.AdapterID = "" },
			wantErr: "compatibility: adapter_id is required",
		},
		{
			name:    "missing adapter version",
			mutate:  func(manifest *ResumeArtifactManifest) { manifest.Compatibility.AdapterVersion = "" },
			wantErr: "compatibility: adapter_version is required",
		},
		{
			name: "missing worker execution contract version",
			mutate: func(manifest *ResumeArtifactManifest) {
				manifest.Compatibility.WorkerExecutionContractVersion = ""
			},
			wantErr: "worker_execution_contract_version is required",
		},
		{
			name:    "missing worker version",
			mutate:  func(manifest *ResumeArtifactManifest) { manifest.Compatibility.WorkerVersion = "" },
			wantErr: "worker_version is required",
		},
		{
			name: "missing container image identity",
			mutate: func(manifest *ResumeArtifactManifest) {
				manifest.Compatibility.ContainerImageIdentity = ""
			},
			wantErr: "container_image_identity is required",
		},
		{
			name:    "missing operating system",
			mutate:  func(manifest *ResumeArtifactManifest) { manifest.Compatibility.OperatingSystem = "" },
			wantErr: "operating_system is required",
		},
		{
			name:    "missing architecture",
			mutate:  func(manifest *ResumeArtifactManifest) { manifest.Compatibility.Architecture = "" },
			wantErr: "architecture is required",
		},
		{
			name:    "missing container runtime",
			mutate:  func(manifest *ResumeArtifactManifest) { manifest.Compatibility.ContainerRuntime = "" },
			wantErr: "container_runtime is required",
		},
		{
			name:    "no files",
			mutate:  func(manifest *ResumeArtifactManifest) { manifest.Files = nil },
			wantErr: "at least one resume artifact file",
		},
		{
			name: "duplicate file",
			mutate: func(manifest *ResumeArtifactManifest) {
				manifest.Files = append(manifest.Files, manifest.Files[0])
			},
			wantErr: "duplicate resume artifact file path",
		},
		{
			name:    "absolute file path",
			mutate:  func(manifest *ResumeArtifactManifest) { manifest.Files[0].Path = "/checkpoint.dmtcp" },
			wantErr: "file path",
		},
		{
			name:    "negative file size",
			mutate:  func(manifest *ResumeArtifactManifest) { manifest.Files[0].SizeBytes = -1 },
			wantErr: "size_bytes must be non-negative",
		},
		{
			name:    "missing file hash",
			mutate:  func(manifest *ResumeArtifactManifest) { manifest.Files[0].SHA256 = "" },
			wantErr: "file sha256 is required",
		},
		{
			name:    "short file hash",
			mutate:  func(manifest *ResumeArtifactManifest) { manifest.Files[0].SHA256 = "abcd" },
			wantErr: "64-character lowercase",
		},
		{
			name: "uppercase file hash",
			mutate: func(manifest *ResumeArtifactManifest) {
				manifest.Files[0].SHA256 = strings.Repeat("A", 64)
			},
			wantErr: "64-character lowercase",
		},
		{
			name:    "no strategy payload",
			mutate:  func(manifest *ResumeArtifactManifest) { manifest.DMTCP = nil },
			wantErr: "exactly one resume strategy payload",
		},
		{
			name: "multiple strategy payloads",
			mutate: func(manifest *ResumeArtifactManifest) {
				manifest.Native = &NativeResumePayload{
					Operation:       "copy",
					AdapterVersion:  "1",
					BackendIdentity: "rclone:v1",
					StateFilePaths:  []string{manifest.Files[0].Path},
				}
			},
			wantErr: "exactly one resume strategy payload",
		},
		{
			name:    "strategy does not match payload",
			mutate:  func(manifest *ResumeArtifactManifest) { manifest.PauseStrategy = PauseStrategyNative },
			wantErr: `pause_strategy "native" requires native payload`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			manifest := testResumeArtifactManifest(PauseStrategyDMTCP)
			test.mutate(&manifest)
			assertResumeArtifactError(t, manifest.Validate(), test.wantErr)
		})
	}
}

func TestResumeArtifactManifestRejectsInvalidStrategyPayload(t *testing.T) {
	tests := []struct {
		name     string
		strategy PauseStrategy
		mutate   func(*ResumeArtifactManifest)
		wantErr  string
	}{
		{
			name:     "dmtcp missing build identity",
			strategy: PauseStrategyDMTCP,
			mutate:   func(manifest *ResumeArtifactManifest) { manifest.DMTCP.BuildIdentity = "" },
			wantErr:  "build_identity is required",
		},
		{
			name:     "dmtcp missing checkpoint paths",
			strategy: PauseStrategyDMTCP,
			mutate:   func(manifest *ResumeArtifactManifest) { manifest.DMTCP.CheckpointPaths = nil },
			wantErr:  "checkpoint_paths requires at least one path",
		},
		{
			name:     "dmtcp duplicate checkpoint path",
			strategy: PauseStrategyDMTCP,
			mutate: func(manifest *ResumeArtifactManifest) {
				manifest.DMTCP.CheckpointPaths[1] = manifest.DMTCP.CheckpointPaths[0]
			},
			wantErr: "checkpoint_paths contains duplicate path",
		},
		{
			name:     "dmtcp checkpoint missing from files",
			strategy: PauseStrategyDMTCP,
			mutate: func(manifest *ResumeArtifactManifest) {
				manifest.DMTCP.CheckpointPaths[0] = "dmtcp/missing.dmtcp"
			},
			wantErr: "is not present in files",
		},
		{
			name:     "native missing operation",
			strategy: PauseStrategyNative,
			mutate:   func(manifest *ResumeArtifactManifest) { manifest.Native.Operation = "" },
			wantErr:  "operation is required",
		},
		{
			name:     "native missing adapter version",
			strategy: PauseStrategyNative,
			mutate: func(manifest *ResumeArtifactManifest) {
				manifest.Native.AdapterVersion = ""
			},
			wantErr: "adapter_version is required",
		},
		{
			name:     "native adapter version mismatch",
			strategy: PauseStrategyNative,
			mutate: func(manifest *ResumeArtifactManifest) {
				manifest.Native.AdapterVersion = "2"
			},
			wantErr: "must match compatibility adapter_version",
		},
		{
			name:     "native missing backend identity",
			strategy: PauseStrategyNative,
			mutate:   func(manifest *ResumeArtifactManifest) { manifest.Native.BackendIdentity = "" },
			wantErr:  "backend_identity is required",
		},
		{
			name:     "native missing state files",
			strategy: PauseStrategyNative,
			mutate:   func(manifest *ResumeArtifactManifest) { manifest.Native.StateFilePaths = nil },
			wantErr:  "state_file_paths requires at least one path",
		},
		{
			name:     "native state file missing from files",
			strategy: PauseStrategyNative,
			mutate: func(manifest *ResumeArtifactManifest) {
				manifest.Native.StateFilePaths[0] = "native/missing.json"
			},
			wantErr: "is not present in files",
		},
		{
			name:     "manual missing handler id",
			strategy: PauseStrategyManual,
			mutate:   func(manifest *ResumeArtifactManifest) { manifest.Manual.HandlerID = "" },
			wantErr:  "handler_id is required",
		},
		{
			name:     "manual missing handler version",
			strategy: PauseStrategyManual,
			mutate:   func(manifest *ResumeArtifactManifest) { manifest.Manual.HandlerVersion = "" },
			wantErr:  "handler_version is required",
		},
		{
			name:     "manual missing state schema",
			strategy: PauseStrategyManual,
			mutate:   func(manifest *ResumeArtifactManifest) { manifest.Manual.StateSchema = "" },
			wantErr:  "state_schema is required",
		},
		{
			name:     "manual unsafe state path",
			strategy: PauseStrategyManual,
			mutate:   func(manifest *ResumeArtifactManifest) { manifest.Manual.StateFilePath = "../state.json" },
			wantErr:  "state_file_path",
		},
		{
			name:     "manual state file missing from files",
			strategy: PauseStrategyManual,
			mutate: func(manifest *ResumeArtifactManifest) {
				manifest.Manual.StateFilePath = "manual/missing.json"
			},
			wantErr: "is not present in files",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			manifest := testResumeArtifactManifest(test.strategy)
			test.mutate(&manifest)
			assertResumeArtifactError(t, manifest.Validate(), test.wantErr)
		})
	}
}

func TestResumeArtifactReferenceValidationAndRoundTrip(t *testing.T) {
	reference := ResumeArtifactReference{
		Schema:               ResumeArtifactSchemaV1,
		ResumeArtifactID:     "resume-001",
		StorageScope:         ResumeArtifactStorageScopeSharedTmp,
		ManifestRelativePath: "goetl/resume/resume-001/manifest.json",
		ManifestSHA256:       strings.Repeat("d", 64),
	}
	if err := reference.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}

	data, err := json.Marshal(reference)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	for _, field := range []string{
		`"schema":"goet/resume-artifact/v1"`,
		`"resume_artifact_id":"resume-001"`,
		`"storage_scope":"shared_tmp"`,
		`"manifest_relative_path":"goetl/resume/resume-001/manifest.json"`,
		`"manifest_sha256":"` + strings.Repeat("d", 64) + `"`,
	} {
		if !strings.Contains(string(data), field) {
			t.Fatalf("reference JSON %s missing %s", data, field)
		}
	}

	var decoded ResumeArtifactReference
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if !reflect.DeepEqual(decoded, reference) {
		t.Fatalf("round trip mismatch: got %#v, want %#v", decoded, reference)
	}

	tests := []struct {
		name    string
		mutate  func(*ResumeArtifactReference)
		wantErr string
	}{
		{
			name:    "unknown schema",
			mutate:  func(reference *ResumeArtifactReference) { reference.Schema = "goet/resume-artifact/v2" },
			wantErr: "unsupported resume artifact schema",
		},
		{
			name:    "unsafe artifact id",
			mutate:  func(reference *ResumeArtifactReference) { reference.ResumeArtifactID = "resume/001" },
			wantErr: "resume_artifact_id",
		},
		{
			name:    "unknown storage scope",
			mutate:  func(reference *ResumeArtifactReference) { reference.StorageScope = "node_local" },
			wantErr: "unsupported resume artifact storage_scope",
		},
		{
			name: "absolute manifest path",
			mutate: func(reference *ResumeArtifactReference) {
				reference.ManifestRelativePath = "/goetl/resume/resume-001/manifest.json"
			},
			wantErr: "manifest_relative_path",
		},
		{
			name: "windows manifest path",
			mutate: func(reference *ResumeArtifactReference) {
				reference.ManifestRelativePath = `goetl\resume\resume-001\manifest.json`
			},
			wantErr: "manifest_relative_path",
		},
		{
			name:    "missing manifest hash",
			mutate:  func(reference *ResumeArtifactReference) { reference.ManifestSHA256 = "" },
			wantErr: "manifest_sha256 is required",
		},
		{
			name: "uppercase manifest hash",
			mutate: func(reference *ResumeArtifactReference) {
				reference.ManifestSHA256 = strings.Repeat("D", 64)
			},
			wantErr: "64-character lowercase",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate := reference
			test.mutate(&candidate)
			assertResumeArtifactError(t, candidate.Validate(), test.wantErr)
		})
	}
}

func testResumeArtifactManifest(strategy PauseStrategy) ResumeArtifactManifest {
	manifest := ResumeArtifactManifest{
		Schema:              ResumeArtifactSchemaV1,
		ResumeArtifactID:    "resume-001",
		ResumeGeneration:    1,
		PauseStrategy:       strategy,
		WorkItemID:          "work-001",
		WorkItemType:        WorkItemTypePythonScript,
		ProducingAttemptID:  "attempt-001",
		ExecutionLineageID:  "lineage-001",
		InputFingerprint:    "sha256:input-001",
		SourceVersion:       "source-001",
		CodeVersion:         "code-001",
		CreatedAt:           "2026-07-30T18:00:00Z",
		StorageScope:        ResumeArtifactStorageScopeSharedTmp,
		StorageRelativePath: "goetl/resume/resume-001",
		RetentionPolicy:     ResumeArtifactRetentionWhileReferenced,
		Compatibility: ResumeArtifactCompatibility{
			AdapterID:                      string(strategy),
			AdapterVersion:                 "1",
			WorkerExecutionContractVersion: "goet/worker-execution/v1",
			WorkerVersion:                  "worker-build-001",
			ContainerImageIdentity:         "sha256:image-001",
			OperatingSystem:                "linux",
			Architecture:                   "amd64",
			ContainerRuntime:               "singularity-ce-4.1.2",
		},
	}

	switch strategy {
	case PauseStrategyDMTCP:
		manifest.Files = []ResumeArtifactFile{
			{
				Path:      "dmtcp/ckpt-parent.dmtcp",
				SizeBytes: 251256862,
				SHA256:    strings.Repeat("a", 64),
			},
			{
				Path:      "dmtcp/ckpt-child.dmtcp",
				SizeBytes: 37035748,
				SHA256:    strings.Repeat("b", 64),
			},
		}
		manifest.DMTCP = &DMTCPResumePayload{
			BuildIdentity: "dmtcp-4.2.0-f8009ce7",
			CheckpointPaths: []string{
				"dmtcp/ckpt-parent.dmtcp",
				"dmtcp/ckpt-child.dmtcp",
			},
		}
	case PauseStrategyNative:
		manifest.WorkItemType = WorkItemTypeAssetMaterialize
		manifest.Files = []ResumeArtifactFile{
			{
				Path:      "native/transfer-state.json",
				SizeBytes: 512,
				SHA256:    strings.Repeat("b", 64),
			},
			{
				Path:      "native/partial.data",
				SizeBytes: 4096,
				SHA256:    strings.Repeat("c", 64),
			},
		}
		manifest.Native = &NativeResumePayload{
			Operation:       "asset.materialize",
			AdapterVersion:  "1",
			BackendIdentity: "rclone-1.71.2:gdrive",
			StateFilePaths: []string{
				"native/transfer-state.json",
				"native/partial.data",
			},
		}
	case PauseStrategyManual:
		manifest.WorkItemType = WorkItemTypeWriteDemoOutput
		manifest.Files = []ResumeArtifactFile{
			{
				Path:      "manual/state.json",
				SizeBytes: 256,
				SHA256:    strings.Repeat("c", 64),
			},
		}
		manifest.Manual = &ManualResumePayload{
			HandlerID:      "write_demo_output",
			HandlerVersion: "1",
			StateSchema:    "goet/manual-write-demo-output/v1",
			StateFilePath:  "manual/state.json",
		}
	}
	return manifest
}

func assertResumeArtifactError(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil {
		t.Fatalf("Validate() error = nil, want containing %q", want)
	}
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("Validate() error = %q, want containing %q", err, want)
	}
}
