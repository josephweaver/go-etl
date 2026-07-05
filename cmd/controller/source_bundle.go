package main

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"goetl/internal/reposource"
)

const (
	sourceBundleRoutePrefix     = "/workflow-runs/"
	sourceBundleRouteSuffix     = "/source-bundle.zip"
	sourceBundleManifestZipPath = ".goet/source-manifest.json"
)

var sourceBundleZipTimestamp = time.Date(1980, 1, 1, 0, 0, 0, 0, time.UTC)

type sourceBundleHTTPError struct {
	status  int
	message string
	cause   error
}

func (e *sourceBundleHTTPError) Error() string {
	return e.message
}

func (e *sourceBundleHTTPError) Unwrap() error {
	return e.cause
}

type sourceBundleEntry struct {
	path string
	data []byte
}

func (c *Controller) sourceBundleHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	runID, ok := sourceBundleRunIDFromPath(r.URL.Path)
	if !ok {
		http.NotFound(w, r)
		return
	}
	if c.workflowStore == nil {
		http.Error(w, "workflow store required", http.StatusServiceUnavailable)
		return
	}

	bundle, err := c.buildSourceBundle(r.Context(), runID)
	if err != nil {
		var responseErr *sourceBundleHTTPError
		if errors.As(err, &responseErr) {
			http.Error(w, responseErr.message, responseErr.status)
			return
		}
		http.Error(w, "build source bundle", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/zip")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(bundle)
}

func sourceBundleRunIDFromPath(path string) (string, bool) {
	if !strings.HasPrefix(path, sourceBundleRoutePrefix) || !strings.HasSuffix(path, sourceBundleRouteSuffix) {
		return "", false
	}
	runID := strings.TrimPrefix(path, sourceBundleRoutePrefix)
	runID = strings.TrimSuffix(runID, sourceBundleRouteSuffix)
	if runID == "" || strings.Contains(runID, "/") {
		return "", false
	}
	return runID, true
}

func (c *Controller) buildSourceBundle(ctx context.Context, runID string) ([]byte, error) {
	manifest, _, err := c.loadRunAdmittedManifest(ctx, runID)
	if err != nil {
		return nil, err
	}
	access, err := reposource.NewCacheAccess(c.repoCacheLayout, manifest)
	if err != nil {
		return nil, &sourceBundleHTTPError{
			status:  http.StatusInternalServerError,
			message: "open admitted source cache",
			cause:   err,
		}
	}
	entries, err := sourceBundleEntries(manifest, access)
	if err != nil {
		return nil, err
	}

	var buffer bytes.Buffer
	if err := writeSourceBundleZip(&buffer, entries); err != nil {
		return nil, &sourceBundleHTTPError{
			status:  http.StatusInternalServerError,
			message: "construct source bundle zip",
			cause:   err,
		}
	}
	return buffer.Bytes(), nil
}

func (c *Controller) loadRunAdmittedManifest(ctx context.Context, runID string) (reposource.AdmittedSourceManifest, []byte, error) {
	run, found, err := c.workflowStore.GetWorkflowRun(ctx, runID)
	if err != nil {
		return reposource.AdmittedSourceManifest{}, nil, &sourceBundleHTTPError{
			status:  http.StatusInternalServerError,
			message: "load workflow run",
			cause:   err,
		}
	}
	if !found {
		return reposource.AdmittedSourceManifest{}, nil, &sourceBundleHTTPError{
			status:  http.StatusNotFound,
			message: "workflow run not found",
		}
	}

	var submissionContext workflowRunSubmissionContext
	if err := json.Unmarshal([]byte(run.SubmissionContextJSON), &submissionContext); err != nil {
		return reposource.AdmittedSourceManifest{}, nil, &sourceBundleHTTPError{
			status:  http.StatusInternalServerError,
			message: "workflow run submission context is invalid",
			cause:   err,
		}
	}
	if missingSourceAdmissionContext(submissionContext.SourceAdmission) {
		return reposource.AdmittedSourceManifest{}, nil, &sourceBundleHTTPError{
			status:  http.StatusInternalServerError,
			message: "workflow run missing source-admission context",
		}
	}
	if strings.TrimSpace(submissionContext.SourceAdmission.ManifestRef) == "" {
		return reposource.AdmittedSourceManifest{}, nil, &sourceBundleHTTPError{
			status:  http.StatusInternalServerError,
			message: "workflow run missing admitted manifest reference",
		}
	}

	manifestData, err := os.ReadFile(filepath.FromSlash(submissionContext.SourceAdmission.ManifestRef))
	if err != nil {
		message := "read admitted source manifest"
		if os.IsNotExist(err) {
			message = "admitted source manifest not found"
		}
		return reposource.AdmittedSourceManifest{}, nil, &sourceBundleHTTPError{
			status:  http.StatusInternalServerError,
			message: message,
			cause:   err,
		}
	}

	var manifest reposource.AdmittedSourceManifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		return reposource.AdmittedSourceManifest{}, nil, &sourceBundleHTTPError{
			status:  http.StatusInternalServerError,
			message: "admitted source manifest is invalid",
			cause:   err,
		}
	}
	return manifest, manifestData, nil
}

func missingSourceAdmissionContext(admission workflowRunSourceAdmissionContext) bool {
	return admission.Schema == "" &&
		admission.ManifestRef == "" &&
		admission.Source == (workflowRunSourceIdentity{}) &&
		admission.SourceRevisionID == nil &&
		len(admission.Files) == 0
}

func sourceBundleEntries(manifest reposource.AdmittedSourceManifest, access reposource.CacheAccess) ([]sourceBundleEntry, error) {
	stageable := make([]reposource.AdmittedSourceManifestFile, 0, len(manifest.Files))
	for _, file := range manifest.Files {
		switch file.Role {
		case reposource.FileRolePythonEntrypoint, reposource.FileRolePythonEnvironment, reposource.FileRoleSupportFile:
			stageable = append(stageable, file)
		}
	}

	sort.Slice(stageable, func(i, j int) bool {
		return stageable[i].SourcePath < stageable[j].SourcePath
	})

	seen := map[string]struct{}{}
	entries := make([]sourceBundleEntry, 0, len(stageable)+1)
	safeManifestData, err := marshalSourceBundleManifest(manifest, stageable)
	if err != nil {
		return nil, err
	}
	if err := appendSourceBundleEntry(&entries, seen, sourceBundleManifestZipPath, safeManifestData); err != nil {
		return nil, err
	}
	for _, file := range stageable {
		entryPath, err := validateSourceBundleEntryPath(file.SourcePath)
		if err != nil {
			return nil, &sourceBundleHTTPError{
				status:  http.StatusInternalServerError,
				message: "unsafe admitted source path",
				cause:   err,
			}
		}
		data, err := access.ReadFile(file.CachePath)
		if err != nil {
			message := "read cached admitted source file"
			switch {
			case errors.Is(err, reposource.ErrCacheMiss):
				message = "cached admitted source file is missing"
			case errors.Is(err, reposource.ErrCacheCorruption):
				message = "cached admitted source file is corrupted"
			}
			return nil, &sourceBundleHTTPError{
				status:  http.StatusInternalServerError,
				message: message,
				cause:   err,
			}
		}
		if err := appendSourceBundleEntry(&entries, seen, entryPath, data); err != nil {
			return nil, err
		}
	}
	return entries, nil
}

type sourceBundleManifest struct {
	Schema string                     `json:"schema"`
	RunID  string                     `json:"run_id"`
	Files  []sourceBundleManifestFile `json:"files"`
}

type sourceBundleManifestFile struct {
	Role                string  `json:"role"`
	SourcePath          string  `json:"source_path"`
	ContentType         string  `json:"content_type,omitempty"`
	SizeBytes           int64   `json:"size_bytes"`
	RawSHA256           *string `json:"raw_sha256,omitempty"`
	CanonicalJSONSHA256 *string `json:"canonical_json_sha256,omitempty"`
}

func marshalSourceBundleManifest(manifest reposource.AdmittedSourceManifest, files []reposource.AdmittedSourceManifestFile) ([]byte, error) {
	safeFiles := make([]sourceBundleManifestFile, 0, len(files))
	for _, file := range files {
		safeFiles = append(safeFiles, sourceBundleManifestFile{
			Role:                string(file.Role),
			SourcePath:          file.SourcePath,
			ContentType:         file.ContentType,
			SizeBytes:           file.SizeBytes,
			RawSHA256:           file.RawSHA256,
			CanonicalJSONSHA256: file.CanonicalJSONSHA256,
		})
	}
	safeManifest := sourceBundleManifest{
		Schema: manifest.Schema,
		RunID:  manifest.RunID,
		Files:  safeFiles,
	}
	return json.Marshal(safeManifest)
}

func appendSourceBundleEntry(entries *[]sourceBundleEntry, seen map[string]struct{}, entryPath string, data []byte) error {
	if _, exists := seen[entryPath]; exists {
		return &sourceBundleHTTPError{
			status:  http.StatusInternalServerError,
			message: "duplicate source bundle entry path",
			cause:   fmt.Errorf("duplicate entry path %s", entryPath),
		}
	}
	seen[entryPath] = struct{}{}
	*entries = append(*entries, sourceBundleEntry{
		path: entryPath,
		data: data,
	})
	return nil
}

func validateSourceBundleEntryPath(value string) (string, error) {
	clean, err := reposource.ValidateRepositoryRelativePath(value)
	if err != nil {
		return "", err
	}
	if clean != value {
		return "", fmt.Errorf("path must be clean slash-separated relative path")
	}
	return clean, nil
}

func writeSourceBundleZip(buffer *bytes.Buffer, entries []sourceBundleEntry) error {
	zipWriter := zip.NewWriter(buffer)
	for _, entry := range entries {
		header := &zip.FileHeader{
			Name:     entry.path,
			Method:   zip.Deflate,
			Modified: sourceBundleZipTimestamp,
		}
		writer, err := zipWriter.CreateHeader(header)
		if err != nil {
			_ = zipWriter.Close()
			return fmt.Errorf("create zip entry %s: %w", entry.path, err)
		}
		if _, err := writer.Write(entry.data); err != nil {
			_ = zipWriter.Close()
			return fmt.Errorf("write zip entry %s: %w", entry.path, err)
		}
	}
	if err := zipWriter.Close(); err != nil {
		return fmt.Errorf("close zip writer: %w", err)
	}
	return nil
}
