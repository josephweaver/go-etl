import json
import os
import sys


required_environment = [
    "GOET_INPUT_JSON",
    "GOET_OUTPUT_JSON",
    "GOET_WORK_ITEM_ID",
    "GOET_ATTEMPT_ID",
    "GOET_SOURCE_DIR",
    "GOET_WORK_DIR",
    "GOET_ARTIFACT_DIR",
    "GOET_DATA_DIR",
    "GOET_TMP_DIR",
    "GOET_LOG_DIR",
]
missing = [name for name in required_environment if not os.environ.get(name)]
if missing:
    raise RuntimeError("missing worker environment: " + ", ".join(missing))

with open(os.environ["GOET_INPUT_JSON"], "r", encoding="utf-8") as handle:
    input_document = json.load(handle)

argument = sys.argv[1] if len(sys.argv) > 1 else ""
print("direct fixture stdout: " + argument)
print("direct fixture stderr: " + argument, file=sys.stderr)

if argument == "fail":
    raise SystemExit(7)

artifact_path = os.path.join(os.environ["GOET_ARTIFACT_DIR"], "reports", "fixture.txt")
os.makedirs(os.path.dirname(artifact_path), exist_ok=True)
with open(artifact_path, "w", encoding="utf-8") as handle:
    handle.write("direct fixture artifact\n")

with open(os.environ["GOET_OUTPUT_JSON"], "w", encoding="utf-8") as handle:
    json.dump(
        {
            "argument": argument,
            "input_work_item_id": input_document["work_item"]["id"],
            "environment_present": sorted(required_environment),
            "artifacts": [
                {
                    "name": "fixture_report",
                    "kind": "file",
                    "format": "txt",
                    "path": "reports/fixture.txt",
                }
            ],
        },
        handle,
    )
