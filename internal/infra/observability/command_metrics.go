package observability

import (
	"fmt"

	"go.opentelemetry.io/otel/metric"
)

const (
	operationExecutionsMetric   = "stowmark.operation.executions"
	operationDurationMetric     = "stowmark.operation.duration"
	stowmarkCommandEventsMetric = "stowmark.command.events"
)

type CommandMetrics struct {
	Executions metric.Int64Counter
	Duration metric.Float64Histogram
	Events   metric.Int64Counter
}

func NewCommandMetrics(meter metric.Meter) (*CommandMetrics, error) {
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

	events, err := meter.Int64Counter(
		stowmarkCommandEventsMetric,
		metric.WithDescription("Number of events returned by Stowmark commands"),
		metric.WithUnit("{event}"),
	)
	if err != nil {
		return nil, fmt.Errorf("create command events counter: %w", err)
	}

	return &CommandMetrics{
		Executions: executions,
		Duration:   duration,
		Events:     events,
	}, nil
}
