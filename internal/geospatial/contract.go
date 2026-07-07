package geospatial

const (
	APIVersionV1Alpha1 = "goet.geospatial/v1alpha1"

	RequestKind = "GeospatialOperationRequest"
	ResultKind  = "GeospatialOperationResult"

	OperationValidate = "validate"
	OperationVersion  = "version"
)

type OperationRequest struct {
	APIVersion string               `json:"api_version"`
	Kind       string               `json:"kind"`
	Operation  string               `json:"operation"`
	Inputs     map[string]InputSpec `json:"inputs"`
	Outputs    map[string]string    `json:"outputs"`
	Options    map[string]any       `json:"options"`
}

type InputSpec struct {
	Path   string `json:"path"`
	Band   *int   `json:"band,omitempty"`
	Nodata *int   `json:"nodata,omitempty"`
}

type OperationResult struct {
	APIVersion string           `json:"api_version"`
	Kind       string           `json:"kind"`
	Operation  string           `json:"operation"`
	Artifacts  []ArtifactResult `json:"artifacts"`
	Summary    map[string]any   `json:"summary"`
	Warnings   []string         `json:"warnings"`
}

type ArtifactResult struct {
	Name   string `json:"name"`
	Path   string `json:"path"`
	Kind   string `json:"kind,omitempty"`
	Format string `json:"format,omitempty"`
}

func NewValidationResult(operation string) OperationResult {
	return OperationResult{
		APIVersion: APIVersionV1Alpha1,
		Kind:       ResultKind,
		Operation:  operation,
		Artifacts:  []ArtifactResult{},
		Summary:    map[string]any{},
		Warnings:   []string{},
	}
}
