package model

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestMaterializedAssetCollectionManifestValidates(t *testing.T) {
	manifest := validMaterializedAssetCollectionManifest()

	if err := manifest.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}

	var decoded MaterializedAssetCollectionManifest
	mustRoundTrip(t, manifest, &decoded)
	if err := decoded.Validate(); err != nil {
		t.Fatalf("round-trip Validate() error = %v", err)
	}
}

func TestMaterializedAssetCollectionManifestRejectsInvalidCases(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*MaterializedAssetCollectionManifest)
		want   string
	}{
		{
			name: "required binding order differs",
			mutate: func(manifest *MaterializedAssetCollectionManifest) {
				manifest.RequiredBindings = []string{"tile"}
			},
			want: "required_bindings must match dimension_order",
		},
		{
			name: "empty dimension values",
			mutate: func(manifest *MaterializedAssetCollectionManifest) {
				manifest.Dimensions["year"] = MaterializedAssetCollectionDimension{Type: "int"}
			},
			want: "values must not be empty",
		},
		{
			name: "wrong dimension value type",
			mutate: func(manifest *MaterializedAssetCollectionManifest) {
				manifest.Dimensions["year"] = MaterializedAssetCollectionDimension{Type: "int", Values: []any{"2008"}}
			},
			want: "want int",
		},
		{
			name: "member count mismatch",
			mutate: func(manifest *MaterializedAssetCollectionManifest) {
				manifest.MemberCount = 15
			},
			want: "member_count = 15",
		},
		{
			name: "path bindings mismatch",
			mutate: func(manifest *MaterializedAssetCollectionManifest) {
				manifest.Path = "/mnt/cache/cdl/current.tif"
			},
			want: "path bindings",
		},
		{
			name: "unprefixed hash",
			mutate: func(manifest *MaterializedAssetCollectionManifest) {
				manifest.MembersSHA256 = validDataSHA256
			},
			want: "sha256: prefix",
		},
		{
			name: "namespaced path binding",
			mutate: func(manifest *MaterializedAssetCollectionManifest) {
				manifest.Path = "/mnt/cache/cdl/${asset.year}.tif"
			},
			want: "must not be namespace-qualified",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			manifest := validMaterializedAssetCollectionManifest()
			test.mutate(&manifest)
			err := manifest.Validate()
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Validate() error = %v, want containing %q", err, test.want)
			}
		})
	}
}

func TestMaterializedAssetCollectionManifestJSONPreservesIntValues(t *testing.T) {
	data := []byte(`{
		"schema": "goet/materialized-asset-collection/v1",
		"asset": "cdl",
		"materialization_domain_id": "msu-hpcc",
		"dimension_order": ["year"],
		"dimensions": {"year": {"type": "int", "values": [2008, 2009]}},
		"path": "/mnt/cache/cdl/${year}.tif",
		"required_bindings": ["year"],
		"member_count": 2,
		"members_sha256": "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		"collection_fingerprint": "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	}`)
	var manifest MaterializedAssetCollectionManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if _, ok := manifest.Dimensions["year"].Values[0].(int); !ok {
		t.Fatalf("year value type = %T, want int", manifest.Dimensions["year"].Values[0])
	}
	if err := manifest.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestMaterializedDataAssetCollectionMemberRoundTrip(t *testing.T) {
	asset := MaterializedDataAsset{
		BindingName:  "cdl",
		ProviderType: DataProviderHTTP,
		Kind:         "raster",
		LocalPath:    "/mnt/cache/cdl/2008.tif",
		CollectionMember: &MaterializedDataAssetCollectionMember{
			CollectionFingerprint:   "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			MemberIndex:             0,
			MemberCount:             16,
			DimensionOrder:          []string{"year"},
			MemberBindings:          map[string]any{"year": 2008},
			DestinationRelativePath: "cdl/2008.tif",
			PathTemplateIdentity:    "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		},
	}

	var decoded MaterializedDataAsset
	mustRoundTrip(t, asset, &decoded)
	if decoded.CollectionMember == nil {
		t.Fatal("collection member metadata missing after round trip")
	}
	if _, ok := decoded.CollectionMember.MemberBindings["year"].(int); !ok {
		t.Fatalf("year binding type = %T, want int", decoded.CollectionMember.MemberBindings["year"])
	}
	if err := decoded.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func validMaterializedAssetCollectionManifest() MaterializedAssetCollectionManifest {
	return MaterializedAssetCollectionManifest{
		Schema:                  MaterializedAssetCollectionManifestSchemaV1,
		Asset:                   "cdl",
		MaterializationDomainID: "msu-hpcc",
		DimensionOrder:          []string{"year"},
		Dimensions: map[string]MaterializedAssetCollectionDimension{
			"year": {Type: "int", Values: []any{2008, 2009, 2010}},
		},
		Path:                  "/mnt/cache/cdl/${year}.tif",
		RequiredBindings:      []string{"year"},
		MemberCount:           3,
		MembersSHA256:         "sha256:" + validDataSHA256,
		CollectionFingerprint: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	}
}
