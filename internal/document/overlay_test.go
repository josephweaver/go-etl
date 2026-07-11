package document

import (
	"reflect"
	"strings"
	"testing"

	"goetl/internal/fingerprint"
)

func TestEffectiveDataTreeAppliesWorkflowAndSubmissionPrecedence(t *testing.T) {
	projectData := map[string]any{
		"field_segments": map[string]any{
			"provider":  "gdrive_release_data",
			"selection": []any{"raster", "header"},
			"location": map[string]any{
				"type":       "gdrive_rclone",
				"remote":     "gdrive",
				"drive_path": "Data/Field_Boundaries/ReleaseData.7z",
			},
		},
	}
	workflowData := map[string]any{
		"field_segments": map[string]any{
			"selection": []any{"header"},
			"location": map[string]any{
				"drive_path": "Data/Field_Boundaries/Headers.7z",
			},
		},
	}
	submissionData := map[string]any{
		"field_segments": map[string]any{
			"location": map[string]any{
				"remote": "shared_drive",
			},
		},
	}

	effective, err := EffectiveDataTree(projectData, workflowData, submissionData)
	if err != nil {
		t.Fatalf("EffectiveDataTree() error = %v", err)
	}

	want := map[string]any{
		"field_segments": map[string]any{
			"provider":  "gdrive_release_data",
			"selection": []any{"header"},
			"location": map[string]any{
				"type":       "gdrive_rclone",
				"remote":     "shared_drive",
				"drive_path": "Data/Field_Boundaries/Headers.7z",
			},
		},
	}
	if !reflect.DeepEqual(effective, want) {
		t.Fatalf("effective data = %#v, want %#v", effective, want)
	}
}

func TestEffectiveDataTreeListsReplaceRatherThanConcatenate(t *testing.T) {
	effective, err := EffectiveDataTree(
		map[string]any{"asset": map[string]any{"roles": []any{"raster", "header"}}},
		map[string]any{"asset": map[string]any{"roles": []any{"header"}}},
		nil,
	)
	if err != nil {
		t.Fatalf("EffectiveDataTree() error = %v", err)
	}

	asset := effective["asset"].(map[string]any)
	roles := asset["roles"].([]any)
	if !reflect.DeepEqual(roles, []any{"header"}) {
		t.Fatalf("roles = %#v, want [header]", roles)
	}
}

func TestEffectiveDataTreeRejectsObjectScalarStructuralMismatch(t *testing.T) {
	tests := []struct {
		name       string
		project    map[string]any
		workflow   map[string]any
		wantSource string
		wantPath   string
	}{
		{
			name:       "object to scalar",
			project:    map[string]any{"asset": map[string]any{"location": map[string]any{"remote": "gdrive"}}},
			workflow:   map[string]any{"asset": map[string]any{"location": "gdrive:Data"}},
			wantSource: "workflow.data",
			wantPath:   "/asset/location",
		},
		{
			name:       "scalar to object",
			project:    map[string]any{"asset": map[string]any{"location": "gdrive:Data"}},
			workflow:   map[string]any{"asset": map[string]any{"location": map[string]any{"remote": "gdrive"}}},
			wantSource: "workflow.data",
			wantPath:   "/asset/location",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := EffectiveDataTree(test.project, test.workflow, nil)
			if err == nil {
				t.Fatal("EffectiveDataTree() succeeded, want structural mismatch")
			}
			if !strings.Contains(err.Error(), test.wantSource) || !strings.Contains(err.Error(), test.wantPath) {
				t.Fatalf("error = %v, want source %q and path %q", err, test.wantSource, test.wantPath)
			}
		})
	}
}

func TestEffectiveDataTreeRejectsNull(t *testing.T) {
	_, err := EffectiveDataTree(
		map[string]any{"asset": map[string]any{"selection": []any{"raster"}}},
		map[string]any{"asset": map[string]any{"selection": nil}},
		nil,
	)
	if err == nil {
		t.Fatal("EffectiveDataTree() succeeded, want null rejection")
	}
	if !strings.Contains(err.Error(), "workflow.data") || !strings.Contains(err.Error(), "/asset/selection") || !strings.Contains(err.Error(), "null") {
		t.Fatalf("error = %v, want workflow layer, path, and null diagnostic", err)
	}
}

func TestEffectiveDataTreeIsStableAcrossJSONAndYAMLSources(t *testing.T) {
	jsonRoot := decodeRootObject(t, []byte(`{
		"project": {
			"asset": {
				"provider": "gdrive_release_data",
				"selection": ["raster", "header"],
				"location": {
					"type": "gdrive_rclone",
					"remote": "gdrive",
					"drive_path": "Data/Field_Boundaries/ReleaseData.7z"
				}
			}
		},
		"workflow": {
			"asset": {
				"selection": ["header"],
				"location": {
					"drive_path": "Data/Field_Boundaries/Headers.7z"
				}
			}
		}
	}`), DecodeOptions{Path: "data.json"})

	yamlRoot := decodeRootObject(t, []byte(`
workflow:
  asset:
    location:
      drive_path: Data/Field_Boundaries/Headers.7z
    selection:
      - header
project:
  asset:
    location:
      remote: gdrive
      drive_path: Data/Field_Boundaries/ReleaseData.7z
      type: gdrive_rclone
    provider: gdrive_release_data
    selection:
      - raster
      - header
`), DecodeOptions{Path: "data.yaml"})

	jsonEffective, err := EffectiveDataTree(sectionObject(t, jsonRoot, "project"), sectionObject(t, jsonRoot, "workflow"), nil)
	if err != nil {
		t.Fatalf("EffectiveDataTree(JSON) error = %v", err)
	}
	yamlEffective, err := EffectiveDataTree(sectionObject(t, yamlRoot, "project"), sectionObject(t, yamlRoot, "workflow"), nil)
	if err != nil {
		t.Fatalf("EffectiveDataTree(YAML) error = %v", err)
	}

	jsonCanonical, jsonHash, err := fingerprint.CanonicalJSONSHA256(jsonEffective)
	if err != nil {
		t.Fatalf("canonical JSON hash for JSON effective data: %v", err)
	}
	yamlCanonical, yamlHash, err := fingerprint.CanonicalJSONSHA256(yamlEffective)
	if err != nil {
		t.Fatalf("canonical JSON hash for YAML effective data: %v", err)
	}
	if jsonHash != yamlHash {
		t.Fatalf("canonical hashes differ: JSON %s %s YAML %s %s", jsonHash, jsonCanonical, yamlHash, yamlCanonical)
	}
}

func decodeRootObject(t *testing.T, data []byte, options DecodeOptions) map[string]any {
	t.Helper()

	value, err := DecodeSource(data, options)
	if err != nil {
		t.Fatalf("DecodeSource(%s) error = %v", options.Path, err)
	}
	root, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("DecodeSource(%s) = %T, want object", options.Path, value)
	}
	return root
}

func sectionObject(t *testing.T, root map[string]any, name string) map[string]any {
	t.Helper()

	section, ok := root[name].(map[string]any)
	if !ok {
		t.Fatalf("section %s = %T, want object", name, root[name])
	}
	return section
}
