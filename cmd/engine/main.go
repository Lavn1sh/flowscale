package main

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"flowscale/config"
	"flowscale/internal/api"
	"flowscale/internal/engine"
	"flowscale/internal/models"
	"flowscale/internal/observability"
	"flowscale/internal/queue"
	"flowscale/internal/repository"
	"flowscale/internal/scheduler"
	"flowscale/internal/worker"
	"flowscale/logger"

	"github.com/google/uuid"
	_ "github.com/lib/pq"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/time/rate"
)

func main() {
	observability.InitLogger()
	tp, err := observability.InitTracer()
	if err != nil {
		slog.Error("Failed to init tracer", "err", err)
	} else {
		defer tp.Shutdown(context.Background())
	}

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

	execRepo := repository.NewExecutionRepo(db)
	userRepo := repository.NewUserRepo(db)
	wfRepo := repository.NewWorkflowRepo(db)

	// Seed admin user
	if user, err := userRepo.GetUserByUsername(context.Background(), "admin"); err != nil || user == nil {
		// Create admin user if it doesn't exist
		hash, _ := bcrypt.GenerateFromPassword([]byte("admin"), bcrypt.DefaultCost)
		_ = userRepo.CreateUser(context.Background(), &models.User{
			ID:           uuid.NewString(),
			Username:     "admin",
			PasswordHash: string(hash),
		})
	}

	mq, err := queue.NewRabbitMQ(cfg.RabbitMQURL)
	if err != nil {
		slog.Error("Failed to connect to RabbitMQ", "error", err)
		os.Exit(1)
	}
	defer mq.Close()
	slog.Info("Connected to RabbitMQ")

	eng := engine.NewEngine(wfRepo, execRepo, mq)

	role := os.Getenv("ROLE")
	if role == "" {
		role = "all"
	}
	slog.Info("Starting Flowscale component", "role", role)

	isAPI := role == "all" || strings.Contains(role, "api")
	isEngine := role == "all" || strings.Contains(role, "engine")
	isScheduler := role == "all" || strings.Contains(role, "scheduler")
	isWorker := role == "all" || strings.Contains(role, "worker")

	var sched *scheduler.Scheduler
	var outboxPub *engine.OutboxPublisher
	var reaper *engine.Reaper
	var w *worker.Worker

	if isEngine {
		// Initialize the Reaper for activity timeout detection
		reaper = engine.NewReaper(eng)

		// Initialize Outbox Publisher
		outboxPub = engine.NewOutboxPublisher(execRepo, mq)
	}

	if isScheduler {
		// Start Scheduler
		sched = scheduler.NewScheduler(wfRepo, eng)
		sched.Start()
		defer sched.Stop()
	}

	if isWorker {
		// Milestone 4: Worker wiring via RabbitMQ
		w = worker.NewWorker(mq, execRepo, 100)

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
	}

	healthHandler := api.NewHealthHandler(db, mq)

	mux := http.NewServeMux()
	mux.HandleFunc("/live", healthHandler.Live)
	mux.HandleFunc("/ready", healthHandler.Ready)
	mux.HandleFunc("/health", healthHandler.Health)
	mux.Handle("/metrics", promhttp.Handler())

	if isAPI {
		wfHandler := api.NewWorkflowHandler(wfRepo)
		execHandler := api.NewExecutionHandler(eng, execRepo)
		dlqHandler := api.NewDLQHandler(eng, execRepo)
		scheduleHandler := api.NewScheduleHandler(wfRepo)
		authHandler := api.NewAuthHandler(userRepo)

		mux.Handle("/api/auth/login", authHandler)
		mux.Handle("/executions", execHandler)
		mux.Handle("/executions/", execHandler)
		mux.Handle("/workflows/start", execHandler)
		mux.Handle("/workflows", wfHandler)
		mux.Handle("/workflows/", wfHandler)
		mux.Handle("/activities/dlq", dlqHandler)
		mux.Handle("/activities/dlq/", dlqHandler)
		mux.Handle("/schedules", scheduleHandler)
		mux.Handle("/schedules/", scheduleHandler)
	}

	// Apply OTEL Tracing, Rate Limiting (100 req/s, burst 50), and Backpressure (queue > 5000)
	limiter := rate.NewLimiter(rate.Limit(100), 50)
	var handler http.Handler = mux
	if isAPI {
		// Wrap with CORS
		handler = api.CorsMiddleware(handler)
		// Wrap with Auth
		handler = api.AuthMiddleware(handler)

		handler = otelhttp.NewHandler(handler, "engine-api")
		handler = api.BackpressureMiddleware(mq, 5000, handler)
		handler = api.RateLimiterMiddleware(limiter, handler)
	}

	addr := ":" + cfg.Port
	if !isAPI {
		addr = ":8081" // Default port for non-API health checks
	}

	srv := &http.Server{
		Addr:    addr,
		Handler: handler,
	}

	// Create root context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start components with root context
	if isEngine {
		go outboxPub.Start(ctx)
		go eng.StartResultConsumer(ctx)
		go reaper.Start(ctx)
	}
	if isWorker {
		go w.Start(ctx)
	}

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
	if isWorker {
		w.Shutdown(shutdownCtx)
	}

	slog.Info("Engine exited gracefully")
}
