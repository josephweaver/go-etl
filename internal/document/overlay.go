package document

import (
	"fmt"
	"sort"
)

func OverlayDataTrees(sources ...DataOverlaySource) (map[string]any, error) {
	effective := make(map[string]any)
	provenance := make(map[string]string)

	for index, source := range sources {
		layer := source.Layer
		if layer == "" {
			layer = fmt.Sprintf("data layer %d", index)
		}
		if source.Tree == nil {
			continue
		}
		if err := overlayDataObject(effective, provenance, source.Tree, layer, ""); err != nil {
			return nil, err
		}
	}
	return effective, nil
}

func overlayDataObject(target map[string]any, provenance map[string]string, source map[string]any, layer string, path string) error {
	keys := make([]string, 0, len(source))
	for key := range source {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, key := range keys {
		itemPath := appendDataPath(path, key)
		next, err := cloneDataValue(source[key], layer, itemPath)
		if err != nil {
			return err
		}

		current, exists := target[key]
		if !exists {
			target[key] = next
			markDataProvenance(provenance, next, layer, itemPath)
			continue
		}

		currentObject, currentIsObject := current.(map[string]any)
		nextObject, nextIsObject := next.(map[string]any)
		switch {
		case currentIsObject && nextIsObject:
			if err := overlayDataObject(currentObject, provenance, nextObject, layer, itemPath); err != nil {
				return err
			}
		case currentIsObject != nextIsObject:
			previousLayer := provenance[itemPath]
			if previousLayer == "" {
				previousLayer = "earlier data layer"
			}
			return fmt.Errorf("data overlay at %s from %s conflicts with %s: cannot replace %s with %s",
				itemPath, layer, previousLayer, dataShapeName(current), dataShapeName(next))
		default:
			target[key] = next
			markDataProvenance(provenance, next, layer, itemPath)
		}
	}
	return nil
}

func cloneDataValue(value any, layer string, path string) (any, error) {
	switch typed := value.(type) {
	case nil:
		return nil, fmt.Errorf("data overlay at %s from %s: null cannot delete or replace data", path, layer)
	case map[string]any:
		copied := make(map[string]any, len(typed))
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			child, err := cloneDataValue(typed[key], layer, appendDataPath(path, key))
			if err != nil {
				return nil, err
			}
			copied[key] = child
		}
		return copied, nil
	case []any:
		copied := make([]any, len(typed))
		for index, item := range typed {
			child, err := cloneDataValue(item, layer, fmt.Sprintf("%s/%d", path, index))
			if err != nil {
				return nil, err
			}
			copied[index] = child
		}
		return copied, nil
	default:
		return typed, nil
	}
}

func markDataProvenance(provenance map[string]string, value any, layer string, path string) {
	provenance[path] = layer
	if object, ok := value.(map[string]any); ok {
		for key, child := range object {
			markDataProvenance(provenance, child, layer, appendDataPath(path, key))
		}
	}
}

func appendDataPath(parent string, segment string) string {
	segment = jsonPointerEscape(segment)
	if parent == "" {
		return "/" + segment
	}
	return parent + "/" + segment
}

func jsonPointerEscape(segment string) string {
	escaped := ""
	for _, char := range segment {
		switch char {
		case '~':
			escaped += "~0"
		case '/':
			escaped += "~1"
		default:
			escaped += string(char)
		}
	}
	return escaped
}

func dataShapeName(value any) string {
	switch value.(type) {
	case map[string]any:
		return "object"
	case []any:
		return "list"
	default:
		return "scalar"
	}
}
