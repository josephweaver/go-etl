package main

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"goetl/internal/model"
)

func TestMain(m *testing.M) {
	if os.Getenv("GOET_FAKE_7Z") == "1" {
		os.Exit(runFakeSevenZip())
	}
	if os.Getenv("GOET_FAKE_RCLONE") == "1" {
		os.Exit(runFakeRclone())
	}
	os.Exit(m.Run())
}

func TestMaterializeDataAssetsReferencesLocalFileUnderNamedRoot(t *testing.T) {
	worker, root := newDataAssetTestWorker(t)
	writeFixture(t, root, "fixture/input.txt", "local fixture")

	item := dataAssetItem(localFileAsset("fixture", "input.txt", model.DataAssetCacheStrategyReference, "", nil))
	manifestPath, ok, err := worker.materializeDataAssets(item, filepath.Join(worker.Config.TmpDir, "work"))
	if err != nil {
		t.Fatalf("materialize: %v", err)
	}
	if !ok {
		t.Fatal("expected manifest")
	}

	manifest := readMaterializedManifest(t, manifestPath)
	asset := manifest.Assets[0]
	if asset.LocalPath != filepath.Join(root, "fixture", "input.txt") {
		t.Fatalf("local path = %q", asset.LocalPath)
	}
	if asset.MaterializationStrategy != model.DataAssetCacheStrategyReference {
		t.Fatalf("strategy = %q", asset.MaterializationStrategy)
	}
	if asset.SourceSHA256 != sha256Text("local fixture") {
		t.Fatalf("sha256 = %q", asset.SourceSHA256)
	}
	if _, err := os.Stat(filepath.Join(worker.Config.DataDir, "cache", "assets")); !os.IsNotExist(err) {
		t.Fatalf("reference mode should not create worker cache, stat err=%v", err)
	}
}

func TestMaterializeDataAssetsUsesControllerProvidedManifest(t *testing.T) {
	worker, _ := newDataAssetTestWorker(t)
	worker.Config.DataLocationRoots = nil
	localPath := filepath.Join(t.TempDir(), "cache", "input.csv")
	size := int64(12)
	manifest := model.MaterializedDataAssetManifest{
		Schema:              model.MaterializedDataAssetManifestSchemaV1,
		AssetKey:            "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		TargetEnvironmentID: "target-local",
		Assets: []model.MaterializedDataAsset{
			{
				BindingName:             "field_tile_fixture",
				ProviderName:            "field_tile_provider",
				ProviderType:            model.DataProviderLocalFile,
				Kind:                    "fixture_matrix",
				Format:                  "csv",
				LocalPath:               localPath,
				MaterializationStrategy: model.DataAssetCacheStrategyWorkerCache,
				CacheKey:                "fixtures/field_tile.csv",
				SourceSizeBytes:         &size,
				SourceSHA256:            strings.Repeat("a", 64),
			},
		},
	}
	item := model.WorkItem{
		ID: "compute-from-cache",
		Parameters: model.Parameters{
			"materialized_data_assets": {Type: "materialized_data_assets", Value: manifest},
		},
	}

	manifestPath, ok, err := worker.materializeDataAssets(item, filepath.Join(worker.Config.TmpDir, "work"))
	if err != nil {
		t.Fatalf("materializeDataAssets() error = %v", err)
	}
	if !ok {
		t.Fatal("materializeDataAssets() ok = false, want true")
	}
	got := readMaterializedManifest(t, manifestPath)
	if got.AssetKey != manifest.AssetKey || got.TargetEnvironmentID != "target-local" {
		t.Fatalf("manifest identity = %+v, want controller-provided identity", got)
	}
	if len(got.Assets) != 1 || got.Assets[0].LocalPath != localPath {
		t.Fatalf("manifest assets = %+v, want provided local path", got.Assets)
	}
	if _, err := os.Stat(filepath.Join(worker.Config.DataDir, "cache", "assets")); !os.IsNotExist(err) {
		t.Fatalf("controller-provided manifest should not materialize provider data, stat err=%v", err)
	}
}

func TestMaterializeDataAssetsCopiesLocalFileToWorkerCache(t *testing.T) {
	worker, root := newDataAssetTestWorker(t)
	writeFixture(t, root, "fixture/input.txt", "cache me")
	size := int64(len("cache me"))
	hash := sha256Text("cache me")

	item := dataAssetItem(localFileAsset("fixture", "input.txt", model.DataAssetCacheStrategyWorkerCache, "local/cache-key", &hash, &size))
	manifestPath, _, err := worker.materializeDataAssets(item, filepath.Join(worker.Config.TmpDir, "work"))
	if err != nil {
		t.Fatalf("materialize: %v", err)
	}

	cachedPath := filepath.Join(worker.Config.effectiveAssetCacheDir(), "local", "cache-key", "source")
	if data := readString(t, cachedPath); data != "cache me" {
		t.Fatalf("cached data = %q", data)
	}
	if _, err := os.Stat(filepath.Join(worker.Config.effectiveAssetCacheDir(), "local", "cache-key", "manifest.json")); err != nil {
		t.Fatalf("expected cache manifest: %v", err)
	}

	manifest := readMaterializedManifest(t, manifestPath)
	asset := manifest.Assets[0]
	if asset.LocalPath != cachedPath {
		t.Fatalf("local path = %q, want %q", asset.LocalPath, cachedPath)
	}
	if asset.CacheKey != "local/cache-key" || asset.CacheImmutable == nil || !*asset.CacheImmutable {
		t.Fatalf("unexpected cache fields: %+v", asset)
	}
}

func TestMaterializeDataAssetsStreamsHTTPToWorkerCache(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("http fixture"))
	}))
	t.Cleanup(server.Close)

	worker, _ := newDataAssetTestWorker(t)
	asset := httpAsset(server.URL, "http/cache-key", nil, nil)
	item := dataAssetItem(asset)

	manifestPath, _, err := worker.materializeDataAssets(item, filepath.Join(worker.Config.TmpDir, "work"))
	if err != nil {
		t.Fatalf("materialize: %v", err)
	}

	cachedPath := filepath.Join(worker.Config.effectiveAssetCacheDir(), "http", "cache-key", "source")
	if data := readString(t, cachedPath); data != "http fixture" {
		t.Fatalf("cached data = %q", data)
	}
	manifest := readMaterializedManifest(t, manifestPath)
	if manifest.Assets[0].SourceSHA256 != sha256Text("http fixture") {
		t.Fatalf("unexpected source hash: %+v", manifest.Assets[0])
	}
}

func TestMaterializeDataAssetsHTTPHonorsTransferByteRateWithInjectedSleeper(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("1234567890"))
	}))
	t.Cleanup(server.Close)

	worker, _ := newDataAssetTestWorker(t)
	asset := httpAsset(server.URL, "http/throttled", nil, nil)
	asset.TransferPolicy = model.DataAssetTransferPolicy{MaxBytesPerSecond: 5}

	var sleeps []time.Duration
	originalSleep := dataAssetThrottleSleep
	dataAssetThrottleSleep = func(duration time.Duration) {
		sleeps = append(sleeps, duration)
	}
	t.Cleanup(func() {
		dataAssetThrottleSleep = originalSleep
	})

	manifestPath, _, err := worker.materializeDataAssets(dataAssetItem(asset), filepath.Join(worker.Config.TmpDir, "work"))
	if err != nil {
		t.Fatalf("materialize: %v", err)
	}

	if len(sleeps) == 0 {
		t.Fatal("expected throttle sleeper to be called")
	}
	var total time.Duration
	for _, sleep := range sleeps {
		if sleep <= 0 {
			t.Fatalf("sleep duration = %s, want positive", sleep)
		}
		total += sleep
	}
	if total < 2*time.Second {
		t.Fatalf("total sleep = %s, want at least 2s for 10 bytes at 5 B/s", total)
	}
	manifest := readMaterializedManifest(t, manifestPath)
	if manifest.Assets[0].SourceSHA256 != sha256Text("1234567890") {
		t.Fatalf("unexpected source hash: %+v", manifest.Assets[0])
	}
}

func TestMaterializeDataAssetsReferencesRegisteredLocation(t *testing.T) {
	worker, root := newDataAssetTestWorker(t)
	writeFixture(t, root, "registered/table.csv", "id,value\n1,a\n")

	item := dataAssetItem(registeredLocationAsset("registered", "table.csv"))
	manifestPath, _, err := worker.materializeDataAssets(item, filepath.Join(worker.Config.TmpDir, "work"))
	if err != nil {
		t.Fatalf("materialize: %v", err)
	}

	manifest := readMaterializedManifest(t, manifestPath)
	asset := manifest.Assets[0]
	if asset.ProviderType != model.DataProviderRegisteredLocation {
		t.Fatalf("provider type = %q", asset.ProviderType)
	}
	if asset.LocalPath != filepath.Join(root, "registered", "table.csv") {
		t.Fatalf("local path = %q", asset.LocalPath)
	}
}

func TestMaterializeDataAssetsRejectsIntegrityMismatches(t *testing.T) {
	worker, root := newDataAssetTestWorker(t)
	writeFixture(t, root, "fixture/input.txt", "actual")
	wrongHash := sha256Text("different")
	wrongSize := int64(99)

	tests := []struct {
		name  string
		asset model.BoundDataAsset
		want  string
	}{
		{
			name:  "sha256",
			asset: localFileAsset("fixture", "input.txt", model.DataAssetCacheStrategyReference, "", &wrongHash, nil),
			want:  "expected sha256",
		},
		{
			name:  "size",
			asset: localFileAsset("fixture", "input.txt", model.DataAssetCacheStrategyReference, "", nil, &wrongSize),
			want:  "expected size",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := worker.materializeDataAssets(dataAssetItem(tt.asset), filepath.Join(worker.Config.TmpDir, "work-"+tt.name))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("expected %q error, got %v", tt.want, err)
			}
		})
	}
}

func TestMaterializeDataAssetsRejectsUnsafeRelativePaths(t *testing.T) {
	worker, _ := newDataAssetTestWorker(t)
	for _, provider := range []string{model.DataProviderLocalFile, model.DataProviderRegisteredLocation} {
		t.Run(provider, func(t *testing.T) {
			asset := localFileAsset("fixture", "../escape.txt", model.DataAssetCacheStrategyReference, "", nil, nil)
			asset.Provider = provider
			asset.Location.Type = provider
			_, _, err := worker.materializeDataAssets(dataAssetItem(asset), filepath.Join(worker.Config.TmpDir, "work-"+provider))
			if err == nil || !strings.Contains(err.Error(), "data_assets[0]") {
				t.Fatalf("expected unsafe path validation error, got %v", err)
			}
		})
	}
}

func TestMaterializeDataAssetsEnforcesMaxAssetBytes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("too large"))
	}))
	t.Cleanup(server.Close)

	worker, _ := newDataAssetTestWorker(t)
	worker.Config.MaxAssetBytes = 3

	_, _, err := worker.materializeDataAssets(dataAssetItem(httpAsset(server.URL, "http/too-large", nil, nil)), filepath.Join(worker.Config.TmpDir, "work"))
	if err == nil || !strings.Contains(err.Error(), "maximum size") {
		t.Fatalf("expected maximum size error, got %v", err)
	}
}

func TestMaterializeDataAssetsReusesAndVerifiesImmutableCache(t *testing.T) {
	worker, root := newDataAssetTestWorker(t)
	writeFixture(t, root, "fixture/input.txt", "first")

	item := dataAssetItem(localFileAsset("fixture", "input.txt", model.DataAssetCacheStrategyWorkerCache, "immutable/key", nil, nil))
	if _, _, err := worker.materializeDataAssets(item, filepath.Join(worker.Config.TmpDir, "work-first")); err != nil {
		t.Fatalf("first materialize: %v", err)
	}

	writeFixture(t, root, "fixture/input.txt", "changed upstream")
	manifestPath, _, err := worker.materializeDataAssets(item, filepath.Join(worker.Config.TmpDir, "work-second"))
	if err != nil {
		t.Fatalf("cache hit should reuse pinned bytes: %v", err)
	}
	manifest := readMaterializedManifest(t, manifestPath)
	if manifest.Assets[0].SourceSHA256 != sha256Text("first") {
		t.Fatalf("cache hit hash = %q", manifest.Assets[0].SourceSHA256)
	}

	cacheSource := filepath.Join(worker.Config.effectiveAssetCacheDir(), "immutable", "key", "source")
	if err := os.WriteFile(cacheSource, []byte("tampered"), 0644); err != nil {
		t.Fatalf("tamper cache: %v", err)
	}
	if _, _, err := worker.materializeDataAssets(item, filepath.Join(worker.Config.TmpDir, "work-tampered")); err == nil || !strings.Contains(err.Error(), "does not match manifest") {
		t.Fatalf("expected tampered cache error, got %v", err)
	}
}

func TestMaterializeDataAssetsRejectsExpectedHashDifferentFromImmutableCache(t *testing.T) {
	worker, root := newDataAssetTestWorker(t)
	writeFixture(t, root, "fixture/input.txt", "first")
	firstHash := sha256Text("first")

	first := dataAssetItem(localFileAsset("fixture", "input.txt", model.DataAssetCacheStrategyWorkerCache, "immutable/hash-key", &firstHash, nil))
	if _, _, err := worker.materializeDataAssets(first, filepath.Join(worker.Config.TmpDir, "work-first")); err != nil {
		t.Fatalf("first materialize: %v", err)
	}

	secondHash := sha256Text("second")
	second := dataAssetItem(localFileAsset("fixture", "input.txt", model.DataAssetCacheStrategyWorkerCache, "immutable/hash-key", &secondHash, nil))
	if _, _, err := worker.materializeDataAssets(second, filepath.Join(worker.Config.TmpDir, "work-second")); err == nil || !strings.Contains(err.Error(), "expected sha256") {
		t.Fatalf("expected immutable cache hash mismatch, got %v", err)
	}
}

func TestMaterializeDataAssetsExtractsZipSelectedPathFromLocalFile(t *testing.T) {
	worker, root := newDataAssetTestWorker(t)
	writeZipFixture(t, root, "fixture/archive.zip", map[string]string{
		"2023_30m_cdls.tif": "tiny raster",
		"ignored.txt":       "not selected",
	})

	asset := localFileAsset("fixture", "archive.zip", model.DataAssetCacheStrategyReference, "", nil)
	asset.Kind = "raster_archive"
	asset.Format = "geotiff_zip"
	asset.Archive = &model.DataAssetArchive{
		Type:   model.DataAssetArchiveTypeZip,
		Select: []model.DataAssetArchiveSelect{{Member: "2023_30m_cdls.tif", As: "cdl.tif"}},
		Expose: model.DataAssetArchiveExposeSelectedPath,
	}

	manifestPath, _, err := worker.materializeDataAssets(dataAssetItem(asset), filepath.Join(worker.Config.TmpDir, "work"))
	if err != nil {
		t.Fatalf("materialize: %v", err)
	}

	manifest := readMaterializedManifest(t, manifestPath)
	materialized := manifest.Assets[0]
	wantLocalPath := filepath.Join(worker.Config.TmpDir, "work", "data-assets", "input_data", "extracted", "cdl.tif")
	if materialized.LocalPath != wantLocalPath {
		t.Fatalf("local path = %q, want %q", materialized.LocalPath, wantLocalPath)
	}
	if got := readString(t, materialized.LocalPath); got != "tiny raster" {
		t.Fatalf("extracted contents = %q", got)
	}
	if materialized.ArchiveType != model.DataAssetArchiveTypeZip {
		t.Fatalf("archive type = %q", materialized.ArchiveType)
	}
	if len(materialized.ArchiveMembers) != 1 {
		t.Fatalf("archive members = %+v", materialized.ArchiveMembers)
	}
	if materialized.ArchiveMembers[0].Member != "2023_30m_cdls.tif" {
		t.Fatalf("archive member = %+v", materialized.ArchiveMembers[0])
	}
	if materialized.ArchiveMembers[0].SHA256 != sha256Text("tiny raster") {
		t.Fatalf("archive member sha256 = %q", materialized.ArchiveMembers[0].SHA256)
	}
	if materialized.SelectedSHA256 != sha256Text("tiny raster") {
		t.Fatalf("selected sha256 = %q", materialized.SelectedSHA256)
	}
	if materialized.SelectedSizeBytes == nil || *materialized.SelectedSizeBytes != int64(len("tiny raster")) {
		t.Fatalf("selected size = %v", materialized.SelectedSizeBytes)
	}
}

func TestMaterializeDataAssetsExtractsZipSelectedDirectoryFromHTTP(t *testing.T) {
	body := zipBytes(t, map[string]string{
		"tile.hdr":    "header",
		"tile.dat":    "data",
		"ignored.txt": "ignored",
	})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(body)
	}))
	t.Cleanup(server.Close)

	worker, _ := newDataAssetTestWorker(t)
	asset := httpAsset(server.URL, "zip/cache-key", nil, nil)
	asset.Kind = "raster_archive"
	asset.Format = "fixture_zip"
	asset.Archive = &model.DataAssetArchive{
		Type: model.DataAssetArchiveTypeZip,
		Select: []model.DataAssetArchiveSelect{
			{Member: "tile.hdr", As: "tile.hdr"},
			{Member: "tile.dat", As: "tile.dat"},
		},
		Expose: model.DataAssetArchiveExposeSelectedDirectory,
	}

	manifestPath, _, err := worker.materializeDataAssets(dataAssetItem(asset), filepath.Join(worker.Config.TmpDir, "work"))
	if err != nil {
		t.Fatalf("materialize: %v", err)
	}

	manifest := readMaterializedManifest(t, manifestPath)
	materialized := manifest.Assets[0]
	wantDir := filepath.Join(worker.Config.effectiveAssetCacheDir(), "zip", "cache-key", "extracted")
	if materialized.LocalPath != wantDir {
		t.Fatalf("local path = %q, want %q", materialized.LocalPath, wantDir)
	}
	if readString(t, filepath.Join(wantDir, "tile.hdr")) != "header" {
		t.Fatal("missing tile.hdr")
	}
	if readString(t, filepath.Join(wantDir, "tile.dat")) != "data" {
		t.Fatal("missing tile.dat")
	}
	if _, err := os.Stat(filepath.Join(wantDir, "ignored.txt")); !os.IsNotExist(err) {
		t.Fatalf("ignored member should not be extracted, stat err=%v", err)
	}
	if len(materialized.ArchiveMembers) != 2 {
		t.Fatalf("archive members = %+v", materialized.ArchiveMembers)
	}
	evidence, err := directoryManifestEvidence(wantDir)
	if err != nil {
		t.Fatalf("directory evidence: %v", err)
	}
	if materialized.SelectedSHA256 != evidence.sha256 {
		t.Fatalf("directory selected sha256 = %q, want %q", materialized.SelectedSHA256, evidence.sha256)
	}
	if materialized.SelectedSizeBytes == nil || *materialized.SelectedSizeBytes != int64(len("header")+len("data")) {
		t.Fatalf("directory selected size = %v", materialized.SelectedSizeBytes)
	}
}

func TestMaterializeDataAssetsArchiveRequiredAndOptionalMembers(t *testing.T) {
	worker, root := newDataAssetTestWorker(t)
	writeZipFixture(t, root, "fixture/archive.zip", map[string]string{"present.txt": "present"})

	requiredMissing := localFileAsset("fixture", "archive.zip", model.DataAssetCacheStrategyReference, "", nil)
	requiredMissing.Archive = &model.DataAssetArchive{
		Type:   model.DataAssetArchiveTypeZip,
		Select: []model.DataAssetArchiveSelect{{Member: "missing.txt"}},
		Expose: model.DataAssetArchiveExposeSelectedPath,
	}
	if _, _, err := worker.materializeDataAssets(dataAssetItem(requiredMissing), filepath.Join(worker.Config.TmpDir, "work-required")); err == nil || !strings.Contains(err.Error(), "required archive member") {
		t.Fatalf("expected missing required error, got %v", err)
	}

	optional := false
	optionalMissing := localFileAsset("fixture", "archive.zip", model.DataAssetCacheStrategyReference, "", nil)
	optionalMissing.Archive = &model.DataAssetArchive{
		Type: model.DataAssetArchiveTypeZip,
		Select: []model.DataAssetArchiveSelect{
			{Member: "present.txt"},
			{Member: "optional.txt", Required: &optional},
		},
		Expose: model.DataAssetArchiveExposeSelectedDirectory,
	}
	manifestPath, _, err := worker.materializeDataAssets(dataAssetItem(optionalMissing), filepath.Join(worker.Config.TmpDir, "work-optional"))
	if err != nil {
		t.Fatalf("optional materialize: %v", err)
	}
	manifest := readMaterializedManifest(t, manifestPath)
	if len(manifest.Assets[0].ArchiveMembers) != 1 || manifest.Assets[0].ArchiveMembers[0].Member != "present.txt" {
		t.Fatalf("optional missing member should not be reported: %+v", manifest.Assets[0].ArchiveMembers)
	}
}

func TestMaterializeDataAssetsRejectsUnsafeArchiveSelectorsAndZipEntries(t *testing.T) {
	worker, root := newDataAssetTestWorker(t)
	writeZipFixture(t, root, "fixture/archive.zip", map[string]string{"safe.txt": "safe"})
	writeZipFixture(t, root, "fixture/malicious.zip", map[string]string{
		"safe.txt":        "safe",
		"../escape.txt":   "escape",
		"nested\\bad.txt": "bad",
	})

	tests := []struct {
		name    string
		archive *model.DataAssetArchive
		source  string
		want    string
	}{
		{
			name:   "unsafe member",
			source: "archive.zip",
			archive: &model.DataAssetArchive{
				Type:   model.DataAssetArchiveTypeZip,
				Select: []model.DataAssetArchiveSelect{{Member: "../escape.txt"}},
				Expose: model.DataAssetArchiveExposeSelectedPath,
			},
			want: "data_assets[0]",
		},
		{
			name:   "unsafe as",
			source: "archive.zip",
			archive: &model.DataAssetArchive{
				Type:   model.DataAssetArchiveTypeZip,
				Select: []model.DataAssetArchiveSelect{{Member: "safe.txt", As: "/absolute.txt"}},
				Expose: model.DataAssetArchiveExposeSelectedPath,
			},
			want: "data_assets[0]",
		},
		{
			name:   "malicious zip entry",
			source: "malicious.zip",
			archive: &model.DataAssetArchive{
				Type:   model.DataAssetArchiveTypeZip,
				Select: []model.DataAssetArchiveSelect{{Member: "safe.txt", As: "safe.txt"}},
				Expose: model.DataAssetArchiveExposeSelectedPath,
			},
			want: "zip entry",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			asset := localFileAsset("fixture", tt.source, model.DataAssetCacheStrategyReference, "", nil)
			asset.Archive = tt.archive
			_, _, err := worker.materializeDataAssets(dataAssetItem(asset), filepath.Join(worker.Config.TmpDir, "work-"+tt.name))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("expected %q error, got %v", tt.want, err)
			}
		})
	}
}

func TestMaterializeDataAssetsSevenZipRequiresConfiguredExtractor(t *testing.T) {
	worker, root := newDataAssetTestWorker(t)
	writeFixture(t, root, "fixture/release.7z", "not a real archive")

	asset := localFileAsset("fixture", "release.7z", model.DataAssetCacheStrategyReference, "", nil)
	asset.Archive = &model.DataAssetArchive{
		Type:   model.DataAssetArchiveTypeSevenZip,
		Select: []model.DataAssetArchiveSelect{{Member: "tile.hdr", As: "tile.hdr"}},
		Expose: model.DataAssetArchiveExposeSelectedDirectory,
	}
	if _, _, err := worker.materializeDataAssets(dataAssetItem(asset), filepath.Join(worker.Config.TmpDir, "work")); err == nil || !strings.Contains(err.Error(), "seven_zip_executable") {
		t.Fatalf("expected missing seven_zip_executable error, got %v", err)
	}
}

func TestMaterializeDataAssetsSevenZipUsesConfiguredExecutableWithStructuredArgs(t *testing.T) {
	worker, root := newDataAssetTestWorker(t)
	argsPath := filepath.Join(worker.Config.TmpDir, "fake-7z-args.json")
	t.Setenv("GOET_FAKE_7Z", "1")
	t.Setenv("GOET_FAKE_7Z_ARGS_PATH", argsPath)
	worker.Config.SevenZipExecutable = os.Args[0]
	writeFixture(t, root, "fixture/release.7z", "fake archive input")

	asset := localFileAsset("fixture", "release.7z", model.DataAssetCacheStrategyReference, "", nil)
	asset.Archive = &model.DataAssetArchive{
		Type: model.DataAssetArchiveTypeSevenZip,
		Select: []model.DataAssetArchiveSelect{
			{Member: "tiles/tile", As: "tile"},
			{Member: "tiles/tile.hdr", As: "tile.hdr"},
		},
		Expose: model.DataAssetArchiveExposeSelectedDirectory,
	}

	manifestPath, _, err := worker.materializeDataAssets(dataAssetItem(asset), filepath.Join(worker.Config.TmpDir, "work"))
	if err != nil {
		t.Fatalf("materialize: %v", err)
	}
	var args []string
	data, err := os.ReadFile(argsPath)
	if err != nil {
		t.Fatalf("read fake 7z args: %v", err)
	}
	if err := json.Unmarshal(data, &args); err != nil {
		t.Fatalf("decode fake 7z args: %v", err)
	}
	if len(args) != 6 || args[0] != "x" || args[1] != "-y" || !strings.HasPrefix(args[2], "-o") || args[4] != "tiles/tile" || args[5] != "tiles/tile.hdr" {
		t.Fatalf("unexpected fake 7z args: %#v", args)
	}

	manifest := readMaterializedManifest(t, manifestPath)
	selectedPath := filepath.Join(worker.Config.TmpDir, "work", "data-assets", "input_data", "extracted", "tile.hdr")
	if manifest.Assets[0].LocalPath != filepath.Dir(selectedPath) {
		t.Fatalf("local path = %q, want selected directory", manifest.Assets[0].LocalPath)
	}
	if got := readString(t, selectedPath); got != "fake:tiles/tile.hdr" {
		t.Fatalf("selected output = %q", got)
	}
	if got := readString(t, filepath.Join(worker.Config.TmpDir, "work", "data-assets", "input_data", "extracted", "tile")); got != "fake:tiles/tile" {
		t.Fatalf("selected prefix output = %q", got)
	}
}

func newDataAssetTestWorker(t *testing.T) (Worker, string) {
	t.Helper()
	worker := newTestWorker(t)
	root := t.TempDir()
	worker.Config.DataLocationRoots = map[string]string{
		"fixture":    filepath.Join(root, "fixture"),
		"registered": filepath.Join(root, "registered"),
	}
	for _, dir := range worker.Config.DataLocationRoots {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("create data root %s: %v", dir, err)
		}
	}
	return worker, root
}

func dataAssetItem(asset model.BoundDataAsset) model.WorkItem {
	return model.WorkItem{
		ID:             "asset-001",
		Type:           model.WorkItemTypePythonScript,
		OutputFilename: "result.json",
		Source: &model.WorkItemSource{
			RunID:        "run-123",
			ManifestPath: "source-manifest.json",
		},
		Parameters: model.Parameters{
			"data_assets": {Type: "data_assets", Value: []model.BoundDataAsset{asset}},
		},
	}
}

func localFileAsset(rootName string, assetPath string, strategy string, cacheKey string, expectedSHA256 *string, expectedSize ...*int64) model.BoundDataAsset {
	sha := ""
	if expectedSHA256 != nil {
		sha = *expectedSHA256
	}
	var size *int64
	if len(expectedSize) > 0 {
		size = expectedSize[0]
	}
	return model.BoundDataAsset{
		BindingName:  "input_data",
		ProviderName: "fixture_provider",
		Kind:         "fixture",
		Format:       "txt",
		Provider:     model.DataProviderLocalFile,
		Location: model.DataAssetLocation{
			Type:         model.DataProviderLocalFile,
			LocationName: rootName,
			Path:         assetPath,
		},
		Integrity: model.DataAssetIntegrity{SHA256: sha, SizeBytes: size},
		Cache: model.DataAssetCache{
			Strategy: strategy,
			CacheKey: cacheKey,
		},
		Materialization: model.DataAssetMaterialization{Strategy: strategy},
	}
}

func registeredLocationAsset(rootName string, assetPath string) model.BoundDataAsset {
	asset := localFileAsset(rootName, assetPath, model.DataAssetCacheStrategyReference, "", nil, nil)
	asset.Provider = model.DataProviderRegisteredLocation
	asset.Location.Type = model.DataProviderRegisteredLocation
	return asset
}

func httpAsset(uri string, cacheKey string, expectedSHA256 *string, expectedSize *int64) model.BoundDataAsset {
	sha := ""
	if expectedSHA256 != nil {
		sha = *expectedSHA256
	}
	return model.BoundDataAsset{
		BindingName:  "http_data",
		ProviderName: "http_provider",
		Kind:         "fixture",
		Format:       "txt",
		Provider:     model.DataProviderHTTP,
		Location: model.DataAssetLocation{
			Type: model.DataProviderHTTP,
			URI:  uri,
		},
		Integrity:       model.DataAssetIntegrity{SHA256: sha, SizeBytes: expectedSize},
		Cache:           model.DataAssetCache{Strategy: model.DataAssetCacheStrategyWorkerCache, CacheKey: cacheKey},
		Materialization: model.DataAssetMaterialization{Strategy: model.DataAssetCacheStrategyWorkerCache},
	}
}

func writeFixture(t *testing.T, root string, rel string, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("create fixture parent: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write fixture %s: %v", path, err)
	}
}

func writeZipFixture(t *testing.T, root string, rel string, files map[string]string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("create zip fixture parent: %v", err)
	}
	if err := os.WriteFile(path, zipBytes(t, files), 0644); err != nil {
		t.Fatalf("write zip fixture %s: %v", path, err)
	}
}

func zipBytes(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for name, contents := range files {
		file, err := writer.Create(name)
		if err != nil {
			t.Fatalf("create zip entry %s: %v", name, err)
		}
		if _, err := file.Write([]byte(contents)); err != nil {
			t.Fatalf("write zip entry %s: %v", name, err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close zip writer: %v", err)
	}
	return buffer.Bytes()
}

func readString(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

func readMaterializedManifest(t *testing.T, path string) model.MaterializedDataAssetManifest {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	var manifest model.MaterializedDataAssetManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatalf("decode manifest: %v", err)
	}
	if err := manifest.Validate(); err != nil {
		t.Fatalf("validate manifest: %v", err)
	}
	if manifest.Schema != model.MaterializedDataAssetManifestSchemaV1 {
		t.Fatalf("schema = %q", manifest.Schema)
	}
	if len(manifest.Assets) != 1 {
		t.Fatalf("asset count = %d", len(manifest.Assets))
	}
	return manifest
}

func sha256Text(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func runFakeSevenZip() int {
	args := os.Args[1:]
	data, err := json.Marshal(args)
	if err != nil {
		return 2
	}
	if path := os.Getenv("GOET_FAKE_7Z_ARGS_PATH"); path != "" {
		if err := os.WriteFile(path, data, 0644); err != nil {
			return 2
		}
	}

	var root string
	sourceIndex := -1
	for i, arg := range args {
		if strings.HasPrefix(arg, "-o") {
			root = strings.TrimPrefix(arg, "-o")
			sourceIndex = i + 1
			break
		}
	}
	if root == "" || sourceIndex < 0 || sourceIndex >= len(args) {
		return 2
	}
	for _, member := range fakeSevenZipMembers(args[sourceIndex+1:]) {
		path := filepath.Join(root, filepath.FromSlash(member))
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return 2
		}
		if err := os.WriteFile(path, []byte("fake:"+member), 0644); err != nil {
			return 2
		}
	}
	return 0
}

func fakeSevenZipMembers(args []string) []string {
	members := []string{}
	for _, arg := range args {
		if strings.HasPrefix(arg, "-i@") {
			data, err := os.ReadFile(strings.TrimPrefix(arg, "-i@"))
			if err != nil {
				return nil
			}
			for _, line := range strings.Split(string(data), "\n") {
				line = strings.TrimSpace(line)
				if line != "" {
					members = append(members, line)
				}
			}
			continue
		}
		members = append(members, strings.TrimPrefix(arg, "-i!"))
	}
	return members
}
