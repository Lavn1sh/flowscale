package main

import (
	"database/sql"
	"log/slog"
	"net/http"
	"os"

	"flowscale/config"
	"flowscale/internal/api"
	"flowscale/internal/repository"
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
	handler := api.NewWorkflowHandler(repo)

	mux := http.NewServeMux()
	mux.Handle("/workflows", handler)
	mux.Handle("/workflows/", handler)

	addr := ":" + cfg.Port
	slog.Info("Starting Workflow Engine", "addr", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		slog.Error("Server failed", "error", err)
		os.Exit(1)
	}
}
