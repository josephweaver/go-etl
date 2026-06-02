package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	"goetl/internal/model"
)

type Controller struct {
	mu       sync.Mutex
	pending  []model.WorkItem
	assigned map[string]model.WorkItem
	failed   map[string]model.WorkFailure
}

func newController(items []model.WorkItem) *Controller {
	return &Controller{
		pending:  items,
		assigned: make(map[string]model.WorkItem),
		failed:   make(map[string]model.WorkFailure),
	}
}

func main() {
	controller := newController([]model.WorkItem{
		{
			ID:             "local-demo-001",
			Type:           model.WorkItemTypeWriteDemoOutput,
			OutputFilename: "local-demo-001.txt",
		},
	})

	http.HandleFunc("/work/next", controller.nextWorkHandler)
	http.HandleFunc("/work/complete", controller.completeWorkHandler)
	http.HandleFunc("/work/fail", controller.failWorkHandler)
	http.HandleFunc("/work", controller.submitWorkHandler)
	http.HandleFunc("/status", controller.statusHandler)

	fmt.Println("controller listening on :8080")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		fmt.Println("controller failed:", err)
	}
}

func (c *Controller) submitWorkHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var item model.WorkItem
	if err := json.NewDecoder(r.Body).Decode(&item); err != nil {
		http.Error(w, "decode work item", http.StatusBadRequest)
		return
	}

	if err := item.Validate(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.hasWorkItemID(item.ID) {
		http.Error(w, "work item id already exists", http.StatusConflict)
		return
	}

	c.pending = append(c.pending, item)
	w.WriteHeader(http.StatusNoContent)
}

func (c *Controller) hasWorkItemID(id string) bool {
	for _, item := range c.pending {
		if item.ID == id {
			return true
		}
	}

	if _, ok := c.assigned[id]; ok {
		return true
	}

	if _, ok := c.failed[id]; ok {
		return true
	}

	return false
}

func (c *Controller) statusHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	c.mu.Lock()
	status := model.ControllerStatus{
		Pending:  len(c.pending),
		Assigned: len(c.assigned),
		Failed:   len(c.failed),
	}
	c.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(status); err != nil {
		http.Error(w, "encode status", http.StatusInternalServerError)
	}
}

func (c *Controller) failWorkHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var failure model.WorkFailure
	if err := json.NewDecoder(r.Body).Decode(&failure); err != nil {
		http.Error(w, "decode failure", http.StatusBadRequest)
		return
	}

	if failure.ID == "" || failure.Error == "" {
		http.Error(w, "work item id and error are required", http.StatusBadRequest)
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if _, ok := c.assigned[failure.ID]; !ok {
		http.Error(w, "work item not assigned", http.StatusNotFound)
		return
	}

	delete(c.assigned, failure.ID)
	c.failed[failure.ID] = failure
	fmt.Println("work item failed:", failure.ID, failure.Error)
	w.WriteHeader(http.StatusNoContent)
}

func (c *Controller) completeWorkHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var completion model.WorkCompletion
	if err := json.NewDecoder(r.Body).Decode(&completion); err != nil {
		http.Error(w, "decode completion", http.StatusBadRequest)
		return
	}

	if completion.ID == "" {
		http.Error(w, "work item id is required", http.StatusBadRequest)
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if _, ok := c.assigned[completion.ID]; !ok {
		http.Error(w, "work item not assigned", http.StatusNotFound)
		return
	}

	delete(c.assigned, completion.ID)
	fmt.Println("work item completed:", completion.ID)
	w.WriteHeader(http.StatusNoContent)
}

func (c *Controller) nextWorkHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if len(c.pending) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	item := c.pending[0]
	c.pending = c.pending[1:]
	c.assigned[item.ID] = item

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(item); err != nil {
		http.Error(w, "encode work item", http.StatusInternalServerError)
	}
}
