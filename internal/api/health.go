package api

import (
	"database/sql"
	"net/http"

	"flowscale/internal/queue"
)

type HealthHandler struct {
	db *sql.DB
	mq *queue.RabbitMQ
}

func NewHealthHandler(db *sql.DB, mq *queue.RabbitMQ) *HealthHandler {
	return &HealthHandler{
		db: db,
		mq: mq,
	}
}

// Live is a pure process check with no dependency calls
func (h *HealthHandler) Live(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

// Ready checks dependencies (Postgres and RabbitMQ)
func (h *HealthHandler) Ready(w http.ResponseWriter, r *http.Request) {
	if err := h.db.PingContext(r.Context()); err != nil {
		http.Error(w, "Database unavailable", http.StatusServiceUnavailable)
		return
	}

	if err := h.mq.Ping(); err != nil {
		http.Error(w, "RabbitMQ unavailable", http.StatusServiceUnavailable)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("READY"))
}

// Health is usually an alias for Ready or Live depending on the environment conventions.
// We'll alias it to Ready.
func (h *HealthHandler) Health(w http.ResponseWriter, r *http.Request) {
	h.Ready(w, r)
}
