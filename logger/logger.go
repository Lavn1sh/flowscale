package logger

import (
	"log/slog"

	"gopkg.in/natefinch/lumberjack.v2"
)

func Init(env string) {
	f := &lumberjack.Logger{
		Filename:   "engine.log",
		MaxSize:    10, // megabytes
		MaxBackups: 3,  // keep up to 3 backups
		MaxAge:     7,  // days
		Compress:   true,
	}

	var handler slog.Handler
	if env == "production" {
		handler = slog.NewJSONHandler(f, &slog.HandlerOptions{Level: slog.LevelInfo})
	} else {
		handler = slog.NewTextHandler(f, &slog.HandlerOptions{Level: slog.LevelDebug})
	}

	logger := slog.New(handler)
	slog.SetDefault(logger)
}
