package document

import (
	"bytes"
	"fmt"
	"io"
	"strconv"

	"gopkg.in/yaml.v3"
)

func decodeYAMLSource(data []byte, sourcePath string) (any, error) {
	decoder := yaml.NewDecoder(bytes.NewReader(data))

	var document yaml.Node
	if err := decoder.Decode(&document); err != nil {
		return nil, sourceError(sourcePath, 1, 1, "yaml", err)
	}

	var extra yaml.Node
	if err := decoder.Decode(&extra); err != io.EOF {
		if err != nil {
			return nil, sourceError(sourcePath, extra.Line, extra.Column, "yaml", err)
		}
		return nil, sourceError(sourcePath, extra.Line, extra.Column, "yaml", fmt.Errorf("source document contains multiple YAML documents"))
	}

	value, err := decodeYAMLNode(&document, sourcePath, "$")
	if err != nil {
		return nil, err
	}
	return value, nil
}

func decodeYAMLNode(node *yaml.Node, sourcePath string, path string) (any, error) {
	if node == nil || node.Kind == 0 {
		return nil, sourceError(sourcePath, 1, 1, "yaml", fmt.Errorf("%s: empty documents are unsupported", path))
	}
	if node.Anchor != "" {
		return nil, sourceError(sourcePath, node.Line, node.Column, "yaml", fmt.Errorf("%s: YAML anchors are unsupported", path))
	}

	switch node.Kind {
	case yaml.DocumentNode:
		if len(node.Content) != 1 {
			return nil, sourceError(sourcePath, node.Line, node.Column, "yaml", fmt.Errorf("%s: document must contain one root value", path))
		}
		return decodeYAMLNode(node.Content[0], sourcePath, path)
	case yaml.MappingNode:
		return decodeYAMLMap(node, sourcePath, path)
	case yaml.SequenceNode:
		return decodeYAMLSequence(node, sourcePath, path)
	case yaml.ScalarNode:
		return decodeYAMLScalar(node, sourcePath, path)
	case yaml.AliasNode:
		return nil, sourceError(sourcePath, node.Line, node.Column, "yaml", fmt.Errorf("%s: YAML aliases are unsupported", path))
	default:
		return nil, sourceError(sourcePath, node.Line, node.Column, "yaml", fmt.Errorf("%s: unsupported YAML node kind %d", path, node.Kind))
	}
}

func decodeYAMLMap(node *yaml.Node, sourcePath string, path string) (map[string]any, error) {
	values := make(map[string]any)
	for index := 0; index < len(node.Content); index += 2 {
		keyNode := node.Content[index]
		valueNode := node.Content[index+1]

		key, err := decodeYAMLStringKey(keyNode, sourcePath, path)
		if err != nil {
			return nil, err
		}
		if _, exists := values[key]; exists {
			return nil, sourceError(sourcePath, keyNode.Line, keyNode.Column, "yaml", fmt.Errorf("%s: duplicate mapping key %q", path, key))
		}
		value, err := decodeYAMLNode(valueNode, sourcePath, valuePath(path, key))
		if err != nil {
			return nil, err
		}
		values[key] = value
	}
	return values, nil
}

func decodeYAMLStringKey(node *yaml.Node, sourcePath string, path string) (string, error) {
	if node.Kind != yaml.ScalarNode || node.Tag != "!!str" {
		return "", sourceError(sourcePath, node.Line, node.Column, "yaml", fmt.Errorf("%s: mapping keys must be strings", path))
	}
	return node.Value, nil
}

func decodeYAMLSequence(node *yaml.Node, sourcePath string, path string) ([]any, error) {
	values := make([]any, 0, len(node.Content))
	for index, item := range node.Content {
		value, err := decodeYAMLNode(item, sourcePath, indexPath(path, index))
		if err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, nil
}

func decodeYAMLScalar(node *yaml.Node, sourcePath string, path string) (any, error) {
	switch node.Tag {
	case "!!str", "!!timestamp":
		return node.Value, nil
	case "!!bool":
		switch node.Value {
		case "true":
			return true, nil
		case "false":
			return false, nil
		default:
			return nil, sourceError(sourcePath, node.Line, node.Column, "yaml", fmt.Errorf("%s: boolean values must be true or false", path))
		}
	case "!!int":
		if !isJSONIntegerText(node.Value) {
			return nil, sourceError(sourcePath, node.Line, node.Column, "yaml", fmt.Errorf("%s: unsupported integer %q; only JSON-compatible decimal integers are supported", path, node.Value))
		}
		value, err := strconv.ParseInt(node.Value, 10, 64)
		if err != nil {
			return nil, sourceError(sourcePath, node.Line, node.Column, "yaml", fmt.Errorf("%s: integer %q is outside int64 range", path, node.Value))
		}
		return value, nil
	case "!!null":
		return nil, sourceError(sourcePath, node.Line, node.Column, "yaml", fmt.Errorf("%s: null values are unsupported", path))
	case "!!float":
		return nil, sourceError(sourcePath, node.Line, node.Column, "yaml", fmt.Errorf("%s: unsupported number %q; fractional numbers are unsupported", path, node.Value))
	default:
		return nil, sourceError(sourcePath, node.Line, node.Column, "yaml", fmt.Errorf("%s: unsupported YAML tag %q", path, node.Tag))
	}
}
