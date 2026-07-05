package main

import (
	"fmt"
	"path/filepath"
	"strings"

	"goetl/internal/model"
	"goetl/internal/reposource"
	"goetl/internal/workflow"
)

const workItemSourceSchemaV1 = "goet/work-item-source/v1"

func prepareCompiledWorkflowForAdmission(
	layout reposource.CacheLayout,
	manifest reposource.AdmittedSourceManifest,
	compileResult workflow.CompileResult,
) (workflow.CompileResult, error) {
	paths, err := layout.PathsForManifest(manifest)
	if err != nil {
		return workflow.CompileResult{}, err
	}

	manifestPath := filepath.ToSlash(paths.ManifestPath)
	admittedByPath := make(map[string]reposource.AdmittedSourceManifestFile, len(manifest.Files))
	for _, file := range manifest.Files {
		admittedByPath[file.SourcePath] = file
	}

	for index := range compileResult.WorkItems {
		item := &compileResult.WorkItems[index].WorkItem
		if item.Type == model.WorkItemTypePythonScript {
			entrypointPath, err := requiredPythonSourcePathParameter(*item, "python_entrypoint")
			if err != nil {
				return workflow.CompileResult{}, fmt.Errorf("compiled work item %s: %w", item.ID, err)
			}
			if err := requireAdmittedSourceRole(admittedByPath, entrypointPath, reposource.FileRolePythonEntrypoint, "python_entrypoint"); err != nil {
				return workflow.CompileResult{}, fmt.Errorf("compiled work item %s: %w", item.ID, err)
			}

			environmentPath, ok, err := optionalPythonSourcePathParameter(*item, "python_environment")
			if err != nil {
				return workflow.CompileResult{}, fmt.Errorf("compiled work item %s: %w", item.ID, err)
			}
			if ok {
				if err := requireAdmittedSourceRole(admittedByPath, environmentPath, reposource.FileRolePythonEnvironment, "python_environment"); err != nil {
					return workflow.CompileResult{}, fmt.Errorf("compiled work item %s: %w", item.ID, err)
				}
			}

			item.Source = &model.WorkItemSource{
				Schema:       workItemSourceSchemaV1,
				RunID:        manifest.RunID,
				ManifestPath: manifestPath,
			}
		}

		if err := item.Validate(); err != nil {
			return workflow.CompileResult{}, fmt.Errorf("compiled work item %s: %w", item.ID, err)
		}
	}

	return compileResult, nil
}

func requiredPythonSourcePathParameter(item model.WorkItem, name string) (string, error) {
	parameter, ok := item.Parameters[name]
	if !ok {
		return "", fmt.Errorf("parameter %s is required", name)
	}
	return normalizePythonSourcePathParameter(parameter, name)
}

func optionalPythonSourcePathParameter(item model.WorkItem, name string) (string, bool, error) {
	parameter, ok := item.Parameters[name]
	if !ok {
		return "", false, nil
	}

	path, err := normalizePythonSourcePathParameter(parameter, name)
	if err != nil {
		return "", false, err
	}
	return path, true, nil
}

func normalizePythonSourcePathParameter(parameter model.Parameter, name string) (string, error) {
	if parameter.Type != "string" && parameter.Type != "path" {
		return "", fmt.Errorf("parameter %s has type %s, want string or path", name, parameter.Type)
	}

	value, ok := parameter.Value.(string)
	if !ok || strings.TrimSpace(value) == "" {
		return "", fmt.Errorf("parameter %s value must be a non-empty string", name)
	}

	path, err := reposource.ValidateRepositoryRelativePath(value)
	if err != nil {
		return "", fmt.Errorf("parameter %s: %w", name, err)
	}
	return path, nil
}

func requireAdmittedSourceRole(
	admittedByPath map[string]reposource.AdmittedSourceManifestFile,
	path string,
	wantRole reposource.FileRole,
	parameterName string,
) error {
	file, ok := admittedByPath[path]
	if !ok {
		return fmt.Errorf("parameter %s path %s is not declared in admitted source_manifest", parameterName, path)
	}
	if file.Role != wantRole {
		return fmt.Errorf("parameter %s path %s has role %s, want %s", parameterName, path, file.Role, wantRole)
	}
	return nil
}
