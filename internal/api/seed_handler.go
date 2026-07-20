package api

import (
	"encoding/json"
	"net/http"
	"time"

	"flowscale/internal/models"
	"flowscale/internal/repository"

	"github.com/google/uuid"
	"github.com/robfig/cron/v3"
)

type SeedHandler struct {
	wfRepo *repository.WorkflowRepo
}

func NewSeedHandler(wfRepo *repository.WorkflowRepo) *SeedHandler {
	return &SeedHandler{wfRepo: wfRepo}
}

func (h *SeedHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}

	wfs := []models.Workflow{
		{Name: "ECommerce-HappyPath", Activities: []models.Activity{{Name: "reserve-inventory", Timeout: "5m"}, {Name: "charge-card", Timeout: "5m"}}},
		{Name: "ECommerce-RetryDemo", Activities: []models.Activity{{Name: "reserve-inventory", Timeout: "5m"}, {Name: "create-shipment", Timeout: "5m", RetryPolicy: &models.RetryPolicy{MaxAttempts: 3, BackoffCoefficient: 1.5, InitialInterval: "2s"}}}},
		{Name: "ECommerce-SagaDemo", Activities: []models.Activity{{Name: "reserve-inventory", Timeout: "5m", Compensation: "release-inventory"}, {Name: "charge-card", Timeout: "5m", Compensation: "refund-payment"}, {Name: "always-fail", Timeout: "5m", RetryPolicy: &models.RetryPolicy{MaxAttempts: 1}}}},
		{Name: "Data-Sync-Cron", Activities: []models.Activity{{Name: "extract-data", Timeout: "5m"}, {Name: "transform-data", Timeout: "5m"}, {Name: "load-data", Timeout: "5m"}}},
		{Name: "Heavy-Batch-Demo", Activities: []models.Activity{
			{Name: "process-chunk-1", Timeout: "5m"},
			{Name: "process-chunk-2", Timeout: "5m"},
			{Name: "process-chunk-3", Timeout: "5m"},
			{Name: "process-chunk-4", Timeout: "5m"},
			{Name: "process-chunk-5", Timeout: "5m"},
		}},
	}

	existingWfs, err := h.wfRepo.ListWorkflows(r.Context(), 1000, 0)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	existingNames := make(map[string]bool)
	for _, ew := range existingWfs {
		existingNames[ew.Name] = true
	}

	for _, wf := range wfs {
		if !existingNames[wf.Name] {
			wfCopy := wf
			wfCopy.ID = uuid.NewString() // Ensure ID is set before creation
			err := h.wfRepo.CreateWorkflow(r.Context(), &wfCopy)
			if err != nil {
				continue
			}
			if wfCopy.Name == "Data-Sync-Cron" {
				now := time.Now()
				parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)
				parsed, _ := parser.Parse("* * * * *")
				sched := &models.Schedule{
					ID:             uuid.NewString(),
					WorkflowID:     wfCopy.ID,
					CronExpression: "* * * * *",
					ScheduleType:   "recurring",
					Status:         models.ScheduleStatusActive,
					CreatedAt:      now,
					UpdatedAt:      now,
					NextRunAt:      parsed.Next(now),
				}
				h.wfRepo.CreateSchedule(r.Context(), sched)
			}
		}
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{"status": "seeded"})
}
