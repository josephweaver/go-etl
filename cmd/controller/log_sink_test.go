package main

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"

	"goetl/internal/model"
)

func TestFilesystemLogSinkWritesControllerLog(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	sink, err := newFilesystemLogSink(root, string(model.LogLevelDebug))
	if err != nil {
		t.Fatalf("newFilesystemLogSink() error = %v", err)
	}

	if err := sink.Write(model.LogObservation{
		Component: "controller",
		Level:     model.LogLevelInfo,
		Timestamp: "2026-07-05T12:00:00Z",
		Message:   "controller boot",
	}); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	path := filepath.Join(root, "controller", "controller.jsonl")
	lines := readJSONLLines(t, path)
	if len(lines) != 1 {
		t.Fatalf("line count = %d, want %d", len(lines), 1)
	}
}

func TestFilesystemLogSinkWritesSubmissionLog(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	sink, err := newFilesystemLogSink(root, string(model.LogLevelDebug))
	if err != nil {
		t.Fatalf("newFilesystemLogSink() error = %v", err)
	}

	if err := sink.Write(model.LogObservation{
		SubmissionID: "sub-1",
		Component:    "worker",
		Level:        model.LogLevelInfo,
		Timestamp:    "2026-07-05T12:00:00Z",
		Message:      "submission update",
	}); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	path := filepath.Join(root, "submissions", "sub-1", "submission.jsonl")
	lines := readJSONLLines(t, path)
	if len(lines) != 1 {
		t.Fatalf("line count = %d, want %d", len(lines), 1)
	}
}

func TestFilesystemLogSinkWritesAttemptLog(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	sink, err := newFilesystemLogSink(root, string(model.LogLevelDebug))
	if err != nil {
		t.Fatalf("newFilesystemLogSink() error = %v", err)
	}

	if err := sink.Write(model.LogObservation{
		SubmissionID: "sub-2",
		AttemptID:    "attempt-1",
		Component:    "worker",
		Level:        model.LogLevelInfo,
		Timestamp:    "2026-07-05T12:00:00Z",
		Message:      "attempt log",
	}); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	path := filepath.Join(root, "submissions", "sub-2", "attempts", "attempt-1.jsonl")
	lines := readJSONLLines(t, path)
	if len(lines) != 1 {
		t.Fatalf("line count = %d, want %d", len(lines), 1)
	}

	submissionPath := filepath.Join(root, "submissions", "sub-2", "submission.jsonl")
	if _, err := os.Stat(submissionPath); !os.IsNotExist(err) {
		t.Fatalf("submission duplicate file exists unexpectedly at %s", submissionPath)
	}
}

func TestFilesystemLogSinkRejectsUnsafeSubmissionID(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	sink, err := newFilesystemLogSink(root, string(model.LogLevelDebug))
	if err != nil {
		t.Fatalf("newFilesystemLogSink() error = %v", err)
	}

	err = sink.Write(model.LogObservation{
		SubmissionID: "sub/../id",
		Component:    "worker",
		Level:        model.LogLevelInfo,
		Timestamp:    "2026-07-05T12:00:00Z",
		Message:      "unsafe id",
	})
	if err == nil {
		t.Fatal("Write() error = nil, want non-nil")
	}
}

func TestFilesystemLogSinkFiltersByLevel(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	sink, err := newFilesystemLogSink(root, string(model.LogLevelWarn))
	if err != nil {
		t.Fatalf("newFilesystemLogSink() error = %v", err)
	}

	if err := sink.Write(model.LogObservation{
		SubmissionID: "sub-4",
		Component:    "worker",
		Level:        model.LogLevelInfo,
		Timestamp:    "2026-07-05T12:00:00Z",
		Message:      "filtered log",
	}); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	submissionPath := filepath.Join(root, "submissions", "sub-4", "submission.jsonl")
	if _, err := os.Stat(submissionPath); !os.IsNotExist(err) {
		t.Fatalf("filtered path exists: %s", submissionPath)
	}
}

func TestFilesystemLogSinkSerializesConcurrentWrites(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	sink, err := newFilesystemLogSink(root, string(model.LogLevelDebug))
	if err != nil {
		t.Fatalf("newFilesystemLogSink() error = %v", err)
	}

	const writes = 40
	var wg sync.WaitGroup
	wg.Add(writes)
	for i := 0; i < writes; i++ {
		go func(i int) {
			defer wg.Done()
			err := sink.Write(model.LogObservation{
				SubmissionID: "sub-5",
				Component:    "worker",
				Level:        model.LogLevelInfo,
				Timestamp:    "2026-07-05T12:00:00Z",
				Message:      "log-" + strconv.Itoa(i),
				Sequence:     uint64(i),
			})
			if err != nil {
				t.Errorf("Write() error = %v", err)
			}
		}(i)
	}
	wg.Wait()

	path := filepath.Join(root, "submissions", "sub-5", "submission.jsonl")
	lines := readJSONLLines(t, path)
	if len(lines) != writes {
		t.Fatalf("line count = %d, want %d", len(lines), writes)
	}
}

func readJSONLLines(t *testing.T, path string) []map[string]any {
	t.Helper()

	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	out := make([]map[string]any, 0)
	for scanner.Scan() {
		var decoded map[string]any
		line := scanner.Bytes()
		if err := json.Unmarshal(line, &decoded); err != nil {
			t.Fatalf("decode json line: %v", err)
		}
		out = append(out, decoded)
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan lines: %v", err)
	}
	return out
}
