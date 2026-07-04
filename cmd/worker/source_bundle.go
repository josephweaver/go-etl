package main

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"

	"goetl/internal/model"
)

type WorkStaging struct {
	AttemptDir string
	SourceDir  string
	WorkDir    string
	LogDir     string
}

type stagedZipEntry struct {
	file       *zip.File
	normalized  string
	isDir      bool
}

func (w Worker) stageWorkItemSourceBundle(item model.WorkItem) (WorkStaging, error) {
	if item.AttemptID == "" {
		return WorkStaging{}, fmt.Errorf("work item attempt id is required")
	}
	if item.Source == nil {
		return WorkStaging{}, fmt.Errorf("work item source is required")
	}
	if strings.TrimSpace(item.Source.RunID) == "" {
		return WorkStaging{}, fmt.Errorf("work item source run id is required")
	}

	sourceURL := strings.TrimRight(w.Config.ControllerURL, "/") +
		"/workflow-runs/" + url.PathEscape(item.Source.RunID) + "/source-bundle.zip"

	response, err := http.Get(sourceURL)
	if err != nil {
		return WorkStaging{}, fmt.Errorf("get source bundle from %s: %w", sourceURL, err)
	}
	defer response.Body.Close()

	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return WorkStaging{}, fmt.Errorf("get source bundle from %s: unexpected status %s", sourceURL, response.Status)
	}

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return WorkStaging{}, fmt.Errorf("read source bundle from %s: %w", sourceURL, err)
	}
	if len(body) == 0 {
		return WorkStaging{}, fmt.Errorf("read source bundle from %s: empty body", sourceURL)
	}

	reader, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		return WorkStaging{}, fmt.Errorf("decode source bundle from %s: %w", sourceURL, err)
	}

	staging := WorkStaging{
		AttemptDir: filepath.Join(w.Config.TmpDir, "attempts", item.AttemptID),
	}
	staging.SourceDir = filepath.Join(staging.AttemptDir, "source")
	staging.WorkDir = filepath.Join(staging.AttemptDir, "work")
	staging.LogDir = filepath.Join(staging.AttemptDir, "logs")

	if err := os.MkdirAll(staging.SourceDir, 0755); err != nil {
		return WorkStaging{}, fmt.Errorf("create source staging dir %s: %w", staging.SourceDir, err)
	}
	if err := os.MkdirAll(staging.WorkDir, 0755); err != nil {
		return WorkStaging{}, fmt.Errorf("create work staging dir %s: %w", staging.WorkDir, err)
	}
	if err := os.MkdirAll(staging.LogDir, 0755); err != nil {
		return WorkStaging{}, fmt.Errorf("create log staging dir %s: %w", staging.LogDir, err)
	}

	plan, err := validateSourceBundleEntries(reader.File)
	if err != nil {
		return WorkStaging{}, err
	}

	for _, entry := range plan {
		if err := extractSourceBundleEntry(staging.SourceDir, entry); err != nil {
			return WorkStaging{}, err
		}
	}

	return staging, nil
}

func validateSourceBundleEntries(entries []*zip.File) ([]stagedZipEntry, error) {
	occupied := make(map[string]zipEntryKind)
	plan := make([]stagedZipEntry, 0, len(entries))

	for _, entry := range entries {
		normalized, isDir, err := validateSourceBundleEntryName(entry.Name)
		if err != nil {
			return nil, fmt.Errorf("validate source bundle entry %q: %w", entry.Name, err)
		}
		isDir = isDir || entry.FileInfo().IsDir()

		if err := checkSourceBundleAncestors(normalized, occupied); err != nil {
			return nil, fmt.Errorf("validate source bundle entry %q: %w", entry.Name, err)
		}

		if _, exists := occupied[normalized]; exists {
			return nil, fmt.Errorf("validate source bundle entry %q: duplicate normalized path %q", entry.Name, normalized)
		}

		if !isDir {
			if descendant := findSourceBundleDescendant(normalized, occupied); descendant != "" {
				return nil, fmt.Errorf("validate source bundle entry %q: directory/file collision with %q", entry.Name, descendant)
			}
		}

		if err := rejectSymlinkLikeEntry(entry); err != nil {
			return nil, fmt.Errorf("validate source bundle entry %q: %w", entry.Name, err)
		}

		occupied[normalized] = kindFromBool(isDir)
		plan = append(plan, stagedZipEntry{
			file:      entry,
			normalized: normalized,
			isDir:     isDir,
		})
	}

	return plan, nil
}

type zipEntryKind int

const (
	zipEntryKindFile zipEntryKind = iota
	zipEntryKindDir
)

func kindFromBool(isDir bool) zipEntryKind {
	if isDir {
		return zipEntryKindDir
	}
	return zipEntryKindFile
}

func validateSourceBundleEntryName(name string) (string, bool, error) {
	if strings.TrimSpace(name) == "" {
		return "", false, fmt.Errorf("entry path is required")
	}
	if strings.Contains(name, "\\") {
		return "", false, fmt.Errorf("entry path must not contain backslashes")
	}
	if isWindowsDriveQualifiedPath(name) {
		return "", false, fmt.Errorf("entry path must not be drive-qualified")
	}
	if path.IsAbs(name) || filepath.IsAbs(name) {
		return "", false, fmt.Errorf("entry path must not be absolute")
	}
	if hasParentTraversal(name) {
		return "", false, fmt.Errorf("entry path must not contain .. traversal")
	}

	normalized := path.Clean(name)
	if normalized == "." || normalized == "" {
		return "", false, fmt.Errorf("entry path is required")
	}
	if path.IsAbs(normalized) {
		return "", false, fmt.Errorf("entry path must not be absolute")
	}
	if hasWindowsDrivePrefix(normalized) {
		return "", false, fmt.Errorf("entry path must not be drive-qualified")
	}

	isDir := strings.HasSuffix(name, "/")
	return normalized, isDir, nil
}

func hasParentTraversal(name string) bool {
	for _, part := range strings.Split(name, "/") {
		if part == ".." {
			return true
		}
	}
	return false
}

func isWindowsDriveQualifiedPath(name string) bool {
	return hasWindowsDrivePrefix(name)
}

func hasWindowsDrivePrefix(name string) bool {
	if len(name) < 2 {
		return false
	}
	drive := name[0]
	if (drive < 'A' || drive > 'Z') && (drive < 'a' || drive > 'z') {
		return false
	}
	return name[1] == ':'
}

func checkSourceBundleAncestors(normalized string, occupied map[string]zipEntryKind) error {
	parent := path.Dir(normalized)
	for parent != "." && parent != "/" {
		if kind, exists := occupied[parent]; exists && kind == zipEntryKindFile {
			return fmt.Errorf("parent path %q is already a file", parent)
		}
		next := path.Dir(parent)
		if next == parent {
			break
		}
		parent = next
	}
	if kind, exists := occupied[normalized]; exists && kind == zipEntryKindFile {
		return fmt.Errorf("path %q is already a file", normalized)
	}
	return nil
}

func findSourceBundleDescendant(normalized string, occupied map[string]zipEntryKind) string {
	prefix := normalized + "/"
	for existing := range occupied {
		if strings.HasPrefix(existing, prefix) {
			return existing
		}
	}
	return ""
}

func rejectSymlinkLikeEntry(entry *zip.File) error {
	if entry.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("symlink-like entries are not allowed")
	}
	return nil
}

func extractSourceBundleEntry(sourceRoot string, entry stagedZipEntry) error {
	target := filepath.Join(sourceRoot, filepath.FromSlash(entry.normalized))
	rel, err := filepath.Rel(sourceRoot, target)
	if err != nil {
		return fmt.Errorf("resolve target path %s: %w", target, err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("entry %q escapes source staging root", entry.file.Name)
	}

	if entry.isDir {
		return os.MkdirAll(target, 0755)
	}

	if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		return fmt.Errorf("create parent directory for %s: %w", target, err)
	}

	src, err := entry.file.Open()
	if err != nil {
		return fmt.Errorf("open zip entry %q: %w", entry.file.Name, err)
	}
	defer src.Close()

	mode := entry.file.Mode().Perm()
	if mode == 0 {
		mode = 0644
	}

	dst, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return fmt.Errorf("create staged file %s: %w", target, err)
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		return fmt.Errorf("write staged file %s: %w", target, err)
	}

	return nil
}
