package prettylog

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"sync"
)

const (
	timeFormat = "[15:04:05.000]"

	reset = "\033[0m"

	black        = 30
	red          = 31
	green        = 32
	yellow       = 33
	blue         = 34
	magenta      = 35
	cyan         = 36
	lightGray    = 37
	darkGray     = 90
	lightRed     = 91
	lightGreen   = 92
	lightYellow  = 93
	lightBlue    = 94
	lightMagenta = 95
	lightCyan    = 96
	white        = 97
)

type Handler struct {
	handler          slog.Handler
	r                func([]string, slog.Attr) slog.Attr
	buf              *bytes.Buffer
	mutex            *sync.Mutex
	writer           io.Writer
	colorize         bool
	outputEmptyAttrs bool
}

type Option func(h *Handler)

// colorizer returns a formatted string with a given color code.
func colorizer(colorCode int, v string) string {
	return fmt.Sprintf("\033[%sm%s%s", strconv.Itoa(colorCode), v, reset)
}

// computeAttrs invokes the Handle function of the nested slog.Handler to write
// the log attributes to the *bytes.Buffer instead of the final io.Writer.
func (h *Handler) computeAttrs(
	ctx context.Context,
	r slog.Record,
) (map[string]any, error) {
	h.mutex.Lock()
	defer func() {
		h.buf.Reset()
		h.mutex.Unlock()
	}()
	if err := h.handler.Handle(ctx, r); err != nil {
		return nil, fmt.Errorf("error when calling inner handler's Handle: %w", err)
	}

	var attrs map[string]any
	err := json.Unmarshal(h.buf.Bytes(), &attrs)
	if err != nil {
		return nil, fmt.Errorf("error when unmarshaling inner handler's Handle result: %w", err)
	}
	return attrs, nil
}

// suppressDefaults creates a wrapper function to suppress default
// attributes (e.g., time, level, message) from the log output.
func suppressDefaults(
	next func([]string, slog.Attr) slog.Attr,
) func([]string, slog.Attr) slog.Attr {
	return func(groups []string, a slog.Attr) slog.Attr {
		if a.Key == slog.TimeKey ||
			a.Key == slog.LevelKey ||
			a.Key == slog.MessageKey {
			return slog.Attr{}
		}
		if next == nil {
			return a
		}
		return next(groups, a)
	}
}

// new creates and returns a new instance of Handler based
// on the provided handlerOptions and options.
//
//	 Note: `new` does not initialize the `writer` and `colorize` field, and an error will occur
//		if the writer is not set using an option such as [WithDestinationWriter] and [WithColor]
//		or by using the exposed [NewHandler] instead.
func new(handlerOptions *slog.HandlerOptions, options ...Option) *Handler {
	if handlerOptions == nil {
		handlerOptions = &slog.HandlerOptions{}
	}

	buf := &bytes.Buffer{}
	handler := &Handler{
		buf: buf,
		handler: slog.NewJSONHandler(buf, &slog.HandlerOptions{
			Level:       handlerOptions.Level,
			AddSource:   handlerOptions.AddSource,
			ReplaceAttr: suppressDefaults(handlerOptions.ReplaceAttr),
		}),
		r:     handlerOptions.ReplaceAttr,
		mutex: &sync.Mutex{},
	}

	for _, opt := range options {
		opt(handler)
	}

	return handler
}

// Enabled returns whether logging is enabled for a given logging level.
func (h *Handler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.handler.Enabled(ctx, level)
}

// WithAttrs returns a new Handler with additional attributes.
func (h *Handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &Handler{handler: h.handler.WithAttrs(attrs), buf: h.buf, r: h.r, mutex: h.mutex, writer: h.writer, colorize: h.colorize}
}

// WithGroup returns a new Handler within a new group.
func (h *Handler) WithGroup(name string) slog.Handler {
	return &Handler{handler: h.handler.WithGroup(name), buf: h.buf, r: h.r, mutex: h.mutex, writer: h.writer, colorize: h.colorize}
}

// Handle processes a slog.Record and writes formatted output to the configured writer.
func (h *Handler) Handle(ctx context.Context, r slog.Record) error {
	colorize := func(code int, value string) string {
		return value
	}
	if h.colorize {
		colorize = colorizer
	}

	var level string
	levelAttr := slog.Attr{
		Key:   slog.LevelKey,
		Value: slog.AnyValue(r.Level),
	}
	if h.r != nil {
		levelAttr = h.r([]string{}, levelAttr)
	}

	if !levelAttr.Equal(slog.Attr{}) {
		level = levelAttr.Value.String() + ":"

		switch r.Level {
		case slog.LevelDebug:
			level = colorize(lightGray, level)
		case slog.LevelInfo:
			level = colorize(cyan, level)
		case slog.LevelWarn:
			level = colorize(lightYellow, level)
		case slog.LevelError:
			level = colorize(red, level)
		default:
			level = colorize(lightMagenta, level)
		}
	}

	var timestamp string
	timeAttr := slog.Attr{
		Key:   slog.TimeKey,
		Value: slog.StringValue(r.Time.Format(timeFormat)),
	}
	if h.r != nil {
		timeAttr = h.r([]string{}, timeAttr)
	}
	if !timeAttr.Equal(slog.Attr{}) {
		timestamp = colorize(lightGray, timeAttr.Value.String())
	}

	var msg string
	msgAttr := slog.Attr{
		Key:   slog.MessageKey,
		Value: slog.StringValue(r.Message),
	}
	if h.r != nil {
		msgAttr = h.r([]string{}, msgAttr)
	}
	if !msgAttr.Equal(slog.Attr{}) {
		msg = colorize(white, msgAttr.Value.String())
	}

	attrs, err := h.computeAttrs(ctx, r)
	if err != nil {
		return err
	}

	var attrsAsBytes []byte
	if h.outputEmptyAttrs || len(attrs) > 0 {
		attrsAsBytes, err = json.MarshalIndent(attrs, "", "  ")
		if err != nil {
			return fmt.Errorf("error when marshaling attrs: %w", err)
		}
	}

	out := strings.Builder{}
	if len(timestamp) > 0 {
		out.WriteString(timestamp)
		out.WriteString(" ")
	}
	if len(level) > 0 {
		out.WriteString(level)
		out.WriteString(" ")
	}
	if len(msg) > 0 {
		out.WriteString(msg)
		out.WriteString(" ")
	}
	if len(attrsAsBytes) > 0 {
		out.WriteString(colorize(darkGray, string(attrsAsBytes)))
	}

	_, err = io.WriteString(h.writer, out.String()+"\n")
	if err != nil {
		return err
	}

	return nil
}

// NewHandler creates a new Handler instance with predefined options.
func NewHandler(opts *slog.HandlerOptions, options ...Option) *Handler {
	options = append([]Option{
		WithDestinationWriter(os.Stderr),
		WithColor(),
	}, options...)

	return new(opts, options...)
}

// WithDestinationWriter sets the destination writer for the handler.
func WithDestinationWriter(writer io.Writer) Option {
	return func(h *Handler) {
		h.writer = writer
	}
}

// WithColor enables colorized output for the handler.
func WithColor() Option {
	return func(h *Handler) {
		h.colorize = true
	}
}

// WithoutColor disables colorized output for the handler.
func WithoutColor() Option {
	return func(h *Handler) {
		h.colorize = false
	}
}

// WithOutputEmptyAttrs enables the output of empty attributes in the log.
func WithOutputEmptyAttrs() Option {
	return func(h *Handler) {
		h.outputEmptyAttrs = true
	}
}
