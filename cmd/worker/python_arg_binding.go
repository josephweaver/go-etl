package main

import (
	"fmt"
	"strings"

	"goetl/internal/variable"
)

func resolvePythonArgvBindings(args []string, dataAssetsPath string, artifactDir string) ([]string, error) {
	if len(args) == 0 {
		return nil, nil
	}

	var dataResolver *variable.Resolver
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
				replacement, err := resolvePythonArgBindingToken(token, dataAssetsPath, artifactDir, &dataResolver)
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

func resolvePythonArgBindingToken(token string, dataAssetsPath string, artifactDir string, dataResolver **variable.Resolver) (string, error) {
	if token == "artifact_dir" {
		return artifactDir, nil
	}

	if !strings.HasPrefix(token, "data.") {
		return "", fmt.Errorf("unsupported argument token %q", token)
	}

	if strings.TrimSpace(dataAssetsPath) == "" {
		return "", fmt.Errorf("no materialized data assets available for data token %q", token)
	}
	if *dataResolver == nil {
		scope, err := dataScopeFromMaterializedDataAssetsPath(dataAssetsPath)
		if err != nil {
			return "", err
		}
		resolver := variable.NewResolver(variable.NewSet(scope), variable.ResolverConfig{})
		*dataResolver = &resolver
	}

	value, err := resolveDataToken(*dataResolver, token)
	if err != nil {
		return "", fmt.Errorf("resolve data token %q: %w", token, err)
	}
	return value, nil
}

func resolveDataToken(resolver *variable.Resolver, token string) (string, error) {
	referenceText, accessor, err := splitDataToken(token)
	if err != nil {
		return "", err
	}
	resolved, err := resolver.Resolve(variable.Reference{
		Name:      variable.Name{Namespace: variable.NamespaceData, Key: referenceText},
		Qualified: true,
	})
	if err != nil {
		return "", err
	}
	if accessor != "" {
		resolved, err = variable.ApplyAccessor(resolved, accessor)
		if err != nil {
			return "", err
		}
	}
	if resolved.Type != variable.TypePath && resolved.Type != variable.TypeString {
		return "", fmt.Errorf("%s has type %s, want path or string", token, resolved.Type)
	}
	value, ok := resolved.Value.(string)
	if !ok || strings.TrimSpace(value) == "" {
		return "", fmt.Errorf("%s is required", token)
	}
	return value, nil
}

func splitDataToken(token string) (string, string, error) {
	const prefix = "data."
	if !strings.HasPrefix(token, prefix) {
		return "", "", fmt.Errorf("unsupported data token %q", token)
	}
	rest := strings.TrimPrefix(token, prefix)
	if rest == "" {
		return "", "", fmt.Errorf("unsupported data token %q", token)
	}
	index := strings.IndexAny(rest, ".[")
	if index == -1 {
		return rest, "", nil
	}
	key := rest[:index]
	if key == "" {
		return "", "", fmt.Errorf("unsupported data token %q", token)
	}
	return key, rest[index:], nil
}
