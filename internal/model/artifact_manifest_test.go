package model

import "testing"

func TestArtifactManifestValidateAcceptsValidFileArtifact(t *testing.T) {
	sizeBytes := int64(42)
	manifest := ArtifactManifest{
		StorageScope: "attempt",
		Artifacts: []ArtifactDescriptor{
			{
				Name:      "summary",
				Kind:      ArtifactKindFile,
				Path:      "reports/summary.json",
				SizeBytes: &sizeBytes,
				SHA256:    "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			},
		},
	}

	if manifest.EffectiveSchema() != ArtifactManifestSchemaV1 {
		t.Fatalf("EffectiveSchema() = %q, want %q", manifest.EffectiveSchema(), ArtifactManifestSchemaV1)
	}
	if err := manifest.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestArtifactManifestValidateAcceptsValidDirectoryArtifact(t *testing.T) {
	manifest := ArtifactManifest{
		Schema:       ArtifactManifestSchemaV1,
		StorageScope: "worker_data",
		Artifacts: []ArtifactDescriptor{
			{
				Name:           "tile-output",
				Kind:           ArtifactKindDirectory,
				Path:           "tiles/tile-001",
				ManifestSHA256: "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789",
			},
		},
	}

	if err := manifest.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestArtifactManifestValidateAcceptsPublishedAssets(t *testing.T) {
	sizeBytes := int64(18)
	manifest := ArtifactManifest{
		Schema:       ArtifactManifestSchemaV1,
		StorageScope: "worker_data",
		Artifacts: []ArtifactDescriptor{
			{
				Name: "summary",
				Kind: ArtifactKindFile,
				Path: "reports/summary.json",
			},
		},
		PublishedAssets: []PublishedDataAsset{
			{
				Name:            "summary_publish",
				FromArtifact:    "summary",
				StorageScope:    DataLocationTypeRegistered,
				LocationName:    "published_data",
				Path:            "reports/summary.json",
				SizeBytes:       &sizeBytes,
				SHA256:          "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
				OverwritePolicy: PublishedDataAssetOverwriteFailIfExists,
			},
		},
	}

	if err := manifest.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestArtifactManifestValidateRejectsInvalidManifestOrDescriptor(t *testing.T) {
	tests := []struct {
		name     string
		manifest ArtifactManifest
	}{
		{
			name: "no artifacts",
			manifest: ArtifactManifest{
				StorageScope: "attempt",
			},
		},
		{
			name: "empty storage scope",
			manifest: ArtifactManifest{
				Artifacts: []ArtifactDescriptor{
					{Name: "summary", Kind: ArtifactKindFile, Path: "summary.json"},
				},
			},
		},
		{
			name: "empty artifact name",
			manifest: ArtifactManifest{
				StorageScope: "attempt",
				Artifacts: []ArtifactDescriptor{
					{Name: "", Kind: ArtifactKindFile, Path: "summary.json"},
				},
			},
		},
		{
			name: "empty artifact kind",
			manifest: ArtifactManifest{
				StorageScope: "attempt",
				Artifacts: []ArtifactDescriptor{
					{Name: "summary", Kind: "", Path: "summary.json"},
				},
			},
		},
		{
			name: "empty artifact path",
			manifest: ArtifactManifest{
				StorageScope: "attempt",
				Artifacts: []ArtifactDescriptor{
					{Name: "summary", Kind: ArtifactKindFile, Path: ""},
				},
			},
		},
		{
			name: "unsafe artifact path",
			manifest: ArtifactManifest{
				StorageScope: "attempt",
				Artifacts: []ArtifactDescriptor{
					{Name: "summary", Kind: ArtifactKindFile, Path: "C:/summary.json"},
				},
			},
		},
		{
			name: "unsupported kind",
			manifest: ArtifactManifest{
				StorageScope: "attempt",
				Artifacts: []ArtifactDescriptor{
					{Name: "summary", Kind: "symlink", Path: "summary.json"},
				},
			},
		},
		{
			name: "invalid published asset",
			manifest: ArtifactManifest{
				StorageScope: "attempt",
				Artifacts: []ArtifactDescriptor{
					{Name: "summary", Kind: ArtifactKindFile, Path: "summary.json"},
				},
				PublishedAssets: []PublishedDataAsset{
					{
						Name:         "bad publish",
						FromArtifact: "summary",
						StorageScope: DataLocationTypeRegistered,
						LocationName: "published_data",
						Path:         "summary.json",
					},
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.manifest.Validate(); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

func TestPublishedDataAssetManifestValidate(t *testing.T) {
	sizeBytes := int64(13)
	manifest := PublishedDataAssetManifest{
		TargetEnvironmentID: "target-local",
		PublishedAssets: []PublishedDataAsset{
			{
				Name:            "publish_summary",
				FromWorkItemID:  "compute-001",
				FromArtifact:    "summary",
				ContentType:     "text/csv",
				StorageScope:    DataLocationTypeRegistered,
				LocationName:    "published_data",
				Path:            "reports/summary.csv",
				SizeBytes:       &sizeBytes,
				SHA256:          "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
				OverwritePolicy: PublishedDataAssetOverwriteFailIfExists,
			},
		},
	}

	if manifest.EffectiveSchema() != PublishedDataAssetManifestSchemaV1 {
		t.Fatalf("EffectiveSchema() = %q, want %q", manifest.EffectiveSchema(), PublishedDataAssetManifestSchemaV1)
	}
	if err := manifest.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	manifest.TargetEnvironmentID = ""
	if err := manifest.Validate(); err == nil {
		t.Fatal("expected missing target_environment_id error")
	}
}
