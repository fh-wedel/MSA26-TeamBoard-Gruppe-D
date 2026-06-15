// Package observability provides structured logging and distributed tracing setup.
package observability

import (
	"context"
	"log/slog"
	"os"
	"strings"

	"go.opentelemetry.io/otel/trace"
)

// SetupLogger creates a JSON slog.Logger, sets it as the default, and returns it.
func SetupLogger(serviceName, level string) *slog.Logger {
	var lvl slog.Level
	if err := lvl.UnmarshalText([]byte(strings.ToUpper(level))); err != nil {
		lvl = slog.LevelInfo
	}
	h := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level:       lvl,
		ReplaceAttr: RedactAttr,
	})
	logger := slog.New(&tracingHandler{next: h}).With("service", serviceName)
	slog.SetDefault(logger)
	return logger
}

// tracingHandler injects trace_id and span_id from context into every log record.
type tracingHandler struct {
	next slog.Handler
}

func (h *tracingHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}

func (h *tracingHandler) Handle(ctx context.Context, r slog.Record) error {
	if span := trace.SpanFromContext(ctx); span.SpanContext().IsValid() {
		sc := span.SpanContext()
		r.AddAttrs(
			slog.String("trace_id", sc.TraceID().String()),
			slog.String("span_id", sc.SpanID().String()),
		)
	}
	return h.next.Handle(ctx, r)
}

func (h *tracingHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &tracingHandler{next: h.next.WithAttrs(attrs)}
}

func (h *tracingHandler) WithGroup(name string) slog.Handler {
	return &tracingHandler{next: h.next.WithGroup(name)}
}
