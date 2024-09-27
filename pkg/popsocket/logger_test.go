package popsocket

import (
	"log/slog"
	"sync"
	"testing"

	"os"
)

// TestNewLoggerSingleton ensures that NewLogger returns a singleton instance.
func TestNewLoggerSingleton(t *testing.T) {
	handler := slog.NewJSONHandler(os.Stdout, nil)

	logger1 := newLogger(handler)
	logger2 := newLogger(handler)

	if logger1 != logger2 {
		t.Errorf("NewLogger should return the same instance, got different instances")
	}
}

// TestNewLoggerConcurrency ensures thread safety by creating the logger only once even with concurrent access.
func TestNewLoggerConcurrency(t *testing.T) {
	handler := slog.NewTextHandler(os.Stdout, nil)
	var wg sync.WaitGroup

	const numGoroutines = 100
	loggerInstances := make([]*slog.Logger, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			loggerInstances[i] = newLogger(handler)
		}(i)
	}

	wg.Wait()

	for i := 1; i < numGoroutines; i++ {
		if loggerInstances[i] != loggerInstances[0] {
			t.Errorf("Logger instance is not singleton, instance %d differs from instance 0", i)
		}
	}
}

