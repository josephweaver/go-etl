package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"goetl/internal/geospatial"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	flags := flag.NewFlagSet("goet-geospatial", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)

	requestPath := flags.String("request", "", "path to geospatial operation request JSON")
	responsePath := flags.String("response", "", "path to write geospatial operation result JSON")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *requestPath == "" {
		return fmt.Errorf("--request is required")
	}
	if *responsePath == "" {
		return fmt.Errorf("--response is required")
	}

	requestData, err := os.ReadFile(*requestPath)
	if err != nil {
		return fmt.Errorf("read request %s: %w", *requestPath, err)
	}

	var envelope struct {
		Operation string `json:"operation"`
	}
	if err := json.Unmarshal(requestData, &envelope); err != nil {
		return fmt.Errorf("decode request %s: %w", *requestPath, err)
	}

	var result geospatial.OperationResult

	switch envelope.Operation {
	case geospatial.OperationRasterPairValueCounts:
		countResult, err := geospatial.ExecuteRasterPairValueCounts(context.Background(), requestData, filepath.Dir(*responsePath))
		if err != nil {
			return err
		}
		result = countResult
	case geospatial.OperationCropPolygons:
		cropResult, err := geospatial.ExecuteCropByPolygons(context.Background(), requestData, filepath.Dir(*responsePath))
		if err != nil {
			return err
		}
		result = cropResult
	case geospatial.OperationPolygonizeRaster:
		polygonizeResult, err := geospatial.ExecutePolygonizeRaster(context.Background(), requestData, filepath.Dir(*responsePath))
		if err != nil {
			return err
		}
		result = polygonizeResult
	case geospatial.OperationRasterInfo:
		var request geospatial.OperationRequest
		if err := json.Unmarshal(requestData, &request); err != nil {
			return fmt.Errorf("decode request %s: %w", *requestPath, err)
		}
		if err := request.Validate(); err != nil {
			return err
		}
		metadata, err := geospatial.CollectRasterMetadata(request.Inputs)
		if err != nil {
			return err
		}
		metadataPath := request.Outputs["metadata_json"]
		metadataArtifactPath := filepath.Join(filepath.Dir(*responsePath), metadataPath)
		if err := os.MkdirAll(filepath.Dir(metadataArtifactPath), 0755); err != nil {
			return err
		}
		metadataPayload := map[string]any{
			"rasters": metadata,
		}
		metadataData, err := json.MarshalIndent(metadataPayload, "", "  ")
		if err != nil {
			return err
		}
		metadataData = append(metadataData, '\n')
		if err := os.WriteFile(metadataArtifactPath, metadataData, 0644); err != nil {
			return err
		}

		result = geospatial.NewValidationResult(request.Operation)
		result.Artifacts = []geospatial.ArtifactResult{{
			Name:   "metadata_json",
			Path:   metadataPath,
			Kind:   "metadata",
			Format: "json",
		}}
		result.Summary = map[string]any{
			"rasters": metadata,
		}
	case geospatial.OperationAlignToGrid, geospatial.OperationReprojectCRS:
		var request geospatial.OperationRequest
		if err := json.Unmarshal(requestData, &request); err != nil {
			return fmt.Errorf("decode request %s: %w", *requestPath, err)
		}
		if err := request.Validate(); err != nil {
			return err
		}
		alignmentResult, err := geospatial.ExecuteRasterAlignment(context.Background(), request, filepath.Dir(*responsePath))
		if err != nil {
			return err
		}
		result = alignmentResult
	case geospatial.OperationStackAligned:
		var request geospatial.OperationRequest
		if err := json.Unmarshal(requestData, &request); err != nil {
			return fmt.Errorf("decode request %s: %w", *requestPath, err)
		}
		if err := request.Validate(); err != nil {
			return err
		}
		stackResult, err := geospatial.ExecuteStackAlignedRasters(context.Background(), request, filepath.Dir(*responsePath))
		if err != nil {
			return err
		}
		result = stackResult
	default:
		var request geospatial.OperationRequest
		if err := json.Unmarshal(requestData, &request); err != nil {
			return fmt.Errorf("decode request %s: %w", *requestPath, err)
		}
		if err := request.Validate(); err != nil {
			return err
		}
		result = geospatial.NewValidationResult(request.Operation)
	}

	if result.Summary == nil {
		result.Summary = map[string]any{}
	}

	responseData, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Errorf("encode response: %w", err)
	}
	responseData = append(responseData, '\n')
	if err := os.WriteFile(*responsePath, responseData, 0644); err != nil {
		return fmt.Errorf("write response %s: %w", *responsePath, err)
	}
	return nil
}
