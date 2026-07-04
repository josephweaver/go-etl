package reposource

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

func PublishAdmittedSource(layout CacheLayout, manifest AdmittedSourceManifest, reads []ReadFileResult) error {
	access, err := NewCacheAccess(layout, manifest)
	if err != nil {
		return err
	}
	readByPath := make(map[string]ReadFileResult, len(reads))
	for _, read := range reads {
		sourcePath, err := ValidateRepositoryRelativePath(read.Request.SourcePath)
		if err != nil {
			return fmt.Errorf("validate read source path %q: %w", read.Request.SourcePath, err)
		}
		readByPath[sourcePath] = read
	}

	for _, file := range manifest.Files {
		read, ok := readByPath[file.SourcePath]
		if !ok {
			return fmt.Errorf("manifest file %s has no admitted bytes", file.SourcePath)
		}
		if err := VerifyCachedFile(file, read.Content.Data); err != nil {
			return err
		}
		path, err := access.FilePathForManifestFile(file)
		if err != nil {
			return err
		}
		if err := publishCacheFile(path, file, read.Content.Data); err != nil {
			return err
		}
	}
	return publishManifest(access, manifest)
}

func publishCacheFile(path string, file AdmittedSourceManifestFile, data []byte) error {
	existing, err := os.ReadFile(path)
	if err == nil {
		if err := VerifyCachedFile(file, existing); err != nil {
			return err
		}
		return nil
	}
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read existing cached file %s: %w", file.CachePath, err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create cache parent for %s: %w", file.CachePath, err)
	}
	return writeFileAtomically(path, data)
}

func publishManifest(access CacheAccess, manifest AdmittedSourceManifest) error {
	paths, err := access.Paths()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("encode admitted source manifest: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(paths.ManifestPath), 0o755); err != nil {
		return fmt.Errorf("create manifest parent: %w", err)
	}
	return writeFileAtomically(paths.ManifestPath, data)
}

func writeFileAtomically(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temp cache file: %w", err)
	}
	tmpPath := tmp.Name()
	defer func() {
		_ = os.Remove(tmpPath)
	}()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write temp cache file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp cache file: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		if errors.Is(err, os.ErrExist) {
			return fmt.Errorf("promote temp cache file: %w", err)
		}
		return fmt.Errorf("promote temp cache file: %w", err)
	}
	return nil
}
