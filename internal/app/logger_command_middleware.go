package app

import (
	"context"
	"log/slog"
	"time"

	"github.com/bruli-lab/go-core/cqs"
	"github.com/bruli-lab/go-core/event"
)

func NewLoggerCommandMiddleware(logger *slog.Logger) cqs.CommandHandlerMiddleware {
	return func(next cqs.CommandHandler) cqs.CommandHandler {
		return cqs.CommandHandlerFunc(func(ctx context.Context, cmd cqs.Command) ([]event.Event, error) {
			startedAt := time.Now()

			logger.DebugContext(ctx, "com	mand started", slog.String("command", cmd.Name()))

			events, err := next.Handle(ctx, cmd)

			attrs := []any{
				slog.String("command", cmd.Name()),
				slog.Int64("duration_ms", time.Since(startedAt).Milliseconds()),
				slog.Int("events", len(events)),
			}

			if err != nil {
				attrs = append(attrs, slog.String("status", "error"), slog.Any("error", err))

				logger.ErrorContext(ctx, "command failed", attrs...)

				return events, err
			}

			attrs = append(attrs, slog.String("status", "success"))

			logger.InfoContext(ctx, "command completed", attrs...)

			return events, nil
		})
	}
}
