package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"flowscale/internal/models"
	"flowscale/internal/queue"
	"flowscale/internal/repository"
	"github.com/google/uuid"
)

type Engine struct {
	wfRepo   *repository.WorkflowRepo
	execRepo *repository.ExecutionRepo
	mq       *queue.RabbitMQ
}

func NewEngine(wfRepo *repository.WorkflowRepo, execRepo *repository.ExecutionRepo, mq *queue.RabbitMQ) *Engine {
	return &Engine{wfRepo: wfRepo, execRepo: execRepo, mq: mq}
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

	if firstActivity != nil {
		e.mq.PublishTask(ctx, firstActivity.ActivityName, models.ActivityTaskMessage{
			ExecutionID:    exec.ID,
			ActivityID:     firstActivity.ID,
			ActivityName:   firstActivity.ActivityName,
			IdempotencyKey: firstActivity.IdempotencyKey,
		})
	}

	return exec, nil
}

func (e *Engine) StartResultConsumer(ctx context.Context) {
	slog.Info("Engine started RabbitMQ result consumer")
	deliveries, err := e.mq.ConsumeResults()
	if err != nil {
		slog.Error("Failed to start result consumer", "err", err)
		return
	}

	for d := range deliveries {
		var res models.ActivityResultMessage
		if err := json.Unmarshal(d.Body, &res); err != nil {
			slog.Error("Failed to unmarshal result message", "err", err)
			d.Nack(false, false) // discard
			continue
		}

		if res.Success {
			err = e.ReportActivitySuccess(ctx, res.ActivityID, res.ExecutionID, res.ActivityName)
		} else {
			err = e.ReportActivityFailure(ctx, res.ActivityID, res.ExecutionID, res.ActivityName)
		}

		if err != nil {
			slog.Error("Failed to process activity result", "err", err)
			d.Nack(false, true) // requeue on DB error
		} else {
			d.Ack(false)
		}
	}
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

		err = e.execRepo.ScheduleNextActivity(ctx, executionID, nextExec, event)
		if err == nil {
			e.mq.PublishTask(ctx, nextExec.ActivityName, models.ActivityTaskMessage{
				ExecutionID:    executionID,
				ActivityID:     nextExec.ID,
				ActivityName:   nextExec.ActivityName,
				IdempotencyKey: nextExec.IdempotencyKey,
			})
		}
		return err
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
	act, err := e.execRepo.GetActivityExecution(ctx, activityID)
	if err != nil {
		return fmt.Errorf("failed to get activity execution: %w", err)
	}

	exec, err := e.execRepo.GetExecution(ctx, executionID)
	if err != nil {
		return err
	}

	wf, err := e.wfRepo.GetWorkflow(ctx, exec.WorkflowID)
	if err != nil {
		return err
	}

	var wfAct *models.Activity
	for i, a := range wf.Activities {
		if a.Name == activityName {
			wfAct = &wf.Activities[i]
			break
		}
	}

	if wfAct == nil {
		return fmt.Errorf("activity %s not found in workflow definition", activityName)
	}

	maxAttempts := 1
	if wfAct.RetryPolicy != nil && wfAct.RetryPolicy.MaxAttempts > 1 {
		maxAttempts = wfAct.RetryPolicy.MaxAttempts
	}

	if act.Attempt < maxAttempts {
		if err := e.execRepo.FailActivity(ctx, activityID); err != nil {
			return err
		}

		nextAttempt := act.Attempt + 1
		tier := 1 << (nextAttempt - 2)
		if tier > 16 {
			tier = 16
		}

		nextExec := &models.ActivityExecution{
			ID:             uuid.NewString(),
			ExecutionID:    executionID,
			ActivityName:   activityName,
			Attempt:        nextAttempt,
			Status:         models.ActivityStatusPending,
			IdempotencyKey: fmt.Sprintf("%s-%s-%d", executionID, activityName, nextAttempt),
		}

		now := time.Now()
		event := &models.WorkflowEvent{
			ID:          uuid.NewString(),
			ExecutionID: executionID,
			EventType:   models.EventActivityScheduled,
			Payload:     []byte(fmt.Sprintf(`{"activity":"%s","attempt":%d}`, activityName, nextAttempt)),
			Timestamp:   now,
		}

		err = e.execRepo.ScheduleNextActivity(ctx, executionID, nextExec, event)
		if err == nil {
			e.mq.PublishRetryTask(ctx, activityName, tier, models.ActivityTaskMessage{
				ExecutionID:    executionID,
				ActivityID:     nextExec.ID,
				ActivityName:   nextExec.ActivityName,
				IdempotencyKey: nextExec.IdempotencyKey,
			})
		}
		return err
	}

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
