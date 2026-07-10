package observability

import (
	"context"
	"fmt"
	"log/slog"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.17.0"
	"gopkg.in/natefinch/lumberjack.v2"
)

// InitTracer initializes an OpenTelemetry tracer provider with a stdout exporter.
func InitTracer() (*sdktrace.TracerProvider, error) {
	f := &lumberjack.Logger{
		Filename:   "traces.json",
		MaxSize:    10, // megabytes
		MaxBackups: 1,  // keep only 1 backup
		MaxAge:     1,  // days
		Compress:   true,
	}
	exporter, err := stdouttrace.New(stdouttrace.WithWriter(f))
	if err != nil {
		return nil, fmt.Errorf("failed to create stdout exporter: %w", err)
	}

	res, err := resource.New(context.Background(),
		resource.WithAttributes(
			semconv.ServiceName("flowscale-engine"),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource: %w", err)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
	)

	otel.SetTracerProvider(tp)
	slog.Info("OpenTelemetry tracer initialized with stdout exporter")

	return tp, nil
}
