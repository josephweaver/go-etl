package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"goetl/internal/model"
)

func resolvePythonArgvBindings(args []string, dataAssetsPath string, artifactDir string) ([]string, error) {
	if len(args) == 0 {
		return nil, nil
	}

	var materializedDataAssets map[string]string
	resolved := make([]string, len(args))

	for i, arg := range args {
		value := &strings.Builder{}
		for cursor := 0; cursor < len(arg); {
			if arg[cursor] == '$' && cursor+1 < len(arg) && arg[cursor+1] == '{' {
				tokenEnd := strings.IndexByte(arg[cursor+2:], '}')
				if tokenEnd < 0 {
					return nil, fmt.Errorf("malformed argument token in %q: missing closing }", arg)
				}
				token := arg[cursor+2 : cursor+2+tokenEnd]
				replacement, err := resolvePythonArgBindingToken(token, dataAssetsPath, artifactDir, &materializedDataAssets)
				if err != nil {
					return nil, err
				}
				value.WriteString(replacement)
				cursor = cursor + 2 + tokenEnd + 1
				continue
			}
			if arg[cursor] == '}' {
				return nil, fmt.Errorf("malformed argument token in %q: unexpected }", arg)
			}
			value.WriteByte(arg[cursor])
			cursor++
		}
		resolved[i] = value.String()
	}

	return resolved, nil
}

func resolvePythonArgBindingToken(token string, dataAssetsPath string, artifactDir string, materializedDataAssets *map[string]string) (string, error) {
	if token == "artifact_dir" {
		return artifactDir, nil
	}

	if !strings.HasPrefix(token, "data.") {
		return "", fmt.Errorf("unsupported argument token %q", token)
	}

	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return "", fmt.Errorf("unsupported data token %q", token)
	}
	if parts[1] == "" {
		return "", fmt.Errorf("unsupported data token %q", token)
	}
	if parts[2] != "local_path" {
		return "", fmt.Errorf("unsupported data token property %q in %q", parts[2], token)
	}

	binding := parts[1]
	if strings.TrimSpace(dataAssetsPath) == "" {
		return "", fmt.Errorf("no materialized data assets available for data token %q", token)
	}
	if *materializedDataAssets == nil {
		assets, err := readMaterializedDataAssets(dataAssetsPath)
		if err != nil {
			return "", err
		}
		*materializedDataAssets = assets
	}

	localPath, ok := (*materializedDataAssets)[binding]
	if !ok {
		return "", fmt.Errorf("data binding %q was not materialized", binding)
	}
	if strings.TrimSpace(localPath) == "" {
		return "", fmt.Errorf("data binding %q has empty local_path", binding)
	}
	return localPath, nil
}

func readMaterializedDataAssets(path string) (map[string]string, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("data assets manifest path is required")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read data assets manifest %s: %w", path, err)
	}

	var manifest model.MaterializedDataAssetManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("decode data assets manifest %s: %w", path, err)
	}
	if err := manifest.Validate(); err != nil {
		return nil, fmt.Errorf("validate data assets manifest %s: %w", path, err)
	}

	result := make(map[string]string, len(manifest.Assets))
	for _, asset := range manifest.Assets {
		if _, exists := result[asset.BindingName]; exists {
			return nil, fmt.Errorf("duplicate data binding %q in data assets manifest", asset.BindingName)
		}
		result[asset.BindingName] = asset.LocalPath
	}
	return result, nil
}
