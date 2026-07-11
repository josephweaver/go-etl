package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"goetl/internal/model"
	"goetl/internal/variable"
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
    "nested": {"b": 2, "a": 1},
    "argv": sys.argv[1:],
    "attempt_id": os.environ["GOET_ATTEMPT_ID"],
    "input_work_item_id": input_document["work_item"]["id"],
    "input_output_filename": input_document["work_item"]["output_filename"],
    "work_item_id": os.environ["GOET_WORK_ITEM_ID"]
}

with open(os.environ["GOET_OUTPUT_JSON"], "w", encoding="utf-8") as handle:
    json.dump(output, handle, indent=2)

print("python stdout")
print("python stderr", file=sys.stderr)
`),
		"config/env.json": `{"environment":"present"}`,
	})
	t.Cleanup(server.Close)
	worker.Config.ControllerURL = server.URL

	item := pythonTestItem("python-001", "attempt-001", model.Parameters{
		"python_entrypoint":  model.Parameter{Type: "path", Value: "scripts/run.py"},
		"python_environment": model.Parameter{Type: "path", Value: "config/env.json"},
		"python_args":        model.Parameter{Type: "list", Value: []any{"alpha", "beta"}},
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

	expectedLogicalOutputRaw := []byte(fmt.Sprintf(`{"nested":{"b":2,"a":1},"argv":["alpha","beta"],"attempt_id":%q,"input_work_item_id":%q,"input_output_filename":%q,"work_item_id":%q}`,
		item.AttemptID, item.ID, item.OutputFilename, item.ID))
	expectedCanonicalOutput, expectedOutputHash, _, err := canonicalJSONDocument(expectedLogicalOutputRaw, "GOET_OUTPUT_JSON")
	if err != nil {
		t.Fatalf("canonicalize expected logical output: %v", err)
	}
	if !bytes.Equal(data, expectedCanonicalOutput) {
		t.Fatalf("promoted output = %s, want %s", data, expectedCanonicalOutput)
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
	if nested, ok := output["nested"].(map[string]any); !ok || nested["a"] != float64(1) || nested["b"] != float64(2) {
		t.Fatalf("unexpected nested output: %#v", output["nested"])
	}
	argv, ok := output["argv"].([]any)
	if !ok || len(argv) != 2 || argv[0] != "alpha" || argv[1] != "beta" {
		t.Fatalf("unexpected argv: %#v", output["argv"])
	}
	if output["input_work_item_id"] != item.ID {
		t.Fatalf("unexpected input work item id: %v", output["input_work_item_id"])
	}
	if output["input_output_filename"] != item.OutputFilename {
		t.Fatalf("unexpected input output filename: %v", output["input_output_filename"])
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

	var wrapper struct {
		Schema          string         `json:"schema"`
		WorkItemID      string         `json:"work_item_id"`
		Operation       string         `json:"operation"`
		Entrypoint      string         `json:"entrypoint"`
		Environment     string         `json:"environment"`
		ExitCode        int            `json:"exit_code"`
		LogicalOutput   map[string]any `json:"logical_output"`
		InputSHA256     string         `json:"input_sha256"`
		OutputSHA256    string         `json:"output_sha256"`
		PreStateSHA256  string         `json:"pre_state_sha256"`
		PostStateSHA256 string         `json:"post_state_sha256"`
		StdoutSHA256    string         `json:"stdout_sha256"`
		StderrSHA256    string         `json:"stderr_sha256"`
	}
	if err := json.Unmarshal([]byte(evidence.OutputJSON), &wrapper); err != nil {
		t.Fatalf("decode evidence wrapper: %v", err)
	}

	expectedEntrypoint := filepath.Join(worker.Config.TmpDir, "attempts", item.AttemptID, "source", "scripts", "run.py")
	expectedEnvironment := filepath.Join(worker.Config.TmpDir, "attempts", item.AttemptID, "source", "config", "env.json")
	if wrapper.Schema != pythonOutputEvidenceSchema {
		t.Fatalf("wrapper schema = %q", wrapper.Schema)
	}
	if wrapper.WorkItemID != item.ID {
		t.Fatalf("wrapper work_item_id = %q", wrapper.WorkItemID)
	}
	if wrapper.Operation != pythonScriptOperation {
		t.Fatalf("wrapper operation = %q", wrapper.Operation)
	}
	if wrapper.Entrypoint != expectedEntrypoint {
		t.Fatalf("wrapper entrypoint = %q, want %q", wrapper.Entrypoint, expectedEntrypoint)
	}
	if wrapper.Environment != expectedEnvironment {
		t.Fatalf("wrapper environment = %q, want %q", wrapper.Environment, expectedEnvironment)
	}
	if wrapper.ExitCode != 0 {
		t.Fatalf("wrapper exit_code = %d", wrapper.ExitCode)
	}
	if wrapper.LogicalOutput["work_item_id"] != item.ID {
		t.Fatalf("wrapper logical_output = %#v", wrapper.LogicalOutput)
	}
	if wrapper.InputSHA256 == "" || wrapper.OutputSHA256 == "" || wrapper.PreStateSHA256 == "" || wrapper.PostStateSHA256 == "" {
		t.Fatalf("wrapper missing hash fields: %+v", wrapper)
	}
	if wrapper.OutputSHA256 != expectedOutputHash || evidence.OutputSHA256 != expectedOutputHash {
		t.Fatalf("unexpected output hash: wrapper=%s evidence=%s want=%s", wrapper.OutputSHA256, evidence.OutputSHA256, expectedOutputHash)
	}
	if evidence.InputSHA256 == "" || evidence.PreStateSHA256 == "" || evidence.PostStateSHA256 == "" {
		t.Fatalf("evidence hashes missing: %+v", evidence)
	}
	if wrapper.StdoutSHA256 == "" || wrapper.StderrSHA256 == "" {
		t.Fatalf("wrapper missing stdout/stderr hashes: %+v", wrapper)
	}
	if strings.Contains(evidence.OutputJSON, "python stdout") || strings.Contains(evidence.OutputJSON, "python stderr") {
		t.Fatalf("evidence wrapper should not embed log contents: %s", evidence.OutputJSON)
	}
}

func TestWorkerRunWorkItemEmitsPythonSubprocessLogs(t *testing.T) {
	requirePython3(t)

	var mu sync.Mutex
	var observations []model.LogObservation
	server := newPythonSourceAndLogServer(t, map[string]string{
		"scripts/run.py": strings.TrimSpace(`
import json
import os
import sys

print("stdout one")
print("")
print("stdout two")
print("stderr one", file=sys.stderr)
print("", file=sys.stderr)
print("stderr two", file=sys.stderr)
output = {"ok": true}
with open(os.environ["GOET_OUTPUT_JSON"], "w", encoding="utf-8") as handle:
    json.dump(output, handle)
`),
		"config/env.json": `{"environment":"present"}`,
	}, func(t *testing.T, w http.ResponseWriter, req *http.Request) {
		t.Helper()

		mu.Lock()
		defer mu.Unlock()

		if req.Method != http.MethodPost {
			t.Fatalf("unexpected method for log endpoint: %s", req.Method)
		}
		if req.URL.Path != "/observations/logs" {
			t.Fatalf("unexpected path for log endpoint: %s", req.URL.Path)
		}

		var observation model.LogObservation
		if err := json.NewDecoder(req.Body).Decode(&observation); err != nil {
			t.Fatalf("decode log observation: %v", err)
		}
		observations = append(observations, observation)
	})
	t.Cleanup(server.Close)

	worker := newPythonTestWorker(t)
	worker.Config.ControllerURL = server.URL
	item := pythonTestItem("python-emit-001", "attempt-emit-001", model.Parameters{
		"python_entrypoint":  model.Parameter{Type: "path", Value: "scripts/run.py"},
		"python_environment": model.Parameter{Type: "path", Value: "config/env.json"},
		"python_args":        model.Parameter{Type: "list", Value: []any{"alpha", "beta"}},
	})
	item.WorkflowDefinitionID = "workflow-demo"
	item.StepDefinitionID = "step-demo"

	if _, err := worker.Run(item); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(observations) < 4 {
		t.Fatalf("expected at least 4 log observations, got %d", len(observations))
	}
	if len(observations) < 6 {
		t.Fatalf("expected at least 6 log observations, got %d", len(observations))
	}

	stdoutObservations := []string{}
	stderrObservations := []string{}
	for _, observation := range observations {
		if observation.Component != "worker-python" {
			t.Fatalf("unexpected component: %q", observation.Component)
		}
		if observation.WorkItemID != item.ID {
			t.Fatalf("unexpected work item id in log: %q", observation.WorkItemID)
		}
		if observation.AttemptID != item.AttemptID {
			t.Fatalf("unexpected attempt id in log: %q", observation.AttemptID)
		}
		if observation.WorkflowID != item.WorkflowDefinitionID {
			t.Fatalf("unexpected workflow id in log: %q", observation.WorkflowID)
		}
		if observation.StepID != item.StepDefinitionID {
			t.Fatalf("unexpected step id in log: %q", observation.StepID)
		}
		if observation.SubmissionID != item.Source.RunID {
			t.Fatalf("unexpected submission id in log: %q", observation.SubmissionID)
		}
		if observation.RunID != item.Source.RunID {
			t.Fatalf("unexpected run id in log: %q", observation.RunID)
		}
		if observation.Sequence == 0 {
			t.Fatalf("unexpected sequence in log: %d", observation.Sequence)
		}

		switch observation.Stream {
		case model.LogStreamStdout:
			if len(stdoutObservations) > 3 {
				t.Fatalf("unexpected stdout count: %d", len(stdoutObservations))
			}
			if observation.Level != model.LogLevelInfo {
				t.Fatalf("unexpected stdout log level: %q", observation.Level)
			}
			stdoutObservations = append(stdoutObservations, observation.Message)
		case model.LogStreamStderr:
			if len(stderrObservations) > 3 {
				t.Fatalf("unexpected stderr count: %d", len(stderrObservations))
			}
			if observation.Level != model.LogLevelWarn {
				t.Fatalf("unexpected stderr log level: %q", observation.Level)
			}
			stderrObservations = append(stderrObservations, observation.Message)
		default:
			t.Fatalf("unexpected stream: %q", observation.Stream)
		}
	}

	if !containsAll(stdoutObservations, []string{"stdout one", "stdout two"}) {
		t.Fatalf("missing expected stdout observations: %#v", stdoutObservations)
	}
	if !containsAll(stderrObservations, []string{"stderr one", "stderr two"}) {
		t.Fatalf("missing expected stderr observations: %#v", stderrObservations)
	}
}

func TestLocalOnlyWorkerRetainsPythonLogsWithoutControllerObservations(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		http.Error(w, "unexpected local-only request", http.StatusInternalServerError)
	}))
	defer server.Close()

	root := t.TempDir()
	stdoutPath := filepath.Join(root, "stdout.log")
	stderrPath := filepath.Join(root, "stderr.log")
	stdout := []byte("local stdout\n")
	stderr := []byte("local stderr\n")
	if err := os.WriteFile(stdoutPath, stdout, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stderrPath, stderr, 0644); err != nil {
		t.Fatal(err)
	}

	worker := Worker{
		Config:    Config{ControllerURL: server.URL},
		LocalOnly: true,
	}
	if err := worker.emitPythonSubprocessLogLines(model.WorkItem{}, stdoutPath, stderrPath, root); err != nil {
		t.Fatalf("emitPythonSubprocessLogLines() error = %v", err)
	}
	if requests != 0 {
		t.Fatalf("controller request count = %d, want 0", requests)
	}
	if got, err := os.ReadFile(stdoutPath); err != nil || !bytes.Equal(got, stdout) {
		t.Fatalf("stdout log = %q, err = %v", got, err)
	}
	if got, err := os.ReadFile(stderrPath); err != nil || !bytes.Equal(got, stderr) {
		t.Fatalf("stderr log = %q, err = %v", got, err)
	}
	if _, err := os.Stat(filepath.Join(root, "fallback-observations.jsonl")); !os.IsNotExist(err) {
		t.Fatalf("unexpected fallback observation file, stat error = %v", err)
	}
}

func TestWorkerRunWorkItemMaterializesWorkerEnvSecretToPythonEnvAndRedactsLogs(t *testing.T) {
	requirePython3(t)

	secret := "goet-secret-python-env-006"
	t.Setenv("GOET_TEST_PYTHON_ENV_SECRET", secret)

	worker := newPythonTestWorker(t)
	server := newPythonSourceServer(t, map[string]string{
		"scripts/run.py": strings.TrimSpace(`
import json
import os
import sys

secret = os.environ["GDRIVE_TOKEN"]
with open(os.environ["GOET_INPUT_JSON"], "r", encoding="utf-8") as handle:
    input_json = handle.read()

print("stdout " + secret)
print("stderr " + secret, file=sys.stderr)

with open(os.environ["GOET_OUTPUT_JSON"], "w", encoding="utf-8") as handle:
    json.dump({
        "secret_available": secret == "goet-secret-python-env-006",
        "argv_has_secret": any(secret in arg for arg in sys.argv),
        "input_has_secret": secret in input_json
    }, handle)
`),
	})
	t.Cleanup(server.Close)
	worker.Config.ControllerURL = server.URL

	item := pythonTestItem("python-secret-env-001", "attempt-secret-env-001", model.Parameters{
		"python_entrypoint": {Type: "path", Value: "scripts/run.py"},
		"gdrive_token":      secretProtectedParameter("GOET_TEST_PYTHON_ENV_SECRET", "env", "GDRIVE_TOKEN"),
	})

	evidence, err := worker.Run(item)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(evidence.OutputJSON, secret) {
		t.Fatalf("evidence leaked secret: %s", evidence.OutputJSON)
	}

	var output map[string]any
	data, err := os.ReadFile(filepath.Join(worker.Config.DataDir, item.OutputFilename))
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	if err := json.Unmarshal(data, &output); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if output["secret_available"] != true {
		t.Fatalf("secret was not available through env: %#v", output)
	}
	if output["argv_has_secret"] != false {
		t.Fatalf("secret reached argv: %#v", output)
	}
	if output["input_has_secret"] != false {
		t.Fatalf("secret reached GOET_INPUT_JSON: %#v", output)
	}

	stdoutLog := readString(t, filepath.Join(worker.Config.TmpDir, "attempts", item.AttemptID, "logs", "stdout.log"))
	stderrLog := readString(t, filepath.Join(worker.Config.TmpDir, "attempts", item.AttemptID, "logs", "stderr.log"))
	for _, logText := range []string{stdoutLog, stderrLog} {
		if strings.Contains(logText, secret) {
			t.Fatalf("log leaked secret: %q", logText)
		}
		if !strings.Contains(logText, "${worker_env.GOET_TEST_PYTHON_ENV_SECRET}") {
			t.Fatalf("log did not contain redaction label: %q", logText)
		}
	}
}

func TestWorkerRunWorkItemControlledSinkSentinelRedactsFailureStdoutAndStderr(t *testing.T) {
	requirePython3(t)

	secret := "goet-secret-sentinel-007-do-not-persist"
	t.Setenv("GOET_TEST_CONTROLLED_SINK_FAILURE_SECRET", secret)

	worker := newPythonTestWorker(t)
	server := newPythonSourceServer(t, map[string]string{
		"scripts/run.py": strings.TrimSpace(`
import os
import sys

secret = os.environ["GDRIVE_TOKEN"]
print("stdout " + secret)
print("stderr " + secret, file=sys.stderr)
sys.exit(7)
`),
	})
	t.Cleanup(server.Close)
	worker.Config.ControllerURL = server.URL

	item := pythonTestItem("python-secret-failure-007", "attempt-secret-failure-007", model.Parameters{
		"python_entrypoint": {Type: "path", Value: "scripts/run.py"},
		"gdrive_token":      secretProtectedParameter("GOET_TEST_CONTROLLED_SINK_FAILURE_SECRET", "env", "GDRIVE_TOKEN"),
	})

	_, err := worker.Run(item)
	if err == nil || !strings.Contains(err.Error(), "python process exited with error") {
		t.Fatalf("expected python failure, got %v", err)
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("failure error leaked secret: %v", err)
	}

	stdoutLog := readString(t, filepath.Join(worker.Config.TmpDir, "attempts", item.AttemptID, "logs", "stdout.log"))
	stderrLog := readString(t, filepath.Join(worker.Config.TmpDir, "attempts", item.AttemptID, "logs", "stderr.log"))
	for _, logText := range []string{stdoutLog, stderrLog} {
		if strings.Contains(logText, secret) {
			t.Fatalf("captured subprocess log leaked secret: %q", logText)
		}
		if !strings.Contains(logText, "${worker_env.GOET_TEST_CONTROLLED_SINK_FAILURE_SECRET}") {
			t.Fatalf("captured subprocess log missing redaction label: %q", logText)
		}
	}
}

func TestWorkerRunWorkItemRejectsPythonOutputContainingMaterializedSecret(t *testing.T) {
	requirePython3(t)

	secret := "goet-secret-sentinel-007-do-not-persist"
	t.Setenv("GOET_TEST_PYTHON_OUTPUT_SECRET", secret)

	worker := newPythonTestWorker(t)
	server := newPythonSourceServer(t, map[string]string{
		"scripts/run.py": strings.TrimSpace(`
import json
import os

with open(os.environ["GOET_OUTPUT_JSON"], "w", encoding="utf-8") as handle:
    json.dump({"leak": os.environ["GDRIVE_TOKEN"]}, handle)
`),
	})
	t.Cleanup(server.Close)
	worker.Config.ControllerURL = server.URL

	item := pythonTestItem("python-secret-output-001", "attempt-secret-output-001", model.Parameters{
		"python_entrypoint": {Type: "path", Value: "scripts/run.py"},
		"gdrive_token":      secretProtectedParameter("GOET_TEST_PYTHON_OUTPUT_SECRET", "env", "GDRIVE_TOKEN"),
	})

	_, err := worker.Run(item)
	if err == nil || !strings.Contains(err.Error(), "GOET_OUTPUT_JSON contains a materialized sensitive value") {
		t.Fatalf("expected secret output rejection, got %v", err)
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("error leaked secret: %v", err)
	}
	if _, err := os.Stat(filepath.Join(worker.Config.DataDir, item.OutputFilename)); !os.IsNotExist(err) {
		t.Fatalf("secret output should not be persisted, stat err=%v", err)
	}
}

func TestWorkerRunWorkItemMaterializesProtectedRefToTempFileAndRemovesIt(t *testing.T) {
	requirePython3(t)

	secret := "goet-secret-python-file-006"
	t.Setenv("GOET_TEST_PYTHON_FILE_SECRET", secret)

	worker := newPythonTestWorker(t)
	server := newPythonSourceServer(t, map[string]string{
		"scripts/run.py": strings.TrimSpace(`
import json
import os
import sys

secret_path = os.environ["GDRIVE_TOKEN_FILE"]
with open(secret_path, "r", encoding="utf-8") as handle:
    secret = handle.read()

print(secret)
with open(os.environ["GOET_OUTPUT_JSON"], "w", encoding="utf-8") as handle:
    json.dump({
        "secret_path": secret_path,
        "content_ok": secret == "goet-secret-python-file-006",
        "argv_has_secret": any(secret in arg for arg in sys.argv)
    }, handle)
`),
	})
	t.Cleanup(server.Close)
	worker.Config.ControllerURL = server.URL

	item := pythonTestItem("python-secret-file-001", "attempt-secret-file-001", model.Parameters{
		"python_entrypoint": {Type: "path", Value: "scripts/run.py"},
		"gdrive_token":      secretProtectedParameter("GOET_TEST_PYTHON_FILE_SECRET", "file", "GDRIVE_TOKEN_FILE"),
	})

	if _, err := worker.Run(item); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var output map[string]any
	data, err := os.ReadFile(filepath.Join(worker.Config.DataDir, item.OutputFilename))
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	if err := json.Unmarshal(data, &output); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if output["content_ok"] != true {
		t.Fatalf("secret file content was not available: %#v", output)
	}
	if output["argv_has_secret"] != false {
		t.Fatalf("secret reached argv: %#v", output)
	}
	secretPath, ok := output["secret_path"].(string)
	if !ok || secretPath == "" {
		t.Fatalf("missing secret path output: %#v", output)
	}
	if _, err := os.Stat(secretPath); !os.IsNotExist(err) {
		t.Fatalf("secret file should be removed after success, stat err=%v path=%s", err, secretPath)
	}

	stdoutLog := readString(t, filepath.Join(worker.Config.TmpDir, "attempts", item.AttemptID, "logs", "stdout.log"))
	if strings.Contains(stdoutLog, secret) {
		t.Fatalf("stdout leaked file-materialized secret: %q", stdoutLog)
	}
}

func TestWorkerRunWorkItemRemovesTempSecretFileAfterPythonFailure(t *testing.T) {
	requirePython3(t)

	secret := "goet-secret-python-file-failure-006"
	t.Setenv("GOET_TEST_PYTHON_FILE_FAILURE_SECRET", secret)

	worker := newPythonTestWorker(t)
	server := newPythonSourceServer(t, map[string]string{
		"scripts/run.py": strings.TrimSpace(`
import os
import sys

secret_path = os.environ["GDRIVE_TOKEN_FILE"]
with open(secret_path, "r", encoding="utf-8") as handle:
    secret = handle.read()
with open(os.path.join(os.environ["GOET_WORK_DIR"], "secret-path.txt"), "w", encoding="utf-8") as handle:
    handle.write(secret_path)
print(secret)
sys.exit(7)
`),
	})
	t.Cleanup(server.Close)
	worker.Config.ControllerURL = server.URL

	item := pythonTestItem("python-secret-file-failure-001", "attempt-secret-file-failure-001", model.Parameters{
		"python_entrypoint": {Type: "path", Value: "scripts/run.py"},
		"gdrive_token":      secretProtectedParameter("GOET_TEST_PYTHON_FILE_FAILURE_SECRET", "file", "GDRIVE_TOKEN_FILE"),
	})

	_, err := worker.Run(item)
	if err == nil || !strings.Contains(err.Error(), "python process exited with error") {
		t.Fatalf("expected python failure, got %v", err)
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("failure leaked secret: %v", err)
	}

	secretPath := readString(t, filepath.Join(worker.Config.TmpDir, "attempts", item.AttemptID, "work", "secret-path.txt"))
	if _, err := os.Stat(secretPath); !os.IsNotExist(err) {
		t.Fatalf("secret file should be removed after failure, stat err=%v path=%s", err, secretPath)
	}
	stdoutLog := readString(t, filepath.Join(worker.Config.TmpDir, "attempts", item.AttemptID, "logs", "stdout.log"))
	if strings.Contains(stdoutLog, secret) {
		t.Fatalf("stdout leaked secret after failure: %q", stdoutLog)
	}
}

func TestWorkerRunWorkItemMissingWorkerEnvSecretFailsSanitizedBeforePythonStarts(t *testing.T) {
	requirePython3(t)

	worker := newPythonTestWorker(t)
	server := newPythonSourceServer(t, map[string]string{
		"scripts/run.py": strings.TrimSpace(`
import os

with open(os.path.join(os.environ["GOET_DATA_DIR"], "python-ran.txt"), "w", encoding="utf-8") as handle:
    handle.write("ran")
`),
	})
	t.Cleanup(server.Close)
	worker.Config.ControllerURL = server.URL

	item := pythonTestItem("python-secret-missing-001", "attempt-secret-missing-001", model.Parameters{
		"python_entrypoint": {Type: "path", Value: "scripts/run.py"},
		"gdrive_token":      secretProtectedParameter("GOET_TEST_PYTHON_MISSING_SECRET", "env", "GDRIVE_TOKEN"),
	})

	_, err := worker.Run(item)
	if err == nil {
		t.Fatal("expected missing worker env error")
	}
	if !strings.Contains(err.Error(), "${worker_env.GOET_TEST_PYTHON_MISSING_SECRET}") {
		t.Fatalf("missing env error did not include redaction label: %v", err)
	}
	if _, err := os.Stat(filepath.Join(worker.Config.DataDir, "python-ran.txt")); !os.IsNotExist(err) {
		t.Fatalf("python should not start when worker env secret is missing, stat err=%v", err)
	}
}

func TestWorkerRunWorkItemFallsBackToFallbackLogOnLogDeliveryFailure(t *testing.T) {
	requirePython3(t)

	server := newPythonSourceAndLogServer(t, map[string]string{
		"scripts/run.py": strings.TrimSpace(`
import json
import os
output = {"ok": true}
with open(os.environ["GOET_OUTPUT_JSON"], "w", encoding="utf-8") as handle:
    json.dump(output, handle)
`),
	}, func(t *testing.T, w http.ResponseWriter, req *http.Request) {
		t.Helper()

		if req.Method != http.MethodPost {
			t.Fatalf("unexpected method for log endpoint: %s", req.Method)
		}
		if req.URL.Path != "/observations/logs" {
			t.Fatalf("unexpected path for log endpoint: %s", req.URL.Path)
		}

		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte("unavailable"))
	}, http.StatusServiceUnavailable)
	t.Cleanup(server.Close)

	worker := newPythonTestWorker(t)
	worker.Config.ControllerURL = server.URL

	item := pythonTestItem("python-fallback-001", "attempt-fallback-001", model.Parameters{
		"python_entrypoint": model.Parameter{Type: "path", Value: "scripts/run.py"},
	})

	if _, err := worker.Run(item); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	fallbackPath := filepath.Join(worker.Config.TmpDir, "attempts", item.AttemptID, "logs", "fallback-observations.jsonl")
	data, err := os.ReadFile(fallbackPath)
	if err != nil {
		t.Fatalf("expected fallback observations file %q: %v", fallbackPath, err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) == 0 || lines[0] == "" {
		t.Fatal("expected fallback observation lines")
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
		"python_entrypoint": model.Parameter{Type: "path", Value: "../escape.py"},
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
		"python_entrypoint":  model.Parameter{Type: "path", Value: "scripts/run.py"},
		"python_environment": model.Parameter{Type: "path", Value: "../env.json"},
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
		"python_entrypoint": model.Parameter{Type: "path", Value: "scripts/run.py"},
		"python_args":       model.Parameter{Type: "list", Value: []any{"ok", 7}},
	})

	if _, err := worker.Run(item); err == nil || !strings.Contains(err.Error(), "python_args") {
		t.Fatalf("expected invalid python_args error, got %v", err)
	}
}

func TestWorkerRunWorkItemPassesMaterializedDataAssetsToPython(t *testing.T) {
	requirePython3(t)

	worker := newPythonTestWorker(t)
	dataRoot := t.TempDir()
	worker.Config.DataLocationRoots = map[string]string{"fixture": dataRoot}
	writeFixture(t, dataRoot, "input.txt", "python asset")

	server := newPythonSourceServer(t, map[string]string{
		"scripts/run.py": strings.TrimSpace(`
import json
import os

with open(os.environ["GOET_DATA_ASSETS_JSON"], "r", encoding="utf-8") as handle:
    assets = json.load(handle)

output = {
    "asset_schema": assets["schema"],
    "binding_name": assets["assets"][0]["binding_name"],
    "local_path": assets["assets"][0]["local_path"],
    "source_sha256": assets["assets"][0]["source_sha256"]
}

with open(os.environ["GOET_OUTPUT_JSON"], "w", encoding="utf-8") as handle:
    json.dump(output, handle)
`),
	})
	t.Cleanup(server.Close)
	worker.Config.ControllerURL = server.URL

	asset := localFileAsset("fixture", "input.txt", model.DataAssetCacheStrategyReference, "", nil)
	item := pythonTestItem("python-assets-001", "attempt-assets-001", model.Parameters{
		"python_entrypoint": {Type: "path", Value: "scripts/run.py"},
		"data_assets":       {Type: "data_assets", Value: []model.BoundDataAsset{asset}},
	})

	if _, err := worker.Run(item); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var output map[string]any
	data, err := os.ReadFile(filepath.Join(worker.Config.DataDir, item.OutputFilename))
	if err != nil {
		t.Fatalf("read python output: %v", err)
	}
	if err := json.Unmarshal(data, &output); err != nil {
		t.Fatalf("decode python output: %v", err)
	}
	if output["asset_schema"] != model.MaterializedDataAssetManifestSchemaV1 {
		t.Fatalf("unexpected asset schema: %#v", output)
	}
	if output["binding_name"] != "input_data" {
		t.Fatalf("unexpected binding name: %#v", output)
	}
	if output["local_path"] != filepath.Join(dataRoot, "input.txt") {
		t.Fatalf("unexpected local path: %#v", output)
	}
	if output["source_sha256"] != sha256Text("python asset") {
		t.Fatalf("unexpected source hash: %#v", output)
	}
	if _, err := os.Stat(filepath.Join(worker.Config.TmpDir, "attempts", item.AttemptID, "work", "data-assets.json")); err != nil {
		t.Fatalf("expected materialized assets manifest: %v", err)
	}
}

func TestWorkerRunWorkItemRejectsDataAssetBeforePythonStarts(t *testing.T) {
	requirePython3(t)

	worker := newPythonTestWorker(t)
	dataRoot := t.TempDir()
	worker.Config.DataLocationRoots = map[string]string{"fixture": dataRoot}
	writeFixture(t, dataRoot, "input.txt", "actual asset")

	server := newPythonSourceServer(t, map[string]string{
		"scripts/run.py": strings.TrimSpace(`
import json
import os

with open(os.path.join(os.environ["GOET_DATA_DIR"], "python-ran.txt"), "w", encoding="utf-8") as handle:
    handle.write("ran")
with open(os.environ["GOET_OUTPUT_JSON"], "w", encoding="utf-8") as handle:
    json.dump({"ok": True}, handle)
`),
	})
	t.Cleanup(server.Close)
	worker.Config.ControllerURL = server.URL

	wrongHash := sha256Text("wrong asset")
	asset := localFileAsset("fixture", "input.txt", model.DataAssetCacheStrategyReference, "", &wrongHash)
	item := pythonTestItem("python-assets-bad-001", "attempt-assets-bad-001", model.Parameters{
		"python_entrypoint": {Type: "path", Value: "scripts/run.py"},
		"data_assets":       {Type: "data_assets", Value: []model.BoundDataAsset{asset}},
	})

	if _, err := worker.Run(item); err == nil || !strings.Contains(err.Error(), "expected sha256") {
		t.Fatalf("expected hash mismatch before python starts, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(worker.Config.DataDir, "python-ran.txt")); !os.IsNotExist(err) {
		t.Fatalf("python should not have created marker, stat err=%v", err)
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
		"python_entrypoint": model.Parameter{Type: "path", Value: "scripts/run.py"},
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
		"python_entrypoint": model.Parameter{Type: "path", Value: "scripts/run.py"},
	})

	if _, err := worker.Run(item); err == nil || !strings.Contains(err.Error(), "missing GOET_OUTPUT_JSON") {
		t.Fatalf("expected missing output json error, got %v", err)
	}
}

func TestWorkerRunWorkItemRejectsInvalidPythonOutputJSON(t *testing.T) {
	requirePython3(t)

	tests := []struct {
		name   string
		script string
		want   string
	}{
		{
			name: "invalid",
			script: `with open(os.environ["GOET_OUTPUT_JSON"], "w", encoding="utf-8") as handle:
    handle.write("{")`,
			want: "decode GOET_OUTPUT_JSON",
		},
		{
			name: "multiple",
			script: `with open(os.environ["GOET_OUTPUT_JSON"], "w", encoding="utf-8") as handle:
    handle.write('{"a":1} {"b":2}')`,
			want: "one JSON document",
		},
		{
			name: "trailing",
			script: `with open(os.environ["GOET_OUTPUT_JSON"], "w", encoding="utf-8") as handle:
    handle.write('{"a":1} trailing')`,
			want: "one JSON document",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			worker := newPythonTestWorker(t)
			server := newPythonSourceServer(t, map[string]string{
				"scripts/run.py": "import os\n" + tt.script + "\n",
			})
			t.Cleanup(server.Close)
			worker.Config.ControllerURL = server.URL

			item := pythonTestItem("python-bad-"+tt.name, "attempt-bad-"+tt.name, model.Parameters{
				"python_entrypoint": model.Parameter{Type: "path", Value: "scripts/run.py"},
			})

			if _, err := worker.Run(item); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("expected %q error, got %v", tt.want, err)
			}
		})
	}
}

func TestWorkerRunWorkItemPromotesPythonFileArtifact(t *testing.T) {
	requirePython3(t)

	worker := newPythonTestWorker(t)
	server := newPythonSourceServer(t, map[string]string{
		"scripts/run.py": strings.TrimSpace(`
import json
import os

artifact_path = os.path.join(os.environ["GOET_ARTIFACT_DIR"], "reports", "example.csv")
os.makedirs(os.path.dirname(artifact_path), exist_ok=True)
with open(artifact_path, "w", encoding="utf-8") as handle:
    handle.write("id,value\n1,a\n")

with open(os.environ["GOET_OUTPUT_JSON"], "w", encoding="utf-8") as handle:
    json.dump({
        "result": "ok",
        "artifacts": [
            {
                "name": "example_output",
                "kind": "file",
                "format": "csv",
                "path": "reports/example.csv"
            }
        ]
    }, handle)
`),
	})
	t.Cleanup(server.Close)
	worker.Config.ControllerURL = server.URL

	item := pythonTestItem("python-artifact-file-001", "attempt-artifact-file-001", model.Parameters{
		"python_entrypoint": {Type: "path", Value: "scripts/run.py"},
	})

	if _, err := worker.Run(item); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	manifest := readArtifactManifestOutput(t, filepath.Join(worker.Config.DataDir, item.OutputFilename))
	if manifest.StorageScope != artifactStorageScopeWorkerDataDir {
		t.Fatalf("storage scope = %q", manifest.StorageScope)
	}
	if len(manifest.Artifacts) != 1 {
		t.Fatalf("artifact count = %d", len(manifest.Artifacts))
	}
	artifact := manifest.Artifacts[0]
	if artifact.Path != "artifacts/raw/python-artifact-file-001/reports/example.csv" {
		t.Fatalf("artifact path = %q", artifact.Path)
	}
	if artifact.SHA256 != sha256Text("id,value\n1,a\n") {
		t.Fatalf("artifact sha256 = %q", artifact.SHA256)
	}
	if artifact.SizeBytes == nil || *artifact.SizeBytes != int64(len("id,value\n1,a\n")) {
		t.Fatalf("artifact size = %v", artifact.SizeBytes)
	}
	if got := readString(t, filepath.Join(worker.Config.DataDir, filepath.FromSlash(artifact.Path))); got != "id,value\n1,a\n" {
		t.Fatalf("promoted artifact = %q", got)
	}
	scriptOutput, ok := manifest.ScriptOutput.(map[string]any)
	if !ok || scriptOutput["result"] != "ok" {
		t.Fatalf("script output = %#v", manifest.ScriptOutput)
	}
}

func TestWorkerRunWorkItemDoesNotScanArtifactContentsForControlledSinkSentinel(t *testing.T) {
	requirePython3(t)

	secret := "goet-secret-sentinel-007-do-not-persist"
	t.Setenv("GOET_TEST_CONTROLLED_SINK_ARTIFACT_SECRET", secret)

	worker := newPythonTestWorker(t)
	server := newPythonSourceServer(t, map[string]string{
		"scripts/run.py": strings.TrimSpace(`
import json
import os

artifact_path = os.path.join(os.environ["GOET_ARTIFACT_DIR"], "reports", "leaky.txt")
os.makedirs(os.path.dirname(artifact_path), exist_ok=True)
with open(artifact_path, "w", encoding="utf-8") as handle:
    handle.write(os.environ["GDRIVE_TOKEN"])

with open(os.environ["GOET_OUTPUT_JSON"], "w", encoding="utf-8") as handle:
    json.dump({
        "result": "artifact content is outside phase-1 secret scanning",
        "artifacts": [
            {
                "name": "leaky_report",
                "kind": "file",
                "path": "reports/leaky.txt"
            }
        ]
    }, handle)
`),
	})
	t.Cleanup(server.Close)
	worker.Config.ControllerURL = server.URL

	item := pythonTestItem("python-artifact-secret-007", "attempt-artifact-secret-007", model.Parameters{
		"python_entrypoint": {Type: "path", Value: "scripts/run.py"},
		"gdrive_token":      secretProtectedParameter("GOET_TEST_CONTROLLED_SINK_ARTIFACT_SECRET", "env", "GDRIVE_TOKEN"),
	})

	evidence, err := worker.Run(item)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(evidence.OutputJSON, secret) {
		t.Fatalf("evidence leaked artifact secret: %s", evidence.OutputJSON)
	}

	manifest := readArtifactManifestOutput(t, filepath.Join(worker.Config.DataDir, item.OutputFilename))
	if len(manifest.Artifacts) != 1 {
		t.Fatalf("artifact count = %d, want one artifact", len(manifest.Artifacts))
	}
	artifactPath := filepath.Join(worker.Config.DataDir, filepath.FromSlash(manifest.Artifacts[0].Path))
	if got := readString(t, artifactPath); got != secret {
		t.Fatalf("artifact content = %q, want unscanned sentinel fixture", got)
	}
}

func TestWorkerRunWorkItemPromotesPythonDirectoryArtifact(t *testing.T) {
	requirePython3(t)

	worker := newPythonTestWorker(t)
	server := newPythonSourceServer(t, map[string]string{
		"scripts/run.py": strings.TrimSpace(`
import json
import os

root = os.path.join(os.environ["GOET_ARTIFACT_DIR"], "dataset")
os.makedirs(os.path.join(root, "nested"), exist_ok=True)
with open(os.path.join(root, "part-b.txt"), "w", encoding="utf-8") as handle:
    handle.write("b")
with open(os.path.join(root, "nested", "part-a.txt"), "w", encoding="utf-8") as handle:
    handle.write("aa")

with open(os.environ["GOET_OUTPUT_JSON"], "w", encoding="utf-8") as handle:
    json.dump({
        "result": "ok",
        "artifacts": [
            {
                "name": "dataset",
                "kind": "directory",
                "path": "dataset"
            }
        ]
    }, handle)
`),
	})
	t.Cleanup(server.Close)
	worker.Config.ControllerURL = server.URL

	item := pythonTestItem("python-artifact-dir-001", "attempt-artifact-dir-001", model.Parameters{
		"python_entrypoint": {Type: "path", Value: "scripts/run.py"},
	})

	if _, err := worker.Run(item); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	manifest := readArtifactManifestOutput(t, filepath.Join(worker.Config.DataDir, item.OutputFilename))
	artifact := manifest.Artifacts[0]
	promotedPath := filepath.Join(worker.Config.DataDir, filepath.FromSlash(artifact.Path))
	evidence, err := directoryManifestEvidence(promotedPath)
	if err != nil {
		t.Fatalf("directory evidence: %v", err)
	}
	if artifact.ManifestSHA256 != evidence.sha256 {
		t.Fatalf("manifest sha256 = %q, want %q", artifact.ManifestSHA256, evidence.sha256)
	}
	if artifact.SizeBytes == nil || *artifact.SizeBytes != int64(len("b")+len("aa")) {
		t.Fatalf("artifact size = %v", artifact.SizeBytes)
	}
}

func TestWorkerRunWorkItemPublishesPythonFileArtifact(t *testing.T) {
	requirePython3(t)

	worker := newPythonTestWorker(t)
	publishedRoot := t.TempDir()
	worker.Config.DataLocationRoots = map[string]string{"published_data": publishedRoot}
	server := newPythonSourceServer(t, map[string]string{
		"scripts/run.py": strings.TrimSpace(`
import json
import os

artifact_path = os.path.join(os.environ["GOET_ARTIFACT_DIR"], "reports", "example.csv")
os.makedirs(os.path.dirname(artifact_path), exist_ok=True)
with open(artifact_path, "w", encoding="utf-8") as handle:
    handle.write("id,value\n1,a\n")

with open(os.environ["GOET_OUTPUT_JSON"], "w", encoding="utf-8") as handle:
    json.dump({
        "result": "ok",
        "artifacts": [
            {
                "name": "example_output",
                "kind": "file",
                "format": "csv",
                "path": "reports/example.csv"
            }
        ]
    }, handle)
`),
	})
	t.Cleanup(server.Close)
	worker.Config.ControllerURL = server.URL

	item := pythonTestItem("python-publish-file-001", "attempt-publish-file-001", model.Parameters{
		"python_entrypoint": {Type: "path", Value: "scripts/run.py"},
		"publish": {Type: "publish_targets", Value: map[string]any{
			"publish_example": map[string]any{
				"from_artifact": "example_output",
				"location": map[string]any{
					"type":          model.DataProviderRegisteredLocation,
					"location_name": "published_data",
					"path":          "reports/example.csv",
				},
				"overwrite_policy": model.PublishedDataAssetOverwriteFailIfExists,
			},
		}},
	})

	if _, err := worker.Run(item); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	manifest := readArtifactManifestOutput(t, filepath.Join(worker.Config.DataDir, item.OutputFilename))
	if len(manifest.PublishedAssets) != 1 {
		t.Fatalf("published asset count = %d", len(manifest.PublishedAssets))
	}
	published := manifest.PublishedAssets[0]
	if published.Name != "publish_example" {
		t.Fatalf("published name = %q", published.Name)
	}
	if published.FromArtifact != "example_output" {
		t.Fatalf("published from_artifact = %q", published.FromArtifact)
	}
	if published.StorageScope != model.DataLocationTypeRegistered {
		t.Fatalf("published storage scope = %q", published.StorageScope)
	}
	if published.LocationName != "published_data" {
		t.Fatalf("published location_name = %q", published.LocationName)
	}
	if published.Path != "reports/example.csv" {
		t.Fatalf("published path = %q", published.Path)
	}
	if published.SHA256 != sha256Text("id,value\n1,a\n") {
		t.Fatalf("published sha256 = %q", published.SHA256)
	}
	if published.SizeBytes == nil || *published.SizeBytes != int64(len("id,value\n1,a\n")) {
		t.Fatalf("published size = %v", published.SizeBytes)
	}
	if got := readString(t, filepath.Join(publishedRoot, "reports", "example.csv")); got != "id,value\n1,a\n" {
		t.Fatalf("published file = %q", got)
	}
}

func TestWorkerRunWorkItemPublishesPythonDirectoryArtifact(t *testing.T) {
	requirePython3(t)

	worker := newPythonTestWorker(t)
	publishedRoot := t.TempDir()
	worker.Config.DataLocationRoots = map[string]string{"published_data": publishedRoot}
	server := newPythonSourceServer(t, map[string]string{
		"scripts/run.py": strings.TrimSpace(`
import json
import os

root = os.path.join(os.environ["GOET_ARTIFACT_DIR"], "dataset")
os.makedirs(os.path.join(root, "nested"), exist_ok=True)
with open(os.path.join(root, "part-b.txt"), "w", encoding="utf-8") as handle:
    handle.write("b")
with open(os.path.join(root, "nested", "part-a.txt"), "w", encoding="utf-8") as handle:
    handle.write("aa")

with open(os.environ["GOET_OUTPUT_JSON"], "w", encoding="utf-8") as handle:
    json.dump({
        "result": "ok",
        "artifacts": [
            {
                "name": "dataset",
                "kind": "directory",
                "path": "dataset"
            }
        ]
    }, handle)
`),
	})
	t.Cleanup(server.Close)
	worker.Config.ControllerURL = server.URL

	item := pythonTestItem("python-publish-dir-001", "attempt-publish-dir-001", model.Parameters{
		"python_entrypoint": {Type: "path", Value: "scripts/run.py"},
		"publish": {Type: "publish_targets", Value: []model.BoundPublishTarget{
			{
				Name:            "publish_dataset",
				FromArtifact:    "dataset",
				TargetName:      "publish_dataset",
				Location:        model.DataAssetLocation{Type: model.DataProviderRegisteredLocation, LocationName: "published_data", Path: "dataset/year=2024"},
				OverwritePolicy: model.PublishedDataAssetOverwriteFailIfExists,
			},
		}},
	})

	if _, err := worker.Run(item); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	manifest := readArtifactManifestOutput(t, filepath.Join(worker.Config.DataDir, item.OutputFilename))
	if len(manifest.PublishedAssets) != 1 {
		t.Fatalf("published asset count = %d", len(manifest.PublishedAssets))
	}
	published := manifest.PublishedAssets[0]
	publishedPath := filepath.Join(publishedRoot, "dataset", "year=2024")
	evidence, err := directoryManifestEvidence(publishedPath)
	if err != nil {
		t.Fatalf("directory evidence: %v", err)
	}
	if published.SHA256 != evidence.sha256 {
		t.Fatalf("published sha256 = %q, want %q", published.SHA256, evidence.sha256)
	}
	if published.SizeBytes == nil || *published.SizeBytes != evidence.size {
		t.Fatalf("published size = %v, want %d", published.SizeBytes, evidence.size)
	}
	if got := readString(t, filepath.Join(publishedPath, "nested", "part-a.txt")); got != "aa" {
		t.Fatalf("published nested file = %q", got)
	}
}

func TestWorkerRunWorkItemRejectsUnsafePythonArtifactPath(t *testing.T) {
	requirePython3(t)

	worker := newPythonTestWorker(t)
	server := newPythonSourceServer(t, map[string]string{
		"scripts/run.py": strings.TrimSpace(`
import json
import os

with open(os.environ["GOET_OUTPUT_JSON"], "w", encoding="utf-8") as handle:
    json.dump({
        "result": "bad",
        "artifacts": [
            {
                "name": "bad",
                "kind": "file",
                "path": "../escape.txt"
            }
        ]
    }, handle)
`),
	})
	t.Cleanup(server.Close)
	worker.Config.ControllerURL = server.URL

	item := pythonTestItem("python-artifact-unsafe-001", "attempt-artifact-unsafe-001", model.Parameters{
		"python_entrypoint": {Type: "path", Value: "scripts/run.py"},
	})

	if _, err := worker.Run(item); err == nil || !strings.Contains(err.Error(), "artifacts[0]") {
		t.Fatalf("expected unsafe artifact error, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(worker.Config.DataDir, item.OutputFilename)); !os.IsNotExist(err) {
		t.Fatalf("failed artifact promotion should not write logical output, stat err=%v", err)
	}
}

func TestWorkerRunWorkItemRejectsMissingPythonArtifact(t *testing.T) {
	requirePython3(t)

	worker := newPythonTestWorker(t)
	server := newPythonSourceServer(t, map[string]string{
		"scripts/run.py": strings.TrimSpace(`
import json
import os

with open(os.environ["GOET_OUTPUT_JSON"], "w", encoding="utf-8") as handle:
    json.dump({
        "result": "missing",
        "artifacts": [
            {
                "name": "missing",
                "kind": "file",
                "path": "missing.txt"
            }
        ]
    }, handle)
`),
	})
	t.Cleanup(server.Close)
	worker.Config.ControllerURL = server.URL

	item := pythonTestItem("python-artifact-missing-001", "attempt-artifact-missing-001", model.Parameters{
		"python_entrypoint": {Type: "path", Value: "scripts/run.py"},
	})

	if _, err := worker.Run(item); err == nil || !strings.Contains(err.Error(), "check artifact source") {
		t.Fatalf("expected missing artifact error, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(worker.Config.DataDir, item.OutputFilename)); !os.IsNotExist(err) {
		t.Fatalf("failed artifact promotion should not write logical output, stat err=%v", err)
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

func newPythonSourceAndLogServer(t *testing.T, files map[string]string, logHandler func(*testing.T, http.ResponseWriter, *http.Request), status ...int) *httptest.Server {
	t.Helper()
	logStatus := http.StatusCreated
	if len(status) > 0 {
		logStatus = status[0]
	}

	body := mustPythonSourceBundle(t, files)

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/source-bundle.zip") {
			w.Header().Set("Content-Type", "application/zip")
			_, _ = w.Write(body)
			return
		}

		if r.URL.Path == "/observations/logs" {
			logHandler(t, w, r)
			w.WriteHeader(logStatus)
			return
		}

		http.NotFound(w, r)
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

func containsAll(values []string, want []string) bool {
	for _, item := range want {
		found := false
		for _, value := range values {
			if value == item {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
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

func secretProtectedParameter(key string, mode string, target string) model.Parameter {
	return model.Parameter{
		Type:         "string",
		ProtectedRef: &variable.ProtectedRef{Provider: "worker_env", Key: key},
		Materialize:  &model.ParameterMaterialization{Mode: mode, Target: target},
	}
}

func readArtifactManifestOutput(t *testing.T, path string) model.ArtifactManifest {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read artifact manifest output: %v", err)
	}
	var manifest model.ArtifactManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatalf("decode artifact manifest output: %v", err)
	}
	if err := manifest.Validate(); err != nil {
		t.Fatalf("validate artifact manifest output: %v", err)
	}
	return manifest
}
