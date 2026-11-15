package middlewares

import (
	"fmt"

	"github.com/bruli-lab/go-core/cqs"
	"github.com/bruli-lab/stowmark/internal/app"
	observabilityinfra "github.com/bruli-lab/stowmark/internal/infra/observability"
)

func BuildCommandMiddlewares(obsv *observabilityinfra.OTLPObservability, dispatcher app.EventDispatcher) (cqs.CommandHandlerMiddleware, error) {
	busMdw := app.NewEventBusCommandMiddleware(dispatcher, obsv.Logger)
	loggerMdw := app.NewLoggerCommandMiddleware(obsv.Logger)
	tracerMdw := app.NewTracerCommandMiddleware(obsv.TracerProvider)
	meterMdw, err := NewMeterCommandMiddleware(obsv.MeterProvider)
	if err != nil {
		return nil, fmt.Errorf("create meter middleware: %w", err)
	}
	return cqs.CommandHandlerMultiMiddleware(busMdw, loggerMdw, tracerMdw, meterMdw), nil
}
