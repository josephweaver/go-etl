package reposource

// RepositoryIdentity identifies a source repository in controller-owned terms.
type RepositoryIdentity struct {
	Value       string
	DisplayName string
}

// ResolvedSourceReference captures the repository source the controller admitted.
type ResolvedSourceReference struct {
	Repository   RepositoryIdentity
	RequestedRef string
	RevisionID   *string
}

// SourceFileRequest identifies one repository-relative file to read.
type SourceFileRequest struct {
	Repository RepositoryIdentity
	RevisionID *string
	SourcePath string
}

// SourceFileContent carries admitted file bytes and optional provider identity.
type SourceFileContent struct {
	Data     []byte
	ObjectID *string
}

// FileRole describes why the controller admitted a file into a manifest.
type FileRole string

const (
	FileRoleProject           FileRole = "project"
	FileRoleWorkflow          FileRole = "workflow"
	FileRolePythonEntrypoint  FileRole = "python_entrypoint"
	FileRolePythonEnvironment FileRole = "python_environment"
	FileRoleSupportFile       FileRole = "support_file"
)

// AdmittedSourceManifest records the source facts the controller admitted.
type AdmittedSourceManifest struct {
	Schema string
	RunID  string
	Source ResolvedSourceReference
	Files  []AdmittedSourceManifestFile
}

// AdmittedSourceManifestFile records one file admitted into the controller cache.
type AdmittedSourceManifestFile struct {
	Role                FileRole
	SourcePath          string
	CachePath           string
	ObjectID            *string
	SizeBytes           int64
	RawSHA256           *string
	CanonicalJSONSHA256 *string
	ContentType         string
}
