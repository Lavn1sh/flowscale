package main

import (
	"context"
	"database/sql"
	"log/slog"
	"net/http"
	"os"
	"time"

	"flowscale/config"
	"flowscale/internal/api"
	"flowscale/internal/engine"
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

	repo := repository.NewWorkflowRepo(db)
	wfHandler := api.NewWorkflowHandler(repo)

	execRepo := repository.NewExecutionRepo(db)
	eng := engine.NewEngine(repo, execRepo)
	execHandler := api.NewExecutionHandler(eng, execRepo)

	// Milestone 3: Local worker wiring
	w := worker.NewWorker(eng)
	w.RegisterActivity("reserve-inventory", func(ctx worker.ActivityContext) error {
		slog.Info("Executing reserve-inventory", "executionID", ctx.ExecutionID)
		time.Sleep(1 * time.Second)
		return nil
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

	addr := ":" + cfg.Port
	slog.Info("Starting Workflow Engine", "addr", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		slog.Error("Server failed", "error", err)
		os.Exit(1)
	}
}
