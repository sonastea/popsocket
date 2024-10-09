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

// TestLogger ensures that the returned logger is a singleton instance.
func TestLogger(t *testing.T) {
	t.Parallel()

	logger1 := newLogger()
	logger2 := newLogger()
	logger3 := newLogger()
	logger4 := newLogger()

	if logger1 != logger2 {
		t.Errorf("NewLogger should return the same instance, got logger1:%v and logger2:%v", logger1, logger2)
	}

	if logger2 != logger3 {
		t.Errorf("NewLogger should return the same instance, got logger2:%v and logger3:%v", logger2, logger3)
	}

	if logger3 != logger4 {
		t.Errorf("NewLogger should return the same instance, got logger3:%v and logger4:%v", logger3, logger4)
	}

	if logger4 != logger1 {
		t.Errorf("NewLogger should return the same instance, got logger4:%v and logger1:%v", logger4, logger1)
	}
}
