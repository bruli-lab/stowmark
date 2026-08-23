package app

import (
	"context"
	"log/slog"
	"time"

	"github.com/bruli-lab/go-core/cqs"
)

func NewLoggerQueryMiddleware(logger *slog.Logger) cqs.QueryHandlerMiddleware {
	return func(next cqs.QueryHandler) cqs.QueryHandler {
		return cqs.QueryHandlerFunc(func(ctx context.Context, query cqs.Query) (any, error) {
			startedAt := time.Now()

			logger.DebugContext(ctx, "query started", slog.String("query", query.Name()))

			events, err := next.Handle(ctx, query)

			attrs := []any{
				slog.String("command", query.Name()),
				slog.Int64("duration_ms", time.Since(startedAt).Milliseconds()),
			}

			if err != nil {
				attrs = append(attrs, slog.String("status", "error"), slog.Any("error", err))

				logger.ErrorContext(ctx, "query failed", attrs...)

				return events, err
			}

			attrs = append(attrs, slog.String("status", "success"))

			logger.InfoContext(ctx, "query completed", attrs...)

			return events, nil
		})
	}
}
