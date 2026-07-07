package model

import (
	"encoding/json"
	"testing"
)

const validDataSHA256 = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func TestDataLocationValidateAcceptsRegisteredLocation(t *testing.T) {
	location := DataLocation{
		Name:    "fixture_data",
		Type:    DataLocationTypeRegistered,
		Access:  DataLocationAccessReadOnly,
		RootRef: "fixture_data_root",
	}

	if err := location.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestDataProviderTemplateValidateAcceptsSupportedProviders(t *testing.T) {
	tests := []struct {
		name     string
		provider DataProviderTemplate
	}{
		{
			name: "http https url template",
			provider: DataProviderTemplate{
				Name:        "cdl_zip",
				Kind:        "raster_archive",
				Format:      "zip",
				Provider:    DataProviderHTTP,
				URLTemplate: "https://example.invalid/cdl/${year}.zip",
				Parameters:  []string{"year"},
			},
		},
		{
			name: "local file location path template",
			provider: DataProviderTemplate{
				Name:     "manual_release",
				Kind:     "release_archive",
				Provider: DataProviderLocalFile,
				Location: &DataLocationPathTemplate{
					Name:         "fixture_data",
					PathTemplate: "manual/${year}/ReleaseData.7z",
				},
				Parameters: []string{"year"},
			},
		},
		{
			name: "registered location path template",
			provider: DataProviderTemplate{
				Name:     "field_tiles",
				Kind:     "tile_dataset",
				Provider: DataProviderRegisteredLocation,
				Location: &DataLocationPathTemplate{
					Name:         "shared_tiles",
					PathTemplate: "tiles/${tile}/field_segments.hdr",
				},
				Parameters: []string{"tile"},
			},
		},
		{
			name: "gdrive rclone path template",
			provider: DataProviderTemplate{
				Name:       "drive_release",
				Kind:       "release_archive",
				Provider:   DataProviderGDriveRclone,
				Parameters: []string{"year"},
				GDrive: &GDriveRcloneTemplate{
					Remote:       "landcore",
					PathTemplate: "releases/${year}/ReleaseData.7z",
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.provider.Validate(); err != nil {
				t.Fatalf("Validate() error = %v", err)
			}
		})
	}
}

func TestStepDataBindingJSONRoundTrip(t *testing.T) {
	binding := StepDataBinding{
		BindingName:  "cropland_year",
		ProviderName: "cdl_zip",
		Parameters:   map[string]any{"year": 2024},
	}

	var decoded StepDataBinding
	mustRoundTrip(t, binding, &decoded)

	if decoded.BindingName != binding.BindingName || decoded.ProviderName != binding.ProviderName {
		t.Fatalf("decoded binding = %+v, want %+v", decoded, binding)
	}
	if err := decoded.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestDataProviderTemplateBindProducesConcreteBoundAsset(t *testing.T) {
	required := true
	provider := DataProviderTemplate{
		Name:        "cdl_zip",
		Kind:        "raster_archive",
		Format:      "geotiff_zip",
		Provider:    DataProviderHTTP,
		URLTemplate: "https://example.invalid/cdl/${year}.zip",
		Parameters:  []string{"year", "sha"},
		Integrity: DataAssetIntegrityTemplate{
			SHA256Template: "${sha}",
			SizeBytes:      int64Ptr(123),
			Required:       true,
		},
		Cache: DataAssetCacheTemplate{
			Strategy:         DataAssetCacheStrategyWorkerCache,
			CacheKeyTemplate: "cdl/${year}/source.zip",
		},
		Archive: &DataAssetArchiveTemplate{
			Type: DataAssetArchiveTypeZip,
			Select: []DataAssetArchiveSelectTemplate{
				{MemberTemplate: "${year}_30m_cdls.tif", As: "cdl.tif", Required: &required},
			},
			Expose: DataAssetArchiveExposeSelectedPath,
		},
	}

	asset, err := provider.Bind("cropland_year", map[string]any{
		"year": 2024,
		"sha":  validDataSHA256,
	})
	if err != nil {
		t.Fatalf("Bind() error = %v", err)
	}

	if asset.Location.URI != "https://example.invalid/cdl/2024.zip" {
		t.Fatalf("location uri = %q", asset.Location.URI)
	}
	if asset.Cache.CacheKey != "cdl/2024/source.zip" {
		t.Fatalf("cache key = %q", asset.Cache.CacheKey)
	}
	if asset.Integrity.SHA256 != validDataSHA256 {
		t.Fatalf("sha256 = %q", asset.Integrity.SHA256)
	}
	if asset.Archive == nil || asset.Archive.Select[0].Member != "2024_30m_cdls.tif" {
		t.Fatalf("archive = %+v", asset.Archive)
	}
	if !asset.Cache.EffectiveImmutable() {
		t.Fatal("worker_cache should default to immutable")
	}

	if _, err := provider.Bind("cropland_year", map[string]any{"year": 2024}); err == nil {
		t.Fatal("expected missing required parameter to fail")
	}
}

func TestDataProviderTemplateValidateRejectsInvalidCases(t *testing.T) {
	tests := []struct {
		name     string
		provider DataProviderTemplate
	}{
		{
			name: "missing provider name",
			provider: DataProviderTemplate{
				Kind:        "raster_archive",
				Provider:    DataProviderHTTP,
				URLTemplate: "https://example.invalid/cdl.zip",
			},
		},
		{
			name: "missing kind",
			provider: DataProviderTemplate{
				Name:        "cdl_zip",
				Provider:    DataProviderHTTP,
				URLTemplate: "https://example.invalid/cdl.zip",
			},
		},
		{
			name: "missing provider type",
			provider: DataProviderTemplate{
				Name: "cdl_zip",
				Kind: "raster_archive",
			},
		},
		{
			name: "missing url template",
			provider: DataProviderTemplate{
				Name:     "cdl_zip",
				Kind:     "raster_archive",
				Provider: DataProviderHTTP,
			},
		},
		{
			name: "missing path template",
			provider: DataProviderTemplate{
				Name:     "manual_release",
				Kind:     "release_archive",
				Provider: DataProviderLocalFile,
				Location: &DataLocationPathTemplate{Name: "fixture_data"},
			},
		},
		{
			name: "missing rclone remote",
			provider: DataProviderTemplate{
				Name:     "drive_release",
				Kind:     "release_archive",
				Provider: DataProviderGDriveRclone,
				GDrive:   &GDriveRcloneTemplate{PathTemplate: "release.7z"},
			},
		},
		{
			name: "missing rclone path",
			provider: DataProviderTemplate{
				Name:     "drive_release",
				Kind:     "release_archive",
				Provider: DataProviderGDriveRclone,
				GDrive:   &GDriveRcloneTemplate{Remote: "landcore"},
			},
		},
		{
			name: "unsupported provider",
			provider: DataProviderTemplate{
				Name:     "s3_asset",
				Kind:     "object",
				Provider: "s3",
			},
		},
		{
			name: "undeclared parameter reference",
			provider: DataProviderTemplate{
				Name:        "cdl_zip",
				Kind:        "raster_archive",
				Provider:    DataProviderHTTP,
				URLTemplate: "https://example.invalid/${year}.zip",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.provider.Validate(); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

func TestDataAssetValidationRejectsUnsafeAndInvalidFields(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{name: "unsupported location type", err: DataLocation{Name: "fixture", Type: "filesystem"}.Validate()},
		{name: "invalid sha256", err: DataAssetIntegrity{SHA256: "not-a-sha"}.Validate()},
		{name: "unsafe cache key", err: DataAssetCache{Strategy: DataAssetCacheStrategyWorkerCache, CacheKey: "../source.zip"}.Validate()},
		{name: "unsafe location template", err: DataLocationPathTemplate{Name: "fixture", PathTemplate: "C:/source.zip"}.Validate()},
		{name: "unsafe archive member template", err: DataAssetArchiveTemplate{Type: DataAssetArchiveTypeZip, Select: []DataAssetArchiveSelectTemplate{{MemberTemplate: "../source.tif"}}}.Validate()},
		{name: "unsafe archive as path", err: DataAssetArchiveTemplate{Type: DataAssetArchiveTypeZip, Select: []DataAssetArchiveSelectTemplate{{MemberTemplate: "source.tif", As: "../source.tif"}}}.Validate()},
		{name: "unsafe publish path template", err: PublishedDataAssetTarget{Name: "tile", Kind: "table", Location: DataLocationPathTemplate{Name: "published", PathTemplate: "../tile.csv"}}.Validate()},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

func TestDataAssetIntegrityCacheAndArchiveValidate(t *testing.T) {
	integrity := DataAssetIntegrity{SHA256: validDataSHA256, SizeBytes: int64Ptr(42), Required: true}
	if err := integrity.Validate(); err != nil {
		t.Fatalf("integrity Validate() error = %v", err)
	}

	cache := DataAssetCache{Strategy: DataAssetCacheStrategyWorkerCache}
	if !cache.EffectiveImmutable() {
		t.Fatal("worker_cache without immutable should default to immutable")
	}

	required := true
	zipArchive := DataAssetArchiveTemplate{
		Type: DataAssetArchiveTypeZip,
		Select: []DataAssetArchiveSelectTemplate{
			{MemberTemplate: "cdl.tif", Required: &required},
		},
		Expose: DataAssetArchiveExposeSelectedPath,
	}
	if err := zipArchive.Validate(); err != nil {
		t.Fatalf("zip archive Validate() error = %v", err)
	}

	sevenZipArchive := DataAssetArchiveTemplate{
		Type: DataAssetArchiveTypeSevenZip,
		Select: []DataAssetArchiveSelectTemplate{
			{MemberTemplate: "tiles/tile_001.hdr", As: "tile_001.hdr", Required: &required},
		},
		Expose: DataAssetArchiveExposeSelectedDirectory,
	}
	if err := sevenZipArchive.Validate(); err != nil {
		t.Fatalf("seven_zip archive Validate() error = %v", err)
	}
}

func TestPublishedDataAssetTargetAndBindingValidate(t *testing.T) {
	target := PublishedDataAssetTarget{
		Name:       "field_cdl_composition_tile",
		Kind:       "tabular_dataset",
		Format:     "csv",
		Parameters: []string{"year", "tile"},
		Location: DataLocationPathTemplate{
			Name:         "published_data",
			PathTemplate: "field_cdl_composition/year=${year}/tile=${tile}/field_cdl_composition.csv",
		},
	}
	if err := target.Validate(); err != nil {
		t.Fatalf("target Validate() error = %v", err)
	}

	binding, err := target.Bind("publish_composition", "composition_artifact", map[string]any{
		"year": 2024,
		"tile": "tile_001",
	})
	if err != nil {
		t.Fatalf("Bind() error = %v", err)
	}
	if binding.OverwritePolicy != "" {
		t.Fatalf("overwrite policy = %q, want default empty", binding.OverwritePolicy)
	}
	if binding.Location.Path != "field_cdl_composition/year=2024/tile=tile_001/field_cdl_composition.csv" {
		t.Fatalf("location path = %q", binding.Location.Path)
	}

	destructive := target
	destructive.OverwritePolicy = "replace"
	if err := destructive.Validate(); err == nil {
		t.Fatal("expected destructive overwrite policy to fail")
	}
}

func TestMaterializedDataAssetManifestValidateAndRoundTrip(t *testing.T) {
	manifest := MaterializedDataAssetManifest{
		Assets: []MaterializedDataAsset{
			{
				BindingName:             "cropland_year",
				ProviderName:            "cdl_zip",
				ProviderType:            DataProviderHTTP,
				Kind:                    "raster",
				Format:                  "geotiff",
				LocalPath:               "/worker/cache/cdl.tif",
				MaterializationStrategy: DataAssetCacheStrategyWorkerCache,
				CacheKey:                "cdl/2024/source.zip",
				CacheImmutable:          boolPtr(true),
				SourceSizeBytes:         int64Ptr(123),
				SourceSHA256:            validDataSHA256,
				SelectedSizeBytes:       int64Ptr(120),
				SelectedSHA256:          validDataSHA256,
				ArchiveType:             DataAssetArchiveTypeZip,
				ArchiveMembers: []MaterializedArchiveMember{
					{Member: "cdl.tif", LocalPath: "/worker/cache/cdl.tif", SizeBytes: int64Ptr(120), SHA256: validDataSHA256},
				},
			},
		},
	}

	var decoded MaterializedDataAssetManifest
	mustRoundTrip(t, manifest, &decoded)

	if decoded.EffectiveSchema() != MaterializedDataAssetManifestSchemaV1 {
		t.Fatalf("EffectiveSchema() = %q", decoded.EffectiveSchema())
	}
	if err := decoded.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}

	invalidCases := []struct {
		name     string
		manifest MaterializedDataAssetManifest
	}{
		{
			name: "missing binding name",
			manifest: MaterializedDataAssetManifest{Assets: []MaterializedDataAsset{
				{ProviderType: DataProviderHTTP, Kind: "raster", LocalPath: "/worker/cache/cdl.tif"},
			}},
		},
		{
			name: "missing kind",
			manifest: MaterializedDataAssetManifest{Assets: []MaterializedDataAsset{
				{BindingName: "cropland_year", ProviderType: DataProviderHTTP, LocalPath: "/worker/cache/cdl.tif"},
			}},
		},
		{
			name: "missing local path",
			manifest: MaterializedDataAssetManifest{Assets: []MaterializedDataAsset{
				{BindingName: "cropland_year", ProviderType: DataProviderHTTP, Kind: "raster"},
			}},
		},
		{
			name: "missing provider type",
			manifest: MaterializedDataAssetManifest{Assets: []MaterializedDataAsset{
				{BindingName: "cropland_year", Kind: "raster", LocalPath: "/worker/cache/cdl.tif"},
			}},
		},
	}
	for _, test := range invalidCases {
		t.Run(test.name, func(t *testing.T) {
			if err := test.manifest.Validate(); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

func TestDataAssetJSONRoundTripsPreserveContracts(t *testing.T) {
	required := true
	provider := DataProviderTemplate{
		Name:        "cdl_zip",
		Kind:        "raster_archive",
		Format:      "geotiff_zip",
		Provider:    DataProviderHTTP,
		URLTemplate: "https://example.invalid/cdl/${year}.zip",
		Parameters:  []string{"year"},
		Integrity:   DataAssetIntegrityTemplate{SHA256Template: validDataSHA256, SizeBytes: int64Ptr(123)},
		Cache:       DataAssetCacheTemplate{Strategy: DataAssetCacheStrategyWorkerCache, CacheKeyTemplate: "cdl/${year}/source.zip", Immutable: boolPtr(true)},
		Archive: &DataAssetArchiveTemplate{
			Type:   DataAssetArchiveTypeZip,
			Select: []DataAssetArchiveSelectTemplate{{MemberTemplate: "${year}_30m_cdls.tif", As: "cdl.tif", Required: &required}},
			Expose: DataAssetArchiveExposeSelectedPath,
		},
	}
	var decodedProvider DataProviderTemplate
	mustRoundTrip(t, provider, &decodedProvider)
	if err := decodedProvider.Validate(); err != nil {
		t.Fatalf("provider Validate() error = %v", err)
	}

	bound, err := provider.Bind("cropland_year", map[string]any{"year": 2024})
	if err != nil {
		t.Fatalf("Bind() error = %v", err)
	}
	var decodedBound BoundDataAsset
	mustRoundTrip(t, bound, &decodedBound)
	if err := decodedBound.Validate(); err != nil {
		t.Fatalf("bound asset Validate() error = %v", err)
	}

	target := PublishedDataAssetTarget{
		Name:       "composition",
		Kind:       "tabular_dataset",
		Parameters: []string{"year"},
		Location:   DataLocationPathTemplate{Name: "published", PathTemplate: "composition/year=${year}/composition.csv"},
	}
	var decodedTarget PublishedDataAssetTarget
	mustRoundTrip(t, target, &decodedTarget)
	if err := decodedTarget.Validate(); err != nil {
		t.Fatalf("target Validate() error = %v", err)
	}

	publishBinding, err := target.Bind("publish_composition", "composition", map[string]any{"year": 2024})
	if err != nil {
		t.Fatalf("target Bind() error = %v", err)
	}
	var decodedPublish BoundPublishTarget
	mustRoundTrip(t, publishBinding, &decodedPublish)
	if err := decodedPublish.Validate(); err != nil {
		t.Fatalf("publish binding Validate() error = %v", err)
	}

	published := PublishedDataAsset{
		Name:            "composition_publish",
		FromArtifact:    "composition",
		StorageScope:    DataLocationTypeRegistered,
		LocationName:    "published",
		Path:            "composition/year=2024/composition.csv",
		OverwritePolicy: PublishedDataAssetOverwriteFailIfExists,
	}
	var decodedPublished PublishedDataAsset
	mustRoundTrip(t, published, &decodedPublished)
	if err := decodedPublished.Validate(); err != nil {
		t.Fatalf("published data asset Validate() error = %v", err)
	}
}

func mustRoundTrip(t *testing.T, input any, output any) {
	t.Helper()
	data, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := json.Unmarshal(data, output); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
}

func int64Ptr(value int64) *int64 {
	return &value
}

func boolPtr(value bool) *bool {
	return &value
}
