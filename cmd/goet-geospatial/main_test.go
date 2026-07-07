package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRunWritesDeterministicValidationResult(t *testing.T) {
	dir := t.TempDir()
	requestPath := filepath.Join(dir, "request.json")
	responsePath := filepath.Join(dir, "result.json")
	requestJSON := `{
  "api_version": "goet.geospatial/v1alpha1",
  "kind": "GeospatialOperationRequest",
  "operation": "validate",
  "inputs": {
    "field_raster": {
      "path": "/worker/cache/yanroy/tile_001/fields.tif",
      "band": 1,
      "nodata": 0
    }
  },
  "outputs": {
    "metadata_json": "metadata/result.json"
  },
  "options": {}
}
`
	if err := os.WriteFile(requestPath, []byte(requestJSON), 0644); err != nil {
		t.Fatalf("write request: %v", err)
	}

	if err := run([]string{"--request", requestPath, "--response", responsePath}); err != nil {
		t.Fatalf("run() error = %v", err)
	}

	got, err := os.ReadFile(responsePath)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	want := `{
  "api_version": "goet.geospatial/v1alpha1",
  "kind": "GeospatialOperationResult",
  "operation": "validate",
  "artifacts": [],
  "summary": {},
  "warnings": []
}
`
	if string(got) != want {
		t.Fatalf("response JSON = %s, want %s", got, want)
	}
}

func TestRunRejectsInvalidRequest(t *testing.T) {
	dir := t.TempDir()
	requestPath := filepath.Join(dir, "request.json")
	responsePath := filepath.Join(dir, "result.json")
	requestJSON := `{
  "api_version": "goet.geospatial/v1alpha1",
  "kind": "GeospatialOperationRequest",
  "operation": "validate",
  "inputs": {},
  "outputs": {
    "bad": "../escape.json"
  },
  "options": {}
}
`
	if err := os.WriteFile(requestPath, []byte(requestJSON), 0644); err != nil {
		t.Fatalf("write request: %v", err)
	}

	if err := run([]string{"--request", requestPath, "--response", responsePath}); err == nil {
		t.Fatal("expected an error")
	}
	if _, err := os.Stat(responsePath); !os.IsNotExist(err) {
		t.Fatalf("response file exists after invalid request, stat error = %v", err)
	}
}
