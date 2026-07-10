package api

import (
	"encoding/json"
	"flowscale/internal/models"
	"flowscale/internal/repository"
	"github.com/google/uuid"
	"log/slog"
	"net/http"
	"strings"
)

type WorkflowHandler struct {
	repo *repository.WorkflowRepo
}

func NewWorkflowHandler(repo *repository.WorkflowRepo) *WorkflowHandler {
	return &WorkflowHandler{repo: repo}
}

func (h *WorkflowHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/workflows")
	path = strings.Trim(path, "/")

	if r.Method == http.MethodPost && path == "" {
		h.handleCreate(w, r)
		return
	}
	if r.Method == http.MethodGet && path == "" {
		h.handleList(w, r)
		return
	}
	if r.Method == http.MethodGet && path != "" {
		h.handleGet(w, r, path)
		return
	}
	if r.Method == http.MethodDelete && path != "" {
		h.handleDelete(w, r, path)
		return
	}

	http.NotFound(w, r)
}

func (h *WorkflowHandler) handleCreate(w http.ResponseWriter, r *http.Request) {
	var wf models.Workflow
	if err := json.NewDecoder(r.Body).Decode(&wf); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	if wf.Name == "" {
		http.Error(w, "workflow name is required", http.StatusBadRequest)
		return
	}

	wf.ID = uuid.NewString()

	if err := h.repo.CreateWorkflow(r.Context(), &wf); err != nil {
		slog.Error("failed to create workflow", "err", err)
		http.Error(w, "failed to create workflow", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(wf)
}

func (h *WorkflowHandler) handleGet(w http.ResponseWriter, r *http.Request, id string) {
	wf, err := h.repo.GetWorkflow(r.Context(), id)
	if err != nil {
		if err.Error() == "workflow not found" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		slog.Error("failed to get workflow", "err", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(wf)
}

func (h *WorkflowHandler) handleList(w http.ResponseWriter, r *http.Request) {
	wfs, err := h.repo.ListWorkflows(r.Context())
	if err != nil {
		slog.Error("failed to list workflows", "err", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	// Always return an array in json
	if wfs == nil {
		wfs = []*models.Workflow{}
	}

	json.NewEncoder(w).Encode(wfs)
}

func (h *WorkflowHandler) handleDelete(w http.ResponseWriter, r *http.Request, id string) {
	err := h.repo.DeleteWorkflow(r.Context(), id)
	if err != nil {
		if err.Error() == "workflow not found" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		slog.Error("failed to delete workflow", "err", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
