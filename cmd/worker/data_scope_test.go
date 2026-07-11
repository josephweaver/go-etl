package main

import (
	"testing"

	"goetl/internal/model"
	"goetl/internal/variable"
)

func TestDataScopeResolvesMaterializedPathsWithTypes(t *testing.T) {
	manifest := model.MaterializedDataAssetManifest{
		Schema:              model.MaterializedDataAssetManifestSchemaV1,
		TargetEnvironmentID: "msu-hpcc",
		Assets: []model.MaterializedDataAsset{{
			BindingName:  "field_segments",
			ProviderType: model.DataProviderLocalFile,
			Kind:         "envi_field_segments",
			LocalPath:    "/shared/cache/h18v07",
			ArchiveMembers: []model.MaterializedArchiveMember{
				{Member: "raster", LocalPath: "/shared/cache/h18v07/WELD_h18v07_2010_field_segments"},
				{Member: "header", LocalPath: "/shared/cache/h18v07/WELD_h18v07_2010_field_segments.hdr"},
			},
		}},
	}

	scope, err := dataScopeFromMaterializedDataAssets(manifest)
	if err != nil {
		t.Fatalf("dataScopeFromMaterializedDataAssets() error = %v", err)
	}
	resolver := variable.NewResolver(variable.NewSet(scope), variable.ResolverConfig{})

	raster, err := resolver.Resolve(variable.Reference{
		Name:      variable.Name{Namespace: variable.NamespaceData, Key: "field_segments"},
		Qualified: true,
	})
	if err != nil {
		t.Fatalf("Resolve(data.field_segments) error = %v", err)
	}
	firstPath, err := variable.ApplyAccessor(raster, ".path[0]")
	if err != nil {
		t.Fatalf("ApplyAccessor(.path[0]) error = %v", err)
	}
	if firstPath.Type != variable.TypePath || firstPath.Value != "/shared/cache/h18v07/WELD_h18v07_2010_field_segments" {
		t.Fatalf("first path = %+v", firstPath)
	}

	header, err := variable.ApplyAccessor(raster, ".files.header.path")
	if err != nil {
		t.Fatalf("ApplyAccessor(header) error = %v", err)
	}
	if header.Type != variable.TypePath || header.Value != "/shared/cache/h18v07/WELD_h18v07_2010_field_segments.hdr" {
		t.Fatalf("header path = %+v", header)
	}
}
