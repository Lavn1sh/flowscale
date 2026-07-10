package api

import (
	"encoding/json"
	"flowscale/internal/engine"
	"flowscale/internal/models"
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

	if r.Method == http.MethodGet && (path == "executions" || path == "executions/") {
		h.handleList(w, r)
		return
	}

	if r.Method == http.MethodPost && strings.HasPrefix(path, "executions/") && strings.HasSuffix(path, "/compensate/retry") {
		id := strings.TrimPrefix(path, "executions/")
		id = strings.TrimSuffix(id, "/compensate/retry")
		h.handleRetryCompensation(w, r, id)
		return
	}

	if r.Method == http.MethodGet && strings.HasPrefix(path, "executions/") {
		id := strings.TrimPrefix(path, "executions/")
		if id == "" {
			h.handleList(w, r)
			return
		}
		if strings.HasSuffix(id, "/events") {
			id = strings.TrimSuffix(id, "/events")
			h.handleGetEvents(w, r, id)
			return
		}
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

func (h *ExecutionHandler) handleList(w http.ResponseWriter, r *http.Request) {
	execs, err := h.execRepo.ListExecutions(r.Context())
	if err != nil {
		slog.Error("failed to list executions", "err", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	if execs == nil {
		execs = make([]models.WorkflowExecution, 0)
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(execs)
}

func (h *ExecutionHandler) handleGetEvents(w http.ResponseWriter, r *http.Request, id string) {
	events, err := h.execRepo.GetExecutionEvents(r.Context(), id)
	if err != nil {
		slog.Error("failed to get execution events", "err", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	if events == nil {
		events = make([]models.WorkflowEvent, 0)
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(events)
}

func (h *ExecutionHandler) handleRetryCompensation(w http.ResponseWriter, r *http.Request, executionID string) {
	if err := h.engine.RetryCompensation(r.Context(), executionID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}
