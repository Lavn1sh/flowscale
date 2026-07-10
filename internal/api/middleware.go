package api

import (
	"log/slog"
	"net/http"

	"flowscale/internal/queue"
	"golang.org/x/time/rate"
)

// RateLimiterMiddleware limits incoming requests to protect the system.
func RateLimiterMiddleware(limiter *rate.Limiter, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !limiter.Allow() {
			http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// BackpressureMiddleware checks RabbitMQ queue depth to reject new workflow executions
// if the system is overloaded.
func BackpressureMiddleware(mq *queue.RabbitMQ, threshold int, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Only apply backpressure to start workflow endpoint, let queries pass.
		if r.Method == http.MethodPost && r.URL.Path == "/workflows/start" {
			// We check the depth of the results queue as a proxy for engine load.
			// (Alternatively, we could check specific activity queues)
			depth, err := mq.GetQueueDepth("engine_results")
			if err != nil {
				slog.Error("failed to check queue depth for backpressure", "err", err)
			} else if depth > threshold {
				slog.Warn("Backpressure activated, rejecting request", "queue_depth", depth, "threshold", threshold)
				http.Error(w, "Service Unavailable - System Overloaded", http.StatusServiceUnavailable)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}
