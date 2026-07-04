package reposource

import "fmt"

const AdmittedSourceManifestSchemaV1 = "goet/admitted-source-manifest/v1"

// DeclaredSourceFile describes a caller-declared file before admission.
type DeclaredSourceFile struct {
	Role                FileRole
	SourcePath          string
	CachePath           string
	ContentType         string
	CanonicalJSONSHA256 *string
}

func BuildAdmittedSourceManifest(runID string, source ResolvedSourceReference, declared []DeclaredSourceFile, reads []ReadFileResult) (AdmittedSourceManifest, error) {
	if runID == "" {
		return AdmittedSourceManifest{}, fmt.Errorf("run id is required")
	}
	if source.Repository.Value == "" {
		return AdmittedSourceManifest{}, fmt.Errorf("source repository identity is required")
	}
	readByPath := make(map[string]ReadFileResult, len(reads))
	for _, read := range reads {
		sourcePath, err := ValidateRepositoryRelativePath(read.Request.SourcePath)
		if err != nil {
			return AdmittedSourceManifest{}, fmt.Errorf("validate read source path %q: %w", read.Request.SourcePath, err)
		}
		read.Request.SourcePath = sourcePath
		readByPath[sourcePath] = read
	}

	files := make([]AdmittedSourceManifestFile, 0, len(declared))
	seen := make(map[string]struct{}, len(declared))
	for _, item := range declared {
		if item.Role == "" {
			return AdmittedSourceManifest{}, fmt.Errorf("file role is required")
		}
		sourcePath, err := ValidateRepositoryRelativePath(item.SourcePath)
		if err != nil {
			return AdmittedSourceManifest{}, fmt.Errorf("validate declared source path %q: %w", item.SourcePath, err)
		}
		cachePath, err := ValidateCacheRelativePath(item.CachePath)
		if err != nil {
			return AdmittedSourceManifest{}, fmt.Errorf("validate declared cache path %q: %w", item.CachePath, err)
		}
		if _, ok := seen[sourcePath]; ok {
			return AdmittedSourceManifest{}, fmt.Errorf("duplicate declared source path %s", sourcePath)
		}
		seen[sourcePath] = struct{}{}
		read, ok := readByPath[sourcePath]
		if !ok {
			return AdmittedSourceManifest{}, fmt.Errorf("declared source file %s was not read", sourcePath)
		}
		rawSHA256 := read.RawSHA256
		files = append(files, AdmittedSourceManifestFile{
			Role:                item.Role,
			SourcePath:          sourcePath,
			CachePath:           cachePath,
			ObjectID:            read.Content.ObjectID,
			SizeBytes:           read.SizeBytes,
			RawSHA256:           &rawSHA256,
			CanonicalJSONSHA256: item.CanonicalJSONSHA256,
			ContentType:         item.ContentType,
		})
	}

	return AdmittedSourceManifest{
		Schema: AdmittedSourceManifestSchemaV1,
		RunID:  runID,
		Source: source,
		Files:  files,
	}, nil
}
