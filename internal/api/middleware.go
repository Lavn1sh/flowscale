package api

import (
	"context"
	"log/slog"
	"net/http"
	"strings"

	"flowscale/internal/observability"
	"flowscale/internal/queue"
	"github.com/golang-jwt/jwt/v5"
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
			depth, err := mq.GetQueueDepth(queue.ActivityResultsQueue)
			if err != nil {
				slog.Error("failed to check queue depth for backpressure", "err", err)
			} else {
				observability.QueueDepth.WithLabelValues(queue.ActivityResultsQueue).Set(float64(depth))
				if depth > threshold {
					slog.Warn("Backpressure activated, rejecting request", "queue_depth", depth, "threshold", threshold)
					http.Error(w, "Service Unavailable - System Overloaded", http.StatusServiceUnavailable)
					return
				}
			}
		}
		next.ServeHTTP(w, r)
	})
}

type contextKey string

const UserIDKey contextKey = "user_id"
const UsernameKey contextKey = "username"

func AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodOptions {
			next.ServeHTTP(w, r)
			return
		}

		path := r.URL.Path
		if path == "/api/auth/login" || path == "/live" || path == "/ready" || path == "/metrics" {
			next.ServeHTTP(w, r)
			return
		}

		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			http.Error(w, "missing authorization header", http.StatusUnauthorized)
			return
		}

		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
			http.Error(w, "invalid authorization header", http.StatusUnauthorized)
			return
		}

		tokenString := parts[1]

		token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, http.ErrAbortHandler
			}
			return JWTSecret, nil
		})

		if err != nil || !token.Valid {
			http.Error(w, "invalid or expired token", http.StatusUnauthorized)
			return
		}

		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			http.Error(w, "invalid token claims", http.StatusUnauthorized)
			return
		}

		ctx := context.WithValue(r.Context(), UserIDKey, claims["sub"])
		ctx = context.WithValue(ctx, UsernameKey, claims["username"])
		
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func CorsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}
		next.ServeHTTP(w, r)
	})
}
