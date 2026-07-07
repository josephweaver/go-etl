package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"goetl/internal/model"
)

func TestPromoteArtifactsPromotesFileArtifact(t *testing.T) {
	staging := t.TempDir()
	dataRoot := t.TempDir()
	writeFixture(t, staging, "reports/output.csv", "id,value\n1,a\n")

	manifest, err := PromoteArtifacts(context.Background(), ArtifactPromotionRequest{
		StagingRoot: staging,
		DataRoot:    dataRoot,
		WorkItemID:  "work-file-001",
		AttemptID:   "attempt-file-001",
		Manifest: model.ArtifactManifest{
			Artifacts: []model.ArtifactDescriptor{
				{Name: "report", Kind: model.ArtifactKindFile, Format: "csv", Path: "reports/output.csv"},
			},
		},
	})
	if err != nil {
		t.Fatalf("PromoteArtifacts() error = %v", err)
	}

	artifact := manifest.Artifacts[0]
	if manifest.StorageScope != artifactStorageScopeWorkerDataDir {
		t.Fatalf("storage scope = %q", manifest.StorageScope)
	}
	if artifact.Path != "artifacts/raw/work-file-001/reports/output.csv" {
		t.Fatalf("artifact path = %q", artifact.Path)
	}
	if artifact.SizeBytes == nil || *artifact.SizeBytes != int64(len("id,value\n1,a\n")) {
		t.Fatalf("size bytes = %v", artifact.SizeBytes)
	}
	if artifact.SHA256 != sha256Text("id,value\n1,a\n") {
		t.Fatalf("sha256 = %q", artifact.SHA256)
	}
	if got := readString(t, filepath.Join(dataRoot, filepath.FromSlash(artifact.Path))); got != "id,value\n1,a\n" {
		t.Fatalf("promoted file = %q", got)
	}
}

func TestPromoteArtifactsPromotesDirectoryArtifact(t *testing.T) {
	staging := t.TempDir()
	dataRoot := t.TempDir()
	writeFixture(t, staging, "dataset/part-b.txt", "b")
	writeFixture(t, staging, "dataset/nested/part-a.txt", "aa")

	manifest, err := PromoteArtifacts(context.Background(), ArtifactPromotionRequest{
		StagingRoot: staging,
		DataRoot:    dataRoot,
		WorkItemID:  "work-dir-001",
		Manifest: model.ArtifactManifest{
			Artifacts: []model.ArtifactDescriptor{
				{Name: "dataset", Kind: model.ArtifactKindDirectory, Path: "dataset"},
			},
		},
	})
	if err != nil {
		t.Fatalf("PromoteArtifacts() error = %v", err)
	}

	artifact := manifest.Artifacts[0]
	promotedPath := filepath.Join(dataRoot, filepath.FromSlash(artifact.Path))
	evidence, err := directoryManifestEvidence(promotedPath)
	if err != nil {
		t.Fatalf("directory evidence: %v", err)
	}
	if artifact.ManifestSHA256 != evidence.sha256 {
		t.Fatalf("manifest sha256 = %q, want %q", artifact.ManifestSHA256, evidence.sha256)
	}
	if artifact.SizeBytes == nil || *artifact.SizeBytes != int64(len("b")+len("aa")) {
		t.Fatalf("size bytes = %v", artifact.SizeBytes)
	}
	if got := readString(t, filepath.Join(promotedPath, "nested", "part-a.txt")); got != "aa" {
		t.Fatalf("promoted nested file = %q", got)
	}
}

func TestPromoteArtifactsRejectsUnsafeAndMissingSources(t *testing.T) {
	tests := []struct {
		name     string
		artifact model.ArtifactDescriptor
		want     string
	}{
		{
			name:     "unsafe path",
			artifact: model.ArtifactDescriptor{Name: "bad", Kind: model.ArtifactKindFile, Path: "../escape.txt"},
			want:     "artifact path must not contain .. segments",
		},
		{
			name:     "missing file",
			artifact: model.ArtifactDescriptor{Name: "missing", Kind: model.ArtifactKindFile, Path: "missing.txt"},
			want:     "check artifact source",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := PromoteArtifacts(context.Background(), ArtifactPromotionRequest{
				StagingRoot: t.TempDir(),
				DataRoot:    t.TempDir(),
				WorkItemID:  "work-bad-001",
				Manifest: model.ArtifactManifest{
					Artifacts: []model.ArtifactDescriptor{tt.artifact},
				},
			})
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestPromoteArtifactsRejectsKindMismatches(t *testing.T) {
	staging := t.TempDir()
	writeFixture(t, staging, "file.txt", "file")
	if err := os.Mkdir(filepath.Join(staging, "dir"), 0755); err != nil {
		t.Fatalf("create dir: %v", err)
	}

	tests := []struct {
		name     string
		artifact model.ArtifactDescriptor
		want     string
	}{
		{
			name:     "file declaration points at directory",
			artifact: model.ArtifactDescriptor{Name: "dir_as_file", Kind: model.ArtifactKindFile, Path: "dir"},
			want:     "file artifact source is a directory",
		},
		{
			name:     "directory declaration points at file",
			artifact: model.ArtifactDescriptor{Name: "file_as_dir", Kind: model.ArtifactKindDirectory, Path: "file.txt"},
			want:     "directory artifact source is not a directory",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := PromoteArtifacts(context.Background(), ArtifactPromotionRequest{
				StagingRoot: staging,
				DataRoot:    t.TempDir(),
				WorkItemID:  "work-kind-001",
				Manifest: model.ArtifactManifest{
					Artifacts: []model.ArtifactDescriptor{tt.artifact},
				},
			})
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want %q", err, tt.want)
			}
		})
	}
}
