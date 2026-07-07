package main

import (
	"encoding/json"
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

func TestCommitDataPublishesFileArtifactAndEmitsEvidence(t *testing.T) {
	worker := newPythonTestWorker(t)
	publishedRoot := t.TempDir()
	worker.Config.DataLocationRoots = map[string]string{"published_data": publishedRoot}

	sourcePath := filepath.Join(worker.Config.DataDir, "artifacts", "raw", "compute-001", "reports", "summary.csv")
	if err := os.MkdirAll(filepath.Dir(sourcePath), 0755); err != nil {
		t.Fatalf("create source parent: %v", err)
	}
	content := "id,value\n1,a\n"
	if err := os.WriteFile(sourcePath, []byte(content), 0644); err != nil {
		t.Fatalf("write source artifact: %v", err)
	}
	sizeBytes := int64(len(content))
	item := testCommitDataItem("reports/summary.csv", model.ArtifactDescriptor{
		Name:        "summary",
		Kind:        model.ArtifactKindFile,
		Path:        "artifacts/raw/compute-001/reports/summary.csv",
		ContentType: "text/csv",
		SizeBytes:   &sizeBytes,
		SHA256:      sha256Text(content),
	})

	evidence, err := worker.commitData(item)
	if err != nil {
		t.Fatalf("commitData() error = %v", err)
	}
	if got := readString(t, filepath.Join(publishedRoot, "reports", "summary.csv")); got != content {
		t.Fatalf("published file = %q", got)
	}
	var manifest model.PublishedDataAssetManifest
	if err := json.Unmarshal([]byte(evidence.OutputJSON), &manifest); err != nil {
		t.Fatalf("decode evidence: %v", err)
	}
	if err := manifest.Validate(); err != nil {
		t.Fatalf("published manifest validate: %v", err)
	}
	if len(manifest.PublishedAssets) != 1 {
		t.Fatalf("published count = %d, want 1", len(manifest.PublishedAssets))
	}
	asset := manifest.PublishedAssets[0]
	if asset.FromWorkItemID != "compute-001" || asset.FromArtifact != "summary" || asset.ContentType != "text/csv" {
		t.Fatalf("published asset source evidence = %+v", asset)
	}
	if asset.SHA256 != sha256Text(content) || asset.SizeBytes == nil || *asset.SizeBytes != sizeBytes {
		t.Fatalf("published asset evidence = %+v, want target bytes", asset)
	}
}

func TestCommitDataRejectsMissingArtifactUnsafePathAndExistingTarget(t *testing.T) {
	worker := newPythonTestWorker(t)
	publishedRoot := t.TempDir()
	worker.Config.DataLocationRoots = map[string]string{"published_data": publishedRoot}

	sourcePath := filepath.Join(worker.Config.DataDir, "artifacts", "raw", "compute-001", "reports", "summary.csv")
	if err := os.MkdirAll(filepath.Dir(sourcePath), 0755); err != nil {
		t.Fatalf("create source parent: %v", err)
	}
	if err := os.WriteFile(sourcePath, []byte("id,value\n1,a\n"), 0644); err != nil {
		t.Fatalf("write source artifact: %v", err)
	}
	descriptor := model.ArtifactDescriptor{Name: "summary", Kind: model.ArtifactKindFile, Path: "artifacts/raw/compute-001/reports/summary.csv"}

	t.Run("missing source artifact", func(t *testing.T) {
		item := testCommitDataItem("reports/missing.csv", descriptor)
		payload := decodeCommitDataParameter(t, item)
		payload.Source.FromArtifact = "missing"
		payload.PublishTarget.FromArtifact = "missing"
		item.Parameters["commit_data"] = model.Parameter{Type: "commit_data", Value: payload}
		if _, err := worker.commitData(item); err == nil || !strings.Contains(err.Error(), "unknown artifact") {
			t.Fatalf("commitData() error = %v, want unknown artifact", err)
		}
	})

	t.Run("unsafe target path", func(t *testing.T) {
		item := testCommitDataItem("../escape.csv", descriptor)
		if _, err := worker.commitData(item); err == nil || !strings.Contains(err.Error(), "path") {
			t.Fatalf("commitData() error = %v, want unsafe path", err)
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
		item := testCommitDataItem("reports/summary.csv", descriptor)
		if _, err := worker.commitData(item); err == nil || !strings.Contains(err.Error(), "already exists") {
			t.Fatalf("commitData() error = %v, want already exists", err)
		}
		if got := readString(t, existingPath); got != "existing" {
			t.Fatalf("existing target changed to %q", got)
		}
	})
}

func testCommitDataItem(targetPath string, artifact model.ArtifactDescriptor) model.WorkItem {
	target := model.BoundPublishTarget{
		Name:            "publish_summary",
		FromArtifact:    "summary",
		TargetName:      "publish_summary",
		Location:        model.DataAssetLocation{Type: model.DataProviderRegisteredLocation, LocationName: "published_data", Path: targetPath},
		OverwritePolicy: model.PublishedDataAssetOverwriteFailIfExists,
	}
	payload := model.CommitDataWorkItemPayload{
		Operator:            string(model.WorkItemTypeCommitData),
		TargetEnvironmentID: "target-local",
		Source:              model.CommitDataSource{FromWorkItemID: "compute-001", FromArtifact: "summary"},
		PublishTarget:       target,
	}
	return model.WorkItem{
		ID:             "commit-001",
		Type:           model.WorkItemTypeCommitData,
		OutputFilename: "commit-001.json",
		DependsOn:      []string{"compute-001"},
		Parameters: model.Parameters{
			"commit_data": {Type: "commit_data", Value: payload},
			"artifact_manifest": {Type: "artifact_manifest", Value: model.ArtifactManifest{
				Schema:       model.ArtifactManifestSchemaV1,
				WorkItemID:   "compute-001",
				StorageScope: artifactStorageScopeWorkerDataDir,
				Artifacts:    []model.ArtifactDescriptor{artifact},
			}},
		},
	}
}

func decodeCommitDataParameter(t *testing.T, item model.WorkItem) model.CommitDataWorkItemPayload {
	t.Helper()
	data, err := json.Marshal(item.Parameters["commit_data"].Value)
	if err != nil {
		t.Fatalf("marshal commit_data parameter: %v", err)
	}
	var payload model.CommitDataWorkItemPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("decode commit_data parameter: %v", err)
	}
	return payload
}

func stringsContains(value string, substr string) bool { return strings.Contains(value, substr) }
