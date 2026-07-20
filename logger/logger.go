package logger

import (
	"io"
	"log/slog"
	"os"

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
	multiWriter := io.MultiWriter(os.Stdout, f)

	if env == "production" {
		handler = slog.NewJSONHandler(multiWriter, &slog.HandlerOptions{Level: slog.LevelInfo})
	} else {
		handler = slog.NewTextHandler(multiWriter, &slog.HandlerOptions{Level: slog.LevelDebug})
	}

	logger := slog.New(handler)
	slog.SetDefault(logger)
}
