package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"goetl/internal/model"
)

func reportWorkComplete(controllerURL string, itemID string) error {
	url := strings.TrimRight(controllerURL, "/") + "/work/complete"

	body, err := json.Marshal(model.WorkCompletion{ID: itemID})
	if err != nil {
		return fmt.Errorf("encode work completion: %w", err)
	}

	response, err := http.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("post work completion to %s: %w", url, err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusNoContent {
		return fmt.Errorf("post work completion to %s: unexpected status %s", url, response.Status)
	}

	return nil
}

func reportWorkFailed(controllerURL string, itemID string, workErr error) error {
	url := strings.TrimRight(controllerURL, "/") + "/work/fail"

	body, err := json.Marshal(model.WorkFailure{ID: itemID, Error: workErr.Error()})
	if err != nil {
		return fmt.Errorf("encode work failure: %w", err)
	}

	response, err := http.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("post work failure to %s: %w", url, err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusNoContent {
		return fmt.Errorf("post work failure to %s: unexpected status %s", url, response.Status)
	}

	return nil
}

func fetchWorkItem(controllerURL string) (model.WorkItem, bool, error) {
	url := strings.TrimRight(controllerURL, "/") + "/work/next"

	response, err := http.Get(url)
	if err != nil {
		return model.WorkItem{}, false, fmt.Errorf("get work item from %s: %w", url, err)
	}
	defer response.Body.Close()

	if response.StatusCode == http.StatusNoContent {
		return model.WorkItem{}, false, nil
	}

	if response.StatusCode != http.StatusOK {
		return model.WorkItem{}, false, fmt.Errorf("get work item from %s: unexpected status %s", url, response.Status)
	}

	var item model.WorkItem
	if err := json.NewDecoder(response.Body).Decode(&item); err != nil {
		return model.WorkItem{}, false, fmt.Errorf("decode work item from %s: %w", url, err)
	}

	if err := item.Validate(); err != nil {
		return model.WorkItem{}, false, fmt.Errorf("validate work item from %s: %w", url, err)
	}

	return item, true, nil
}
