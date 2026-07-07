package main

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	fp "goetl/internal/fingerprint"
	"goetl/internal/model"
)

type archiveExtractionRequest struct {
	sourcePath          string
	extractionRoot      string
	archive             model.DataAssetArchive
	sevenZipExecutable  string
	maxSelectedFileSize int64
}

type archiveExtractionResult struct {
	localPath string
	members   []model.MaterializedArchiveMember
	selected  assetEvidence
}

func extractArchiveSelection(req archiveExtractionRequest) (archiveExtractionResult, error) {
	if err := req.archive.Validate(); err != nil {
		return archiveExtractionResult{}, err
	}
	if strings.TrimSpace(req.sourcePath) == "" {
		return archiveExtractionResult{}, fmt.Errorf("archive source path is required")
	}
	if strings.TrimSpace(req.extractionRoot) == "" {
		return archiveExtractionResult{}, fmt.Errorf("archive extraction root is required")
	}

	selections, err := validatedArchiveSelections(req.archive.Select)
	if err != nil {
		return archiveExtractionResult{}, err
	}
	if len(selections) == 0 {
		return archiveExtractionResult{}, fmt.Errorf("archive select requires at least one member")
	}

	if err := prepareExtractionRoot(req.extractionRoot); err != nil {
		return archiveExtractionResult{}, err
	}

	switch req.archive.Type {
	case model.DataAssetArchiveTypeZip:
		return extractZIPSelection(req, selections)
	case model.DataAssetArchiveTypeSevenZip:
		return extractSevenZipSelection(req, selections)
	default:
		return archiveExtractionResult{}, fmt.Errorf("unsupported archive type %q", req.archive.Type)
	}
}

type archiveSelection struct {
	member   string
	as       string
	required bool
}

func validatedArchiveSelections(selectors []model.DataAssetArchiveSelect) ([]archiveSelection, error) {
	selections := make([]archiveSelection, 0, len(selectors))
	members := map[string]struct{}{}
	outputs := map[string]struct{}{}
	for i, selector := range selectors {
		member, err := cleanDataRelativePath(selector.Member)
		if err != nil {
			return nil, fmt.Errorf("archive select %d member: %w", i, err)
		}
		as := selector.As
		if as == "" {
			as = member
		}
		as, err = cleanDataRelativePath(as)
		if err != nil {
			return nil, fmt.Errorf("archive select %d as: %w", i, err)
		}
		if _, ok := members[member]; ok {
			return nil, fmt.Errorf("duplicate archive member selection %q", member)
		}
		if _, ok := outputs[as]; ok {
			return nil, fmt.Errorf("duplicate archive output path %q", as)
		}
		members[member] = struct{}{}
		outputs[as] = struct{}{}
		selections = append(selections, archiveSelection{
			member:   member,
			as:       as,
			required: archiveSelectionRequired(selector),
		})
	}
	return selections, nil
}

func archiveSelectionRequired(selector model.DataAssetArchiveSelect) bool {
	if selector.Required == nil {
		return true
	}
	return *selector.Required
}

func prepareExtractionRoot(root string) error {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return fmt.Errorf("resolve archive extraction root %s: %w", root, err)
	}
	if err := os.RemoveAll(rootAbs); err != nil {
		return fmt.Errorf("clear archive extraction root %s: %w", rootAbs, err)
	}
	return os.MkdirAll(rootAbs, 0755)
}

func extractZIPSelection(req archiveExtractionRequest, selections []archiveSelection) (archiveExtractionResult, error) {
	reader, err := zip.OpenReader(req.sourcePath)
	if err != nil {
		return archiveExtractionResult{}, fmt.Errorf("open zip archive %s: %w", req.sourcePath, err)
	}
	defer reader.Close()

	byMember := make(map[string]archiveSelection, len(selections))
	for _, selection := range selections {
		byMember[selection.member] = selection
	}

	extracted := map[string]model.MaterializedArchiveMember{}
	for _, file := range reader.File {
		member, err := cleanDataRelativePath(file.Name)
		if err != nil {
			return archiveExtractionResult{}, fmt.Errorf("zip entry %q: %w", file.Name, err)
		}
		selection, ok := byMember[member]
		if !ok {
			continue
		}
		if file.FileInfo().IsDir() {
			return archiveExtractionResult{}, fmt.Errorf("selected zip member %q is a directory", member)
		}

		outputPath, err := pathInsideRoot(req.extractionRoot, selection.as)
		if err != nil {
			return archiveExtractionResult{}, err
		}
		evidence, err := extractZipFile(file, outputPath, req.maxSelectedFileSize)
		if err != nil {
			return archiveExtractionResult{}, fmt.Errorf("extract zip member %q: %w", member, err)
		}
		size := evidence.size
		extracted[member] = model.MaterializedArchiveMember{
			Member:    member,
			LocalPath: outputPath,
			SizeBytes: &size,
			SHA256:    evidence.sha256,
		}
	}

	return archiveSelectionResult(req, selections, extracted)
}

func extractZipFile(file *zip.File, outputPath string, limit int64) (assetEvidence, error) {
	source, err := file.Open()
	if err != nil {
		return assetEvidence{}, err
	}
	defer source.Close()
	return writeStreamAtomically(outputPath, source, limit)
}

func extractSevenZipSelection(req archiveExtractionRequest, selections []archiveSelection) (archiveExtractionResult, error) {
	executable := strings.TrimSpace(req.sevenZipExecutable)
	if executable == "" {
		return archiveExtractionResult{}, fmt.Errorf("archive type seven_zip requires configured seven_zip_executable")
	}

	args := []string{"x", "-y", "-o" + req.extractionRoot, req.sourcePath}
	for _, selection := range selections {
		args = append(args, selection.member)
	}
	command := exec.Command(executable, args...)
	output, err := command.CombinedOutput()
	if err != nil {
		return archiveExtractionResult{}, fmt.Errorf("run seven_zip extractor %s: %w: %s", executable, err, strings.TrimSpace(string(output)))
	}

	extracted := map[string]model.MaterializedArchiveMember{}
	for _, selection := range selections {
		outputPath, err := pathInsideRoot(req.extractionRoot, selection.as)
		if err != nil {
			return archiveExtractionResult{}, err
		}
		extractedMemberPath, err := pathInsideRoot(req.extractionRoot, selection.member)
		if err != nil {
			return archiveExtractionResult{}, err
		}
		if filepath.Clean(extractedMemberPath) != filepath.Clean(outputPath) {
			if _, err := os.Stat(extractedMemberPath); err != nil {
				if selection.required {
					return archiveExtractionResult{}, fmt.Errorf("required seven_zip member %q was not extracted: %w", selection.member, err)
				}
				continue
			}
			if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
				return archiveExtractionResult{}, fmt.Errorf("create seven_zip output parent %s: %w", filepath.Dir(outputPath), err)
			}
			if err := os.Rename(extractedMemberPath, outputPath); err != nil {
				return archiveExtractionResult{}, fmt.Errorf("move seven_zip member %q to selected path: %w", selection.member, err)
			}
		}
		evidence, err := hashFileWithLimit(outputPath, req.maxSelectedFileSize)
		if err != nil {
			if selection.required {
				return archiveExtractionResult{}, fmt.Errorf("required seven_zip member %q was not extracted: %w", selection.member, err)
			}
			continue
		}
		size := evidence.size
		extracted[selection.member] = model.MaterializedArchiveMember{
			Member:    selection.member,
			LocalPath: outputPath,
			SizeBytes: &size,
			SHA256:    evidence.sha256,
		}
	}

	return archiveSelectionResult(req, selections, extracted)
}

func archiveSelectionResult(req archiveExtractionRequest, selections []archiveSelection, extracted map[string]model.MaterializedArchiveMember) (archiveExtractionResult, error) {
	members := make([]model.MaterializedArchiveMember, 0, len(extracted))
	var selectedPath string
	var selectedEvidence assetEvidence

	for _, selection := range selections {
		member, ok := extracted[selection.member]
		if !ok {
			if selection.required {
				return archiveExtractionResult{}, fmt.Errorf("required archive member %q was not found", selection.member)
			}
			continue
		}
		members = append(members, member)
		if req.archive.Expose == model.DataAssetArchiveExposeSelectedPath && selection.required {
			selectedPath = member.LocalPath
			selectedEvidence = assetEvidence{size: *member.SizeBytes, sha256: member.SHA256}
		}
	}

	switch req.archive.Expose {
	case model.DataAssetArchiveExposeSelectedPath:
		if selectedPath == "" {
			return archiveExtractionResult{}, fmt.Errorf("archive expose selected_path requires one extracted required member")
		}
		return archiveExtractionResult{localPath: selectedPath, members: members, selected: selectedEvidence}, nil
	case "", model.DataAssetArchiveExposeSelectedDirectory:
		evidence, err := directoryManifestEvidence(req.extractionRoot)
		if err != nil {
			return archiveExtractionResult{}, err
		}
		return archiveExtractionResult{localPath: req.extractionRoot, members: members, selected: evidence}, nil
	default:
		return archiveExtractionResult{}, fmt.Errorf("unsupported archive expose %q", req.archive.Expose)
	}
}

func pathInsideRoot(root string, relativePath string) (string, error) {
	safe, err := cleanDataRelativePath(relativePath)
	if err != nil {
		return "", err
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("resolve extraction root %s: %w", root, err)
	}
	candidate := filepath.Join(rootAbs, filepath.FromSlash(safe))
	rel, err := filepath.Rel(rootAbs, candidate)
	if err != nil {
		return "", fmt.Errorf("resolve extracted path %s: %w", candidate, err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("archive output path escapes extraction root: %s", relativePath)
	}
	return candidate, nil
}

func directoryManifestEvidence(root string) (assetEvidence, error) {
	entries := []map[string]any{}
	var total int64
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return assetEvidence{}, fmt.Errorf("resolve directory evidence root %s: %w", root, err)
	}
	if err := filepath.WalkDir(rootAbs, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(rootAbs, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if _, err := cleanDataRelativePath(rel); err != nil {
			return err
		}
		hash, err := streamingFileSHA256(path)
		if err != nil {
			return err
		}
		total += info.Size()
		entries = append(entries, map[string]any{
			"path":       rel,
			"size_bytes": info.Size(),
			"sha256":     hash,
		})
		return nil
	}); err != nil {
		return assetEvidence{}, fmt.Errorf("compute directory manifest evidence for %s: %w", rootAbs, err)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i]["path"].(string) < entries[j]["path"].(string) })
	_, hash, err := fp.CanonicalJSONSHA256(entries)
	if err != nil {
		return assetEvidence{}, fmt.Errorf("hash directory manifest evidence: %w", err)
	}
	return assetEvidence{size: total, sha256: hash}, nil
}

func streamingFileSHA256(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open file for sha256 %s: %w", path, err)
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", fmt.Errorf("hash file %s: %w", path, err)
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}
