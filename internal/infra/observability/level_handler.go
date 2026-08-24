package observability

import (
	"context"
	"log/slog"
)

var _ slog.Handler = (*levelHandler)(nil)

type levelHandler struct {
	level slog.Leveler
	next  slog.Handler
}

func newLevelHandler(level slog.Leveler, next slog.Handler) slog.Handler {
	return &levelHandler{
		level: level,
		next:  next,
	}
}

func (l *levelHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return level >= l.level.Level() &&
		l.next.Enabled(ctx, level)
}

//nolint:gocritic // slog.Handler requires slog.Record to be passed by value.
func (l *levelHandler) Handle(ctx context.Context, record slog.Record) error {
	return l.next.Handle(ctx, record)
}

func (l *levelHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &levelHandler{
		level: l.level,
		next:  l.next.WithAttrs(attrs),
	}
}

func (l *levelHandler) WithGroup(name string) slog.Handler {
	return &levelHandler{
		level: l.level,
		next:  l.next.WithGroup(name),
	}
}
