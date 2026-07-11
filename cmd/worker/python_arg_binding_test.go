package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"goetl/internal/model"
)

func TestResolvePythonArgvBindingsInterpolatesDataAndArtifactTokens(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	manifest := filepath.Join(root, "data-assets.json")
	writeManifest := strings.TrimSpace(`
	{
	  "schema": "goet/materialized-data-assets/v1",
	  "assets": [
	    {
	      "binding_name": "cropland_year",
	      "provider_type": "local_file",
	      "kind": "fixture",
	      "local_path": "/data/input/cdl.tif"
	    },
	    {
	      "binding_name": "field_tile",
	      "provider_type": "local_file",
	      "kind": "fixture",
	      "local_path": "/data/input/field.tif"
	    }
	  ]
	}`)
	if err := os.WriteFile(manifest, []byte(writeManifest), 0644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	args := []string{
		"--cdl",
		"${data.cropland_year.path[0]}",
		"--tile",
		"${data.field_tile.path[0]}",
		"--out",
		"${artifact_dir}/field_cdl_composition.csv",
		"mixed-${artifact_dir}/prefix",
	}

	artifactDir := filepath.Join(root, "artifacts")
	got, err := resolvePythonArgvBindings(args, manifest, artifactDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := []string{
		"--cdl",
		"/data/input/cdl.tif",
		"--tile",
		"/data/input/field.tif",
		"--out",
		artifactDir + "/field_cdl_composition.csv",
		"mixed-" + artifactDir + "/prefix",
	}
	if len(got) != len(want) {
		t.Fatalf("argument count = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("argument %d = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestResolvePythonArgvBindingsRejectsUnknownDataBinding(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	manifest := filepath.Join(root, "data-assets.json")
	if err := os.WriteFile(manifest, []byte(`{
  "schema": "goet/materialized-data-assets/v1",
  "assets": [
    {
      "binding_name": "cropland_year",
      "provider_type": "local_file",
      "kind": "fixture",
      "local_path": "/data/input/cdl.tif"
    }
  ]
}`), 0644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	_, err := resolvePythonArgvBindings([]string{"${data.field_tile.path[0]}"}, manifest, filepath.Join(root, "artifacts"))
	if err == nil || !strings.Contains(err.Error(), "field_tile") {
		t.Fatalf("expected unknown binding error, got %v", err)
	}
}

func TestResolvePythonArgvBindingsRejectsUnsupportedProperty(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	manifest := filepath.Join(root, "data-assets.json")
	if err := os.WriteFile(manifest, []byte(`{
  "schema": "goet/materialized-data-assets/v1",
  "assets": [
    {
      "binding_name": "cropland_year",
      "provider_type": "local_file",
      "kind": "fixture",
      "local_path": "/data/input/cdl.tif"
    }
  ]
}`), 0644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	_, err := resolvePythonArgvBindings([]string{"${data.cropland_year.sha256}"}, manifest, filepath.Join(root, "artifacts"))
	if err == nil || !strings.Contains(err.Error(), "object field not found") {
		t.Fatalf("expected unsupported property error, got %v", err)
	}
}

func TestResolvePythonArgvBindingsRejectsBareDataAlias(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	manifest := filepath.Join(root, "data-assets.json")
	if err := os.WriteFile(manifest, []byte(`{
  "schema": "goet/materialized-data-assets/v1",
  "assets": [
    {
      "binding_name": "field_segments",
      "provider_type": "local_file",
      "kind": "fixture",
      "local_path": "/data/input/field.tif"
    }
  ]
}`), 0644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	_, err := resolvePythonArgvBindings([]string{"${field_segments}"}, manifest, filepath.Join(root, "artifacts"))
	if err == nil || !strings.Contains(err.Error(), `unsupported argument token "field_segments"`) {
		t.Fatalf("expected bare alias rejection, got %v", err)
	}
}

func TestResolvePythonArgvBindingsInterpolatesNamedFileRolePath(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	manifest := filepath.Join(root, "data-assets.json")
	writeManifest := strings.TrimSpace(`
	{
	  "schema": "goet/materialized-data-assets/v1",
	  "assets": [
	    {
	      "binding_name": "field_segments",
	      "provider_type": "local_file",
	      "kind": "fixture",
	      "local_path": "/data/input",
	      "archive_members": [
	        {
	          "member": "header",
	          "local_path": "/data/input/field_segments.hdr"
	        }
	      ]
	    }
	  ]
	}`)
	if err := os.WriteFile(manifest, []byte(writeManifest), 0644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	got, err := resolvePythonArgvBindings([]string{"${data.field_segments.files.header.path}"}, manifest, filepath.Join(root, "artifacts"))
	if err != nil {
		t.Fatalf("resolvePythonArgvBindings() error = %v", err)
	}
	if len(got) != 1 || got[0] != "/data/input/field_segments.hdr" {
		t.Fatalf("args = %+v, want header path", got)
	}
}

func TestResolvePythonArgvBindingsRejectsMalformedTokens(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	manifest := filepath.Join(root, "data-assets.json")
	if err := os.WriteFile(manifest, []byte(`{"schema":"goet/materialized-data-assets/v1","assets":[]}`), 0644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	tests := []string{
		"${data.cropland_year.path[0]",
		"${artifact_dir",
		"prefix${artifact_dir}suffix}",
	}
	for _, arg := range tests {
		t.Run(arg, func(t *testing.T) {
			_, err := resolvePythonArgvBindings([]string{arg}, manifest, filepath.Join(root, "artifacts"))
			if err == nil {
				t.Fatalf("expected error for %q", arg)
			}
		})
	}
}

func TestWorkerRunWorkItemBindsPythonArgumentTokens(t *testing.T) {
	requirePython3(t)

	worker := newPythonTestWorker(t)
	dataRoot := t.TempDir()
	worker.Config.DataLocationRoots = map[string]string{"fixture": dataRoot}
	writeFixture(t, dataRoot, "input-cdl.tif", "cdl bytes")
	writeFixture(t, dataRoot, "input-tile.tif", "field bytes")

	server := newPythonSourceServer(t, map[string]string{
		"scripts/run.py": strings.TrimSpace(`
import json
import os
import sys

output = {"argv": sys.argv[1:]}
with open(os.environ["GOET_OUTPUT_JSON"], "w", encoding="utf-8") as handle:
    json.dump(output, handle, indent=2)
`),
	})
	t.Cleanup(server.Close)
	worker.Config.ControllerURL = server.URL

	croplandAsset := localFileAsset("fixture", "input-cdl.tif", model.DataAssetCacheStrategyReference, "", nil)
	croplandAsset.BindingName = "cropland_year"
	fieldAsset := localFileAsset("fixture", "input-tile.tif", model.DataAssetCacheStrategyReference, "", nil)
	fieldAsset.BindingName = "field_tile"

	item := pythonTestItem("python-args-001", "attempt-args-001", model.Parameters{
		"python_entrypoint": {Type: "path", Value: "scripts/run.py"},
		"data_assets": {
			Type: "data_assets",
			Value: []model.BoundDataAsset{
				croplandAsset,
				fieldAsset,
			},
		},
		"python_args": {
			Type: "list",
			Value: []any{
				"--cdl", "${data.cropland_year.path[0]}",
				"--tile", "${data.field_tile.path[0]}",
				"--out", "${artifact_dir}/field_cdl_composition.csv",
			},
		},
	})

	if _, err := worker.Run(item); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := filepath.Join(worker.Config.DataDir, item.OutputFilename)
	data, err := os.ReadFile(output)
	if err != nil {
		t.Fatalf("read result: %v", err)
	}
	payload := struct {
		Argv []string `json:"argv"`
	}{}
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("decode result: %v", err)
	}

	expectedArtifact := filepath.Join(worker.Config.TmpDir, "attempts", item.AttemptID, "artifacts", "field_cdl_composition.csv")
	wantArgv := []string{
		"--cdl", filepath.Join(dataRoot, "input-cdl.tif"),
		"--tile", filepath.Join(dataRoot, "input-tile.tif"),
		"--out", expectedArtifact,
	}
	if len(payload.Argv) != len(wantArgv) {
		t.Fatalf("argv count = %d, want %d", len(payload.Argv), len(wantArgv))
	}
	for i := range wantArgv {
		if payload.Argv[i] != wantArgv[i] {
			t.Fatalf("argv[%d] = %q, want %q", i, payload.Argv[i], wantArgv[i])
		}
	}
}

func TestWorkerRunWorkItemRejectsMissingDataBindingBeforePythonStarts(t *testing.T) {
	requirePython3(t)

	worker := newPythonTestWorker(t)
	dataRoot := t.TempDir()
	worker.Config.DataLocationRoots = map[string]string{"fixture": dataRoot}
	writeFixture(t, dataRoot, "input-cdl.tif", "cdl bytes")

	marker := filepath.Join(worker.Config.TmpDir, "attempts", "attempt-missing-binding-001", "work", "ran.txt")
	server := newPythonSourceServer(t, map[string]string{
		"scripts/run.py": strings.TrimSpace(`
import pathlib
pathlib.Path("` + strings.ReplaceAll(filepath.ToSlash(marker), "\\", "\\\\") + `").write_text("ran", encoding="utf-8")
with open(__import__("os").environ["GOET_OUTPUT_JSON"], "w", encoding="utf-8") as handle:
    handle.write("{}")
`),
	})
	t.Cleanup(server.Close)
	worker.Config.ControllerURL = server.URL

	item := pythonTestItem("python-args-fail-001", "attempt-missing-binding-001", model.Parameters{
		"python_entrypoint": {Type: "path", Value: "scripts/run.py"},
		"data_assets": {
			Type: "data_assets",
			Value: []model.BoundDataAsset{{
				BindingName:  "cropland_year",
				ProviderName: "fixture_provider",
				Kind:         "fixture",
				Format:       "txt",
				Provider:     model.DataProviderLocalFile,
				Location: model.DataAssetLocation{
					Type:         model.DataProviderLocalFile,
					LocationName: "fixture",
					Path:         "input-cdl.tif",
				},
			}},
		},
		"python_args": {
			Type:  "list",
			Value: []any{"--cdl", "${data.missing.path[0]}"},
		},
	})

	if _, err := worker.Run(item); err == nil || !strings.Contains(err.Error(), "data binding \"missing\"") {
		t.Fatalf("expected missing binding error, got %v", err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("python should not have started, marker exists: %v", err)
	}
}
