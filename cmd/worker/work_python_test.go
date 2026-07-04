package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"goetl/internal/model"
)

func TestWorkerRunWorkItemDispatchesPythonScript(t *testing.T) {
	requirePython3(t)

	worker := newPythonTestWorker(t)
	server := newPythonSourceServer(t, map[string]string{
		"scripts/run.py": strings.TrimSpace(`
import json
import os
import sys

with open(os.environ["GOET_INPUT_JSON"], "r", encoding="utf-8") as handle:
    input_document = json.load(handle)

output = {
    "work_item_id": os.environ["GOET_WORK_ITEM_ID"],
    "attempt_id": os.environ["GOET_ATTEMPT_ID"],
    "source_dir": os.environ["GOET_SOURCE_DIR"],
    "work_dir": os.environ["GOET_WORK_DIR"],
    "data_dir": os.environ["GOET_DATA_DIR"],
    "tmp_dir": os.environ["GOET_TMP_DIR"],
    "log_dir": os.environ["GOET_LOG_DIR"],
    "entrypoint": os.environ["GOET_PYTHON_ENTRYPOINT"],
    "environment": os.environ.get("GOET_PYTHON_ENVIRONMENT_JSON", ""),
    "argv": sys.argv[1:],
    "input_work_item_id": input_document["work_item"]["id"],
    "input_output_filename": input_document["work_item"]["output_filename"]
}

with open(os.environ["GOET_OUTPUT_JSON"], "w", encoding="utf-8") as handle:
    json.dump(output, handle, sort_keys=True)

print("python stdout")
print("python stderr", file=sys.stderr)
`),
		"config/env.json": `{"environment":"present"}`,
	})
	t.Cleanup(server.Close)
	worker.Config.ControllerURL = server.URL

	item := pythonTestItem("python-001", "attempt-001", model.Parameters{
		"python_entrypoint":  {Type: "path", Value: "scripts/run.py"},
		"python_environment": {Type: "path", Value: "config/env.json"},
		"python_args":        {Type: "list", Value: []any{"alpha", "beta"}},
	})

	evidence, err := worker.Run(item)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if evidence.OutputJSON == "" {
		t.Fatal("expected output evidence")
	}

	resultPath := filepath.Join(worker.Config.DataDir, item.OutputFilename)
	data, err := os.ReadFile(resultPath)
	if err != nil {
		t.Fatalf("read result: %v", err)
	}

	var output map[string]any
	if err := json.Unmarshal(data, &output); err != nil {
		t.Fatalf("decode result: %v", err)
	}

	if output["work_item_id"] != item.ID {
		t.Fatalf("unexpected work_item_id: %v", output["work_item_id"])
	}
	if output["attempt_id"] != item.AttemptID {
		t.Fatalf("unexpected attempt_id: %v", output["attempt_id"])
	}
	if output["input_work_item_id"] != item.ID {
		t.Fatalf("unexpected input work item id: %v", output["input_work_item_id"])
	}
	if output["input_output_filename"] != item.OutputFilename {
		t.Fatalf("unexpected input output filename: %v", output["input_output_filename"])
	}
	if output["entrypoint"] == "" || output["environment"] == "" {
		t.Fatalf("expected resolved source paths: %+v", output)
	}
	argv, ok := output["argv"].([]any)
	if !ok || len(argv) != 2 || argv[0] != "alpha" || argv[1] != "beta" {
		t.Fatalf("unexpected argv: %#v", output["argv"])
	}
	if output["source_dir"] != filepath.Join(worker.Config.TmpDir, "attempts", item.AttemptID, "source") {
		t.Fatalf("unexpected source dir: %v", output["source_dir"])
	}

	inputPath := filepath.Join(worker.Config.TmpDir, "attempts", item.AttemptID, "work", "input.json")
	if _, err := os.Stat(inputPath); err != nil {
		t.Fatalf("expected input json: %v", err)
	}

	stdoutLog, err := os.ReadFile(filepath.Join(worker.Config.TmpDir, "attempts", item.AttemptID, "logs", "stdout.log"))
	if err != nil {
		t.Fatalf("read stdout log: %v", err)
	}
	stderrLog, err := os.ReadFile(filepath.Join(worker.Config.TmpDir, "attempts", item.AttemptID, "logs", "stderr.log"))
	if err != nil {
		t.Fatalf("read stderr log: %v", err)
	}
	if !strings.Contains(string(stdoutLog), "python stdout") {
		t.Fatalf("stdout was not captured: %q", stdoutLog)
	}
	if !strings.Contains(string(stderrLog), "python stderr") {
		t.Fatalf("stderr was not captured: %q", stderrLog)
	}
}

func TestWorkerRunWorkItemRejectsMissingPythonEntrypoint(t *testing.T) {
	worker := newPythonTestWorker(t)
	server := newPythonSourceServer(t, map[string]string{
		"scripts/run.py": "print('ok')\n",
	})
	t.Cleanup(server.Close)
	worker.Config.ControllerURL = server.URL

	item := pythonTestItem("python-002", "attempt-002", nil)

	if _, err := worker.Run(item); err == nil || !strings.Contains(err.Error(), "python_entrypoint") {
		t.Fatalf("expected missing entrypoint error, got %v", err)
	}
}

func TestWorkerRunWorkItemRejectsUnsafePythonEntrypoint(t *testing.T) {
	worker := newPythonTestWorker(t)
	server := newPythonSourceServer(t, map[string]string{
		"scripts/run.py": "print('ok')\n",
	})
	t.Cleanup(server.Close)
	worker.Config.ControllerURL = server.URL

	item := pythonTestItem("python-003", "attempt-003", model.Parameters{
		"python_entrypoint": {Type: "path", Value: "../escape.py"},
	})

	if _, err := worker.Run(item); err == nil || !strings.Contains(err.Error(), "unsafe python_entrypoint path") {
		t.Fatalf("expected unsafe entrypoint error, got %v", err)
	}
}

func TestWorkerRunWorkItemRejectsUnsafePythonEnvironment(t *testing.T) {
	worker := newPythonTestWorker(t)
	server := newPythonSourceServer(t, map[string]string{
		"scripts/run.py": "print('ok')\n",
	})
	t.Cleanup(server.Close)
	worker.Config.ControllerURL = server.URL

	item := pythonTestItem("python-004", "attempt-004", model.Parameters{
		"python_entrypoint":  {Type: "path", Value: "scripts/run.py"},
		"python_environment": {Type: "path", Value: "../env.json"},
	})

	if _, err := worker.Run(item); err == nil || !strings.Contains(err.Error(), "unsafe python_environment path") {
		t.Fatalf("expected unsafe environment error, got %v", err)
	}
}

func TestWorkerRunWorkItemRejectsInvalidPythonArgs(t *testing.T) {
	worker := newPythonTestWorker(t)
	server := newPythonSourceServer(t, map[string]string{
		"scripts/run.py": "print('ok')\n",
	})
	t.Cleanup(server.Close)
	worker.Config.ControllerURL = server.URL

	item := pythonTestItem("python-005", "attempt-005", model.Parameters{
		"python_entrypoint": {Type: "path", Value: "scripts/run.py"},
		"python_args":       {Type: "list", Value: []any{"ok", 7}},
	})

	if _, err := worker.Run(item); err == nil || !strings.Contains(err.Error(), "python_args") {
		t.Fatalf("expected invalid python_args error, got %v", err)
	}
}

func TestWorkerRunWorkItemRejectsNonZeroPythonExit(t *testing.T) {
	requirePython3(t)

	worker := newPythonTestWorker(t)
	server := newPythonSourceServer(t, map[string]string{
		"scripts/run.py": "import sys\nsys.exit(7)\n",
	})
	t.Cleanup(server.Close)
	worker.Config.ControllerURL = server.URL

	item := pythonTestItem("python-006", "attempt-006", model.Parameters{
		"python_entrypoint": {Type: "path", Value: "scripts/run.py"},
	})

	if _, err := worker.Run(item); err == nil || !strings.Contains(err.Error(), "python process exited with error") {
		t.Fatalf("expected non-zero exit error, got %v", err)
	}
}

func TestWorkerRunWorkItemRejectsMissingPythonOutputJSON(t *testing.T) {
	requirePython3(t)

	worker := newPythonTestWorker(t)
	server := newPythonSourceServer(t, map[string]string{
		"scripts/run.py": "print('ok')\n",
	})
	t.Cleanup(server.Close)
	worker.Config.ControllerURL = server.URL

	item := pythonTestItem("python-007", "attempt-007", model.Parameters{
		"python_entrypoint": {Type: "path", Value: "scripts/run.py"},
	})

	if _, err := worker.Run(item); err == nil || !strings.Contains(err.Error(), "missing GOET_OUTPUT_JSON") {
		t.Fatalf("expected missing output json error, got %v", err)
	}
}

func pythonTestItem(id string, attemptID string, parameters model.Parameters) model.WorkItem {
	if parameters == nil {
		parameters = model.Parameters{}
	}

	return model.WorkItem{
		ID:             id,
		AttemptID:      attemptID,
		Type:           model.WorkItemTypePythonScript,
		OutputFilename: "result.json",
		Source: &model.WorkItemSource{
			RunID:        "run-123",
			ManifestPath: "source-manifest.json",
		},
		Parameters: parameters,
	}
}

func newPythonTestWorker(t *testing.T) Worker {
	t.Helper()

	root := t.TempDir()
	config := Config{
		LogDir:        filepath.Join(root, "logs"),
		TmpDir:        filepath.Join(root, "tmp"),
		DataDir:       filepath.Join(root, "data"),
		ControllerURL: "https://controller.local",
	}

	for _, dir := range []string{config.LogDir, config.TmpDir, config.DataDir} {
		if err := os.Mkdir(dir, 0755); err != nil {
			t.Fatalf("create directory %s: %v", dir, err)
		}
	}

	return Worker{Config: config}
}

func newPythonSourceServer(t *testing.T, files map[string]string) *httptest.Server {
	t.Helper()

	body := mustPythonSourceBundle(t, files)

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/source-bundle.zip") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/zip")
		_, _ = w.Write(body)
	}))
}

func mustPythonSourceBundle(t *testing.T, files map[string]string) []byte {
	t.Helper()

	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for name, contents := range files {
		file, err := writer.Create(name)
		if err != nil {
			t.Fatalf("create zip entry %s: %v", name, err)
		}
		if _, err := file.Write([]byte(contents)); err != nil {
			t.Fatalf("write zip entry %s: %v", name, err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close zip writer: %v", err)
	}

	return buffer.Bytes()
}

func requirePython3(t *testing.T) {
	t.Helper()

	path, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 not available")
	}

	if err := exec.Command(path, "--version").Run(); err != nil {
		t.Skipf("python3 not functional: %v", err)
	}
}
