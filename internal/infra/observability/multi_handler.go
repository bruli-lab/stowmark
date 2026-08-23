package observability

import (
	"context"
	"errors"
	"log/slog"
)

type multiHandler struct {
	handlers []slog.Handler
}

func newMultiHandler(handlers ...slog.Handler) slog.Handler {
	return &multiHandler{
		handlers: handlers,
	}
}

func (m *multiHandler) Enabled(ctx context.Context, level slog.Level) bool {
	for _, handler := range m.handlers {
		if handler.Enabled(ctx, level) {
			return true
		}
	}

	return false
}
//nolint:gocritic // slog.Handler requires slog.Record to be passed by value.
func (m *multiHandler) Handle(ctx context.Context, record slog.Record) error {
	var errs []error

	for _, handler := range m.handlers {
		if !handler.Enabled(ctx, record.Level) {
			continue
		}

		if err := handler.Handle(ctx, record.Clone()); err != nil {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}

func (m *multiHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	handlers := make([]slog.Handler, 0, len(m.handlers))

	for _, handler := range m.handlers {
		handlers = append(handlers, handler.WithAttrs(attrs))
	}

	return &multiHandler{
		handlers: handlers,
	}
}

func (m *multiHandler) WithGroup(name string) slog.Handler {
	handlers := make([]slog.Handler, 0, len(m.handlers))

	for _, handler := range m.handlers {
		handlers = append(handlers, handler.WithGroup(name))
	}

	return &multiHandler{
		handlers: handlers,
	}
}
