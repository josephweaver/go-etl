package reposource

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

// LocalProvider reads declared files from a configured local source root.
type LocalProvider struct {
	repository RepositoryIdentity
	root       string
}

func NewLocalProvider(repository RepositoryIdentity, root string) LocalProvider {
	return LocalProvider{
		repository: repository,
		root:       root,
	}
}

func (p LocalProvider) Resolve(ctx context.Context, requestedRef string) (ResolvedSourceReference, error) {
	if err := ctx.Err(); err != nil {
		return ResolvedSourceReference{}, err
	}
	if p.repository.Value == "" {
		return ResolvedSourceReference{}, fmt.Errorf("repository identity is required")
	}
	if p.root == "" {
		return ResolvedSourceReference{}, fmt.Errorf("local source root is required")
	}
	return ResolvedSourceReference{
		Repository:   p.repository,
		RequestedRef: requestedRef,
		RevisionID:   nil,
	}, nil
}

func (p LocalProvider) ReadFiles(ctx context.Context, resolved ResolvedSourceReference, paths []string) ([]ReadFileResult, error) {
	requests, err := sourceFileRequests(resolved.Repository, resolved.RevisionID, paths)
	if err != nil {
		return nil, err
	}

	root, err := filepath.Abs(p.root)
	if err != nil {
		return nil, fmt.Errorf("resolve local source root: %w", err)
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return nil, fmt.Errorf("resolve local source root symlinks: %w", err)
	}

	results := make([]ReadFileResult, 0, len(requests))
	for _, request := range requests {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		sourcePath, err := localSourcePath(root, request.SourcePath)
		if err != nil {
			return nil, err
		}
		data, err := os.ReadFile(sourcePath)
		if err != nil {
			return nil, fmt.Errorf("read local source file %s: %w", request.SourcePath, err)
		}
		results = append(results, newReadFileResult(request, data, nil))
	}
	return results, nil
}

func (p LocalProvider) ProvenanceWarning() string {
	return LocalProvenanceWarning
}

func localSourcePath(root string, sourcePath string) (string, error) {
	candidate := filepath.Join(root, filepath.FromSlash(sourcePath))
	resolved, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		return "", fmt.Errorf("resolve local source file %s: %w", sourcePath, err)
	}
	rel, err := filepath.Rel(root, resolved)
	if err != nil {
		return "", fmt.Errorf("check local source file %s: %w", sourcePath, err)
	}
	if rel == ".." || filepath.IsAbs(rel) {
		return "", fmt.Errorf("source path escapes local source root")
	}
	if len(rel) > 3 && rel[:3] == ".."+string(filepath.Separator) {
		return "", fmt.Errorf("source path escapes local source root")
	}
	return resolved, nil
}
