package queue

import (
	"context"
	"encoding/json"
	"fmt"

	"flowscale/internal/observability"
	"github.com/rabbitmq/amqp091-go"
)

const (
	WorkflowExchange     = "workflow.exchange"
	ResultsExchange      = "results.exchange"
	DlqExchange          = "dlq.exchange"
	RetryExchange        = "retry.exchange"
	ActivityResultsQueue = "activity.results.queue"
)

type RabbitMQ struct {
	conn *amqp091.Connection
	ch   *amqp091.Channel
}

func NewRabbitMQ(url string) (*RabbitMQ, error) {
	conn, err := amqp091.Dial(url)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to rabbitmq: %w", err)
	}

	ch, err := conn.Channel()
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to open a channel: %w", err)
	}

	r := &RabbitMQ{conn: conn, ch: ch}
	if err := r.declareTopology(); err != nil {
		return nil, err
	}

	return r, nil
}

func (r *RabbitMQ) declareTopology() error {
	// Exchanges
	if err := r.ch.ExchangeDeclare(WorkflowExchange, "topic", true, false, false, false, nil); err != nil {
		return err
	}
	if err := r.ch.ExchangeDeclare(ResultsExchange, "topic", true, false, false, false, nil); err != nil {
		return err
	}
	if err := r.ch.ExchangeDeclare(DlqExchange, "topic", true, false, false, false, nil); err != nil {
		return err
	}
	if err := r.ch.ExchangeDeclare(RetryExchange, "headers", true, false, false, false, nil); err != nil {
		return err
	}

	// Retry Tiered Parking Queues
	retryTiers := []int{1, 2, 4, 8, 16}
	for _, tier := range retryTiers {
		queueName := fmt.Sprintf("retry.%ds.queue", tier)

		args := amqp091.Table{
			"x-message-ttl":          int32(tier * 1000), // ms
			"x-dead-letter-exchange": WorkflowExchange,
		}

		_, err := r.ch.QueueDeclare(queueName, true, false, false, false, args)
		if err != nil {
			return err
		}

		bindArgs := amqp091.Table{
			"x-match":    "all",
			"delay-tier": tier,
		}
		if err := r.ch.QueueBind(queueName, "", RetryExchange, false, bindArgs); err != nil {
			return err
		}
	}

	// Global Results Queue
	_, err := r.ch.QueueDeclare(ActivityResultsQueue, true, false, false, false, nil)
	if err != nil {
		return err
	}
	if err := r.ch.QueueBind(ActivityResultsQueue, "result", ResultsExchange, false, nil); err != nil {
		return err
	}

	return nil
}

func (r *RabbitMQ) RegisterActivityQueue(activityName string) error {
	queueName := fmt.Sprintf("%s.queue", activityName)
	_, err := r.ch.QueueDeclare(queueName, true, false, false, false, nil)
	if err != nil {
		return err
	}
	if err := r.ch.QueueBind(queueName, activityName, WorkflowExchange, false, nil); err != nil {
		return err
	}

	dlqName := fmt.Sprintf("activity.%s.dlq", activityName)
	_, err = r.ch.QueueDeclare(dlqName, true, false, false, false, nil)
	if err != nil {
		return err
	}
	if err := r.ch.QueueBind(dlqName, activityName, DlqExchange, false, nil); err != nil {
		return err
	}

	return nil
}

func (r *RabbitMQ) PublishTask(ctx context.Context, activityName string, payload interface{}) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	headers := observability.Inject(ctx, nil)
	return r.ch.PublishWithContext(ctx, WorkflowExchange, activityName, false, false, amqp091.Publishing{
		ContentType: "application/json",
		Body:        body,
		Headers:     headers,
	})
}

func (r *RabbitMQ) PublishRetryTask(ctx context.Context, activityName string, tier int, payload interface{}) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	headers := observability.Inject(ctx, amqp091.Table{
		"delay-tier": tier,
	})
	return r.ch.PublishWithContext(ctx, RetryExchange, activityName, false, false, amqp091.Publishing{
		ContentType: "application/json",
		Body:        body,
		Headers:     headers,
	})
}

func (r *RabbitMQ) PublishDLQ(ctx context.Context, activityName string, payload interface{}) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	headers := observability.Inject(ctx, nil)
	return r.ch.PublishWithContext(ctx, DlqExchange, activityName, false, false, amqp091.Publishing{
		ContentType: "application/json",
		Body:        body,
		Headers:     headers,
	})
}

func (r *RabbitMQ) PublishResult(ctx context.Context, payload interface{}) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	headers := observability.Inject(ctx, nil)
	return r.ch.PublishWithContext(ctx, ResultsExchange, "result", false, false, amqp091.Publishing{
		ContentType: "application/json",
		Body:        body,
		Headers:     headers,
	})
}

func (r *RabbitMQ) ConsumeActivity(activityName string) (<-chan amqp091.Delivery, error) {
	queueName := fmt.Sprintf("%s.queue", activityName)
	return r.ch.Consume(queueName, "", false, false, false, false, nil)
}

func (r *RabbitMQ) ConsumeResults() (<-chan amqp091.Delivery, error) {
	return r.ch.Consume(ActivityResultsQueue, "", false, false, false, false, nil)
}

func (r *RabbitMQ) Close() {
	if r.ch != nil {
		r.ch.Close()
	}
	if r.conn != nil {
		r.conn.Close()
	}
}

func (r *RabbitMQ) Ping() error {
	if r.conn == nil || r.conn.IsClosed() {
		return fmt.Errorf("rabbitmq connection is closed")
	}
	return nil
}

func (r *RabbitMQ) GetQueueDepth(queueName string) (int, error) {
	q, err := r.ch.QueueInspect(queueName)
	if err != nil {
		return 0, err
	}
	return q.Messages, nil
}
