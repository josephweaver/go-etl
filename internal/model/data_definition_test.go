package model

import (
	"reflect"
	"testing"
)

func TestDataInputDefinitionBindsYanRoyArchiveSelection(t *testing.T) {
	definition := yanRoyFieldSegmentsDefinition()

	asset, err := definition.BoundInputAsset("yan_roy_field_segments", map[string]any{"tile": "h18v07"})
	if err != nil {
		t.Fatalf("BoundInputAsset() error = %v", err)
	}

	if asset.BindingName != "yan_roy_field_segments" {
		t.Fatalf("binding name = %q", asset.BindingName)
	}
	if asset.ProviderName != "gdrive_release_data" {
		t.Fatalf("provider name = %q", asset.ProviderName)
	}
	if asset.Provider != DataProviderGDriveRclone {
		t.Fatalf("provider = %q", asset.Provider)
	}
	if asset.Location.Remote != "gdrive" || asset.Location.DrivePath != "Data/Field_Boundaries/ReleaseData.7z" {
		t.Fatalf("location = %+v", asset.Location)
	}
	if asset.Cache.CacheKey != "gdrive/field_boundaries/release-data/source.7z" {
		t.Fatalf("cache key = %q", asset.Cache.CacheKey)
	}
	if asset.Archive == nil || asset.Archive.Type != DataAssetArchiveTypeSevenZip {
		t.Fatalf("archive = %+v", asset.Archive)
	}
	gotMembers := archiveMembers(asset.Archive.Select)
	wantMembers := []string{
		"h18v07/WELD_h18v07_2010_field_segments",
		"h18v07/WELD_h18v07_2010_field_segments.hdr",
	}
	if !reflect.DeepEqual(gotMembers, wantMembers) {
		t.Fatalf("archive members = %#v, want %#v", gotMembers, wantMembers)
	}
}

func TestDataInputDefinitionHeaderOnlySelectionPreservesOrder(t *testing.T) {
	definition := yanRoyFieldSegmentsDefinition()

	asset, err := definition.BoundInputAssetWithSelection("yan_roy_field_segments", []string{"header"}, map[string]any{"tile": "h18v07"})
	if err != nil {
		t.Fatalf("BoundInputAssetWithSelection() error = %v", err)
	}

	gotMembers := archiveMembers(asset.Archive.Select)
	wantMembers := []string{"h18v07/WELD_h18v07_2010_field_segments.hdr"}
	if !reflect.DeepEqual(gotMembers, wantMembers) {
		t.Fatalf("archive members = %#v, want %#v", gotMembers, wantMembers)
	}
	if asset.Archive.Select[0].As != "WELD_h18v07_2010_field_segments.hdr" {
		t.Fatalf("archive as = %q", asset.Archive.Select[0].As)
	}
}

func TestDataInputDefinitionRejectsUnknownAndDuplicateSelection(t *testing.T) {
	definition := yanRoyFieldSegmentsDefinition()

	tests := []struct {
		name      string
		selection []string
	}{
		{name: "unknown", selection: []string{"thumbnail"}},
		{name: "duplicate", selection: []string{"header", "header"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := definition.ProviderTemplate("yan_roy_field_segments", test.selection); err == nil {
				t.Fatal("ProviderTemplate() succeeded, want selection error")
			}
		})
	}
}

func TestDataInputDefinitionWorkflowBindingOverridesProjectProviderDetails(t *testing.T) {
	definition := yanRoyFieldSegmentsDefinition()
	definition.Select = []string{"header"}
	definition.Binding.Location.Remote = "shared_drive"
	definition.Binding.Cache.CacheKey = "gdrive/field_boundaries/headers/source.7z"

	asset, err := definition.BoundInputAsset("yan_roy_field_segments", map[string]any{"tile": "h18v07"})
	if err != nil {
		t.Fatalf("BoundInputAsset() error = %v", err)
	}
	if asset.Location.Remote != "shared_drive" {
		t.Fatalf("remote = %q, want workflow override", asset.Location.Remote)
	}
	if asset.Cache.CacheKey != "gdrive/field_boundaries/headers/source.7z" {
		t.Fatalf("cache key = %q, want workflow override", asset.Cache.CacheKey)
	}
	if gotMembers := archiveMembers(asset.Archive.Select); !reflect.DeepEqual(gotMembers, []string{"h18v07/WELD_h18v07_2010_field_segments.hdr"}) {
		t.Fatalf("archive members = %#v, want header only", gotMembers)
	}
}

func TestDataOutputDefinitionBindsGDriveTarget(t *testing.T) {
	definition := DataOutputDefinition{
		Kind:   "archive",
		Format: "zip",
		Binding: DataOutputBindingDefinition{
			Provider: DataProviderGDriveRclone,
			Location: DataDefinitionLocation{
				Remote:    "gdrive",
				DrivePath: "Data/ETL/reports/${asset.year}/report.zip",
			},
			OverwritePolicy: PublishedDataAssetOverwriteFailIfExists,
		},
		Parameters: map[string]DataParameterDefinition{"year": {Type: "int"}},
	}

	target, err := definition.BoundOutputTarget("report_archive", "report_archive", map[string]any{"year": 2026})
	if err != nil {
		t.Fatalf("BoundOutputTarget() error = %v", err)
	}
	if target.Location.Type != DataProviderGDriveRclone || target.Location.Remote != "gdrive" {
		t.Fatalf("location = %+v", target.Location)
	}
	if target.Location.DrivePath != "Data/ETL/reports/2026/report.zip" {
		t.Fatalf("drive path = %q", target.Location.DrivePath)
	}
	if target.OverwritePolicy != PublishedDataAssetOverwriteFailIfExists {
		t.Fatalf("overwrite = %q", target.OverwritePolicy)
	}
}

func yanRoyFieldSegmentsDefinition() DataInputDefinition {
	required := true
	sizeBytes := int64(261861012)
	immutable := true
	return DataInputDefinition{
		Kind: "envi_field_segments",
		Parameters: map[string]DataParameterDefinition{
			"tile": {Type: "string"},
		},
		Files: map[string]DataFileRoleDefinition{
			"raster": {
				Member:   "${asset.tile}/WELD_${asset.tile}_2010_field_segments",
				As:       "WELD_${asset.tile}_2010_field_segments",
				Required: &required,
			},
			"header": {
				Member:   "${asset.tile}/WELD_${asset.tile}_2010_field_segments.hdr",
				As:       "WELD_${asset.tile}_2010_field_segments.hdr",
				Required: &required,
			},
		},
		Select: []string{"raster", "header"},
		Binding: DataInputBindingDefinition{
			ProviderName: "gdrive_release_data",
			Provider:     DataProviderGDriveRclone,
			Location: DataDefinitionLocation{
				Remote:    "gdrive",
				DrivePath: "Data/Field_Boundaries/ReleaseData.7z",
			},
			Archive: DataArchiveBindingDefinition{
				Type:   DataAssetArchiveTypeSevenZip,
				Expose: DataAssetArchiveExposeSelectedDirectory,
			},
			Integrity: DataAssetIntegrityTemplate{
				SizeBytes: &sizeBytes,
				Required:  true,
			},
			Cache: DataDefinitionCache{
				Strategy:  DataAssetCacheStrategyWorkerCache,
				CacheKey:  "gdrive/field_boundaries/release-data/source.7z",
				Immutable: &immutable,
			},
			Materialization: DataDefinitionMaterialization{
				Scope:    "shared",
				Strategy: DataAssetCacheStrategyWorkerCache,
			},
		},
	}
}

func archiveMembers(selectors []DataAssetArchiveSelect) []string {
	members := make([]string, 0, len(selectors))
	for _, selector := range selectors {
		members = append(members, selector.Member)
	}
	return members
}
