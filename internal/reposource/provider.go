package reposource

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

const LocalProvenanceWarning = "Local source files do not provide source-control authenticity. Use a source-control provider when provenance must be verifiable."

// Provider resolves one source and reads explicitly requested files from it.
type Provider interface {
	Resolve(ctx context.Context, requestedRef string) (ResolvedSourceReference, error)
	ReadFiles(ctx context.Context, resolved ResolvedSourceReference, paths []string) ([]ReadFileResult, error)
}

// ReadFileResult is one provider-read file plus provider and hash evidence.
type ReadFileResult struct {
	Request   SourceFileRequest
	Content   SourceFileContent
	RawSHA256 string
	SizeBytes int64
}

func newReadFileResult(request SourceFileRequest, data []byte, objectID *string) ReadFileResult {
	raw := sha256.Sum256(data)
	copied := make([]byte, len(data))
	copy(copied, data)
	return ReadFileResult{
		Request: request,
		Content: SourceFileContent{
			Data:     copied,
			ObjectID: objectID,
		},
		RawSHA256: hex.EncodeToString(raw[:]),
		SizeBytes: int64(len(data)),
	}
}

func sourceFileRequests(repository RepositoryIdentity, revisionID *string, paths []string) ([]SourceFileRequest, error) {
	requests := make([]SourceFileRequest, 0, len(paths))
	for _, value := range paths {
		sourcePath, err := ValidateRepositoryRelativePath(value)
		if err != nil {
			return nil, fmt.Errorf("validate source path %q: %w", value, err)
		}
		requests = append(requests, SourceFileRequest{
			Repository: repository,
			RevisionID: revisionID,
			SourcePath: sourcePath,
		})
	}
	return requests, nil
}
