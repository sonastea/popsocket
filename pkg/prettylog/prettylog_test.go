package prettylog

import (
	"log/slog"
	"os"
	"strings"
	"testing"
)

type newHandlerTestCase struct {
	name                     string
	handlerOptions           *slog.HandlerOptions
	options                  []Option
	expectedWriter           any
	expectedColor            bool
	expectedLevel            slog.Level
	expectedReplace          func(groups []string, a slog.Attr) slog.Attr
	expectedOutputEmptyAttrs bool
}

// TestNew tests the `new` function with multiple scenarios using a slice of structs.
func TestNew(t *testing.T) {
	t.Parallel()

	tests := []newHandlerTestCase{
		{
			name:            "Default Options",
			handlerOptions:  nil,
			expectedWriter:  nil,
			expectedColor:   false,
			expectedLevel:   slog.LevelInfo,
			expectedReplace: nil,
		},
		{
			name:           "With Handler Options",
			handlerOptions: &slog.HandlerOptions{Level: slog.LevelWarn, AddSource: true},
			expectedWriter: nil,
			expectedColor:  false,
			expectedLevel:  slog.LevelWarn,
		},
		{
			name:            "With Custom Writer and With Color",
			handlerOptions:  nil,
			options:         []Option{WithDestinationWriter(os.Stdout), WithColor()},
			expectedWriter:  os.Stdout,
			expectedColor:   true,
			expectedLevel:   slog.LevelInfo,
			expectedReplace: nil,
		},
		{
			name:           "With ReplaceAttr Function",
			handlerOptions: &slog.HandlerOptions{ReplaceAttr: func(groups []string, attr slog.Attr) slog.Attr { return slog.Attr{Key: "REPLACED"} }},
			expectedWriter: nil,
			expectedColor:  false,
			expectedLevel:  slog.LevelInfo,
		},
		{
			name:            "Without Color",
			handlerOptions:  nil,
			options:         []Option{WithoutColor()},
			expectedWriter:  nil,
			expectedColor:   false,
			expectedLevel:   slog.LevelInfo,
			expectedReplace: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := new(tt.handlerOptions, tt.options...)

			if handler == nil {
				t.Fatalf("Expected a handler instance, got nil")
			}

			if tt.expectedWriter != nil && handler.writer != tt.expectedWriter {
				t.Errorf("Expected writer %v, got %v", tt.expectedWriter, handler.writer)
			}

			if handler.colorize != tt.expectedColor {
				t.Errorf("Expected colorize to be %v, got %v", tt.expectedColor, handler.colorize)
			}

			if !handler.handler.Enabled(nil, tt.expectedLevel) {
				t.Errorf("Expected handler to be enabled for level %v, but it was not", tt.expectedLevel)
			}

			if tt.handlerOptions != nil && tt.handlerOptions.ReplaceAttr != nil {
				attr := handler.r([]string{}, slog.Attr{Key: slog.LevelKey, Value: slog.StringValue("INFO")})
				if attr.Key != "REPLACED" {
					t.Errorf("Expected ReplaceAttr to modify the key to 'REPLACED', got %s", attr.Key)
				}
			}
		})
	}
}

// TestNewHandler tests the `NewHandler` function to ensure it returns the properly defined defaults.
func TestNewHandler(t *testing.T) {
	t.Parallel()

	tests := []newHandlerTestCase{
		{
			name:            "Default Options",
			handlerOptions:  nil,
			options:         nil,
			expectedWriter:  os.Stderr,
			expectedColor:   true,
			expectedLevel:   slog.LevelInfo,
			expectedReplace: nil,
		},
		{
			name:                     "Without Empty Attrs",
			handlerOptions:           nil,
			options:                  []Option{WithOutputEmptyAttrs()},
			expectedWriter:           os.Stderr,
			expectedColor:            true,
			expectedLevel:            slog.LevelInfo,
			expectedReplace:          nil,
			expectedOutputEmptyAttrs: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := NewHandler(&slog.HandlerOptions{
				Level:       slog.LevelInfo,
				AddSource:   false,
				ReplaceAttr: nil,
			})

			if handler.writer != tt.expectedWriter {
				t.Errorf("Expected writer %v, got %v", tt.expectedWriter, handler.writer)
			}

			if handler.colorize != tt.expectedColor {
				t.Errorf("Expected colorize to be %v, got %v", tt.expectedColor, handler.colorize)
			}

			if strings.Contains(tt.name, "Empty Attrs") && handler.outputEmptyAttrs != false {
				t.Errorf("Expected outputEmptyAttrs to be %v, got %v", tt.expectedOutputEmptyAttrs, handler.outputEmptyAttrs)
			}
		})
	}
}
