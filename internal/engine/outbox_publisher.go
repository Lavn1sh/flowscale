package engine

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"flowscale/internal/models"
	"flowscale/internal/queue"
	"flowscale/internal/repository"
)

type OutboxPublisher struct {
	repo *repository.ExecutionRepo
	mq   *queue.RabbitMQ
}

func NewOutboxPublisher(repo *repository.ExecutionRepo, mq *queue.RabbitMQ) *OutboxPublisher {
	return &OutboxPublisher{
		repo: repo,
		mq:   mq,
	}
}

func (p *OutboxPublisher) Start(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.processOutbox(ctx)
		}
	}
}

func (p *OutboxPublisher) processOutbox(ctx context.Context) {
	msgs, err := p.repo.GetPendingOutboxMessages(ctx, 100)
	if err != nil {
		slog.Error("failed to get outbox messages", "err", err)
		return
	}

	for _, msg := range msgs {
		var task models.ActivityTaskMessage
		if err := json.Unmarshal(msg.Payload, &task); err != nil {
			slog.Error("failed to unmarshal outbox message payload", "err", err, "id", msg.ID)
			continue
		}

		// Tier logic:
		// -1 : Publish to DLQ
		// 0  : Normal Task Publish
		// >0 : Publish Retry Task (Tiered)
		var pubErr error
		switch msg.Tier {
		case -1:
			pubErr = p.mq.PublishDLQ(ctx, msg.Topic, task)
		case 0:
			pubErr = p.mq.PublishTask(ctx, msg.Topic, task)
		default:
			pubErr = p.mq.PublishRetryTask(ctx, msg.Topic, msg.Tier, task)
		}

		if pubErr != nil {
			slog.Error("failed to publish outbox message to rabbitmq", "err", pubErr, "id", msg.ID)
			// Will retry on next tick
			continue
		}

		// Delete from outbox once successfully published
		if err := p.repo.DeleteOutboxMessage(ctx, msg.ID); err != nil {
			slog.Error("failed to delete published outbox message", "err", err, "id", msg.ID)
		}
	}
}
