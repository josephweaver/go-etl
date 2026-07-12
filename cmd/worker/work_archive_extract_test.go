package main

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"goetl/internal/model"
)

func TestWorkerArchiveExtractPromotesSelectedZipMember(t *testing.T) {
	worker, root := newDataAssetTestWorker(t)
	writeZipFixture(t, root, "fixture/aqi.zip", map[string]string{
		"annual_aqi_by_county_2024.csv": "county,aqi\n001,42\n",
	})
	sourcePath := filepath.Join(root, "fixture", "aqi.zip")
	item := archiveExtractTestItem(model.ArchiveExtractWorkItemPayload{
		Operator:    string(model.WorkItemTypeArchiveExtract),
		ArchiveType: "zip",
		Source:      model.ArchiveExtractSource{LocalPath: sourcePath},
		Members: []model.ArchiveExtractMember{
			{Member: "annual_aqi_by_county_2024.csv", As: "selected.csv", Required: true},
		},
		OutputPath: "reports/annual_aqi_by_county_2024.csv",
	})

	evidence, err := worker.Run(item)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	manifest := decodeArchiveExtractArtifactManifest(t, evidence.OutputJSON)
	if manifest.WorkItemID != item.ID || manifest.StorageScope != artifactStorageScopeWorkerDataDir {
		t.Fatalf("manifest identity = %+v", manifest)
	}
	if len(manifest.Artifacts) != 1 {
		t.Fatalf("artifact count = %d, want 1", len(manifest.Artifacts))
	}
	artifact := manifest.Artifacts[0]
	if artifact.Name != "reports/annual_aqi_by_county_2024.csv" ||
		artifact.Kind != model.ArtifactKindFile ||
		artifact.Format != "zip_member" {
		t.Fatalf("artifact = %+v", artifact)
	}
	if artifact.SizeBytes == nil || *artifact.SizeBytes != int64(len("county,aqi\n001,42\n")) {
		t.Fatalf("artifact size = %v", artifact.SizeBytes)
	}
	if artifact.SHA256 != sha256Text("county,aqi\n001,42\n") {
		t.Fatalf("artifact sha256 = %q", artifact.SHA256)
	}
	if artifact.Metadata["archive_type"] != "zip" || artifact.Metadata["archive_member"] != "annual_aqi_by_county_2024.csv" {
		t.Fatalf("artifact metadata = %+v", artifact.Metadata)
	}
	promotedPath := filepath.Join(worker.Config.DataDir, filepath.FromSlash(artifact.Path))
	if got := readString(t, promotedPath); got != "county,aqi\n001,42\n" {
		t.Fatalf("promoted contents = %q", got)
	}
}

func TestWorkerArchiveExtractUsesMaterializedDataAssetSource(t *testing.T) {
	worker, root := newDataAssetTestWorker(t)
	writeZipFixture(t, root, "fixture/materialized.zip", map[string]string{
		"table.csv": "id,value\n1,a\n",
	})
	sourcePath := filepath.Join(root, "fixture", "materialized.zip")
	payload := model.ArchiveExtractWorkItemPayload{
		Operator:    string(model.WorkItemTypeArchiveExtract),
		ArchiveType: "zip",
		Source: model.ArchiveExtractSource{
			MaterializedAsset: &model.ArchiveMaterializedAssetSource{
				FromWorkItemID: "materialize-aqi-2024",
				BindingName:    "annual_aqi_zip",
			},
		},
		Members:    []model.ArchiveExtractMember{{Member: "table.csv", As: "table.csv", Required: true}},
		OutputPath: "table.csv",
	}
	item := archiveExtractTestItem(payload)
	item.Parameters["materialized_data_assets"] = model.Parameter{
		Type: "materialized_data_assets",
		Value: model.MaterializedDataAssetManifest{
			Schema: model.MaterializedDataAssetManifestSchemaV1,
			Assets: []model.MaterializedDataAsset{
				{
					BindingName:  "annual_aqi_zip",
					ProviderName: "fixture",
					ProviderType: model.DataProviderLocalFile,
					Kind:         "archive",
					Format:       "zip",
					LocalPath:    sourcePath,
				},
			},
		},
	}

	evidence, err := worker.ArchiveExtract(newTestOperationContext(t, worker, item))
	if err != nil {
		t.Fatalf("ArchiveExtract() error = %v", err)
	}
	manifest := decodeArchiveExtractArtifactManifest(t, evidence.OutputJSON)
	artifact := manifest.Artifacts[0]
	if got := readString(t, filepath.Join(worker.Config.DataDir, filepath.FromSlash(artifact.Path))); got != "id,value\n1,a\n" {
		t.Fatalf("promoted contents = %q", got)
	}
}

func TestWorkerArchiveExtractRejectsMissingParameter(t *testing.T) {
	worker := newTestWorker(t)
	item := model.WorkItem{
		ID:             "archive-extract-missing",
		AttemptID:      "attempt-archive-extract-missing",
		Type:           model.WorkItemTypeArchiveExtract,
		OutputFilename: "archive-extract.json",
	}
	_, err := worker.Run(item)
	if err == nil || !strings.Contains(err.Error(), "archive_extract parameter is required") {
		t.Fatalf("Run() error = %v, want missing archive_extract parameter", err)
	}
}

func TestWorkerArchiveExtractRejectsMultipleMembersInPhaseOne(t *testing.T) {
	worker, root := newDataAssetTestWorker(t)
	writeZipFixture(t, root, "fixture/two.zip", map[string]string{
		"a.csv": "a",
		"b.csv": "b",
	})
	item := archiveExtractTestItem(model.ArchiveExtractWorkItemPayload{
		Operator:    string(model.WorkItemTypeArchiveExtract),
		ArchiveType: "zip",
		Source:      model.ArchiveExtractSource{LocalPath: filepath.Join(root, "fixture", "two.zip")},
		Members: []model.ArchiveExtractMember{
			{Member: "a.csv", As: "a.csv", Required: true},
			{Member: "b.csv", As: "b.csv", Required: true},
		},
		OutputPath: "selected.csv",
	})

	_, err := worker.ArchiveExtract(newTestOperationContext(t, worker, item))
	if err == nil || !strings.Contains(err.Error(), "supports exactly one required selected member") {
		t.Fatalf("ArchiveExtract() error = %v, want phase-one member rejection", err)
	}
}

func TestWorkerArchiveExtractRejectsUnsafeZipEntry(t *testing.T) {
	worker, root := newDataAssetTestWorker(t)
	writeZipFixture(t, root, "fixture/malicious.zip", map[string]string{
		"safe.csv":    "safe",
		"../evil.csv": "evil",
	})
	item := archiveExtractTestItem(model.ArchiveExtractWorkItemPayload{
		Operator:    string(model.WorkItemTypeArchiveExtract),
		ArchiveType: "zip",
		Source:      model.ArchiveExtractSource{LocalPath: filepath.Join(root, "fixture", "malicious.zip")},
		Members:     []model.ArchiveExtractMember{{Member: "safe.csv", As: "safe.csv", Required: true}},
		OutputPath:  "safe.csv",
	})

	_, err := worker.ArchiveExtract(newTestOperationContext(t, worker, item))
	if err == nil || !strings.Contains(err.Error(), "zip entry") {
		t.Fatalf("ArchiveExtract() error = %v, want unsafe zip entry rejection", err)
	}
}

func archiveExtractTestItem(payload model.ArchiveExtractWorkItemPayload) model.WorkItem {
	return model.WorkItem{
		ID:             "archive-extract-test",
		AttemptID:      "attempt-archive-extract-test",
		Type:           model.WorkItemTypeArchiveExtract,
		OutputFilename: "archive-extract.json",
		Parameters: model.Parameters{
			"archive_extract": {Type: "archive_extract", Value: payload},
		},
	}
}

func decodeArchiveExtractArtifactManifest(t *testing.T, outputJSON string) model.ArtifactManifest {
	t.Helper()
	var manifest model.ArtifactManifest
	if err := json.Unmarshal([]byte(outputJSON), &manifest); err != nil {
		t.Fatalf("decode archive_extract output: %v", err)
	}
	if err := manifest.Validate(); err != nil {
		t.Fatalf("validate archive_extract output: %v", err)
	}
	return manifest
}
