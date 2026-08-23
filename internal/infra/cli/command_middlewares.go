package cli

import (
	"fmt"

	"github.com/bruli-lab/go-core/cqs"
	"github.com/bruli-lab/stowmark/internal/app"
	observabilityinfra "github.com/bruli-lab/stowmark/internal/infra/observability"
)

func buildCommandMiddlewares(obsv *observabilityinfra.OTLPObservability, err error) (cqs.CommandHandlerMiddleware, error) {
	loggerMdw := app.NewLoggerCommandMiddleware(obsv.Logger)
	tracerMdw := app.NewTracerCommandMiddleware(obsv.TracerProvider)
	meterMdw, err := app.NewMeterCommandMiddleware(obsv.MeterProvider)
	if err != nil {
		return nil, fmt.Errorf("create meter middleware: %w", err)
	}
	multiMdw := cqs.CommandHandlerMultiMiddleware(loggerMdw, tracerMdw, meterMdw)
	return multiMdw, nil
}
