package model

type SubmissionAcknowledgement struct {
	SubmissionID         string `json:"submission_id"`
	WorkflowID           string `json:"workflow_id"`
	InitialWorkItemCount int    `json:"initial_work_item_count"`
}

type SubmissionStatus struct {
	SubmissionID   string `json:"submission_id"`
	WorkflowID     string `json:"workflow_id"`
	Status         string `json:"status"`
	KnownWorkItems int    `json:"known_work_items"`
	Queued         int    `json:"queued"`
	Running        int    `json:"running"`
	Completed      int    `json:"completed"`
	Failed         int    `json:"failed"`
	Skipped        int    `json:"skipped"`
}
