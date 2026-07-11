package document

import (
	"reflect"
	"testing"

	"goetl/internal/model"
)

func TestDataDefinitionsFromOverlaidValueCompilesHeaderOnlyAsset(t *testing.T) {
	project := map[string]any{
		"inputs": map[string]any{
			"yan_roy_field_segments": map[string]any{
				"kind": "envi_field_segments",
				"parameters": map[string]any{
					"tile": map[string]any{"type": "string"},
				},
				"files": map[string]any{
					"raster": map[string]any{
						"member":   "${asset.tile}/WELD_${asset.tile}_2010_field_segments",
						"as":       "WELD_${asset.tile}_2010_field_segments",
						"required": true,
					},
					"header": map[string]any{
						"member":   "${asset.tile}/WELD_${asset.tile}_2010_field_segments.hdr",
						"as":       "WELD_${asset.tile}_2010_field_segments.hdr",
						"required": true,
					},
				},
				"select": []any{"raster", "header"},
				"binding": map[string]any{
					"provider_name": "gdrive_release_data",
					"provider":      "gdrive_rclone",
					"location": map[string]any{
						"remote":     "gdrive",
						"drive_path": "Data/Field_Boundaries/ReleaseData.7z",
					},
					"archive": map[string]any{
						"type":   "seven_zip",
						"expose": "selected_directory",
					},
					"cache": map[string]any{
						"strategy":  "worker_cache",
						"cache_key": "gdrive/field_boundaries/release-data/source.7z",
						"immutable": true,
					},
				},
			},
		},
	}
	workflow := map[string]any{
		"inputs": map[string]any{
			"yan_roy_field_segments": map[string]any{
				"select": []any{"header"},
			},
		},
	}

	effective, err := EffectiveDataTree(project, workflow, nil)
	if err != nil {
		t.Fatalf("EffectiveDataTree() error = %v", err)
	}
	definitions, err := DataDefinitionsFromValue(effective)
	if err != nil {
		t.Fatalf("DataDefinitionsFromValue() error = %v", err)
	}

	asset, err := definitions.Inputs["yan_roy_field_segments"].BoundInputAsset("yan_roy_field_segments", map[string]any{"tile": "h18v07"})
	if err != nil {
		t.Fatalf("BoundInputAsset() error = %v", err)
	}
	gotMembers := make([]string, 0, len(asset.Archive.Select))
	for _, selector := range asset.Archive.Select {
		gotMembers = append(gotMembers, selector.Member)
	}
	wantMembers := []string{"h18v07/WELD_h18v07_2010_field_segments.hdr"}
	if !reflect.DeepEqual(gotMembers, wantMembers) {
		t.Fatalf("archive members = %#v, want %#v", gotMembers, wantMembers)
	}
	if asset.Provider != model.DataProviderGDriveRclone {
		t.Fatalf("provider = %q", asset.Provider)
	}
}
