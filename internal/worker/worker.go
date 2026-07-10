package worker

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"

	"flowscale/internal/models"
	"flowscale/internal/observability"
	"flowscale/internal/queue"
	"flowscale/internal/repository"
	"github.com/rabbitmq/amqp091-go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

type ActivityContext struct {
	context.Context
	ExecutionID string
	ActivityID  string
	mq          *queue.RabbitMQ
}

func (c *ActivityContext) Heartbeat() error {
	payload := map[string]string{
		"execution_id": c.ExecutionID,
		"activity_id":  c.ActivityID,
	}
	return c.mq.PublishHeartbeat(c.Context, payload)
}

type ActivityFunc func(ctx ActivityContext) error

type Worker struct {
	mq         *queue.RabbitMQ
	execRepo   *repository.ExecutionRepo
	activities map[string]ActivityFunc
	wg         sync.WaitGroup
	sem        chan struct{} // semaphore for backpressure
}

func NewWorker(mq *queue.RabbitMQ, execRepo *repository.ExecutionRepo, maxConcurrency int) *Worker {
	if maxConcurrency <= 0 {
		maxConcurrency = 100 // default
	}
	return &Worker{
		mq:         mq,
		execRepo:   execRepo,
		activities: make(map[string]ActivityFunc),
		sem:        make(chan struct{}, maxConcurrency),
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
					
					// Apply backpressure by acquiring semaphore
					select {
					case w.sem <- struct{}{}:
					case <-ctx.Done():
						d.Nack(false, true)
						return
					}

					w.wg.Add(1)
					observability.WorkerUtilization.Inc()
					go func(delivery amqp091.Delivery) {
						defer w.wg.Done()
						defer func() {
							<-w.sem
							observability.WorkerUtilization.Dec()
						}() // release semaphore

						var task models.ActivityTaskMessage
						if err := json.Unmarshal(delivery.Body, &task); err != nil {
							slog.Error("failed to unmarshal task", "err", err)
							delivery.Nack(false, false)
							return
						}

						slog.Info("Worker received task", "activity", activityName, "id", task.ActivityID)

						// Deduplication check
						actExec, dbErr := w.execRepo.GetActivityExecution(ctx, task.ActivityID)
						if dbErr == nil {
							if actExec.Status == models.ActivityStatusCompleted || actExec.Status == models.ActivityStatusFailed {
								slog.Warn("Duplicate task detected, skipping execution", "activity", activityName, "id", task.ActivityID)
								delivery.Ack(false)
								return
							}
						}

						// Extract trace context from message headers
						actCtx := ActivityContext{
							Context:     observability.Extract(ctx, delivery.Headers),
							ExecutionID: task.ExecutionID,
							ActivityID:  task.ActivityID,
							mq:          w.mq,
						}

						observability.ActivitiesStartedTotal.Inc()

						// Start activity span
						spanCtx, span := otel.Tracer("worker").Start(actCtx.Context, "ExecuteActivity", trace.WithAttributes(
							attribute.String("activityName", activityName),
							attribute.String("activityID", task.ActivityID),
							attribute.String("executionID", task.ExecutionID),
						))
						actCtx.Context = spanCtx

						err := handler(actCtx)
						if err != nil {
							span.RecordError(err)
						}
						span.End()
						res := models.ActivityResultMessage{
							ExecutionID:  task.ExecutionID,
							ActivityID:   task.ActivityID,
							ActivityName: activityName,
						}

						if err != nil {
							slog.Error("Worker activity failed", "activity", activityName, "err", err)
							res.Error = err.Error()
							if _, ok := err.(*NonRetryableError); ok {
								res.NonRetryable = true
							}
							observability.ActivitiesFailedTotal.Inc()
						} else {
							res.Success = true
							slog.Info("Worker activity succeeded", "activity", activityName)
							observability.ActivitiesCompletedTotal.Inc()
						}

						if err := w.mq.PublishResult(ctx, res); err != nil {
							slog.Error("failed to publish result", "err", err)
							delivery.Nack(false, true) // requeue if we can't publish result
						} else {
							delivery.Ack(false)
						}
					}(d)
				}
			}
		}(name, fn, msgs)
	}
}

func (w *Worker) Shutdown(ctx context.Context) {
	slog.Info("Worker shutting down, waiting for active tasks...")
	
	done := make(chan struct{})
	go func() {
		w.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		slog.Info("Worker drained successfully")
	case <-ctx.Done():
		slog.Warn("Worker shutdown timed out, dropping active tasks")
	}
}

