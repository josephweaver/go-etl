#!/usr/bin/env bash
set -euo pipefail

fake_hpcc=0
if [[ "${1:-}" == "--fake-hpcc" ]]; then
  fake_hpcc=1
elif [[ "${1:-}" == "--help" || "${1:-}" == "-h" ]]; then
  cat <<'USAGE'
Usage:
  bash scripts/cdl-yanroy-fixture-smoke.sh
  bash scripts/cdl-yanroy-fixture-smoke.sh --fake-hpcc

The default mode starts a local controller and one local worker. --fake-hpcc
uses the local fake Slurm/sbatch boundary. Neither mode downloads real CDL,
reads real Yan/Roy data, contacts Google Drive, or needs credentials.
USAGE
  exit 0
elif [[ -n "${1:-}" ]]; then
  echo "unknown argument: $1" >&2
  exit 2
fi

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
demo_root="$(cd "$repo_root/.." && pwd)/go-etl-demo-project"
[[ -f "$demo_root/workflows/cdl-yanroy-fixture.json" ]] || {
  echo "sibling CDL/Yan/Roy fixture workflow missing under $demo_root" >&2
  exit 1
}
command -v go >/dev/null || { echo "go is required" >&2; exit 1; }
command -v curl >/dev/null || { echo "curl is required" >&2; exit 1; }
command -v python3 >/dev/null || { echo "python3 is required" >&2; exit 1; }

mode_name="local"
if [[ "$fake_hpcc" == "1" ]]; then
  mode_name="fake-hpcc"
fi
run_root="$repo_root/.run/cdl-yanroy-fixture-$mode_name"
controller_url="http://localhost:8080"
worker_data_root="$run_root/worker-data"
worker_tmp_root="$run_root/worker-tmp"
worker_log_root="$run_root/worker-logs"
asset_cache_root="$run_root/asset-cache"
published_root="$run_root/published-data"
fixture_root="$demo_root/data/fixtures/cdl-yanroy"
controller_config_path="$run_root/controller.json"
worker_config_path="$run_root/worker.json"
submission_path="$run_root/submission.json"

rm -rf "$run_root"
mkdir -p "$worker_data_root" "$worker_tmp_root" "$worker_log_root" "$asset_cache_root" "$published_root"
cp "$repo_root/cmd/controller/defaults.json" "$run_root/defaults.json"

if [[ "$fake_hpcc" == "1" ]]; then
  runtime_root="$run_root/runtime"
  slurm_run_root="$run_root/slurm"
  worker_config_path="$runtime_root/config/worker.json"
  worker_log_root="$runtime_root/logs"
  worker_script_path="$runtime_root/scripts/worker.slurm"
  mkdir -p "$runtime_root" "$slurm_run_root"
fi

python3 - "$controller_config_path" "$run_root/controller.sqlite" "$run_root" "${runtime_root:-}" "$worker_data_root" "$asset_cache_root" "$fixture_root" "$published_root" "$fake_hpcc" <<'PY'
import json
import sys

(
    controller_config_path,
    db_path,
    run_root,
    runtime_root,
    worker_data_root,
    asset_cache_root,
    fixture_root,
    published_root,
    fake_hpcc,
) = sys.argv[1:10]
config = {
    "api_version": "goet/v1alpha1",
    "kind": "Controller",
    "variables": [
        {"name": {"namespace": "controller_config", "key": "controller_url"}, "type": "string", "expression": "http://localhost:8080"},
        {"name": {"namespace": "controller_config", "key": "main_database_driver"}, "type": "string", "expression": "sqlite"},
        {"name": {"namespace": "controller_config", "key": "main_database_connection_string"}, "type": "string", "expression": db_path},
        {"name": {"namespace": "controller_config", "key": "controller_root_dir"}, "type": "path", "expression": run_root},
    ],
}
if fake_hpcc == "1":
    config["execution_environment"] = {
        "name": "cdl-yanroy-fixture-local-slurm",
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
            "python_executable": "python3",
        }},
    }
with open(controller_config_path, "w", encoding="utf-8") as handle:
    json.dump(config, handle, indent=2)
    handle.write("\n")
PY

python3 - "$submission_path" "$fake_hpcc" "${worker_config_path:-}" "${worker_log_root:-}" "${worker_script_path:-}" <<'PY'
import json
import sys

submission_path, fake_hpcc, worker_config_path, worker_log_root, worker_script_path = sys.argv[1:6]
submission = {
    "project": {"repository": "local:demo", "ref": "main", "path": "project.json"},
    "workflow": {"repository": "local:demo", "ref": "main", "path": "workflows/cdl-yanroy-fixture.json"},
    "variables": [],
}
if fake_hpcc == "1":
    submission["variables"] = [
        {"name": {"namespace": "worker_config", "key": "scheduler"}, "type": "object", "expression": {
            "type": {"type": "string", "expression": "slurm"},
            "settings": {"type": "object", "expression": {
                "script_path": {"type": "path", "expression": worker_script_path},
                "job_name": {"type": "string", "expression": "goetl-cdl-yanroy-fixture"},
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
with open(submission_path, "w", encoding="utf-8") as handle:
    json.dump(submission, handle, indent=2)
    handle.write("\n")
PY

if [[ "$fake_hpcc" == "0" ]]; then
  python3 - "$worker_config_path" "$worker_log_root" "$worker_tmp_root" "$worker_data_root" "$asset_cache_root" "$fixture_root" "$published_root" <<'PY'
import json
import sys

path, log_root, tmp_root, data_root, cache_root, fixture_root, published_root = sys.argv[1:8]
config = {
    "log_dir": log_root,
    "tmp_dir": tmp_root,
    "data_dir": data_root,
    "controller_url": "http://localhost:8080",
    "python_executable": "python3",
    "asset_cache_dir": cache_root,
    "data_location_roots": {
        "fixture_data": fixture_root,
        "published_data": published_root,
    },
}
with open(path, "w", encoding="utf-8") as handle:
    json.dump(config, handle, indent=2)
    handle.write("\n")
PY
fi

cleanup() {
  curl -fsS -X POST "$controller_url/shutdown" >/dev/null 2>&1 || true
  if [[ -n "${controller_pid:-}" ]] && kill -0 "$controller_pid" 2>/dev/null; then
    wait "$controller_pid" 2>/dev/null || true
  fi
}
trap cleanup EXIT

if [[ "$fake_hpcc" == "1" ]]; then
  export PATH="$repo_root/scripts/fake-hpcc:$PATH"
  export FAKE_SLURM_RUN_ROOT="$slurm_run_root"
  export FAKE_SLURM_FOREGROUND=1
fi

(
  cd "$repo_root"
  go run ./cmd/controller --config "$controller_config_path"
) > "$run_root/controller.out.log" 2> "$run_root/controller.err.log" &
controller_pid="$!"

for _ in $(seq 1 90); do
  if curl -fsS "$controller_url/status" >/dev/null 2>&1; then
    break
  fi
  sleep 0.5
done
curl -fsS "$controller_url/status" >/dev/null

ack="$(curl -fsS -X POST -H 'Content-Type: application/json' --data-binary "@$submission_path" "$controller_url/workflow")"
submission_id="$(python3 -c 'import json,sys; print(json.load(sys.stdin)["submission_id"])' <<<"$ack")"
export submission_id

if [[ "$fake_hpcc" == "0" ]]; then
  (
    cd "$repo_root"
    go run ./cmd/worker "$worker_config_path"
  ) > "$run_root/worker.out.log" 2> "$run_root/worker.err.log"
fi

for _ in $(seq 1 120); do
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

manifest_path="$worker_data_root/cdl-yanroy-fixture-fixture_tile_001.json"
composition_path="$published_root/field_cdl_composition/year=2023/tile=fixture_tile_001/field_cdl_composition.csv"
dominant_path="$published_root/field_dominant_crop/year=2023/tile=fixture_tile_001/field_dominant_crop.csv"
python3 - "$manifest_path" "$composition_path" "$dominant_path" "$mode_name" <<'PY'
import json
import os
import sys

manifest_path, composition_path, dominant_path, mode_name = sys.argv[1:5]
expected_composition = """field_id,field_tile_id,year,crop_code,crop_type,field_pixel_count,crop_pixel_count,crop_fraction,is_dominant_crop,dominant_crop_code,dominant_crop_type,dominant_crop_fraction,assignment_policy
1,fixture_tile_001,2023,5,corn,5,4,0.8,true,5,corn,0.8,dominant_share_v1
1,fixture_tile_001,2023,1,soybeans,5,1,0.2,false,5,corn,0.8,dominant_share_v1
2,fixture_tile_001,2023,1,soybeans,5,4,0.8,true,1,soybeans,0.8,dominant_share_v1
2,fixture_tile_001,2023,2,wheat,5,1,0.2,false,1,soybeans,0.8,dominant_share_v1
3,fixture_tile_001,2023,2,wheat,5,4,0.8,true,2,wheat,0.8,dominant_share_v1
3,fixture_tile_001,2023,5,corn,5,1,0.2,false,2,wheat,0.8,dominant_share_v1
"""
expected_dominant = """field_id,field_tile_id,year,dominant_crop_code,dominant_crop_type,dominant_crop_fraction,field_pixel_count,assignment_status,assignment_policy
1,fixture_tile_001,2023,5,corn,0.8,5,assigned,dominant_share_v1
2,fixture_tile_001,2023,1,soybeans,0.8,5,assigned,dominant_share_v1
3,fixture_tile_001,2023,2,wheat,0.8,5,assigned,dominant_share_v1
"""
with open(manifest_path, "r", encoding="utf-8") as handle:
    manifest = json.load(handle)
if len(manifest.get("artifacts", [])) != 2:
    raise SystemExit(f"artifact manifest should contain 2 artifacts: {manifest_path}")
if len(manifest.get("published_assets", [])) != 2:
    raise SystemExit(f"artifact manifest should contain 2 published assets: {manifest_path}")
with open(composition_path, "r", encoding="utf-8") as handle:
    actual_composition = handle.read()
if actual_composition != expected_composition:
    raise SystemExit(f"unexpected composition output: {composition_path}\n{actual_composition}")
with open(dominant_path, "r", encoding="utf-8") as handle:
    actual_dominant = handle.read()
if actual_dominant != expected_dominant:
    raise SystemExit(f"unexpected dominant output: {dominant_path}\n{actual_dominant}")
print(json.dumps({
    "mode": mode_name,
    "submission_id": os.environ.get("submission_id", ""),
    "manifest": manifest_path,
    "composition": composition_path,
    "dominant": dominant_path,
}, indent=2))
PY
