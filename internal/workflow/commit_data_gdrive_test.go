package workflow

import (
	"testing"

	"goetl/internal/model"
)

func TestCommitDataResourceConstraintsUseGDriveRcloneUploadMutex(t *testing.T) {
	constraints, err := CommitDataResourceConstraints(model.BoundPublishTarget{
		Name:         "publish_archive",
		FromArtifact: "archive",
		TargetName:   "publish_archive",
		Location: model.DataAssetLocation{
			Type:      model.DataProviderGDriveRclone,
			Remote:    "gdrive",
			DrivePath: "Data/ETL/test/goetl-publish-smoke.zip",
		},
		OverwritePolicy: model.PublishedDataAssetOverwriteFailIfExists,
	}, "hpcc")
	if err != nil {
		t.Fatalf("CommitDataResourceConstraints() error = %v", err)
	}
	if len(constraints) != 1 {
		t.Fatalf("constraint count = %d, want 1", len(constraints))
	}
	if constraints[0].ResourceKey != "provider:gdrive-rclone:gdrive/upload" {
		t.Fatalf("resource key = %q", constraints[0].ResourceKey)
	}
}
