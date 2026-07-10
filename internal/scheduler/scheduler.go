package scheduler

import (
	"context"
	"flowscale/internal/engine"
	"flowscale/internal/models"
	"flowscale/internal/repository"
	"log/slog"
	"time"

	"github.com/robfig/cron/v3"
)

type Scheduler struct {
	repo   *repository.WorkflowRepo
	engine *engine.Engine
	ticker *time.Ticker
	quit   chan struct{}
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
