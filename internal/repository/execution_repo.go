package repository

import (
	"context"
	"database/sql"
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

	// Insert execution
	_, err = tx.ExecContext(ctx, 
		`INSERT INTO workflow_executions (id, workflow_id, status, current_activity, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		exec.ID, exec.WorkflowID, exec.Status, exec.CurrentActivity, exec.CreatedAt, exec.UpdatedAt,
	)
	if err != nil {
		return err
	}

	// Insert initial event
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
