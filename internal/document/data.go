package document

import (
	"bytes"
	"encoding/json"
	"fmt"

	"goetl/internal/model"
)

const (
	DataLayerProject    = "project.data"
	DataLayerWorkflow   = "workflow.data"
	DataLayerSubmission = "submission.data"
)

type DataOverlaySource struct {
	Layer string
	Tree  map[string]any
}

func DataDefinitionsFromValue(value map[string]any) (model.DataDefinitions, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return model.DataDefinitions{}, fmt.Errorf("encode data definitions: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()

	var definitions model.DataDefinitions
	if err := decoder.Decode(&definitions); err != nil {
		return model.DataDefinitions{}, fmt.Errorf("decode data definitions: %w", err)
	}
	if err := definitions.Validate(); err != nil {
		return model.DataDefinitions{}, err
	}
	return definitions, nil
}

func EffectiveDataTree(projectData, workflowData, submissionData map[string]any) (map[string]any, error) {
	return OverlayDataTrees(
		DataOverlaySource{Layer: DataLayerProject, Tree: projectData},
		DataOverlaySource{Layer: DataLayerWorkflow, Tree: workflowData},
		DataOverlaySource{Layer: DataLayerSubmission, Tree: submissionData},
	)
}
