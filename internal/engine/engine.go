package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"flowscale/internal/models"
	"flowscale/internal/observability"
	"flowscale/internal/queue"
	"flowscale/internal/repository"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
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
	ctx, span := otel.Tracer("engine").Start(ctx, "StartWorkflow", trace.WithAttributes(
		attribute.String("workflowID", workflowID),
	))
	defer span.End()

	wf, err := e.wfRepo.GetWorkflow(ctx, workflowID)
	if err != nil {
		span.RecordError(err)
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

	var outboxMsg *models.OutboxMessage
	if firstActivity != nil {
		payload, _ := json.Marshal(models.ActivityTaskMessage{
			ExecutionID:    exec.ID,
			ActivityID:     firstActivity.ID,
			ActivityName:   firstActivity.ActivityName,
			IdempotencyKey: firstActivity.IdempotencyKey,
		})
		outboxMsg = &models.OutboxMessage{
			ID:      uuid.NewString(),
			Topic:   firstActivity.ActivityName,
			Tier:    0,
			Payload: payload,
		}
	}

	if err := e.execRepo.CreateExecution(ctx, exec, event, firstActivity, outboxMsg); err != nil {
		return nil, fmt.Errorf("failed to create execution: %w", err)
	}

	observability.WorkflowsStartedTotal.Inc()


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
			err = e.ReportActivitySuccess(observability.Extract(ctx, d.Headers), res.ActivityID, res.ExecutionID, res.ActivityName)
		} else {
			err = e.ReportActivityFailure(observability.Extract(ctx, d.Headers), res.ActivityID, res.ExecutionID, res.ActivityName, res.NonRetryable)
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
	ctx, span := otel.Tracer("engine").Start(ctx, "ReportActivitySuccess", trace.WithAttributes(
		attribute.String("executionID", executionID),
		attribute.String("activityID", activityID),
		attribute.String("activityName", activityName),
	))
	defer span.End()

	conn, err := e.execRepo.LockWorkflow(ctx, executionID)
	if err != nil {
		span.RecordError(err)
		return err
	}
	defer e.execRepo.UnlockWorkflow(ctx, conn, executionID)

	actExec, err := e.execRepo.GetActivityExecution(ctx, activityID)
	if err != nil {
		return err
	}
	if actExec.Status == models.ActivityStatusCompleted {
		slog.Warn("Duplicate success report ignored", "activityID", activityID)
		return nil
	}

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

	if exec.Status == models.ExecutionStatusCompensating {
		acts, err := e.execRepo.ListActivityExecutions(ctx, executionID)
		if err != nil {
			return err
		}

		nextComp := determineNextCompensation(wf, acts)
		now := time.Now()

		if nextComp != nil {
			nextExec := &models.ActivityExecution{
				ID:             uuid.NewString(),
				ExecutionID:    executionID,
				ActivityName:   nextComp.Name,
				Attempt:        1,
				Status:         models.ActivityStatusPending,
				IdempotencyKey: fmt.Sprintf("%s-%s-1", executionID, nextComp.Name),
			}

			event := &models.WorkflowEvent{
				ID:          uuid.NewString(),
				ExecutionID: executionID,
				EventType:   models.EventCompensationStarted,
				Payload:     []byte(`{"activity":"` + nextComp.Name + `"}`),
				Timestamp:   now,
			}

			payload, _ := json.Marshal(models.ActivityTaskMessage{
				ExecutionID:    executionID,
				ActivityID:     nextExec.ID,
				ActivityName:   nextExec.ActivityName,
				IdempotencyKey: nextExec.IdempotencyKey,
			})
			outboxMsg := &models.OutboxMessage{
				ID:      uuid.NewString(),
				Topic:   nextComp.Name,
				Payload: payload,
			}
			return e.execRepo.StartCompensating(ctx, executionID, nextExec, event, outboxMsg)
		} else {
			event := &models.WorkflowEvent{
				ID:          uuid.NewString(),
				ExecutionID: executionID,
				EventType:   models.EventCompensationCompleted,
				Payload:     []byte(`{}`),
				Timestamp:   now,
			}
			err := e.execRepo.CompensatedExecution(ctx, executionID, event)
			if err == nil {
				observability.WorkflowsFailedTotal.Inc()
			}
			return err
		}
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
		// if activityName is a compensation activity, then we shouldn't reach here because exec.Status would be COMPENSATING
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

		payload, _ := json.Marshal(models.ActivityTaskMessage{
			ExecutionID:    executionID,
			ActivityID:     nextExec.ID,
			ActivityName:   nextExec.ActivityName,
			IdempotencyKey: nextExec.IdempotencyKey,
		})
		outboxMsg := &models.OutboxMessage{
			ID:      uuid.NewString(),
			Topic:   nextExec.ActivityName,
			Payload: payload,
		}
		return e.execRepo.ScheduleNextActivity(ctx, executionID, nextExec, event, outboxMsg)
	} else {
		event := &models.WorkflowEvent{
			ID:          uuid.NewString(),
			ExecutionID: executionID,
			EventType:   models.EventWorkflowCompleted,
			Payload:     []byte(`{}`),
			Timestamp:   now,
		}
		err := e.execRepo.CompleteExecution(ctx, executionID, event)
		if err == nil {
			observability.WorkflowsCompletedTotal.Inc()
		}
		return err
	}
}

func (e *Engine) ReportActivityFailure(ctx context.Context, activityID string, executionID string, activityName string, nonRetryable bool) error {
	ctx, span := otel.Tracer("engine").Start(ctx, "ReportActivityFailure", trace.WithAttributes(
		attribute.String("executionID", executionID),
		attribute.String("activityID", activityID),
		attribute.String("activityName", activityName),
	))
	defer span.End()

	conn, err := e.execRepo.LockWorkflow(ctx, executionID)
	if err != nil {
		span.RecordError(err)
		return err
	}
	defer e.execRepo.UnlockWorkflow(ctx, conn, executionID)

	act, err := e.execRepo.GetActivityExecution(ctx, activityID)
	if err != nil {
		return fmt.Errorf("failed to get activity execution: %w", err)
	}

	if act.Status == models.ActivityStatusFailed {
		slog.Warn("Duplicate failure report ignored", "activityID", activityID)
		return nil
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
		for i, a := range wf.Activities {
			if a.Compensation == activityName {
				wfAct = &wf.Activities[i]
				break
			}
		}
	}

	if wfAct == nil {
		return fmt.Errorf("activity %s not found in workflow definition", activityName)
	}

	maxAttempts := 1
	if wfAct.RetryPolicy != nil && wfAct.RetryPolicy.MaxAttempts > 1 {
		maxAttempts = wfAct.RetryPolicy.MaxAttempts
	}

	if act.Attempt < maxAttempts && !nonRetryable {
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
			Payload:     fmt.Appendf(nil, `{"activity":"%s","attempt":%d}`, activityName, nextAttempt),
			Timestamp:   now,
		}

		payload, _ := json.Marshal(models.ActivityTaskMessage{
			ExecutionID:    executionID,
			ActivityID:     nextExec.ID,
			ActivityName:   nextExec.ActivityName,
			IdempotencyKey: nextExec.IdempotencyKey,
		})
		outboxMsg := &models.OutboxMessage{
			ID:      uuid.NewString(),
			Topic:   activityName,
			Tier:    tier,
			Payload: payload,
		}
		return e.execRepo.ScheduleNextActivity(ctx, executionID, nextExec, event, outboxMsg)
	}

	payload, _ := json.Marshal(models.ActivityTaskMessage{
		ExecutionID:    executionID,
		ActivityID:     activityID,
		ActivityName:   activityName,
		IdempotencyKey: act.IdempotencyKey,
	})
	outboxMsg := &models.OutboxMessage{
		ID:      uuid.NewString(),
		Topic:   activityName, // The publisher will use PublishDLQ for this? Wait, outbox publisher needs to know it's a DLQ message.
		Tier:    -1, // We can use tier = -1 to signify a DLQ message
		Payload: payload,
	}

	if err := e.execRepo.DeadLetterActivity(ctx, activityID, outboxMsg); err != nil {
		return err
	}

	acts, err := e.execRepo.ListActivityExecutions(ctx, executionID)
	if err != nil {
		return err
	}

	nextComp := determineNextCompensation(wf, acts)
	now := time.Now()

	if nextComp != nil && exec.Status != models.ExecutionStatusCompensating {
		// Start compensating
		nextExec := &models.ActivityExecution{
			ID:             uuid.NewString(),
			ExecutionID:    executionID,
			ActivityName:   nextComp.Name,
			Attempt:        1,
			Status:         models.ActivityStatusPending,
			IdempotencyKey: fmt.Sprintf("%s-%s-1", executionID, nextComp.Name),
		}
		event := &models.WorkflowEvent{
			ID:          uuid.NewString(),
			ExecutionID: executionID,
			EventType:   models.EventCompensationStarted,
			Payload:     []byte(`{"activity":"` + nextComp.Name + `"}`),
			Timestamp:   now,
		}
		payload, _ := json.Marshal(models.ActivityTaskMessage{
			ExecutionID:    executionID,
			ActivityID:     nextExec.ID,
			ActivityName:   nextExec.ActivityName,
			IdempotencyKey: nextExec.IdempotencyKey,
		})
		outboxMsg := &models.OutboxMessage{
			ID:      uuid.NewString(),
			Topic:   nextComp.Name,
			Payload: payload,
		}
		return e.execRepo.StartCompensating(ctx, executionID, nextExec, event, outboxMsg)
	}

	// If already compensating or no compensations, fail workflow
	event := &models.WorkflowEvent{
		ID:          uuid.NewString(),
		ExecutionID: executionID,
		EventType:   models.EventWorkflowFailed,
		Payload:     []byte(`{"activity":"` + activityName + `"}`),
		Timestamp:   now,
	}
	return e.execRepo.FailExecution(ctx, executionID, event)
}

func (e *Engine) RetryDeadLetteredActivity(ctx context.Context, activityID string) error {
	act, err := e.execRepo.GetActivityExecution(ctx, activityID)
	if err != nil {
		return err
	}

	if act.DeadLetteredAt == nil {
		return fmt.Errorf("activity is not dead-lettered")
	}

	nextAttempt := act.Attempt + 1
	nextExec := &models.ActivityExecution{
		ID:             uuid.NewString(),
		ExecutionID:    act.ExecutionID,
		ActivityName:   act.ActivityName,
		Attempt:        nextAttempt,
		Status:         models.ActivityStatusPending,
		IdempotencyKey: fmt.Sprintf("%s-%s-%d", act.ExecutionID, act.ActivityName, nextAttempt),
	}

	now := time.Now()
	event := &models.WorkflowEvent{
		ID:          uuid.NewString(),
		ExecutionID: act.ExecutionID,
		EventType:   models.EventActivityScheduled,
		Payload:     fmt.Appendf(nil, `{"activity":"%s","attempt":%d,"manual_retry":true}`, act.ActivityName, nextAttempt),
		Timestamp:   now,
	}

	payload, _ := json.Marshal(models.ActivityTaskMessage{
		ExecutionID:    act.ExecutionID,
		ActivityID:     nextExec.ID,
		ActivityName:   nextExec.ActivityName,
		IdempotencyKey: nextExec.IdempotencyKey,
	})
	outboxMsg := &models.OutboxMessage{
		ID:      uuid.NewString(),
		Topic:   act.ActivityName,
		Payload: payload,
	}
	return e.execRepo.ScheduleNextActivity(ctx, act.ExecutionID, nextExec, event, outboxMsg)
}

func (e *Engine) RetryCompensation(ctx context.Context, executionID string) error {
	ctx, span := otel.Tracer("engine").Start(ctx, "RetryCompensation", trace.WithAttributes(
		attribute.String("executionID", executionID),
	))
	defer span.End()

	conn, err := e.execRepo.LockWorkflow(ctx, executionID)
	if err != nil {
		span.RecordError(err)
		return err
	}
	defer e.execRepo.UnlockWorkflow(ctx, conn, executionID)

	exec, err := e.execRepo.GetExecution(ctx, executionID)
	if err != nil {
		return err
	}

	if exec.Status != models.ExecutionStatusFailed {
		return fmt.Errorf("execution is not FAILED, cannot retry compensation")
	}

	acts, err := e.execRepo.ListActivityExecutions(ctx, executionID)
	if err != nil {
		return err
	}

	wf, err := e.wfRepo.GetWorkflow(ctx, exec.WorkflowID)
	if err != nil {
		return err
	}

	// Find the failed compensation activity
	var failedComp *models.ActivityExecution
	for _, act := range acts {
		if act.DeadLetteredAt != nil {
			// check if it's a compensation activity
			isNormal := false
			for _, wa := range wf.Activities {
				if wa.Name == act.ActivityName {
					isNormal = true
					break
				}
			}
			if !isNormal {
				// it's a compensation activity!
				failedComp = &act
				break
			}
		}
	}

	if failedComp == nil {
		return fmt.Errorf("no failed compensation activity found for execution %s", executionID)
	}

	nextAttempt := failedComp.Attempt + 1
	nextExec := &models.ActivityExecution{
		ID:             uuid.NewString(),
		ExecutionID:    executionID,
		ActivityName:   failedComp.ActivityName,
		Attempt:        nextAttempt,
		Status:         models.ActivityStatusPending,
		IdempotencyKey: fmt.Sprintf("%s-%s-%d", executionID, failedComp.ActivityName, nextAttempt),
	}

	now := time.Now()
	event := &models.WorkflowEvent{
		ID:          uuid.NewString(),
		ExecutionID: executionID,
		EventType:   models.EventCompensationStarted,
		Payload:     fmt.Appendf(nil, `{"activity":"%s","attempt":%d,"manual_retry":true}`, failedComp.ActivityName, nextAttempt),
		Timestamp:   now,
	}

	payload, _ := json.Marshal(models.ActivityTaskMessage{
		ExecutionID:    executionID,
		ActivityID:     nextExec.ID,
		ActivityName:   nextExec.ActivityName,
		IdempotencyKey: nextExec.IdempotencyKey,
	})
	outboxMsg := &models.OutboxMessage{
		ID:      uuid.NewString(),
		Topic:   failedComp.ActivityName,
		Payload: payload,
	}
	return e.execRepo.StartCompensating(ctx, executionID, nextExec, event, outboxMsg)
}

func determineNextCompensation(wf *models.Workflow, acts []models.ActivityExecution) *models.Activity {
	var completedNormal []models.ActivityExecution
	var completedComps = make(map[string]bool)

	for _, a := range acts {
		if a.Status == models.ActivityStatusCompleted || a.Status == models.ActivityStatusFailed {
			isNormal := false
			for _, wa := range wf.Activities {
				if a.ActivityName == wa.Name {
					isNormal = true
					break
				}
			}
			if isNormal {
				if a.Status == models.ActivityStatusCompleted {
					completedNormal = append(completedNormal, a)
				}
			} else {
				completedComps[a.ActivityName] = true
			}
		}
	}

	for i := len(completedNormal) - 1; i >= 0; i-- {
		actExec := completedNormal[i]
		var wfAct *models.Activity
		for j := range wf.Activities {
			if wf.Activities[j].Name == actExec.ActivityName {
				wfAct = &wf.Activities[j]
				break
			}
		}

		if wfAct != nil && wfAct.Compensation != "" {
			if !completedComps[wfAct.Compensation] {
				return &models.Activity{
					Name:        wfAct.Compensation,
					RetryPolicy: wfAct.RetryPolicy,
					Timeout:     wfAct.Timeout,
				}
			}
		}
	}
	return nil
}
