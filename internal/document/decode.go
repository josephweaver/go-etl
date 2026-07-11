package document

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"path/filepath"
	"strconv"
	"strings"
)

func DecodeSource(data []byte, options DecodeOptions) (any, error) {
	format, err := SourceFormatFor(options)
	if err != nil {
		return nil, err
	}

	switch format {
	case SourceFormatJSON:
		return decodeJSONSource(data, options.Path)
	case SourceFormatYAML:
		return decodeYAMLSource(data, options.Path)
	default:
		return nil, fmt.Errorf("unsupported source format %q", format)
	}
}

func SourceFormatFor(options DecodeOptions) (SourceFormat, error) {
	if options.Format != "" {
		switch options.Format {
		case SourceFormatJSON, SourceFormatYAML:
			return options.Format, nil
		default:
			return "", fmt.Errorf("unsupported source format %q", options.Format)
		}
	}

	if strings.TrimSpace(options.MediaType) != "" {
		format, ok := formatFromMediaType(options.MediaType)
		if !ok {
			return "", fmt.Errorf("unsupported source media type %q", options.MediaType)
		}
		return format, nil
	}

	switch strings.ToLower(filepath.Ext(options.Path)) {
	case ".json":
		return SourceFormatJSON, nil
	case ".yaml", ".yml":
		return SourceFormatYAML, nil
	default:
		return "", fmt.Errorf("source format requires an explicit format, media type, or .json/.yaml/.yml path")
	}
}

func formatFromMediaType(value string) (SourceFormat, bool) {
	mediaType, _, err := mime.ParseMediaType(value)
	if err != nil {
		mediaType = value
	}
	mediaType = strings.ToLower(strings.TrimSpace(mediaType))

	switch {
	case mediaType == "application/json",
		mediaType == "text/json",
		strings.HasSuffix(mediaType, "+json"):
		return SourceFormatJSON, true
	case mediaType == "application/yaml",
		mediaType == "application/x-yaml",
		mediaType == "application/vnd.yaml",
		mediaType == "text/yaml",
		mediaType == "text/x-yaml":
		return SourceFormatYAML, true
	default:
		return "", false
	}
}

func decodeJSONSource(data []byte, sourcePath string) (any, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()

	value, err := decodeJSONValue(decoder, data, sourcePath, "$")
	if err != nil {
		return nil, err
	}
	if token, err := decoder.Token(); err != io.EOF {
		if err != nil {
			line, column := offsetLineColumn(data, decoder.InputOffset())
			return nil, sourceError(sourcePath, line, column, "json", err)
		}
		line, column := offsetLineColumn(data, decoder.InputOffset())
		return nil, sourceError(sourcePath, line, column, "json", fmt.Errorf("source document contains multiple JSON values after %v", token))
	}

	return value, nil
}

func decodeJSONValue(decoder *json.Decoder, data []byte, sourcePath string, path string) (any, error) {
	token, err := decoder.Token()
	if err != nil {
		line, column := offsetLineColumn(data, decoder.InputOffset())
		return nil, sourceError(sourcePath, line, column, "json", err)
	}

	switch typed := token.(type) {
	case json.Delim:
		switch typed {
		case '{':
			return decodeJSONObject(decoder, data, sourcePath, path)
		case '[':
			return decodeJSONArray(decoder, data, sourcePath, path)
		default:
			line, column := offsetLineColumn(data, decoder.InputOffset())
			return nil, sourceError(sourcePath, line, column, "json", fmt.Errorf("%s: unexpected delimiter %q", path, typed))
		}
	case string:
		return typed, nil
	case bool:
		return typed, nil
	case json.Number:
		return decodeJSONInteger(typed, data, sourcePath, path, decoder.InputOffset())
	case nil:
		line, column := offsetLineColumn(data, decoder.InputOffset())
		return nil, sourceError(sourcePath, line, column, "json", fmt.Errorf("%s: null values are unsupported", path))
	default:
		line, column := offsetLineColumn(data, decoder.InputOffset())
		return nil, sourceError(sourcePath, line, column, "json", fmt.Errorf("%s: unsupported value %T", path, token))
	}
}

func decodeJSONObject(decoder *json.Decoder, data []byte, sourcePath string, path string) (map[string]any, error) {
	values := make(map[string]any)
	for decoder.More() {
		keyToken, err := decoder.Token()
		if err != nil {
			line, column := offsetLineColumn(data, decoder.InputOffset())
			return nil, sourceError(sourcePath, line, column, "json", err)
		}
		key, ok := keyToken.(string)
		if !ok {
			line, column := offsetLineColumn(data, decoder.InputOffset())
			return nil, sourceError(sourcePath, line, column, "json", fmt.Errorf("%s: object keys must be strings", path))
		}
		itemPath := valuePath(path, key)
		if _, exists := values[key]; exists {
			line, column := offsetLineColumn(data, decoder.InputOffset())
			return nil, sourceError(sourcePath, line, column, "json", fmt.Errorf("%s: duplicate object key %q", path, key))
		}
		value, err := decodeJSONValue(decoder, data, sourcePath, itemPath)
		if err != nil {
			return nil, err
		}
		values[key] = value
	}

	if token, err := decoder.Token(); err != nil || token != json.Delim('}') {
		line, column := offsetLineColumn(data, decoder.InputOffset())
		if err != nil {
			return nil, sourceError(sourcePath, line, column, "json", err)
		}
		return nil, sourceError(sourcePath, line, column, "json", fmt.Errorf("%s: expected object end", path))
	}
	return values, nil
}

func decodeJSONArray(decoder *json.Decoder, data []byte, sourcePath string, path string) ([]any, error) {
	values := []any{}
	for decoder.More() {
		value, err := decodeJSONValue(decoder, data, sourcePath, indexPath(path, len(values)))
		if err != nil {
			return nil, err
		}
		values = append(values, value)
	}

	if token, err := decoder.Token(); err != nil || token != json.Delim(']') {
		line, column := offsetLineColumn(data, decoder.InputOffset())
		if err != nil {
			return nil, sourceError(sourcePath, line, column, "json", err)
		}
		return nil, sourceError(sourcePath, line, column, "json", fmt.Errorf("%s: expected list end", path))
	}
	return values, nil
}

func decodeJSONInteger(number json.Number, data []byte, sourcePath string, path string, offset int64) (int64, error) {
	text := number.String()
	if !isJSONIntegerText(text) {
		line, column := offsetLineColumn(data, offset)
		return 0, sourceError(sourcePath, line, column, "json", fmt.Errorf("%s: unsupported number %q; only JSON integer numbers are supported", path, text))
	}
	value, err := strconv.ParseInt(text, 10, 64)
	if err != nil {
		line, column := offsetLineColumn(data, offset)
		return 0, sourceError(sourcePath, line, column, "json", fmt.Errorf("%s: integer %q is outside int64 range", path, text))
	}
	return value, nil
}
