package main

import (
	"context"
	"io"

	"golang.org/x/exp/slog"
)

type contextKey string

const loggerKey contextKey = "logger"

func WithLogger(ctx context.Context, logger *slog.Logger) context.Context {
	return context.WithValue(ctx, loggerKey, logger)
}

func GetLogger(ctx context.Context) *slog.Logger {
	logger, ok := ctx.Value(loggerKey).(*slog.Logger)
	if !ok {
		return slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return logger
}
