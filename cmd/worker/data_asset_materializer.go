package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"goetl/internal/model"
)

const workerCacheManifestSchemaV1 = "goet/worker-asset-cache/v1"

type assetMaterializer struct {
	config  Config
	workDir string
}

type assetEvidence struct {
	size   int64
	sha256 string
}

type workerCacheManifest struct {
	Schema          string `json:"schema"`
	CacheKey        string `json:"cache_key"`
	BindingName     string `json:"binding_name"`
	ProviderName    string `json:"provider_name"`
	ProviderType    string `json:"provider_type"`
	SourceSizeBytes int64  `json:"source_size_bytes"`
	SourceSHA256    string `json:"source_sha256"`
	Immutable       bool   `json:"immutable"`
	WrittenAt       string `json:"written_at"`
}

func (w Worker) materializeDataAssets(item model.WorkItem, workDir string) (string, bool, error) {
	assets, err := boundDataAssetsFromWorkItem(item)
	if err != nil {
		return "", false, err
	}
	if len(assets) == 0 {
		return "", false, nil
	}

	materializer := assetMaterializer{config: w.Config, workDir: workDir}
	manifest := model.MaterializedDataAssetManifest{
		Schema: model.MaterializedDataAssetManifestSchemaV1,
		Assets: make([]model.MaterializedDataAsset, 0, len(assets)),
	}
	for _, asset := range assets {
		materialized, err := materializer.materialize(asset)
		if err != nil {
			return "", false, fmt.Errorf("materialize data asset %q: %w", asset.BindingName, err)
		}
		manifest.Assets = append(manifest.Assets, materialized)
	}
	if err := manifest.Validate(); err != nil {
		return "", false, fmt.Errorf("validate materialized data assets manifest: %w", err)
	}

	path := filepath.Join(workDir, "data-assets.json")
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return "", false, fmt.Errorf("encode materialized data assets manifest: %w", err)
	}
	if err := atomicWriteFile(path, data, 0644); err != nil {
		return "", false, fmt.Errorf("write materialized data assets manifest %s: %w", path, err)
	}
	return path, true, nil
}

func boundDataAssetsFromWorkItem(item model.WorkItem) ([]model.BoundDataAsset, error) {
	parameter, ok := item.Parameters["data_assets"]
	if !ok {
		return nil, nil
	}
	if parameter.Type != "data_assets" && parameter.Type != "list" {
		return nil, fmt.Errorf("parameter data_assets has type %s, want data_assets or list", parameter.Type)
	}

	data, err := json.Marshal(parameter.Value)
	if err != nil {
		return nil, fmt.Errorf("encode data_assets parameter: %w", err)
	}
	var assets []model.BoundDataAsset
	if err := json.Unmarshal(data, &assets); err != nil {
		return nil, fmt.Errorf("decode data_assets parameter: %w", err)
	}
	for i, asset := range assets {
		if err := asset.Validate(); err != nil {
			return nil, fmt.Errorf("data_assets[%d]: %w", i, err)
		}
	}
	return assets, nil
}

func (m assetMaterializer) materialize(asset model.BoundDataAsset) (model.MaterializedDataAsset, error) {
	strategy, err := materializationStrategy(asset)
	if err != nil {
		return model.MaterializedDataAsset{}, err
	}

	var materialized model.MaterializedDataAsset
	switch strategy {
	case model.DataAssetCacheStrategyReference:
		materialized, err = m.materializeReference(asset)
	case model.DataAssetCacheStrategyWorkerCache:
		materialized, err = m.materializeWorkerCache(asset)
	default:
		return model.MaterializedDataAsset{}, fmt.Errorf("unsupported materialization strategy %q", strategy)
	}
	if err != nil {
		return model.MaterializedDataAsset{}, err
	}
	if asset.Archive == nil {
		return materialized, nil
	}
	return m.extractArchive(asset, materialized)
}

func materializationStrategy(asset model.BoundDataAsset) (string, error) {
	if asset.Materialization.Strategy != "" {
		return asset.Materialization.Strategy, nil
	}
	if asset.Cache.Strategy != "" {
		return asset.Cache.Strategy, nil
	}
	switch asset.Provider {
	case model.DataProviderHTTP:
		return model.DataAssetCacheStrategyWorkerCache, nil
	case model.DataProviderLocalFile, model.DataProviderRegisteredLocation:
		return model.DataAssetCacheStrategyReference, nil
	default:
		return "", fmt.Errorf("unsupported data provider %q", asset.Provider)
	}
}

func (m assetMaterializer) materializeReference(asset model.BoundDataAsset) (model.MaterializedDataAsset, error) {
	if asset.Provider == model.DataProviderHTTP {
		return model.MaterializedDataAsset{}, fmt.Errorf("http assets require worker_cache materialization")
	}

	localPath, err := m.resolveNamedLocationPath(asset.Location.LocationName, asset.Location.Path)
	if err != nil {
		return model.MaterializedDataAsset{}, err
	}
	evidence, err := hashFileWithLimit(localPath, m.config.effectiveMaxAssetBytes())
	if err != nil {
		return model.MaterializedDataAsset{}, err
	}
	if err := verifyExpectedIntegrity(asset, evidence); err != nil {
		return model.MaterializedDataAsset{}, err
	}

	return materializedAsset(asset, localPath, model.DataAssetCacheStrategyReference, "", nil, evidence), nil
}

func (m assetMaterializer) materializeWorkerCache(asset model.BoundDataAsset) (model.MaterializedDataAsset, error) {
	cacheKey, err := cacheKeyForAsset(asset)
	if err != nil {
		return model.MaterializedDataAsset{}, err
	}
	cacheDir := filepath.Join(m.config.effectiveAssetCacheDir(), filepath.FromSlash(cacheKey))
	sourcePath := filepath.Join(cacheDir, "source")
	manifestPath := filepath.Join(cacheDir, "manifest.json")
	immutable := asset.Cache.EffectiveImmutable()

	if sourceExists(sourcePath) || sourceExists(manifestPath) {
		evidence, err := readAndVerifyCache(sourcePath, manifestPath)
		if err != nil {
			return model.MaterializedDataAsset{}, err
		}
		if err := verifyExpectedIntegrity(asset, evidence); err != nil {
			return model.MaterializedDataAsset{}, err
		}
		return materializedAsset(asset, sourcePath, model.DataAssetCacheStrategyWorkerCache, cacheKey, &immutable, evidence), nil
	}

	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return model.MaterializedDataAsset{}, fmt.Errorf("create asset cache dir %s: %w", cacheDir, err)
	}
	evidence, err := m.acquireSource(asset, sourcePath)
	if err != nil {
		return model.MaterializedDataAsset{}, err
	}
	if err := verifyExpectedIntegrity(asset, evidence); err != nil {
		_ = os.Remove(sourcePath)
		return model.MaterializedDataAsset{}, err
	}

	cacheManifest := workerCacheManifest{
		Schema:          workerCacheManifestSchemaV1,
		CacheKey:        cacheKey,
		BindingName:     asset.BindingName,
		ProviderName:    asset.ProviderName,
		ProviderType:    asset.Provider,
		SourceSizeBytes: evidence.size,
		SourceSHA256:    evidence.sha256,
		Immutable:       immutable,
		WrittenAt:       time.Now().UTC().Format(time.RFC3339Nano),
	}
	if err := writeCacheManifest(manifestPath, cacheManifest); err != nil {
		_ = os.Remove(sourcePath)
		return model.MaterializedDataAsset{}, err
	}

	return materializedAsset(asset, sourcePath, model.DataAssetCacheStrategyWorkerCache, cacheKey, &immutable, evidence), nil
}

func (m assetMaterializer) extractArchive(asset model.BoundDataAsset, source model.MaterializedDataAsset) (model.MaterializedDataAsset, error) {
	extractionRoot, err := m.archiveExtractionRoot(asset, source)
	if err != nil {
		return model.MaterializedDataAsset{}, err
	}

	result, err := extractArchiveSelection(archiveExtractionRequest{
		sourcePath:          source.LocalPath,
		extractionRoot:      extractionRoot,
		archive:             *asset.Archive,
		sevenZipExecutable:  m.config.SevenZipExecutable,
		maxSelectedFileSize: m.config.effectiveMaxAssetBytes(),
	})
	if err != nil {
		return model.MaterializedDataAsset{}, err
	}

	source.LocalPath = result.localPath
	source.ArchiveType = asset.Archive.Type
	source.ArchiveMembers = result.members
	source.SelectedSizeBytes = &result.selected.size
	source.SelectedSHA256 = result.selected.sha256
	return source, nil
}

func (m assetMaterializer) archiveExtractionRoot(asset model.BoundDataAsset, source model.MaterializedDataAsset) (string, error) {
	if source.CacheKey != "" {
		cacheKey, err := cleanDataRelativePath(source.CacheKey)
		if err != nil {
			return "", err
		}
		return filepath.Join(m.config.effectiveAssetCacheDir(), filepath.FromSlash(cacheKey), "extracted"), nil
	}
	if strings.TrimSpace(m.workDir) == "" {
		return "", fmt.Errorf("archive extraction work directory is not configured")
	}
	binding, err := cleanDataRelativePath(asset.BindingName)
	if err != nil {
		return "", err
	}
	return filepath.Join(m.workDir, "data-assets", filepath.FromSlash(binding), "extracted"), nil
}

func (m assetMaterializer) acquireSource(asset model.BoundDataAsset, destination string) (assetEvidence, error) {
	switch asset.Provider {
	case model.DataProviderHTTP:
		return m.downloadHTTP(asset.Location.URI, destination)
	case model.DataProviderLocalFile, model.DataProviderRegisteredLocation:
		source, err := m.resolveNamedLocationPath(asset.Location.LocationName, asset.Location.Path)
		if err != nil {
			return assetEvidence{}, err
		}
		return copyFileWithLimit(source, destination, m.config.effectiveMaxAssetBytes())
	default:
		return assetEvidence{}, fmt.Errorf("unsupported data provider %q", asset.Provider)
	}
}

func (m assetMaterializer) downloadHTTP(uri string, destination string) (assetEvidence, error) {
	response, err := http.Get(uri)
	if err != nil {
		return assetEvidence{}, fmt.Errorf("download %s: %w", uri, err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return assetEvidence{}, fmt.Errorf("download %s: unexpected status %s", uri, response.Status)
	}
	return writeStreamAtomically(destination, response.Body, m.config.effectiveMaxAssetBytes())
}

func (m assetMaterializer) resolveNamedLocationPath(name string, relativePath string) (string, error) {
	root, ok := m.config.DataLocationRoots[name]
	if !ok || strings.TrimSpace(root) == "" {
		return "", fmt.Errorf("data location root %q is not configured", name)
	}
	safe, err := cleanDataRelativePath(relativePath)
	if err != nil {
		return "", err
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("resolve data location root %s: %w", root, err)
	}
	candidate := filepath.Join(rootAbs, filepath.FromSlash(safe))
	rel, err := filepath.Rel(rootAbs, candidate)
	if err != nil {
		return "", fmt.Errorf("resolve data asset path %s: %w", candidate, err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("data asset path escapes configured root: %s", relativePath)
	}
	info, err := os.Stat(candidate)
	if err != nil {
		return "", fmt.Errorf("check data asset path %s: %w", candidate, err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("data asset path is a directory: %s", candidate)
	}
	return candidate, nil
}

func materializedAsset(asset model.BoundDataAsset, localPath string, strategy string, cacheKey string, immutable *bool, evidence assetEvidence) model.MaterializedDataAsset {
	size := evidence.size
	return model.MaterializedDataAsset{
		BindingName:             asset.BindingName,
		ProviderName:            asset.ProviderName,
		ProviderType:            asset.Provider,
		Kind:                    asset.Kind,
		Format:                  asset.Format,
		LocalPath:               localPath,
		MaterializationStrategy: strategy,
		CacheKey:                cacheKey,
		CacheImmutable:          immutable,
		SourceSizeBytes:         &size,
		SourceSHA256:            evidence.sha256,
	}
}

func cacheKeyForAsset(asset model.BoundDataAsset) (string, error) {
	if asset.Cache.CacheKey != "" {
		return cleanDataRelativePath(asset.Cache.CacheKey)
	}
	data, err := json.Marshal(asset)
	if err != nil {
		return "", fmt.Errorf("encode data asset cache identity: %w", err)
	}
	sum := sha256.Sum256(data)
	return "derived/" + asset.BindingName + "/" + hex.EncodeToString(sum[:]), nil
}

func verifyExpectedIntegrity(asset model.BoundDataAsset, evidence assetEvidence) error {
	if asset.Integrity.SizeBytes != nil && evidence.size != *asset.Integrity.SizeBytes {
		return fmt.Errorf("expected size %d, observed %d", *asset.Integrity.SizeBytes, evidence.size)
	}
	if asset.Integrity.SHA256 != "" && evidence.sha256 != asset.Integrity.SHA256 {
		return fmt.Errorf("expected sha256 %s, observed %s", asset.Integrity.SHA256, evidence.sha256)
	}
	return nil
}

func readAndVerifyCache(sourcePath string, manifestPath string) (assetEvidence, error) {
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return assetEvidence{}, fmt.Errorf("read asset cache manifest %s: %w", manifestPath, err)
	}
	var manifest workerCacheManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return assetEvidence{}, fmt.Errorf("decode asset cache manifest %s: %w", manifestPath, err)
	}
	if manifest.Schema != workerCacheManifestSchemaV1 {
		return assetEvidence{}, fmt.Errorf("unsupported asset cache manifest schema %q", manifest.Schema)
	}
	evidence, err := hashFileWithLimit(sourcePath, 0)
	if err != nil {
		return assetEvidence{}, err
	}
	if evidence.size != manifest.SourceSizeBytes || evidence.sha256 != manifest.SourceSHA256 {
		return assetEvidence{}, fmt.Errorf("asset cache source does not match manifest")
	}
	return evidence, nil
}

func writeCacheManifest(path string, manifest workerCacheManifest) error {
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("encode asset cache manifest: %w", err)
	}
	return atomicWriteFile(path, data, 0644)
}

func copyFileWithLimit(source string, destination string, limit int64) (assetEvidence, error) {
	file, err := os.Open(source)
	if err != nil {
		return assetEvidence{}, fmt.Errorf("open data asset source %s: %w", source, err)
	}
	defer file.Close()
	return writeStreamAtomically(destination, file, limit)
}

func hashFileWithLimit(path string, limit int64) (assetEvidence, error) {
	file, err := os.Open(path)
	if err != nil {
		return assetEvidence{}, fmt.Errorf("open data asset %s: %w", path, err)
	}
	defer file.Close()
	hash := sha256.New()
	size, err := copyHashWithLimit(io.Discard, file, hash, limit)
	if err != nil {
		return assetEvidence{}, fmt.Errorf("read data asset %s: %w", path, err)
	}
	return assetEvidence{size: size, sha256: hex.EncodeToString(hash.Sum(nil))}, nil
}

func writeStreamAtomically(destination string, source io.Reader, limit int64) (assetEvidence, error) {
	if err := os.MkdirAll(filepath.Dir(destination), 0755); err != nil {
		return assetEvidence{}, fmt.Errorf("create parent directory for %s: %w", destination, err)
	}
	tmp := destination + ".tmp-" + randomHex(8)
	file, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0644)
	if err != nil {
		return assetEvidence{}, fmt.Errorf("create temporary data asset %s: %w", tmp, err)
	}

	hash := sha256.New()
	size, copyErr := copyHashWithLimit(file, source, hash, limit)
	closeErr := file.Close()
	if copyErr != nil {
		_ = os.Remove(tmp)
		return assetEvidence{}, copyErr
	}
	if closeErr != nil {
		_ = os.Remove(tmp)
		return assetEvidence{}, fmt.Errorf("close temporary data asset %s: %w", tmp, closeErr)
	}
	if err := os.Rename(tmp, destination); err != nil {
		_ = os.Remove(tmp)
		return assetEvidence{}, fmt.Errorf("move data asset from %s to %s: %w", tmp, destination, err)
	}
	return assetEvidence{size: size, sha256: hex.EncodeToString(hash.Sum(nil))}, nil
}

func copyHashWithLimit(dst io.Writer, src io.Reader, h hash.Hash, limit int64) (int64, error) {
	buffer := make([]byte, 32*1024)
	var written int64
	for {
		n, readErr := src.Read(buffer)
		if n > 0 {
			written += int64(n)
			if limit > 0 && written > limit {
				return written, fmt.Errorf("data asset exceeds maximum size %d bytes", limit)
			}
			chunk := buffer[:n]
			if _, err := h.Write(chunk); err != nil {
				return written, err
			}
			if _, err := dst.Write(chunk); err != nil {
				return written, err
			}
		}
		if readErr == io.EOF {
			return written, nil
		}
		if readErr != nil {
			return written, readErr
		}
	}
}

func sourceExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func cleanDataRelativePath(value string) (string, error) {
	if strings.TrimSpace(value) == "" {
		return "", fmt.Errorf("data asset relative path is required")
	}
	if strings.Contains(value, "\\") {
		return "", fmt.Errorf("data asset relative path must not contain backslashes")
	}
	if hasWindowsDrivePrefix(value) {
		return "", fmt.Errorf("data asset relative path must not be drive-qualified")
	}
	if path.IsAbs(value) || filepath.IsAbs(value) {
		return "", fmt.Errorf("data asset relative path must not be absolute")
	}
	for _, part := range strings.Split(value, "/") {
		if part == "." || part == ".." || part == "" {
			return "", fmt.Errorf("data asset relative path must use clean non-traversing segments")
		}
	}
	clean := path.Clean(value)
	if clean != value {
		return "", fmt.Errorf("data asset relative path must be clean")
	}
	return clean, nil
}
