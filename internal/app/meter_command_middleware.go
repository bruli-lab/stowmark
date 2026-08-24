package app

import (
	"context"
	"time"

	"github.com/bruli-lab/go-core/cqs"
	"github.com/bruli-lab/go-core/event"
	"github.com/bruli-lab/stowmark/internal/infra/observability"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

func NewMeterCommandMiddleware(provider metric.MeterProvider) (cqs.CommandHandlerMiddleware, error) {
	meter := provider.Meter(instrumentationName)
	metrics, err := observability.NewCommandMetrics(meter)
	if err != nil {
		return nil, err
	}
	return func(next cqs.CommandHandler) cqs.CommandHandler {
		return cqs.CommandHandlerFunc(
			func(ctx context.Context, cmd cqs.Command) ([]event.Event, error) {
				startedAt := time.Now()

				events, err := next.Handle(ctx, cmd)

				status := "success"
				if err != nil {
					status = "error"
				}

				attrs := metric.WithAttributes(
					attribute.String("operation.name", cmd.Name()),
					attribute.String("operation.type", "command"),
					attribute.String("operation.status", status),
				)

				metrics.Executions.Add(ctx, 1, attrs)

				metrics.Duration.Record(ctx, time.Since(startedAt).Seconds(), attrs)

				if len(events) > 0 {
					metrics.Events.Add(ctx, int64(len(events)), metric.WithAttributes(
						attribute.String("operation.name", cmd.Name()),
					))
				}

				return events, err
			},
		)
	}, nil
}
