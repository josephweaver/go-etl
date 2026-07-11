//go:build gdal
// +build gdal

package geospatial

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const rasterInfoPathRole = "input"

var epsgRegex = regexp.MustCompile(`(?i)EPSG[" ]*,?[" ]*([0-9]+)|(?i)ID\["EPSG",\s*([0-9]+)\]`)

type gdalInfo struct {
	Driver       any        `json:"driver"`
	Size         []int      `json:"size"`
	Bands        []gdalBand `json:"bands"`
	GeoTransform []float64  `json:"geoTransform"`
}

type gdalDriver struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	ShortName   string `json:"shortName"`
}

type gdalBand struct {
	Band        int      `json:"band"`
	DataType    string   `json:"type"`
	NoDataValue *float64 `json:"noDataValue"`
}

func CollectRasterMetadata(inputs map[string]InputSpec) ([]RasterMetadata, error) {
	names := make([]string, 0, len(inputs))
	for name := range inputs {
		names = append(names, name)
	}
	sort.Strings(names)

	metadata := make([]RasterMetadata, 0, len(names))
	for _, name := range names {
		spec := inputs[name]
		rasterMetadata, err := collectRasterMetadata(name, spec)
		if err != nil {
			return nil, fmt.Errorf("input %q: %w", name, err)
		}
		metadata = append(metadata, rasterMetadata)
	}
	return metadata, nil
}

func collectRasterMetadata(name string, spec InputSpec) (RasterMetadata, error) {
	rawInfo, rawDoc, _, wkt, err := gdalInfoJSON(spec.Path)
	if err != nil {
		return RasterMetadata{}, fmt.Errorf("read raster metadata: %w", err)
	}

	meta, err := parseGDALInfo(rawInfo, rawDoc)
	if err != nil {
		return RasterMetadata{}, err
	}

	wktPresent := strings.TrimSpace(wkt) != ""
	epsg := 0
	if wktPresent {
		epsg = parseEPSGFromWKT(wkt)
	}
	bounds, err := computeBounds(meta.GeoTransform, meta.Width, meta.Height)
	if err != nil {
		return RasterMetadata{}, err
	}

	bands := make([]RasterBandMetadata, 0, len(meta.Bands))
	for _, band := range meta.Bands {
		bands = append(bands, RasterBandMetadata{
			Index:  band.Band,
			DType:  band.DataType,
			Nodata: resolveNodata(spec.Nodata, band.NoDataValue),
		})
	}

	return RasterMetadata{
		Name:          name,
		PathRole:      rasterInfoPathRole,
		Driver:        meta.Driver,
		Width:         meta.Width,
		Height:        meta.Height,
		BandCount:     len(meta.Bands),
		CRSWKTPresent: wktPresent,
		CRSWKT:        strings.TrimSpace(wkt),
		EPSG:          epsg,
		GeoTransform:  meta.GeoTransform,
		Bounds:        bounds,
		Bands:         bands,
	}, nil
}

func gdalInfoJSON(rasterPath string) (gdalInfo, map[string]any, []byte, string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "gdalinfo", "-json", rasterPath)
	rawOutput, err := cmd.CombinedOutput()
	if err != nil {
		return gdalInfo{}, nil, rawOutput, "", fmt.Errorf("%w: %s", err, strings.TrimSpace(string(rawOutput)))
	}

	var rawDoc map[string]any
	if err := json.Unmarshal(rawOutput, &rawDoc); err != nil {
		return gdalInfo{}, nil, nil, "", fmt.Errorf("parse gdalinfo json: %w", err)
	}

	var info gdalInfo
	if err := json.Unmarshal(rawOutput, &info); err != nil {
		return gdalInfo{}, nil, nil, "", fmt.Errorf("parse gdalinfo json: %w", err)
	}

	wkt := findWKTFromJSON(rawDoc)
	return info, rawDoc, rawOutput, wkt, nil
}

func parseGDALInfo(info gdalInfo, rawDoc map[string]any) (rasterInfo, error) {
	if len(info.Size) != 2 {
		return rasterInfo{}, fmt.Errorf("unexpected raster size %v", info.Size)
	}
	width, height := info.Size[0], info.Size[1]
	if width <= 0 || height <= 0 {
		return rasterInfo{}, fmt.Errorf("invalid raster size [%d, %d]", width, height)
	}
	if len(info.GeoTransform) != 6 {
		return rasterInfo{}, fmt.Errorf("unexpected geoTransform %v", info.GeoTransform)
	}

	driverName := resolveGDALDriverNameFromJSON(rawDoc, info.Driver)
	return rasterInfo{
		Driver:       driverName,
		Width:        width,
		Height:       height,
		Bands:        info.Bands,
		GeoTransform: info.GeoTransform,
	}, nil
}

func resolveGDALDriverName(driver gdalDriver) string {
	if strings.TrimSpace(driver.Name) != "" {
		return driver.Name
	}
	if strings.TrimSpace(driver.ShortName) != "" {
		return driver.ShortName
	}
	return strings.TrimSpace(driver.Description)
}

func resolveGDALDriverNameFromJSON(rawDoc map[string]any, rawDriver any) string {
	switch typed := rawDriver.(type) {
	case map[string]any:
		if name := resolveGDALDriverNameFromMap(typed); name != "" {
			return name
		}
	case string:
		if s := strings.TrimSpace(typed); s != "" {
			return s
		}
	}

	if s := strings.TrimSpace(lookupStringCaseInsensitive(rawDoc, "driverShortName")); s != "" {
		return s
	}
	if s := strings.TrimSpace(lookupStringCaseInsensitive(rawDoc, "driver_short_name")); s != "" {
		return s
	}
	if s := strings.TrimSpace(lookupStringCaseInsensitive(rawDoc, "driverName")); s != "" {
		return s
	}
	if s := strings.TrimSpace(lookupStringCaseInsensitive(rawDoc, "driver_name")); s != "" {
		return s
	}
	if s := strings.TrimSpace(lookupStringCaseInsensitive(rawDoc, "driverLongName")); s != "" {
		return s
	}
	if s := strings.TrimSpace(lookupStringCaseInsensitive(rawDoc, "driver_long_name")); s != "" {
		return s
	}
	if s := strings.TrimSpace(lookupStringCaseInsensitive(rawDoc, "shortName")); s != "" {
		return s
	}
	if s := strings.TrimSpace(lookupStringCaseInsensitive(rawDoc, "short_name")); s != "" {
		return s
	}
	if s := strings.TrimSpace(lookupStringCaseInsensitive(rawDoc, "name")); s != "" {
		return s
	}
	if s := strings.TrimSpace(lookupStringCaseInsensitive(rawDoc, "description")); s != "" {
		return s
	}

	if typed, ok := rawDoc["driver"].(map[string]any); ok {
		if nested := resolveGDALDriverNameFromMap(typed); nested != "" {
			return nested
		}
	}

	return resolveGDALDriverName(gdalDriver{})
}

func lookupStringCaseInsensitive(value map[string]any, key string) string {
	if value == nil {
		return ""
	}

	if raw, ok := value[key]; ok {
		return toString(raw)
	}

	lower := strings.ToLower(key)
	for k, raw := range value {
		if strings.EqualFold(k, key) || strings.ToLower(k) == lower {
			return toString(raw)
		}
	}
	return ""
}

func resolveGDALDriverNameFromMap(typed map[string]any) string {
	for _, key := range []string{"name", "shortName", "short_name", "description", "longName", "long_name", "value"} {
		if raw, ok := typed[key]; ok {
			if s := strings.TrimSpace(toString(raw)); s != "" {
				return s
			}
		}
	}
	return ""
}

func toString(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case fmt.Stringer:
		return typed.String()
	default:
		return ""
	}
}

type rasterInfo struct {
	Driver       string
	Width        int
	Height       int
	Bands        []gdalBand
	GeoTransform []float64
}

func parseEPSGFromWKT(wkt string) int {
	matches := epsgRegex.FindAllStringSubmatchIndex(wkt, -1)
	for _, match := range matches {
		if match[0] < 0 || match[1] < 0 {
			continue
		}
		if wktBracketDepthAt(wkt, match[0]) != 1 {
			continue
		}
		if strings.Trim(strings.TrimSpace(wkt[match[1]:]), "]") != "" {
			continue
		}
		for index := 2; index+1 < len(match); index += 2 {
			if match[index] < 0 || match[index+1] < 0 {
				continue
			}
			candidate := wkt[match[index]:match[index+1]]
			if candidate == "" {
				continue
			}
			value, err := strconv.Atoi(candidate)
			if err != nil {
				continue
			}
			return value
		}
	}
	return 0
}

func wktBracketDepthAt(wkt string, offset int) int {
	depth := 0
	for index, char := range wkt {
		if index >= offset {
			break
		}
		switch char {
		case '[':
			depth++
		case ']':
			if depth > 0 {
				depth--
			}
		}
	}
	return depth
}

func computeBounds(gt []float64, width int, height int) (RasterBounds, error) {
	if len(gt) != 6 {
		return RasterBounds{}, fmt.Errorf("geoTransform must include 6 values")
	}

	widthF := float64(width)
	heightF := float64(height)

	candidates := [][2]float64{
		{gt[0], gt[3]},
		{gt[0] + widthF*gt[1] + heightF*gt[2], gt[3] + widthF*gt[4] + heightF*gt[5]},
		{gt[0] + heightF*gt[2], gt[3] + heightF*gt[5]},
		{gt[0] + widthF*gt[1], gt[3] + widthF*gt[4]},
	}

	minX := candidates[0][0]
	maxX := candidates[0][0]
	minY := candidates[0][1]
	maxY := candidates[0][1]

	for _, candidate := range candidates[1:] {
		minX = math.Min(minX, candidate[0])
		maxX = math.Max(maxX, candidate[0])
		minY = math.Min(minY, candidate[1])
		maxY = math.Max(maxY, candidate[1])
	}

	return RasterBounds{
		MinX: minX,
		MinY: minY,
		MaxX: maxX,
		MaxY: maxY,
	}, nil
}

func findWKTFromJSON(value any) string {
	switch typed := value.(type) {
	case map[string]any:
		for key, item := range typed {
			if strings.EqualFold(key, "wkt") {
				if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
					return s
				}
			}
			if nested := findWKTFromJSON(item); nested != "" {
				return nested
			}
		}
	case []any:
		for _, item := range typed {
			if nested := findWKTFromJSON(item); nested != "" {
				return nested
			}
		}
	}
	return ""
}

func resolveNodata(nodataOverride *int, noDataFromBand *float64) *int {
	if nodataOverride != nil {
		return nodataOverride
	}
	if noDataFromBand == nil {
		return nil
	}

	rounded := math.Trunc(*noDataFromBand)
	if rounded != *noDataFromBand {
		return nil
	}

	result := int(rounded)
	return &result
}
