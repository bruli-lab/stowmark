package observability

import (
	"context"
	"errors"
	"log/slog"

	"github.com/bruli-lab/stowmark/internal/config"
	"go.opentelemetry.io/contrib/bridges/otelslog"
)

func New(ctx context.Context, conf *config.ObservabilityConfig) (*OTLPObservability, error) {
	if conf.OTELExporterEndpoint == nil || *conf.OTELExporterEndpoint == "" {
		return newNoop(), nil
	}
	return newOTLP(ctx, *conf)
}

func newOTLP(ctx context.Context, cfg config.ObservabilityConfig) (*OTLPObservability, error) {
	res, err := newResource(ctx, cfg)
	if err != nil {
		return nil, err
	}

	tracerProvider, err := newTracerProvider(ctx, cfg, res)
	if err != nil {
		return nil, err
	}

	meterProvider, err := newMeterProvider(ctx, cfg, res)
	if err != nil {
		_ = tracerProvider.Shutdown(ctx)

		return nil, err
	}

	loggerProvider, err := newLoggerProvider(ctx, cfg, res)
	if err != nil {
		_ = meterProvider.Shutdown(ctx)
		_ = tracerProvider.Shutdown(ctx)

		return nil, err
	}

	consoleHandler := slog.Default().Handler()

	otelHandler := otelslog.NewHandler(
		instrumentationName,
		otelslog.WithLoggerProvider(loggerProvider),
	)

	logger := slog.New(
		newMultiHandler(
			consoleHandler,
			otelHandler,
		),
	)

	shutdown := func(ctx context.Context) error {
		return errors.Join(
			loggerProvider.Shutdown(ctx),
			meterProvider.Shutdown(ctx),
			tracerProvider.Shutdown(ctx),
		)
	}

	return NewOTLPObservability(
		tracerProvider,
		meterProvider,
		loggerProvider,
		logger,
		shutdown,
	), nil
}
