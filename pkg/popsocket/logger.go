package popsocket

import (
	"log/slog"
	"sync"
)

var (
	once   sync.Once
	logger *slog.Logger
)

// newLogger returns a singleton slog.Logger
func newLogger(handler slog.Handler) *slog.Logger {
  once.Do(func() {
    logger = slog.New(handler)
  })
	return logger
}
