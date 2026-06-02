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
}

func newController(items []model.WorkItem) *Controller {
	return &Controller{
		pending:  items,
		assigned: make(map[string]model.WorkItem),
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

	fmt.Println("controller listening on :8080")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		fmt.Println("controller failed:", err)
	}
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
