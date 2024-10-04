package popsocket

import (
	"log/slog"
	"sync"
	"testing"
)

// TestNewLoggerSingleton ensures that NewLogger returns a singleton instance.
func TestNewLoggerSingleton(t *testing.T) {
	t.Parallel()

	logger1 := newLogger()
	logger2 := newLogger()

	if logger1 != logger2 {
		t.Errorf("NewLogger should return the same instance, got different instances")
	}
}

// TestNewLoggerConcurrency ensures thread safety by creating the logger only once even with concurrent access.
func TestNewLoggerConcurrency(t *testing.T) {
	t.Parallel()

	var wg sync.WaitGroup

	const numGoroutines = 100
	loggerInstances := make([]*slog.Logger, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			loggerInstances[i] = newLogger()
		}(i)
	}

	wg.Wait()

	for i := 1; i < numGoroutines; i++ {
		if loggerInstances[i] != loggerInstances[0] {
			t.Errorf("Logger instance is not singleton, instance %d differs from instance 0", i)
		}
	}
}
