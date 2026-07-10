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
