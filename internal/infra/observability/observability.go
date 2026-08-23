package observability

import (
	"context"
	"errors"
	"log/slog"
	"os"

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
	level := logLevel(cfg.LogLevel)

	consoleHandler := slog.NewTextHandler(
		os.Stderr,
		&slog.HandlerOptions{
			Level: level,
		},
	)

	otelHandler := otelslog.NewHandler(
		instrumentationName,
		otelslog.WithLoggerProvider(loggerProvider),
	)

	logger := slog.New(
		newMultiHandler(
			consoleHandler,
			newLevelHandler(level, otelHandler),
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

func logLevel(level string) slog.Level {
	switch level {
	case "info":
		return slog.LevelInfo
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelDebug
	}
}
