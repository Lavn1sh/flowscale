package main

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"flowscale/config"
	"flowscale/internal/api"
	"flowscale/internal/engine"
	"flowscale/internal/queue"
	"flowscale/internal/repository"
	"flowscale/internal/scheduler"
	"flowscale/internal/worker"
	"flowscale/logger"

	_ "github.com/lib/pq"
	"golang.org/x/time/rate"
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
	scheduleHandler := api.NewScheduleHandler(repo)

	// Start Outbox Publisher
	outboxPub := engine.NewOutboxPublisher(execRepo, mq)

	// Start Scheduler
	sched := scheduler.NewScheduler(repo, eng)
	sched.Start()
	defer sched.Stop()

	// Result consumer is started below

	// Milestone 4: Worker wiring via RabbitMQ
	w := worker.NewWorker(mq, execRepo, 100)

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
	w.RegisterActivity("refund-payment", func(ctx worker.ActivityContext) error {
		slog.Info("Executing refund-payment", "executionID", ctx.ExecutionID)
		time.Sleep(1 * time.Second)
		return nil
	})
	w.RegisterActivity("cancel-shipment", func(ctx worker.ActivityContext) error {
		slog.Info("Executing cancel-shipment", "executionID", ctx.ExecutionID)
		time.Sleep(1 * time.Second)
		return nil
	})
	w.RegisterActivity("create-shipment", func(ctx worker.ActivityContext) error {
		slog.Info("Executing create-shipment", "executionID", ctx.ExecutionID)
		time.Sleep(1 * time.Second)
		return fmt.Errorf("simulated shipment failure")
	})

	healthHandler := api.NewHealthHandler(db, mq)

	mux := http.NewServeMux()
	mux.HandleFunc("/live", healthHandler.Live)
	mux.HandleFunc("/ready", healthHandler.Ready)
	mux.HandleFunc("/health", healthHandler.Health)
	mux.Handle("/workflows/start", execHandler)
	mux.Handle("/executions", execHandler)
	mux.Handle("/executions/", execHandler)
	mux.Handle("/workflows", wfHandler)
	mux.Handle("/workflows/", wfHandler)
	mux.Handle("/activities/dlq", dlqHandler)
	mux.Handle("/activities/dlq/", dlqHandler)
	mux.Handle("/schedules", scheduleHandler)
	mux.Handle("/schedules/", scheduleHandler)

	// Apply rate limiting (100 req/s, burst 50) and backpressure (queue > 5000)
	limiter := rate.NewLimiter(rate.Limit(100), 50)
	var handler http.Handler = mux
	handler = api.BackpressureMiddleware(mq, 5000, handler)
	handler = api.RateLimiterMiddleware(limiter, handler)

	addr := ":" + cfg.Port
	srv := &http.Server{
		Addr:    addr,
		Handler: handler,
	}

	// Create root context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start components with root context
	go outboxPub.Start(ctx)
	go eng.StartResultConsumer(ctx)
	go w.Start(ctx)

	go func() {
		slog.Info("Starting Workflow Engine", "addr", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("Server failed", "error", err)
			os.Exit(1)
		}
	}()

	// Wait for interrupt signal to gracefully shutdown the server
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit
	slog.Info("Shutting down engine...")

	// Cancel root context to stop background components
	cancel()

	// Shutdown HTTP server with a timeout
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("Server forced to shutdown", "error", err)
	}

	// Shutdown worker gracefully
	w.Shutdown(shutdownCtx)

	slog.Info("Engine exited gracefully")
}
