package document

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"goetl/internal/variable"
)

func TestDecodeStrictProject(t *testing.T) {
	document, err := DecodeStrictProject([]byte(`{
		"api_version": "goet/v1alpha1",
		"kind": "Project",
		"id": "landcore-rci",
		"variables": {
			"target_environment_id": "msu-hpcc"
		},
		"data": {
			"inputs": {
				"yan_roy_field_segments": {
					"kind": "envi_field_segments",
					"select": ["raster", "header"],
					"binding": {
						"provider_name": "gdrive_release_data",
						"provider": "gdrive_rclone",
						"location": {
							"remote": "gdrive",
							"drive_path": "Data/Field_Boundaries/ReleaseData.7z"
						}
					}
				}
			}
		}
	}`))
	if err != nil {
		t.Fatalf("DecodeStrictProject() error = %v", err)
	}
	if document.APIVersion != APIVersionV1Alpha1 || document.Kind != KindProject || document.ID != "landcore-rci" {
		t.Fatalf("decoded envelope = %+v", document)
	}
	assertRawJSONEqual(t, document.Variables["target_environment_id"], `"msu-hpcc"`)
	assertRawJSONContains(t, document.Data, "yan_roy_field_segments")
}

func TestDecodeStrictWorkflowReferenceExample(t *testing.T) {
	document, err := DecodeStrictWorkflow([]byte(`{
		"api_version": "goet/v1alpha1",
		"kind": "Workflow",
		"id": "yan-roy-header-analysis",
		"variables": {
			"tiles": ["h18v07", "h18v08"]
		},
		"data": {
			"inputs": {
				"yan_roy_field_segments": {
					"select": ["header"]
				}
			}
		},
		"steps": [
			{
				"id": "cache-field-segment-headers",
				"fan_out": {
					"over": "${workflow.tiles[*]}",
					"as": "tile",
					"id": "${fanout.tile}"
				},
				"data": {
					"materialize": {
						"field_segments": {
							"asset": "yan_roy_field_segments",
							"with": {
								"tile": "${fanout.tile}"
							}
						}
					}
				},
				"work": {
					"type": "cache_data"
				}
			},
			{
				"id": "analyze-field-segment-headers",
				"fan_out": {
					"over": "${workflow.tiles[*]}",
					"as": "tile",
					"id": "${fanout.tile}"
				},
				"data": {
					"inputs": {
						"field_segments": {
							"asset": "yan_roy_field_segments",
							"with": {
								"tile": "${fanout.tile}"
							}
						}
					}
				},
				"work": {
					"type": "python_script",
					"parameters": {
						"python_entrypoint": "scripts/analyze_header.py",
						"args": [
							"--header",
							"${data.field_segments.path[0]}"
						]
					}
				}
			}
		],
		"source_manifest": {
			"files": [
				{
					"role": "python_entrypoint",
					"path": "scripts/analyze_header.py",
					"content_type": "text/x-python"
				}
			]
		}
	}`))
	if err != nil {
		t.Fatalf("DecodeStrictWorkflow() error = %v", err)
	}
	if document.ID != "yan-roy-header-analysis" {
		t.Fatalf("workflow id = %q", document.ID)
	}
	if len(document.Steps) != 2 {
		t.Fatalf("steps count = %d, want 2", len(document.Steps))
	}
	if document.Steps[0].ID != "cache-field-segment-headers" || document.Steps[1].ID != "analyze-field-segment-headers" {
		t.Fatalf("decoded steps = %+v", document.Steps)
	}
	assertRawJSONEqual(t, document.Variables["tiles"], `["h18v07","h18v08"]`)
	assertRawJSONContains(t, document.Steps[0].Work, "cache_data")
	assertRawJSONContains(t, document.SourceManifest, "python_entrypoint")
}

func TestDecodeStrictController(t *testing.T) {
	document, err := DecodeStrictController([]byte(`{
		"api_version": "goet/v1alpha1",
		"kind": "Controller",
		"id": "local-controller",
		"variables": {
			"controller_url": "http://localhost:8080",
			"worker_max_count": 2
		},
		"execution_environment": {
			"id": "local",
			"runtime": {
				"type": "local_process"
			}
		}
	}`))
	if err != nil {
		t.Fatalf("DecodeStrictController() error = %v", err)
	}
	if document.ID != "local-controller" {
		t.Fatalf("controller id = %q", document.ID)
	}
	assertRawJSONEqual(t, document.Variables["worker_max_count"], `2`)
	assertRawJSONContains(t, document.ExecutionEnvironment, "local_process")
}

func TestDecodeStrictSubmissionOverrides(t *testing.T) {
	document, err := DecodeStrictSubmissionOverrides([]byte(`{
		"api_version": "goet/v1alpha1",
		"kind": "SubmissionOverrides",
		"id": "submission-override-001",
		"overrides": {
			"code_version": "experiment-17",
			"worker_max_count": 10
		},
		"data": {
			"inputs": {
				"yan_roy_field_segments": {
					"select": ["header"]
				}
			}
		}
	}`))
	if err != nil {
		t.Fatalf("DecodeStrictSubmissionOverrides() error = %v", err)
	}
	if document.Kind != KindSubmissionOverrides {
		t.Fatalf("kind = %q", document.Kind)
	}
	assertRawJSONEqual(t, document.Overrides["worker_max_count"], `10`)
	assertRawJSONContains(t, document.Data, "yan_roy_field_segments")
}

func TestDecodeStrictRejectsUnknownTopLevelFields(t *testing.T) {
	tests := []struct {
		name   string
		raw    string
		decode func([]byte) error
	}{
		{
			name: "controller",
			raw: `{
				"api_version": "goet/v1alpha1",
				"kind": "Controller",
				"id": "controller-001",
				"metadata": {}
			}`,
			decode: func(data []byte) error {
				_, err := DecodeStrictController(data)
				return err
			},
		},
		{
			name: "project",
			raw: `{
				"api_version": "goet/v1alpha1",
				"kind": "Project",
				"id": "project-001",
				"metadata": {}
			}`,
			decode: func(data []byte) error {
				_, err := DecodeStrictProject(data)
				return err
			},
		},
		{
			name: "workflow",
			raw: `{
				"api_version": "goet/v1alpha1",
				"kind": "Workflow",
				"id": "workflow-001",
				"metadata": {}
			}`,
			decode: func(data []byte) error {
				_, err := DecodeStrictWorkflow(data)
				return err
			},
		},
		{
			name: "submission overrides",
			raw: `{
				"api_version": "goet/v1alpha1",
				"kind": "SubmissionOverrides",
				"id": "overrides-001",
				"metadata": {}
			}`,
			decode: func(data []byte) error {
				_, err := DecodeStrictSubmissionOverrides(data)
				return err
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.decode([]byte(test.raw))
			if err == nil || !strings.Contains(err.Error(), `unknown top-level field "metadata"`) {
				t.Fatalf("decode error = %v, want unknown metadata field", err)
			}
		})
	}
}

func TestPublicDocumentsDoNotExposeInternalVariableSlices(t *testing.T) {
	internalVariableSlice := reflect.TypeOf([]variable.Variable{})
	publicDocuments := []reflect.Type{
		reflect.TypeOf(ControllerDocument{}),
		reflect.TypeOf(ProjectDocument{}),
		reflect.TypeOf(WorkflowDocument{}),
		reflect.TypeOf(SubmissionOverridesDocument{}),
	}

	for _, documentType := range publicDocuments {
		for index := 0; index < documentType.NumField(); index++ {
			field := documentType.Field(index)
			if field.Type == internalVariableSlice {
				t.Fatalf("%s.%s exposes []variable.Variable", documentType.Name(), field.Name)
			}
		}
	}
}

func assertRawJSONEqual(t *testing.T, got json.RawMessage, want string) {
	t.Helper()

	var gotValue any
	if err := json.Unmarshal(got, &gotValue); err != nil {
		t.Fatalf("decode got raw JSON %s: %v", got, err)
	}
	var wantValue any
	if err := json.Unmarshal([]byte(want), &wantValue); err != nil {
		t.Fatalf("decode want raw JSON %s: %v", want, err)
	}
	if !reflect.DeepEqual(gotValue, wantValue) {
		t.Fatalf("raw JSON = %s, want %s", got, want)
	}
}

func assertRawJSONContains(t *testing.T, got json.RawMessage, want string) {
	t.Helper()

	if !strings.Contains(string(got), want) {
		t.Fatalf("raw JSON = %s, want substring %q", got, want)
	}
}
