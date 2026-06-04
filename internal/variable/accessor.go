package variable

import (
	"fmt"
	"strconv"
	"strings"
)

func ApplyAccessor(value ResolvedValue, accessor string) (ResolvedValue, error) {
	parts, err := parseScalarAccessors(accessor)
	if err != nil {
		return ResolvedValue{}, err
	}

	resolved := value
	for _, part := range parts {
		switch {
		case strings.HasPrefix(part, "."):
			resolved, err = ApplyFieldAccessor(resolved, part)
		case strings.HasPrefix(part, "["):
			resolved, err = ApplyIndexAccessor(resolved, part)
		default:
			return ResolvedValue{}, fmt.Errorf("unsupported accessor: %s", part)
		}
		if err != nil {
			return ResolvedValue{}, err
		}
	}

	return resolved, nil
}

func ApplyFieldAccessor(value ResolvedValue, accessor string) (ResolvedValue, error) {
	field, err := parseFieldAccessor(accessor)
	if err != nil {
		return ResolvedValue{}, err
	}

	if value.Type != TypeObject {
		return ResolvedValue{}, fmt.Errorf("field accessor %s requires object, got %s", accessor, value.Type)
	}

	resolved, ok := value.Object[field]
	if !ok {
		return ResolvedValue{}, fmt.Errorf("object field not found: %s", field)
	}

	return resolved, nil
}

func ApplyIndexAccessor(value ResolvedValue, accessor string) (ResolvedValue, error) {
	index, err := parseIndexAccessor(accessor)
	if err != nil {
		return ResolvedValue{}, err
	}

	if value.Type.Kind != KindList {
		return ResolvedValue{}, fmt.Errorf("index accessor %s requires list, got %s", accessor, value.Type)
	}

	if index < 0 || index >= len(value.List) {
		return ResolvedValue{}, fmt.Errorf("list index out of range: %d", index)
	}

	return value.List[index], nil
}

func ApplyFanOutAccessor(value ResolvedValue, accessor string) ([]ResolvedValue, error) {
	if accessor != "[*]" {
		return nil, fmt.Errorf("fan-out accessor must be [*]")
	}

	if value.Type.Kind != KindList {
		return nil, fmt.Errorf("fan-out accessor requires list, got %s", value.Type)
	}

	return value.List, nil
}

func parseScalarAccessors(accessor string) ([]string, error) {
	if accessor == "" {
		return nil, fmt.Errorf("accessor is required")
	}

	parts := []string{}
	for len(accessor) > 0 {
		switch accessor[0] {
		case '.':
			nextIndex := strings.IndexAny(accessor[1:], ".[")
			if nextIndex == -1 {
				parts = append(parts, accessor)
				accessor = ""
				continue
			}

			end := nextIndex + 1
			parts = append(parts, accessor[:end])
			accessor = accessor[end:]
		case '[':
			end := strings.Index(accessor, "]")
			if end == -1 {
				return nil, fmt.Errorf("index accessor must use [index]")
			}

			part := accessor[:end+1]
			if part == "[*]" {
				return nil, fmt.Errorf("fan-out accessor is not scalar: %s", part)
			}

			parts = append(parts, part)
			accessor = accessor[end+1:]
		default:
			return nil, fmt.Errorf("accessor must start with . or [")
		}
	}

	return parts, nil
}

func parseFieldAccessor(accessor string) (string, error) {
	if !strings.HasPrefix(accessor, ".") {
		return "", fmt.Errorf("field accessor must start with .")
	}

	field := strings.TrimPrefix(accessor, ".")
	if field == "" {
		return "", fmt.Errorf("field accessor name is required")
	}

	if strings.ContainsAny(field, ".[]") {
		return "", fmt.Errorf("field accessor supports one field only: %s", accessor)
	}

	return field, nil
}

func parseIndexAccessor(accessor string) (int, error) {
	if !strings.HasPrefix(accessor, "[") || !strings.HasSuffix(accessor, "]") {
		return 0, fmt.Errorf("index accessor must use [index]")
	}

	indexText := strings.TrimSuffix(strings.TrimPrefix(accessor, "["), "]")
	if indexText == "" {
		return 0, fmt.Errorf("index accessor value is required")
	}

	index, err := strconv.Atoi(indexText)
	if err != nil {
		return 0, fmt.Errorf("parse index accessor %s: %w", accessor, err)
	}

	return index, nil
}
