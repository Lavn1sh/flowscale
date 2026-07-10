package main

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"flowscale/config"
	"flowscale/internal/api"
	"flowscale/internal/engine"
	"flowscale/internal/queue"
	"flowscale/internal/repository"
	"flowscale/internal/worker"
	"flowscale/logger"

	_ "github.com/lib/pq"
)

func main() {
	cfg := config.Load()
	logger.Init(cfg.Environment)

	db, err := sql.Open("postgres", cfg.DatabaseURL)
	if err != nil {
		slog.Error("Failed to open db", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		slog.Error("Failed to ping db", "error", err)
		os.Exit(1)
	}
	slog.Info("Connected to PostgreSQL")

	mq, err := queue.NewRabbitMQ(cfg.RabbitMQURL)
	if err != nil {
		slog.Error("Failed to connect to RabbitMQ", "error", err)
		os.Exit(1)
	}
	defer mq.Close()
	slog.Info("Connected to RabbitMQ")

	repo := repository.NewWorkflowRepo(db)
	wfHandler := api.NewWorkflowHandler(repo)

	execRepo := repository.NewExecutionRepo(db)
	eng := engine.NewEngine(repo, execRepo, mq)
	execHandler := api.NewExecutionHandler(eng, execRepo)
	dlqHandler := api.NewDLQHandler(eng, execRepo)

	// Start result consumer
	go eng.StartResultConsumer(context.Background())

	// Milestone 4: Worker wiring via RabbitMQ
	w := worker.NewWorker(mq)

	w.RegisterActivity("reserve-inventory", func(ctx worker.ActivityContext) error {
		slog.Info("Executing reserve-inventory", "executionID", ctx.ExecutionID)
		slog.Warn("Simulating failure for reserve-inventory")
		return fmt.Errorf("simulated permanent failure")
	})
	w.RegisterActivity("charge-card", func(ctx worker.ActivityContext) error {
		slog.Info("Executing charge-card", "executionID", ctx.ExecutionID)
		time.Sleep(1 * time.Second)
		return nil
	})
	w.RegisterActivity("release-inventory", func(ctx worker.ActivityContext) error {
		slog.Info("Executing release-inventory", "executionID", ctx.ExecutionID)
		time.Sleep(1 * time.Second)
		return nil
	})
	w.RegisterActivity("create-shipment", func(ctx worker.ActivityContext) error {
		slog.Info("Executing create-shipment", "executionID", ctx.ExecutionID)
		time.Sleep(1 * time.Second)
		return nil
	})
	go w.Start(context.Background())

	mux := http.NewServeMux()
	mux.Handle("/workflows/start", execHandler)
	mux.Handle("/executions/", execHandler)
	mux.Handle("/workflows", wfHandler)
	mux.Handle("/workflows/", wfHandler)
	mux.Handle("/activities/dlq", dlqHandler)
	mux.Handle("/activities/dlq/", dlqHandler)

	addr := ":" + cfg.Port
	slog.Info("Starting Workflow Engine", "addr", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		slog.Error("Server failed", "error", err)
		os.Exit(1)
	}
}
