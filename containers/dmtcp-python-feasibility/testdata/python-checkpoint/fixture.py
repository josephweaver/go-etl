#!/usr/bin/env python3

import hashlib
import json
import os
from pathlib import Path
import subprocess
import sys
import time


ROOT = Path(os.environ.get("GOETL_PYTHON_TEST_ROOT", "/checkpoint-test"))
RUN = os.environ.get("GOETL_PYTHON_RUN", "run")
MARKERS = ROOT / "markers"
OUTPUT = ROOT / "output"
STATE = ROOT / "state"


def prepare_directories():
    for path in (ROOT, MARKERS, OUTPUT, STATE):
        path.mkdir(mode=0o700, parents=True, exist_ok=True)


def write_json(path, value):
    temporary = path.with_suffix(path.suffix + ".tmp")
    with temporary.open("w", encoding="utf-8") as handle:
        json.dump(value, handle, indent=2, sort_keys=True)
        handle.write("\n")
        handle.flush()
        os.fsync(handle.fileno())
    temporary.replace(path)


def write_marker(name):
    path = MARKERS / f"{RUN}.{name}"
    with path.open("x", encoding="utf-8") as handle:
        handle.write(f"{name}\n")
        handle.flush()
        os.fsync(handle.fileno())


def append_line(handle, value):
    handle.write(f"{value}\n")
    handle.flush()
    os.fsync(handle.fileno())


def wait_for(path, timeout=600):
    deadline = time.monotonic() + timeout
    while not path.exists():
        if time.monotonic() >= deadline:
            raise TimeoutError(f"timed out waiting for {path}")
        time.sleep(0.1)


def run_control():
    progress_path = STATE / f"{RUN}.control-progress.txt"
    with progress_path.open("a", encoding="utf-8") as progress:
        append_line(progress, "before-checkpoint")
        write_marker("control-ready")
        wait_for(STATE / "control.release")
        append_line(progress, "after-resume")
    write_json(
        OUTPUT / f"{RUN}.control.json",
        {
            "phase": "complete",
            "progress": progress_path.read_text(encoding="utf-8").splitlines(),
            "schema": "goetl/dmtcp-python-control/v1",
        },
    )
    write_marker("control-complete")


def run_child():
    progress_path = STATE / f"{RUN}.child-progress.txt"
    with progress_path.open("a", encoding="utf-8") as progress:
        append_line(progress, "child-before-checkpoint")
        write_marker("child-ready")
        wait_for(STATE / f"{RUN}.child.release")
        values = [((index * 17) + 11) % 997 for index in range(20000)]
        digest = hashlib.sha256(
            ",".join(str(value) for value in values).encode("ascii")
        ).hexdigest()
        append_line(progress, "child-after-resume")
    write_json(
        OUTPUT / f"{RUN}.child.json",
        {
            "count": len(values),
            "digest": digest,
            "progress": progress_path.read_text(encoding="utf-8").splitlines(),
            "schema": "goetl/dmtcp-python-child/v1",
        },
    )
    write_marker("child-complete")


def run_parent():
    import numpy

    input_path = Path(os.environ["GOET_INPUT_JSON"])
    output_path = Path(os.environ["GOET_OUTPUT_JSON"])
    with input_path.open(encoding="utf-8") as handle:
        configuration = json.load(handle)

    matrix_size = int(configuration["matrix_size"])
    repetitions = int(configuration["repetitions"])
    seed = int(configuration["seed"])

    child_environment = os.environ.copy()
    child = subprocess.Popen(
        [sys.executable, str(Path(__file__).resolve()), "child"],
        env=child_environment,
        close_fds=True,
    )
    wait_for(MARKERS / f"{RUN}.child-ready")

    random = numpy.random.default_rng(seed)
    left = random.standard_normal((matrix_size, matrix_size), dtype=numpy.float64)
    right = random.standard_normal((matrix_size, matrix_size), dtype=numpy.float64)
    progress_path = STATE / f"{RUN}.parent-progress.txt"
    digests = []

    with progress_path.open("a", encoding="utf-8") as progress:
        append_line(progress, "parent-before-checkpoint")
        write_marker("parent-native-start")
        for repetition in range(repetitions):
            product = left @ right
            digest = hashlib.sha256(product.tobytes(order="C")).hexdigest()
            digests.append(digest)
            append_line(progress, f"native-repetition-{repetition + 1}")
            left, right = right, product / float(matrix_size)

        wait_for(STATE / f"{RUN}.parent.release")
        child_status = child.wait(timeout=300)
        if child_status != 0:
            raise RuntimeError(f"child exited with status {child_status}")
        append_line(progress, "parent-after-resume")

    child_output_path = OUTPUT / f"{RUN}.child.json"
    with child_output_path.open(encoding="utf-8") as handle:
        child_output = json.load(handle)

    write_json(
        output_path,
        {
            "child": child_output,
            "matrix_size": matrix_size,
            "native_digests": digests,
            "numpy_version": numpy.__version__,
            "progress": progress_path.read_text(encoding="utf-8").splitlines(),
            "repetitions": repetitions,
            "schema": "goetl/dmtcp-python-parent/v1",
            "seed": seed,
        },
    )
    write_marker("parent-complete")


def run_compare(expected_run, actual_run):
    expected_parent = json.loads(
        (OUTPUT / f"{expected_run}.parent.json").read_text(encoding="utf-8")
    )
    actual_parent = json.loads(
        (OUTPUT / f"{actual_run}.parent.json").read_text(encoding="utf-8")
    )
    if expected_parent != actual_parent:
        raise RuntimeError("resumed parent/child output differs from baseline")
    result = {
        "actual_run": actual_run,
        "expected_run": expected_run,
        "match": True,
        "native_repetitions": actual_parent["repetitions"],
        "schema": "goetl/dmtcp-python-compare/v1",
    }
    write_json(OUTPUT / "compare.json", result)


def main():
    prepare_directories()
    if len(sys.argv) < 2:
        raise RuntimeError("usage: fixture.py <control|parent|child|compare>")
    mode = sys.argv[1]
    if mode == "control":
        run_control()
    elif mode == "parent":
        run_parent()
    elif mode == "child":
        run_child()
    elif mode == "compare" and len(sys.argv) == 4:
        run_compare(sys.argv[2], sys.argv[3])
    else:
        raise RuntimeError(f"unsupported fixture arguments: {sys.argv[1:]}")


if __name__ == "__main__":
    main()
