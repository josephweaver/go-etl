package geospatial

const (
	APIVersionV1Alpha1 = "goet.geospatial/v1alpha1"

	RequestKind = "GeospatialOperationRequest"
	ResultKind  = "GeospatialOperationResult"

	OperationValidate     = "validate"
	OperationVersion      = "version"
	OperationRasterInfo   = "raster_info"
	OperationReprojectCRS = "reproject_crs"
	OperationAlignToGrid  = "align_to_grid"
	OperationStackAligned = "stack_aligned_rasters"
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
	Path       string `json:"path"`
	Band       *int   `json:"band,omitempty"`
	Nodata     *int   `json:"nodata,omitempty"`
	OutputBand *int   `json:"output_band,omitempty"`
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

type RasterMetadata struct {
	Name          string               `json:"name"`
	PathRole      string               `json:"path_role"`
	Driver        string               `json:"driver"`
	Width         int                  `json:"width"`
	Height        int                  `json:"height"`
	BandCount     int                  `json:"band_count"`
	CRSWKTPresent bool                 `json:"crs_wkt_present"`
	EPSG          int                  `json:"epsg"`
	GeoTransform  []float64            `json:"geo_transform"`
	Bounds        RasterBounds         `json:"bounds"`
	Bands         []RasterBandMetadata `json:"bands"`
}

type RasterBounds struct {
	MinX float64 `json:"min_x"`
	MinY float64 `json:"min_y"`
	MaxX float64 `json:"max_x"`
	MaxY float64 `json:"max_y"`
}

type RasterBandMetadata struct {
	Index  int    `json:"index"`
	DType  string `json:"dtype"`
	Nodata *int   `json:"nodata,omitempty"`
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
