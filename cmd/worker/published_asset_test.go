package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"goetl/internal/model"
)

func TestPublishPromotedArtifactsPublishesFileArtifact(t *testing.T) {
	worker := newPythonTestWorker(t)
	publishedRoot := t.TempDir()
	worker.Config.DataLocationRoots = map[string]string{"published_data": publishedRoot}

	sourcePath := filepath.Join(worker.Config.DataDir, "artifacts", "raw", "work-001", "reports", "summary.csv")
	if err := os.MkdirAll(filepath.Dir(sourcePath), 0755); err != nil {
		t.Fatalf("create source parent: %v", err)
	}
	if err := os.WriteFile(sourcePath, []byte("id,value\n1,a\n"), 0644); err != nil {
		t.Fatalf("write source artifact: %v", err)
	}

	item := model.WorkItem{
		ID: "work-001",
		Parameters: model.Parameters{
			"publish": {Type: "publish_targets", Value: []model.BoundPublishTarget{
				{
					Name:            "publish_summary",
					FromArtifact:    "summary",
					TargetName:      "publish_summary",
					Location:        model.DataAssetLocation{Type: model.DataProviderRegisteredLocation, LocationName: "published_data", Path: "reports/summary.csv"},
					OverwritePolicy: model.PublishedDataAssetOverwriteFailIfExists,
				},
			}},
		},
	}
	promoted := model.ArtifactManifest{
		StorageScope: artifactStorageScopeWorkerDataDir,
		Artifacts: []model.ArtifactDescriptor{
			{
				Name: "summary",
				Kind: model.ArtifactKindFile,
				Path: "artifacts/raw/work-001/reports/summary.csv",
			},
		},
	}

	published, err := worker.publishPromotedArtifacts(item, promoted)
	if err != nil {
		t.Fatalf("publishPromotedArtifacts() error = %v", err)
	}
	if len(published) != 1 {
		t.Fatalf("published count = %d", len(published))
	}
	if got := readString(t, filepath.Join(publishedRoot, "reports", "summary.csv")); got != "id,value\n1,a\n" {
		t.Fatalf("published file = %q", got)
	}
	if published[0].SHA256 != sha256Text("id,value\n1,a\n") {
		t.Fatalf("published sha256 = %q", published[0].SHA256)
	}
}

func TestPublishPromotedArtifactsPublishesDirectoryArtifact(t *testing.T) {
	worker := newPythonTestWorker(t)
	publishedRoot := t.TempDir()
	worker.Config.DataLocationRoots = map[string]string{"published_data": publishedRoot}

	sourcePath := filepath.Join(worker.Config.DataDir, "artifacts", "raw", "work-001", "dataset")
	if err := os.MkdirAll(filepath.Join(sourcePath, "nested"), 0755); err != nil {
		t.Fatalf("create source dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sourcePath, "part-b.txt"), []byte("b"), 0644); err != nil {
		t.Fatalf("write source file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sourcePath, "nested", "part-a.txt"), []byte("aa"), 0644); err != nil {
		t.Fatalf("write nested source file: %v", err)
	}

	item := model.WorkItem{
		ID: "work-001",
		Parameters: model.Parameters{
			"publish": {Type: "publish_targets", Value: map[string]any{
				"publish_dataset": map[string]any{
					"from_artifact": "dataset",
					"location": map[string]any{
						"type":          model.DataProviderRegisteredLocation,
						"location_name": "published_data",
						"path":          "dataset/year=2024",
					},
					"overwrite_policy": model.PublishedDataAssetOverwriteFailIfExists,
				},
			}},
		},
	}
	promoted := model.ArtifactManifest{
		StorageScope: artifactStorageScopeWorkerDataDir,
		Artifacts: []model.ArtifactDescriptor{
			{
				Name: "dataset",
				Kind: model.ArtifactKindDirectory,
				Path: "artifacts/raw/work-001/dataset",
			},
		},
	}

	published, err := worker.publishPromotedArtifacts(item, promoted)
	if err != nil {
		t.Fatalf("publishPromotedArtifacts() error = %v", err)
	}
	if len(published) != 1 {
		t.Fatalf("published count = %d", len(published))
	}
	wantPath := filepath.Join(publishedRoot, "dataset", "year=2024")
	evidence, err := directoryManifestEvidence(wantPath)
	if err != nil {
		t.Fatalf("directory evidence: %v", err)
	}
	if published[0].SHA256 != evidence.sha256 {
		t.Fatalf("published sha256 = %q, want %q", published[0].SHA256, evidence.sha256)
	}
}

func TestPublishPromotedArtifactsRejectsUnknownArtifactAndLocation(t *testing.T) {
	worker := newPythonTestWorker(t)
	worker.Config.DataLocationRoots = map[string]string{"published_data": t.TempDir()}

	sourcePath := filepath.Join(worker.Config.DataDir, "artifacts", "raw", "work-001", "reports", "summary.csv")
	if err := os.MkdirAll(filepath.Dir(sourcePath), 0755); err != nil {
		t.Fatalf("create source parent: %v", err)
	}
	if err := os.WriteFile(sourcePath, []byte("id,value\n1,a\n"), 0644); err != nil {
		t.Fatalf("write source artifact: %v", err)
	}

	promoted := model.ArtifactManifest{
		StorageScope: artifactStorageScopeWorkerDataDir,
		Artifacts: []model.ArtifactDescriptor{
			{Name: "summary", Kind: model.ArtifactKindFile, Path: "artifacts/raw/work-001/reports/summary.csv"},
		},
	}

	t.Run("unknown artifact", func(t *testing.T) {
		item := model.WorkItem{
			Parameters: model.Parameters{
				"publish": {Type: "publish_targets", Value: []model.BoundPublishTarget{
					{
						Name:            "publish_summary",
						FromArtifact:    "missing",
						TargetName:      "publish_summary",
						Location:        model.DataAssetLocation{Type: model.DataProviderRegisteredLocation, LocationName: "published_data", Path: "reports/summary.csv"},
						OverwritePolicy: model.PublishedDataAssetOverwriteFailIfExists,
					},
				}},
			},
		}
		if _, err := worker.publishPromotedArtifacts(item, promoted); err == nil || !stringsContains(err.Error(), "unknown artifact") {
			t.Fatalf("expected unknown artifact error, got %v", err)
		}
	})

	t.Run("unknown location", func(t *testing.T) {
		item := model.WorkItem{
			Parameters: model.Parameters{
				"publish": {Type: "publish_targets", Value: []model.BoundPublishTarget{
					{
						Name:            "publish_summary",
						FromArtifact:    "summary",
						TargetName:      "publish_summary",
						Location:        model.DataAssetLocation{Type: model.DataProviderRegisteredLocation, LocationName: "missing_root", Path: "reports/summary.csv"},
						OverwritePolicy: model.PublishedDataAssetOverwriteFailIfExists,
					},
				}},
			},
		}
		if _, err := worker.publishPromotedArtifacts(item, promoted); err == nil || !stringsContains(err.Error(), "publish location root") {
			t.Fatalf("expected unknown location error, got %v", err)
		}
	})
}

func TestPublishPromotedArtifactsRejectsUnsafePathAndFailIfExists(t *testing.T) {
	worker := newPythonTestWorker(t)
	publishedRoot := t.TempDir()
	worker.Config.DataLocationRoots = map[string]string{"published_data": publishedRoot}

	sourcePath := filepath.Join(worker.Config.DataDir, "artifacts", "raw", "work-001", "reports", "summary.csv")
	if err := os.MkdirAll(filepath.Dir(sourcePath), 0755); err != nil {
		t.Fatalf("create source parent: %v", err)
	}
	if err := os.WriteFile(sourcePath, []byte("id,value\n1,a\n"), 0644); err != nil {
		t.Fatalf("write source artifact: %v", err)
	}

	promoted := model.ArtifactManifest{
		StorageScope: artifactStorageScopeWorkerDataDir,
		Artifacts: []model.ArtifactDescriptor{
			{Name: "summary", Kind: model.ArtifactKindFile, Path: "artifacts/raw/work-001/reports/summary.csv"},
		},
	}

	t.Run("unsafe path", func(t *testing.T) {
		item := model.WorkItem{
			Parameters: model.Parameters{
				"publish": {Type: "publish_targets", Value: []model.BoundPublishTarget{
					{
						Name:            "publish_summary",
						FromArtifact:    "summary",
						TargetName:      "publish_summary",
						Location:        model.DataAssetLocation{Type: model.DataProviderRegisteredLocation, LocationName: "published_data", Path: "../escape.csv"},
						OverwritePolicy: model.PublishedDataAssetOverwriteFailIfExists,
					},
				}},
			},
		}
		if _, err := worker.publishPromotedArtifacts(item, promoted); err == nil || !stringsContains(err.Error(), "path") {
			t.Fatalf("expected unsafe path error, got %v", err)
		}
	})

	t.Run("fail if exists", func(t *testing.T) {
		existingPath := filepath.Join(publishedRoot, "reports", "summary.csv")
		if err := os.MkdirAll(filepath.Dir(existingPath), 0755); err != nil {
			t.Fatalf("create existing parent: %v", err)
		}
		if err := os.WriteFile(existingPath, []byte("existing"), 0644); err != nil {
			t.Fatalf("write existing target: %v", err)
		}
		item := model.WorkItem{
			Parameters: model.Parameters{
				"publish": {Type: "publish_targets", Value: []model.BoundPublishTarget{
					{
						Name:            "publish_summary",
						FromArtifact:    "summary",
						TargetName:      "publish_summary",
						Location:        model.DataAssetLocation{Type: model.DataProviderRegisteredLocation, LocationName: "published_data", Path: "reports/summary.csv"},
						OverwritePolicy: model.PublishedDataAssetOverwriteFailIfExists,
					},
				}},
			},
		}
		if _, err := worker.publishPromotedArtifacts(item, promoted); err == nil || !stringsContains(err.Error(), "already exists") {
			t.Fatalf("expected fail_if_exists error, got %v", err)
		}
		if got := readString(t, existingPath); got != "existing" {
			t.Fatalf("existing target changed to %q", got)
		}
	})
}

func stringsContains(value string, substr string) bool { return strings.Contains(value, substr) }
