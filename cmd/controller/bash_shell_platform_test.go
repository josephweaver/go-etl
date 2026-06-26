package main

import "testing"

func TestBashShellPlatformNewline(t *testing.T) {
	if got := (BashShellPlatform{}).Newline(); got != "\n" {
		t.Fatalf("newline = %q, want LF", got)
	}
}

func TestBashShellPlatformQuoteArg(t *testing.T) {
	got := (BashShellPlatform{}).QuoteArg("worker's config.json")
	want := "'worker'\"'\"'s config.json'"
	if got != want {
		t.Fatalf("quoted arg = %q, want %q", got, want)
	}
}

func TestBashShellPlatformLocalizePath(t *testing.T) {
	got, err := (BashShellPlatform{}).LocalizePath("/data/goetl/config/worker.json")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "/data/goetl/config/worker.json" {
		t.Fatalf("path = %q, want unchanged Linux path", got)
	}
}

func TestBashShellPlatformLocalizePathRejectsNewline(t *testing.T) {
	if _, err := (BashShellPlatform{}).LocalizePath("/data/goetl\n/config"); err == nil {
		t.Fatal("expected an error")
	}
}

func TestBashShellPlatformCopyCommand(t *testing.T) {
	got, err := (BashShellPlatform{}).CopyCommand("/tmp/worker script.slurm", "/data/goetl/scripts/worker's.slurm")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "cp '/tmp/worker script.slurm' '/data/goetl/scripts/worker'\"'\"'s.slurm'"
	if got != want {
		t.Fatalf("copy command = %q, want %q", got, want)
	}
}

func TestBashShellPlatformRemoveFileCommand(t *testing.T) {
	got, err := (BashShellPlatform{}).RemoveFileCommand("/tmp/goetl-worker.slurm")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "rm -f '/tmp/goetl-worker.slurm'"
	if got != want {
		t.Fatalf("remove command = %q, want %q", got, want)
	}
}

func TestBashShellPlatformMakeDirectoryCommand(t *testing.T) {
	got, err := (BashShellPlatform{}).MakeDirectoryCommand("/data/goetl/runtime dir")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "mkdir -p '/data/goetl/runtime dir'"
	if got != want {
		t.Fatalf("mkdir command = %q, want %q", got, want)
	}
}

func TestBashShellPlatformMoveCommand(t *testing.T) {
	got, err := (BashShellPlatform{}).MoveCommand("/tmp/worker temp", "/data/goetl/worker final")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "mv '/tmp/worker temp' '/data/goetl/worker final'"
	if got != want {
		t.Fatalf("move command = %q, want %q", got, want)
	}
}

func TestBashShellPlatformRemoveTreeCommandRejectsRoot(t *testing.T) {
	if _, err := (BashShellPlatform{}).RemoveTreeCommand("/"); err == nil {
		t.Fatal("expected root recursive remove to be rejected")
	}
}

func TestBashShellPlatformRemoveTreeCommand(t *testing.T) {
	got, err := (BashShellPlatform{}).RemoveTreeCommand("/data/goetl/tmp dir")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "rm -rf '/data/goetl/tmp dir'"
	if got != want {
		t.Fatalf("remove tree command = %q, want %q", got, want)
	}
}

func TestBashShellPlatformChmodCommand(t *testing.T) {
	got, err := (BashShellPlatform{}).ChmodCommand("0640", "/data/goetl/config file")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "chmod '0640' '/data/goetl/config file'"
	if got != want {
		t.Fatalf("chmod command = %q, want %q", got, want)
	}
}

func TestBashShellPlatformChownCommand(t *testing.T) {
	got, err := (BashShellPlatform{}).ChownCommand("worker:etl", "/data/goetl/config file")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "chown 'worker:etl' '/data/goetl/config file'"
	if got != want {
		t.Fatalf("chown command = %q, want %q", got, want)
	}
}
