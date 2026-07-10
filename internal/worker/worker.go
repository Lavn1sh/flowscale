package worker

import (
	"context"
	"database/sql"
	"log/slog"
	"time"

	"flowscale/internal/engine"
)

type ActivityContext struct {
	context.Context
	ExecutionID string
	ActivityID  string
}

type ActivityFunc func(ctx ActivityContext) error

type Worker struct {
	engine     *engine.Engine
	activities map[string]ActivityFunc
}

func NewWorker(eng *engine.Engine) *Worker {
	return &Worker{
		engine:     eng,
		activities: make(map[string]ActivityFunc),
	}
}

func (w *Worker) RegisterActivity(name string, fn ActivityFunc) {
	w.activities[name] = fn
}

func (w *Worker) Start(ctx context.Context) {
	slog.Info("Worker started local polling loop")
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			act, err := w.engine.PollPendingActivity(ctx)
			if err != nil {
				if err != sql.ErrNoRows {
					slog.Error("Failed to poll activity", "err", err)
				}
				continue
			}

			slog.Info("Polled activity", "name", act.ActivityName, "id", act.ID)

			fn, ok := w.activities[act.ActivityName]
			if !ok {
				slog.Error("Activity not registered", "name", act.ActivityName)
				w.engine.ReportActivityFailure(ctx, act.ID, act.ExecutionID, act.ActivityName)
				continue
			}

			actCtx := ActivityContext{
				Context:     ctx,
				ExecutionID: act.ExecutionID,
				ActivityID:  act.ID,
			}

			err = fn(actCtx)
			if err != nil {
				slog.Error("Activity failed", "name", act.ActivityName, "err", err)
				w.engine.ReportActivityFailure(ctx, act.ID, act.ExecutionID, act.ActivityName)
			} else {
				slog.Info("Activity succeeded", "name", act.ActivityName)
				w.engine.ReportActivitySuccess(ctx, act.ID, act.ExecutionID, act.ActivityName)
			}
		}
	}
}
