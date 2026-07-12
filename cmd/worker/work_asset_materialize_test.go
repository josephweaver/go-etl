package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"goetl/internal/model"
)

func TestWorkerAssetMaterializeMaterializesAndReportsManifest(t *testing.T) {
	worker, root := newDataAssetTestWorker(t)
	writeFixture(t, root, "fixture/source.txt", "asset-materialize fixture")
	hash := sha256Text("asset-materialize fixture")
	asset := localFileAsset("fixture", "source.txt", model.DataAssetCacheStrategyWorkerCache, "cache/source.txt", &hash)
	payload := model.AssetMaterializeWorkItemPayload{
		Operator:            string(model.WorkItemTypeAssetMaterialize),
		TargetEnvironmentID: "target-local",
		AssetKey:            "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		DedupeKey:           "asset_materialize:target-local:sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		BindingName:         asset.BindingName,
		ProviderName:        asset.ProviderName,
		ProviderType:        asset.Provider,
		Kind:                asset.Kind,
		ResolvedLocation:    asset.Location,
		Cache:               asset.Cache,
		Integrity:           asset.Integrity,
		Parameters:          asset.Parameters,
	}

	item := AssetMaterializeTestItem(payload, asset)
	evidence, err := worker.AssetMaterialize(newTestOperationContext(t, worker, item))
	if err != nil {
		t.Fatalf("AssetMaterialize() error = %v", err)
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

func TestWorkerAssetMaterializeFailsOnConflictingImmutableCacheEvidence(t *testing.T) {
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
	payload := model.AssetMaterializeWorkItemPayload{
		Operator:            string(model.WorkItemTypeAssetMaterialize),
		TargetEnvironmentID: "target-local",
		AssetKey:            "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		DedupeKey:           "asset_materialize:target-local:sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		BindingName:         asset.BindingName,
		ProviderName:        asset.ProviderName,
		ProviderType:        asset.Provider,
		Kind:                asset.Kind,
		ResolvedLocation:    asset.Location,
		Cache:               asset.Cache,
		Integrity:           asset.Integrity,
	}

	item := AssetMaterializeTestItem(payload, asset)
	_, err := worker.AssetMaterialize(newTestOperationContext(t, worker, item))
	if err == nil || !strings.Contains(err.Error(), "asset cache source does not match manifest") {
		t.Fatalf("AssetMaterialize() error = %v, want conflicting cache evidence", err)
	}
}

func TestWorkerAssetMaterializePromotesToDeterministicDestination(t *testing.T) {
	worker, root := newDataAssetTestWorker(t)
	writeFixture(t, root, "fixture/source.txt", "destination fixture")
	hash := sha256Text("destination fixture")
	asset := localFileAsset("fixture", "source.txt", model.DataAssetCacheStrategyWorkerCache, "cache/destination-source.txt", &hash)
	payload := assetMaterializePayloadForTest(asset)
	payload.MaterializationDomainID = "target-local"
	payload.DestinationRelativePath = "cdl/2017.txt"
	payload.MaterializationKey = "sha256:" + sha256Text("cdl/2017.txt")
	payload.CollectionMember = collectionMemberForTest("cdl/2017.txt")

	evidence, err := worker.AssetMaterialize(newTestOperationContext(t, worker, AssetMaterializeTestItem(payload, asset)))
	if err != nil {
		t.Fatalf("AssetMaterialize() error = %v", err)
	}
	manifest := decodeAssetMaterializeOutput(t, evidence.OutputJSON)
	materialized := manifest.Assets[0]
	wantDestination := filepath.Join(worker.Config.effectiveAssetCacheDir(), "cdl", "2017.txt")
	if materialized.LocalPath != wantDestination {
		t.Fatalf("local path = %q, want destination %q", materialized.LocalPath, wantDestination)
	}
	if got := readString(t, wantDestination); got != "destination fixture" {
		t.Fatalf("destination contents = %q", got)
	}
	if materialized.DestinationRelativePath != payload.DestinationRelativePath ||
		materialized.MaterializationKey != payload.MaterializationKey ||
		materialized.MaterializationDomainID != payload.MaterializationDomainID {
		t.Fatalf("destination metadata = %+v, want payload destination metadata", materialized)
	}
	if materialized.DestinationSizeBytes == nil || *materialized.DestinationSizeBytes != int64(len("destination fixture")) {
		t.Fatalf("destination size = %v", materialized.DestinationSizeBytes)
	}
	if materialized.DestinationSHA256 != hash {
		t.Fatalf("destination sha256 = %q, want %q", materialized.DestinationSHA256, hash)
	}
	if materialized.CollectionMember == nil || materialized.CollectionMember.MemberBindings["year"] != 2017 {
		t.Fatalf("collection member = %+v", materialized.CollectionMember)
	}

	destinationManifest := readDestinationManifestForTest(t, worker, payload.MaterializationKey)
	if destinationManifest.AssetKey != payload.AssetKey || destinationManifest.SHA256 != hash {
		t.Fatalf("destination manifest = %+v", destinationManifest)
	}
	cachedSource := filepath.Join(worker.Config.effectiveAssetCacheDir(), "cache", "destination-source.txt", "source")
	if got := readString(t, cachedSource); got != "destination fixture" {
		t.Fatalf("source cache contents = %q", got)
	}
}

func TestWorkerAssetMaterializeReusesPinnedDestinationWithoutProviderAcquisition(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		_, _ = w.Write([]byte("http destination fixture"))
	}))
	t.Cleanup(server.Close)

	worker, _ := newDataAssetTestWorker(t)
	asset := httpAsset(server.URL, "http/destination-source.txt", nil, nil)
	payload := assetMaterializePayloadForTest(asset)
	payload.MaterializationDomainID = "target-local"
	payload.DestinationRelativePath = "http/fixture.txt"
	payload.MaterializationKey = "sha256:" + sha256Text("http/fixture.txt")

	item := AssetMaterializeTestItem(payload, asset)
	if _, err := worker.AssetMaterialize(newTestOperationContext(t, worker, item)); err != nil {
		t.Fatalf("first AssetMaterialize() error = %v", err)
	}
	if requests != 1 {
		t.Fatalf("HTTP requests after first run = %d, want 1", requests)
	}
	evidence, err := worker.AssetMaterialize(newTestOperationContext(t, worker, item))
	if err != nil {
		t.Fatalf("second AssetMaterialize() error = %v", err)
	}
	if requests != 1 {
		t.Fatalf("HTTP requests after pinned destination reuse = %d, want 1", requests)
	}
	manifest := decodeAssetMaterializeOutput(t, evidence.OutputJSON)
	if manifest.Assets[0].LocalPath != filepath.Join(worker.Config.effectiveAssetCacheDir(), "http", "fixture.txt") {
		t.Fatalf("reused local path = %q", manifest.Assets[0].LocalPath)
	}
}

func TestWorkerAssetMaterializeFailsWhenDestinationManifestMissing(t *testing.T) {
	worker, root := newDataAssetTestWorker(t)
	writeFixture(t, root, "fixture/source.txt", "missing manifest source")
	asset := localFileAsset("fixture", "source.txt", model.DataAssetCacheStrategyWorkerCache, "cache/missing-manifest.txt", nil)
	payload := assetMaterializePayloadForTest(asset)
	payload.MaterializationDomainID = "target-local"
	payload.DestinationRelativePath = "missing/manifest.txt"
	payload.MaterializationKey = "sha256:" + sha256Text("missing/manifest.txt")

	destination := filepath.Join(worker.Config.effectiveAssetCacheDir(), "missing", "manifest.txt")
	if err := os.MkdirAll(filepath.Dir(destination), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(destination, []byte("orphan destination"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := worker.AssetMaterialize(newTestOperationContext(t, worker, AssetMaterializeTestItem(payload, asset)))
	if err == nil || !strings.Contains(err.Error(), "must both exist or both be absent") {
		t.Fatalf("AssetMaterialize() error = %v, want missing destination manifest error", err)
	}
}

func TestWorkerAssetMaterializeFailsWhenDestinationDiffersFromPinnedManifest(t *testing.T) {
	worker, root := newDataAssetTestWorker(t)
	writeFixture(t, root, "fixture/source.txt", "pinned fixture")
	asset := localFileAsset("fixture", "source.txt", model.DataAssetCacheStrategyWorkerCache, "cache/pinned-source.txt", nil)
	payload := assetMaterializePayloadForTest(asset)
	payload.MaterializationDomainID = "target-local"
	payload.DestinationRelativePath = "pinned/fixture.txt"
	payload.MaterializationKey = "sha256:" + sha256Text("pinned/fixture.txt")
	item := AssetMaterializeTestItem(payload, asset)

	if _, err := worker.AssetMaterialize(newTestOperationContext(t, worker, item)); err != nil {
		t.Fatalf("first AssetMaterialize() error = %v", err)
	}
	destination := filepath.Join(worker.Config.effectiveAssetCacheDir(), "pinned", "fixture.txt")
	if err := os.WriteFile(destination, []byte("changed after manifest"), 0644); err != nil {
		t.Fatal(err)
	}
	_, err := worker.AssetMaterialize(newTestOperationContext(t, worker, item))
	if err == nil || !strings.Contains(err.Error(), "does not match pinned manifest") {
		t.Fatalf("AssetMaterialize() error = %v, want pinned manifest mismatch", err)
	}
}

func TestWorkerAssetMaterializeFailsWhenDestinationPinnedToDifferentMaterializationKey(t *testing.T) {
	worker, root := newDataAssetTestWorker(t)
	writeFixture(t, root, "fixture/source.txt", "pinned key fixture")
	asset := localFileAsset("fixture", "source.txt", model.DataAssetCacheStrategyWorkerCache, "cache/pinned-key-source.txt", nil)
	payload := assetMaterializePayloadForTest(asset)
	payload.MaterializationDomainID = "target-local"
	payload.DestinationRelativePath = "pinned/key.txt"
	payload.MaterializationKey = "sha256:" + sha256Text("pinned/key.txt")
	if _, err := worker.AssetMaterialize(newTestOperationContext(t, worker, AssetMaterializeTestItem(payload, asset))); err != nil {
		t.Fatalf("first AssetMaterialize() error = %v", err)
	}

	payload.MaterializationKey = "sha256:" + sha256Text("pinned/key-other.txt")
	_, err := worker.AssetMaterialize(newTestOperationContext(t, worker, AssetMaterializeTestItem(payload, asset)))
	if err == nil || !strings.Contains(err.Error(), "must both exist or both be absent") {
		t.Fatalf("AssetMaterialize() error = %v, want different materialization key to require matching manifest", err)
	}
}

func TestWorkerAssetMaterializePromotesZipSelectedPathToDestination(t *testing.T) {
	worker, root := newDataAssetTestWorker(t)
	writeZipFixture(t, root, "fixture/source.zip", map[string]string{
		"2023_30m_cdls.tif": "tiny raster",
	})
	asset := localFileAsset("fixture", "source.zip", model.DataAssetCacheStrategyWorkerCache, "cache/source.zip", nil)
	asset.Kind = "raster"
	asset.Format = "tif"
	asset.Archive = &model.DataAssetArchive{
		Type:   model.DataAssetArchiveTypeZip,
		Select: []model.DataAssetArchiveSelect{{Member: "2023_30m_cdls.tif", As: "cdl.tif"}},
		Expose: model.DataAssetArchiveExposeSelectedPath,
	}
	payload := assetMaterializePayloadForTest(asset)
	payload.MaterializationDomainID = "target-local"
	payload.DestinationRelativePath = "archive/cdl.tif"
	payload.MaterializationKey = "sha256:" + sha256Text("archive/cdl.tif")

	evidence, err := worker.AssetMaterialize(newTestOperationContext(t, worker, AssetMaterializeTestItem(payload, asset)))
	if err != nil {
		t.Fatalf("AssetMaterialize() error = %v", err)
	}
	manifest := decodeAssetMaterializeOutput(t, evidence.OutputJSON)
	materialized := manifest.Assets[0]
	if materialized.LocalPath != filepath.Join(worker.Config.effectiveAssetCacheDir(), "archive", "cdl.tif") {
		t.Fatalf("local path = %q", materialized.LocalPath)
	}
	if got := readString(t, materialized.LocalPath); got != "tiny raster" {
		t.Fatalf("destination contents = %q", got)
	}
	if len(materialized.ArchiveMembers) != 1 || materialized.ArchiveMembers[0].Member != "2023_30m_cdls.tif" {
		t.Fatalf("archive members = %+v", materialized.ArchiveMembers)
	}
	if materialized.SelectedSHA256 != sha256Text("tiny raster") || materialized.DestinationSHA256 != sha256Text("tiny raster") {
		t.Fatalf("selected/destination hashes = %q/%q", materialized.SelectedSHA256, materialized.DestinationSHA256)
	}
	if materialized.SourceSHA256 == materialized.DestinationSHA256 {
		t.Fatalf("source and destination evidence should remain separate: %+v", materialized)
	}
}

func AssetMaterializeTestItem(payload model.AssetMaterializeWorkItemPayload, asset model.BoundDataAsset) model.WorkItem {
	return model.WorkItem{
		ID:             "asset-materialize-test",
		AttemptID:      "attempt-asset-materialize-test",
		Type:           model.WorkItemTypeAssetMaterialize,
		OutputFilename: "asset-materialize-test.json",
		Parameters: model.Parameters{
			"asset_materialize":     {Type: "asset_materialize", Value: payload},
			"data_assets":           {Type: "data_assets", Value: []model.BoundDataAsset{asset}},
			"target_environment_id": {Type: "string", Value: payload.TargetEnvironmentID},
		},
	}
}

func assetMaterializePayloadForTest(asset model.BoundDataAsset) model.AssetMaterializeWorkItemPayload {
	assetKey := "sha256:" + sha256Text(asset.BindingName+":"+asset.Cache.CacheKey+":"+asset.Location.URI+":"+asset.Location.Path)
	return model.AssetMaterializeWorkItemPayload{
		Operator:            string(model.WorkItemTypeAssetMaterialize),
		TargetEnvironmentID: "target-local",
		AssetKey:            assetKey,
		DedupeKey:           "asset_materialize:target-local:" + assetKey,
		BindingName:         asset.BindingName,
		ProviderName:        asset.ProviderName,
		ProviderType:        asset.Provider,
		Kind:                asset.Kind,
		Format:              asset.Format,
		ResolvedLocation:    asset.Location,
		Cache:               asset.Cache,
		Integrity:           asset.Integrity,
		Archive:             asset.Archive,
		TransferPolicy:      asset.TransferPolicy,
		Parameters:          asset.Parameters,
	}
}

func collectionMemberForTest(destination string) *model.MaterializedDataAssetCollectionMember {
	return &model.MaterializedDataAssetCollectionMember{
		CollectionFingerprint:   "sha256:" + sha256Text("collection"),
		MemberIndex:             0,
		MemberCount:             1,
		DimensionOrder:          []string{"year"},
		MemberBindings:          map[string]any{"year": 2017},
		DestinationRelativePath: destination,
		PathTemplateIdentity:    "sha256:" + sha256Text("template"),
	}
}

func decodeAssetMaterializeOutput(t *testing.T, output string) model.MaterializedDataAssetManifest {
	t.Helper()
	var manifest model.MaterializedDataAssetManifest
	if err := json.Unmarshal([]byte(output), &manifest); err != nil {
		t.Fatalf("decode output manifest: %v", err)
	}
	if err := manifest.Validate(); err != nil {
		t.Fatalf("validate output manifest: %v", err)
	}
	if len(manifest.Assets) != 1 {
		t.Fatalf("asset count = %d, want 1", len(manifest.Assets))
	}
	return manifest
}

func readDestinationManifestForTest(t *testing.T, worker Worker, materializationKey string) materializedAssetDestinationManifest {
	t.Helper()
	path := filepath.Join(
		worker.Config.effectiveAssetCacheDir(),
		".goet",
		"materialized-destinations",
		strings.TrimPrefix(materializationKey, "sha256:")+".json",
	)
	manifest, err := readDestinationManifest(path)
	if err != nil {
		t.Fatalf("read destination manifest: %v", err)
	}
	return manifest
}
