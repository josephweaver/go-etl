package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"goetl/internal/model"
)

func (c WorkerControllerClient) ConfirmCheckpoint(
	ctx context.Context,
	session WorkerSession,
	confirmation model.WorkCheckpointConfirmation,
) (model.WorkCheckpointAcknowledgement, error) {
	if err := session.ValidateIdentity(); err != nil {
		return model.WorkCheckpointAcknowledgement{}, err
	}
	if err := confirmation.Validate(); err != nil {
		return model.WorkCheckpointAcknowledgement{}, fmt.Errorf("validate checkpoint confirmation: %w", err)
	}

	acknowledgement, err := c.postCheckpointRequest(ctx, "/work/checkpoint/confirm", session, confirmation)
	if err != nil {
		return model.WorkCheckpointAcknowledgement{}, fmt.Errorf("confirm checkpoint: %w", err)
	}
	if acknowledgement.Operation != model.CheckpointOperationConfirmation ||
		acknowledgement.ResumeArtifactID != confirmation.Reference.ResumeArtifactID ||
		acknowledgement.Reference != confirmation.Reference ||
		acknowledgement.CaptureKind != confirmation.CaptureKind ||
		acknowledgement.Disposition != confirmation.Disposition ||
		acknowledgement.SuspendedAt != confirmation.SuspendedAt {
		return model.WorkCheckpointAcknowledgement{}, fmt.Errorf("checkpoint confirmation acknowledgement does not match request")
	}
	return acknowledgement, nil
}

func (c WorkerControllerClient) SuspendLatestCheckpoint(
	ctx context.Context,
	session WorkerSession,
	suspension model.WorkCheckpointSuspendLatest,
) (model.WorkCheckpointAcknowledgement, error) {
	if err := session.ValidateIdentity(); err != nil {
		return model.WorkCheckpointAcknowledgement{}, err
	}
	if err := suspension.Validate(); err != nil {
		return model.WorkCheckpointAcknowledgement{}, fmt.Errorf("validate checkpoint suspension: %w", err)
	}

	acknowledgement, err := c.postCheckpointRequest(ctx, "/work/checkpoint/suspend-latest", session, suspension)
	if err != nil {
		return model.WorkCheckpointAcknowledgement{}, fmt.Errorf("suspend from latest checkpoint: %w", err)
	}
	if acknowledgement.Operation != model.CheckpointOperationSuspendLatest ||
		acknowledgement.SuspendedAt != suspension.SuspendedAt {
		return model.WorkCheckpointAcknowledgement{}, fmt.Errorf("suspend-latest acknowledgement does not match request")
	}
	return acknowledgement, nil
}

func (c WorkerControllerClient) postCheckpointRequest(
	ctx context.Context,
	path string,
	session WorkerSession,
	payload any,
) (model.WorkCheckpointAcknowledgement, error) {
	request, err := c.newJSONRequest(ctx, http.MethodPost, path, payload)
	if err != nil {
		return model.WorkCheckpointAcknowledgement{}, fmt.Errorf("create checkpoint request: %w", err)
	}
	if err := addWorkerSessionHeaders(request, session); err != nil {
		return model.WorkCheckpointAcknowledgement{}, err
	}
	response, err := c.client.Do(request, http.StatusOK)
	if err != nil {
		return model.WorkCheckpointAcknowledgement{}, fmt.Errorf("post checkpoint request: %w", err)
	}
	defer response.Body.Close()

	var acknowledgement model.WorkCheckpointAcknowledgement
	decoder := json.NewDecoder(response.Body)
	if err := decoder.Decode(&acknowledgement); err != nil {
		return model.WorkCheckpointAcknowledgement{}, fmt.Errorf("decode checkpoint acknowledgement: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return model.WorkCheckpointAcknowledgement{}, fmt.Errorf("checkpoint acknowledgement must contain one JSON document")
	}
	if err := acknowledgement.Validate(); err != nil {
		return model.WorkCheckpointAcknowledgement{}, fmt.Errorf("validate checkpoint acknowledgement: %w", err)
	}
	return acknowledgement, nil
}
