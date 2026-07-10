package api

import (
	"encoding/json"
	"flowscale/internal/engine"
	"flowscale/internal/repository"
	"log/slog"
	"net/http"
	"strings"
)

type StartWorkflowRequest struct {
	WorkflowID string `json:"workflow_id"`
}

type ExecutionHandler struct {
	engine   *engine.Engine
	execRepo *repository.ExecutionRepo
}

func NewExecutionHandler(eng *engine.Engine, execRepo *repository.ExecutionRepo) *ExecutionHandler {
	return &ExecutionHandler{engine: eng, execRepo: execRepo}
}

func (h *ExecutionHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/")

	if r.Method == http.MethodPost && path == "workflows/start" {
		h.handleStart(w, r)
		return
	}

	if r.Method == http.MethodGet && strings.HasPrefix(path, "executions/") {
		id := strings.TrimPrefix(path, "executions/")
		h.handleGet(w, r, id)
		return
	}

	http.NotFound(w, r)
}

func (h *ExecutionHandler) handleStart(w http.ResponseWriter, r *http.Request) {
	var req StartWorkflowRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	exec, err := h.engine.StartWorkflow(r.Context(), req.WorkflowID)
	if err != nil {
		slog.Error("failed to start workflow", "err", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(exec)
}

func (h *ExecutionHandler) handleGet(w http.ResponseWriter, r *http.Request, id string) {
	exec, err := h.execRepo.GetExecution(r.Context(), id)
	if err != nil {
		slog.Error("failed to get execution", "err", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(exec)
}
