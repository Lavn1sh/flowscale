package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"flowscale/internal/engine"
	"flowscale/internal/models"
	"flowscale/internal/repository"
)

type DLQHandler struct {
	engine   *engine.Engine
	execRepo *repository.ExecutionRepo
}

func NewDLQHandler(e *engine.Engine, execRepo *repository.ExecutionRepo) *DLQHandler {
	return &DLQHandler{engine: e, execRepo: execRepo}
}

func (h *DLQHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/")

	if r.Method == http.MethodGet && path == "activities/dlq" {
		h.handleList(w, r)
		return
	}

	if r.Method == http.MethodPost && strings.HasPrefix(path, "activities/dlq/") && strings.HasSuffix(path, "/retry") {
		// path: activities/dlq/{id}/retry
		parts := strings.Split(path, "/")
		if len(parts) == 4 {
			id := parts[2]
			h.handleRetry(w, r, id)
			return
		}
	}

	http.NotFound(w, r)
}

func (h *DLQHandler) handleList(w http.ResponseWriter, r *http.Request) {
	acts, err := h.execRepo.ListDeadLetteredActivities(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if acts == nil {
		acts = make([]models.ActivityExecution, 0)
	}
	if err := json.NewEncoder(w).Encode(acts); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (h *DLQHandler) handleRetry(w http.ResponseWriter, r *http.Request, id string) {
	if err := h.engine.RetryDeadLetteredActivity(r.Context(), id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}
