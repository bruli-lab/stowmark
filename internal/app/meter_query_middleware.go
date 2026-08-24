package app

import (
	"context"
	"fmt"
	"time"

	"github.com/bruli-lab/go-core/cqs"
	"github.com/bruli-lab/stowmark/internal/infra/observability"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

func NewMeterQueryMiddleware(provider metric.MeterProvider) (cqs.QueryHandlerMiddleware, error) {
	meter := provider.Meter(instrumentationName)

	metrics, err := observability.NewQueryMetrics(meter)
	if err != nil {
		return nil, fmt.Errorf("create query metrics: %w", err)
	}

	return func(next cqs.QueryHandler) cqs.QueryHandler {
		return cqs.QueryHandlerFunc(
			func(ctx context.Context, query cqs.Query) (any, error) {
				startedAt := time.Now()

				result, err := next.Handle(ctx, query)

				status := "success"
				if err != nil {
					status = "error"
				}

				attrs := metric.WithAttributes(
					attribute.String("operation.name", query.Name()),
					attribute.String("operation.type", "query"),
					attribute.String("operation.status", status),
				)

				metrics.Executions.Add(ctx, 1, attrs)

				metrics.Duration.Record(ctx, time.Since(startedAt).Seconds(), attrs)

				return result, err
			},
		)
	}, nil
}
