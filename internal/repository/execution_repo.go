package repository

import (
	"context"
	"database/sql"
	"time"

	"flowscale/internal/models"
)

type ExecutionRepo struct {
	db *sql.DB
}

func NewExecutionRepo(db *sql.DB) *ExecutionRepo {
	return &ExecutionRepo{db: db}
}

func (r *ExecutionRepo) CreateExecution(ctx context.Context, exec *models.WorkflowExecution, initialEvent *models.WorkflowEvent, initialActivity *models.ActivityExecution) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.ExecContext(ctx,
		`INSERT INTO workflow_executions (id, workflow_id, status, current_activity, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		exec.ID, exec.WorkflowID, exec.Status, exec.CurrentActivity, exec.CreatedAt, exec.UpdatedAt,
	)
	if err != nil {
		return err
	}

	_, err = tx.ExecContext(ctx,
		`INSERT INTO workflow_events (id, execution_id, event_type, payload, timestamp)
		 VALUES ($1, $2, $3, $4, $5)`,
		initialEvent.ID, initialEvent.ExecutionID, initialEvent.EventType, initialEvent.Payload, initialEvent.Timestamp,
	)
	if err != nil {
		return err
	}

	if initialActivity != nil {
		_, err = tx.ExecContext(ctx,
			`INSERT INTO activity_executions (id, execution_id, activity_name, attempt, status, idempotency_key)
			 VALUES ($1, $2, $3, $4, $5, $6)`,
			initialActivity.ID, initialActivity.ExecutionID, initialActivity.ActivityName,
			initialActivity.Attempt, initialActivity.Status, initialActivity.IdempotencyKey,
		)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (r *ExecutionRepo) GetExecution(ctx context.Context, id string) (*models.WorkflowExecution, error) {
	var exec models.WorkflowExecution
	err := r.db.QueryRowContext(ctx,
		"SELECT id, workflow_id, status, current_activity, created_at, updated_at FROM workflow_executions WHERE id = $1", id,
	).Scan(&exec.ID, &exec.WorkflowID, &exec.Status, &exec.CurrentActivity, &exec.CreatedAt, &exec.UpdatedAt)

	if err != nil {
		return nil, err
	}
	return &exec, nil
}

func (r *ExecutionRepo) ListExecutions(ctx context.Context) ([]models.WorkflowExecution, error) {
	rows, err := r.db.QueryContext(ctx,
		"SELECT id, workflow_id, status, current_activity, created_at, updated_at FROM workflow_executions ORDER BY created_at DESC",
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var execs []models.WorkflowExecution
	for rows.Next() {
		var exec models.WorkflowExecution
		var curActivity sql.NullString
		if err := rows.Scan(&exec.ID, &exec.WorkflowID, &exec.Status, &curActivity, &exec.CreatedAt, &exec.UpdatedAt); err != nil {
			return nil, err
		}
		if curActivity.Valid {
			exec.CurrentActivity = curActivity.String
		}
		execs = append(execs, exec)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return execs, nil
}

func (r *ExecutionRepo) GetExecutionEvents(ctx context.Context, executionID string) ([]models.WorkflowEvent, error) {
	rows, err := r.db.QueryContext(ctx,
		"SELECT id, execution_id, event_type, payload, timestamp FROM workflow_events WHERE execution_id = $1 ORDER BY timestamp ASC",
		executionID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []models.WorkflowEvent
	for rows.Next() {
		var event models.WorkflowEvent
		var payload []byte
		if err := rows.Scan(&event.ID, &event.ExecutionID, &event.EventType, &payload, &event.Timestamp); err != nil {
			return nil, err
		}
		event.Payload = payload
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return events, nil
}

func (r *ExecutionRepo) GetActivityExecution(ctx context.Context, activityID string) (*models.ActivityExecution, error) {
	var act models.ActivityExecution
	err := r.db.QueryRowContext(ctx,
		"SELECT id, execution_id, activity_name, attempt, status, idempotency_key, started_at, completed_at, dead_lettered_at FROM activity_executions WHERE id = $1",
		activityID,
	).Scan(&act.ID, &act.ExecutionID, &act.ActivityName, &act.Attempt, &act.Status, &act.IdempotencyKey, &act.StartedAt, &act.CompletedAt, &act.DeadLetteredAt)
	if err != nil {
		return nil, err
	}
	return &act, nil
}

func (r *ExecutionRepo) CompleteActivity(ctx context.Context, activityID string) error {
	_, err := r.db.ExecContext(ctx,
		"UPDATE activity_executions SET status = $1, completed_at = $2 WHERE id = $3",
		models.ActivityStatusCompleted, time.Now(), activityID,
	)
	return err
}

func (r *ExecutionRepo) FailActivity(ctx context.Context, activityID string) error {
	_, err := r.db.ExecContext(ctx,
		"UPDATE activity_executions SET status = $1 WHERE id = $2",
		models.ActivityStatusFailed, activityID,
	)
	return err
}

func (r *ExecutionRepo) DeadLetterActivity(ctx context.Context, activityID string) error {
	_, err := r.db.ExecContext(ctx,
		"UPDATE activity_executions SET status = $1, dead_lettered_at = $2 WHERE id = $3",
		models.ActivityStatusFailed, time.Now(), activityID,
	)
	return err
}

func (r *ExecutionRepo) ListDeadLetteredActivities(ctx context.Context) ([]models.ActivityExecution, error) {
	rows, err := r.db.QueryContext(ctx,
		"SELECT id, execution_id, activity_name, attempt, status, idempotency_key, started_at, completed_at, dead_lettered_at FROM activity_executions WHERE dead_lettered_at IS NOT NULL ORDER BY dead_lettered_at DESC",
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var acts []models.ActivityExecution
	for rows.Next() {
		var act models.ActivityExecution
		if err := rows.Scan(&act.ID, &act.ExecutionID, &act.ActivityName, &act.Attempt, &act.Status, &act.IdempotencyKey, &act.StartedAt, &act.CompletedAt, &act.DeadLetteredAt); err != nil {
			return nil, err
		}
		acts = append(acts, act)
	}
	return acts, rows.Err()
}

func (r *ExecutionRepo) ScheduleNextActivity(ctx context.Context, executionID string, nextActivity *models.ActivityExecution, event *models.WorkflowEvent) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.ExecContext(ctx,
		`UPDATE workflow_executions SET current_activity = $1, updated_at = $2 WHERE id = $3`,
		nextActivity.ActivityName, time.Now(), executionID,
	)
	if err != nil {
		return err
	}

	_, err = tx.ExecContext(ctx,
		`INSERT INTO activity_executions (id, execution_id, activity_name, attempt, status, idempotency_key)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		nextActivity.ID, nextActivity.ExecutionID, nextActivity.ActivityName,
		nextActivity.Attempt, nextActivity.Status, nextActivity.IdempotencyKey,
	)
	if err != nil {
		return err
	}

	_, err = tx.ExecContext(ctx,
		`INSERT INTO workflow_events (id, execution_id, event_type, payload, timestamp)
		 VALUES ($1, $2, $3, $4, $5)`,
		event.ID, event.ExecutionID, event.EventType, event.Payload, event.Timestamp,
	)
	if err != nil {
		return err
	}

	return tx.Commit()
}

func (r *ExecutionRepo) CompleteExecution(ctx context.Context, executionID string, event *models.WorkflowEvent) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.ExecContext(ctx,
		`UPDATE workflow_executions SET status = $1, updated_at = $2 WHERE id = $3`,
		models.ExecutionStatusCompleted, time.Now(), executionID,
	)
	if err != nil {
		return err
	}

	_, err = tx.ExecContext(ctx,
		`INSERT INTO workflow_events (id, execution_id, event_type, payload, timestamp)
		 VALUES ($1, $2, $3, $4, $5)`,
		event.ID, event.ExecutionID, event.EventType, event.Payload, event.Timestamp,
	)
	if err != nil {
		return err
	}

	return tx.Commit()
}

func (r *ExecutionRepo) FailExecution(ctx context.Context, executionID string, event *models.WorkflowEvent) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.ExecContext(ctx,
		`UPDATE workflow_executions SET status = $1, updated_at = $2 WHERE id = $3`,
		models.ExecutionStatusFailed, time.Now(), executionID,
	)
	if err != nil {
		return err
	}

	_, err = tx.ExecContext(ctx,
		`INSERT INTO workflow_events (id, execution_id, event_type, payload, timestamp)
		 VALUES ($1, $2, $3, $4, $5)`,
		event.ID, event.ExecutionID, event.EventType, event.Payload, event.Timestamp,
	)
	if err != nil {
		return err
	}

	return tx.Commit()
}
