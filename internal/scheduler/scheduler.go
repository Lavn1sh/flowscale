package scheduler

import (
	"context"
	"database/sql"
	"flowscale/internal/engine"
	"flowscale/internal/models"
	"flowscale/internal/repository"
	"log/slog"
	"time"

	"github.com/robfig/cron/v3"
)

const schedulerLockKey = 1000

type Scheduler struct {
	repo       *repository.WorkflowRepo
	engine     *engine.Engine
	ticker     *time.Ticker
	quit       chan struct{}
	leaderConn *sql.Conn
}

func NewScheduler(repo *repository.WorkflowRepo, e *engine.Engine) *Scheduler {
	return &Scheduler{
		repo:   repo,
		engine: e,
		quit:   make(chan struct{}),
	}
}

func (s *Scheduler) Start() {
	slog.Info("Starting Scheduler service")
	s.ticker = time.NewTicker(5 * time.Second)
	go s.loop()
}

func (s *Scheduler) Stop() {
	if s.ticker != nil {
		s.ticker.Stop()
	}
	close(s.quit)
	s.releaseLock()
}

func (s *Scheduler) releaseLock() {
	if s.leaderConn != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_, _ = s.leaderConn.ExecContext(ctx, "SELECT pg_advisory_unlock($1)", schedulerLockKey)
		s.leaderConn.Close()
		s.leaderConn = nil
		slog.Info("Scheduler released leader lock")
	}
}

func (s *Scheduler) tryAcquireLock() bool {
	ctx := context.Background()
	if s.leaderConn != nil {
		// Ping to ensure connection is still alive
		if err := s.leaderConn.PingContext(ctx); err != nil {
			slog.Warn("Scheduler lost db connection, dropping leadership", "err", err)
			s.leaderConn.Close()
			s.leaderConn = nil
			return false
		}
		return true // Already holding lock
	}

	conn, err := s.repo.DB().Conn(ctx)
	if err != nil {
		slog.Error("Scheduler failed to get db connection for lock", "err", err)
		return false
	}

	var acquired bool
	err = conn.QueryRowContext(ctx, "SELECT pg_try_advisory_lock($1)", schedulerLockKey).Scan(&acquired)
	if err != nil {
		slog.Error("Scheduler failed to acquire advisory lock", "err", err)
		conn.Close()
		return false
	}

	if acquired {
		slog.Info("Scheduler acquired leader lock, acting as leader")
		s.leaderConn = conn
		return true
	}

	conn.Close()
	return false
}

func (s *Scheduler) loop() {
	for {
		select {
		case <-s.ticker.C:
			s.poll()
		case <-s.quit:
			return
		}
	}
}

func (s *Scheduler) poll() {
	if !s.tryAcquireLock() {
		return // Not the leader, skip polling
	}

	now := time.Now()
	schedules, err := s.repo.GetDueSchedules(context.Background(), now)
	if err != nil {
		slog.Error("failed to poll schedules", "err", err)
		return
	}

	for _, sched := range schedules {
		slog.Info("executing due schedule", "schedule_id", sched.ID, "workflow_id", sched.WorkflowID)

		// Enqueue the workflow
		_, err := s.engine.StartWorkflow(context.Background(), sched.WorkflowID)
		if err != nil {
			slog.Error("failed to start workflow for schedule", "schedule_id", sched.ID, "err", err)
			continue
		}

		// Calculate next run
		s.advanceSchedule(sched, now)
	}
}

func (s *Scheduler) advanceSchedule(sched models.Schedule, now time.Time) {
	if sched.ScheduleType == models.ScheduleTypeOnce || sched.ScheduleType == models.ScheduleTypeDelayed {
		err := s.repo.UpdateScheduleState(context.Background(), sched.ID, nil, models.ScheduleStatusFinished)
		if err != nil {
			slog.Error("failed to finish schedule", "schedule_id", sched.ID, "err", err)
		}
		return
	}

	if sched.ScheduleType == models.ScheduleTypeRecurring {
		var nextRunAt time.Time
		if sched.CronExpression != "" {
			parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)
			schedule, err := parser.Parse(sched.CronExpression)
			if err != nil {
				slog.Error("invalid cron expression", "schedule_id", sched.ID, "err", err)
				s.repo.UpdateScheduleState(context.Background(), sched.ID, nil, models.ScheduleStatusFinished)
				return
			}
			nextRunAt = schedule.Next(now)
		} else if sched.Interval != "" {
			duration, err := time.ParseDuration(sched.Interval)
			if err != nil {
				slog.Error("invalid interval", "schedule_id", sched.ID, "err", err)
				s.repo.UpdateScheduleState(context.Background(), sched.ID, nil, models.ScheduleStatusFinished)
				return
			}
			nextRunAt = now.Add(duration)
		} else {
			// fallback
			s.repo.UpdateScheduleState(context.Background(), sched.ID, nil, models.ScheduleStatusFinished)
			return
		}

		err := s.repo.UpdateScheduleState(context.Background(), sched.ID, &nextRunAt, models.ScheduleStatusActive)
		if err != nil {
			slog.Error("failed to update schedule next_run_at", "schedule_id", sched.ID, "err", err)
		}
	}
}
