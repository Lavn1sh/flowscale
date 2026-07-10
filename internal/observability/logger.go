package observability

import (
	"log/slog"
	"os"
)

// InitLogger initializes the global logger to output structured JSON.
func InitLogger() {
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})
	logger := slog.New(handler)
	slog.SetDefault(logger)
}
