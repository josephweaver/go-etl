package model

import (
	"strings"
	"testing"
)

func TestPublishedDataAssetManifestValidateGDriveRcloneAsset(t *testing.T) {
	size := int64(12)
	manifest := PublishedDataAssetManifest{
		Schema:              PublishedDataAssetManifestSchemaV1,
		TargetEnvironmentID: "hpcc",
		PublishedAssets: []PublishedDataAsset{
			{
				Name:            "publish_archive",
				FromArtifact:    "archive",
				StorageScope:    DataProviderGDriveRclone,
				LocationName:    "gdrive",
				Path:            "Data/ETL/test/goetl-publish-smoke.zip",
				SizeBytes:       &size,
				SHA256:          strings.Repeat("a", 64),
				OverwritePolicy: PublishedDataAssetOverwriteFailIfExists,
			},
		},
	}
	if err := manifest.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}
