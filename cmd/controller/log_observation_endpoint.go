package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"goetl/internal/model"
)

type logObservationReceiver interface {
	AcceptLogObservation(context.Context, model.LogObservation) error
}

func (c *Controller) logObservationsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body := r.Body
	if c.maxRequestBytes > 0 {
		if r.ContentLength > int64(c.maxRequestBytes) {
			http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
			return
		}
		body = http.MaxBytesReader(w, r.Body, int64(c.maxRequestBytes))
	}

	var payload model.LogObservation
	decoder := json.NewDecoder(body)
	if err := decoder.Decode(&payload); err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
			return
		}
		http.Error(w, "decode log observation", http.StatusBadRequest)
		return
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		http.Error(w, "log observation must be one JSON document", http.StatusBadRequest)
		return
	}

	if err := payload.Validate(); err != nil {
		http.Error(w, "invalid log observation: "+err.Error(), http.StatusBadRequest)
		return
	}

	if err := c.AcceptLogObservation(r.Context(), payload); err != nil {
		http.Error(w, "accept log observation", http.StatusInternalServerError)
		return
	}

	// No durable handling in this slice; log observations are accepted and acknowledged.
	w.WriteHeader(http.StatusNoContent)
}

func (c *Controller) AcceptLogObservation(ctx context.Context, observation model.LogObservation) error {
	return nil
}

var _ logObservationReceiver = (*Controller)(nil)
