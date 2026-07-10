package api

import (
	"encoding/json"
	"flowscale/internal/models"
	"flowscale/internal/repository"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/robfig/cron/v3"
)

type ScheduleHandler struct {
	repo *repository.WorkflowRepo
}

func NewScheduleHandler(repo *repository.WorkflowRepo) *ScheduleHandler {
	return &ScheduleHandler{repo: repo}
}

func (h *ScheduleHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/")

	if r.Method == http.MethodPost && path == "schedules" {
		h.handleCreate(w, r)
		return
	}
	if r.Method == http.MethodGet && path == "schedules" {
		h.handleList(w, r)
		return
	}
	if r.Method == http.MethodDelete && strings.HasPrefix(path, "schedules/") {
		id := strings.TrimPrefix(path, "schedules/")
		h.handleDelete(w, r, id)
		return
	}

	http.NotFound(w, r)
}

func (h *ScheduleHandler) handleCreate(w http.ResponseWriter, r *http.Request) {
	var sched models.Schedule
	if err := json.NewDecoder(r.Body).Decode(&sched); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	sched.ID = uuid.NewString()
	sched.Status = models.ScheduleStatusActive
	sched.CreatedAt = time.Now()
	sched.UpdatedAt = time.Now()

	now := time.Now()
	if sched.ScheduleType == models.ScheduleTypeOnce || sched.ScheduleType == models.ScheduleTypeDelayed {
		if sched.RunAt != nil {
			sched.NextRunAt = *sched.RunAt
		} else {
			sched.NextRunAt = now
		}
	} else if sched.ScheduleType == models.ScheduleTypeRecurring {
		if sched.CronExpression != "" {
			parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)
			parsed, err := parser.Parse(sched.CronExpression)
			if err != nil {
				http.Error(w, "invalid cron expression", http.StatusBadRequest)
				return
			}
			sched.NextRunAt = parsed.Next(now)
		} else if sched.Interval != "" {
			duration, err := time.ParseDuration(sched.Interval)
			if err != nil {
				http.Error(w, "invalid interval", http.StatusBadRequest)
				return
			}
			sched.NextRunAt = now.Add(duration)
		} else {
			http.Error(w, "recurring schedule requires cron_expression or interval", http.StatusBadRequest)
			return
		}
	} else {
		http.Error(w, "invalid schedule_type", http.StatusBadRequest)
		return
	}

	if err := h.repo.CreateSchedule(r.Context(), &sched); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(sched)
}

func (h *ScheduleHandler) handleList(w http.ResponseWriter, r *http.Request) {
	schedules, err := h.repo.ListSchedules(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if schedules == nil {
		schedules = make([]models.Schedule, 0)
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(schedules)
}

func (h *ScheduleHandler) handleDelete(w http.ResponseWriter, r *http.Request, id string) {
	if err := h.repo.DeleteSchedule(r.Context(), id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
