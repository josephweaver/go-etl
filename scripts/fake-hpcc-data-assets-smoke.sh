#!/usr/bin/env bash
set -euo pipefail

if [[ "${1:-}" == "--help" || "${1:-}" == "-h" ]]; then
  cat <<'USAGE'
Usage:
  bash scripts/fake-hpcc-data-assets-smoke.sh

Requires go, bash, curl, and python3. The script creates temporary smoke
fixtures under ../go-etl-demo-project/.goetl-smoke and runtime state under
.run/fake-hpcc-data-assets.
USAGE
  exit 0
fi

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
demo_root="$(cd "$repo_root/.." && pwd)/go-etl-demo-project"
if [[ ! -f "$demo_root/project.json" ]]; then
  echo "sibling demo project missing: $demo_root" >&2
  exit 1
fi

command -v go >/dev/null || { echo "go is required" >&2; exit 1; }
command -v curl >/dev/null || { echo "curl is required" >&2; exit 1; }
command -v python3 >/dev/null || { echo "python3 is required" >&2; exit 1; }

run_root="$repo_root/.run/fake-hpcc-data-assets"
runtime_root="$run_root/runtime"
worker_data_root="$run_root/worker-data"
fixture_root="$run_root/fixture-data"
published_root="$run_root/published-data"
slurm_run_root="$run_root/slurm"
source_root="$demo_root/.goetl-smoke/fake-hpcc-data-assets"
source_script_dir="$source_root/scripts"
controller_url="http://localhost:8080"

rm -rf "$run_root"
mkdir -p "$runtime_root" "$worker_data_root" "$fixture_root" "$published_root" "$slurm_run_root" "$source_script_dir"
cp "$repo_root/cmd/controller/defaults.json" "$run_root/defaults.json"

printf 'smoke input\n' > "$fixture_root/input.txt"
python3 - "$fixture_root/archive.zip" <<'PY'
import sys
import zipfile

with zipfile.ZipFile(sys.argv[1], "w") as archive:
    archive.writestr("selected-note.txt", "archive note\n")
PY

cat > "$source_script_dir/fake_hpcc_data_assets.py" <<'PY'
import argparse
import json
import os

parser = argparse.ArgumentParser()
parser.add_argument("--input", required=True)
parser.add_argument("--archive", required=True)
parser.add_argument("--out", required=True)
args = parser.parse_args()

with open(args.input, "r", encoding="utf-8") as handle:
    input_text = handle.read().strip()
with open(args.archive, "r", encoding="utf-8") as handle:
    archive_text = handle.read().strip()

os.makedirs(os.path.dirname(args.out), exist_ok=True)
with open(args.out, "w", encoding="utf-8") as handle:
    handle.write("source,content\n")
    handle.write(f"input,{input_text}\n")
    handle.write(f"archive,{archive_text}\n")

with open(os.environ["GOET_OUTPUT_JSON"], "w", encoding="utf-8") as handle:
    json.dump({
        "result": "ok",
        "artifacts": [{
            "name": "summary",
            "kind": "file",
            "format": "csv",
            "path": "reports/summary.csv"
        }]
    }, handle)
PY

workflow_path="$source_root/workflow.json"
controller_config_path="$run_root/controller.json"
submission_path="$run_root/submission.json"
worker_config_path="$runtime_root/config/worker.json"
worker_log_root="$runtime_root/logs"
worker_script_path="$runtime_root/scripts/worker.slurm"
asset_cache_root="$runtime_root/cache/assets"

python3 - "$workflow_path" "$worker_config_path" "$worker_log_root" "$worker_script_path" <<'PY'
import json
import sys

workflow_path, worker_config_path, worker_log_root, worker_script_path = sys.argv[1:5]
worker_variables = [
    {"name": {"namespace": "worker_config", "key": "scheduler"}, "type": "object", "expression": {
        "type": {"type": "string", "expression": "slurm"},
        "settings": {"type": "object", "expression": {
            "script_path": {"type": "path", "expression": worker_script_path},
            "job_name": {"type": "string", "expression": "goetl-worker"},
        }},
    }},
    {"name": {"namespace": "worker_config", "key": "runtime"}, "type": "object", "expression": {
        "type": {"type": "string", "expression": "worker"},
        "settings": {"type": "object", "expression": {
            "executable": {"type": "string", "expression": "go"},
            "args": {"type": "list", "expression": [
                {"type": "string", "expression": "run"},
                {"type": "string", "expression": "./cmd/worker"},
            ]},
            "config_path": {"type": "path", "expression": worker_config_path},
            "log_dir": {"type": "path", "expression": worker_log_root},
        }},
    }},
    {"name": {"namespace": "worker_config", "key": "worker_min_count"}, "type": "int", "expression": 1},
    {"name": {"namespace": "worker_config", "key": "worker_max_count"}, "type": "int", "expression": 1},
    {"name": {"namespace": "worker_config", "key": "worker_count_per_start"}, "type": "int", "expression": 1},
    {"name": {"namespace": "worker_config", "key": "worker_min_elapsed_time_between_starts"}, "type": "string", "expression": "0s"},
]
workflow = {
    "workflow": {
        "ID": "fake-hpcc-data-assets-smoke",
        "Variables": [
            {"name": {"namespace": "workflow", "key": "smoke_items"}, "type": "list", "expression": [
                {"type": "object", "expression": {"id": {"type": "string", "expression": "smoke"}}}
            ]}
        ],
        "Steps": [{
            "ID": "fake-hpcc-data-assets",
            "FanOut": {"WorkItem": {
                "FanOutExpression": "${smoke_items[*]}",
                "IDTokenAccessor": ".id",
                "OutputAccessor": ".id",
                "Type": "python_script",
                "OutputPrefix": "fake-hpcc-data-assets",
                "OutputExtension": ".json",
                "Parameters": {
                    "python_entrypoint": {"type": "path", "value": ".goetl-smoke/fake-hpcc-data-assets/scripts/fake_hpcc_data_assets.py"},
                    "python_args": {"type": "list", "value": ["--input", "${data.input_data.local_path}", "--archive", "${data.archived_note.local_path}", "--out", "${artifact_dir}/reports/summary.csv"]},
                    "data_assets": {"type": "data_assets", "value": [
                        {
                            "binding_name": "input_data",
                            "provider_name": "fixture_input",
                            "kind": "text",
                            "format": "txt",
                            "provider": "local_file",
                            "location": {"type": "local_file", "location_name": "fixture_data", "path": "input.txt"},
                            "materialization": {"strategy": "reference"},
                        },
                        {
                            "binding_name": "archived_note",
                            "provider_name": "fixture_archive",
                            "kind": "text_archive",
                            "format": "zip",
                            "provider": "local_file",
                            "location": {"type": "local_file", "location_name": "fixture_data", "path": "archive.zip"},
                            "cache": {"strategy": "worker_cache", "cache_key": "fake-hpcc-data-assets/archive.zip"},
                            "archive": {"type": "zip", "select": [{"member": "selected-note.txt", "as": "note.txt", "required": True}], "expose": "selected_path"},
                            "materialization": {"strategy": "worker_cache"},
                        },
                    ]},
                    "publish": {"type": "publish_targets", "value": {
                        "publish_summary": {
                            "from_artifact": "summary",
                            "location": {"type": "registered_location", "location_name": "published_data", "path": "reports/summary.csv"},
                            "overwrite_policy": "fail_if_exists",
                        }
                    }},
                },
            }},
        }],
    },
    "source_manifest": {"files": [
        {"role": "python_entrypoint", "path": ".goetl-smoke/fake-hpcc-data-assets/scripts/fake_hpcc_data_assets.py", "content_type": "text/x-python"},
    ]},
    "variables": worker_variables,
}
with open(workflow_path, "w", encoding="utf-8") as handle:
    json.dump(workflow, handle, indent=2)
    handle.write("\n")
PY

python3 - "$controller_config_path" "$run_root/controller.sqlite" "$runtime_root" "$worker_data_root" "$asset_cache_root" "$fixture_root" "$published_root" <<'PY'
import json
import sys

controller_config_path, db_path, runtime_root, worker_data_root, asset_cache_root, fixture_root, published_root = sys.argv[1:8]
config = {
    "api_version": "goet/v1alpha1",
    "kind": "Controller",
    "variables": [
        {"name": {"namespace": "controller_config", "key": "controller_url"}, "type": "string", "expression": "http://localhost:8080"},
        {"name": {"namespace": "controller_config", "key": "main_database_driver"}, "type": "string", "expression": "sqlite"},
        {"name": {"namespace": "controller_config", "key": "main_database_connection_string"}, "type": "string", "expression": db_path},
    ],
    "execution_environment": {
        "name": "fake-hpcc-data-assets-local-slurm",
        "transports": [{"name": "local", "type": "local"}],
        "dialect": {"type": "bash"},
        "scheduler": {"type": "slurm"},
        "runtime": {"type": "worker", "settings": {
            "root": runtime_root,
            "controller_url": "http://localhost:8080",
            "data_dir": worker_data_root,
            "asset_cache_dir": asset_cache_root,
            "data_location_roots": {
                "fixture_data": fixture_root,
                "published_data": published_root,
            },
        }},
    },
}
with open(controller_config_path, "w", encoding="utf-8") as handle:
    json.dump(config, handle, indent=2)
    handle.write("\n")
PY

cat > "$submission_path" <<'JSON'
{
  "project": {"repository": "local:demo", "ref": "main", "path": "project.json"},
  "workflow": {"repository": "local:demo", "ref": "main", "path": ".goetl-smoke/fake-hpcc-data-assets/workflow.json"},
  "variables": []
}
JSON

cleanup() {
  curl -fsS -X POST "$controller_url/shutdown" >/dev/null 2>&1 || true
  if [[ -n "${controller_pid:-}" ]] && kill -0 "$controller_pid" 2>/dev/null; then
    wait "$controller_pid" 2>/dev/null || true
  fi
}
trap cleanup EXIT

export PATH="$repo_root/scripts/fake-hpcc:$PATH"
export FAKE_SLURM_RUN_ROOT="$slurm_run_root"
export FAKE_SLURM_FOREGROUND=1

(
  cd "$repo_root"
  go run ./cmd/controller --config "$controller_config_path"
) > "$run_root/controller.out.log" 2> "$run_root/controller.err.log" &
controller_pid="$!"

for _ in $(seq 1 60); do
  if curl -fsS "$controller_url/status" >/dev/null 2>&1; then
    break
  fi
  sleep 0.5
done
curl -fsS "$controller_url/status" >/dev/null

ack="$(curl -fsS -X POST -H 'Content-Type: application/json' --data-binary "@$submission_path" "$controller_url/workflow")"
submission_id="$(python3 -c 'import json,sys; print(json.load(sys.stdin)["submission_id"])' <<<"$ack")"
export submission_id

for _ in $(seq 1 90); do
  status_json="$(curl -fsS "$controller_url/submissions/$submission_id/status")"
  status="$(python3 -c 'import json,sys; print(json.load(sys.stdin)["status"])' <<<"$status_json")"
  if [[ "$status" == "completed" ]]; then
    break
  fi
  if [[ "$status" == "failed" ]]; then
    echo "submission failed: $submission_id" >&2
    exit 1
  fi
  sleep 1
done
[[ "${status:-}" == "completed" ]]

manifest_path="$worker_data_root/fake-hpcc-data-assets-smoke.json"
published_path="$published_root/reports/summary.csv"
python3 - "$manifest_path" "$worker_data_root" "$published_path" <<'PY'
import json
import os
import sys

manifest_path, worker_data_root, published_path = sys.argv[1:4]
with open(manifest_path, "r", encoding="utf-8") as handle:
    manifest = json.load(handle)
if not manifest.get("artifacts"):
    raise SystemExit(f"artifact manifest missing artifacts: {manifest_path}")
if not manifest.get("published_assets"):
    raise SystemExit(f"artifact manifest missing published assets: {manifest_path}")
promoted = os.path.join(worker_data_root, *manifest["artifacts"][0]["path"].split("/"))
if not os.path.isfile(promoted):
    raise SystemExit(f"promoted artifact missing: {promoted}")
if not os.path.isfile(published_path):
    raise SystemExit(f"published artifact missing: {published_path}")
print(json.dumps({
    "submission_id": os.environ.get("submission_id", ""),
    "manifest": manifest_path,
    "promoted_artifact": promoted,
    "published_artifact": published_path,
    "controller_stdout": os.path.join(os.path.dirname(manifest_path), "..", "controller.out.log"),
    "controller_stderr": os.path.join(os.path.dirname(manifest_path), "..", "controller.err.log"),
}, indent=2))
PY
