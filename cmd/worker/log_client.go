package main

import (
	"context"
	"fmt"
	"net/http"

	"goetl/internal/model"
)

type LogDeliveryError struct {
	Err error
}

func (e LogDeliveryError) Error() string {
	return fmt.Sprintf("log delivery failed: %v", e.Err)
}

func (e LogDeliveryError) Unwrap() error {
	return e.Err
}

type LogClient struct {
	ControllerURL string
	Controller    WorkerControllerClient
}

func (c LogClient) SendLogObservation(observation model.LogObservation) error {
	if err := observation.Validate(); err != nil {
		return &LogDeliveryError{Err: fmt.Errorf("invalid log observation: %w", err)}
	}

	controller, err := c.controllerClient()
	if err != nil {
		return &LogDeliveryError{Err: err}
	}
	request, err := controller.newJSONRequest(context.Background(), http.MethodPost, "/observations/logs", observation)
	if err != nil {
		return &LogDeliveryError{Err: fmt.Errorf("create log observation request: %w", err)}
	}
	response, err := controller.client.Do(request, http.StatusOK, http.StatusCreated, http.StatusAccepted, http.StatusNoContent)
	if err != nil {
		return &LogDeliveryError{Err: fmt.Errorf("post log observation: %w", err)}
	}
	defer response.Body.Close()

	return nil
}

func (c LogClient) controllerClient() (WorkerControllerClient, error) {
	if c.Controller.Initialized() {
		return c.Controller, nil
	}
	return newUnauthenticatedWorkerControllerClient(c.ControllerURL)
}

func (c LogClient) SendLogObservationWithFallback(observation model.LogObservation, fallbackLogDir string) error {
	err := c.SendLogObservation(observation)
	if err == nil {
		return nil
	}

	if fallbackErr := appendFallbackLogObservation(fallbackLogDir, observation); fallbackErr != nil {
		return &LogDeliveryError{Err: fmt.Errorf("log delivery fallback failed: %v, %w", err, fallbackErr)}
	}

	return err
}
