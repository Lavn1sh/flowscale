package engine

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"flowscale/internal/models"
)

type Reaper struct {
	engine *Engine
}

func NewReaper(engine *Engine) *Reaper {
	return &Reaper{engine: engine}
}

// Start runs the reaper process in the background. It consumes heartbeats to update the DB,
// and periodically polls the DB for timed-out activities.
func (r *Reaper) Start(ctx context.Context) {
	// 1. Consume heartbeats from RabbitMQ
	go r.consumeHeartbeats(ctx)

	// 2. Periodically scan for timed out activities
	go r.scanForTimeouts(ctx)
}

func (r *Reaper) consumeHeartbeats(ctx context.Context) {
	slog.Info("Reaper started RabbitMQ heartbeat consumer")
	deliveries, err := r.engine.mq.ConsumeHeartbeats()
	if err != nil {
		slog.Error("Failed to start heartbeat consumer", "err", err)
		return
	}

	for {
		select {
		case <-ctx.Done():
			return
		case d, ok := <-deliveries:
			if !ok {
				return
			}
			var payload map[string]string
			if err := json.Unmarshal(d.Body, &payload); err != nil {
				slog.Error("Failed to unmarshal heartbeat", "err", err)
				d.Nack(false, false)
				continue
			}

			activityID := payload["activity_id"]
			if activityID != "" {
				err := r.engine.execRepo.RecordHeartbeat(ctx, activityID)
				if err != nil {
					slog.Warn("Failed to record heartbeat", "activityID", activityID, "err", err)
				}
			}
			d.Ack(false)
		}
	}
}

func (r *Reaper) scanForTimeouts(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	// Hardcoded 5 minute timeout threshold
	timeoutDuration := 5 * time.Minute

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// We only want one engine instance doing the timeout reaping to avoid race conditions.
			// Re-use the scheduler's leader lock logic or just try advisory lock for reaping.
			// Let's use a specific advisory lock key for the reaper.
			r.reapTimeouts(ctx, timeoutDuration)
		}
	}
}

func (r *Reaper) reapTimeouts(ctx context.Context, timeoutDuration time.Duration) {
	conn, err := r.engine.wfRepo.DB().Conn(ctx)
	if err != nil {
		slog.Error("Reaper failed to get db connection", "err", err)
		return
	}
	defer conn.Close()

	// Advisory lock key for reaper = 2000
	var acquired bool
	err = conn.QueryRowContext(ctx, "SELECT pg_try_advisory_lock(2000)").Scan(&acquired)
	if err != nil || !acquired {
		// Not the leader or failed to acquire
		return
	}
	defer conn.ExecContext(ctx, "SELECT pg_advisory_unlock(2000)")

	// Find PENDING activities whose last_heartbeat_at is older than timeoutDuration.
	// If last_heartbeat_at is NULL, we fallback to comparing created_at (since it was just scheduled).
	// Wait, we don't have created_at in activity_executions?
	// Let's check schema.
	// activity_executions has no created_at in schema. It might be easier to just say last_heartbeat_at.
	// Actually, if an activity is scheduled, we should probably set last_heartbeat_at = NOW() upon scheduling.
	// Let's update `execution_repo.go` to insert last_heartbeat_at = NOW() when creating ActivityExecution.
	
	thresholdTime := time.Now().Add(-timeoutDuration)
	
	rows, err := conn.QueryContext(ctx, `
		SELECT id, execution_id, activity_name 
		FROM activity_executions 
		WHERE status = $1 AND last_heartbeat_at < $2
	`, models.ActivityStatusPending, thresholdTime)
	if err != nil {
		slog.Error("Reaper failed to query timeouts", "err", err)
		return
	}
	defer rows.Close()

	type timeoutAct struct {
		ID string
		ExecID string
		Name string
	}
	var timedOut []timeoutAct

	for rows.Next() {
		var act timeoutAct
		if err := rows.Scan(&act.ID, &act.ExecID, &act.Name); err == nil {
			timedOut = append(timedOut, act)
		}
	}
	rows.Close()

	for _, act := range timedOut {
		slog.Info("Reaper detected activity timeout", "activity_id", act.ID, "execution_id", act.ExecID)
		
		// Mark as failed. ReportActivityFailure takes care of locking and state transitions.
		// We can inject a specific error or just let it fail naturally.
		// Wait, ReportActivityFailure requires us to know if it's non-retryable?
		// We can just call ReportActivityFailure which handles standard retry logic.
		err := r.engine.ReportActivityFailure(ctx, act.ID, act.ExecID, act.Name, false)
		if err != nil {
			slog.Error("Reaper failed to report activity failure", "activity_id", act.ID, "err", err)
		}
	}
}
