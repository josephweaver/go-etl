import json
import os
import sys
import time


def append_durable_marker(path, value):
    os.makedirs(os.path.dirname(path), exist_ok=True)
    with open(path, "a", encoding="utf-8") as handle:
        handle.write(value + "\n")
        handle.flush()
        os.fsync(handle.fileno())


if len(sys.argv) != 4:
    raise RuntimeError("expected continuation, pre-marker, and post-marker paths")

continuation_path, pre_marker_path, post_marker_path = sys.argv[1:]

append_durable_marker(pre_marker_path, "before-checkpoint")

while not os.path.exists(continuation_path):
    time.sleep(0.1)

with open(os.environ["GOET_INPUT_JSON"], "r", encoding="utf-8") as handle:
    input_document = json.load(handle)

with open(os.environ["GOET_OUTPUT_JSON"], "w", encoding="utf-8") as handle:
    json.dump(
        {
            "checkpoint_fixture": "resumed",
            "input_work_item_id": input_document["work_item"]["id"],
            "artifacts": [],
        },
        handle,
    )
    handle.flush()
    os.fsync(handle.fileno())

append_durable_marker(post_marker_path, "after-resume")
