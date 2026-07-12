package workflow

import (
	"fmt"

	"goetl/internal/model"
	"goetl/internal/variable"
)

type ExplicitDataInputTemplate struct {
	Definitions model.DataDefinitions
	Alias       string
	Asset       string
	Selection   []string
	With        map[string]variable.TypedExpression
}

func compileExplicitDataInputs(
	resolver variable.Resolver,
	context FanOutItemContext,
	item *model.WorkItem,
	templates []ExplicitDataInputTemplate,
) error {
	if len(templates) == 0 {
		return nil
	}
	if item.Type == model.WorkItemTypeCacheData || item.Type == model.WorkItemTypeCommitData {
		return fmt.Errorf("data inputs require compute work, got %q", item.Type)
	}

	assets := make([]model.BoundDataAsset, 0, len(templates))
	for _, template := range templates {
		definition, ok := template.Definitions.Inputs[template.Asset]
		if !ok {
			return fmt.Errorf("data input %q is not defined", template.Asset)
		}
		instance, err := instantiateDataAssetWithContext(resolver, context, template.Asset, definition, template.Selection, template.With)
		if err != nil {
			return err
		}
		instance.BoundAsset.BindingName = template.Alias
		assets = append(assets, instance.BoundAsset)
	}

	if item.Parameters == nil {
		item.Parameters = model.Parameters{}
	}
	if _, exists := item.Parameters["data_assets"]; exists {
		return fmt.Errorf("data_assets parameter is already set")
	}
	item.Parameters["data_assets"] = model.Parameter{Type: "data_assets", Value: assets}
	return nil
}
