//go:build gdal
// +build gdal

package geospatial

import (
	"bufio"
	"context"
	"fmt"
	"math"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

const (
	gdalByteDType   = "Byte"
	gdalInt32DType  = "Int32"
	gdalUInt16DType = "UInt16"
	gdalUInt32DType = "UInt32"
)

func init() {
	executeRasterPairValueCountsImpl = executeRasterPairValueCountsGDAL
}

func executeRasterPairValueCountsGDAL(ctx context.Context, request RasterPairValueCountsRequest) (pairCountExecutionResult, error) {
	fieldMetadata, fieldBandMetadata, valueMetadata, valueBandMetadata, err := resolveRasterPairMetadata(request)
	if err != nil {
		return pairCountExecutionResult{}, err
	}

	if request.RequireAlignedGrid && !GridsEqual(GridFromMetadata(fieldMetadata), GridFromMetadata(valueMetadata)) {
		return pairCountExecutionResult{}, fmt.Errorf("field and value rasters are not aligned")
	}

	fieldNodata, err := resolveRasterPairNodata(request.Field, fieldBandMetadata)
	if err != nil {
		return pairCountExecutionResult{}, err
	}
	valueNodata, err := resolveRasterPairNodata(request.Value, valueBandMetadata)
	if err != nil {
		return pairCountExecutionResult{}, err
	}

	width := fieldMetadata.Width
	height := fieldMetadata.Height
	accumulator := newPairCountAccumulator(request.IncludeValueNodata)

	for yOffset := 0; yOffset < height; yOffset += request.ChunkRows {
		windowHeight := request.ChunkRows
		if remaining := height - yOffset; remaining < windowHeight {
			windowHeight = remaining
		}

		fieldValues, err := readUInt32RasterWindowXYZ(ctx, request.Field.Path, request.Field.Band, width, windowHeight, yOffset)
		if err != nil {
			return pairCountExecutionResult{}, fmt.Errorf("read field raster window at row %d: %w", yOffset, err)
		}
		valueValues, err := readUInt16RasterWindowXYZ(ctx, request.Value.Path, request.Value.Band, width, windowHeight, yOffset)
		if err != nil {
			return pairCountExecutionResult{}, fmt.Errorf("read value raster window at row %d: %w", yOffset, err)
		}

		if err := accumulator.AddChunk(fieldValues, valueValues, uint32(fieldNodata), uint16(valueNodata)); err != nil {
			return pairCountExecutionResult{}, err
		}
	}

	return pairCountExecutionResult{
		rows:     accumulator.Rows(),
		metadata: accumulator.Metadata(),
	}, nil
}

func resolveRasterPairMetadata(request RasterPairValueCountsRequest) (RasterMetadata, RasterBandMetadata, RasterMetadata, RasterBandMetadata, error) {
	fieldMetadata, err := collectSingleRasterMetadata(request.Field.Name, InputSpec{Path: request.Field.Path})
	if err != nil {
		return RasterMetadata{}, RasterBandMetadata{}, RasterMetadata{}, RasterBandMetadata{}, fmt.Errorf("collect metadata for %q: %w", request.Field.Name, err)
	}
	fieldBandMetadata, err := rasterPairBandMetadata(request.Field, fieldMetadata, request.FieldDType)
	if err != nil {
		return RasterMetadata{}, RasterBandMetadata{}, RasterMetadata{}, RasterBandMetadata{}, err
	}

	valueMetadata := fieldMetadata
	valueBandMetadata := fieldBandMetadata
	if request.Field.Path != request.Value.Path || request.Field.Band != request.Value.Band {
		valueMetadata, err = collectSingleRasterMetadata(request.Value.Name, InputSpec{Path: request.Value.Path})
		if err != nil {
			return RasterMetadata{}, RasterBandMetadata{}, RasterMetadata{}, RasterBandMetadata{}, fmt.Errorf("collect metadata for %q: %w", request.Value.Name, err)
		}
		valueBandMetadata, err = rasterPairBandMetadata(request.Value, valueMetadata, request.ValueDType)
		if err != nil {
			return RasterMetadata{}, RasterBandMetadata{}, RasterMetadata{}, RasterBandMetadata{}, err
		}
	}

	if fieldMetadata.Width != valueMetadata.Width || fieldMetadata.Height != valueMetadata.Height {
		return RasterMetadata{}, RasterBandMetadata{}, RasterMetadata{}, RasterBandMetadata{}, fmt.Errorf("field and value rasters have different dimensions")
	}

	return fieldMetadata, fieldBandMetadata, valueMetadata, valueBandMetadata, nil
}

func rasterPairBandMetadata(source rasterPairBandSource, metadata RasterMetadata, wantDType string) (RasterBandMetadata, error) {
	if source.Band > len(metadata.Bands) {
		return RasterBandMetadata{}, fmt.Errorf("input %q band %d does not exist", source.Name, source.Band)
	}
	bandMetadata := metadata.Bands[source.Band-1]
	if !rasterPairDTypeCompatible(bandMetadata.DType, wantDType) {
		return RasterBandMetadata{}, fmt.Errorf("input %q dtype %q is not compatible with requested dtype %q", source.Name, bandMetadata.DType, wantDType)
	}
	return bandMetadata, nil
}

func rasterPairDTypeCompatible(gdalDType string, wantDType string) bool {
	switch wantDType {
	case "byte":
		return gdalDType == gdalByteDType
	case "uint16":
		return gdalDType == gdalUInt16DType
	case "uint32":
		return gdalDType == gdalUInt32DType
	case "int32":
		return gdalDType == gdalInt32DType
	default:
		return false
	}
}

func resolveRasterPairNodata(source rasterPairBandSource, bandMetadata RasterBandMetadata) (int, error) {
	nodata := firstNonNilInt(source.Nodata, bandMetadata.Nodata)
	if nodata == nil {
		return 0, fmt.Errorf("input %q nodata is required when band metadata does not declare nodata", source.Name)
	}
	if err := validateRasterPairNodata(source.Name+".nodata", nodata); err != nil {
		return 0, err
	}
	return *nodata, nil
}

func readUInt16RasterWindowXYZ(ctx context.Context, rasterPath string, band int, width int, height int, yOffset int) ([]uint16, error) {
	values, err := readUintRasterWindowXYZ(ctx, rasterPath, band, width, height, yOffset, uint16MaxValue, "uint16")
	if err != nil {
		return nil, err
	}
	narrowed := make([]uint16, len(values))
	for index, value := range values {
		narrowed[index] = uint16(value)
	}
	return narrowed, nil
}

func readUInt32RasterWindowXYZ(ctx context.Context, rasterPath string, band int, width int, height int, yOffset int) ([]uint32, error) {
	return readUintRasterWindowXYZ(ctx, rasterPath, band, width, height, yOffset, uint32MaxValue, "uint32")
}

func readUintRasterWindowXYZ(ctx context.Context, rasterPath string, band int, width int, height int, yOffset int, maxValue uint64, dtypeName string) ([]uint32, error) {
	windowCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	args := []string{
		"-q",
		"-of", "XYZ",
		"-b", strconv.Itoa(band),
		"-srcwin", "0", strconv.Itoa(yOffset), strconv.Itoa(width), strconv.Itoa(height),
		rasterPath,
		"/vsistdout/",
	}

	cmd := exec.CommandContext(windowCtx, "gdal_translate", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("gdal_translate: %w: %s", err, strings.TrimSpace(string(output)))
	}

	values := make([]uint32, 0, width*height)
	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 3 {
			return nil, fmt.Errorf("unexpected XYZ line %q", line)
		}
		value, err := parseUintXYZValue(fields[2], maxValue, dtypeName)
		if err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read XYZ output: %w", err)
	}

	expected := width * height
	if len(values) != expected {
		return nil, fmt.Errorf("window returned %d values, want %d", len(values), expected)
	}
	return values, nil
}

func parseUInt16XYZValue(raw string) (uint16, error) {
	value, err := parseUintXYZValue(raw, uint16MaxValue, "uint16")
	if err != nil {
		return 0, err
	}
	return uint16(value), nil
}

func parseUintXYZValue(raw string, maxValue uint64, dtypeName string) (uint32, error) {
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0, fmt.Errorf("parse raster value %q: %w", raw, err)
	}
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, fmt.Errorf("raster value %q is not finite", raw)
	}
	if value < 0 || value > float64(maxValue) {
		return 0, fmt.Errorf("raster value %q is out of range for dtype %s", raw, dtypeName)
	}
	if math.Trunc(value) != value {
		return 0, fmt.Errorf("raster value %q is not an integer compatible with dtype %s", raw, dtypeName)
	}
	return uint32(value), nil
}
