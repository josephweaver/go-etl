package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"goetl/internal/model"
)

type pythonInputDocument struct {
	WorkItem model.WorkItem `json:"work_item"`
}

func (w Worker) runPythonScript(item model.WorkItem) (WorkEvidence, error) {
	staging, err := w.stageWorkItemSourceBundle(item)
	if err != nil {
		return WorkEvidence{}, err
	}

	entrypointValue, err := stringParameter(item, "python_entrypoint")
	if err != nil {
		return WorkEvidence{}, fmt.Errorf("resolve python_entrypoint: %w", err)
	}

	entrypointPath, err := resolveSourcePathWithinRoot(staging.SourceDir, entrypointValue, "python_entrypoint")
	if err != nil {
		return WorkEvidence{}, err
	}

	environmentPath, hasEnvironment, err := optionalSourcePathParameter(item, staging.SourceDir, "python_environment")
	if err != nil {
		return WorkEvidence{}, err
	}

	args, err := pythonArgsParameter(item)
	if err != nil {
		return WorkEvidence{}, err
	}

	dataAssetsPath, hasDataAssets, err := w.materializeDataAssets(item, staging.WorkDir)
	if err != nil {
		return WorkEvidence{}, err
	}
	args, err = resolvePythonArgvBindings(args, dataAssetsPath, staging.ArtifactDir)
	if err != nil {
		return WorkEvidence{}, err
	}

	inputDocument := pythonInputDocument{WorkItem: item}
	inputJSON, err := json.MarshalIndent(inputDocument, "", "  ")
	if err != nil {
		return WorkEvidence{}, fmt.Errorf("encode python input json: %w", err)
	}

	inputPath := filepath.Join(staging.WorkDir, "input.json")
	if err := os.WriteFile(inputPath, inputJSON, 0644); err != nil {
		return WorkEvidence{}, fmt.Errorf("write python input json %s: %w", inputPath, err)
	}

	outputPath := filepath.Join(staging.WorkDir, "output.json")
	stdoutPath := filepath.Join(staging.LogDir, "stdout.log")
	stderrPath := filepath.Join(staging.LogDir, "stderr.log")

	stdoutFile, err := os.OpenFile(stdoutPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return WorkEvidence{}, fmt.Errorf("open stdout log %s: %w", stdoutPath, err)
	}
	defer stdoutFile.Close()

	stderrFile, err := os.OpenFile(stderrPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return WorkEvidence{}, fmt.Errorf("open stderr log %s: %w", stderrPath, err)
	}

	command := exec.Command(w.pythonExecutable(), append([]string{entrypointPath}, args...)...)
	command.Dir = staging.SourceDir
	command.Stdout = stdoutFile
	command.Stderr = stderrFile
	command.Env = append(os.Environ(),
		"GOET_WORK_ITEM_ID="+item.ID,
		"GOET_ATTEMPT_ID="+item.AttemptID,
		"GOET_INPUT_JSON="+inputPath,
		"GOET_OUTPUT_JSON="+outputPath,
		"GOET_SOURCE_DIR="+staging.SourceDir,
		"GOET_WORK_DIR="+staging.WorkDir,
		"GOET_ARTIFACT_DIR="+staging.ArtifactDir,
		"GOET_DATA_DIR="+w.Config.DataDir,
		"GOET_TMP_DIR="+w.Config.TmpDir,
		"GOET_LOG_DIR="+staging.LogDir,
		"GOET_PYTHON_ENTRYPOINT="+entrypointPath,
	)
	if hasEnvironment {
		command.Env = append(command.Env, "GOET_PYTHON_ENVIRONMENT_JSON="+environmentPath)
	}
	if hasDataAssets {
		command.Env = append(command.Env, "GOET_DATA_ASSETS_JSON="+dataAssetsPath)
	}

	secretEnv, redactor, cleanupSecrets, err := w.materializePythonProtectedRefs(context.Background(), item, staging.WorkDir)
	if err != nil {
		return WorkEvidence{}, err
	}
	defer cleanupSecrets()
	command.Env = append(command.Env, secretEnv...)

	if err := command.Start(); err != nil {
		_ = stdoutFile.Close()
		_ = stderrFile.Close()
		return WorkEvidence{}, fmt.Errorf("launch python process: %w", err)
	}

	if err := command.Wait(); err != nil {
		_ = stdoutFile.Close()
		_ = stderrFile.Close()
		if scrubErr := scrubPythonSubprocessLogs(stdoutPath, stderrPath, redactor); scrubErr != nil {
			return WorkEvidence{}, scrubErr
		}
		return WorkEvidence{}, fmt.Errorf("python process exited with error: %w", err)
	}

	if err := stdoutFile.Close(); err != nil {
		_ = stderrFile.Close()
		return WorkEvidence{}, fmt.Errorf("close stdout log %s: %w", stdoutPath, err)
	}
	if err := stderrFile.Close(); err != nil {
		return WorkEvidence{}, fmt.Errorf("close stderr log %s: %w", stderrPath, err)
	}

	if err := scrubPythonSubprocessLogs(stdoutPath, stderrPath, redactor); err != nil {
		return WorkEvidence{}, err
	}

	_ = w.emitPythonSubprocessLogLines(item, stdoutPath, stderrPath, staging.LogDir)

	outputJSON, err := os.ReadFile(outputPath)
	if os.IsNotExist(err) {
		return WorkEvidence{}, fmt.Errorf("missing GOET_OUTPUT_JSON after successful python process exit: %s", outputPath)
	}
	if err != nil {
		return WorkEvidence{}, fmt.Errorf("read python output json %s: %w", outputPath, err)
	}
	if redactor.RedactString(string(outputJSON)) != string(outputJSON) {
		return WorkEvidence{}, fmt.Errorf("GOET_OUTPUT_JSON contains a materialized sensitive value")
	}

	logicalOutput, outputSHA256, err := w.pythonLogicalOutput(item, outputJSON, staging.ArtifactDir)
	if err != nil {
		return WorkEvidence{}, err
	}

	dataPath := filepath.Join(w.Config.DataDir, item.OutputFilename)
	preState, err := outputFileState(dataPath)
	if err != nil {
		return WorkEvidence{}, err
	}

	if err := atomicWriteFile(dataPath, logicalOutput, 0644); err != nil {
		return WorkEvidence{}, fmt.Errorf("write completed python output %s: %w", dataPath, err)
	}

	postState, err := outputFileState(dataPath)
	if err != nil {
		return WorkEvidence{}, err
	}

	stdoutSHA256, hasStdout, err := logFileSHA256(stdoutPath)
	if err != nil {
		return WorkEvidence{}, err
	}
	stderrSHA256, hasStderr, err := logFileSHA256(stderrPath)
	if err != nil {
		return WorkEvidence{}, err
	}
	if !hasStdout {
		stdoutSHA256 = ""
	}
	if !hasStderr {
		stderrSHA256 = ""
	}

	inputSHA256, err := pythonInputObservationSHA256(item, entrypointPath, environmentPath, args, inputDocument)
	if err != nil {
		return WorkEvidence{}, err
	}

	preStateSHA256, err := canonicalObservationSHA256(preState)
	if err != nil {
		return WorkEvidence{}, err
	}
	postStateSHA256, err := canonicalObservationSHA256(postState)
	if err != nil {
		return WorkEvidence{}, err
	}

	outputJSONText, err := pythonOutputEvidenceJSONText(
		item,
		entrypointPath,
		environmentPath,
		0,
		logicalOutput,
		inputSHA256,
		outputSHA256,
		preStateSHA256,
		postStateSHA256,
		stdoutSHA256,
		stderrSHA256,
	)
	if err != nil {
		return WorkEvidence{}, err
	}

	preStateJSON, err := json.Marshal(preState)
	if err != nil {
		return WorkEvidence{}, fmt.Errorf("encode pre-state evidence: %w", err)
	}

	postStateJSON, err := json.Marshal(postState)
	if err != nil {
		return WorkEvidence{}, fmt.Errorf("encode post-state evidence: %w", err)
	}

	return WorkEvidence{
		InputSHA256:     inputSHA256,
		OutputSHA256:    outputSHA256,
		PreStateSHA256:  preStateSHA256,
		PostStateSHA256: postStateSHA256,
		OutputJSON:      outputJSONText,
		PreStateJSON:    string(preStateJSON),
		PostStateJSON:   string(postStateJSON),
	}, nil
}

func (w Worker) materializePythonProtectedRefs(ctx context.Context, item model.WorkItem, workDir string) ([]string, *Redactor, func(), error) {
	envelope := item.ExecutionEnvelope
	if envelope == nil {
		built, err := model.NewExecutionEnvelope(item)
		if err != nil {
			return nil, nil, nil, err
		}
		envelope = &built
	}

	redactor := NewRedactor()
	cleanup := func() {}
	if len(envelope.Variables.ProtectedRefs) == 0 {
		return nil, redactor, cleanup, nil
	}

	resolver := WorkerEnvProtectedValueResolver{}
	secretEnv := []string{}
	secretDir := ""
	secretFileCount := 0

	for _, name := range sortedKeys(envelope.Variables.ProtectedRefs) {
		ref := envelope.Variables.ProtectedRefs[name]
		if ref.Materialize == nil {
			continue
		}

		value, err := resolver.ResolveProtectedValue(ctx, ref)
		if err != nil {
			cleanup()
			return nil, nil, nil, fmt.Errorf("resolve python materialized sensitive value %s: %w", name, err)
		}
		redactor.Register(value)

		switch ref.Materialize.Mode {
		case "env":
			secretEnv = append(secretEnv, ref.Materialize.Target+"="+value.Plaintext())
		case "file":
			if secretDir == "" {
				secretDir = filepath.Join(workDir, "secret-materializations")
				if err := os.MkdirAll(secretDir, 0700); err != nil {
					return nil, nil, nil, fmt.Errorf("create python secret materialization directory: %w", err)
				}
				cleanup = func() {
					_ = os.RemoveAll(secretDir)
				}
			}
			secretFileCount++
			path := filepath.Join(secretDir, fmt.Sprintf("secret-%d", secretFileCount))
			if err := os.WriteFile(path, []byte(value.Plaintext()), 0600); err != nil {
				cleanup()
				return nil, nil, nil, fmt.Errorf("write python secret materialization file: %w", err)
			}
			secretEnv = append(secretEnv, ref.Materialize.Target+"="+path)
		default:
			cleanup()
			return nil, nil, nil, fmt.Errorf("unsupported python secret materialization mode %q", ref.Materialize.Mode)
		}
	}

	return secretEnv, redactor, cleanup, nil
}

func scrubPythonSubprocessLogs(stdoutPath string, stderrPath string, redactor *Redactor) error {
	if err := scrubPythonSubprocessLog(stdoutPath, redactor); err != nil {
		return err
	}
	return scrubPythonSubprocessLog(stderrPath, redactor)
}

func scrubPythonSubprocessLog(path string, redactor *Redactor) error {
	if redactor == nil {
		return nil
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read python subprocess log %s for redaction: %w", path, err)
	}
	redacted := redactor.RedactBytes(data)
	if string(redacted) == string(data) {
		return nil
	}
	if err := atomicWriteFile(path, redacted, 0644); err != nil {
		return fmt.Errorf("rewrite redacted python subprocess log %s: %w", path, err)
	}
	return nil
}

func (w Worker) pythonLogicalOutput(item model.WorkItem, outputJSON []byte, artifactDir string) ([]byte, string, error) {
	logicalOutput, outputSHA256, decoded, err := canonicalJSONDocument(outputJSON, "GOET_OUTPUT_JSON")
	if err != nil {
		return nil, "", err
	}

	artifacts, scriptOutput, hasArtifacts, err := pythonArtifactDeclarations(decoded)
	if err != nil {
		return nil, "", err
	}
	if !hasArtifacts {
		return logicalOutput, outputSHA256, nil
	}

	runID := ""
	if item.Source != nil {
		runID = item.Source.RunID
	}
	promoted, err := PromoteArtifacts(context.Background(), ArtifactPromotionRequest{
		StagingRoot: artifactDir,
		DataRoot:    w.Config.DataDir,
		RunID:       runID,
		WorkItemID:  item.ID,
		AttemptID:   item.AttemptID,
		Manifest: model.ArtifactManifest{
			Schema:       model.ArtifactManifestSchemaV1,
			StorageScope: "artifact_staging",
			Artifacts:    artifacts,
			ScriptOutput: scriptOutput,
		},
	})
	if err != nil {
		return nil, "", fmt.Errorf("promote python artifacts: %w", err)
	}

	publishedAssets, err := w.publishPromotedArtifacts(item, promoted)
	if err != nil {
		return nil, "", fmt.Errorf("publish python artifacts: %w", err)
	}
	if len(publishedAssets) > 0 {
		promoted.PublishedAssets = publishedAssets
	}

	data, err := json.Marshal(promoted)
	if err != nil {
		return nil, "", fmt.Errorf("encode promoted artifact manifest: %w", err)
	}
	canonical, hash, _, err := canonicalJSONDocument(data, "promoted artifact manifest")
	if err != nil {
		return nil, "", err
	}
	return canonical, hash, nil
}

func pythonArtifactDeclarations(decoded any) ([]model.ArtifactDescriptor, any, bool, error) {
	object, ok := decoded.(map[string]any)
	if !ok {
		return nil, nil, false, nil
	}
	raw, ok := object["artifacts"]
	if !ok {
		return nil, nil, false, nil
	}

	data, err := json.Marshal(raw)
	if err != nil {
		return nil, nil, false, fmt.Errorf("encode artifacts declaration: %w", err)
	}
	var artifacts []model.ArtifactDescriptor
	if err := json.Unmarshal(data, &artifacts); err != nil {
		return nil, nil, false, fmt.Errorf("decode artifacts declaration: %w", err)
	}
	if len(artifacts) == 0 {
		return nil, nil, false, nil
	}
	for i, artifact := range artifacts {
		if err := artifact.Validate(); err != nil {
			return nil, nil, false, fmt.Errorf("artifacts[%d]: %w", i, err)
		}
	}

	scriptOutput := make(map[string]any, len(object)-1)
	for key, value := range object {
		if key == "artifacts" {
			continue
		}
		scriptOutput[key] = value
	}
	return artifacts, scriptOutput, true, nil
}

func (w Worker) emitPythonSubprocessLogLines(item model.WorkItem, stdoutPath string, stderrPath string, fallbackLogDir string) error {
	if w.LocalOnly {
		return nil
	}
	if err := w.emitPythonSubprocessLogLinesFromPath(item, stdoutPath, model.LogStreamStdout, model.LogLevelInfo, fallbackLogDir); err != nil {
		return err
	}

	return w.emitPythonSubprocessLogLinesFromPath(item, stderrPath, model.LogStreamStderr, model.LogLevelWarn, fallbackLogDir)
}

func (w Worker) emitPythonSubprocessLogLinesFromPath(
	item model.WorkItem,
	path string,
	stream string,
	level model.LogLevel,
	fallbackLogDir string,
) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open %s log %s: %w", stream, path, err)
	}
	defer file.Close()

	controller, err := w.controllerClient()
	if err != nil {
		return fmt.Errorf("controller client: %w", err)
	}
	client := LogClient{Controller: controller}
	scanner := bufio.NewScanner(file)
	var observedErr error

	submissionID := ""
	runID := ""
	if item.Source != nil {
		submissionID = item.Source.RunID
		runID = item.Source.RunID
	}

	sequence := uint64(1)
	for scanner.Scan() {
		line := scanner.Text()

		observation := model.LogObservation{
			Component:    "worker-python",
			Stream:       stream,
			Level:        level,
			SubmissionID: submissionID,
			WorkflowID:   item.WorkflowDefinitionID,
			WorkItemID:   item.ID,
			AttemptID:    item.AttemptID,
			RunID:        runID,
			StepID:       item.StepDefinitionID,
			Timestamp:    time.Now().UTC().Format(time.RFC3339Nano),
			Sequence:     sequence,
			Message:      line,
		}
		sequence++

		if err := client.SendLogObservationWithFallback(observation, fallbackLogDir); err != nil {
			observedErr = err
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("scan %s log %s: %w", stream, path, err)
	}

	return observedErr
}

func (w Worker) pythonExecutable() string {
	executable := strings.TrimSpace(w.Config.PythonExecutable)
	if executable == "" {
		return "python3"
	}
	return executable
}

func optionalSourcePathParameter(item model.WorkItem, root string, name string) (string, bool, error) {
	parameter, ok := item.Parameters[name]
	if !ok {
		return "", false, nil
	}

	if parameter.Type != "string" && parameter.Type != "path" {
		return "", false, fmt.Errorf("parameter %s has type %s, want string or path", name, parameter.Type)
	}

	value, ok := parameter.Value.(string)
	if !ok || strings.TrimSpace(value) == "" {
		return "", false, fmt.Errorf("parameter %s value must be a non-empty string", name)
	}

	path, err := resolveSourcePathWithinRoot(root, value, name)
	if err != nil {
		return "", false, err
	}

	return path, true, nil
}

func resolveSourcePathWithinRoot(root string, value string, name string) (string, error) {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("resolve %s root %s: %w", name, root, err)
	}

	candidate := value
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(rootAbs, candidate)
	}
	candidate = filepath.Clean(candidate)

	rel, err := filepath.Rel(rootAbs, candidate)
	if err != nil {
		return "", fmt.Errorf("resolve %s path %s: %w", name, value, err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("unsafe %s path: %s", name, value)
	}

	info, err := os.Stat(candidate)
	if err != nil {
		return "", fmt.Errorf("check %s path %s: %w", name, candidate, err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("%s path is a directory: %s", name, candidate)
	}

	return candidate, nil
}

func pythonArgsParameter(item model.WorkItem) ([]string, error) {
	parameter, ok := item.Parameters["python_args"]
	if !ok {
		return nil, nil
	}
	if parameter.Type != "list" {
		return nil, fmt.Errorf("parameter python_args has type %s, want list", parameter.Type)
	}

	switch value := parameter.Value.(type) {
	case []string:
		return append([]string(nil), value...), nil
	case []any:
		args := make([]string, len(value))
		for i, raw := range value {
			text, ok := raw.(string)
			if !ok {
				return nil, fmt.Errorf("parameter python_args must be a list of strings")
			}
			args[i] = text
		}
		return args, nil
	default:
		return nil, fmt.Errorf("parameter python_args must be a list of strings")
	}
}
