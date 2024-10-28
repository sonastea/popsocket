package popsocket

import (
	"bytes"
	"log/slog"
	"sync"

	"github.com/sonastea/popsocket/pkg/prettylog"
)

var (
	once   sync.Once
	logger *slog.Logger
)

// newLogger returns a singleton slog.Logger.
func newLogger() *slog.Logger {
	once.Do(func() {
		prettyHandler := prettylog.NewHandler(&slog.HandlerOptions{
			// TODO don't hardcode; use env?, pass to popsocket.New()?, logger helper funcs?.
			Level:       slog.LevelInfo,
			AddSource:   false,
			ReplaceAttr: nil,
		})

		logger = slog.New(prettyHandler)
	})
	return logger
}

// Logger is a wrapper around newLogger that allows global access to the singleton instance.
func Logger() *slog.Logger {
	return newLogger()
}

func newTestLogger() (*slog.Logger, *bytes.Buffer) {
	buf := &bytes.Buffer{}
	handler := slog.NewTextHandler(buf, nil)
	logger := slog.New(handler)
	return logger, buf
}
