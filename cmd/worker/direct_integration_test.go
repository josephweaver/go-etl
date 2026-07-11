package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"goetl/internal/model"
)

func TestRunDirectPythonTargetFixture(t *testing.T) {
	python := directPythonExecutable(t)

	t.Run("sentinel controller", func(t *testing.T) {
		var requests atomic.Int64
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requests.Add(1)
			http.Error(w, "unexpected direct request", http.StatusInternalServerError)
		}))
		defer server.Close()

		run := runDirectPythonFixture(t, python, server.URL, "fixture-value")
		assertDirectPythonFixtureSuccess(t, run)
		if got := requests.Load(); got != 0 {
			t.Fatalf("controller request count = %d, want 0", got)
		}
	})

	t.Run("no controller URL", func(t *testing.T) {
		run := runDirectPythonFixture(t, python, "", "fixture-value")
		assertDirectPythonFixtureSuccess(t, run)
	})
}

func TestRunDirectPythonTargetFixtureFailure(t *testing.T) {
	python := directPythonExecutable(t)
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		http.Error(w, "unexpected direct request", http.StatusInternalServerError)
	}))
	defer server.Close()

	run := runDirectPythonFixture(t, python, server.URL, "fail")
	if run.exit != directExitFailure {
		t.Fatalf("runDirectCommand() exit = %d, want 1", run.exit)
	}
	if run.result.Status != directStatusFailed || !strings.Contains(run.result.Error, "python process exited with error") {
		t.Fatalf("result = %+v", run.result)
	}
	if run.result.DataOutputPath != "" || run.result.Evidence != nil {
		t.Fatalf("failed result advertises completed output: %+v", run.result)
	}
	attemptDir := filepath.Join(run.tmpDir, "attempts", run.result.AttemptID)
	assertDirectFixtureFileContains(t, filepath.Join(attemptDir, "logs", "stdout.log"), "direct fixture stdout: fail")
	assertDirectFixtureFileContains(t, filepath.Join(attemptDir, "logs", "stderr.log"), "direct fixture stderr: fail")
	if _, err := os.Stat(filepath.Join(run.dataDir, "direct-python-result.json")); !os.IsNotExist(err) {
		t.Fatalf("completion output exists after failed Python process, stat error = %v", err)
	}
	if got := requests.Load(); got != 0 {
		t.Fatalf("controller request count = %d, want 0", got)
	}
}

type directPythonFixtureRun struct {
	exit     int
	stdout   string
	stderr   string
	result   DirectExecutionResult
	tmpDir   string
	dataDir  string
	root     string
	itemPath string
}

func runDirectPythonFixture(t *testing.T, python string, controllerURL string, argument string) directPythonFixtureRun {
	t.Helper()
	root := t.TempDir()
	logDir := filepath.Join(root, "logs")
	tmpDir := filepath.Join(root, "tmp")
	dataDir := filepath.Join(root, "data")
	for _, dir := range []string{logDir, tmpDir, dataDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}

	configPath := filepath.Join(root, "worker.json")
	configJSON, err := json.Marshal(Config{
		LogDir:           logDir,
		TmpDir:           tmpDir,
		DataDir:          dataDir,
		ControllerURL:    controllerURL,
		PythonExecutable: python,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, configJSON, 0644); err != nil {
		t.Fatal(err)
	}

	itemData, err := os.ReadFile(filepath.Join("testdata", "direct-python", "work-item.json"))
	if err != nil {
		t.Fatal(err)
	}
	var item model.WorkItem
	if err := json.Unmarshal(itemData, &item); err != nil {
		t.Fatal(err)
	}
	item.Parameters["python_args"] = model.Parameter{Type: "list", Value: []any{argument}}
	itemPath := filepath.Join(root, "work-item.json")
	itemData, err = json.MarshalIndent(item, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(itemPath, append(itemData, '\n'), 0644); err != nil {
		t.Fatal(err)
	}

	bundlePath := filepath.Join(root, "source-bundle.zip")
	buildDirectPythonSourceBundle(t, bundlePath)
	resultPath := filepath.Join(root, "worker-result.json")
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exit := runDirectCommand([]string{
		"--config", configPath,
		"--work-item", itemPath,
		"--source-bundle", bundlePath,
		"--result", resultPath,
	}, &stdout, &stderr)

	return directPythonFixtureRun{
		exit:     exit,
		stdout:   stdout.String(),
		stderr:   stderr.String(),
		result:   readDirectResult(t, resultPath),
		tmpDir:   tmpDir,
		dataDir:  dataDir,
		root:     root,
		itemPath: itemPath,
	}
}

func assertDirectPythonFixtureSuccess(t *testing.T, run directPythonFixtureRun) {
	t.Helper()
	if run.exit != directExitSuccess {
		t.Fatalf("runDirectCommand() exit = %d, stderr = %s", run.exit, run.stderr)
	}
	if run.result.Schema != directResultSchema || run.result.Status != directStatusCompleted {
		t.Fatalf("result = %+v", run.result)
	}
	if run.result.WorkItemID != "direct-python-001" || !strings.HasPrefix(run.result.AttemptID, "direct-attempt-") {
		t.Fatalf("result identity = %+v", run.result)
	}
	if run.result.Evidence == nil || run.result.Evidence.OutputJSON == "" {
		t.Fatalf("result evidence = %+v", run.result.Evidence)
	}
	if !strings.Contains(run.stdout, "worker-result.json") {
		t.Fatalf("command stdout = %q", run.stdout)
	}

	attemptDir := filepath.Join(run.tmpDir, "attempts", run.result.AttemptID)
	if run.result.AttemptDir != attemptDir {
		t.Fatalf("result attempt dir = %q, want %q", run.result.AttemptDir, attemptDir)
	}
	for _, path := range []string{
		filepath.Join(attemptDir, "source", "main.py"),
		filepath.Join(attemptDir, "work", "input.json"),
		filepath.Join(attemptDir, "work", "output.json"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("stat fixture output %s: %v", path, err)
		}
	}
	assertDirectFixtureFileContains(t, filepath.Join(attemptDir, "logs", "stdout.log"), "direct fixture stdout: fixture-value")
	assertDirectFixtureFileContains(t, filepath.Join(attemptDir, "logs", "stderr.log"), "direct fixture stderr: fixture-value")

	dataOutputPath := filepath.Join(run.dataDir, "direct-python-result.json")
	if run.result.DataOutputPath != dataOutputPath {
		t.Fatalf("result data output path = %q, want %q", run.result.DataOutputPath, dataOutputPath)
	}
	if _, err := os.Stat(dataOutputPath); err != nil {
		t.Fatalf("stat data output: %v", err)
	}
	artifactPath := filepath.Join(run.dataDir, "artifacts", "raw", "direct-python-001", "reports", "fixture.txt")
	assertDirectFixtureFileContains(t, artifactPath, "direct fixture artifact")

	var wrapper struct {
		Schema        string          `json:"schema"`
		LogicalOutput json.RawMessage `json:"logical_output"`
	}
	if err := json.Unmarshal([]byte(run.result.Evidence.OutputJSON), &wrapper); err != nil {
		t.Fatalf("decode Python evidence wrapper: %v", err)
	}
	if wrapper.Schema != pythonOutputEvidenceSchema {
		t.Fatalf("Python evidence schema = %q", wrapper.Schema)
	}
	var manifest model.ArtifactManifest
	if err := json.Unmarshal(wrapper.LogicalOutput, &manifest); err != nil {
		t.Fatalf("decode result evidence artifact manifest: %v", err)
	}
	if len(manifest.Artifacts) != 1 || manifest.Artifacts[0].Name != "fixture_report" {
		t.Fatalf("artifact manifest = %+v", manifest)
	}
	scriptOutput, ok := manifest.ScriptOutput.(map[string]any)
	if !ok || scriptOutput["argument"] != "fixture-value" || scriptOutput["input_work_item_id"] != "direct-python-001" {
		t.Fatalf("script output = %#v", manifest.ScriptOutput)
	}
}

func buildDirectPythonSourceBundle(t *testing.T, destination string) {
	t.Helper()
	sourcePath := filepath.Join("testdata", "direct-python", "source", "main.py")
	source, err := os.Open(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()

	bundle, err := os.Create(destination)
	if err != nil {
		t.Fatal(err)
	}
	zipWriter := zip.NewWriter(bundle)
	entry, err := zipWriter.Create("main.py")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(entry, source); err != nil {
		t.Fatal(err)
	}
	if err := zipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := bundle.Close(); err != nil {
		t.Fatal(err)
	}
}

func directPythonExecutable(t *testing.T) string {
	t.Helper()
	for _, candidate := range []string{"python3", "python"} {
		path, err := exec.LookPath(candidate)
		if err != nil {
			continue
		}
		if err := exec.Command(path, "-c", "import sys; raise SystemExit(0 if sys.version_info.major == 3 else 1)").Run(); err == nil {
			return path
		}
	}
	t.Skip("Python 3 executable not available")
	return ""
}

func assertDirectFixtureFileContains(t *testing.T, path string, want string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if !strings.Contains(string(data), want) {
		t.Fatalf("%s = %q, want substring %q", path, data, want)
	}
}
