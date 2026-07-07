package geospatial

import (
	"fmt"
	"strings"

	"goetl/internal/model"
)

func (request OperationRequest) Validate() error {
	if request.APIVersion != APIVersionV1Alpha1 {
		return fmt.Errorf("unsupported api_version %q", request.APIVersion)
	}
	if request.Kind != RequestKind {
		return fmt.Errorf("unsupported kind %q", request.Kind)
	}
	if !isSupportedOperation(request.Operation) {
		return fmt.Errorf("unsupported operation %q", request.Operation)
	}

	if request.Operation == OperationRasterInfo && len(request.Inputs) == 0 {
		return fmt.Errorf("raster_info operation requires at least one input")
	}
	if request.Operation == OperationRasterInfo {
		if _, ok := request.Outputs["metadata_json"]; !ok {
			return fmt.Errorf("raster_info operation requires output \"metadata_json\"")
		}
	}

	for name, input := range request.Inputs {
		if strings.TrimSpace(name) == "" {
			return fmt.Errorf("input name is required")
		}
		if strings.TrimSpace(input.Path) == "" {
			return fmt.Errorf("input %q path is required", name)
		}
		if input.Band != nil && *input.Band <= 0 {
			return fmt.Errorf("input %q band must be greater than 0", name)
		}
	}
	for name, outputPath := range request.Outputs {
		if strings.TrimSpace(name) == "" {
			return fmt.Errorf("output name is required")
		}
		if _, err := model.ValidateArtifactRelativePath(outputPath); err != nil {
			return fmt.Errorf("output %q path: %w", name, err)
		}
	}
	return nil
}

func isSupportedOperation(operation string) bool {
	switch operation {
	case OperationValidate, OperationVersion, OperationRasterInfo:
		return true
	default:
		return false
	}
}
