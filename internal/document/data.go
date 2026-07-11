package document

const (
	DataLayerProject    = "project.data"
	DataLayerWorkflow   = "workflow.data"
	DataLayerSubmission = "submission.data"
)

type DataOverlaySource struct {
	Layer string
	Tree  map[string]any
}

func EffectiveDataTree(projectData, workflowData, submissionData map[string]any) (map[string]any, error) {
	return OverlayDataTrees(
		DataOverlaySource{Layer: DataLayerProject, Tree: projectData},
		DataOverlaySource{Layer: DataLayerWorkflow, Tree: workflowData},
		DataOverlaySource{Layer: DataLayerSubmission, Tree: submissionData},
	)
}
