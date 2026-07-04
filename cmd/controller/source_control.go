package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const localUnversionedCommit = "local-unversioned"

type SourceControlAdapter interface {
	Resolve(ctx context.Context, ref SourceDocumentReference) (ResolvedSourceDocument, error)
}

type SourceDocumentReference struct {
	Repository string `json:"repository"`
	Ref        string `json:"ref"`
	Path       string `json:"path"`
}

type ResolvedSourceDocument struct {
	RepositoryIdentity string
	RequestedRef       string
	ResolvedCommit     string
	Path               string
	SourceObjectID     string
	Data               []byte
}

type LocalSourceControlAdapter struct {
	roots map[string]string
}

func NewLocalSourceControlAdapter(roots map[string]string) LocalSourceControlAdapter {
	copied := make(map[string]string, len(roots))
	for repository, root := range roots {
		copied[repository] = root
	}
	return LocalSourceControlAdapter{roots: copied}
}

func (a LocalSourceControlAdapter) Resolve(ctx context.Context, ref SourceDocumentReference) (ResolvedSourceDocument, error) {
	root, ok := a.roots[ref.Repository]
	if !ok {
		return ResolvedSourceDocument{}, fmt.Errorf("unknown source repository: %s", ref.Repository)
	}
	if ref.Ref == "" {
		return ResolvedSourceDocument{}, fmt.Errorf("source ref is required")
	}

	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return ResolvedSourceDocument{}, fmt.Errorf("resolve repository root: %w", err)
	}
	documentPath, err := safeRepositoryPath(rootAbs, ref.Path)
	if err != nil {
		return ResolvedSourceDocument{}, err
	}

	data, err := os.ReadFile(documentPath)
	if err != nil {
		return ResolvedSourceDocument{}, fmt.Errorf("read source document: %w", err)
	}

	resolvedCommit := localUnversionedCommit
	sourceObjectID := sha256HexBytes(data)
	if gitRepository(ctx, rootAbs) {
		commit, err := gitOutput(ctx, rootAbs, "rev-parse", ref.Ref+"^{commit}")
		if err != nil {
			return ResolvedSourceDocument{}, fmt.Errorf("resolve git ref %s: %w", ref.Ref, err)
		}
		resolvedCommit = commit

		objectID, err := gitOutput(ctx, rootAbs, "rev-parse", commit+":"+filepath.ToSlash(filepath.Clean(ref.Path)))
		if err == nil {
			sourceObjectID = objectID
		}
	}

	return ResolvedSourceDocument{
		RepositoryIdentity: ref.Repository,
		RequestedRef:       ref.Ref,
		ResolvedCommit:     resolvedCommit,
		Path:               filepath.ToSlash(filepath.Clean(ref.Path)),
		SourceObjectID:     sourceObjectID,
		Data:               data,
	}, nil
}

func safeRepositoryPath(root string, repositoryPath string) (string, error) {
	if repositoryPath == "" {
		return "", fmt.Errorf("source path is required")
	}
	if filepath.IsAbs(repositoryPath) {
		return "", fmt.Errorf("source path must be repository-relative")
	}

	clean := filepath.Clean(filepath.FromSlash(repositoryPath))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("source path escapes repository root")
	}

	candidate := filepath.Join(root, clean)
	candidateAbs, err := filepath.Abs(candidate)
	if err != nil {
		return "", fmt.Errorf("resolve source path: %w", err)
	}
	rel, err := filepath.Rel(root, candidateAbs)
	if err != nil {
		return "", fmt.Errorf("check source path: %w", err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("source path escapes repository root")
	}
	return candidateAbs, nil
}

func gitRepository(ctx context.Context, root string) bool {
	output, err := gitOutput(ctx, root, "rev-parse", "--is-inside-work-tree")
	return err == nil && output == "true"
}

func gitOutput(ctx context.Context, root string, args ...string) (string, error) {
	commandArgs := append([]string{"-C", root}, args...)
	output, err := exec.CommandContext(ctx, "git", commandArgs...).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%s: %w", strings.TrimSpace(string(output)), err)
	}
	return strings.TrimSpace(string(output)), nil
}
