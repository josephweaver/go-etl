package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"goetl/internal/model"
	"goetl/internal/variable"
)

func newTestOperationContext(t *testing.T, worker Worker, item model.WorkItem) OperationContext {
	t.Helper()
	ctx, err := worker.operationContext(context.Background(), item, trustedGoSensitiveNeeds(item.Type))
	if err != nil {
		t.Fatalf("operationContext() error = %v", err)
	}
	return ctx
}

func TestOperationContextSeparatesPublicAndSensitiveValues(t *testing.T) {
	worker := newTestWorker(t)
	t.Setenv("GOET_DEMO_SECRET", "plain-secret")
	item := model.WorkItem{
		ID:             "demo-sensitive-001",
		Type:           model.WorkItemTypeWriteDemoOutput,
		OutputFilename: "result.txt",
		Parameters: model.Parameters{
			"year": {
				Type:  "int",
				Value: 2026,
			},
			"demo_secret": {
				Type:         "string",
				ProtectedRef: &variable.ProtectedRef{Provider: "worker_env", Key: "GOET_DEMO_SECRET"},
			},
			"unneeded_secret": {
				Type:         "string",
				ProtectedRef: &variable.ProtectedRef{Provider: "worker_env", Key: "GOET_UNNEEDED_SECRET"},
			},
		},
	}

	ctx := newTestOperationContext(t, worker, item)

	if got := ctx.Public["year"].Value; got != 2026 {
		t.Fatalf("public year = %#v", got)
	}
	if _, ok := ctx.Sensitive["demo_secret"]; !ok {
		t.Fatalf("missing declared sensitive value: %+v", ctx.Sensitive)
	}
	if got := ctx.Sensitive["demo_secret"].Plaintext(); got != "plain-secret" {
		t.Fatalf("sensitive plaintext = %q", got)
	}
	if _, ok := ctx.Sensitive["unneeded_secret"]; ok {
		t.Fatalf("undeclared sensitive value was resolved: %+v", ctx.Sensitive)
	}
}

func TestOperationContextDoesNotResolveSensitiveValuesForHandlerWithoutNeeds(t *testing.T) {
	worker := newTestWorker(t)
	item := model.WorkItem{
		ID:             "summary-sensitive-001",
		Type:           model.WorkItemTypeSummarizeInputFile,
		OutputFilename: "summary.txt",
		Parameters: model.Parameters{
			"input_path": {
				Type:  "path",
				Value: "input.txt",
			},
			"demo_secret": {
				Type:         "string",
				ProtectedRef: &variable.ProtectedRef{Provider: "worker_env", Key: "GOET_MISSING_SECRET"},
			},
		},
	}

	ctx := newTestOperationContext(t, worker, item)
	if len(ctx.Sensitive) != 0 {
		t.Fatalf("handler without sensitive needs received secrets: %+v", ctx.Sensitive)
	}
}

func TestTrustedGoHandlerConsumesSensitiveValueWithSafeLogging(t *testing.T) {
	worker := newTestWorker(t)
	t.Setenv("GOET_DEMO_SECRET", "plain-secret")
	item := model.WorkItem{
		ID:             "demo-sensitive-001",
		Type:           model.WorkItemTypeWriteDemoOutput,
		OutputFilename: "result.txt",
		Parameters: model.Parameters{
			"demo_secret": {
				Type:         "string",
				ProtectedRef: &variable.ProtectedRef{Provider: "worker_env", Key: "GOET_DEMO_SECRET"},
			},
		},
	}

	if _, err := worker.Run(item); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	logPath := filepath.Join(worker.Config.LogDir, "worker.log")
	logOutput, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read worker log: %v", err)
	}
	if strings.Contains(string(logOutput), "plain-secret") {
		t.Fatalf("worker log contains plaintext secret: %s", logOutput)
	}
	if !strings.Contains(string(logOutput), "${worker_env.GOET_DEMO_SECRET}") {
		t.Fatalf("worker log does not contain redaction label: %s", logOutput)
	}
}

func TestOperationContextFormattingDoesNotPrintPlaintext(t *testing.T) {
	worker := newTestWorker(t)
	t.Setenv("GOET_DEMO_SECRET", "plain-secret")
	item := model.WorkItem{
		ID:             "demo-format-001",
		Type:           model.WorkItemTypeWriteDemoOutput,
		OutputFilename: "result.txt",
		Parameters: model.Parameters{
			"demo_secret": {
				Type:         "string",
				ProtectedRef: &variable.ProtectedRef{Provider: "worker_env", Key: "GOET_DEMO_SECRET"},
			},
		},
	}
	ctx := newTestOperationContext(t, worker, item)

	formats := []string{
		fmt.Sprint(ctx),
		fmt.Sprintf("%+v", ctx),
		fmt.Sprintf("%#v", ctx),
	}
	encoded, err := json.Marshal(ctx)
	if err != nil {
		t.Fatalf("MarshalJSON() error = %v", err)
	}
	formats = append(formats, string(encoded))

	for _, formatted := range formats {
		if strings.Contains(formatted, "plain-secret") {
			t.Fatalf("formatted operation context contains plaintext: %q", formatted)
		}
	}
}

func TestOperationContextRejectsMissingDeclaredWorkerEnv(t *testing.T) {
	worker := newTestWorker(t)
	item := model.WorkItem{
		ID:             "demo-missing-secret-001",
		Type:           model.WorkItemTypeWriteDemoOutput,
		OutputFilename: "result.txt",
		Parameters: model.Parameters{
			"demo_secret": {
				Type:         "string",
				ProtectedRef: &variable.ProtectedRef{Provider: "worker_env", Key: "GOET_MISSING_SECRET"},
			},
		},
	}

	_, err := worker.operationContext(context.Background(), item, trustedGoSensitiveNeeds(item.Type))
	if err == nil {
		t.Fatal("expected an error")
	}
	if strings.Contains(err.Error(), "plain-secret") {
		t.Fatalf("error contains plaintext: %v", err)
	}
}
