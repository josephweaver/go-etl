package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"goetl/internal/model"
)

func TestDirectOptionsRequireConfig(t *testing.T) {
	_, err := parseDirectOptions([]string{"--work-item", "item.json"}, ioDiscard{})
	if err == nil || !strings.Contains(err.Error(), "--config is required") {
		t.Fatalf("parseDirectOptions() error = %v, want missing config", err)
	}
}

func TestDirectOptionsRequireWorkItem(t *testing.T) {
	_, err := parseDirectOptions([]string{"--config", "worker.json"}, ioDiscard{})
	if err == nil || !strings.Contains(err.Error(), "--work-item is required") {
		t.Fatalf("parseDirectOptions() error = %v, want missing work item", err)
	}
}

func TestDirectOptionsRejectUnexpectedArguments(t *testing.T) {
	_, err := parseDirectOptions([]string{"--config", "worker.json", "--work-item", "item.json", "extra"}, ioDiscard{})
	if err == nil || !strings.Contains(err.Error(), "unexpected positional arguments") {
		t.Fatalf("parseDirectOptions() error = %v, want unexpected arguments", err)
	}
}

func TestDirectOptionsAcceptSourceBundle(t *testing.T) {
	options, err := parseDirectOptions([]string{
		"--config", "worker.json",
		"--work-item", "item.json",
		"--source-bundle", "source.zip",
	}, ioDiscard{})
	if err != nil {
		t.Fatalf("parseDirectOptions() error = %v", err)
	}
	if options.SourceBundlePath != "source.zip" {
		t.Fatalf("source bundle path = %q", options.SourceBundlePath)
	}
}

func TestLoadDirectWorkItemReadsModelWorkItem(t *testing.T) {
	path := writeDirectWorkItem(t, t.TempDir(), model.WorkItem{
		ID:             "direct-load",
		AttemptID:      "attempt-existing",
		Type:           model.WorkItemTypeWriteDemoOutput,
		OutputFilename: "load.txt",
	})

	item, err := loadDirectWorkItem(path)
	if err != nil {
		t.Fatalf("loadDirectWorkItem() error = %v", err)
	}
	if item.ID != "direct-load" || item.AttemptID != "attempt-existing" {
		t.Fatalf("loadDirectWorkItem() = %+v", item)
	}
}

func TestLoadDirectWorkItemRejectsTrailingJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "item.json")
	content := `{"id":"one","type":"write_demo_output","output_filename":"one.txt"} {"id":"two"}`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	if _, err := loadDirectWorkItem(path); err == nil || !strings.Contains(err.Error(), "exactly one JSON document") {
		t.Fatalf("loadDirectWorkItem() error = %v, want trailing JSON error", err)
	}
}

func TestNormalizeDirectWorkItemPreservesAttemptID(t *testing.T) {
	item := model.WorkItem{AttemptID: "existing-attempt_1"}
	got, err := normalizeDirectWorkItem(item)
	if err != nil {
		t.Fatalf("normalizeDirectWorkItem() error = %v", err)
	}
	if got.AttemptID != item.AttemptID {
		t.Fatalf("attempt ID = %q, want %q", got.AttemptID, item.AttemptID)
	}
}

func TestNormalizeDirectWorkItemCreatesSafeUniqueAttemptID(t *testing.T) {
	first, err := normalizeDirectWorkItem(model.WorkItem{})
	if err != nil {
		t.Fatalf("first normalizeDirectWorkItem() error = %v", err)
	}
	second, err := normalizeDirectWorkItem(model.WorkItem{})
	if err != nil {
		t.Fatalf("second normalizeDirectWorkItem() error = %v", err)
	}
	if !strings.HasPrefix(first.AttemptID, "direct-attempt-") || !validDirectAttemptID(first.AttemptID) {
		t.Fatalf("first attempt ID = %q", first.AttemptID)
	}
	if first.AttemptID == second.AttemptID {
		t.Fatalf("generated duplicate attempt ID %q", first.AttemptID)
	}
}

func TestNormalizeDirectWorkItemRejectsUnsafeAttemptID(t *testing.T) {
	for _, attemptID := range []string{".", "..", "../escape", `C:\\escape`, "run:dummy", "with space"} {
		t.Run(attemptID, func(t *testing.T) {
			if _, err := normalizeDirectWorkItem(model.WorkItem{AttemptID: attemptID}); err == nil {
				t.Fatalf("normalizeDirectWorkItem(%q) expected error", attemptID)
			}
		})
	}
}

func TestNormalizeDirectPythonSuppliesMissingSourceBookkeeping(t *testing.T) {
	item, err := normalizeDirectWorkItem(model.WorkItem{Type: model.WorkItemTypePythonScript})
	if err != nil {
		t.Fatalf("normalizeDirectWorkItem() error = %v", err)
	}
	if item.Source == nil {
		t.Fatal("normalized Python source is nil")
	}
	if item.Source.RunID != "direct-run-dummy" {
		t.Fatalf("source run ID = %q", item.Source.RunID)
	}
	if item.Source.ManifestPath != "source-manifest.json" {
		t.Fatalf("source manifest path = %q", item.Source.ManifestPath)
	}
}

func TestNormalizeDirectPythonPreservesSourceBookkeeping(t *testing.T) {
	source := &model.WorkItemSource{RunID: "existing-run", ManifestPath: "existing-manifest.json"}
	item, err := normalizeDirectWorkItem(model.WorkItem{
		Type:   model.WorkItemTypePythonScript,
		Source: source,
	})
	if err != nil {
		t.Fatalf("normalizeDirectWorkItem() error = %v", err)
	}
	if item.Source.RunID != source.RunID || item.Source.ManifestPath != source.ManifestPath {
		t.Fatalf("normalized source = %+v, want %+v", item.Source, source)
	}
	if item.Source == source {
		t.Fatal("normalizeDirectWorkItem() retained mutable source pointer")
	}
}

func TestRunDirectCommandExecutesDemoItemOnce(t *testing.T) {
	fixture := newDirectTestFixture(t, "")
	itemPath := writeDirectWorkItem(t, fixture.root, model.WorkItem{
		ID:             "direct-demo",
		Type:           model.WorkItemTypeWriteDemoOutput,
		OutputFilename: "demo.txt",
	})

	exit, stdout, stderr := runDirectTestCommand(fixture, itemPath)
	if exit != directExitSuccess {
		t.Fatalf("runDirectCommand() exit = %d, stderr = %s", exit, stderr)
	}
	if !strings.Contains(stdout, fixture.resultPath) {
		t.Fatalf("stdout = %q, want result path", stdout)
	}
	result := readDirectResult(t, fixture.resultPath)
	if result.Status != directStatusCompleted || result.Evidence == nil {
		t.Fatalf("result = %+v", result)
	}
	if result.AttemptID == "" || !validDirectAttemptID(result.AttemptID) {
		t.Fatalf("attempt ID = %q", result.AttemptID)
	}
	if _, err := os.Stat(filepath.Join(fixture.dataDir, "demo.txt")); err != nil {
		t.Fatalf("stat demo output: %v", err)
	}
	workerLog, err := os.ReadFile(filepath.Join(fixture.logDir, "worker.log"))
	if err != nil {
		t.Fatalf("read worker log: %v", err)
	}
	if count := strings.Count(string(workerLog), "worker starting\n"); count != 1 {
		t.Fatalf("worker starting count = %d, want 1", count)
	}
}

func TestRunDirectCommandExecutesSummaryItem(t *testing.T) {
	fixture := newDirectTestFixture(t, "")
	inputPath := filepath.Join(fixture.root, "input.txt")
	if err := os.WriteFile(inputPath, []byte("summary input\n"), 0644); err != nil {
		t.Fatal(err)
	}
	itemPath := writeDirectWorkItem(t, fixture.root, model.WorkItem{
		ID:             "direct-summary",
		Type:           model.WorkItemTypeSummarizeInputFile,
		OutputFilename: "summary.txt",
		Parameters: model.Parameters{
			"input_path": {Type: "path", Value: inputPath},
		},
	})

	exit, _, stderr := runDirectTestCommand(fixture, itemPath)
	if exit != directExitSuccess {
		t.Fatalf("runDirectCommand() exit = %d, stderr = %s", exit, stderr)
	}
	output, err := os.ReadFile(filepath.Join(fixture.dataDir, "summary.txt"))
	if err != nil {
		t.Fatalf("read summary output: %v", err)
	}
	if !strings.Contains(string(output), inputPath) {
		t.Fatalf("summary output = %q, want input path", output)
	}
}

func TestRunDirectCommandPassesManifestOperationsToWorkerRun(t *testing.T) {
	for _, itemType := range []model.WorkItemType{model.WorkItemTypeAssetMaterialize, model.WorkItemTypeCommitData} {
		t.Run(string(itemType), func(t *testing.T) {
			fixture := newDirectTestFixture(t, "")
			itemPath := writeDirectWorkItem(t, fixture.root, model.WorkItem{
				ID:             "direct-" + string(itemType),
				Type:           itemType,
				OutputFilename: string(itemType) + ".json",
			})

			exit, _, _ := runDirectTestCommand(fixture, itemPath)
			if exit != directExitFailure {
				t.Fatalf("runDirectCommand() exit = %d, want operation validation failure", exit)
			}
			result := readDirectResult(t, fixture.resultPath)
			want := string(itemType) + " parameter is required"
			if itemType == model.WorkItemTypeAssetMaterialize {
				want = "asset_materialize parameter is required"
			}
			if !strings.Contains(result.Error, want) {
				t.Fatalf("result error = %q, want %q", result.Error, want)
			}
		})
	}
}

func TestRunDirectCommandRequiresSourceBundleForPython(t *testing.T) {
	fixture := newDirectTestFixture(t, "")
	itemPath := writeDirectWorkItem(t, fixture.root, model.WorkItem{
		ID:             "direct-python-missing-bundle",
		Type:           model.WorkItemTypePythonScript,
		OutputFilename: "python.json",
	})

	exit, _, stderr := runDirectTestCommand(fixture, itemPath)
	if exit != directExitFailure || !strings.Contains(stderr, "requires --source-bundle") {
		t.Fatalf("exit = %d, stderr = %q", exit, stderr)
	}
	if _, err := os.Stat(fixture.resultPath); !os.IsNotExist(err) {
		t.Fatalf("result exists after preflight failure, stat error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(fixture.logDir, "worker.log")); !os.IsNotExist(err) {
		t.Fatalf("worker ran before preflight, stat error = %v", err)
	}
}

func TestRunDirectCommandUsesLocalSourceBundle(t *testing.T) {
	fixture := newDirectTestFixture(t, "")
	itemPath := writeDirectWorkItem(t, fixture.root, model.WorkItem{
		ID:             "direct-python-local-bundle",
		Type:           model.WorkItemTypePythonScript,
		OutputFilename: "python.json",
	})
	bundlePath := filepath.Join(fixture.root, "source.zip")
	if err := os.WriteFile(bundlePath, []byte("not a zip"), 0644); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exit := runDirectCommand([]string{
		"--config", fixture.configPath,
		"--work-item", itemPath,
		"--source-bundle", bundlePath,
		"--result", fixture.resultPath,
	}, &stdout, &stderr)
	if exit != directExitFailure {
		t.Fatalf("runDirectCommand() exit = %d, want invalid ZIP failure", exit)
	}
	result := readDirectResult(t, fixture.resultPath)
	if !strings.Contains(result.Error, "decode source bundle") {
		t.Fatalf("result error = %q, want local ZIP decode error", result.Error)
	}
}

func TestDirectSourceFreeWorkDoesNotReadSourceBundle(t *testing.T) {
	fixture := newDirectTestFixture(t, "")
	itemPath := writeDirectWorkItem(t, fixture.root, model.WorkItem{
		ID:             "direct-extra-source",
		Type:           model.WorkItemTypeWriteDemoOutput,
		OutputFilename: "extra-source.txt",
	})
	missingBundlePath := filepath.Join(fixture.root, "missing-source.zip")
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exit := runDirectCommand([]string{
		"--config", fixture.configPath,
		"--work-item", itemPath,
		"--source-bundle", missingBundlePath,
		"--result", fixture.resultPath,
	}, &stdout, &stderr)
	if exit != directExitSuccess {
		t.Fatalf("runDirectCommand() exit = %d, stderr = %s", exit, stderr.String())
	}
}

func TestRunDirectCommandWritesFailureResult(t *testing.T) {
	fixture := newDirectTestFixture(t, "")
	itemPath := writeDirectWorkItem(t, fixture.root, model.WorkItem{
		ID:             "direct-failure",
		Type:           "unknown",
		OutputFilename: "failure.txt",
	})

	exit, _, stderr := runDirectTestCommand(fixture, itemPath)
	if exit != directExitFailure || !strings.Contains(stderr, "direct work failed") {
		t.Fatalf("exit = %d, stderr = %q", exit, stderr)
	}
	result := readDirectResult(t, fixture.resultPath)
	if result.Status != directStatusFailed || !strings.Contains(result.Error, "unsupported work item type") {
		t.Fatalf("result = %+v", result)
	}
	if result.DataOutputPath != "" || result.AttemptDir != "" || result.Evidence != nil {
		t.Fatalf("failed result advertises output: %+v", result)
	}
}

func TestRunDirectCommandRemovesStaleResultBeforeInvalidWorkItem(t *testing.T) {
	fixture := newDirectTestFixture(t, "")
	if err := os.WriteFile(fixture.resultPath, []byte(`{"status":"completed"}`), 0644); err != nil {
		t.Fatal(err)
	}
	itemPath := filepath.Join(fixture.root, "invalid-item.json")
	if err := os.WriteFile(itemPath, []byte(`{"id":`), 0644); err != nil {
		t.Fatal(err)
	}

	exit, _, _ := runDirectTestCommand(fixture, itemPath)
	if exit != directExitFailure {
		t.Fatalf("runDirectCommand() exit = %d", exit)
	}
	if _, err := os.Stat(fixture.resultPath); !os.IsNotExist(err) {
		t.Fatalf("stale result still exists, stat error = %v", err)
	}
}

func TestRunDirectCommandDoesNotRemoveInputUsedAsResult(t *testing.T) {
	fixture := newDirectTestFixture(t, "")
	itemPath := writeDirectWorkItem(t, fixture.root, model.WorkItem{
		ID:             "direct-input-result",
		Type:           model.WorkItemTypeWriteDemoOutput,
		OutputFilename: "input-result.txt",
	})
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exit := runDirectCommand([]string{
		"--config", fixture.configPath,
		"--work-item", itemPath,
		"--result", itemPath,
	}, &stdout, &stderr)
	if exit != directExitFailure || !strings.Contains(stderr.String(), "must differ from input path") {
		t.Fatalf("exit = %d, stderr = %q", exit, stderr.String())
	}
	if _, err := os.Stat(itemPath); err != nil {
		t.Fatalf("work-item input was removed: %v", err)
	}
}

func TestRunDirectCommandOverwritesExistingResult(t *testing.T) {
	fixture := newDirectTestFixture(t, "")
	if err := os.WriteFile(fixture.resultPath, []byte("stale"), 0644); err != nil {
		t.Fatal(err)
	}
	itemPath := writeDirectWorkItem(t, fixture.root, model.WorkItem{
		ID:             "direct-overwrite",
		Type:           model.WorkItemTypeWriteDemoOutput,
		OutputFilename: "overwrite.txt",
	})

	exit, _, stderr := runDirectTestCommand(fixture, itemPath)
	if exit != directExitSuccess {
		t.Fatalf("runDirectCommand() exit = %d, stderr = %s", exit, stderr)
	}
	if result := readDirectResult(t, fixture.resultPath); result.WorkItemID != "direct-overwrite" {
		t.Fatalf("result work item ID = %q", result.WorkItemID)
	}
}

func TestRunDirectCommandSendsZeroControllerRequests(t *testing.T) {
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		http.Error(w, "unexpected direct request", http.StatusInternalServerError)
	}))
	defer server.Close()

	fixture := newDirectTestFixture(t, server.URL)
	itemPath := writeDirectWorkItem(t, fixture.root, model.WorkItem{
		ID:             "direct-no-controller",
		Type:           model.WorkItemTypeWriteDemoOutput,
		OutputFilename: "no-controller.txt",
	})
	exit, _, stderr := runDirectTestCommand(fixture, itemPath)
	if exit != directExitSuccess {
		t.Fatalf("runDirectCommand() exit = %d, stderr = %s", exit, stderr)
	}
	if got := requests.Load(); got != 0 {
		t.Fatalf("controller request count = %d, want 0", got)
	}
}

func TestWriteDirectExecutionResultUsesSnakeCaseEvidence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "result.json")
	result := DirectExecutionResult{
		Schema: directResultSchema,
		Status: directStatusCompleted,
		Evidence: &DirectExecutionEvidence{
			OutputSHA256: "abc123",
			OutputJSON:   `{"ok":true}`,
		},
	}
	if err := writeDirectExecutionResult(path, result); err != nil {
		t.Fatalf("writeDirectExecutionResult() error = %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasSuffix(data, []byte("\n")) {
		t.Fatalf("result does not end with newline: %q", data)
	}
	if !bytes.Contains(data, []byte(`"output_sha256"`)) || bytes.Contains(data, []byte(`"OutputSHA256"`)) {
		t.Fatalf("unexpected evidence JSON: %s", data)
	}
}

type directTestFixture struct {
	root       string
	configPath string
	resultPath string
	logDir     string
	tmpDir     string
	dataDir    string
}

func newDirectTestFixture(t *testing.T, controllerURL string) directTestFixture {
	t.Helper()
	root := t.TempDir()
	fixture := directTestFixture{
		root:       root,
		configPath: filepath.Join(root, "worker.json"),
		resultPath: filepath.Join(root, "result.json"),
		logDir:     filepath.Join(root, "logs"),
		tmpDir:     filepath.Join(root, "tmp"),
		dataDir:    filepath.Join(root, "data"),
	}
	for _, path := range []string{fixture.logDir, fixture.tmpDir, fixture.dataDir} {
		if err := os.MkdirAll(path, 0755); err != nil {
			t.Fatal(err)
		}
	}
	data, err := json.Marshal(Config{
		LogDir:        fixture.logDir,
		TmpDir:        fixture.tmpDir,
		DataDir:       fixture.dataDir,
		ControllerURL: controllerURL,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fixture.configPath, data, 0644); err != nil {
		t.Fatal(err)
	}
	return fixture
}

func writeDirectWorkItem(t *testing.T, root string, item model.WorkItem) string {
	t.Helper()
	path := filepath.Join(root, fmt.Sprintf("%s-item.json", item.ID))
	data, err := json.Marshal(item)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func runDirectTestCommand(fixture directTestFixture, itemPath string) (int, string, string) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exit := runDirectCommand([]string{
		"--config", fixture.configPath,
		"--work-item", itemPath,
		"--result", fixture.resultPath,
	}, &stdout, &stderr)
	return exit, stdout.String(), stderr.String()
}

func readDirectResult(t *testing.T, path string) DirectExecutionResult {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read direct result: %v", err)
	}
	var result DirectExecutionResult
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("decode direct result: %v\n%s", err, data)
	}
	return result
}

type ioDiscard struct{}

func (ioDiscard) Write(p []byte) (int, error) {
	return len(p), nil
}
