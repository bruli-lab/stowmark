package middlewares

import (
	"context"
	"fmt"

	"github.com/bruli-lab/go-core/event"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

type EventMetricsListener struct {
	dispatched metric.Int64Counter
}

func (e EventMetricsListener) Listen(ctx context.Context, ev event.Event) error {
	e.dispatched.Add(ctx, 1, metric.WithAttributes(attribute.String("event.name", ev.EventName())))
	return nil
}

func NewEventMetricsListener(provider metric.MeterProvider) (*EventMetricsListener, error) {
	counter, err := provider.Meter(instrumentationName).Int64Counter(
		"stowmark.events.dispatched",
		metric.WithDescription("Domain events dispatched to the event bus"),
	)
	if err != nil {
		return nil, fmt.Errorf("create events dispatched counter: %w", err)
	}
	return &EventMetricsListener{dispatched: counter}, nil
}
