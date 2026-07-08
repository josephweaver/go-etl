package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"goetl/internal/model"
)

func TestWorkerCacheDataMaterializesAndReportsManifest(t *testing.T) {
	worker, root := newDataAssetTestWorker(t)
	writeFixture(t, root, "fixture/source.txt", "cache-data fixture")
	hash := sha256Text("cache-data fixture")
	asset := localFileAsset("fixture", "source.txt", model.DataAssetCacheStrategyWorkerCache, "cache/source.txt", &hash)
	payload := model.CacheDataWorkItemPayload{
		Operator:            string(model.WorkItemTypeCacheData),
		TargetEnvironmentID: "target-local",
		AssetKey:            "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		DedupeKey:           "cache_data:target-local:sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		BindingName:         asset.BindingName,
		ProviderName:        asset.ProviderName,
		ProviderType:        asset.Provider,
		Kind:                asset.Kind,
		ResolvedLocation:    asset.Location,
		Cache:               asset.Cache,
		Integrity:           asset.Integrity,
		Parameters:          asset.Parameters,
	}

	item := cacheDataTestItem(payload, asset)
	evidence, err := worker.cacheData(newTestOperationContext(t, worker, item))
	if err != nil {
		t.Fatalf("cacheData() error = %v", err)
	}
	if evidence.OutputJSON == "" || evidence.PreStateJSON == "" || evidence.PostStateJSON == "" {
		t.Fatalf("evidence missing JSON documents: %+v", evidence)
	}

	var manifest model.MaterializedDataAssetManifest
	if err := json.Unmarshal([]byte(evidence.OutputJSON), &manifest); err != nil {
		t.Fatalf("decode output manifest: %v", err)
	}
	if manifest.AssetKey != payload.AssetKey || manifest.TargetEnvironmentID != payload.TargetEnvironmentID {
		t.Fatalf("manifest identity = %+v, want asset_key and target", manifest)
	}
	if len(manifest.Assets) != 1 {
		t.Fatalf("manifest asset count = %d, want 1", len(manifest.Assets))
	}
	if manifest.Assets[0].CacheKey != "cache/source.txt" || manifest.Assets[0].SourceSHA256 != hash {
		t.Fatalf("manifest asset = %+v, want cached source evidence", manifest.Assets[0])
	}
}

func TestWorkerCacheDataFailsOnConflictingImmutableCacheEvidence(t *testing.T) {
	worker, root := newDataAssetTestWorker(t)
	writeFixture(t, root, "fixture/source.txt", "new fixture")
	asset := localFileAsset("fixture", "source.txt", model.DataAssetCacheStrategyWorkerCache, "cache/conflict.txt", nil)
	cacheDir := filepath.Join(worker.Config.effectiveAssetCacheDir(), "cache", "conflict.txt")
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cacheDir, "source"), []byte("different cached bytes"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := writeCacheManifest(filepath.Join(cacheDir, "manifest.json"), workerCacheManifest{
		Schema:          workerCacheManifestSchemaV1,
		CacheKey:        "cache/conflict.txt",
		BindingName:     asset.BindingName,
		ProviderName:    asset.ProviderName,
		ProviderType:    asset.Provider,
		SourceSizeBytes: int64(len("new fixture")),
		SourceSHA256:    sha256Text("new fixture"),
		Immutable:       true,
		WrittenAt:       "2026-07-07T00:00:00Z",
	}); err != nil {
		t.Fatal(err)
	}
	payload := model.CacheDataWorkItemPayload{
		Operator:            string(model.WorkItemTypeCacheData),
		TargetEnvironmentID: "target-local",
		AssetKey:            "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		DedupeKey:           "cache_data:target-local:sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		BindingName:         asset.BindingName,
		ProviderName:        asset.ProviderName,
		ProviderType:        asset.Provider,
		Kind:                asset.Kind,
		ResolvedLocation:    asset.Location,
		Cache:               asset.Cache,
		Integrity:           asset.Integrity,
	}

	item := cacheDataTestItem(payload, asset)
	_, err := worker.cacheData(newTestOperationContext(t, worker, item))
	if err == nil || !strings.Contains(err.Error(), "asset cache source does not match manifest") {
		t.Fatalf("cacheData() error = %v, want conflicting cache evidence", err)
	}
}

func cacheDataTestItem(payload model.CacheDataWorkItemPayload, asset model.BoundDataAsset) model.WorkItem {
	return model.WorkItem{
		ID:             "cache-data-test",
		AttemptID:      "attempt-cache-data-test",
		Type:           model.WorkItemTypeCacheData,
		OutputFilename: "cache-data-test.json",
		Parameters: model.Parameters{
			"cache_data":            {Type: "cache_data", Value: payload},
			"data_assets":           {Type: "data_assets", Value: []model.BoundDataAsset{asset}},
			"target_environment_id": {Type: "string", Value: payload.TargetEnvironmentID},
		},
	}
}
