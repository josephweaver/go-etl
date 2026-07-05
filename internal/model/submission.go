package model

type SubmissionAcknowledgement struct {
	SubmissionID         string `json:"submission_id"`
	WorkflowID           string `json:"workflow_id"`
	InitialWorkItemCount int    `json:"initial_work_item_count"`
}
