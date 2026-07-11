package document

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

const (
	APIVersionV1Alpha1 = "goet/v1alpha1"

	KindController          = "Controller"
	KindProject             = "Project"
	KindWorkflow            = "Workflow"
	KindSubmissionOverrides = "SubmissionOverrides"
)

type VariableValues map[string]json.RawMessage

type ControllerDocument struct {
	APIVersion           string          `json:"api_version"`
	Kind                 string          `json:"kind"`
	ID                   string          `json:"id"`
	Variables            VariableValues  `json:"variables,omitempty"`
	ExecutionEnvironment json.RawMessage `json:"execution_environment,omitempty"`
}

type ProjectDocument struct {
	APIVersion     string          `json:"api_version"`
	Kind           string          `json:"kind"`
	ID             string          `json:"id"`
	Variables      VariableValues  `json:"variables,omitempty"`
	Data           json.RawMessage `json:"data,omitempty"`
	SourceManifest json.RawMessage `json:"source_manifest,omitempty"`
}

type WorkflowDocument struct {
	APIVersion     string          `json:"api_version"`
	Kind           string          `json:"kind"`
	ID             string          `json:"id"`
	Variables      VariableValues  `json:"variables,omitempty"`
	Data           json.RawMessage `json:"data,omitempty"`
	Steps          []WorkflowStep  `json:"steps,omitempty"`
	SourceManifest json.RawMessage `json:"source_manifest,omitempty"`
}

type WorkflowStep struct {
	ID           string          `json:"id"`
	ParallelWith json.RawMessage `json:"parallel_with,omitempty"`
	FanOut       json.RawMessage `json:"fan_out,omitempty"`
	Data         json.RawMessage `json:"data,omitempty"`
	Work         json.RawMessage `json:"work,omitempty"`
}

type SubmissionOverridesDocument struct {
	APIVersion string          `json:"api_version"`
	Kind       string          `json:"kind"`
	ID         string          `json:"id"`
	Overrides  VariableValues  `json:"overrides,omitempty"`
	Data       json.RawMessage `json:"data,omitempty"`
}

func DecodeStrictController(data []byte) (ControllerDocument, error) {
	var document ControllerDocument
	if err := decodeStrictTopLevel(data, controllerTopLevelFields, &document); err != nil {
		return ControllerDocument{}, err
	}
	if err := document.ValidateEnvelope(); err != nil {
		return ControllerDocument{}, err
	}
	return document, nil
}

func DecodeStrictProject(data []byte) (ProjectDocument, error) {
	var document ProjectDocument
	if err := decodeStrictTopLevel(data, projectTopLevelFields, &document); err != nil {
		return ProjectDocument{}, err
	}
	if err := document.ValidateEnvelope(); err != nil {
		return ProjectDocument{}, err
	}
	return document, nil
}

func DecodeStrictWorkflow(data []byte) (WorkflowDocument, error) {
	var document WorkflowDocument
	if err := decodeStrictTopLevel(data, workflowTopLevelFields, &document); err != nil {
		return WorkflowDocument{}, err
	}
	if err := document.ValidateEnvelope(); err != nil {
		return WorkflowDocument{}, err
	}
	return document, nil
}

func DecodeStrictSubmissionOverrides(data []byte) (SubmissionOverridesDocument, error) {
	var document SubmissionOverridesDocument
	if err := decodeStrictTopLevel(data, submissionOverridesTopLevelFields, &document); err != nil {
		return SubmissionOverridesDocument{}, err
	}
	if err := document.ValidateEnvelope(); err != nil {
		return SubmissionOverridesDocument{}, err
	}
	return document, nil
}

func (document ControllerDocument) ValidateEnvelope() error {
	return validateEnvelope(document.APIVersion, document.Kind, KindController, document.ID)
}

func (document ProjectDocument) ValidateEnvelope() error {
	return validateEnvelope(document.APIVersion, document.Kind, KindProject, document.ID)
}

func (document WorkflowDocument) ValidateEnvelope() error {
	return validateEnvelope(document.APIVersion, document.Kind, KindWorkflow, document.ID)
}

func (document SubmissionOverridesDocument) ValidateEnvelope() error {
	return validateEnvelope(document.APIVersion, document.Kind, KindSubmissionOverrides, document.ID)
}

func validateEnvelope(apiVersion string, kind string, wantKind string, id string) error {
	if apiVersion != APIVersionV1Alpha1 {
		return fmt.Errorf("api_version must be %q, got %q", APIVersionV1Alpha1, apiVersion)
	}
	if kind != wantKind {
		return fmt.Errorf("kind must be %q, got %q", wantKind, kind)
	}
	if id == "" {
		return fmt.Errorf("id is required")
	}
	return nil
}

func decodeStrictTopLevel(data []byte, allowed map[string]struct{}, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()

	var fields map[string]json.RawMessage
	if err := decoder.Decode(&fields); err != nil {
		return fmt.Errorf("decode document: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return fmt.Errorf("document must contain one JSON document")
	}
	for name := range fields {
		if _, ok := allowed[name]; !ok {
			return fmt.Errorf("unknown top-level field %q", name)
		}
	}

	if err := json.Unmarshal(data, target); err != nil {
		return fmt.Errorf("decode document fields: %w", err)
	}
	return nil
}

var controllerTopLevelFields = fieldSet(
	"api_version",
	"kind",
	"id",
	"variables",
	"execution_environment",
)

var projectTopLevelFields = fieldSet(
	"api_version",
	"kind",
	"id",
	"variables",
	"data",
	"source_manifest",
)

var workflowTopLevelFields = fieldSet(
	"api_version",
	"kind",
	"id",
	"variables",
	"data",
	"steps",
	"source_manifest",
)

var submissionOverridesTopLevelFields = fieldSet(
	"api_version",
	"kind",
	"id",
	"overrides",
	"data",
)

func fieldSet(names ...string) map[string]struct{} {
	fields := make(map[string]struct{}, len(names))
	for _, name := range names {
		fields[name] = struct{}{}
	}
	return fields
}
