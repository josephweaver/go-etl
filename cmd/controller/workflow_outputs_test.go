package main

import (
	"fmt"
	"strings"
	"testing"

	"goetl/internal/model"
	"goetl/internal/variable"
)

func TestValidateCompletedWorkOutputJSONSizeAcceptsSmallArtifactReference(t *testing.T) {
	output := `{"artifact":{"uri":"s3://bucket/key.json","sha256":"0123456789abcdef","bytes":1234567,"content_type":"application/json"},"summary":{"row_count":12500,"partition_count":8}}`
	if err := validateCompletedWorkOutputJSONSize(output); err != nil {
		t.Fatalf("validateCompletedWorkOutputJSONSize() error = %v", err)
	}
}

func TestValidateCompletedWorkOutputJSONSizeRejectsOversizedOutput(t *testing.T) {
	output := `{"log":"` + strings.Repeat("x", maxCompletedWorkOutputJSONBytes) + `"}`
	err := validateCompletedWorkOutputJSONSize(output)
	if err == nil {
		t.Fatal("expected oversized output error")
	}
	assertOversizedOutputError(t, err, len([]byte(output)), maxCompletedWorkOutputJSONBytes)
}

func TestValidateLogicalStepOutputJSONSizeRejectsOversizedAggregate(t *testing.T) {
	output := `{"rows":"` + strings.Repeat("x", maxLogicalStepOutputJSONBytes) + `"}`
	err := validateLogicalStepOutputJSONSize(output)
	if err == nil {
		t.Fatal("expected oversized logical output error")
	}
	assertOversizedOutputError(t, err, len([]byte(output)), maxLogicalStepOutputJSONBytes)
}

func TestResolvedOutputFromJSONConvertsNestedObject(t *testing.T) {
	value, err := resolvedOutputFromJSON(`{
		"path": "s3://bucket/value",
		"count": 3,
		"ok": true,
		"items": [
			{"name": "a"},
			{"name": "b"}
		]
	}`)
	if err != nil {
		t.Fatalf("resolvedOutputFromJSON() error = %v", err)
	}
	if value.Type != variable.TypeObject {
		t.Fatalf("type = %s, want object", value.Type)
	}
	if value.Object["path"].Type != variable.TypeString || value.Object["path"].Value != "s3://bucket/value" {
		t.Fatalf("path = %#v, want string", value.Object["path"])
	}
	if value.Object["count"].Type != variable.TypeInt || value.Object["count"].Value != 3 {
		t.Fatalf("count = %#v, want int 3", value.Object["count"])
	}
	if value.Object["ok"].Type != variable.TypeBool || value.Object["ok"].Value != true {
		t.Fatalf("ok = %#v, want bool true", value.Object["ok"])
	}
	items := value.Object["items"]
	if items.Type != variable.TypeList || len(items.List) != 2 {
		t.Fatalf("items = %#v, want two-item list", items)
	}
	if items.List[1].Object["name"].Value != "b" {
		t.Fatalf("second item name = %#v, want b", items.List[1].Object["name"])
	}
}

func TestResolvedOutputFromJSONConvertsNestedList(t *testing.T) {
	value, err := resolvedOutputFromJSON(`[{"name":"a"}, {"name":"b"}]`)
	if err != nil {
		t.Fatalf("resolvedOutputFromJSON() error = %v", err)
	}
	if value.Type != variable.TypeList || len(value.List) != 2 {
		t.Fatalf("value = %#v, want two-item list", value)
	}
	if value.List[0].Type != variable.TypeObject {
		t.Fatalf("first item type = %s, want object", value.List[0].Type)
	}
}

func TestResolvedOutputFromJSONConvertsScalars(t *testing.T) {
	tests := []struct {
		raw      string
		wantType variable.Type
		want     any
	}{
		{raw: `"done"`, wantType: variable.TypeString, want: "done"},
		{raw: `true`, wantType: variable.TypeBool, want: true},
		{raw: `42`, wantType: variable.TypeInt, want: 42},
	}
	for _, test := range tests {
		t.Run(test.raw, func(t *testing.T) {
			value, err := resolvedOutputFromJSON(test.raw)
			if err != nil {
				t.Fatalf("resolvedOutputFromJSON() error = %v", err)
			}
			if value.Type != test.wantType || value.Value != test.want {
				t.Fatalf("value = %#v, want %s/%#v", value, test.wantType, test.want)
			}
		})
	}
}

func TestResolvedOutputFromJSONRejectsNull(t *testing.T) {
	_, err := resolvedOutputFromJSON(`{"value": null}`)
	if err == nil || !strings.Contains(err.Error(), "output /value is null") {
		t.Fatalf("error = %v, want null rejection", err)
	}
}

func TestResolvedOutputFromJSONRejectsNonIntegerNumber(t *testing.T) {
	_, err := resolvedOutputFromJSON(`{"count": 1.25}`)
	if err == nil || !strings.Contains(err.Error(), "output /count has non-integer number 1.25") {
		t.Fatalf("error = %v, want non-integer rejection", err)
	}
}

func TestResolvedOutputFromJSONRejectsTrailingTokens(t *testing.T) {
	_, err := resolvedOutputFromJSON(`{"ok": true} {"extra": true}`)
	if err == nil || !strings.Contains(err.Error(), "one JSON document") {
		t.Fatalf("error = %v, want trailing-token rejection", err)
	}
}

func TestAggregateStepOutputNonFanoutStoresSingleObject(t *testing.T) {
	output, _, err := aggregateStepOutputJSON(model.WorkflowDependencyStep{
		StepIndex: 0,
		WorkItems: []model.WorkflowDependencyWorkItemMembership{
			completedOutputMembership("work-0", 0, `{"value":"a"}`),
		},
	})
	if err != nil {
		t.Fatalf("aggregateStepOutputJSON() error = %v", err)
	}
	if output != `{"value":"a"}` {
		t.Fatalf("output = %s, want single object", output)
	}
}

func TestAggregateStepOutputFanoutStoresOutputsByWorkItemIndex(t *testing.T) {
	output, _, err := aggregateStepOutputJSON(fanoutStepInCompletionOrder())
	if err != nil {
		t.Fatalf("aggregateStepOutputJSON() error = %v", err)
	}
	if output != `[{"value":"a"},{"value":"b"},{"value":"c"}]` {
		t.Fatalf("output = %s, want ordered fanout list", output)
	}
}

func TestAggregateStepOutputFanoutIgnoresCompletionOrder(t *testing.T) {
	left, _, err := aggregateStepOutputJSON(fanoutStepInCompletionOrder())
	if err != nil {
		t.Fatalf("aggregateStepOutputJSON(left) error = %v", err)
	}
	right, _, err := aggregateStepOutputJSON(model.WorkflowDependencyStep{
		StepIndex: 0,
		WorkItems: []model.WorkflowDependencyWorkItemMembership{
			completedOutputMembership("work-0", 0, `{"value":"a"}`),
			completedOutputMembership("work-1", 1, `{"value":"b"}`),
			completedOutputMembership("work-2", 2, `{"value":"c"}`),
		},
	})
	if err != nil {
		t.Fatalf("aggregateStepOutputJSON(right) error = %v", err)
	}
	if left != right {
		t.Fatalf("outputs differ by completion order: %s != %s", left, right)
	}
}

func TestAggregateStepOutputReturnsEmptyListForEmptyFanoutStep(t *testing.T) {
	output, _, err := aggregateStepOutputJSON(model.WorkflowDependencyStep{
		StepIndex: 0,
		WorkItems: []model.WorkflowDependencyWorkItemMembership{},
	})
	if err != nil {
		t.Fatalf("aggregateStepOutputJSON() error = %v", err)
	}
	if output != "[]" {
		t.Fatalf("output = %q, want []", output)
	}
}

func TestAggregateStepOutputRejectsMissingItemOutput(t *testing.T) {
	_, _, err := aggregateStepOutputJSON(model.WorkflowDependencyStep{
		StepIndex: 0,
		WorkItems: []model.WorkflowDependencyWorkItemMembership{
			{WorkItemID: "work-0", WorkItemIndex: 0, State: model.WorkItemMembershipStateCompleted},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "missing output JSON") {
		t.Fatalf("error = %v, want missing output rejection", err)
	}
}

func TestAggregateStepOutputRejectsDuplicateWorkItemIndex(t *testing.T) {
	_, _, err := aggregateStepOutputJSON(model.WorkflowDependencyStep{
		StepIndex: 0,
		WorkItems: []model.WorkflowDependencyWorkItemMembership{
			completedOutputMembership("work-a", 0, `{"value":"a"}`),
			completedOutputMembership("work-b", 0, `{"value":"b"}`),
		},
	})
	if err == nil || !strings.Contains(err.Error(), "duplicate work item index") {
		t.Fatalf("error = %v, want duplicate index rejection", err)
	}
}

func TestAggregateStepOutputRejectsOversizedLogicalStepOutput(t *testing.T) {
	workItems := make([]model.WorkflowDependencyWorkItemMembership, 0, 18)
	for index := 0; index < 18; index++ {
		workItems = append(workItems, completedOutputMembership(
			fmt.Sprintf("work-%02d", index),
			index,
			`{"value":"`+strings.Repeat("x", 15500)+`"}`,
		))
	}

	_, _, err := aggregateStepOutputJSON(model.WorkflowDependencyStep{
		StepIndex: 0,
		WorkItems: workItems,
	})
	if err == nil {
		t.Fatal("expected oversized logical step output error")
	}
	if !strings.Contains(err.Error(), "store bulk data externally") {
		t.Fatalf("error = %v, want artifact-storage instruction", err)
	}
}

func TestWorkflowStepScopeResolvesCompletedPriorStep(t *testing.T) {
	scope, err := workflowStepScope(model.WorkflowDependencyPlan{
		RunID:      "run-1",
		WorkflowID: "workflow-1",
		State:      model.WorkflowStateRunning,
		Stages: []model.WorkflowDependencyStage{{
			StageIndex: 0,
			State:      model.WorkflowStageStateCompleted,
			Steps: []model.WorkflowDependencyStep{{
				StageIndex: 0,
				StepIndex:  0,
				StepID:     "step-0",
				State:      model.WorkflowStepStateCompleted,
				OutputJSON: `{"answer":42,"label":"done"}`,
			}},
		}},
	}, 1)
	if err != nil {
		t.Fatalf("workflowStepScope() error = %v", err)
	}

	resolver := variable.NewResolver(variable.NewSet(scope), variable.ResolverConfig{})
	step, err := resolver.Resolve(variable.Reference{Name: variable.Name{Namespace: variable.NamespaceWorkflow, Key: "step"}, Qualified: true})
	if err != nil {
		t.Fatalf("Resolve(workflow.step) error = %v", err)
	}
	answer, err := variable.ApplyAccessor(step, "[0].answer")
	if err != nil {
		t.Fatalf("ApplyAccessor(answer) error = %v", err)
	}
	label, err := variable.ApplyAccessor(step, "[0].label")
	if err != nil {
		t.Fatalf("ApplyAccessor(label) error = %v", err)
	}
	if answer.Type != variable.TypeInt || answer.Value != 42 {
		t.Fatalf("answer = %#v, want int 42", answer)
	}
	if label.Type != variable.TypeString || label.Value != "done" {
		t.Fatalf("label = %#v, want string done", label)
	}
}

func TestWorkflowStepScopeStillResolvesBeforePrune(t *testing.T) {
	scope, err := workflowStepScope(planWithStepOutput("run-1", `{"answer":42}`), 1)
	if err != nil {
		t.Fatalf("workflowStepScope() error = %v", err)
	}
	if got := resolvedWorkflowStepAnswer(t, scope); got != 42 {
		t.Fatalf("answer = %d, want 42", got)
	}
}

func TestWorkflowStepScopePreservesDistinctParallelStageStepOutputs(t *testing.T) {
	scope, err := workflowStepScope(model.WorkflowDependencyPlan{
		RunID:      "run-1",
		WorkflowID: "workflow-1",
		State:      model.WorkflowStateRunning,
		Stages: []model.WorkflowDependencyStage{{
			StageIndex:   0,
			State:        model.WorkflowStageStateCompleted,
			ParallelWith: "A",
			Steps: []model.WorkflowDependencyStep{
				completedStepWithOutput(0, `{"left":1}`),
				completedStepWithOutput(1, `{"right":2}`),
			},
		}},
	}, 2)
	if err != nil {
		t.Fatalf("workflowStepScope() error = %v", err)
	}

	resolver := variable.NewResolver(variable.NewSet(scope), variable.ResolverConfig{})
	step, err := resolver.Resolve(variable.Reference{Name: variable.Name{Namespace: variable.NamespaceWorkflow, Key: "step"}, Qualified: true})
	if err != nil {
		t.Fatalf("Resolve(workflow.step) error = %v", err)
	}
	left, err := variable.ApplyAccessor(step, "[0].left")
	if err != nil {
		t.Fatalf("ApplyAccessor(left) error = %v", err)
	}
	right, err := variable.ApplyAccessor(step, "[1].right")
	if err != nil {
		t.Fatalf("ApplyAccessor(right) error = %v", err)
	}
	if left.Type != variable.TypeInt || left.Value != 1 {
		t.Fatalf("left = %#v, want int 1", left)
	}
	if right.Type != variable.TypeInt || right.Value != 2 {
		t.Fatalf("right = %#v, want int 2", right)
	}
}

func TestWorkflowStepScopeExcludesFutureStep(t *testing.T) {
	scope, err := workflowStepScope(model.WorkflowDependencyPlan{
		RunID:      "run-1",
		WorkflowID: "workflow-1",
		State:      model.WorkflowStateRunning,
		Stages: []model.WorkflowDependencyStage{{
			StageIndex: 0,
			State:      model.WorkflowStageStateCompleted,
			Steps: []model.WorkflowDependencyStep{
				completedStepWithOutput(0, `{"answer":42}`),
				completedStepWithOutput(1, `{"answer":99}`),
			},
		}},
	}, 1)
	if err != nil {
		t.Fatalf("workflowStepScope() error = %v", err)
	}
	resolver := variable.NewResolver(variable.NewSet(scope), variable.ResolverConfig{})
	step, err := resolver.Resolve(variable.Reference{Name: variable.Name{Namespace: variable.NamespaceWorkflow, Key: "step"}, Qualified: true})
	if err != nil {
		t.Fatalf("Resolve(workflow.step) error = %v", err)
	}
	if _, err := variable.ApplyAccessor(step, "[1]"); err == nil {
		t.Fatal("future step should fail through list accessor")
	}
}

func TestWorkflowStepScopeErrorsOnMissingPriorOutput(t *testing.T) {
	_, err := workflowStepScope(model.WorkflowDependencyPlan{
		RunID:      "run-1",
		WorkflowID: "workflow-1",
		State:      model.WorkflowStateRunning,
		Stages: []model.WorkflowDependencyStage{{
			StageIndex: 0,
			State:      model.WorkflowStageStateActive,
			Steps: []model.WorkflowDependencyStep{{
				StageIndex: 0,
				StepIndex:  0,
				StepID:     "step-0",
				State:      model.WorkflowStepStateCompleted,
			}},
		}},
	}, 1)
	if err == nil || !strings.Contains(err.Error(), "missing output JSON") {
		t.Fatalf("error = %v, want missing output error", err)
	}
}

func TestWorkflowStepScopeErrorsIfRequiredOutputWasPruned(t *testing.T) {
	_, err := workflowStepScope(model.WorkflowDependencyPlan{
		RunID:      "run-1",
		WorkflowID: "workflow-1",
		State:      model.WorkflowStateRunning,
		Stages: []model.WorkflowDependencyStage{{
			StageIndex: 0,
			State:      model.WorkflowStageStateCompleted,
			Steps: []model.WorkflowDependencyStep{{
				StageIndex:       0,
				StepIndex:        2,
				StepID:           "step-2",
				State:            model.WorkflowStepStateCompleted,
				OutputJSONSHA256: "hash",
				OutputJSONBytes:  13,
				OutputJSONPruned: true,
			}},
		}},
	}, 3)
	if err == nil || !strings.Contains(err.Error(), "workflow.step[2] output was pruned") {
		t.Fatalf("error = %v, want pruned output error", err)
	}
}

func TestWorkflowStepScopeDoesNotTreatPrunedOutputAsEmptyObject(t *testing.T) {
	_, err := workflowStepScope(model.WorkflowDependencyPlan{
		RunID:      "run-1",
		WorkflowID: "workflow-1",
		State:      model.WorkflowStateRunning,
		Stages: []model.WorkflowDependencyStage{{
			StageIndex: 0,
			State:      model.WorkflowStageStateCompleted,
			Steps: []model.WorkflowDependencyStep{{
				StageIndex:       0,
				StepIndex:        0,
				StepID:           "step-0",
				State:            model.WorkflowStepStateCompleted,
				OutputJSONSHA256: "hash",
				OutputJSONBytes:  2,
				OutputJSONPruned: true,
			}},
		}},
	}, 1)
	if err == nil {
		t.Fatal("expected pruned output error")
	}
	if strings.Contains(err.Error(), "object field not found") {
		t.Fatalf("pruned output was treated as a normal empty object: %v", err)
	}
}

func TestPruneWorkItemOutputJSONKeepsHashAndByteCount(t *testing.T) {
	step := model.WorkflowDependencyStep{
		WorkItems: []model.WorkflowDependencyWorkItemMembership{{
			WorkItemID:       "work-0",
			WorkItemIndex:    0,
			State:            model.WorkItemMembershipStateCompleted,
			OutputJSON:       `{"value":"a"}`,
			OutputJSONSHA256: "hash",
		}},
	}

	pruneWorkItemOutputJSON(&step)

	item := step.WorkItems[0]
	if item.OutputJSON != "" {
		t.Fatalf("output json = %q, want pruned", item.OutputJSON)
	}
	if item.OutputJSONSHA256 != "hash" || item.OutputJSONBytes != len([]byte(`{"value":"a"}`)) || !item.OutputJSONPruned {
		t.Fatalf("metadata after prune = %+v, want hash/bytes/pruned", item)
	}
}

func TestPruneStepOutputJSONKeepsHashAndByteCount(t *testing.T) {
	step := model.WorkflowDependencyStep{
		OutputJSON:       `{"value":"a"}`,
		OutputJSONSHA256: "hash",
	}

	pruneStepOutputJSON(&step)

	if step.OutputJSON != "" {
		t.Fatalf("output json = %q, want pruned", step.OutputJSON)
	}
	if step.OutputJSONSHA256 != "hash" || step.OutputJSONBytes != len([]byte(`{"value":"a"}`)) || !step.OutputJSONPruned {
		t.Fatalf("metadata after prune = %+v, want hash/bytes/pruned", step)
	}
}

func TestWorkflowStepScopeUsesSubmissionScopedPlanOnly(t *testing.T) {
	left, err := workflowStepScope(planWithStepOutput("run-1", `{"answer":1}`), 1)
	if err != nil {
		t.Fatalf("workflowStepScope(left) error = %v", err)
	}
	right, err := workflowStepScope(planWithStepOutput("run-2", `{"answer":2}`), 1)
	if err != nil {
		t.Fatalf("workflowStepScope(right) error = %v", err)
	}

	leftAnswer := resolvedWorkflowStepAnswer(t, left)
	rightAnswer := resolvedWorkflowStepAnswer(t, right)
	if leftAnswer != 1 || rightAnswer != 2 {
		t.Fatalf("answers = %d/%d, want isolated submission scopes 1/2", leftAnswer, rightAnswer)
	}
}

func completedOutputMembership(id string, index int, outputJSON string) model.WorkflowDependencyWorkItemMembership {
	return model.WorkflowDependencyWorkItemMembership{
		WorkItemID:    id,
		WorkItemIndex: index,
		State:         model.WorkItemMembershipStateCompleted,
		OutputJSON:    outputJSON,
	}
}

func fanoutStepInCompletionOrder() model.WorkflowDependencyStep {
	return model.WorkflowDependencyStep{
		StepIndex: 0,
		WorkItems: []model.WorkflowDependencyWorkItemMembership{
			completedOutputMembership("work-2", 2, `{"value":"c"}`),
			completedOutputMembership("work-0", 0, `{"value":"a"}`),
			completedOutputMembership("work-1", 1, `{"value":"b"}`),
		},
	}
}

func completedStepWithOutput(stepIndex int, outputJSON string) model.WorkflowDependencyStep {
	return model.WorkflowDependencyStep{
		StageIndex: 0,
		StepIndex:  stepIndex,
		StepID:     "step",
		State:      model.WorkflowStepStateCompleted,
		OutputJSON: outputJSON,
	}
}

func planWithStepOutput(runID string, outputJSON string) model.WorkflowDependencyPlan {
	return model.WorkflowDependencyPlan{
		RunID:      runID,
		WorkflowID: "workflow-1",
		State:      model.WorkflowStateRunning,
		Stages: []model.WorkflowDependencyStage{{
			StageIndex: 0,
			State:      model.WorkflowStageStateCompleted,
			Steps:      []model.WorkflowDependencyStep{completedStepWithOutput(0, outputJSON)},
		}},
	}
}

func resolvedWorkflowStepAnswer(t *testing.T, scope variable.Scope) int {
	t.Helper()

	resolver := variable.NewResolver(variable.NewSet(scope), variable.ResolverConfig{})
	step, err := resolver.Resolve(variable.Reference{Name: variable.Name{Namespace: variable.NamespaceWorkflow, Key: "step"}, Qualified: true})
	if err != nil {
		t.Fatalf("Resolve(workflow.step) error = %v", err)
	}
	answer, err := variable.ApplyAccessor(step, "[0].answer")
	if err != nil {
		t.Fatalf("ApplyAccessor(answer) error = %v", err)
	}
	integer, ok := answer.Value.(int)
	if !ok {
		t.Fatalf("answer value = %#v, want int", answer.Value)
	}
	return integer
}

func assertOversizedOutputError(t *testing.T, err error, actual int, limit int) {
	t.Helper()

	text := err.Error()
	for _, want := range []string{
		fmt.Sprintf("%d bytes", actual),
		fmt.Sprintf("limit is %d bytes", limit),
		"store bulk data externally",
		"artifact reference",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("error = %q, want %q", text, want)
		}
	}
}
