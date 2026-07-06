package model

type SubmissionAcknowledgement struct {
	SubmissionID         string `json:"submission_id"`
	WorkflowID           string `json:"workflow_id"`
	InitialWorkItemCount int    `json:"initial_work_item_count"`
}

type SubmissionStatus struct {
	SubmissionID   string                      `json:"submission_id"`
	WorkflowID     string                      `json:"workflow_id"`
	Status         string                      `json:"status"`
	KnownWorkItems int                         `json:"known_work_items"`
	Queued         int                         `json:"queued"`
	Running        int                         `json:"running"`
	Completed      int                         `json:"completed"`
	Failed         int                         `json:"failed"`
	Skipped        int                         `json:"skipped"`
	Dependency     *SubmissionDependencyStatus `json:"dependency,omitempty"`
}

type SubmissionDependencyStatus struct {
	WorkflowState     string                            `json:"workflow_state"`
	CurrentStageIndex *int                              `json:"current_stage_index,omitempty"`
	StageCount        int                               `json:"stage_count"`
	Counts            SubmissionDependencyCounts        `json:"counts"`
	Failed            *SubmissionDependencyFailure      `json:"failed,omitempty"`
	Stages            []SubmissionDependencyStageStatus `json:"stages"`
}

type SubmissionDependencyCounts struct {
	AssignablePending int `json:"assignable_pending"`
	BlockedFuture     int `json:"blocked_future"`
	Active            int `json:"active"`
	Completed         int `json:"completed"`
	Failed            int `json:"failed"`
	Skipped           int `json:"skipped"`
}

type SubmissionDependencyFailure struct {
	StageIndex    int    `json:"stage_index"`
	StepIndex     *int   `json:"step_index,omitempty"`
	StepID        string `json:"step_id,omitempty"`
	WorkItemID    string `json:"work_item_id,omitempty"`
	FailureReason string `json:"failure_reason"`
}

type SubmissionDependencyStageStatus struct {
	StageIndex   int                              `json:"stage_index"`
	State        string                           `json:"state"`
	ParallelWith string                           `json:"parallel_with,omitempty"`
	StepCount    int                              `json:"step_count"`
	Counts       SubmissionDependencyCounts       `json:"counts"`
	Steps        []SubmissionDependencyStepStatus `json:"steps"`
}

type SubmissionDependencyStepStatus struct {
	StageIndex int                        `json:"stage_index"`
	StepIndex  int                        `json:"step_index"`
	StepID     string                     `json:"step_id"`
	State      string                     `json:"state"`
	Counts     SubmissionDependencyCounts `json:"counts"`
}
