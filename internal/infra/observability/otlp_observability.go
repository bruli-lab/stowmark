package observability

import (
	"context"
	"log/slog"

	"go.opentelemetry.io/otel/log"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

const instrumentationName = "github.com/bruli-lab/stowmark"

type OTLPObservability struct {
	TracerProvider trace.TracerProvider
	MeterProvider  metric.MeterProvider
	LoggerProvider log.LoggerProvider
	Logger         *slog.Logger
	shutdown       func(context.Context) error
}

func (o OTLPObservability) Tracer(name string) trace.Tracer {
	return o.TracerProvider.Tracer(name)
}

func (o OTLPObservability) Meter(name string) metric.Meter {
	return o.MeterProvider.Meter(name)
}

func (o OTLPObservability) Shutdown(ctx context.Context) error {
	if o.shutdown == nil {
		return nil
	}

	return o.shutdown(ctx)
}

func NewOTLPObservability(
	tracerProvider trace.TracerProvider,
	meterProvider metric.MeterProvider,
	logger *slog.Logger,
	shutdown func(context.Context) error,
) *OTLPObservability {
	return &OTLPObservability{
		TracerProvider: tracerProvider,
		MeterProvider:  meterProvider,
		Logger:         logger,
		shutdown:       shutdown,
	}
}
