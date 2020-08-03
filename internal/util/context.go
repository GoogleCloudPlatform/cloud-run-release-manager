package util

import (
	"context"

	"github.com/sirupsen/logrus"
)

type contextKeyLogger struct{}

// The logger context key
var loggerKey contextKeyLogger

// ContextWithLogger returns a copy of the parent context that includes the
// logger.
func ContextWithLogger(ctx context.Context, logger *logrus.Entry) context.Context {
	return context.WithValue(ctx, loggerKey, logger)
}

// LoggerFrom returns the logger from the context.
func LoggerFrom(ctx context.Context) *logrus.Entry {
	value := ctx.Value(loggerKey)
	logger, ok := value.(*logrus.Entry)
	if !ok {
		logrus.Warnf("received wrong type of logger (%T)", value)
		logger = logrus.NewEntry(logrus.New())
	}

	return logger
}
