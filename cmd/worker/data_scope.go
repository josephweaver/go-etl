package main

import (
	"encoding/json"
	"fmt"
	"os"

	"goetl/internal/model"
	"goetl/internal/variable"
)

func dataScopeFromMaterializedDataAssetsPath(path string) (variable.Scope, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read data assets manifest %s: %w", path, err)
	}

	var manifest model.MaterializedDataAssetManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("decode data assets manifest %s: %w", path, err)
	}
	return dataScopeFromMaterializedDataAssets(manifest)
}

func dataScopeFromMaterializedDataAssets(manifest model.MaterializedDataAssetManifest) (variable.Scope, error) {
	projections, err := model.MaterializedDataProjections(manifest)
	if err != nil {
		return nil, fmt.Errorf("project materialized data assets: %w", err)
	}

	variables := make([]variable.Variable, 0, len(projections))
	for bindingName, projection := range projections {
		expression, err := materializedDataProjectionExpression(projection)
		if err != nil {
			return nil, fmt.Errorf("build data projection %s: %w", bindingName, err)
		}
		variables = append(variables, variable.Variable{
			Name: variable.Name{Namespace: variable.NamespaceData, Key: bindingName},
			TypedExpression: variable.TypedExpression{
				Type:       variable.TypeObject,
				Expression: expression,
			},
		})
	}

	scope, err := variable.NewScope(variables...)
	if err != nil {
		return nil, err
	}
	return scope, nil
}

func materializedDataProjectionExpression(projection model.MaterializedDataProjection) (map[string]variable.TypedExpression, error) {
	pathItems := make([]variable.TypedExpression, 0, len(projection.Path))
	for _, item := range projection.Path {
		pathItems = append(pathItems, variable.TypedExpression{Type: variable.TypePath, Expression: item})
	}

	expression := map[string]variable.TypedExpression{
		"path": {Type: variable.TypeList, Expression: pathItems},
	}
	if projection.AssetKey != "" {
		expression["asset_key"] = variable.TypedExpression{Type: variable.TypeString, Expression: projection.AssetKey}
	}
	if projection.MaterializationDomainID != "" {
		expression["materialization_domain_id"] = variable.TypedExpression{Type: variable.TypeString, Expression: projection.MaterializationDomainID}
	}
	if len(projection.Files) > 0 {
		files := make(map[string]variable.TypedExpression, len(projection.Files))
		for role, file := range projection.Files {
			fields := map[string]variable.TypedExpression{
				"path": {Type: variable.TypePath, Expression: file.Path},
			}
			if file.Member != "" {
				fields["member"] = variable.TypedExpression{Type: variable.TypeString, Expression: file.Member}
			}
			if file.SHA256 != "" {
				fields["sha256"] = variable.TypedExpression{Type: variable.TypeString, Expression: file.SHA256}
			}
			if file.SizeBytes != nil {
				fields["size_bytes"] = variable.TypedExpression{Type: variable.TypeInt, Expression: int(*file.SizeBytes)}
			}
			files[role] = variable.TypedExpression{Type: variable.TypeObject, Expression: fields}
		}
		expression["files"] = variable.TypedExpression{Type: variable.TypeObject, Expression: files}
	}
	return expression, nil
}
