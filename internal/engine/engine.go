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

func (e *Engine) PollPendingActivity(ctx context.Context) (*models.ActivityExecution, error) {
	return e.execRepo.GetAndLockPendingActivity(ctx)
}

func (e *Engine) ReportActivitySuccess(ctx context.Context, activityID string, executionID string, activityName string) error {
	if err := e.execRepo.CompleteActivity(ctx, activityID); err != nil {
		return err
	}

	exec, err := e.execRepo.GetExecution(ctx, executionID)
	if err != nil {
		return err
	}

	wf, err := e.wfRepo.GetWorkflow(ctx, exec.WorkflowID)
	if err != nil {
		return err
	}

	var nextAct *models.Activity
	found := false
	for i, act := range wf.Activities {
		if act.Name == activityName {
			if i+1 < len(wf.Activities) {
				nextAct = &wf.Activities[i+1]
			}
			found = true
			break
		}
	}

	if !found {
		return fmt.Errorf("activity %s not found in workflow definition", activityName)
	}

	now := time.Now()
	if nextAct != nil {
		nextExec := &models.ActivityExecution{
			ID:             uuid.NewString(),
			ExecutionID:    executionID,
			ActivityName:   nextAct.Name,
			Attempt:        1,
			Status:         models.ActivityStatusPending,
			IdempotencyKey: fmt.Sprintf("%s-%s-%d", executionID, nextAct.Name, 1),
		}

		event := &models.WorkflowEvent{
			ID:          uuid.NewString(),
			ExecutionID: executionID,
			EventType:   models.EventActivityScheduled,
			Payload:     []byte(`{"activity":"` + nextAct.Name + `"}`),
			Timestamp:   now,
		}

		return e.execRepo.ScheduleNextActivity(ctx, executionID, nextExec, event)
	} else {
		event := &models.WorkflowEvent{
			ID:          uuid.NewString(),
			ExecutionID: executionID,
			EventType:   models.EventWorkflowCompleted,
			Payload:     []byte(`{}`),
			Timestamp:   now,
		}
		return e.execRepo.CompleteExecution(ctx, executionID, event)
	}
}

func (e *Engine) ReportActivityFailure(ctx context.Context, activityID string, executionID string, activityName string) error {
	if err := e.execRepo.FailActivity(ctx, activityID); err != nil {
		return err
	}

	now := time.Now()
	event := &models.WorkflowEvent{
		ID:          uuid.NewString(),
		ExecutionID: executionID,
		EventType:   models.EventWorkflowFailed,
		Payload:     []byte(`{"activity":"` + activityName + `"}`),
		Timestamp:   now,
	}
	return e.execRepo.FailExecution(ctx, executionID, event)
}
