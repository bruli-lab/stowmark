package observability

import (
	"context"
	"log/slog"
	"os"

	metricnoop "go.opentelemetry.io/otel/metric/noop"
	tracenoop "go.opentelemetry.io/otel/trace/noop"
)

func newNoop() *OTLPObservability {
	return &OTLPObservability{
		TracerProvider: tracenoop.NewTracerProvider(),
		MeterProvider:  metricnoop.NewMeterProvider(),
		Logger: slog.New(
			slog.NewTextHandler(os.Stderr, nil),
		),
		shutdown: func(context.Context) error {
			return nil
		},
	}
}
