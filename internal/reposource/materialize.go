package reposource

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func MaterializeManifest(ctx context.Context, access CacheAccess, destination string) error {
	if destination == "" {
		return fmt.Errorf("destination is required")
	}
	destinationAbs, err := filepath.Abs(destination)
	if err != nil {
		return fmt.Errorf("resolve destination: %w", err)
	}
	destinationAbs = filepath.Clean(destinationAbs)
	for _, file := range access.manifest.Files {
		if err := ctx.Err(); err != nil {
			return err
		}
		cachePath, err := ValidateCacheRelativePath(file.CachePath)
		if err != nil {
			return fmt.Errorf("validate materialized cache path %q: %w", file.CachePath, err)
		}
		data, err := access.ReadFile(cachePath)
		if err != nil {
			return err
		}
		target, err := materializedFilePath(destinationAbs, cachePath)
		if err != nil {
			return err
		}
		if err := writeMaterializedFile(target, data); err != nil {
			return fmt.Errorf("materialize %s: %w", cachePath, err)
		}
	}
	return nil
}

func materializedFilePath(destinationRoot string, cachePath string) (string, error) {
	target, err := joinUnderRoot(destinationRoot, filepath.FromSlash(cachePath))
	if err != nil {
		return "", fmt.Errorf("materialized path escapes destination: %w", err)
	}
	rel, err := filepath.Rel(destinationRoot, target)
	if err != nil {
		return "", fmt.Errorf("check materialized path: %w", err)
	}
	if rel == "." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." || filepath.IsAbs(rel) {
		return "", fmt.Errorf("materialized path escapes destination")
	}
	return target, nil
}

func writeMaterializedFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create parent directories: %w", err)
	}
	return writeFileAtomically(path, data)
}
