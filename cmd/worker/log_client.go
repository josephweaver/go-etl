package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

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
}

func (c LogClient) SendLogObservation(observation model.LogObservation) error {
	if err := observation.Validate(); err != nil {
		return LogDeliveryError{Err: fmt.Errorf("invalid log observation: %w", err)}
	}

	url := logObservationsURL(c.ControllerURL)
	body, err := json.Marshal(observation)
	if err != nil {
		return LogDeliveryError{Err: fmt.Errorf("encode log observation: %w", err)}
	}

	response, err := http.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return LogDeliveryError{Err: fmt.Errorf("post log observation to %s: %w", url, err)}
	}
	defer response.Body.Close()

	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return LogDeliveryError{Err: fmt.Errorf("post log observation to %s: unexpected status %s", url, response.Status)}
	}

	return nil
}

func (c LogClient) SendLogObservationWithFallback(observation model.LogObservation, fallbackLogDir string) error {
	err := c.SendLogObservation(observation)
	if err == nil {
		return nil
	}

	if fallbackErr := appendFallbackLogObservation(fallbackLogDir, observation); fallbackErr != nil {
		return LogDeliveryError{Err: fmt.Errorf("log delivery fallback failed: %v, %w", err, fallbackErr)}
	}

	return err
}

func logObservationsURL(controllerURL string) string {
	return strings.TrimRight(controllerURL, "/") + "/observations/logs"
}
