package model

import "testing"

func TestMaterializedDataProjectionsExposeHeaderOnlyPath(t *testing.T) {
	size := int64(976)
	manifest := MaterializedDataAssetManifest{
		Schema:              MaterializedDataAssetManifestSchemaV1,
		AssetKey:            "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		TargetEnvironmentID: "msu-hpcc",
		Assets: []MaterializedDataAsset{{
			BindingName:  "field_segments",
			ProviderType: DataProviderLocalFile,
			Kind:         "envi_field_segments",
			LocalPath:    "/shared/cache/h18v07",
			ArchiveMembers: []MaterializedArchiveMember{{
				Member:    "header",
				LocalPath: "/shared/cache/h18v07/WELD_h18v07_2010_field_segments.hdr",
				SizeBytes: &size,
				SHA256:    "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			}},
		}},
	}

	projections, err := MaterializedDataProjections(manifest)
	if err != nil {
		t.Fatalf("MaterializedDataProjections() error = %v", err)
	}
	projection := projections["field_segments"]
	if len(projection.Path) != 1 {
		t.Fatalf("path = %+v, want one header path", projection.Path)
	}
	if projection.Path[0] != "/shared/cache/h18v07/WELD_h18v07_2010_field_segments.hdr" {
		t.Fatalf("path[0] = %q", projection.Path[0])
	}
	if projection.Files["header"].Path != projection.Path[0] {
		t.Fatalf("header path = %q, want positional path %q", projection.Files["header"].Path, projection.Path[0])
	}
}

func TestMaterializedDataProjectionsPreserveArchiveMemberOrder(t *testing.T) {
	manifest := MaterializedDataAssetManifest{
		Schema:              MaterializedDataAssetManifestSchemaV1,
		TargetEnvironmentID: "msu-hpcc",
		Assets: []MaterializedDataAsset{{
			BindingName:  "field_segments",
			ProviderType: DataProviderLocalFile,
			Kind:         "envi_field_segments",
			LocalPath:    "/shared/cache/h18v07",
			ArchiveMembers: []MaterializedArchiveMember{
				{Member: "raster", LocalPath: "/shared/cache/h18v07/WELD_h18v07_2010_field_segments"},
				{Member: "header", LocalPath: "/shared/cache/h18v07/WELD_h18v07_2010_field_segments.hdr"},
			},
		}},
	}

	projections, err := MaterializedDataProjections(manifest)
	if err != nil {
		t.Fatalf("MaterializedDataProjections() error = %v", err)
	}
	path := projections["field_segments"].Path
	if len(path) != 2 {
		t.Fatalf("path = %+v, want raster and header", path)
	}
	if path[0] != "/shared/cache/h18v07/WELD_h18v07_2010_field_segments" || path[1] != "/shared/cache/h18v07/WELD_h18v07_2010_field_segments.hdr" {
		t.Fatalf("path order = %+v", path)
	}
}
