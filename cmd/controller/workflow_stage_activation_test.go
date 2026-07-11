package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"goetl/internal/model"
)

func TestCompleteWorkHandlerActivatesNextSequentialStage(t *testing.T) {
	store := openTestWorkflowExecutionStore(t)
	defer store.Close()
	controller := newController()
	controller.workflowStore = store
	var signals []string
	controller.workerStateChanged = func(reason string) {
		signals = append(signals, reason)
	}
	root := setupLocalWorkflowSource(t, controller)
	writeLocalWorkflowSourceWithSteps(t, root, []int{2024}, "",
		`
		{
			"ID": "download",
			"FanOut": {
				"WorkItem": {
					"FanOutExpression": "${years[*]}",
					"Type": "write_demo_output",
					"OutputPrefix": "download",
					"OutputExtension": ".txt"
				}
			}
		},
		{
			"ID": "summarize",
			"FanOut": {
				"WorkItem": {
					"FanOutExpression": "${workflow.step[*]}",
					"TokenAccessor": ".next_year",
					"Type": "write_demo_output",
					"OutputPrefix": "summarize",
					"OutputExtension": ".txt",
					"resource_constraints": [
						{
							"resource_key": "ctlr/python-env:torch",
							"requested_units": 1,
							"operator": "<=",
							"target_units": 1
						}
					]
				}
			}
		}
	`)

	response := submitLocalWorkflowSource(t, controller)
	if response.Code != http.StatusAccepted {
		t.Fatalf("submit status = %d, body: %s", response.Code, response.Body.String())
	}
	queued, err := store.ListQueuedWorkItems(context.Background())
	if err != nil {
		t.Fatalf("ListQueuedWorkItems() error = %v", err)
	}
	if len(queued) != 1 || queued[0].StageIndex != 0 {
		t.Fatalf("initial queued work = %+v, want one stage 0 item", queued)
	}

	claim := claimNextWorkForActivationTest(t, controller)
	completeResponse := completeClaimForActivationTest(t, controller, claim, `{"next_year":2026}`)
	if completeResponse.Code != http.StatusNoContent {
		t.Fatalf("complete status = %d, body: %s", completeResponse.Code, completeResponse.Body.String())
	}
	if !stringSliceContains(signals, "workflow_stage_activated") {
		t.Fatalf("signals = %+v, want workflow_stage_activated", signals)
	}

	queued, err = store.ListQueuedWorkItems(context.Background())
	if err != nil {
		t.Fatalf("ListQueuedWorkItems() error = %v", err)
	}
	if len(queued) != 1 {
		t.Fatalf("queued work count after activation = %d, want 1", len(queued))
	}
	if queued[0].StageIndex != 1 {
		t.Fatalf("activated queued stage index = %d, want 1", queued[0].StageIndex)
	}
	var payload model.WorkItem
	if err := json.Unmarshal([]byte(queued[0].WorkerPayloadJSON), &payload); err != nil {
		t.Fatalf("decode activated payload: %v", err)
	}
	if payload.ID != "summarize-2026" || payload.OutputFilename != "summarize-2026.txt" {
		t.Fatalf("activated payload = %+v, want workflow.step output token 2026", payload)
	}
	constraints, err := store.ListWorkItemResourceConstraints(context.Background(), queued[0].ID)
	if err != nil {
		t.Fatalf("ListWorkItemResourceConstraints() error = %v", err)
	}
	if len(constraints) != 1 || constraints[0].ResourceKey != "ctlr/python-env:torch" {
		t.Fatalf("activated constraints = %+v, want python-env constraint", constraints)
	}
	if constraints[0].WorkItemID != queued[0].ID || constraints[0].RequestedUnits != 1 {
		t.Fatalf("activated constraint = %+v, want resolved constraint for queued work item %s", constraints[0], queued[0].ID)
	}

	stage1, found, err := controller.ReadStageState(context.Background(), queued[0].RunID, 1)
	if err != nil {
		t.Fatalf("ReadStageState(1) error = %v", err)
	}
	if !found || stage1.State != model.WorkflowStageStateReady {
		t.Fatalf("stage 1 = %+v found=%v, want ready", stage1, found)
	}

	next := claimNextWorkForActivationTest(t, controller)
	if next.ID != "summarize-2026" {
		t.Fatalf("next assigned work id = %q, want summarize-2026", next.ID)
	}
	finalResponse := completeClaimForActivationTest(t, controller, next, `{"final":true}`)
	if finalResponse.Code != http.StatusNoContent {
		t.Fatalf("final complete status = %d, body: %s", finalResponse.Code, finalResponse.Body.String())
	}
	status, found, err := controller.submissionStatus(context.Background(), claim.WorkflowInstanceID)
	if err != nil {
		t.Fatalf("submissionStatus() error = %v", err)
	}
	if !found || status.Status != "completed" {
		t.Fatalf("submission status = %+v found=%v, want completed", status, found)
	}

	duplicate := completeClaimForActivationTest(t, controller, claim, `{"next_year":2026}`)
	if duplicate.Code != http.StatusNoContent {
		t.Fatalf("duplicate complete status = %d, want idempotent 204", duplicate.Code)
	}
	queued, err = store.ListQueuedWorkItems(context.Background())
	if err != nil {
		t.Fatalf("ListQueuedWorkItems() error = %v", err)
	}
	if len(queued) != 0 {
		t.Fatalf("duplicate completion queued %d extra items, want 0", len(queued))
	}
	counts, err := store.CountWorkItemsForRun(context.Background(), claim.WorkflowInstanceID)
	if err != nil {
		t.Fatalf("CountWorkItemsForRun() error = %v", err)
	}
	if counts.Queued+counts.Running+counts.Completed+counts.Failed != 2 {
		t.Fatalf("work item count after duplicate completion = %+v, want exactly two records", counts)
	}
}

func stringSliceContains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func TestCompleteWorkHandlerFailsWorkflowWhenDownstreamStageCannotCompile(t *testing.T) {
	store := openTestWorkflowExecutionStore(t)
	defer store.Close()
	controller := newController()
	controller.workflowStore = store
	root := setupLocalWorkflowSource(t, controller)
	writeLocalWorkflowSourceWithSteps(t, root, []int{2024}, "",
		`
		{
			"ID": "download",
			"FanOut": {
				"WorkItem": {
					"FanOutExpression": "${years[*]}",
					"Type": "write_demo_output",
					"OutputPrefix": "download",
					"OutputExtension": ".txt"
				}
			}
		},
		{
			"ID": "summarize",
			"FanOut": {
				"WorkItem": {
					"FanOutExpression": "${workflow.step[*]}",
					"TokenAccessor": ".missing",
					"Type": "write_demo_output",
					"OutputPrefix": "summarize",
					"OutputExtension": ".txt"
				}
			}
		}
	`)

	response := submitLocalWorkflowSource(t, controller)
	if response.Code != http.StatusAccepted {
		t.Fatalf("submit status = %d, body: %s", response.Code, response.Body.String())
	}
	var ack model.SubmissionAcknowledgement
	if err := json.NewDecoder(response.Body).Decode(&ack); err != nil {
		t.Fatalf("decode acknowledgement: %v", err)
	}
	claim := claimNextWorkForActivationTest(t, controller)
	completeResponse := completeClaimForActivationTest(t, controller, claim, `{"next_year":2026}`)
	if completeResponse.Code != http.StatusNoContent {
		t.Fatalf("complete status = %d, body: %s", completeResponse.Code, completeResponse.Body.String())
	}

	stage1, found, err := controller.ReadStageState(context.Background(), ack.SubmissionID, 1)
	if err != nil {
		t.Fatalf("ReadStageState(1) error = %v", err)
	}
	if !found || stage1.State != model.WorkflowStageStateFailed {
		t.Fatalf("stage 1 = %+v found=%v, want failed", stage1, found)
	}
	if !strings.Contains(stage1.FailureReason, "missing") {
		t.Fatalf("stage 1 failure reason = %q, want compile diagnostic", stage1.FailureReason)
	}
	status, found, err := controller.submissionStatus(context.Background(), ack.SubmissionID)
	if err != nil {
		t.Fatalf("submissionStatus() error = %v", err)
	}
	if !found || status.Status != "failed" {
		t.Fatalf("submission status = %+v found=%v, want failed", status, found)
	}
	queued, err := store.ListQueuedWorkItems(context.Background())
	if err != nil {
		t.Fatalf("ListQueuedWorkItems() error = %v", err)
	}
	if len(queued) != 0 {
		t.Fatalf("queued work count after failed activation = %d, want 0", len(queued))
	}
	duplicate := completeClaimForActivationTest(t, controller, claim, `{"next_year":2026}`)
	if duplicate.Code != http.StatusNoContent {
		t.Fatalf("duplicate complete status = %d, body: %s", duplicate.Code, duplicate.Body.String())
	}
	queued, err = store.ListQueuedWorkItems(context.Background())
	if err != nil {
		t.Fatalf("ListQueuedWorkItems() after duplicate error = %v", err)
	}
	if len(queued) != 0 {
		t.Fatalf("queued work count after duplicate failed activation = %d, want 0", len(queued))
	}
}

func TestFailedParallelWorkItemPreventsLateSiblingFromActivatingNextStage(t *testing.T) {
	store := openTestWorkflowExecutionStore(t)
	defer store.Close()
	controller := newController()
	controller.workflowStore = store
	root := setupLocalWorkflowSource(t, controller)
	writeLocalWorkflowSourceWithVariableDeclarationsAndSteps(
		t,
		root,
		`
			{
				"name": {"namespace": "workflow", "key": "years"},
				"type": "list",
				"expression": [
					{"type": "int", "expression": 2024},
					{"type": "int", "expression": 2025}
				]
			}
		`,
		"",
		`
			{
				"ID": "download",
				"FanOut": {
					"WorkItem": {
						"FanOutExpression": "${years[*]}",
						"Type": "write_demo_output",
						"OutputPrefix": "download",
						"OutputExtension": ".txt"
					}
				}
			},
			{
				"ID": "summarize",
				"FanOut": {
					"WorkItem": {
						"FanOutExpression": "${workflow.step[*]}",
						"TokenAccessor": ".next_year",
						"Type": "write_demo_output",
						"OutputPrefix": "summarize",
						"OutputExtension": ".txt"
					}
				}
			}
		`,
	)

	response := submitLocalWorkflowSource(t, controller)
	if response.Code != http.StatusAccepted {
		t.Fatalf("submit status = %d, body: %s", response.Code, response.Body.String())
	}
	var ack model.SubmissionAcknowledgement
	if err := json.NewDecoder(response.Body).Decode(&ack); err != nil {
		t.Fatalf("decode acknowledgement: %v", err)
	}

	failed := claimNextWorkForActivationTest(t, controller)
	lateSibling := claimNextWorkForActivationTest(t, controller)
	failResponse := failClaimForActivationTest(t, controller, failed, "boom")
	if failResponse.Code != http.StatusNoContent {
		t.Fatalf("fail status = %d, body: %s", failResponse.Code, failResponse.Body.String())
	}
	lateResponse := completeClaimForActivationTest(t, controller, lateSibling, `{"next_year":2026}`)
	if lateResponse.Code != http.StatusNoContent {
		t.Fatalf("late sibling complete status = %d, body: %s", lateResponse.Code, lateResponse.Body.String())
	}

	queued, err := store.ListQueuedWorkItems(context.Background())
	if err != nil {
		t.Fatalf("ListQueuedWorkItems() error = %v", err)
	}
	if len(queued) != 0 {
		t.Fatalf("queued work count after failed workflow = %d, want 0", len(queued))
	}
	stage1, found, err := controller.ReadStageState(context.Background(), ack.SubmissionID, 1)
	if err != nil {
		t.Fatalf("ReadStageState(1) error = %v", err)
	}
	if !found || stage1.State != model.WorkflowStageStateBlocked {
		t.Fatalf("stage 1 = %+v found=%v, want still blocked", stage1, found)
	}
	status, found, err := controller.submissionStatus(context.Background(), ack.SubmissionID)
	if err != nil {
		t.Fatalf("submissionStatus() error = %v", err)
	}
	if !found || status.Status != "failed" {
		t.Fatalf("submission status = %+v found=%v, want failed", status, found)
	}
}

func TestCompleteWorkHandlerInvalidOutputJSONFailsWorkflowAndDoesNotActivateNextStage(t *testing.T) {
	store := openTestWorkflowExecutionStore(t)
	defer store.Close()
	controller := newController()
	controller.workflowStore = store
	root := setupLocalWorkflowSource(t, controller)
	writeLocalWorkflowSourceWithSteps(t, root, []int{2024}, "",
		`
		{
			"ID": "download",
			"FanOut": {
				"WorkItem": {
					"FanOutExpression": "${years[*]}",
					"Type": "write_demo_output",
					"OutputPrefix": "download",
					"OutputExtension": ".txt"
				}
			}
		},
		{
			"ID": "summarize",
			"FanOut": {
				"WorkItem": {
					"FanOutExpression": "${workflow.step[*]}",
					"Type": "write_demo_output",
					"OutputPrefix": "summarize",
					"OutputExtension": ".txt"
				}
			}
		}
	`)

	response := submitLocalWorkflowSource(t, controller)
	if response.Code != http.StatusAccepted {
		t.Fatalf("submit status = %d, body: %s", response.Code, response.Body.String())
	}
	var ack model.SubmissionAcknowledgement
	if err := json.NewDecoder(response.Body).Decode(&ack); err != nil {
		t.Fatalf("decode acknowledgement: %v", err)
	}
	claim := claimNextWorkForActivationTest(t, controller)
	completeResponse := completeClaimForActivationTest(t, controller, claim, `{"next_year":null}`)
	if completeResponse.Code != http.StatusInternalServerError {
		t.Fatalf("complete status = %d, body: %s, want output-capture failure", completeResponse.Code, completeResponse.Body.String())
	}

	queued, err := store.ListQueuedWorkItems(context.Background())
	if err != nil {
		t.Fatalf("ListQueuedWorkItems() error = %v", err)
	}
	if len(queued) != 0 {
		t.Fatalf("queued work count after invalid output = %d, want 0", len(queued))
	}
	step0, found, err := controller.ReadStepState(context.Background(), ack.SubmissionID, 0)
	if err != nil {
		t.Fatalf("ReadStepState(0) error = %v", err)
	}
	if !found || step0.State != model.WorkflowStepStateFailed {
		t.Fatalf("step 0 = %+v found=%v, want failed", step0, found)
	}
	if !strings.Contains(step0.FailureReason, "null") {
		t.Fatalf("step 0 failure reason = %q, want output diagnostic", step0.FailureReason)
	}
	stage1, found, err := controller.ReadStageState(context.Background(), ack.SubmissionID, 1)
	if err != nil {
		t.Fatalf("ReadStageState(1) error = %v", err)
	}
	if !found || stage1.State != model.WorkflowStageStateBlocked {
		t.Fatalf("stage 1 = %+v found=%v, want blocked", stage1, found)
	}
	status, found, err := controller.submissionStatus(context.Background(), ack.SubmissionID)
	if err != nil {
		t.Fatalf("submissionStatus() error = %v", err)
	}
	if !found || status.Status != "failed" {
		t.Fatalf("submission status = %+v found=%v, want failed", status, found)
	}
}

func TestCompleteWorkHandlerAdvancesMixedParallelStageWhenOnlyNonEmptyStepCompletes(t *testing.T) {
	store := openTestWorkflowExecutionStore(t)
	defer store.Close()
	controller := newController()
	controller.workflowStore = store
	root := setupLocalWorkflowSource(t, controller)
	writeLocalWorkflowSourceWithVariableDeclarationsAndSteps(
		t,
		root,
		`
			{
				"name": {"namespace": "workflow", "key": "empty_years"},
				"type": "list",
				"expression": []
			},
			{
				"name": {"namespace": "workflow", "key": "years"},
				"type": "list",
				"expression": [
					{"type": "int", "expression": 2024},
					{"type": "int", "expression": 2025}
				]
			}
		`,
		"",
		`
			{
				"ID": "download-empty",
				"parallel_with": "group-a",
				"FanOut": {
					"WorkItem": {
						"FanOutExpression": "${empty_years[*]}",
						"Type": "write_demo_output",
						"OutputPrefix": "download-empty",
						"OutputExtension": ".txt"
					}
				}
			},
			{
				"ID": "download-work",
				"parallel_with": "group-a",
				"FanOut": {
					"WorkItem": {
						"FanOutExpression": "${years[*]}",
						"Type": "write_demo_output",
						"OutputPrefix": "download-work",
						"OutputExtension": ".txt"
					}
				}
			},
			{
				"ID": "summarize",
				"FanOut": {
					"WorkItem": {
						"FanOutExpression": "${years[*]}",
						"Type": "write_demo_output",
						"OutputPrefix": "summarize",
						"OutputExtension": ".txt"
					}
				}
			}
		`,
	)

	response := submitLocalWorkflowSource(t, controller)
	if response.Code != http.StatusAccepted {
		t.Fatalf("submit status = %d, body: %s", response.Code, response.Body.String())
	}

	initial, err := store.ListQueuedWorkItems(context.Background())
	if err != nil {
		t.Fatalf("ListQueuedWorkItems() error = %v", err)
	}
	if len(initial) != 2 {
		t.Fatalf("initial queued work count = %d, want 2", len(initial))
	}
	for _, item := range initial {
		if item.StageIndex != 0 {
			t.Fatalf("initial queued stage index = %d, want 0", item.StageIndex)
		}
	}

	stage0, found, err := controller.ReadStageState(context.Background(), initial[0].RunID, 0)
	if err != nil {
		t.Fatalf("ReadStageState(0) error = %v", err)
	}
	if !found {
		t.Fatal("stage 0 not found")
	}
	if stage0.State != model.WorkflowStageStateReady && stage0.State != model.WorkflowStageStateActive {
		t.Fatalf("stage 0 state = %q, want ready or active", stage0.State)
	}
	if len(stage0.Steps) != 2 {
		t.Fatalf("stage 0 step count = %d, want 2", len(stage0.Steps))
	}
	for _, step := range stage0.Steps {
		if step.StepID == "download-empty" {
			if step.OutputJSON != "[]" {
				t.Fatalf("empty parallel step output = %q, want []", step.OutputJSON)
			}
			if len(step.WorkItems) != 0 {
				t.Fatalf("empty parallel step work item count = %d, want 0", len(step.WorkItems))
			}
			if step.State != model.WorkflowStepStateCompleted {
				t.Fatalf("empty parallel step state = %q, want completed", step.State)
			}
		}
	}

	first := claimNextWorkForActivationTest(t, controller)
	complete := completeClaimForActivationTest(t, controller, first, `{"value":"first"}`)
	if complete.Code != http.StatusNoContent {
		t.Fatalf("complete status = %d, body: %s", complete.Code, complete.Body.String())
	}

	mid, err := store.ListQueuedWorkItems(context.Background())
	if err != nil {
		t.Fatalf("ListQueuedWorkItems() after first completion error = %v", err)
	}
	if len(mid) != 1 {
		t.Fatalf("queued work count after first completion = %d, want 1", len(mid))
	}
	if mid[0].StageIndex != 0 {
		t.Fatalf("queued stage index after first completion = %d, want 0", mid[0].StageIndex)
	}

	second := claimNextWorkForActivationTest(t, controller)
	final := completeClaimForActivationTest(t, controller, second, `{"value":"second"}`)
	if final.Code != http.StatusNoContent {
		t.Fatalf("final complete status = %d, body: %s", final.Code, final.Body.String())
	}

	after, err := store.ListQueuedWorkItems(context.Background())
	if err != nil {
		t.Fatalf("ListQueuedWorkItems() after parallel stage completion error = %v", err)
	}
	if len(after) != 2 {
		t.Fatalf("queued work count after stage advancement = %d, want 2", len(after))
	}
	if after[0].StageIndex != 1 || after[1].StageIndex != 1 {
		t.Fatalf("queued stage indexes after advancement = %d/%d, want 1/1", after[0].StageIndex, after[1].StageIndex)
	}

	stage0, found, err = controller.ReadStageState(context.Background(), after[0].RunID, 0)
	if err != nil {
		t.Fatalf("ReadStageState(0) after completion error = %v", err)
	}
	if !found {
		t.Fatal("stage 0 not found after completion")
	}
	if stage0.State != model.WorkflowStageStateCompleted {
		t.Fatalf("stage 0 state after completion = %q, want completed", stage0.State)
	}
}

func claimNextWorkForActivationTest(t *testing.T, controller *Controller) model.WorkItem {
	t.Helper()

	request := httptest.NewRequest(http.MethodGet, "/work/next", nil)
	withTestWorkerSessionHeaders(t, controller, request)
	response := httptest.NewRecorder()
	controller.nextWorkHandler(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("next work status = %d, body: %s", response.Code, response.Body.String())
	}
	var item model.WorkItem
	if err := json.NewDecoder(response.Body).Decode(&item); err != nil {
		t.Fatalf("decode next work: %v", err)
	}
	if item.ID == "" || item.AttemptID == "" {
		t.Fatalf("claimed item missing id or attempt: %+v", item)
	}
	return item
}

func completeClaimForActivationTest(t *testing.T, controller *Controller, item model.WorkItem, outputJSON string) *httptest.ResponseRecorder {
	t.Helper()

	body := `{
		"id":` + quoteJSONString(item.ID) + `,
		"attempt_id":` + quoteJSONString(item.AttemptID) + `,
		"output_json":` + quoteJSONString(outputJSON) + `,
		"pre_state_json":"{}",
		"post_state_json":"{}",
		"completed_at":"2026-07-06T12:00:00Z"
	}`
	request := httptest.NewRequest(http.MethodPost, "/work/complete", bytes.NewBufferString(body))
	response := httptest.NewRecorder()
	controller.completeWorkHandler(response, request)
	return response
}

func failClaimForActivationTest(t *testing.T, controller *Controller, item model.WorkItem, reason string) *httptest.ResponseRecorder {
	t.Helper()

	body := `{
		"id":` + quoteJSONString(item.ID) + `,
		"attempt_id":` + quoteJSONString(item.AttemptID) + `,
		"error":` + quoteJSONString(reason) + `,
		"failed_at":"2026-07-06T12:00:00Z"
	}`
	request := httptest.NewRequest(http.MethodPost, "/work/fail", bytes.NewBufferString(body))
	response := httptest.NewRecorder()
	controller.failWorkHandler(response, request)
	return response
}

func quoteJSONString(value string) string {
	encoded, _ := json.Marshal(value)
	return strings.TrimSpace(string(encoded))
}
