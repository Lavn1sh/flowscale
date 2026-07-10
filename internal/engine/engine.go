package engine

import (
	"context"
	"fmt"
	"time"

	"flowscale/internal/models"
	"flowscale/internal/repository"
	"github.com/google/uuid"
)

type Engine struct {
	wfRepo   *repository.WorkflowRepo
	execRepo *repository.ExecutionRepo
}

func NewEngine(wfRepo *repository.WorkflowRepo, execRepo *repository.ExecutionRepo) *Engine {
	return &Engine{wfRepo: wfRepo, execRepo: execRepo}
}

func (e *Engine) StartWorkflow(ctx context.Context, workflowID string) (*models.WorkflowExecution, error) {
	wf, err := e.wfRepo.GetWorkflow(ctx, workflowID)
	if err != nil {
		return nil, fmt.Errorf("failed to get workflow: %w", err)
	}

	now := time.Now()
	exec := &models.WorkflowExecution{
		ID:         uuid.NewString(),
		WorkflowID: workflowID,
		Status:     models.ExecutionStatusRunning,
		CreatedAt:  now,
		UpdatedAt:  now,
	}

	var firstActivity *models.ActivityExecution
	if len(wf.Activities) > 0 {
		act := wf.Activities[0]
		exec.CurrentActivity = act.Name
		
		firstActivity = &models.ActivityExecution{
			ID:             uuid.NewString(),
			ExecutionID:    exec.ID,
			ActivityName:   act.Name,
			Attempt:        1,
			Status:         models.ActivityStatusPending,
			IdempotencyKey: fmt.Sprintf("%s-%s-%d", exec.ID, act.Name, 1),
		}
	} else {
		exec.Status = models.ExecutionStatusCompleted
	}

	event := &models.WorkflowEvent{
		ID:          uuid.NewString(),
		ExecutionID: exec.ID,
		EventType:   models.EventWorkflowStarted,
		Payload:     []byte(`{}`),
		Timestamp:   now,
	}

	if err := e.execRepo.CreateExecution(ctx, exec, event, firstActivity); err != nil {
		return nil, fmt.Errorf("failed to create execution: %w", err)
	}

	return exec, nil
}
