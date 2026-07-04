package reposource

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"goetl/internal/fingerprint"
)

var (
	ErrCacheMiss       = errors.New("repository cache miss")
	ErrCacheCorruption = errors.New("repository cache corruption")
)

func VerifyCachedFile(file AdmittedSourceManifestFile, data []byte) error {
	if file.SizeBytes >= 0 && int64(len(data)) != file.SizeBytes {
		return fmt.Errorf("%w: %s size %d != %d", ErrCacheCorruption, file.CachePath, len(data), file.SizeBytes)
	}
	if file.RawSHA256 != nil {
		if got := fingerprint.SHA256Hex(data); got != *file.RawSHA256 {
			return fmt.Errorf("%w: %s raw sha256 mismatch", ErrCacheCorruption, file.CachePath)
		}
	}
	if file.CanonicalJSONSHA256 != nil {
		got, err := cachedCanonicalJSONSHA256(data)
		if err != nil {
			return fmt.Errorf("%w: %s canonical json: %v", ErrCacheCorruption, file.CachePath, err)
		}
		if got != *file.CanonicalJSONSHA256 {
			return fmt.Errorf("%w: %s canonical json sha256 mismatch", ErrCacheCorruption, file.CachePath)
		}
	}
	return nil
}

func cachedCanonicalJSONSHA256(data []byte) (string, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return "", err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return "", fmt.Errorf("multiple json values")
		}
		return "", err
	}
	_, hash, err := fingerprint.CanonicalJSONSHA256(value)
	if err != nil {
		return "", err
	}
	return hash, nil
}
