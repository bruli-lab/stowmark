package middlewares

import (
	"fmt"

	"github.com/bruli-lab/go-core/cqs"
	"github.com/bruli-lab/stowmark/internal/app"
	observabilityinfra "github.com/bruli-lab/stowmark/internal/infra/observability"
)

func BuildCommandMiddlewares(obsv *observabilityinfra.OTLPObservability, evtr *app.EventsTracing) (cqs.CommandHandlerMiddleware, error) {
	loggerMdw := app.NewLoggerCommandMiddleware(obsv.Logger)
	tracerMdw := app.NewTracerCommandMiddleware(obsv.TracerProvider, evtr)
	meterMdw, err := app.NewMeterCommandMiddleware(obsv.MeterProvider)
	if err != nil {
		return nil, fmt.Errorf("create meter middleware: %w", err)
	}
	multiMdw := cqs.CommandHandlerMultiMiddleware(loggerMdw, tracerMdw, meterMdw)
	return multiMdw, nil
}
