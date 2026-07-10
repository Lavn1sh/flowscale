package observability

import (
	"context"

	"github.com/rabbitmq/amqp091-go"
	"go.opentelemetry.io/otel/propagation"
)

// AMQPCarrier adapts amqp091.Table to satisfy the OpenTelemetry TextMapCarrier interface
// for context propagation over RabbitMQ.
type AMQPCarrier struct {
	Headers amqp091.Table
}

// Get returns the value associated with the passed key.
func (c AMQPCarrier) Get(key string) string {
	if c.Headers == nil {
		return ""
	}
	if val, ok := c.Headers[key]; ok {
		if s, ok := val.(string); ok {
			return s
		}
	}
	return ""
}

// Set stores the key-value pair.
func (c AMQPCarrier) Set(key string, value string) {
	if c.Headers == nil {
		c.Headers = make(amqp091.Table)
	}
	c.Headers[key] = value
}

// Keys lists the keys stored in this carrier.
func (c AMQPCarrier) Keys() []string {
	keys := make([]string, 0, len(c.Headers))
	for k := range c.Headers {
		keys = append(keys, k)
	}
	return keys
}

// Inject injects the context into the AMQP headers.
func Inject(ctx context.Context, headers amqp091.Table) amqp091.Table {
	if headers == nil {
		headers = make(amqp091.Table)
	}
	carrier := AMQPCarrier{Headers: headers}
	propagation.TraceContext{}.Inject(ctx, carrier)
	return carrier.Headers
}

// Extract extracts the context from the AMQP headers.
func Extract(ctx context.Context, headers amqp091.Table) context.Context {
	carrier := AMQPCarrier{Headers: headers}
	return propagation.TraceContext{}.Extract(ctx, carrier)
}
