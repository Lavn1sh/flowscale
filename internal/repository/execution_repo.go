package repository

import (
	"context"
	"database/sql"
	"fmt"
	"hash/fnv"
	"strings"
	"time"

	"flowscale/internal/models"
)

type ExecutionRepo struct {
	db *sql.DB
}

func NewExecutionRepo(db *sql.DB) *ExecutionRepo {
	return &ExecutionRepo{db: db}
}

// LockWorkflow acquires a session-level advisory lock for the given workflow execution ID.
func (r *ExecutionRepo) LockWorkflow(ctx context.Context, executionID string) (*sql.Conn, error) {
	conn, err := r.db.Conn(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get connection for lock: %w", err)
	}

	h := fnv.New64a()
	h.Write([]byte(executionID))
	lockKey := int64(h.Sum64())

	_, err = conn.ExecContext(ctx, "SELECT pg_advisory_lock($1)", lockKey)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to acquire advisory lock: %w", err)
	}

	return conn, nil
}

// UnlockWorkflow releases the session-level advisory lock for the workflow execution ID and closes the connection.
func (r *ExecutionRepo) UnlockWorkflow(ctx context.Context, conn *sql.Conn, executionID string) error {
	if conn == nil {
		return nil
	}
	defer conn.Close()

	h := fnv.New64a()
	h.Write([]byte(executionID))
	lockKey := int64(h.Sum64())

	_, err := conn.ExecContext(ctx, "SELECT pg_advisory_unlock($1)", lockKey)
	if err != nil {
		return fmt.Errorf("failed to release advisory lock: %w", err)
	}
	return nil
}

// RecordHeartbeat updates the last_heartbeat_at timestamp for a specific activity execution.
func (r *ExecutionRepo) RecordHeartbeat(ctx context.Context, activityID string) error {
	res, err := r.db.ExecContext(ctx,
		"UPDATE activity_executions SET last_heartbeat_at = $1 WHERE id = $2 AND status = $3",
		time.Now(), activityID, models.ActivityStatusPending,
	)
	if err != nil {
		return fmt.Errorf("failed to update heartbeat: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return fmt.Errorf("activity execution not found or not in pending state")
	}
	return nil
}

func (r *ExecutionRepo) CreateExecution(ctx context.Context, exec *models.WorkflowExecution, initialEvent *models.WorkflowEvent, initialActivity *models.ActivityExecution, outboxMsg *models.OutboxMessage) error {
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
			`INSERT INTO activity_executions (id, execution_id, activity_name, attempt, status, idempotency_key, last_heartbeat_at)
			 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			initialActivity.ID, initialActivity.ExecutionID, initialActivity.ActivityName,
			initialActivity.Attempt, initialActivity.Status, initialActivity.IdempotencyKey, time.Now(),
		)
		if err != nil {
			return err
		}
	}

	if outboxMsg != nil {
		if err := r.InsertOutboxMessage(ctx, tx, *outboxMsg); err != nil {
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

func (r *ExecutionRepo) ListExecutions(ctx context.Context, status, workflowID, timeRange string, limit, offset int) ([]models.WorkflowExecution, error) {
	query := "SELECT id, workflow_id, status, current_activity, created_at, updated_at FROM workflow_executions"
	var conditions []string
	var args []interface{}
	argID := 1

	if status != "" {
		conditions = append(conditions, fmt.Sprintf("status = $%d", argID))
		args = append(args, status)
		argID++
	}

	if workflowID != "" {
		conditions = append(conditions, fmt.Sprintf("workflow_id = $%d", argID))
		args = append(args, workflowID)
		argID++
	}

	if timeRange != "" {
		var d time.Duration
		switch timeRange {
		case "1h":
			d = -1 * time.Hour
		case "24h":
			d = -24 * time.Hour
		case "7d":
			d = -7 * 24 * time.Hour
		}
		if d != 0 {
			conditions = append(conditions, fmt.Sprintf("created_at >= $%d", argID))
			args = append(args, time.Now().Add(d))
			argID++
		}
	}

	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}

	query += " ORDER BY created_at DESC"
	
	if limit > 0 {
		query += fmt.Sprintf(" LIMIT $%d", argID)
		args = append(args, limit)
		argID++
	}
	if offset > 0 {
		query += fmt.Sprintf(" OFFSET $%d", argID)
		args = append(args, offset)
		argID++
	}

	rows, err := r.db.QueryContext(ctx, query, args...)
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

func (r *ExecutionRepo) DeadLetterActivity(ctx context.Context, activityID string, outboxMsg *models.OutboxMessage) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.ExecContext(ctx,
		"UPDATE activity_executions SET status = $1, dead_lettered_at = $2 WHERE id = $3",
		models.ActivityStatusFailed, time.Now(), activityID,
	)
	if err != nil {
		return err
	}

	if outboxMsg != nil {
		if err := r.InsertOutboxMessage(ctx, tx, *outboxMsg); err != nil {
			return err
		}
	}

	return tx.Commit()
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

func (r *ExecutionRepo) RetryActivity(ctx context.Context, executionID string, nextActivity *models.ActivityExecution, event *models.WorkflowEvent, outboxMsg *models.OutboxMessage) error {
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
		`INSERT INTO activity_executions (id, execution_id, activity_name, attempt, status, idempotency_key, last_heartbeat_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		nextActivity.ID, nextActivity.ExecutionID, nextActivity.ActivityName,
		nextActivity.Attempt, nextActivity.Status, nextActivity.IdempotencyKey, time.Now(),
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

	if outboxMsg != nil {
		if err := r.InsertOutboxMessage(ctx, tx, *outboxMsg); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (r *ExecutionRepo) ScheduleNextActivity(ctx context.Context, executionID string, nextActivity *models.ActivityExecution, event *models.WorkflowEvent, outboxMsg *models.OutboxMessage) error {
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
		`INSERT INTO activity_executions (id, execution_id, activity_name, attempt, status, idempotency_key, last_heartbeat_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		nextActivity.ID, nextActivity.ExecutionID, nextActivity.ActivityName,
		nextActivity.Attempt, nextActivity.Status, nextActivity.IdempotencyKey, time.Now(),
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

	if outboxMsg != nil {
		if err := r.InsertOutboxMessage(ctx, tx, *outboxMsg); err != nil {
			return err
		}
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

func (r *ExecutionRepo) CancelExecution(ctx context.Context, executionID string, event *models.WorkflowEvent) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.ExecContext(ctx,
		`UPDATE workflow_executions SET status = $1, updated_at = $2 WHERE id = $3 AND status NOT IN ($4, $5, $6)`,
		models.ExecutionStatusCancelled, time.Now(), executionID,
		models.ExecutionStatusCompleted, models.ExecutionStatusFailed, models.ExecutionStatusCancelled,
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

func (r *ExecutionRepo) ListActivityExecutions(ctx context.Context, executionID string) ([]models.ActivityExecution, error) {
	rows, err := r.db.QueryContext(ctx,
		"SELECT id, execution_id, activity_name, attempt, status, idempotency_key, started_at, completed_at, dead_lettered_at FROM activity_executions WHERE execution_id = $1 ORDER BY completed_at ASC NULLS LAST",
		executionID,
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
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return acts, nil
}

func (r *ExecutionRepo) StartCompensating(ctx context.Context, executionID string, nextActivity *models.ActivityExecution, event *models.WorkflowEvent, outboxMsg *models.OutboxMessage) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.ExecContext(ctx,
		`UPDATE workflow_executions SET status = $1, current_activity = $2, updated_at = $3 WHERE id = $4`,
		models.ExecutionStatusCompensating, nextActivity.ActivityName, time.Now(), executionID,
	)
	if err != nil {
		return err
	}

	_, err = tx.ExecContext(ctx,
		`INSERT INTO activity_executions (id, execution_id, activity_name, attempt, status, idempotency_key, last_heartbeat_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		nextActivity.ID, nextActivity.ExecutionID, nextActivity.ActivityName,
		nextActivity.Attempt, nextActivity.Status, nextActivity.IdempotencyKey, time.Now(),
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

	if outboxMsg != nil {
		if err := r.InsertOutboxMessage(ctx, tx, *outboxMsg); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (r *ExecutionRepo) CompensatedExecution(ctx context.Context, executionID string, event *models.WorkflowEvent) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.ExecContext(ctx,
		`UPDATE workflow_executions SET status = $1, updated_at = $2 WHERE id = $3`,
		models.ExecutionStatusCompensated, time.Now(), executionID,
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

func (r *ExecutionRepo) InsertOutboxMessage(ctx context.Context, tx *sql.Tx, msg models.OutboxMessage) error {
	_, err := tx.ExecContext(ctx,
		"INSERT INTO outbox_messages (id, topic, tier, payload) VALUES ($1, $2, $3, $4)",
		msg.ID, msg.Topic, msg.Tier, msg.Payload,
	)
	return err
}

func (r *ExecutionRepo) GetPendingOutboxMessages(ctx context.Context, limit int) ([]models.OutboxMessage, error) {
	rows, err := r.db.QueryContext(ctx,
		"SELECT id, topic, tier, payload, created_at FROM outbox_messages ORDER BY created_at ASC LIMIT $1",
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var msgs []models.OutboxMessage
	for rows.Next() {
		var msg models.OutboxMessage
		if err := rows.Scan(&msg.ID, &msg.Topic, &msg.Tier, &msg.Payload, &msg.CreatedAt); err != nil {
			return nil, err
		}
		msgs = append(msgs, msg)
	}
	return msgs, rows.Err()
}

func (r *ExecutionRepo) DeleteOutboxMessage(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, "DELETE FROM outbox_messages WHERE id = $1", id)
	return err
}

func (r *ExecutionRepo) DeleteExecution(ctx context.Context, id string) error {
	res, err := r.db.ExecContext(ctx, "DELETE FROM workflow_executions WHERE id = $1", id)
	if err != nil {
		return err
	}
	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return fmt.Errorf("execution not found")
	}
	return nil
}

