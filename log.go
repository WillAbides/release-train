package main

import (
	"context"
	"io"
	"log/slog"
)

type contextKey string

const loggerKey contextKey = "logger"

var discardLogger = slog.New(slog.NewTextHandler(io.Discard, nil))

func withLogger(ctx context.Context, logger *slog.Logger) context.Context {
	return context.WithValue(ctx, loggerKey, logger)
}

func getLogger(ctx context.Context) *slog.Logger {
	logger, ok := ctx.Value(loggerKey).(*slog.Logger)
	if !ok {
		return discardLogger
	}
	return logger
}
