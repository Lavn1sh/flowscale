package worker

import (
	"context"
	"encoding/json"
	"log/slog"

	"flowscale/internal/models"
	"flowscale/internal/queue"
	"flowscale/internal/repository"
	"github.com/rabbitmq/amqp091-go"
)

type ActivityContext struct {
	context.Context
	ExecutionID string
	ActivityID  string
}

type ActivityFunc func(ctx ActivityContext) error

type Worker struct {
	mq         *queue.RabbitMQ
	execRepo   *repository.ExecutionRepo
	activities map[string]ActivityFunc
}

func NewWorker(mq *queue.RabbitMQ, execRepo *repository.ExecutionRepo) *Worker {
	return &Worker{
		mq:         mq,
		execRepo:   execRepo,
		activities: make(map[string]ActivityFunc),
	}
}

func (w *Worker) RegisterActivity(name string, fn ActivityFunc) {
	w.activities[name] = fn
}

func (w *Worker) Start(ctx context.Context) {
	slog.Info("Worker started RabbitMQ consumer")

	for name, fn := range w.activities {
		w.mq.RegisterActivityQueue(name)

		msgs, err := w.mq.ConsumeActivity(name)
		if err != nil {
			slog.Error("failed to start consuming", "activity", name, "err", err)
			continue
		}

		go func(activityName string, handler ActivityFunc, deliveries <-chan amqp091.Delivery) {
			for {
				select {
				case <-ctx.Done():
					return
				case d, ok := <-deliveries:
					if !ok {
						return // channel closed
					}
					var task models.ActivityTaskMessage
					if err := json.Unmarshal(d.Body, &task); err != nil {
						slog.Error("failed to unmarshal task", "err", err)
						d.Nack(false, false)
						continue
					}

					slog.Info("Worker received task", "activity", activityName, "id", task.ActivityID)

					// Deduplication check
					actExec, dbErr := w.execRepo.GetActivityExecution(ctx, task.ActivityID)
					if dbErr == nil {
						if actExec.Status == models.ActivityStatusCompleted || actExec.Status == models.ActivityStatusFailed {
							slog.Warn("Duplicate task detected, skipping execution", "activity", activityName, "id", task.ActivityID)
							d.Ack(false)
							continue
						}
					}

					actCtx := ActivityContext{
						Context:     ctx,
						ExecutionID: task.ExecutionID,
						ActivityID:  task.ActivityID,
					}

					err := handler(actCtx)
					res := models.ActivityResultMessage{
						ExecutionID:  task.ExecutionID,
						ActivityID:   task.ActivityID,
						ActivityName: task.ActivityName,
						Success:      err == nil,
					}
					if err != nil {
						slog.Error("Worker activity failed", "activity", activityName, "err", err)
						res.Error = err.Error()
					} else {
						slog.Info("Worker activity succeeded", "activity", activityName)
					}

					if err := w.mq.PublishResult(ctx, res); err != nil {
						slog.Error("failed to publish result", "err", err)
						d.Nack(false, true) // requeue if we can't publish result
					} else {
						d.Ack(false)
					}
				}
			}
		}(name, fn, msgs)
	}
}
