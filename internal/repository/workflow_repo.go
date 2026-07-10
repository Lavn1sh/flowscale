package repository

import (
	"context"
	"database/sql"
	"flowscale/internal/models"
	"fmt"
	_ "github.com/lib/pq"
	"time"
)

type WorkflowRepo struct {
	db *sql.DB
}

func NewWorkflowRepo(db *sql.DB) *WorkflowRepo {
	return &WorkflowRepo{db: db}
}

func (r *WorkflowRepo) DB() *sql.DB {
	return r.db
}

func (r *WorkflowRepo) CreateWorkflow(ctx context.Context, wf *models.Workflow) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Insert Workflow
	_, err = tx.ExecContext(ctx,
		"INSERT INTO workflows (id, name) VALUES ($1, $2)",
		wf.ID, wf.Name,
	)
	if err != nil {
		return fmt.Errorf("failed to insert workflow: %w", err)
	}

	// Insert Activities
	for i, act := range wf.Activities {
		var maxAttempts sql.NullInt64
		var backoffStrategy sql.NullString

		if act.RetryPolicy != nil {
			maxAttempts.Int64 = int64(act.RetryPolicy.MaxAttempts)
			maxAttempts.Valid = true
			backoffStrategy.String = act.RetryPolicy.BackoffStrategy
			backoffStrategy.Valid = true
		}

		var compActivity sql.NullString
		if act.Compensation != "" {
			compActivity.String = act.Compensation
			compActivity.Valid = true
		}

		var timeout sql.NullString
		if act.Timeout != "" {
			timeout.String = act.Timeout
			timeout.Valid = true
		}

		actID := fmt.Sprintf("%s-act-%d", wf.ID, i) // simplistic ID for now

		_, err = tx.ExecContext(ctx,
			`INSERT INTO activities (
				id, workflow_id, name, compensation_activity_name, 
				retry_max_attempts, retry_backoff_strategy, timeout, position
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
			actID, wf.ID, act.Name, compActivity,
			maxAttempts, backoffStrategy, timeout, i,
		)
		if err != nil {
			return fmt.Errorf("failed to insert activity %s: %w", act.Name, err)
		}
	}

	return tx.Commit()
}

func (r *WorkflowRepo) GetWorkflow(ctx context.Context, id string) (*models.Workflow, error) {
	var wf models.Workflow
	err := r.db.QueryRowContext(ctx, "SELECT id, name FROM workflows WHERE id = $1", id).Scan(&wf.ID, &wf.Name)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("workflow not found")
		}
		return nil, err
	}

	rows, err := r.db.QueryContext(ctx,
		`SELECT name, compensation_activity_name, retry_max_attempts, retry_backoff_strategy, timeout 
		 FROM activities WHERE workflow_id = $1 ORDER BY position`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var act models.Activity
		var comp, backoff, timeout sql.NullString
		var maxAttempts sql.NullInt64

		if err := rows.Scan(&act.Name, &comp, &maxAttempts, &backoff, &timeout); err != nil {
			return nil, err
		}

		if comp.Valid {
			act.Compensation = comp.String
		}
		if timeout.Valid {
			act.Timeout = timeout.String
		}
		if maxAttempts.Valid {
			act.RetryPolicy = &models.RetryPolicy{
				MaxAttempts:     int(maxAttempts.Int64),
				BackoffStrategy: backoff.String,
			}
		}
		wf.Activities = append(wf.Activities, act)
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	return &wf, nil
}

func (r *WorkflowRepo) ListWorkflows(ctx context.Context, limit, offset int) ([]*models.Workflow, error) {
	query := `SELECT id, name FROM workflows ORDER BY created_at DESC LIMIT $1 OFFSET $2`
	rows, err := r.db.QueryContext(ctx, query, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var wfs []*models.Workflow
	for rows.Next() {
		var wf models.Workflow
		if err := rows.Scan(&wf.ID, &wf.Name); err != nil {
			return nil, err
		}
		wfs = append(wfs, &wf)
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	return wfs, nil
}

func (r *WorkflowRepo) DeleteWorkflow(ctx context.Context, id string) error {
	res, err := r.db.ExecContext(ctx, "DELETE FROM workflows WHERE id = $1", id)
	if err != nil {
		return err
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return fmt.Errorf("workflow not found")
	}
	return nil
}

func (r *WorkflowRepo) CreateSchedule(ctx context.Context, schedule *models.Schedule) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO scheduled_workflows (id, workflow_id, schedule_type, run_at, cron_expression, interval, next_run_at, status, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
		schedule.ID, schedule.WorkflowID, schedule.ScheduleType, schedule.RunAt, schedule.CronExpression, schedule.Interval, schedule.NextRunAt, schedule.Status, schedule.CreatedAt, schedule.UpdatedAt,
	)
	return err
}

func (r *WorkflowRepo) ListSchedules(ctx context.Context) ([]models.Schedule, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, workflow_id, schedule_type, run_at, cron_expression, interval, next_run_at, status, created_at, updated_at FROM scheduled_workflows ORDER BY created_at DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var schedules []models.Schedule
	for rows.Next() {
		var s models.Schedule
		var runAt sql.NullTime
		var cronExpr, interval sql.NullString
		if err := rows.Scan(&s.ID, &s.WorkflowID, &s.ScheduleType, &runAt, &cronExpr, &interval, &s.NextRunAt, &s.Status, &s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, err
		}
		if runAt.Valid {
			s.RunAt = &runAt.Time
		}
		if cronExpr.Valid {
			s.CronExpression = cronExpr.String
		}
		if interval.Valid {
			s.Interval = interval.String
		}
		schedules = append(schedules, s)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return schedules, nil
}

func (r *WorkflowRepo) DeleteSchedule(ctx context.Context, id string) error {
	res, err := r.db.ExecContext(ctx, "DELETE FROM scheduled_workflows WHERE id = $1", id)
	if err != nil {
		return err
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return fmt.Errorf("schedule not found")
	}
	return nil
}

func (r *WorkflowRepo) GetDueSchedules(ctx context.Context, now time.Time) ([]models.Schedule, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, workflow_id, schedule_type, run_at, cron_expression, interval, next_run_at, status, created_at, updated_at 
		 FROM scheduled_workflows 
		 WHERE next_run_at <= $1 AND status = 'ACTIVE'`,
		now,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var schedules []models.Schedule
	for rows.Next() {
		var s models.Schedule
		var runAt sql.NullTime
		var cronExpr, interval sql.NullString
		if err := rows.Scan(&s.ID, &s.WorkflowID, &s.ScheduleType, &runAt, &cronExpr, &interval, &s.NextRunAt, &s.Status, &s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, err
		}
		if runAt.Valid {
			s.RunAt = &runAt.Time
		}
		if cronExpr.Valid {
			s.CronExpression = cronExpr.String
		}
		if interval.Valid {
			s.Interval = interval.String
		}
		schedules = append(schedules, s)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return schedules, nil
}

func (r *WorkflowRepo) UpdateScheduleState(ctx context.Context, id string, nextRunAt *time.Time, status string) error {
	if nextRunAt != nil {
		_, err := r.db.ExecContext(ctx,
			"UPDATE scheduled_workflows SET next_run_at = $1, status = $2, updated_at = $3 WHERE id = $4",
			*nextRunAt, status, time.Now(), id,
		)
		return err
	}

	_, err := r.db.ExecContext(ctx,
		"UPDATE scheduled_workflows SET status = $1, updated_at = $2 WHERE id = $3",
		status, time.Now(), id,
	)
	return err
}
