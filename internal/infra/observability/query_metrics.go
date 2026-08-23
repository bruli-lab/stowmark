package observability

import (
	"fmt"

	"go.opentelemetry.io/otel/metric"
)

type QueryMetrics struct {
	Executions metric.Int64Counter
	Duration   metric.Float64Histogram
}

func NewQueryMetrics(meter metric.Meter) (*QueryMetrics, error) {
	executions, err := meter.Int64Counter(
		operationExecutionsMetric,
		metric.WithDescription("Number of Stowmark operations executed"),
		metric.WithUnit("{operation}"),
	)
	if err != nil {
		return nil, fmt.Errorf("create operation executions counter: %w", err)
	}

	duration, err := meter.Float64Histogram(
		operationDurationMetric,
		metric.WithDescription("Duration of Stowmark operations"),
		metric.WithUnit("s"),
	)
	if err != nil {
		return nil, fmt.Errorf("create operation duration histogram: %w", err)
	}

	return &QueryMetrics{
		Executions: executions,
		Duration:   duration,
	}, nil
}
