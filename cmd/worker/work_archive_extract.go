package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"goetl/internal/model"
)

func (w Worker) ArchiveExtract(ctx OperationContext) (WorkEvidence, error) {
	item := ctx.WorkItem
	payload, err := archiveExtractPayloadFromWorkItem(item)
	if err != nil {
		return WorkEvidence{}, err
	}
	if payload.ArchiveType != model.DataAssetArchiveTypeZip {
		return WorkEvidence{}, fmt.Errorf("archive_extract supports zip archives only in this slice")
	}
	if len(payload.Members) != 1 || !payload.Members[0].Required {
		return WorkEvidence{}, fmt.Errorf("archive_extract supports exactly one required selected member in this slice")
	}

	sourcePath, err := archiveExtractSourcePath(item, payload.Source)
	if err != nil {
		return WorkEvidence{}, err
	}

	attemptID := item.AttemptID
	if attemptID == "" {
		attemptID = item.ID + "-attempt"
	}
	workDir := filepath.Join(w.Config.TmpDir, "attempts", attemptID, "work", "archive_extract")
	artifactDir := filepath.Join(w.Config.TmpDir, "attempts", attemptID, "artifacts")
	if err := os.MkdirAll(workDir, 0755); err != nil {
		return WorkEvidence{}, fmt.Errorf("create archive_extract work dir %s: %w", workDir, err)
	}

	preState := map[string]any{
		"operator":     string(model.WorkItemTypeArchiveExtract),
		"archive_type": payload.ArchiveType,
		"source_path":  sourcePath,
		"member":       payload.Members[0].Member,
	}
	preStateSHA256, err := canonicalObservationSHA256(preState)
	if err != nil {
		return WorkEvidence{}, err
	}

	required := payload.Members[0].Required
	result, err := extractArchiveSelection(archiveExtractionRequest{
		sourcePath:     sourcePath,
		extractionRoot: workDir,
		archive: model.DataAssetArchive{
			Type: model.DataAssetArchiveTypeZip,
			Select: []model.DataAssetArchiveSelect{
				{
					Member:   payload.Members[0].Member,
					As:       payload.Members[0].As,
					Required: &required,
				},
			},
			Expose: model.DataAssetArchiveExposeSelectedPath,
		},
		maxSelectedFileSize: w.Config.effectiveMaxAssetBytes(),
	})
	if err != nil {
		return WorkEvidence{}, err
	}

	stagedPath := filepath.Join(artifactDir, filepath.FromSlash(payload.OutputPath))
	if _, err := copyFileWithLimit(result.localPath, stagedPath, w.Config.effectiveMaxAssetBytes(), 0); err != nil {
		return WorkEvidence{}, fmt.Errorf("stage archive_extract output %s: %w", payload.OutputPath, err)
	}

	runID := ""
	if item.Source != nil {
		runID = item.Source.RunID
	}
	promoted, err := PromoteArtifacts(context.Background(), ArtifactPromotionRequest{
		StagingRoot: artifactDir,
		DataRoot:    w.Config.DataDir,
		RunID:       runID,
		WorkItemID:  item.ID,
		AttemptID:   item.AttemptID,
		Manifest: model.ArtifactManifest{
			Schema:       model.ArtifactManifestSchemaV1,
			StorageScope: "artifact_staging",
			Artifacts: []model.ArtifactDescriptor{
				{
					Name:   payload.OutputPath,
					Kind:   model.ArtifactKindFile,
					Format: "zip_member",
					Path:   payload.OutputPath,
					Metadata: map[string]any{
						"archive_type":   payload.ArchiveType,
						"archive_member": payload.Members[0].Member,
					},
				},
			},
		},
	})
	if err != nil {
		return WorkEvidence{}, fmt.Errorf("promote archive_extract artifact: %w", err)
	}

	manifestJSON, err := json.Marshal(promoted)
	if err != nil {
		return WorkEvidence{}, fmt.Errorf("encode archive_extract manifest: %w", err)
	}
	outputJSON, outputSHA256, _, err := canonicalJSONDocument(manifestJSON, "archive_extract manifest")
	if err != nil {
		return WorkEvidence{}, err
	}

	postState := map[string]any{
		"operator":      string(model.WorkItemTypeArchiveExtract),
		"archive_type":  payload.ArchiveType,
		"source_path":   sourcePath,
		"member":        payload.Members[0].Member,
		"output_sha256": outputSHA256,
	}
	postStateSHA256, err := canonicalObservationSHA256(postState)
	if err != nil {
		return WorkEvidence{}, err
	}
	preStateJSON, err := json.Marshal(preState)
	if err != nil {
		return WorkEvidence{}, fmt.Errorf("encode archive_extract pre-state: %w", err)
	}
	postStateJSON, err := json.Marshal(postState)
	if err != nil {
		return WorkEvidence{}, fmt.Errorf("encode archive_extract post-state: %w", err)
	}

	return WorkEvidence{
		InputSHA256:     preStateSHA256,
		OutputSHA256:    outputSHA256,
		PreStateSHA256:  preStateSHA256,
		PostStateSHA256: postStateSHA256,
		OutputJSON:      string(outputJSON),
		PreStateJSON:    string(preStateJSON),
		PostStateJSON:   string(postStateJSON),
	}, nil
}

func archiveExtractPayloadFromWorkItem(item model.WorkItem) (model.ArchiveExtractWorkItemPayload, error) {
	parameter, ok := item.Parameters["archive_extract"]
	if !ok {
		return model.ArchiveExtractWorkItemPayload{}, fmt.Errorf("archive_extract parameter is required")
	}
	if parameter.Type != "archive_extract" {
		return model.ArchiveExtractWorkItemPayload{}, fmt.Errorf("parameter archive_extract has type %s, want archive_extract", parameter.Type)
	}
	data, err := json.Marshal(parameter.Value)
	if err != nil {
		return model.ArchiveExtractWorkItemPayload{}, fmt.Errorf("encode archive_extract parameter: %w", err)
	}
	var payload model.ArchiveExtractWorkItemPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return model.ArchiveExtractWorkItemPayload{}, fmt.Errorf("decode archive_extract parameter: %w", err)
	}
	if err := payload.Validate(); err != nil {
		return model.ArchiveExtractWorkItemPayload{}, err
	}
	return payload, nil
}

func archiveExtractSourcePath(item model.WorkItem, source model.ArchiveExtractSource) (string, error) {
	if source.LocalPath != "" {
		return source.LocalPath, nil
	}
	if source.MaterializedAsset == nil {
		return "", fmt.Errorf("archive_extract source is required")
	}
	manifest, ok, err := materializedDataAssetsFromWorkItem(item)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", fmt.Errorf("archive_extract materialized_asset source requires materialized_data_assets parameter")
	}
	var matches []model.MaterializedDataAsset
	for _, asset := range manifest.Assets {
		if asset.BindingName == source.MaterializedAsset.BindingName {
			matches = append(matches, asset)
		}
	}
	if len(matches) != 1 {
		return "", fmt.Errorf("archive_extract materialized_asset binding %q matched %d assets", source.MaterializedAsset.BindingName, len(matches))
	}
	return matches[0].LocalPath, nil
}
