package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"goetl/internal/model"
)

func TestMaterializeDataAssetsGDriveRcloneRequiresEnablementAndExecutable(t *testing.T) {
	worker, _ := newDataAssetTestWorker(t)
	asset := gdriveRcloneAsset("Risk Model/data.txt", "gdrive/cache-key", nil, nil)

	if _, _, err := worker.materializeDataAssets(dataAssetItem(asset), filepath.Join(worker.Config.TmpDir, "work-disabled")); err == nil || !strings.Contains(err.Error(), "gdrive_rclone provider is disabled") {
		t.Fatalf("expected disabled provider error, got %v", err)
	}

	worker.Config.EnableGDriveRcloneProvider = true
	if _, _, err := worker.materializeDataAssets(dataAssetItem(asset), filepath.Join(worker.Config.TmpDir, "work-missing-exe")); err == nil || !strings.Contains(err.Error(), "rclone_executable") {
		t.Fatalf("expected missing executable error, got %v", err)
	}
}

func TestMaterializeDataAssetsGDriveRcloneCopiesFixtureWithStructuredArgs(t *testing.T) {
	worker, _ := newDataAssetTestWorker(t)
	source := filepath.Join(worker.Config.TmpDir, "fixture with spaces.txt")
	if err := os.WriteFile(source, []byte("drive fixture"), 0644); err != nil {
		t.Fatalf("write source: %v", err)
	}
	argsPath := filepath.Join(worker.Config.TmpDir, "rclone-args.jsonl")
	configPath := filepath.Join(worker.Config.TmpDir, "secret-token-rclone.conf")
	configureFakeRclone(t, &worker, source, argsPath, configPath)

	asset := gdriveRcloneAsset("Risk Model 2021/Data/ReleaseData.7z", "gdrive/landcore/source.7z", nil, nil)
	manifestPath, _, err := worker.materializeDataAssets(dataAssetItem(asset), filepath.Join(worker.Config.TmpDir, "work"))
	if err != nil {
		t.Fatalf("materialize: %v", err)
	}

	cachedPath := filepath.Join(worker.Config.effectiveAssetCacheDir(), "gdrive", "landcore", "source.7z", "source")
	if got := readString(t, cachedPath); got != "drive fixture" {
		t.Fatalf("cached data = %q", got)
	}
	manifest := readMaterializedManifest(t, manifestPath)
	materialized := manifest.Assets[0]
	if materialized.ProviderType != model.DataProviderGDriveRclone {
		t.Fatalf("provider type = %q", materialized.ProviderType)
	}
	if materialized.SourceSHA256 != sha256Text("drive fixture") {
		t.Fatalf("source sha256 = %q", materialized.SourceSHA256)
	}

	calls := readFakeRcloneCalls(t, argsPath)
	if len(calls) != 1 {
		t.Fatalf("rclone call count = %d", len(calls))
	}
	args := calls[0]
	if len(args) != 5 {
		t.Fatalf("unexpected args: %#v", args)
	}
	if args[0] != "--config" || args[1] != configPath || args[2] != "copyto" {
		t.Fatalf("unexpected leading args: %#v", args)
	}
	if args[3] != "landcore:Risk Model 2021/Data/ReleaseData.7z" {
		t.Fatalf("remote path arg = %q", args[3])
	}
	if !strings.HasPrefix(filepath.Base(args[4]), "source.tmp-") {
		t.Fatalf("destination arg should be temporary source path: %#v", args)
	}
}

func TestMaterializeDataAssetsGDriveRcloneValidatesBeforeInvocation(t *testing.T) {
	worker, _ := newDataAssetTestWorker(t)
	argsPath := filepath.Join(worker.Config.TmpDir, "rclone-args.jsonl")
	configureFakeRclone(t, &worker, filepath.Join(worker.Config.TmpDir, "missing-source.txt"), argsPath, "")

	asset := gdriveRcloneAsset("../escape.txt", "gdrive/unsafe", nil, nil)
	if _, _, err := worker.materializeDataAssets(dataAssetItem(asset), filepath.Join(worker.Config.TmpDir, "work")); err == nil || !strings.Contains(err.Error(), "data_assets[0]") {
		t.Fatalf("expected validation error, got %v", err)
	}
	if _, err := os.Stat(argsPath); !os.IsNotExist(err) {
		t.Fatalf("fake rclone should not be invoked, stat err=%v", err)
	}
}

func TestMaterializeDataAssetsGDriveRcloneRejectsIntegrityMismatches(t *testing.T) {
	tests := []struct {
		name string
		hash *string
		size *int64
		want string
	}{
		{name: "sha256", hash: stringPtr(sha256Text("wrong")), want: "expected sha256"},
		{name: "size", size: int64PtrWorker(99), want: "expected size"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			worker, _ := newDataAssetTestWorker(t)
			source := filepath.Join(worker.Config.TmpDir, "source.txt")
			if err := os.WriteFile(source, []byte("actual"), 0644); err != nil {
				t.Fatalf("write source: %v", err)
			}
			configureFakeRclone(t, &worker, source, filepath.Join(worker.Config.TmpDir, "rclone-args.jsonl"), "")
			asset := gdriveRcloneAsset("release/source.txt", "gdrive/integrity-"+tt.name, tt.hash, tt.size)

			_, _, err := worker.materializeDataAssets(dataAssetItem(asset), filepath.Join(worker.Config.TmpDir, "work"))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("expected %q error, got %v", tt.want, err)
			}
			sourcePath := filepath.Join(worker.Config.effectiveAssetCacheDir(), "gdrive", "integrity-"+tt.name, "source")
			if _, statErr := os.Stat(sourcePath); !os.IsNotExist(statErr) {
				t.Fatalf("failed integrity should remove cache source, stat err=%v", statErr)
			}
		})
	}
}

func TestMaterializeDataAssetsGDriveRcloneReusesImmutableCacheWithoutSecondInvocation(t *testing.T) {
	worker, _ := newDataAssetTestWorker(t)
	source := filepath.Join(worker.Config.TmpDir, "source.txt")
	if err := os.WriteFile(source, []byte("first"), 0644); err != nil {
		t.Fatalf("write source: %v", err)
	}
	argsPath := filepath.Join(worker.Config.TmpDir, "rclone-args.jsonl")
	configureFakeRclone(t, &worker, source, argsPath, "")

	item := dataAssetItem(gdriveRcloneAsset("release/source.txt", "gdrive/reuse/source.txt", nil, nil))
	if _, _, err := worker.materializeDataAssets(item, filepath.Join(worker.Config.TmpDir, "work-first")); err != nil {
		t.Fatalf("first materialize: %v", err)
	}
	if err := os.WriteFile(source, []byte("changed upstream"), 0644); err != nil {
		t.Fatalf("change source: %v", err)
	}
	manifestPath, _, err := worker.materializeDataAssets(item, filepath.Join(worker.Config.TmpDir, "work-second"))
	if err != nil {
		t.Fatalf("second materialize: %v", err)
	}

	calls := readFakeRcloneCalls(t, argsPath)
	if len(calls) != 1 {
		t.Fatalf("rclone call count = %d", len(calls))
	}
	manifest := readMaterializedManifest(t, manifestPath)
	if manifest.Assets[0].SourceSHA256 != sha256Text("first") {
		t.Fatalf("cache hash = %q", manifest.Assets[0].SourceSHA256)
	}
}

func TestMaterializeDataAssetsGDriveRcloneFailureRedactsSecretsAndLeavesNoCacheEntry(t *testing.T) {
	worker, _ := newDataAssetTestWorker(t)
	argsPath := filepath.Join(worker.Config.TmpDir, "rclone-args.jsonl")
	configPath := filepath.Join(worker.Config.TmpDir, "private-token-rclone.conf")
	configureFakeRclone(t, &worker, filepath.Join(worker.Config.TmpDir, "source.txt"), argsPath, configPath)
	t.Setenv("GOET_FAKE_RCLONE_FAIL", "1")
	t.Setenv("GOET_FAKE_RCLONE_SECRET", "super-secret-token")

	asset := gdriveRcloneAsset("release/source.txt", "gdrive/failure/source.txt", nil, nil)
	_, _, err := worker.materializeDataAssets(dataAssetItem(asset), filepath.Join(worker.Config.TmpDir, "work"))
	if err == nil {
		t.Fatal("expected rclone failure")
	}
	message := err.Error()
	if strings.Contains(message, configPath) || strings.Contains(message, "super-secret-token") {
		t.Fatalf("error leaked secret details: %s", message)
	}
	if !strings.Contains(message, "[redacted") {
		t.Fatalf("expected redacted marker in error, got %s", message)
	}
	cacheDir := filepath.Join(worker.Config.effectiveAssetCacheDir(), "gdrive", "failure", "source.txt")
	if _, statErr := os.Stat(filepath.Join(cacheDir, "manifest.json")); !os.IsNotExist(statErr) {
		t.Fatalf("failed rclone should not leave manifest, stat err=%v", statErr)
	}
	if _, statErr := os.Stat(filepath.Join(cacheDir, "source")); !os.IsNotExist(statErr) {
		t.Fatalf("failed rclone should not leave source, stat err=%v", statErr)
	}
}

func TestMaterializeDataAssetsGDriveRcloneArchiveExtractionUsesAcquiredFile(t *testing.T) {
	worker, _ := newDataAssetTestWorker(t)
	source := filepath.Join(worker.Config.TmpDir, "release.zip")
	if err := os.WriteFile(source, zipBytesForRcloneTest(t, map[string]string{"tile/tile.hdr": "header"}), 0644); err != nil {
		t.Fatalf("write source: %v", err)
	}
	configureFakeRclone(t, &worker, source, filepath.Join(worker.Config.TmpDir, "rclone-args.jsonl"), "")

	asset := gdriveRcloneAsset("release/release.zip", "gdrive/archive/release.zip", nil, nil)
	asset.Kind = "field_boundary_archive"
	asset.Format = "zip"
	asset.Archive = &model.DataAssetArchive{
		Type:   model.DataAssetArchiveTypeZip,
		Select: []model.DataAssetArchiveSelect{{Member: "tile/tile.hdr", As: "field_segments.hdr"}},
		Expose: model.DataAssetArchiveExposeSelectedDirectory,
	}

	manifestPath, _, err := worker.materializeDataAssets(dataAssetItem(asset), filepath.Join(worker.Config.TmpDir, "work"))
	if err != nil {
		t.Fatalf("materialize: %v", err)
	}
	manifest := readMaterializedManifest(t, manifestPath)
	materialized := manifest.Assets[0]
	if got := readString(t, filepath.Join(materialized.LocalPath, "field_segments.hdr")); got != "header" {
		t.Fatalf("extracted content = %q", got)
	}
	if materialized.ArchiveType != model.DataAssetArchiveTypeZip || len(materialized.ArchiveMembers) != 1 {
		t.Fatalf("archive evidence = %+v", materialized)
	}
}

func configureFakeRclone(t *testing.T, worker *Worker, source string, argsPath string, configPath string) {
	t.Helper()
	t.Setenv("GOET_FAKE_RCLONE", "1")
	t.Setenv("GOET_FAKE_RCLONE_SOURCE", source)
	t.Setenv("GOET_FAKE_RCLONE_ARGS_PATH", argsPath)
	worker.Config.EnableGDriveRcloneProvider = true
	worker.Config.RcloneExecutable = os.Args[0]
	worker.Config.RcloneConfigPath = configPath
}

func gdriveRcloneAsset(drivePath string, cacheKey string, expectedSHA256 *string, expectedSize *int64) model.BoundDataAsset {
	sha := ""
	if expectedSHA256 != nil {
		sha = *expectedSHA256
	}
	return model.BoundDataAsset{
		BindingName:  "drive_data",
		ProviderName: "drive_provider",
		Kind:         "fixture",
		Format:       "txt",
		Provider:     model.DataProviderGDriveRclone,
		Location: model.DataAssetLocation{
			Type:      model.DataProviderGDriveRclone,
			Remote:    "landcore",
			DrivePath: drivePath,
		},
		Integrity:       model.DataAssetIntegrity{SHA256: sha, SizeBytes: expectedSize},
		Cache:           model.DataAssetCache{Strategy: model.DataAssetCacheStrategyWorkerCache, CacheKey: cacheKey},
		Materialization: model.DataAssetMaterialization{Strategy: model.DataAssetCacheStrategyWorkerCache},
	}
}

func readFakeRcloneCalls(t *testing.T, path string) [][]string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fake rclone args: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	calls := make([][]string, 0, len(lines))
	for _, line := range lines {
		var args []string
		if err := json.Unmarshal([]byte(line), &args); err != nil {
			t.Fatalf("decode fake rclone args %q: %v", line, err)
		}
		calls = append(calls, args)
	}
	return calls
}

func zipBytesForRcloneTest(t *testing.T, files map[string]string) []byte {
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

func runFakeRclone() int {
	args := os.Args[1:]
	if path := os.Getenv("GOET_FAKE_RCLONE_ARGS_PATH"); path != "" {
		data, err := json.Marshal(args)
		if err != nil {
			return 2
		}
		file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			return 2
		}
		if _, err := fmt.Fprintln(file, string(data)); err != nil {
			_ = file.Close()
			return 2
		}
		if err := file.Close(); err != nil {
			return 2
		}
	}

	if os.Getenv("GOET_FAKE_RCLONE_FAIL") == "1" {
		secret := os.Getenv("GOET_FAKE_RCLONE_SECRET")
		configPath := ""
		for i := 0; i+1 < len(args); i++ {
			if args[i] == "--config" {
				configPath = args[i+1]
				break
			}
		}
		_, _ = fmt.Fprintf(os.Stdout, "using config %s access_token=%s\n", configPath, secret)
		_, _ = fmt.Fprintf(os.Stderr, "Bearer %s\n", secret)
		return 17
	}

	copyIndex := -1
	for i, arg := range args {
		if arg == "copyto" {
			copyIndex = i
			break
		}
	}
	if copyIndex < 0 || copyIndex+2 >= len(args) {
		return 2
	}
	source := os.Getenv("GOET_FAKE_RCLONE_SOURCE")
	destination := args[copyIndex+2]
	data, err := os.ReadFile(source)
	if err != nil {
		return 2
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0755); err != nil {
		return 2
	}
	if err := os.WriteFile(destination, data, 0644); err != nil {
		return 2
	}
	return 0
}

func stringPtr(value string) *string {
	return &value
}

func int64PtrWorker(value int64) *int64 {
	return &value
}
