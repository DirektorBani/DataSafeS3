package observability

import (
	"context"
	"log/slog"
	"os"
	"time"
)

type LoggerOptions struct {
	Level string
}

type externalSinkHandler struct {
	level slog.Level
}

func (h *externalSinkHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level
}

func (h *externalSinkHandler) Handle(_ context.Context, r slog.Record) error {
	attrs := map[string]any{
		"time":  r.Time.UTC().Format(time.RFC3339Nano),
		"level": r.Level.String(),
		"msg":   r.Message,
	}
	r.Attrs(func(a slog.Attr) bool {
		attrs[a.Key] = a.Value.Any()
		return true
	})
	GlobalSinkManager().Emit(attrs)
	return nil
}

func (h *externalSinkHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return h
}

func (h *externalSinkHandler) WithGroup(name string) slog.Handler {
	return h
}

func NewJSONLogger(opts LoggerOptions) *slog.Logger {
	level := parseLevel(opts.Level)
	stdHandler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level})
	multi := &multiHandler{handlers: []slog.Handler{stdHandler, &externalSinkHandler{level: level}}}
	return slog.New(multi)
}

func parseLevel(s string) slog.Level {
	switch s {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

type multiHandler struct {
	handlers []slog.Handler
}

func (m *multiHandler) Enabled(ctx context.Context, level slog.Level) bool {
	for _, h := range m.handlers {
		if h.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

func (m *multiHandler) Handle(ctx context.Context, r slog.Record) error {
	for _, h := range m.handlers {
		_ = h.Handle(ctx, r.Clone())
	}
	return nil
}

func (m *multiHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	out := make([]slog.Handler, len(m.handlers))
	for i, h := range m.handlers {
		out[i] = h.WithAttrs(attrs)
	}
	return &multiHandler{handlers: out}
}

func (m *multiHandler) WithGroup(name string) slog.Handler {
	out := make([]slog.Handler, len(m.handlers))
	for i, h := range m.handlers {
		out[i] = h.WithGroup(name)
	}
	return &multiHandler{handlers: out}
}

// EmitTestRecord sends a structured record to all enabled sinks (for diagnostics).
func EmitTestRecord(msg string) {
	GlobalSinkManager().Emit(map[string]any{"time": time.Now().UTC(), "level": "info", "msg": msg})
}
